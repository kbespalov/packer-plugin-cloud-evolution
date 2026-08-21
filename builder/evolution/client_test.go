// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIAMTokenAndRefreshOn401(t *testing.T) {
	t.Parallel()
	tokens := 0
	vms := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		tokens++
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["keyId"] != "k" || body["secret"] != "s" {
			t.Fatalf("iam body %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-" + itoa(tokens), "expires_in": 3600})
	})
	mux.HandleFunc("/api/v1/vms/vm-1", func(w http.ResponseWriter, r *http.Request) {
		vms++
		if r.Header.Get("Authorization") == "Bearer tok-1" && vms == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`[{"code":"unauthorized","message":"expired"}]`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok-2" {
			t.Fatalf("auth %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "vm-1", "name": "n", "state": "running", "interfaces": []any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := NewClient(ClientConfig{
		BaseURL: srv.URL, IAMURL: srv.URL, ProjectID: "proj",
		KeyID: "k", KeySecret: "s", HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetInstance(context.Background(), "vm-1")
	if err != nil || got.ID != "vm-1" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if tokens != 2 {
		t.Fatalf("tokens=%d", tokens)
	}
}

func TestCreateInstancePostsArray(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1.1/vms" || r.Method != http.MethodPost {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body []map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("must be JSON array: %s", raw)
		}
		if len(body) != 1 {
			t.Fatalf("want one VM, got %d", len(body))
		}
		meta := body[0]["image_metadata"].(map[string]any)
		if meta["name"] != "ubuntu" || meta["public_key"] != "ssh-ed25519 AAAA" {
			t.Fatalf("metadata %#v", meta)
		}
		if _, ok := body[0]["cloud_init"]; ok {
			t.Fatal("must not send cloud_init on a typical public source image")
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": "vm-1", "name": body[0]["name"], "state": "creating",
			"interfaces": []map[string]any{{"id": "if-1", "ip_address": "10.0.0.5", "primary": true}},
			"disks":      []map[string]any{{"id": "disk-1", "name": "packer-1-boot", "bootable": true}},
		}})
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CreateInstance(context.Background(), CreateInstanceRequest{
		Name: "packer-1", ImageID: "img", FlavorID: "fl", SubnetID: "sn", Zone: "az",
		DiskName: "packer-1-boot", DiskSizeGiB: 10, LinuxLogin: "ubuntu", PublicKey: "ssh-ed25519 AAAA",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "vm-1" || got.PrivateIP != "10.0.0.5" || got.InterfaceID != "if-1" || got.BootDiskID != "disk-1" {
		t.Fatalf("%+v", got)
	}
}

func TestCreateImageOmitsEnabled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		zones := body["availability_zones"].([]any)
		z0 := zones[0].(map[string]any)
		if _, ok := z0["enabled"]; ok {
			t.Fatal("enabled is extra_forbidden on Evolution")
		}
		if body["disk_id"] != "disk-1" || body["project_id"] != "proj" {
			t.Fatalf("%#v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "img-1", "name": body["name"], "type": "private", "public": false,
			"availability_zones": []map[string]any{{"availability_zone_id": "az", "state": "creating"}},
		})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CreateImage(context.Background(), CreateImageRequest{Name: "Golden", DiskID: "disk-1", Zone: "az", MinDiskGiB: 10})
	if err != nil || got.Type != "private" || got.ZoneStates["az"] != "creating" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestParseAPIErrorArray(t *testing.T) {
	t.Parallel()
	err := parseAPIError(422, []byte(`[{"message":"Extra inputs are not permitted","code":"extra_forbidden","field":"body.availability_zones.0.enabled"}]`))
	api, ok := AsAPIError(err)
	if !ok || api.Code != "extra_forbidden" || api.Field == "" {
		t.Fatalf("%v", err)
	}
}

func TestParseAPIErrorObject(t *testing.T) {
	t.Parallel()
	err := parseAPIError(409, []byte(`{"code":"quota_exceeded","message":"floating ip quota"}`))
	api, ok := AsAPIError(err)
	if !ok || api.Code != "quota_exceeded" || !strings.Contains(api.Message, "quota") {
		t.Fatalf("%v", err)
	}
}

func TestCreateInstanceOmitsNewFloatingIP(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "new_floating_ip") || strings.Contains(string(raw), "cloud_init") {
			t.Fatalf("forbidden field in create body: %s", raw)
		}
		if strings.Contains(string(raw), "page_size") {
			t.Fatal("page_size is 422 on Evolution lists")
		}
		var body []map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		ifaces := body[0]["interfaces"].([]any)
		iface := ifaces[0].(map[string]any)
		if _, ok := iface["new_floating_ip"]; ok {
			t.Fatal("new_floating_ip is extra_forbidden")
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "vm-1", "name": "n", "state": "creating"}})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateInstance(context.Background(), CreateInstanceRequest{
		Name: "n", ImageID: "img", FlavorID: "fl", SubnetID: "sn", Zone: "az",
		DiskName: "n-boot", DiskSizeGiB: 10,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateFloatingIP(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/floating-ips" || r.Method != http.MethodPost {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body["interface_id"] != "if-1" || body["project_id"] != "proj" {
			t.Fatalf("%#v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fip-1", "ip_address": "203.0.113.9"})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CreateFloatingIP(context.Background(), "packer-fip", "if-1", "az")
	if err != nil || got.ID != "fip-1" || got.IPAddress != "203.0.113.9" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestCreateFloatingIPWaitsForAddress(t *testing.T) {
	t.Parallel()
	gets := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/floating-ips", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fip-2", "name": "x"})
	})
	mux.HandleFunc("/api/v1/floating-ips/fip-2", func(w http.ResponseWriter, r *http.Request) {
		gets++
		ip := ""
		if gets >= 2 {
			ip = "203.0.113.20"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fip-2", "ip_address": ip})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CreateFloatingIP(context.Background(), "x", "if-1", "az")
	if err != nil || got.IPAddress != "203.0.113.20" {
		t.Fatalf("%+v %v", got, err)
	}
	if gets < 2 {
		t.Fatalf("gets=%d", gets)
	}
}

func TestDeleteInstanceRetriesAfterFIP(t *testing.T) {
	t.Parallel()
	deletes := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vms/vm-1", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			deletes++
			if deletes == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`[{"code":"floating_ip_can_not_be_detached_from_vm_in_current_state","message":"Floating IP can not be detached from Vm in current state"}]`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			if deletes < 2 {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "vm-1", "name": "n", "state": "stopped"})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`[{"code":"not_found"}]`))
		default:
			t.Fatalf("method %s", r.Method)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	d := &ClientDriver{Client: client, Interval: time.Millisecond, Timeout: time.Second}
	if err := d.DeleteInstance(context.Background(), "vm-1"); err != nil {
		t.Fatal(err)
	}
	if deletes < 2 {
		t.Fatalf("deletes=%d", deletes)
	}
}

func TestDoRetriesGET503(t *testing.T) {
	t.Parallel()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || !strings.HasPrefix(r.Header.Get("User-Agent"), "packer-plugin-cloud-evolution/") {
			t.Fatalf("user-agent %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("X-Request-ID") == "" {
			t.Fatal("missing X-Request-ID")
		}
		n++
		if n < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"busy"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "vm-1", "name": "n", "state": "running"})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetInstance(context.Background(), "vm-1")
	if err != nil || got.ID != "vm-1" {
		t.Fatalf("%+v %v", got, err)
	}
	if n != 3 {
		t.Fatalf("n=%d", n)
	}
}

func TestDoDoesNotRetryPOST5xx(t *testing.T) {
	t.Parallel()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"boom"}`))
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateInstance(context.Background(), CreateInstanceRequest{
		Name: "n", ImageID: "img", FlavorID: "fl", SubnetID: "sn", Zone: "az",
		DiskName: "n-boot", DiskSizeGiB: 10,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if n != 1 {
		t.Fatalf("POST 500 must not retry, n=%d", n)
	}
}

func TestIAMTokenCamelCase(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "camel-tok", "expiresIn": 3600})
			return
		}
		if r.Header.Get("Authorization") != "Bearer camel-tok" {
			t.Fatalf("auth %s", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "vm-1", "name": "n", "state": "running"})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{
		BaseURL: srv.URL, IAMURL: srv.URL, ProjectID: "proj",
		KeyID: "k", KeySecret: "s", HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetInstance(context.Background(), "vm-1")
	if err != nil || got.ID != "vm-1" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestListAllPaginates(t *testing.T) {
	t.Parallel()
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_size") != "" {
			t.Fatal("page_size")
		}
		pages++
		if r.URL.Query().Get("offset") == "0" {
			items := make([]map[string]any, 50)
			for i := range items {
				items[i] = map[string]any{"id": "p0-" + itoa(i), "name": "other"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": 51})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "img-keep", "name": "keep", "type": "private"}},
			"total": 51,
		})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.FindImage(context.Background(), "keep")
	if err != nil || got.ID != "img-keep" {
		t.Fatalf("%+v %v", got, err)
	}
	if pages < 2 {
		t.Fatalf("pages=%d", pages)
	}
}

func TestJoinURLRejectsAbsolute(t *testing.T) {
	t.Parallel()
	if _, err := joinURL("https://compute.api.cloud.ru", "https://evil.example/steal"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestNewClientRejectsBadURL(t *testing.T) {
	t.Parallel()
	if _, err := NewClient(ClientConfig{BaseURL: "ftp://x", ProjectID: "p", Token: "t"}); err == nil {
		t.Fatal("expected url error")
	}
}

func TestFindImageByName(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_size") != "" {
			t.Fatal("page_size is 422")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "img-9", "name": "ytsaurus-ubuntu-24-04", "type": "private"}},
			"total": 1,
		})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.FindImage(context.Background(), "ytsaurus-ubuntu-24-04")
	if err != nil || got.ID != "img-9" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestFindDiskByName(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page_size") != "" {
			t.Fatal("page_size is 422")
		}
		if q.Get("name") != "packer-1-boot" || q.Get("limit") == "" {
			t.Fatalf("query %#v", q)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "disk-9", "name": "packer-1-boot", "state": "in_use", "bootable": true}},
			"total": 1,
		})
	}))
	t.Cleanup(srv.Close)
	client, err := NewClient(ClientConfig{BaseURL: srv.URL, ProjectID: "proj", Token: "t", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.FindDisk(context.Background(), "packer-1-boot")
	if err != nil || got.ID != "disk-9" {
		t.Fatalf("%+v %v", got, err)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
