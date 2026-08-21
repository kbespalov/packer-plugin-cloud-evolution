// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"os"
	"testing"
)

// TestAccBuilderPrepareLive is a smoke check that live credentials decode.
// A full packer build is `make testacc` once the binary is installed.
func TestAccBuilderPrepareLive(t *testing.T) {
	if os.Getenv("PACKER_ACC") == "" {
		t.Skip("PACKER_ACC is not set")
	}
	raw := map[string]interface{}{
		"project_id":      os.Getenv("EVOLUTION_PROJECT_ID"),
		"zone":            os.Getenv("EVOLUTION_ZONE"),
		"subnet_id":       os.Getenv("EVOLUTION_SUBNET_ID"),
		"flavor_id":       os.Getenv("EVOLUTION_FLAVOR_ID"),
		"source_image_id": os.Getenv("EVOLUTION_SOURCE_IMAGE_ID"),
		"image_name":      "AccImage",
		"ssh_username":    "ubuntu",
	}
	var c Config
	if _, err := c.Prepare(raw); err != nil {
		t.Fatal(err)
	}
}
