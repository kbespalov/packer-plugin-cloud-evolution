// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"os"
	"strings"
	"testing"
	"time"
)

func validRaw() map[string]interface{} {
	return map[string]interface{}{
		"key_id":          "key",
		"key_secret":      "secret",
		"project_id":      "proj",
		"zone":            "az-1",
		"subnet_id":       "subnet",
		"flavor_id":       "flavor",
		"source_image_id": "img",
		"image_name":      "MyImage-1",
		"ssh_username":    "ubuntu",
	}
}

func TestConfigPrepareRequired(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{"project_id", "zone", "subnet_id", "flavor_id", "source_image_id", "image_name"} {
		raw := validRaw()
		delete(raw, missing)
		var c Config
		_, err := c.Prepare(raw)
		if err == nil {
			t.Fatalf("missing %s: expected error", missing)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Fatalf("missing %s: err=%v", missing, err)
		}
	}
}

func TestConfigPrepareDefaults(t *testing.T) {
	t.Parallel()
	var c Config
	warn, err := c.Prepare(validRaw())
	if err != nil {
		t.Fatal(err)
	}
	if c.DiskSizeGb != 10 || c.DiskType != "SSD" || c.LinuxLogin != "ubuntu" {
		t.Fatalf("defaults: %+v", c)
	}
	if c.StateTimeout != 15*time.Minute || c.PollInterval != 5*time.Second {
		t.Fatalf("timeouts: %s %s", c.StateTimeout, c.PollInterval)
	}
	if !c.floatingIP() {
		t.Fatal("use_floating_ip should default true")
	}
	if !strings.HasPrefix(c.InstanceName, "packer-") {
		t.Fatalf("instance name %q", c.InstanceName)
	}
	if len(warn) != 0 {
		t.Fatalf("warnings: %v", warn)
	}
}

func TestConfigPrepareImageName(t *testing.T) {
	t.Parallel()
	raw := validRaw()
	raw["image_name"] = "1bad"
	var c Config
	if _, err := c.Prepare(raw); err == nil {
		t.Fatal("expected image_name pattern error")
	}
}

func TestConfigPrepareAuthEnv(t *testing.T) {
	t.Setenv("EVOLUTION_KEY_ID", "from-env")
	t.Setenv("EVOLUTION_KEY_SECRET", "from-env-secret")
	t.Setenv("EVOLUTION_PROJECT_ID", "from-env-proj")
	raw := validRaw()
	delete(raw, "key_id")
	delete(raw, "key_secret")
	delete(raw, "project_id")
	var c Config
	if _, err := c.Prepare(raw); err != nil {
		t.Fatal(err)
	}
	if c.KeyID != "from-env" || c.ProjectID != "from-env-proj" {
		t.Fatalf("env not applied: %#v", c)
	}
	_ = os.Unsetenv("EVOLUTION_KEY_ID")
}

func TestConfigPrepareLinuxLoginWarning(t *testing.T) {
	t.Parallel()
	raw := validRaw()
	raw["linux_login"] = "user1"
	raw["ssh_username"] = "user1"
	var c Config
	warn, err := c.Prepare(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(warn) == 0 {
		t.Fatal("expected warning about linux_login")
	}
}

func TestConfigPrepareTokenWithoutKeys(t *testing.T) {
	t.Parallel()
	raw := validRaw()
	delete(raw, "key_id")
	delete(raw, "key_secret")
	raw["token"] = "static"
	var c Config
	if _, err := c.Prepare(raw); err != nil {
		t.Fatal(err)
	}
}
