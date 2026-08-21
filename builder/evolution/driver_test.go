// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testDriver(t *testing.T, h http.Handler) *ClientDriver {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return &ClientDriver{Client: client, Interval: time.Millisecond, Timeout: 200 * time.Millisecond}
}

func TestWaitInstanceFailsFastOnErrorState(t *testing.T) {
	t.Parallel()
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "vm-1", "name": "n", "state": "ERROR"})
	}))
	_, err := d.WaitInstance(context.Background(), "vm-1", Instance.Running)
	if err == nil || !strings.Contains(err.Error(), "failed state") {
		t.Fatalf("%v", err)
	}
}

func TestWaitInstanceRetriesInitial404(t *testing.T) {
	t.Parallel()
	n := 0
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`[{"code":"not_found"}]`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "vm-1", "name": "n", "state": "running",
			"interfaces": []map[string]any{{"id": "if-1", "ip_address": "10.0.0.2", "primary": true}},
		})
	}))
	got, err := d.WaitInstance(context.Background(), "vm-1", Instance.Provisionable)
	if err != nil || got.PrivateIP != "10.0.0.2" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestWaitInstanceErrorsAfterDisappear(t *testing.T) {
	t.Parallel()
	n := 0
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "vm-1", "name": "n", "state": "creating"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`[{"code":"not_found"}]`))
	}))
	_, err := d.WaitInstance(context.Background(), "vm-1", Instance.Running)
	if err == nil || !strings.Contains(err.Error(), "disappeared") {
		t.Fatalf("%v", err)
	}
}

func TestStopInstanceAlreadyStopped(t *testing.T) {
	t.Parallel()
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "vm-1", "name": "n", "state": "Shutoff"})
	}))
	if err := d.StopInstance(context.Background(), "vm-1"); err != nil {
		t.Fatal(err)
	}
}

func TestStopInstanceMissingIsOK(t *testing.T) {
	t.Parallel()
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`[{"code":"not_found"}]`))
	}))
	if err := d.StopInstance(context.Background(), "vm-1"); err != nil {
		t.Fatal(err)
	}
}

func TestDetachDiskAlreadyAvailable(t *testing.T) {
	t.Parallel()
	posts := 0
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "disk-1", "name": "boot", "state": "available"})
	}))
	if err := d.DetachDisk(context.Background(), "vm-1", "disk-1"); err != nil {
		t.Fatal(err)
	}
	if posts != 0 {
		t.Fatalf("must not POST detach when already available, posts=%d", posts)
	}
}

func TestDeleteDiskWaitsUntilFree(t *testing.T) {
	t.Parallel()
	gets := 0
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets++
			state := "in_use"
			if gets >= 2 {
				state = "available"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "disk-1", "name": "boot", "state": state})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("%s", r.Method)
		}
	}))
	if err := d.DeleteDisk(context.Background(), "disk-1"); err != nil {
		t.Fatal(err)
	}
}

func TestWaitDiskFailsFast(t *testing.T) {
	t.Parallel()
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "disk-1", "name": "boot", "state": "error"})
	}))
	_, err := d.WaitDisk(context.Background(), "disk-1", Disk.Available)
	if err == nil || !strings.Contains(err.Error(), "failed state") {
		t.Fatalf("%v", err)
	}
}

func TestWaitImageFailsFast(t *testing.T) {
	t.Parallel()
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "img-1", "name": "n", "type": "private",
			"availability_zones": []map[string]any{{"availability_zone_id": "az", "state": "ERROR"}},
		})
	}))
	_, err := d.WaitImage(context.Background(), "img-1")
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("%v", err)
	}
}

func TestDeleteImageNotFound(t *testing.T) {
	t.Parallel()
	d := testDriver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`[{"code":"not_found"}]`))
	}))
	if err := d.DeleteImage(context.Background(), "img-1"); err != nil {
		t.Fatal(err)
	}
}
