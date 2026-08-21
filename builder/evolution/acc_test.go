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
	project := envTrim(EnvProjectID)
	zone := envTrim(EnvZoneID)
	subnet := envTrim(EnvSubnetID)
	flavor := envTrim(EnvFlavorID)
	source := envTrim(EnvSourceImageID)
	raw := map[string]interface{}{
		"project_id":      project,
		"zone":            zone,
		"subnet_id":       subnet,
		"flavor_id":       flavor,
		"source_image_id": source,
		"image_name":      "AccImage",
		"ssh_username":    "ubuntu",
	}
	var c Config
	if _, err := c.Prepare(raw); err != nil {
		t.Fatal(err)
	}
}
