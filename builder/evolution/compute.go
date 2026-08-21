// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const fipAddressTimeout = 2 * time.Minute

type vmCreateDisk struct {
	Name         string `json:"name"`
	Size         int    `json:"size"`
	DiskTypeName string `json:"disk_type_name,omitempty"`
}

type vmCreateIface struct {
	Type           string   `json:"type"`
	SubnetID       string   `json:"subnet_id,omitempty"`
	SecurityGroups []string `json:"security_groups,omitempty"`
}

type vmCreateRequest struct {
	ProjectID          string            `json:"project_id"`
	Name               string            `json:"name"`
	FlavorID           string            `json:"flavor_id,omitempty"`
	ImageID            string            `json:"image_id,omitempty"`
	AvailabilityZoneID string            `json:"availability_zone_id,omitempty"`
	ImageMetadata      map[string]string `json:"image_metadata,omitempty"`
	Disks              []vmCreateDisk    `json:"disks"`
	Interfaces         []vmCreateIface   `json:"interfaces,omitempty"`
}

type vmFloatingIP struct {
	ID        string `json:"id"`
	IPAddress string `json:"ip_address"`
}

type vmIfaceView struct {
	ID         string        `json:"id"`
	IPAddress  string        `json:"ip_address"`
	Primary    bool          `json:"primary"`
	FloatingIP *vmFloatingIP `json:"floating_ip"`
}

type vmDiskView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Bootable bool   `json:"bootable"`
	Primary  bool   `json:"primary"`
}

type vmView struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	State      string        `json:"state"`
	Interfaces []vmIfaceView `json:"interfaces"`
	Disks      []vmDiskView  `json:"disks"`
}

type listPage[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type diskVMRef struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
}

type diskView struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	State    string      `json:"state"`
	Bootable bool        `json:"bootable"`
	VMs      []diskVMRef `json:"vms"`
}

type imageAZCreate struct {
	AvailabilityZoneID string `json:"availability_zone_id,omitempty"`
}

type imageCreateBody struct {
	Name              string          `json:"name"`
	DisplayName       string          `json:"display_name,omitempty"`
	Description       string          `json:"description,omitempty"`
	ProjectID         string          `json:"project_id"`
	DiskID            string          `json:"disk_id,omitempty"`
	MinCPU            int             `json:"min_cpu,omitempty"`
	MinRAM            int             `json:"min_ram,omitempty"`
	MinDisk           int             `json:"min_disk,omitempty"`
	UserDataTemplate  string          `json:"user_data_template,omitempty"`
	AvailabilityZones []imageAZCreate `json:"availability_zones"`
}

type imageAZView struct {
	AvailabilityZoneID   string `json:"availability_zone_id"`
	AvailabilityZoneName string `json:"availability_zone_name"`
	State                string `json:"state"`
}

type imageView struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Type              string        `json:"type"`
	Public            bool          `json:"public"`
	MinDisk           int           `json:"min_disk"`
	UserDataTemplate  string        `json:"user_data_template"`
	AvailabilityZones []imageAZView `json:"availability_zones"`
}

type fipCreate struct {
	Name               string `json:"name"`
	ProjectID          string `json:"project_id"`
	InterfaceID        string `json:"interface_id"`
	AvailabilityZoneID string `json:"availability_zone_id,omitempty"`
}

type fipView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
}

func (c *Client) CreateInstance(ctx context.Context, req CreateInstanceRequest) (Instance, error) {
	login := req.LinuxLogin
	if login == "" {
		login = "ubuntu"
	}
	meta := map[string]string{
		"hostname": req.Hostname,
		"name":     login,
	}
	if req.PublicKey != "" {
		meta["public_key"] = strings.TrimSpace(req.PublicKey)
	}
	diskType := req.DiskType
	if diskType == "" {
		diskType = "SSD"
	}
	body := vmCreateRequest{
		ProjectID:          c.projectID,
		Name:               req.Name,
		FlavorID:           req.FlavorID,
		ImageID:            req.ImageID,
		AvailabilityZoneID: req.Zone,
		ImageMetadata:      meta,
		Disks: []vmCreateDisk{{
			Name:         req.DiskName,
			Size:         req.DiskSizeGiB,
			DiskTypeName: diskType,
		}},
	}
	if req.SubnetID != "" {
		iface := vmCreateIface{Type: "regular", SubnetID: req.SubnetID}
		if len(req.SecurityGroupIDs) > 0 {
			iface.SecurityGroups = req.SecurityGroupIDs
		}
		body.Interfaces = []vmCreateIface{iface}
	}
	// POST /api/v1.1/vms is a batch handler. One VM = a one-element array.
	// An object or [] is 422. Do not send a batch of N.
	var created []vmView
	if err := c.do(ctx, http.MethodPost, "/api/v1.1/vms", []vmCreateRequest{body}, &created); err != nil {
		return Instance{}, err
	}
	if len(created) != 1 || created[0].ID == "" {
		return Instance{}, fmt.Errorf("create vm: expected exactly one VM, got %d", len(created))
	}
	return instanceFromView(created[0], req.DiskName), nil
}

func (c *Client) GetInstance(ctx context.Context, id string) (Instance, error) {
	var view vmView
	if err := c.do(ctx, http.MethodGet, "/api/v1/vms/"+url.PathEscape(id), nil, &view); err != nil {
		return Instance{}, err
	}
	return instanceFromView(view, ""), nil
}

func (c *Client) SetPower(ctx context.Context, id, state string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/vms/"+url.PathEscape(id)+"/set-power", map[string]string{"state": state}, nil)
}

func (c *Client) DeleteInstance(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/vms/"+url.PathEscape(id), nil, nil)
}

func (c *Client) GetDisk(ctx context.Context, id string) (Disk, error) {
	var view diskView
	if err := c.do(ctx, http.MethodGet, "/api/v1/disks/"+url.PathEscape(id), nil, &view); err != nil {
		return Disk{}, err
	}
	return diskFromView(view), nil
}

func (c *Client) FindDisk(ctx context.Context, name string) (Disk, error) {
	q := url.Values{}
	q.Set("project_id", c.projectID)
	q.Set("name", name)
	items, err := listAll[diskView](ctx, c, "/api/v1/disks", q)
	if err != nil {
		return Disk{}, err
	}
	var matches []diskView
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return Disk{}, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "disk " + name}
	case 1:
		return diskFromView(matches[0]), nil
	default:
		return Disk{}, fmt.Errorf("find disk %q: %d matches", name, len(matches))
	}
}

func (c *Client) DetachDisk(ctx context.Context, instanceID, diskID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/disks/"+url.PathEscape(diskID)+"/detach", map[string]string{"vm_id": instanceID}, nil)
}

func (c *Client) DeleteDisk(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/disks/"+url.PathEscape(id), nil, nil)
}

func (c *Client) GetImage(ctx context.Context, id string) (Image, error) {
	var view imageView
	if err := c.do(ctx, http.MethodGet, "/api/v1/images/"+url.PathEscape(id), nil, &view); err != nil {
		return Image{}, err
	}
	return imageFromView(view), nil
}

func (c *Client) FindImage(ctx context.Context, name string) (Image, error) {
	q := url.Values{}
	q.Set("project_id", c.projectID)
	q.Set("name", name)
	items, err := listAll[imageView](ctx, c, "/api/v1/images", q)
	if err != nil {
		return Image{}, err
	}
	var matches []imageView
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return Image{}, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "image " + name}
	case 1:
		return imageFromView(matches[0]), nil
	default:
		return Image{}, fmt.Errorf("find image %q: %d matches", name, len(matches))
	}
}

// listAll walks offset/limit pages. page_size is 422 on Evolution.
func listAll[T any](ctx context.Context, c *Client, path string, q url.Values) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	const limit = 50
	const maxPages = 20
	var all []T
	for pageNum := 0; pageNum < maxPages; pageNum++ {
		q.Set("limit", strconv.Itoa(limit))
		q.Set("offset", strconv.Itoa(pageNum*limit))
		if q.Get("page_size") != "" {
			return nil, fmt.Errorf("page_size is not allowed on Evolution lists")
		}
		var page listPage[T]
		if err := c.do(ctx, http.MethodGet, path+"?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(page.Items) == 0 || len(page.Items) < limit {
			break
		}
		if page.Total > 0 && len(all) >= page.Total {
			break
		}
	}
	return all, nil
}

func (c *Client) CreateImage(ctx context.Context, req CreateImageRequest) (Image, error) {
	display := req.DisplayName
	if display == "" {
		display = req.Name
	}
	body := imageCreateBody{
		Name:             req.Name,
		DisplayName:      display,
		Description:      req.Description,
		ProjectID:        c.projectID,
		DiskID:           req.DiskID,
		MinCPU:           req.MinCPU,
		MinRAM:           req.MinRAM,
		MinDisk:          req.MinDiskGiB,
		UserDataTemplate: req.UserDataTemplate,
		AvailabilityZones: []imageAZCreate{{
			AvailabilityZoneID: req.Zone,
		}},
	}
	var view imageView
	if err := c.do(ctx, http.MethodPost, "/api/v1/images", body, &view); err != nil {
		return Image{}, err
	}
	return imageFromView(view), nil
}

func (c *Client) DeleteImage(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/images/"+url.PathEscape(id), nil, nil)
}

func (c *Client) GetFloatingIP(ctx context.Context, id string) (FloatingIP, error) {
	var view fipView
	if err := c.do(ctx, http.MethodGet, "/api/v1/floating-ips/"+url.PathEscape(id), nil, &view); err != nil {
		return FloatingIP{}, err
	}
	return FloatingIP{ID: view.ID, Name: view.Name, IPAddress: view.IPAddress}, nil
}

func (c *Client) CreateFloatingIP(ctx context.Context, name, interfaceID, zone string) (FloatingIP, error) {
	var view fipView
	err := c.do(ctx, http.MethodPost, "/api/v1/floating-ips", fipCreate{
		Name:               name,
		ProjectID:          c.projectID,
		InterfaceID:        interfaceID,
		AvailabilityZoneID: zone,
	}, &view)
	if err != nil {
		return FloatingIP{}, err
	}
	out := FloatingIP{ID: view.ID, Name: view.Name, IPAddress: view.IPAddress}
	if out.ID == "" {
		return FloatingIP{}, fmt.Errorf("create floating IP: empty id")
	}
	if out.IPAddress != "" {
		return out, nil
	}
	last := out
	err = poll(ctx, 2*time.Second, fipAddressTimeout, func(ctx context.Context) (bool, error) {
		got, getErr := c.GetFloatingIP(ctx, out.ID)
		if getErr != nil {
			if isNotFound(getErr) {
				return false, nil
			}
			return false, getErr
		}
		last = got
		return got.IPAddress != "", nil
	})
	if err != nil {
		if last.IPAddress == "" {
			return last, fmt.Errorf("floating IP %s has an empty address: %w", out.ID, err)
		}
		return last, err
	}
	return last, nil
}

func (c *Client) DeleteFloatingIP(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/floating-ips/"+url.PathEscape(id), nil, nil)
}

func instanceFromView(view vmView, bootName string) Instance {
	out := Instance{ID: view.ID, Name: view.Name, State: view.State, BootDiskName: bootName}
	for _, iface := range view.Interfaces {
		if iface.Primary || out.PrivateIP == "" {
			out.PrivateIP = iface.IPAddress
			out.InterfaceID = iface.ID
			if iface.FloatingIP != nil {
				out.FloatingIP = iface.FloatingIP.IPAddress
				out.FloatingIPID = iface.FloatingIP.ID
			}
		}
	}
	for _, disk := range view.Disks {
		if disk.ID == "" {
			continue
		}
		use := disk.Bootable || disk.Primary || (bootName != "" && disk.Name == bootName)
		if out.BootDiskID == "" || use {
			out.BootDiskID = disk.ID
			if disk.Name != "" {
				out.BootDiskName = disk.Name
			}
			if disk.Bootable || disk.Primary {
				break
			}
		}
	}
	if out.BootDiskID == "" && len(view.Disks) == 1 && view.Disks[0].ID != "" {
		out.BootDiskID = view.Disks[0].ID
		if view.Disks[0].Name != "" {
			out.BootDiskName = view.Disks[0].Name
		}
	}
	return out
}

func diskFromView(view diskView) Disk {
	vm := ""
	for _, ref := range view.VMs {
		if ref.ID != "" {
			vm = ref.ID
			break
		}
	}
	return Disk{ID: view.ID, Name: view.Name, State: view.State, Bootable: view.Bootable, VMID: vm}
}

func imageFromView(view imageView) Image {
	states := map[string]string{}
	for _, az := range view.AvailabilityZones {
		key := az.AvailabilityZoneID
		if key == "" {
			key = az.AvailabilityZoneName
		}
		states[key] = strings.ToLower(az.State)
	}
	return Image{
		ID:               view.ID,
		Name:             view.Name,
		Type:             view.Type,
		Public:           view.Public,
		MinDiskGiB:       view.MinDisk,
		UserDataTemplate: view.UserDataTemplate,
		ZoneStates:       states,
	}
}
