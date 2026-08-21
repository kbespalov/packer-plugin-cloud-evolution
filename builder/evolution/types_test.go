// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import "testing"

func TestImageReady(t *testing.T) {
	t.Parallel()
	if (Image{ID: "x"}).Ready() {
		t.Fatal("no zones")
	}
	ready := Image{ID: "x", ZoneStates: map[string]string{"az": "created"}}
	if !ready.Ready() {
		t.Fatal("created should be ready")
	}
	creating := Image{ID: "x", ZoneStates: map[string]string{"az": "creating"}}
	if creating.Ready() {
		t.Fatal("creating")
	}
	failed := Image{ID: "x", ZoneStates: map[string]string{"az": "error"}}
	if !failed.Failed() {
		t.Fatal("error")
	}
}
