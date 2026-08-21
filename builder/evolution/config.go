// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config
//go:generate packer-sdc struct-markdown

package evolution

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

var imageNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.\-_]*$`)

// Config is the `cloud-evolution` builder configuration.
type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	Comm                communicator.Config `mapstructure:",squash"`

	// key_id is the service-account access key id.
	// Env: EVOLUTION_KEY_ID.
	KeyID string `mapstructure:"key_id"`
	// key_secret is the service-account secret.
	// Env: EVOLUTION_KEY_SECRET.
	KeySecret string `mapstructure:"key_secret"`
	// token is a static IAM bearer token. Prefer key_id+key_secret:
	// tokens expire in about an hour.
	// Env: EVOLUTION_TOKEN.
	Token string `mapstructure:"token"`
	// project_id is the Evolution project (required).
	// Env: EVOLUTION_PROJECT_ID.
	ProjectID string `mapstructure:"project_id"`
	// compute_url overrides https://compute.api.cloud.ru.
	ComputeURL string `mapstructure:"compute_url"`
	// iam_url overrides https://iam.api.cloud.ru.
	IAMURL string `mapstructure:"iam_url"`

	// zone is the availability zone id (UUID), not the display name.
	Zone string `mapstructure:"zone"`
	// subnet_id is the VPC subnet the builder VM is attached to.
	SubnetID string `mapstructure:"subnet_id"`
	// flavor_id is the Compute flavor UUID.
	FlavorID string `mapstructure:"flavor_id"`
	// source_image_id is the image the builder VM is created from.
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

	c.KeyID = firstNonEmpty(c.KeyID, os.Getenv("EVOLUTION_KEY_ID"))
	c.KeySecret = firstNonEmpty(c.KeySecret, os.Getenv("EVOLUTION_KEY_SECRET"))
	c.Token = firstNonEmpty(c.Token, os.Getenv("EVOLUTION_TOKEN"))
	c.ProjectID = firstNonEmpty(c.ProjectID, os.Getenv("EVOLUTION_PROJECT_ID"))

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
	}

	if c.DiskSizeGb == 0 {
		c.DiskSizeGb = 10
	}
	if c.DiskSizeGb < 1 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disk_size_gb must be >= 1"))
	}
	if c.DiskType == "" {
		c.DiskType = "SSD"
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
	if c.StateTimeout == 0 {
		c.StateTimeout = 15 * time.Minute
	}
	if c.PollInterval == 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.UseFloatingIP == nil {
		v := true
		c.UseFloatingIP = &v
	}
	if c.Comm.SSHUsername == "" {
		c.Comm.SSHUsername = c.LinuxLogin
	}

	if es := c.Comm.Prepare(&c.ctx); len(es) > 0 {
		errs = packersdk.MultiErrorAppend(errs, es...)
	}
	if errs != nil && len(errs.Errors) > 0 {
		return warnings, errs
	}
	return warnings, nil
}

func (c *Config) floatingIP() bool {
	return c.UseFloatingIP != nil && *c.UseFloatingIP
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
