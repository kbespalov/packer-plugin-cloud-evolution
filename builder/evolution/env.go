// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"os"
	"strings"
)

// Environment variables use the CLOUDRU_EVOLUTION_ prefix so they do not
// collide with other "evolution" products.
const (
	EnvKeyID            = "CLOUDRU_EVOLUTION_KEY_ID"
	EnvKeySecret        = "CLOUDRU_EVOLUTION_KEY_SECRET"
	EnvToken            = "CLOUDRU_EVOLUTION_TOKEN"
	EnvProjectID        = "CLOUDRU_EVOLUTION_PROJECT_ID"
	EnvZoneID           = "CLOUDRU_EVOLUTION_ZONE_ID"
	EnvSubnetID         = "CLOUDRU_EVOLUTION_SUBNET_ID"
	EnvFlavorID         = "CLOUDRU_EVOLUTION_FLAVOR_ID"
	EnvSourceImageID    = "CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID"
	EnvSecurityGroupIDs = "CLOUDRU_EVOLUTION_SECURITY_GROUP_IDS"
	EnvImageName        = "CLOUDRU_EVOLUTION_IMAGE_NAME"
	EnvComputeURL       = "CLOUDRU_EVOLUTION_COMPUTE_URL"
	EnvIAMURL           = "CLOUDRU_EVOLUTION_IAM_URL"
)

var allEnv = []string{
	EnvKeyID, EnvKeySecret, EnvToken, EnvProjectID,
	EnvZoneID, EnvSubnetID, EnvFlavorID, EnvSourceImageID,
	EnvSecurityGroupIDs, EnvImageName, EnvComputeURL, EnvIAMURL,
}

func envTrim(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func applyEnv(dst *string, name string) {
	if strings.TrimSpace(*dst) != "" {
		return
	}
	if v := envTrim(name); v != "" {
		*dst = v
	}
}

func envCSV(name string) []string {
	raw := envTrim(name)
	if raw == "" {
		return nil
	}
	var values []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			values = append(values, p)
		}
	}
	return values
}

func allEnvNames() []string {
	return append([]string(nil), allEnv...)
}
