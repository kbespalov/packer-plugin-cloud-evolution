// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"bytes"
	"context"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
)

func testState(t *testing.T, driver Driver, cfg *Config) *multistep.BasicStateBag {
	t.Helper()
	ui := &packer.BasicUi{
		Reader:      bytes.NewReader(nil),
		Writer:      &bytes.Buffer{},
		ErrorWriter: &bytes.Buffer{},
	}
	state := new(multistep.BasicStateBag)
	state.Put("ui", ui)
	state.Put("driver", driver)
	state.Put("config", cfg)
	return state
}

func testCfg() *Config {
	useFIP := true
	return &Config{
		ProjectID:     "proj",
		Zone:          "az-1",
		SubnetID:      "sn",
		FlavorID:      "fl",
		SourceImageID: "src-img",
		ImageName:     "Golden",
		InstanceName:  "packer-test",
		DiskSizeGb:    10,
		DiskType:      "SSD",
		LinuxLogin:    "ubuntu",
		UseFloatingIP: &useFIP,
	}
}

func TestStepCreateInstanceHappyPath(t *testing.T) {
	t.Parallel()
	driver := NewFakeDriver()
	driver.images["src-img"] = Image{ID: "src-img", Name: "Ubuntu-24.04", Type: "public", UserDataTemplate: "#cloud-config"}
	cfg := testCfg()
	cfg.Comm.SSHPublicKey = []byte("ssh-ed25519 AAAA")
	state := testState(t, driver, cfg)
	step := &stepCreateInstance{GeneratedData: &packerbuilderdata.GeneratedData{State: state}}
	if action := step.Run(context.Background(), state); action != multistep.ActionContinue {
		t.Fatalf("action=%v err=%v", action, state.Get("error"))
	}
	inst := state.Get("instance").(Instance)
	if inst.ID == "" || inst.PrivateIP == "" || state.Get("disk_id") == "" {
		t.Fatalf("state instance=%+v disk=%v", inst, state.Get("disk_id"))
	}
	step.Cleanup(state)
	if _, err := driver.GetInstance(context.Background(), inst.ID); err == nil {
		t.Fatal("cleanup should delete the instance")
	}
}

func TestStepCreateInstanceRequiresKey(t *testing.T) {
	t.Parallel()
	state := testState(t, NewFakeDriver(), testCfg())
	step := &stepCreateInstance{}
	if action := step.Run(context.Background(), state); action != multistep.ActionHalt {
		t.Fatal("expected halt without ssh key")
	}
}

func TestBakeStepsProducePrivateImage(t *testing.T) {
	t.Parallel()
	driver := NewFakeDriver()
	driver.images["src-img"] = Image{ID: "src-img", Name: "Ubuntu-24.04", Type: "public", UserDataTemplate: "tpl"}
	cfg := testCfg()
	cfg.Comm.SSHPublicKey = []byte("ssh-ed25519 AAAA")
	state := testState(t, driver, cfg)
	gen := &packerbuilderdata.GeneratedData{State: state}

	steps := []multistep.Step{
		&stepCreateInstance{GeneratedData: gen},
		&stepFloatingIP{},
		&stepStopInstance{},
		&stepDetachDisk{},
		&stepCreateImage{GeneratedData: gen},
	}
	for i, s := range steps {
		if action := s.Run(context.Background(), state); action != multistep.ActionContinue {
			t.Fatalf("step %d halt: %v", i, state.Get("error"))
		}
	}
	img := state.Get("image").(Image)
	if img.ID == "" || img.Type != "private" || img.Public {
		t.Fatalf("image %+v", img)
	}
	if img.UserDataTemplate != "tpl" {
		t.Fatalf("template %q", img.UserDataTemplate)
	}
	if ip := state.Get("instance_ip").(string); ip != "203.0.113.10" {
		t.Fatalf("fip %s", ip)
	}
	disk, err := driver.GetDisk(context.Background(), state.Get("disk_id").(string))
	if err != nil || !disk.Available() {
		t.Fatalf("disk should be detached: %+v %v", disk, err)
	}
}

func TestCommHost(t *testing.T) {
	t.Parallel()
	state := new(multistep.BasicStateBag)
	if _, err := commHost(state); err == nil {
		t.Fatal("expected error")
	}
	state.Put("instance_ip", "198.51.100.2")
	host, err := commHost(state)
	if err != nil || host != "198.51.100.2" {
		t.Fatalf("%s %v", host, err)
	}
}

func TestStepCleanupDeletesDiskAfterSkip(t *testing.T) {
	t.Parallel()
	driver := NewFakeDriver()
	driver.images["src-img"] = Image{ID: "src-img", Name: "Ubuntu-24.04", Type: "public"}
	cfg := testCfg()
	cfg.SkipCreateImage = true
	cfg.Comm.SSHPublicKey = []byte("ssh-ed25519 AAAA")
	state := testState(t, driver, cfg)
	step := &stepCreateInstance{}
	if action := step.Run(context.Background(), state); action != multistep.ActionContinue {
		t.Fatalf("action=%v err=%v", action, state.Get("error"))
	}
	diskID := state.Get("disk_id").(string)
	step.Cleanup(state)
	if _, err := driver.GetDisk(context.Background(), diskID); err == nil {
		t.Fatal("cleanup should delete the boot disk even when skip_create_image is set")
	}
}

func TestArtifact(t *testing.T) {
	t.Parallel()
	driver := NewFakeDriver()
	driver.images["img-1"] = Image{ID: "img-1", Name: "Golden", Type: "private"}
	a := &Artifact{Image: Image{ID: "img-1", Name: "Golden", Type: "private"}, driver: driver}
	if a.BuilderId() != BuilderID || a.Id() != "img-1" {
		t.Fatalf("%s %s", a.BuilderId(), a.Id())
	}
	if a.State("ImageType") != "private" {
		t.Fatal(a.State("ImageType"))
	}
	if err := a.Destroy(); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.GetImage(context.Background(), "img-1"); err == nil {
		t.Fatal("destroy should delete the image")
	}
}
