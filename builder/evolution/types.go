// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import "strings"

func normState(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Instance is a Compute VM as the builder needs it.
type Instance struct {
	ID           string
	Name         string
	State        string
	PrivateIP    string
	InterfaceID  string
	FloatingIP   string
	FloatingIPID string
	BootDiskID   string
	BootDiskName string
}

func (i Instance) Running() bool { return normState(i.State) == "running" }

func (i Instance) Stopped() bool {
	switch normState(i.State) {
	case "stopped", "shutoff", "terminated":
		return true
	default:
		return false
	}
}

func (i Instance) Stopping() bool {
	switch normState(i.State) {
	case "stopping", "powering_off", "shutting_down":
		return true
	default:
		return false
	}
}

func (i Instance) Failed() bool {
	switch normState(i.State) {
	case "error", "failed":
		return true
	default:
		return false
	}
}

// Provisionable is running with a NIC and a private address. The floating IP
// is allocated after this; SSH may still use the public address.
func (i Instance) Provisionable() bool {
	return i.Running() && i.InterfaceID != "" && i.PrivateIP != ""
}

// Disk is a Compute volume.
type Disk struct {
	ID       string
	Name     string
	State    string
	VMID     string
	Bootable bool
}

func (d Disk) Available() bool { return normState(d.State) == "available" }

func (d Disk) InUse() bool {
	switch normState(d.State) {
	case "in_use", "in-use":
		return true
	default:
		return false
	}
}

func (d Disk) Failed() bool {
	switch normState(d.State) {
	case "error", "failed":
		return true
	default:
		return false
	}
}

// Image is a catalog entry (public or private).
type Image struct {
	ID               string
	Name             string
	Type             string
	Public           bool
	MinDiskGiB       int
	UserDataTemplate string
	ZoneStates       map[string]string
}

func (img Image) Ready() bool {
	if img.ID == "" || len(img.ZoneStates) == 0 {
		return false
	}
	for _, state := range img.ZoneStates {
		if !imageZoneReady(state) {
			return false
		}
	}
	return true
}

func (img Image) Failed() bool {
	for _, state := range img.ZoneStates {
		if imageZoneFailed(state) {
			return true
		}
	}
	return false
}

func imageZoneReady(state string) bool {
	switch normState(state) {
	case "created", "available", "ready", "active", "uploaded", "loaded", "ok":
		return true
	default:
		return false
	}
}

func imageZoneFailed(state string) bool {
	switch normState(state) {
	case "error", "failed", "deleted", "unknown":
		return true
	default:
		return false
	}
}

// FloatingIP is an allocated public address bound to a NIC.
type FloatingIP struct {
	ID        string
	Name      string
	IPAddress string
}

// CreateInstanceRequest is the driver-level VM create.
type CreateInstanceRequest struct {
	Name             string
	ImageID          string
	FlavorID         string
	SubnetID         string
	Zone             string
	DiskName         string
	DiskSizeGiB      int
	DiskType         string
	SecurityGroupIDs []string
	Hostname         string
	LinuxLogin       string
	PublicKey        string
}

// CreateImageRequest is POST /api/v1/images.
type CreateImageRequest struct {
	Name             string
	DisplayName      string
	Description      string
	DiskID           string
	Zone             string
	MinCPU           int
	MinRAM           int
	MinDiskGiB       int
	UserDataTemplate string
}
