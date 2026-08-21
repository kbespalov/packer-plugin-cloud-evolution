// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"net/http"
	"strings"
	"testing"
)

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
	failed := Image{ID: "x", ZoneStates: map[string]string{"az": "ERROR"}}
	if !failed.Failed() {
		t.Fatal("error")
	}
}

func TestInstanceStates(t *testing.T) {
	t.Parallel()
	if !(Instance{State: "RUNNING"}).Running() {
		t.Fatal("running is case-insensitive")
	}
	if !(Instance{State: "Shutoff"}).Stopped() {
		t.Fatal("shutoff")
	}
	if !(Instance{State: "powering_off"}).Stopping() {
		t.Fatal("stopping")
	}
	if !(Instance{State: "ERROR"}).Failed() {
		t.Fatal("failed")
	}
	if (Instance{State: "running", InterfaceID: "if", PrivateIP: "10.0.0.1"}).Provisionable() == false {
		t.Fatal("provisionable")
	}
	if (Instance{State: "running", InterfaceID: "if"}).Provisionable() {
		t.Fatal("missing private IP")
	}
}

func TestDiskStates(t *testing.T) {
	t.Parallel()
	if !(Disk{State: "AVAILABLE"}).Available() {
		t.Fatal("available")
	}
	if !(Disk{State: "in-use"}).InUse() {
		t.Fatal("in-use")
	}
	if !(Disk{State: "Failed"}).Failed() {
		t.Fatal("failed")
	}
}

func TestLookupExists(t *testing.T) {
	t.Parallel()
	ok, err := lookupExists("id", nil)
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	ok, err = lookupExists("", &APIError{Status: http.StatusNotFound})
	if ok || err != nil {
		t.Fatal("404 is not exists")
	}
	_, err = lookupExists("", &RequestError{Err: errTimeout()})
	if err == nil {
		t.Fatal("transport error must surface")
	}
}

func TestAnnotateQuota(t *testing.T) {
	t.Parallel()
	err := annotateQuota(&APIError{Status: 422, Code: "organization_quota_exceeded", Message: "compute.image.custome"})
	if err == nil || !strings.Contains(err.Error(), "unused private image") {
		t.Fatalf("%v", err)
	}
	if annotateQuota(nil) != nil {
		t.Fatal("nil")
	}
}
