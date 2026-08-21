// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
)

type stepCreateImage struct {
	GeneratedData *packerbuilderdata.GeneratedData
}

func (s *stepCreateImage) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	cfg := state.Get("config").(*Config)
	driver := state.Get("driver").(Driver)
	if cfg.SkipCreateImage {
		ui.Say("Skipping image creation")
		return multistep.ActionContinue
	}
	found, err := driver.FindImage(ctx, cfg.ImageName)
	exists, err := lookupExists(found.ID, err)
	if err != nil {
		return stepHalt(state, fmt.Errorf("check image_name %q: %w", cfg.ImageName, err))
	}
	if exists {
		return stepHalt(state, fmt.Errorf("image_name %q already exists (id=%s); Evolution names are unique per project", cfg.ImageName, found.ID))
	}
	rawDisk, ok := state.GetOk("disk_id")
	diskID, _ := rawDisk.(string)
	if !ok || diskID == "" {
		return stepHalt(state, fmt.Errorf("disk_id is not set; cannot create image"))
	}
	template := ""
	if raw, ok := state.GetOk("source_image"); ok {
		template = raw.(Image).UserDataTemplate
	}
	ui.Say(fmt.Sprintf("Creating private image %s from disk %s...", cfg.ImageName, diskID))
	img, err := driver.CreateImage(ctx, CreateImageRequest{
		Name:             cfg.ImageName,
		Description:      cfg.ImageDescription,
		DiskID:           diskID,
		Zone:             cfg.Zone,
		MinCPU:           1,
		MinRAM:           1,
		MinDiskGiB:       cfg.DiskSizeGb,
		UserDataTemplate: template,
	})
	if err != nil {
		err = annotateQuota(err)
		if alreadyExists(err) {
			return stepHalt(state, fmt.Errorf("image_name %q already exists: %w", cfg.ImageName, err))
		}
		return stepHalt(state, fmt.Errorf("create image: %w", err))
	}
	// Track immediately so Cleanup can drop a failed catalog entry (quota).
	state.Put("image_id", img.ID)
	ui.Say(fmt.Sprintf("Waiting for image %s (this often takes several minutes)...", img.ID))
	ready, err := driver.WaitImage(ctx, img.ID)
	if err != nil {
		return stepHalt(state, fmt.Errorf("wait image: %w", annotateQuota(err)))
	}
	if ready.Public {
		return stepHalt(state, fmt.Errorf("created image %s is public; expected private", ready.ID))
	}
	state.Put("image", ready)
	if s.GeneratedData != nil {
		s.GeneratedData.Put("ImageID", ready.ID)
		s.GeneratedData.Put("ImageName", ready.Name)
		s.GeneratedData.Put("ImageType", ready.Type)
	}
	ui.Say(fmt.Sprintf("Image ready id=%s name=%s type=%s", ready.ID, ready.Name, ready.Type))
	return multistep.ActionContinue
}

func (s *stepCreateImage) Cleanup(state multistep.StateBag) {
	if _, ok := state.GetOk("image"); ok {
		return
	}
	raw, ok := state.GetOk("image_id")
	if !ok {
		return
	}
	id, _ := raw.(string)
	if id == "" {
		return
	}
	ui := uiFromState(state)
	driver := state.Get("driver").(Driver)
	ctx, cancel := cleanupContext(state)
	defer cancel()
	ui.Say(fmt.Sprintf("Deleting incomplete image %s...", id))
	if err := ignoreNotFound(driver.DeleteImage(ctx, id)); err != nil {
		ui.Error(fmt.Sprintf("delete image %s: %s (delete it manually; it still counts against custom-image quota)", id, err))
	}
}
