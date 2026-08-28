// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
)

// Builder creates a private Cloud.ru Evolution image from an existing image.
// Lifecycle follows packer-plugin-yandex: SSH key → instance → connect →
// provision → stop → (Evolution-specific) detach boot disk → create image.
type Builder struct {
	config Config
	runner multistep.Runner
	// driver, if set, is used instead of a live Client. Tests inject FakeDriver.
	driver Driver
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec {
	return b.config.FlatMapstructure().HCL2Spec()
}

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	warnings, err := b.config.Prepare(raws...)
	if err != nil {
		return nil, warnings, err
	}
	return []string{"ImageID", "ImageName", "ImageType", "SourceImageID", "SourceImageName"}, warnings, nil
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	driver := b.driver
	if driver == nil {
		client, err := NewClient(ClientConfig{
			BaseURL:   b.config.ComputeURL,
			IAMURL:    b.config.IAMURL,
			ProjectID: b.config.ProjectID,
			Token:     b.config.Token,
			KeyID:     b.config.KeyID,
			KeySecret: b.config.KeySecret,
		})
		if err != nil {
			return nil, err
		}
		driver = &ClientDriver{Client: client, Interval: b.config.PollInterval, Timeout: b.config.StateTimeout}
	}

	state := new(multistep.BasicStateBag)
	state.Put("config", &b.config)
	state.Put("driver", driver)
	state.Put("hook", hook)
	state.Put("ui", ui)
	generated := &packerbuilderdata.GeneratedData{State: state}

	steps := []multistep.Step{
		&communicator.StepSSHKeyGen{
			CommConf:            &b.config.Comm,
			SSHTemporaryKeyPair: b.config.Comm.SSH.SSHTemporaryKeyPair,
		},
		&stepCreateInstance{GeneratedData: generated},
		&stepFloatingIP{},
		&communicator.StepConnect{
			Config:    &b.config.Comm,
			Host:      commHost,
			SSHConfig: b.config.Comm.SSHConfigFunc(),
		},
		&commonsteps.StepProvision{},
		&commonsteps.StepCleanupTempKeys{Comm: &b.config.Comm},
		&stepStopInstance{},
		&stepDetachDisk{},
		&stepCreateImage{GeneratedData: generated},
	}

	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	if raw, ok := state.GetOk("error"); ok {
		return nil, raw.(error)
	}
	if b.config.SkipCreateImage {
		return nil, nil
	}
	raw, ok := state.GetOk("image")
	if !ok {
		return nil, fmt.Errorf("build finished without an image")
	}
	return &Artifact{
		Image:     raw.(Image),
		driver:    driver,
		StateData: map[string]interface{}{"generated_data": state.Get("generated_data")},
	}, nil
}

func commHost(state multistep.StateBag) (string, error) {
	// ssh_host in the template overrides the discovered address (bastion,
	// DNAT, VPN split-horizon setups).
	if raw, ok := state.GetOk("config"); ok {
		if cfg, ok := raw.(*Config); ok && cfg != nil && cfg.Comm.Host() != "" {
			return cfg.Comm.Host(), nil
		}
	}
	if ip, ok := state.GetOk("instance_ip"); ok {
		if s, ok := ip.(string); ok && s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("instance IP is not set")
}
