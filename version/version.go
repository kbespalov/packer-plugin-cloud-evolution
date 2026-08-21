// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package version

import "github.com/hashicorp/packer-plugin-sdk/version"

var (
	// Version is the main version number that is being run at the moment.
	Version = "0.1.0"

	// VersionPrerelease is empty for a final release. "dev" means local builds.
	VersionPrerelease = "dev"

	// PluginVersion is what Packer reports for this plugin process.
	PluginVersion = version.InitializePluginVersion(Version, VersionPrerelease)
)
