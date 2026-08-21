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
	rawInst, okInst := state.GetOk("instance_id")
	rawDisk, okDisk := state.GetOk("disk_id")
	instanceID, _ := rawInst.(string)
	diskID, _ := rawDisk.(string)
	if !okInst || !okDisk || instanceID == "" || diskID == "" {
		return stepHalt(state, fmt.Errorf("instance_id or disk_id is not set; cannot detach"))
	}
	ui.Say("Detaching boot disk (image create requires state=available)...")
	if err := driver.DetachDisk(ctx, instanceID, diskID); err != nil {
		return stepHalt(state, fmt.Errorf("detach boot disk: %w", err))
	}
	return multistep.ActionContinue
}

func (s *stepDetachDisk) Cleanup(multistep.StateBag) {}
