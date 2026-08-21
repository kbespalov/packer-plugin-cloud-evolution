// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepDetachDisk struct{}

func (s *stepDetachDisk) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	driver := state.Get("driver").(Driver)
	cfg := state.Get("config").(*Config)
	if cfg.SkipCreateImage {
		return multistep.ActionContinue
	}
	instanceID := state.Get("instance_id").(string)
	diskID := state.Get("disk_id").(string)
	ui.Say("Detaching boot disk (image create requires state=available)...")
	if err := driver.DetachDisk(ctx, instanceID, diskID); err != nil {
		return stepHalt(state, fmt.Errorf("detach boot disk: %w", err))
	}
	return multistep.ActionContinue
}

func (s *stepDetachDisk) Cleanup(multistep.StateBag) {}
