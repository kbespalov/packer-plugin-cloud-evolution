// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"strings"
	"testing"
	"time"
)

func clearEvolutionEnv(t *testing.T) {
	t.Helper()
	for _, k := range allEnvNames() {
		t.Setenv(k, "")
	}
}

func validRaw() map[string]interface{} {
	return map[string]interface{}{
		"key_id":          "key",
		"key_secret":      "secret",
		"project_id":      "proj",
		"zone":            "00000000-0000-0000-0000-000000000001",
		"subnet_id":       "subnet",
		"flavor_id":       "flavor",
		"source_image_id": "img",
		"image_name":      "MyImage-1",
		"ssh_username":    "ubuntu",
	}
}

func TestConfigPrepareRequired(t *testing.T) {
	clearEvolutionEnv(t)
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
	clearEvolutionEnv(t)
	var c Config
	warn, err := c.Prepare(validRaw())
	if err != nil {
		t.Fatal(err)
	}
	if c.DiskSizeGb != 10 || c.DiskType != "SSD" || c.LinuxLogin != "ubuntu" {
		t.Fatalf("defaults: %+v", c)
	}
	if c.StateTimeout != 30*time.Minute || c.PollInterval != 5*time.Second {
		t.Fatalf("timeouts: %s %s", c.StateTimeout, c.PollInterval)
	}
	if !c.floatingIP() {
		t.Fatal("use_floating_ip should default true")
	}
	if c.Comm.SSHTemporaryKeyPairType != "ed25519" {
		t.Fatalf("temporary key type %q", c.Comm.SSHTemporaryKeyPairType)
	}
	if c.Comm.PauseBeforeConnect != 20*time.Second {
		t.Fatalf("pause %s", c.Comm.PauseBeforeConnect)
	}
	if c.Comm.SSHTimeout == 0 {
		t.Fatal("ssh_timeout must be set; Packer waits forever when it is 0")
	}
	if !strings.HasPrefix(c.InstanceName, "packer-") {
		t.Fatalf("instance name %q", c.InstanceName)
	}
	if len(warn) != 0 {
		t.Fatalf("warnings: %v", warn)
	}
}

func TestConfigPrepareImageName(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	raw["image_name"] = "1bad"
	var c Config
	if _, err := c.Prepare(raw); err == nil {
		t.Fatal("expected image_name pattern error")
	}
}

func TestConfigPrepareAuthEnv(t *testing.T) {
	clearEvolutionEnv(t)
	t.Setenv(EnvKeyID, "from-env")
	t.Setenv(EnvKeySecret, "from-env-secret")
	t.Setenv(EnvProjectID, "from-env-proj")
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
}

func TestConfigPrepareIgnoresBareEvolutionPrefix(t *testing.T) {
	clearEvolutionEnv(t)
	t.Setenv("EVOLUTION_KEY_ID", "old-key")
	t.Setenv("EVOLUTION_KEY_SECRET", "old-secret")
	t.Setenv("EVOLUTION_PROJECT_ID", "old-proj")
	raw := validRaw()
	delete(raw, "key_id")
	delete(raw, "key_secret")
	delete(raw, "project_id")
	var c Config
	if _, err := c.Prepare(raw); err == nil {
		t.Fatal("bare EVOLUTION_* must not satisfy auth")
	}
}

func TestConfigPrepareLinuxLoginWarning(t *testing.T) {
	clearEvolutionEnv(t)
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

func TestConfigPrepareTrimsAndNormalizes(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	raw["project_id"] = "  proj  "
	raw["disk_type"] = "hdd"
	raw["image_name"] = "  MyImage-1  "
	var c Config
	if _, err := c.Prepare(raw); err != nil {
		t.Fatal(err)
	}
	if c.ProjectID != "proj" || c.DiskType != "HDD" || c.ImageName != "MyImage-1" {
		t.Fatalf("%#v", c)
	}
}

func TestConfigPrepareDiskType(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	raw["disk_type"] = "nvme"
	var c Config
	if _, err := c.Prepare(raw); err == nil {
		t.Fatal("expected disk_type error")
	}
}

func TestConfigPrepareZoneWarning(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	raw["zone"] = "ru.AZ-2"
	var c Config
	warn, err := c.Prepare(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(warn) == 0 {
		t.Fatal("expected zone UUID warning")
	}
}

func TestConfigPrepareDiskSize(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	raw["disk_size_gb"] = 0
	var c Config
	if _, err := c.Prepare(raw); err != nil {
		t.Fatal(err)
	}
	if c.DiskSizeGb != 10 {
		t.Fatalf("default %d", c.DiskSizeGb)
	}
	raw["disk_size_gb"] = 5000
	c = Config{}
	if _, err := c.Prepare(raw); err == nil {
		t.Fatal("expected oversized disk error")
	}
}

func TestConfigPrepareSSHUsernameMismatch(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	raw["linux_login"] = "ubuntu"
	raw["ssh_username"] = "admin"
	var c Config
	warn, err := c.Prepare(raw)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range warn {
		if strings.Contains(w, "ssh_username") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings: %v", warn)
	}
}

func TestConfigPrepareCapsPollInterval(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	raw["poll_interval"] = "1h"
	raw["state_timeout"] = "30m"
	var c Config
	warn, err := c.Prepare(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.PollInterval != 30*time.Minute {
		t.Fatalf("poll_interval=%s", c.PollInterval)
	}
	if len(warn) == 0 {
		t.Fatal("expected cap warning")
	}
}

func TestConfigPrepareTokenWithoutKeys(t *testing.T) {
	clearEvolutionEnv(t)
	raw := validRaw()
	delete(raw, "key_id")
	delete(raw, "key_secret")
	raw["token"] = "static"
	var c Config
	if _, err := c.Prepare(raw); err != nil {
		t.Fatal(err)
	}
}
