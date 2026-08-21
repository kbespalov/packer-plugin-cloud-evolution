// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"testing"

	"bytes"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestBuilderPrepareGeneratedData(t *testing.T) {
	clearEvolutionEnv(t)
	var b Builder
	generated, _, err := b.Prepare(validRaw())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ImageID", "ImageName", "ImageType", "SourceImageID", "SourceImageName"}
	if len(generated) != len(want) {
		t.Fatalf("%v", generated)
	}
	for i := range want {
		if generated[i] != want[i] {
			t.Fatalf("%v", generated)
		}
	}
}

func TestBuilderRunSkipCreateImage(t *testing.T) {
	clearEvolutionEnv(t)
	driver := NewFakeDriver()
	driver.images["img"] = Image{ID: "img", Name: "Ubuntu", Type: "public"}
	raw := validRaw()
	raw["skip_create_image"] = true
	raw["use_floating_ip"] = false
	var b Builder
	if _, _, err := b.Prepare(raw); err != nil {
		t.Fatal(err)
	}
	b.config.Comm.SSHPublicKey = []byte("ssh-ed25519 AAAA")
	b.driver = driver
	// SkipConnect: we cannot run communicator.StepConnect without a host
	// listening. Exercise the cloud steps only.
	state := new(multistep.BasicStateBag)
	state.Put("config", &b.config)
	state.Put("driver", driver)
	state.Put("ui", &packersdk.BasicUi{Reader: bytes.NewReader(nil), Writer: &bytes.Buffer{}, ErrorWriter: &bytes.Buffer{}})
	step := &stepCreateInstance{}
	if step.Run(context.Background(), state) != multistep.ActionContinue {
		t.Fatalf("%v", state.Get("error"))
	}
}

func TestNewClientRequiresAuth(t *testing.T) {
	t.Parallel()
	if _, err := NewClient(ClientConfig{ProjectID: "p"}); err == nil {
		t.Fatal("expected auth error")
	}
}
