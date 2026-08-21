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
	return d.Interval, d.Timeout
}

func (d *ClientDriver) CreateInstance(ctx context.Context, req CreateInstanceRequest) (Instance, error) {
	return d.Client.CreateInstance(ctx, req)
}
func (d *ClientDriver) GetInstance(ctx context.Context, id string) (Instance, error) {
	return d.Client.GetInstance(ctx, id)
}
func (d *ClientDriver) WaitInstance(ctx context.Context, id string, pred func(Instance) bool) (Instance, error) {
	var last Instance
	interval, timeout := d.wait()
	err := poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		got, err := d.Client.GetInstance(ctx, id)
		if err != nil {
			return false, err
		}
		last = got
		return pred(got), nil
	})
	return last, err
}
func (d *ClientDriver) StopInstance(ctx context.Context, id string) error {
	if err := d.Client.SetPower(ctx, id, "power_off"); err != nil {
		return err
	}
	_, err := d.WaitInstance(ctx, id, Instance.Stopped)
	return err
}
func (d *ClientDriver) DeleteInstance(ctx context.Context, id string) error {
	return d.Client.DeleteInstance(ctx, id)
}
func (d *ClientDriver) GetDisk(ctx context.Context, id string) (Disk, error) {
	return d.Client.GetDisk(ctx, id)
}
func (d *ClientDriver) FindDisk(ctx context.Context, name string) (Disk, error) {
	return d.Client.FindDisk(ctx, name)
}
func (d *ClientDriver) DetachDisk(ctx context.Context, instanceID, diskID string) error {
	if err := d.Client.DetachDisk(ctx, instanceID, diskID); err != nil {
		return err
	}
	_, err := d.WaitDisk(ctx, diskID, Disk.Available)
	return err
}
func (d *ClientDriver) WaitDisk(ctx context.Context, id string, pred func(Disk) bool) (Disk, error) {
	var last Disk
	interval, timeout := d.wait()
	err := poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		got, err := d.Client.GetDisk(ctx, id)
		if err != nil {
			return false, err
		}
		last = got
		return pred(got), nil
	})
	return last, err
}
func (d *ClientDriver) DeleteDisk(ctx context.Context, id string) error {
	return d.Client.DeleteDisk(ctx, id)
}
func (d *ClientDriver) GetImage(ctx context.Context, id string) (Image, error) {
	return d.Client.GetImage(ctx, id)
}
func (d *ClientDriver) CreateImage(ctx context.Context, req CreateImageRequest) (Image, error) {
	return d.Client.CreateImage(ctx, req)
}
func (d *ClientDriver) WaitImage(ctx context.Context, id string) (Image, error) {
	var last Image
	interval, timeout := d.wait()
	err := poll(ctx, interval, timeout, func(ctx context.Context) (bool, error) {
		got, err := d.Client.GetImage(ctx, id)
		if err != nil {
			return false, err
		}
		last = got
		if got.Failed() {
			return false, fmtImageFailed(got)
		}
		return got.Ready(), nil
	})
	return last, err
}
func (d *ClientDriver) DeleteImage(ctx context.Context, id string) error {
	return d.Client.DeleteImage(ctx, id)
}
func (d *ClientDriver) CreateFloatingIP(ctx context.Context, name, interfaceID, zone string) (FloatingIP, error) {
	return d.Client.CreateFloatingIP(ctx, name, interfaceID, zone)
}
func (d *ClientDriver) DeleteFloatingIP(ctx context.Context, id string) error {
	return d.Client.DeleteFloatingIP(ctx, id)
}

func fmtImageFailed(img Image) error {
	return &APIError{
		Status:  409,
		Code:    "image_failed",
		Message: fmt.Sprintf("image %s entered a failed zone state: %v", img.ID, img.ZoneStates),
	}
}
