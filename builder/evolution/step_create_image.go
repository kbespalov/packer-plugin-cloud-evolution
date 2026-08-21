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
	diskID := state.Get("disk_id").(string)
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
		return stepHalt(state, fmt.Errorf("create image: %w", err))
	}
	ui.Say(fmt.Sprintf("Waiting for image %s (this often takes several minutes)...", img.ID))
	ready, err := driver.WaitImage(ctx, img.ID)
	if err != nil {
		return stepHalt(state, fmt.Errorf("wait image: %w", err))
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

func (s *stepCreateImage) Cleanup(multistep.StateBag) {}
