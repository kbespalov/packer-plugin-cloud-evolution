// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/plugin"

	"github.com/kbespalov/packer-plugin-cloud-evolution/builder/evolution"
	"github.com/kbespalov/packer-plugin-cloud-evolution/version"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterBuilder(plugin.DEFAULT_NAME, new(evolution.Builder))
	pps.SetVersion(version.PluginVersion)
	if err := pps.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
