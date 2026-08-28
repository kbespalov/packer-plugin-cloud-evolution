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
	if strings.Contains(pub, "PRIVATE KEY") {
		return stepHalt(state, fmt.Errorf("ssh public key looks like a private key; pass the .pub material"))
	}

	source, err := driver.GetImage(ctx, cfg.SourceImageID)
	if err != nil {
		if isNotFound(err) {
			return stepHalt(state, fmt.Errorf("source_image_id %q was not found", cfg.SourceImageID))
		}
		return stepHalt(state, fmt.Errorf("source image: %w", err))
	}
	if source.Failed() {
		return stepHalt(state, fmt.Errorf("source image %s is in a failed zone state: %v", source.ID, source.ZoneStates))
	}
	if source.MinDiskGiB > 0 && cfg.DiskSizeGb < source.MinDiskGiB {
		return stepHalt(state, fmt.Errorf("disk_size_gb %d is smaller than source image min_disk %d", cfg.DiskSizeGb, source.MinDiskGiB))
	}
	if st, ok := source.ZoneStates[cfg.Zone]; ok && imageZoneFailed(st) {
		return stepHalt(state, fmt.Errorf("source image %s is not available in zone %s (state=%s)", source.ID, cfg.Zone, st))
	}
	state.Put("source_image", source)
	if s.GeneratedData != nil {
		s.GeneratedData.Put("SourceImageID", source.ID)
		s.GeneratedData.Put("SourceImageName", source.Name)
	}
	ui.Say(fmt.Sprintf("Using source image %s (%s, type=%s)", source.ID, source.Name, source.Type))

	if !cfg.SkipCreateImage {
		found, findErr := driver.FindImage(ctx, cfg.ImageName)
		exists, findErr := lookupExists(found.ID, findErr)
		if findErr != nil {
			return stepHalt(state, fmt.Errorf("check image_name %q: %w", cfg.ImageName, findErr))
		}
		if exists {
			return stepHalt(state, fmt.Errorf("image_name %q already exists (id=%s); Evolution names are unique per project", cfg.ImageName, found.ID))
		}
	}

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
		err = annotateQuota(err)
		if alreadyExists(err) {
			return stepHalt(state, fmt.Errorf("instance_name %q already exists: %w", cfg.InstanceName, err))
		}
		return stepHalt(state, fmt.Errorf("create instance: %w", err))
	}
	state.Put("instance_id", inst.ID)
	state.Put("instance_name", inst.Name)
	// Track the boot disk before the wait: when WaitInstance fails, Cleanup
	// must still delete the disk (VM delete does not always cascade).
	if inst.BootDiskID != "" {
		state.Put("disk_id", inst.BootDiskID)
	}
	ui.Say(fmt.Sprintf("Waiting for instance %s (NIC + private IP)...", inst.ID))
	ready, err := driver.WaitInstance(ctx, inst.ID, Instance.Provisionable)
	if err != nil {
		return stepHalt(state, fmt.Errorf("wait instance: %w", err))
	}
	if ready.BootDiskID == "" {
		disk, diskErr := driver.FindDisk(ctx, diskName)
		if diskErr != nil {
			return stepHalt(state, fmt.Errorf("find boot disk %q: %w", diskName, diskErr))
		}
		ready.BootDiskID = disk.ID
		ready.BootDiskName = disk.Name
	}
	if ready.BootDiskID == "" {
		return stepHalt(state, fmt.Errorf("instance %s has no boot disk id", ready.ID))
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
	ctx, cancel := cleanupContext(state)
	defer cancel()

	// Live: DELETE FIP on a stopped VM is 204. Immediate DELETE VM then
	// returns 422 floating_ip_can_not_be_detached_from_vm_in_current_state
	// until the NIC drops the address. Driver retries that.
	if raw, ok := state.GetOk("floating_ip_id"); ok {
		id := raw.(string)
		if id != "" {
			ui.Say("Deleting floating IP...")
			if err := ignoreNotFound(driver.DeleteFloatingIP(ctx, id)); err != nil {
				ui.Error(fmt.Sprintf("delete floating IP %s: %s (delete it manually)", id, err))
			}
		}
	}
	if raw, ok := state.GetOk("instance_id"); ok {
		id := raw.(string)
		if id != "" {
			ui.Say("Destroying instance...")
			_ = ignoreNotFound(driver.StopInstance(ctx, id))
			if err := ignoreNotFound(driver.DeleteInstance(ctx, id)); err != nil {
				ui.Error(fmt.Sprintf("delete instance %s: %s (delete it manually)", id, err))
			}
		}
	}
	if raw, ok := state.GetOk("disk_id"); ok {
		id := raw.(string)
		if id != "" {
			ui.Say("Destroying boot disk...")
			if _, err := driver.WaitDisk(ctx, id, Disk.Available); err != nil && !isNotFound(err) {
				ui.Error(fmt.Sprintf("wait disk %s available: %s", id, err))
			}
			if err := ignoreNotFound(driver.DeleteDisk(ctx, id)); err != nil {
				ui.Error(fmt.Sprintf("delete disk %s: %s (delete it manually)", id, err))
			}
		}
	}
}
