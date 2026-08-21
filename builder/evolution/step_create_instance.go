// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
)

type stepCreateInstance struct {
	GeneratedData *packerbuilderdata.GeneratedData
}

func (s *stepCreateInstance) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	cfg := state.Get("config").(*Config)
	driver := state.Get("driver").(Driver)

	pub := strings.TrimSpace(string(cfg.Comm.SSHPublicKey))
	if pub == "" {
		return stepHalt(state, fmt.Errorf("ssh public key is empty; communicator must generate or load a key before create"))
	}

	source, err := driver.GetImage(ctx, cfg.SourceImageID)
	if err != nil {
		return stepHalt(state, fmt.Errorf("source image: %w", err))
	}
	state.Put("source_image", source)
	if s.GeneratedData != nil {
		s.GeneratedData.Put("SourceImageID", source.ID)
		s.GeneratedData.Put("SourceImageName", source.Name)
	}
	ui.Say(fmt.Sprintf("Using source image %s (%s, type=%s)", source.ID, source.Name, source.Type))

	diskName := cfg.InstanceName + "-boot"
	ui.Say(fmt.Sprintf("Creating instance %s...", cfg.InstanceName))
	inst, err := driver.CreateInstance(ctx, CreateInstanceRequest{
		Name:             cfg.InstanceName,
		ImageID:          cfg.SourceImageID,
		FlavorID:         cfg.FlavorID,
		SubnetID:         cfg.SubnetID,
		Zone:             cfg.Zone,
		DiskName:         diskName,
		DiskSizeGiB:      cfg.DiskSizeGb,
		DiskType:         cfg.DiskType,
		SecurityGroupIDs: cfg.SecurityGroupIDs,
		Hostname:         cfg.InstanceName,
		LinuxLogin:       cfg.LinuxLogin,
		PublicKey:        pub,
	})
	if err != nil {
		return stepHalt(state, fmt.Errorf("create instance: %w", err))
	}
	state.Put("instance_id", inst.ID)
	state.Put("instance_name", inst.Name)
	// instance_id is the name provisioners expect (same as Yandex).
	ui.Say(fmt.Sprintf("Waiting for instance %s to become running...", inst.ID))
	ready, err := driver.WaitInstance(ctx, inst.ID, Instance.Running)
	if err != nil {
		return stepHalt(state, fmt.Errorf("wait instance: %w", err))
	}
	if ready.BootDiskID == "" {
		disk, diskErr := driver.FindDisk(ctx, diskName)
		if diskErr != nil {
			return stepHalt(state, fmt.Errorf("find boot disk: %w", diskErr))
		}
		ready.BootDiskID = disk.ID
		ready.BootDiskName = disk.Name
	}
	state.Put("instance", ready)
	state.Put("disk_id", ready.BootDiskID)
	state.Put("instance_private_ip", ready.PrivateIP)
	ui.Say(fmt.Sprintf("Instance running private_ip=%s boot_disk=%s", ready.PrivateIP, ready.BootDiskID))
	return multistep.ActionContinue
}

func (s *stepCreateInstance) Cleanup(state multistep.StateBag) {
	ui := uiFromState(state)
	driver := state.Get("driver").(Driver)
	ctx := context.Background()

	if raw, ok := state.GetOk("floating_ip_id"); ok {
		id := raw.(string)
		if id != "" {
			ui.Say("Deleting floating IP...")
			if err := driver.DeleteFloatingIP(ctx, id); err != nil {
				ui.Error(fmt.Sprintf("delete floating IP %s: %s (delete it manually)", id, err))
			}
		}
	}
	if raw, ok := state.GetOk("instance_id"); ok {
		id := raw.(string)
		if id != "" {
			ui.Say("Destroying instance...")
			_ = driver.StopInstance(ctx, id)
			if err := driver.DeleteInstance(ctx, id); err != nil {
				ui.Error(fmt.Sprintf("delete instance %s: %s (delete it manually)", id, err))
			}
		}
	}
	if raw, ok := state.GetOk("disk_id"); ok {
		id := raw.(string)
		if id != "" {
			ui.Say("Destroying boot disk...")
			if err := driver.DeleteDisk(ctx, id); err != nil {
				ui.Error(fmt.Sprintf("delete disk %s: %s (delete it manually)", id, err))
			}
		}
	}
}
