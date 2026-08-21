// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config
//go:generate packer-sdc struct-markdown

package evolution

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

var imageNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.\-_]*$`)
var uuidLike = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Config is the `cloud-evolution` builder configuration.
type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	Comm                communicator.Config `mapstructure:",squash"`

	// key_id is the service-account access key id.
	// Env: CLOUDRU_EVOLUTION_KEY_ID.
	KeyID string `mapstructure:"key_id"`
	// key_secret is the service-account secret.
	// Env: CLOUDRU_EVOLUTION_KEY_SECRET.
	KeySecret string `mapstructure:"key_secret"`
	// token is a static IAM bearer token. Prefer key_id+key_secret:
	// tokens expire in about an hour.
	// Env: CLOUDRU_EVOLUTION_TOKEN.
	Token string `mapstructure:"token"`
	// project_id is the Evolution project (required).
	// Env: CLOUDRU_EVOLUTION_PROJECT_ID.
	ProjectID string `mapstructure:"project_id"`
	// compute_url overrides https://compute.api.cloud.ru.
	// Env: CLOUDRU_EVOLUTION_COMPUTE_URL.
	ComputeURL string `mapstructure:"compute_url"`
	// iam_url overrides https://iam.api.cloud.ru.
	// Env: CLOUDRU_EVOLUTION_IAM_URL.
	IAMURL string `mapstructure:"iam_url"`

	// zone is the availability zone id (UUID), not the display name.
	// Env: CLOUDRU_EVOLUTION_ZONE_ID.
	Zone string `mapstructure:"zone"`
	// subnet_id is the VPC subnet the builder VM is attached to.
	// Env: CLOUDRU_EVOLUTION_SUBNET_ID.
	SubnetID string `mapstructure:"subnet_id"`
	// flavor_id is the Compute flavor UUID.
	// Env: CLOUDRU_EVOLUTION_FLAVOR_ID.
	FlavorID string `mapstructure:"flavor_id"`
	// source_image_id is the image the builder VM is created from.
	// Env: CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID.
	SourceImageID string `mapstructure:"source_image_id"`
	// disk_size_gb is the boot disk size. Default 10.
	DiskSizeGb int `mapstructure:"disk_size_gb"`
	// disk_type is the catalog name, SSD or HDD. Default SSD.
	DiskType string `mapstructure:"disk_type"`
	// security_group_ids are optional NIC security groups.
	SecurityGroupIDs []string `mapstructure:"security_group_ids"`
	// instance_name defaults to packer-<uuid>.
	InstanceName string `mapstructure:"instance_name"`
	// linux_login is image_metadata.name. Public Ubuntu-24.04 only
	// injects the SSH key when this is "ubuntu".
	LinuxLogin string `mapstructure:"linux_login"`

	// image_name is the private image name (required). Must match
	// ^[a-zA-Z][a-zA-Z0-9.\-_]*$ and be unique in the project.
	// Env: CLOUDRU_EVOLUTION_IMAGE_NAME.
	ImageName string `mapstructure:"image_name"`
	// image_description is stored on the catalog entry.
	ImageDescription string `mapstructure:"image_description"`
	// skip_create_image stops after provision (debug). No artifact.
	SkipCreateImage bool `mapstructure:"skip_create_image"`

	// use_floating_ip allocates a public IP after the NIC exists.
	// new_floating_ip on VM create is extra_forbidden. Default true.
	UseFloatingIP *bool `mapstructure:"use_floating_ip"`
	// state_timeout is how long to wait for VM/disk/image. Default 15m.
	StateTimeout time.Duration `mapstructure:"state_timeout"`
	// poll_interval is the GET interval. Default 5s.
	PollInterval time.Duration `mapstructure:"poll_interval"`

	ctx interpolate.Context
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         BuilderID,
		Interpolate:        true,
		InterpolateContext: &c.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{"run_command"},
		},
	}, raws...)
	if err != nil {
		return nil, err
	}

	var errs *packersdk.MultiError
	var warnings []string

	applyEnv(&c.KeyID, EnvKeyID)
	applyEnv(&c.KeySecret, EnvKeySecret)
	applyEnv(&c.Token, EnvToken)
	applyEnv(&c.ProjectID, EnvProjectID)
	applyEnv(&c.Zone, EnvZoneID)
	applyEnv(&c.SubnetID, EnvSubnetID)
	applyEnv(&c.FlavorID, EnvFlavorID)
	applyEnv(&c.SourceImageID, EnvSourceImageID)
	applyEnv(&c.ImageName, EnvImageName)
	applyEnv(&c.ComputeURL, EnvComputeURL)
	applyEnv(&c.IAMURL, EnvIAMURL)
	if len(c.SecurityGroupIDs) == 0 {
		c.SecurityGroupIDs = envCSV(EnvSecurityGroupIDs)
	}
	c.SecurityGroupIDs = trimNonEmpty(c.SecurityGroupIDs)

	trimCfgStrings(c)

	if c.ProjectID == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("project_id must be set"))
	}
	if c.KeyID == "" || c.KeySecret == "" {
		if c.Token == "" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("set key_id and key_secret, or token"))
		}
	}
	if c.Zone == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("zone must be set (availability zone UUID)"))
	} else if !uuidLike.MatchString(c.Zone) {
		warnings = append(warnings, fmt.Sprintf("zone %q does not look like a UUID; Evolution wants the availability zone id, not the display name (ru.AZ-2)", c.Zone))
	}
	if c.SubnetID == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("subnet_id must be set"))
	}
	if c.FlavorID == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("flavor_id must be set"))
	}
	if c.SourceImageID == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("source_image_id must be set"))
	}
	if c.ImageName == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("image_name must be set"))
	} else if !imageNamePattern.MatchString(c.ImageName) {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("image_name %q does not match %s", c.ImageName, imageNamePattern))
	} else if len(c.ImageName) > 255 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("image_name is longer than 255 characters"))
	}

	if c.DiskSizeGb == 0 {
		c.DiskSizeGb = 10
	}
	if c.DiskSizeGb < 1 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disk_size_gb must be >= 1"))
	}
	if c.DiskSizeGb > 4096 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disk_size_gb must be <= 4096"))
	}
	c.DiskType = strings.ToUpper(c.DiskType)
	if c.DiskType == "" {
		c.DiskType = "SSD"
	}
	if c.DiskType != "SSD" && c.DiskType != "HDD" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disk_type must be SSD or HDD"))
	}
	if c.LinuxLogin == "" {
		c.LinuxLogin = "ubuntu"
	}
	if c.LinuxLogin != "ubuntu" {
		warnings = append(warnings, "linux_login is not \"ubuntu\": public Ubuntu images often skip injecting public_key")
	}
	if c.InstanceName == "" {
		c.InstanceName = "packer-" + uuid.TimeOrderedUUID()
	}
	if strings.ContainsAny(c.InstanceName, " \t\n/") || len(c.InstanceName) > 255 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("instance_name %q is empty, contains whitespace/slash, or is longer than 255 characters", c.InstanceName))
	}
	if c.StateTimeout < 0 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("state_timeout must be >= 0"))
	}
	if c.PollInterval < 0 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("poll_interval must be >= 0"))
	}
	if c.StateTimeout == 0 {
		// Image-from-disk was ~8–12m live; provision + wait needs headroom.
		c.StateTimeout = 30 * time.Minute
	}
	if c.PollInterval == 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.PollInterval > c.StateTimeout {
		warnings = append(warnings, "poll_interval capped to state_timeout so a wait cannot sleep longer than its deadline")
		c.PollInterval = c.StateTimeout
	}
	if c.ComputeURL != "" {
		if _, err := normalizeBaseURL(c.ComputeURL, defaultComputeURL); err != nil {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("compute_url: %w", err))
		}
	}
	if c.IAMURL != "" {
		if _, err := normalizeBaseURL(c.IAMURL, defaultIAMURL); err != nil {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("iam_url: %w", err))
		}
	}
	if c.UseFloatingIP == nil {
		v := true
		c.UseFloatingIP = &v
	}
	if c.Comm.SSHUsername == "" {
		c.Comm.SSHUsername = c.LinuxLogin
	} else if c.Comm.SSHUsername != c.LinuxLogin {
		warnings = append(warnings, fmt.Sprintf("ssh_username %q differs from linux_login %q; public Ubuntu injects public_key for linux_login only", c.Comm.SSHUsername, c.LinuxLogin))
	}
	// Public Ubuntu metadata was live-tested with a raw ssh-ed25519 key.
	// Packer's communicator default is RSA.
	if c.Comm.SSHTemporaryKeyPairType == "" && c.Comm.SSHPrivateKeyFile == "" {
		c.Comm.SSHTemporaryKeyPairType = "ed25519"
	}
	if c.Comm.PauseBeforeConnect == 0 {
		c.Comm.PauseBeforeConnect = 20 * time.Second
	}

	if es := c.Comm.Prepare(&c.ctx); len(es) > 0 {
		errs = packersdk.MultiErrorAppend(errs, es...)
	}
	// StepConnectSSH waits forever when SSHTimeout is 0 (the SDK only
	// defaults it if handshake attempts are also unset).
	if c.Comm.SSHTimeout == 0 {
		c.Comm.SSHTimeout = 10 * time.Minute
	}
	if errs != nil && len(errs.Errors) > 0 {
		return warnings, errs
	}
	return warnings, nil
}

func (c *Config) floatingIP() bool {
	return c.UseFloatingIP != nil && *c.UseFloatingIP
}

func trimCfgStrings(c *Config) {
	c.KeyID = strings.TrimSpace(c.KeyID)
	c.KeySecret = strings.TrimSpace(c.KeySecret)
	c.Token = strings.TrimSpace(c.Token)
	c.ProjectID = strings.TrimSpace(c.ProjectID)
	c.ComputeURL = strings.TrimSpace(c.ComputeURL)
	c.IAMURL = strings.TrimSpace(c.IAMURL)
	c.Zone = strings.TrimSpace(c.Zone)
	c.SubnetID = strings.TrimSpace(c.SubnetID)
	c.FlavorID = strings.TrimSpace(c.FlavorID)
	c.SourceImageID = strings.TrimSpace(c.SourceImageID)
	c.DiskType = strings.TrimSpace(c.DiskType)
	c.InstanceName = strings.TrimSpace(c.InstanceName)
	c.LinuxLogin = strings.TrimSpace(c.LinuxLogin)
	c.ImageName = strings.TrimSpace(c.ImageName)
	c.ImageDescription = strings.TrimSpace(c.ImageDescription)
}

func trimNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
