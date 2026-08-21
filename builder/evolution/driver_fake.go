// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// FakeDriver is an in-memory Driver for unit tests. It does not speak HTTP.
type FakeDriver struct {
	mu         sync.Mutex
	instances  map[string]Instance
	disks      map[string]Disk
	images     map[string]Image
	fips       map[string]FloatingIP
	createErr  error
	imageReady bool
	next       int
}

func NewFakeDriver() *FakeDriver {
	return &FakeDriver{
		instances:  map[string]Instance{},
		disks:      map[string]Disk{},
		images:     map[string]Image{},
		fips:       map[string]FloatingIP{},
		imageReady: true,
	}
}

func (f *FakeDriver) id(prefix string) string {
	f.next++
	return fmt.Sprintf("%s-%d", prefix, f.next)
}

func (f *FakeDriver) CreateInstance(_ context.Context, req CreateInstanceRequest) (Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return Instance{}, f.createErr
	}
	diskID := f.id("disk")
	vmID := f.id("vm")
	disk := Disk{ID: diskID, Name: req.DiskName, State: "in_use", VMID: vmID, Bootable: true}
	inst := Instance{
		ID:           vmID,
		Name:         req.Name,
		State:        "running",
		PrivateIP:    "10.0.0.8",
		InterfaceID:  f.id("iface"),
		BootDiskID:   diskID,
		BootDiskName: req.DiskName,
	}
	f.disks[diskID] = disk
	f.instances[vmID] = inst
	return inst, nil
}

func (f *FakeDriver) GetInstance(_ context.Context, id string) (Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.instances[id]
	if !ok {
		return Instance{}, &APIError{Status: http.StatusNotFound, Code: "not_found"}
	}
	return got, nil
}

func (f *FakeDriver) WaitInstance(ctx context.Context, id string, pred func(Instance) bool) (Instance, error) {
	got, err := f.GetInstance(ctx, id)
	if err != nil {
		return Instance{}, err
	}
	if pred != nil && !pred(got) {
		return got, fmt.Errorf("instance %s state %s did not match", id, got.State)
	}
	return got, nil
}

func (f *FakeDriver) StopInstance(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.instances[id]
	if !ok {
		return &APIError{Status: http.StatusNotFound, Code: "not_found"}
	}
	got.State = "stopped"
	f.instances[id] = got
	return nil
}

func (f *FakeDriver) DeleteInstance(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.instances, id)
	for diskID, d := range f.disks {
		if d.VMID == id {
			d.State = "available"
			d.VMID = ""
			f.disks[diskID] = d
		}
	}
	return nil
}

func (f *FakeDriver) GetDisk(_ context.Context, id string) (Disk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.disks[id]
	if !ok {
		return Disk{}, &APIError{Status: http.StatusNotFound, Code: "not_found"}
	}
	return got, nil
}

func (f *FakeDriver) FindDisk(_ context.Context, name string) (Disk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.disks {
		if d.Name == name {
			return d, nil
		}
	}
	return Disk{}, &APIError{Status: http.StatusNotFound, Code: "not_found"}
}

func (f *FakeDriver) DetachDisk(_ context.Context, _, diskID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.disks[diskID]
	if !ok {
		return &APIError{Status: http.StatusNotFound, Code: "not_found"}
	}
	got.State = "available"
	got.VMID = ""
	f.disks[diskID] = got
	return nil
}

func (f *FakeDriver) WaitDisk(ctx context.Context, id string, pred func(Disk) bool) (Disk, error) {
	got, err := f.GetDisk(ctx, id)
	if err != nil {
		return Disk{}, err
	}
	if pred != nil && !pred(got) {
		return got, fmt.Errorf("disk %s state %s did not match", id, got.State)
	}
	return got, nil
}

func (f *FakeDriver) DeleteDisk(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.disks, id)
	return nil
}

func (f *FakeDriver) GetImage(_ context.Context, id string) (Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.images[id]
	if !ok {
		return Image{}, &APIError{Status: http.StatusNotFound, Code: "not_found"}
	}
	return got, nil
}

func (f *FakeDriver) CreateImage(_ context.Context, req CreateImageRequest) (Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.id("img")
	state := "creating"
	if f.imageReady {
		state = "created"
	}
	img := Image{
		ID:               id,
		Name:             req.Name,
		Type:             "private",
		Public:           false,
		UserDataTemplate: req.UserDataTemplate,
		ZoneStates:       map[string]string{req.Zone: state},
	}
	f.images[id] = img
	return img, nil
}

func (f *FakeDriver) WaitImage(ctx context.Context, id string) (Image, error) {
	got, err := f.GetImage(ctx, id)
	if err != nil {
		return Image{}, err
	}
	if !got.Ready() {
		return got, fmt.Errorf("image %s not ready: %v", id, got.ZoneStates)
	}
	return got, nil
}

func (f *FakeDriver) DeleteImage(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.images, id)
	return nil
}

func (f *FakeDriver) CreateFloatingIP(_ context.Context, name, _, _ string) (FloatingIP, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.id("fip")
	fip := FloatingIP{ID: id, Name: name, IPAddress: "203.0.113.10"}
	f.fips[id] = fip
	return fip, nil
}

func (f *FakeDriver) DeleteFloatingIP(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.fips, id)
	return nil
}
