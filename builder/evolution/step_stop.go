// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepStopInstance struct{}

func (s *stepStopInstance) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	driver := state.Get("driver").(Driver)
	id := state.Get("instance_id").(string)
	ui.Say("Stopping instance...")
	if err := driver.StopInstance(ctx, id); err != nil {
		return stepHalt(state, fmt.Errorf("stop instance: %w", err))
	}
	return multistep.ActionContinue
}

func (s *stepStopInstance) Cleanup(multistep.StateBag) {}
