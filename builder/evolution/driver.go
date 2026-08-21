// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"
	"time"
)

// Driver is the Compute surface the builder steps need. Production uses
// Client; tests use a fake. Modeled after packer-plugin-yandex's Driver.
type Driver interface {
	CreateInstance(ctx context.Context, req CreateInstanceRequest) (Instance, error)
	GetInstance(ctx context.Context, id string) (Instance, error)
	WaitInstance(ctx context.Context, id string, pred func(Instance) bool) (Instance, error)
	StopInstance(ctx context.Context, id string) error
	DeleteInstance(ctx context.Context, id string) error

	GetDisk(ctx context.Context, id string) (Disk, error)
	FindDisk(ctx context.Context, name string) (Disk, error)
	DetachDisk(ctx context.Context, instanceID, diskID string) error
	WaitDisk(ctx context.Context, id string, pred func(Disk) bool) (Disk, error)
	DeleteDisk(ctx context.Context, id string) error

	GetImage(ctx context.Context, id string) (Image, error)
	FindImage(ctx context.Context, name string) (Image, error)
	CreateImage(ctx context.Context, req CreateImageRequest) (Image, error)
	WaitImage(ctx context.Context, id string) (Image, error)
	DeleteImage(ctx context.Context, id string) error

	CreateFloatingIP(ctx context.Context, name, interfaceID, zone string) (FloatingIP, error)
	DeleteFloatingIP(ctx context.Context, id string) error
}

// ClientDriver wraps Client with poll intervals from the Packer config.
type ClientDriver struct {
	Client   *Client
	Interval time.Duration
	Timeout  time.Duration
}

func (d *ClientDriver) wait() (time.Duration, time.Duration) {
	interval, timeout := d.Interval, d.Timeout
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	if interval > timeout {
		interval = timeout
	}
	return interval, timeout
}

func (d *ClientDriver) CreateInstance(ctx context.Context, req CreateInstanceRequest) (Instance, error) {
	return d.Client.CreateInstance(ctx, req)
}
func (d *ClientDriver) GetInstance(ctx context.Context, id string) (Instance, error) {
	return d.Client.GetInstance(ctx, id)
}
func (d *ClientDriver) WaitInstance(ctx context.Context, id string, pred func(Instance) bool) (Instance, error) {
	var last Instance
	seen := false
	interval, timeout := d.wait()
	err := poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		got, err := d.Client.GetInstance(ctx, id)
		if err != nil {
			if isNotFound(err) {
				if seen {
					return false, fmt.Errorf("instance %s disappeared", id)
				}
				// Create is eventually consistent: GET 404 for a few seconds.
				return false, nil
			}
			return false, err
		}
		seen = true
		last = got
		if got.Failed() {
			return false, fmt.Errorf("instance %s entered failed state %q", got.ID, got.State)
		}
		if pred == nil {
			return true, nil
		}
		return pred(got), nil
	})
	return last, err
}
func (d *ClientDriver) StopInstance(ctx context.Context, id string) error {
	got, err := d.Client.GetInstance(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if got.Failed() {
		return fmt.Errorf("instance %s entered failed state %q", got.ID, got.State)
	}
	if got.Stopped() {
		return nil
	}
	if !got.Stopping() {
		if err := d.Client.SetPower(ctx, id, "power_off"); err != nil {
			got, getErr := d.Client.GetInstance(ctx, id)
			if getErr == nil && (got.Stopped() || got.Stopping()) {
				// power_off raced with an in-flight stop
			} else if isNotFound(err) || isNotFound(getErr) {
				return nil
			} else {
				return err
			}
		}
	}
	_, err = d.WaitInstance(ctx, id, Instance.Stopped)
	return err
}
func (d *ClientDriver) DeleteInstance(ctx context.Context, id string) error {
	interval, timeout := d.wait()
	err := poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		err := d.Client.DeleteInstance(ctx, id)
		if err == nil {
			return true, nil
		}
		if api, ok := AsAPIError(err); ok && api.NotFound() {
			return true, nil
		}
		// After FIP delete, Evolution briefly refuses VM delete on a stopped NIC.
		if api, ok := AsAPIError(err); ok && api.Code == "floating_ip_can_not_be_detached_from_vm_in_current_state" {
			return false, nil
		}
		if api, ok := AsAPIError(err); ok && api.Retryable() {
			return false, nil
		}
		return false, err
	})
	if err != nil {
		return err
	}
	return poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		_, err := d.Client.GetInstance(ctx, id)
		if err == nil {
			return false, nil
		}
		if api, ok := AsAPIError(err); ok && api.NotFound() {
			return true, nil
		}
		if api, ok := AsAPIError(err); ok && api.Retryable() {
			return false, nil
		}
		return false, err
	})
}
func (d *ClientDriver) GetDisk(ctx context.Context, id string) (Disk, error) {
	return d.Client.GetDisk(ctx, id)
}
func (d *ClientDriver) FindDisk(ctx context.Context, name string) (Disk, error) {
	return d.Client.FindDisk(ctx, name)
}
func (d *ClientDriver) DetachDisk(ctx context.Context, instanceID, diskID string) error {
	interval, timeout := d.wait()
	return poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		disk, err := d.Client.GetDisk(ctx, diskID)
		if err != nil {
			return false, err
		}
		if disk.Failed() {
			return false, fmt.Errorf("disk %s entered failed state %q", disk.ID, disk.State)
		}
		if disk.Available() {
			return true, nil
		}
		err = d.Client.DetachDisk(ctx, instanceID, diskID)
		if err != nil && !detachAlreadyDone(err) {
			if resourceBusy(err) {
				return false, nil
			}
			fresh, getErr := d.Client.GetDisk(ctx, diskID)
			if getErr == nil && fresh.Available() {
				return true, nil
			}
			return false, err
		}
		fresh, getErr := d.Client.GetDisk(ctx, diskID)
		if getErr != nil {
			return false, getErr
		}
		if fresh.Failed() {
			return false, fmt.Errorf("disk %s entered failed state %q", fresh.ID, fresh.State)
		}
		return fresh.Available(), nil
	})
}
func (d *ClientDriver) WaitDisk(ctx context.Context, id string, pred func(Disk) bool) (Disk, error) {
	var last Disk
	seen := false
	interval, timeout := d.wait()
	err := poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		got, err := d.Client.GetDisk(ctx, id)
		if err != nil {
			if isNotFound(err) {
				if seen {
					return false, fmt.Errorf("disk %s disappeared", id)
				}
				return false, nil
			}
			return false, err
		}
		seen = true
		last = got
		if got.Failed() {
			return false, fmt.Errorf("disk %s entered failed state %q", got.ID, got.State)
		}
		if pred == nil {
			return true, nil
		}
		return pred(got), nil
	})
	return last, err
}
func (d *ClientDriver) DeleteDisk(ctx context.Context, id string) error {
	interval, timeout := d.wait()
	return poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		disk, err := d.Client.GetDisk(ctx, id)
		if err != nil {
			if isNotFound(err) {
				return true, nil
			}
			return false, err
		}
		if disk.InUse() {
			return false, nil
		}
		err = d.Client.DeleteDisk(ctx, id)
		if err == nil || isNotFound(err) {
			return true, nil
		}
		if resourceBusy(err) {
			return false, nil
		}
		return false, err
	})
}
func (d *ClientDriver) GetImage(ctx context.Context, id string) (Image, error) {
	return d.Client.GetImage(ctx, id)
}
func (d *ClientDriver) FindImage(ctx context.Context, name string) (Image, error) {
	return d.Client.FindImage(ctx, name)
}
func (d *ClientDriver) CreateImage(ctx context.Context, req CreateImageRequest) (Image, error) {
	return d.Client.CreateImage(ctx, req)
}
func (d *ClientDriver) WaitImage(ctx context.Context, id string) (Image, error) {
	var last Image
	seen := false
	interval, timeout := d.wait()
	err := poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		got, err := d.Client.GetImage(ctx, id)
		if err != nil {
			if isNotFound(err) {
				if seen {
					return false, fmt.Errorf("image %s disappeared", id)
				}
				return false, nil
			}
			return false, err
		}
		seen = true
		last = got
		if got.Failed() {
			return false, fmtImageFailed(got)
		}
		return got.Ready(), nil
	})
	return last, err
}
func (d *ClientDriver) DeleteImage(ctx context.Context, id string) error {
	err := d.Client.DeleteImage(ctx, id)
	if isNotFound(err) {
		return nil
	}
	return err
}
func (d *ClientDriver) CreateFloatingIP(ctx context.Context, name, interfaceID, zone string) (FloatingIP, error) {
	return d.Client.CreateFloatingIP(ctx, name, interfaceID, zone)
}
func (d *ClientDriver) DeleteFloatingIP(ctx context.Context, id string) error {
	err := d.Client.DeleteFloatingIP(ctx, id)
	if isNotFound(err) {
		return nil
	}
	return err
}

func fmtImageFailed(img Image) error {
	return &APIError{
		Status:  409,
		Code:    "image_failed",
		Message: fmt.Sprintf("image %s entered a failed zone state: %v", img.ID, img.ZoneStates),
	}
}
