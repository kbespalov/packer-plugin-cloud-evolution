// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepFloatingIP struct{}

func (s *stepFloatingIP) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	cfg := state.Get("config").(*Config)
	driver := state.Get("driver").(Driver)
	inst := state.Get("instance").(Instance)

	if !cfg.floatingIP() {
		state.Put("instance_ip", inst.PrivateIP)
		ui.Say(fmt.Sprintf("SSH via private IP %s (use_floating_ip=false)", inst.PrivateIP))
		return multistep.ActionContinue
	}
	if inst.InterfaceID == "" {
		return stepHalt(state, fmt.Errorf("instance has no interface id; cannot allocate floating IP"))
	}
	ui.Say("Allocating floating IP (cannot be set on VM create)...")
	fip, err := driver.CreateFloatingIP(ctx, cfg.InstanceName+"-fip", inst.InterfaceID, cfg.Zone)
	if err != nil {
		return stepHalt(state, fmt.Errorf("create floating IP: %w", err))
	}
	state.Put("floating_ip_id", fip.ID)
	state.Put("instance_ip", fip.IPAddress)
	ui.Say(fmt.Sprintf("Floating IP %s (%s)", fip.IPAddress, fip.ID))
	return multistep.ActionContinue
}

func (s *stepFloatingIP) Cleanup(multistep.StateBag) {}
