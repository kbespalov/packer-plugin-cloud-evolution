// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"bytes"
	"fmt"
	"io"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func stepHalt(state multistep.StateBag, err error) multistep.StepAction {
	state.Put("error", err)
	uiFromState(state).Error(fmt.Sprintf("%s", err))
	return multistep.ActionHalt
}

func uiFromState(state multistep.StateBag) packersdk.Ui {
	if raw, ok := state.GetOk("ui"); ok {
		if ui, ok := raw.(packersdk.Ui); ok && ui != nil {
			return ui
		}
	}
	return &packersdk.BasicUi{
		Reader:      bytes.NewReader(nil),
		Writer:      io.Discard,
		ErrorWriter: io.Discard,
	}
}
