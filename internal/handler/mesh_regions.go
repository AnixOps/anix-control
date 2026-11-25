package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MeshHandler serves the panel side of the intranet mesh
// (internal/meshregion): settings, regions, members and their addresses,
// host groups, device names, the intranet names and region changes. The
// /api/v5 group admits administrators and records every write in the
// operation log.
type MeshHandler struct {
	db  func() *gorm.DB
	now func() time.Time
}

// NewMeshHandler reads the loaded database on each request.
func NewMeshHandler() *MeshHandler {
	return &MeshHandler{db: database.Get, now: time.Now}
}

func (h *MeshHandler) service(c *gin.Context) (*meshregion.Service, bool) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return nil, false
	}
	return &meshregion.Service{DB: db.WithContext(c.Request.Context()), Now: h.now}, true
}

func meshError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, meshregion.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, meshregion.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, meshregion.ErrConflict):
		kernelError(c, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, meshregion.ErrExhausted):
		kernelError(c, http.StatusConflict, "address_exhausted", "no free address is left in the range")
	default:
		kernelDBError(c, err)
	}
}

func meshID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "id must be a positive integer")
		return 0, false
	}
	return uint(id), true
}

func meshActor(c *gin.Context) *uint {
	if id := kernelActorID(c); id != 0 {
		return &id
	}
	return nil
}

// bind reads the JSON body into request, after answering an error itself.
func meshBind(c *gin.Context, request any) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "a JSON body with the documented fields is expected")
		return false
	}
	return true
}

// meshOverview is the mesh page's first answer.
type meshOverview struct {
	Settings   model.MeshSettings         `json:"settings"`
	Regions    []meshregion.RegionView    `json:"regions"`
	HostGroups []meshregion.HostGroupView `json:"host_groups"`
	// PendingChanges counts the region changes awaiting a decision.
	PendingChanges int `json:"pending_changes"`
}

// Overview godoc
// @Summary The intranet mesh at a glance
// @Description The settings (name_suffix, host_group_supernet, host_group_prefix, reserved_subnets), every region with its counts and warnings (few_border_nodes, border_without_probe_address, device_pool_low), every host group, and how many region changes await a decision. The first call creates the four default regions: cn 中国大陆, ap 亚太, eu 欧洲, na 北美.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Success 200 {object} meshOverview
// @Router /api/v5/mesh [get]
func (h *MeshHandler) Overview(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	settings, err := s.Settings()
	if err != nil {
		meshError(c, err)
		return
	}
	regions, err := s.Regions()
	if err != nil {
		meshError(c, err)
		return
	}
	groups, err := s.HostGroups()
	if err != nil {
		meshError(c, err)
		return
	}
	pending, err := s.Changes("pending")
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, meshOverview{Settings: settings, Regions: regions, HostGroups: groups, PendingChanges: len(pending)})
}

type meshSettingsRequest struct {
	NameSuffix        string   `json:"name_suffix"`
	HostGroupSupernet string   `json:"host_group_supernet"`
	HostGroupPrefix   int      `json:"host_group_prefix"`
	ReservedSubnets   []string `json:"reserved_subnets"`
}

// UpdateSettings godoc
// @Summary Change the mesh settings
// @Description name_suffix is the intranet domain suffix (mesh.internal when empty). host_group_supernet is the private IPv4 range host group segments come from, host_group_prefix their prefix length; both change only while no host group exists. reserved_subnets are IPv4 ranges no region or host group may overlap, such as the existing network's subnet during the migration. 400 invalid_request, 409 conflict.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshSettingsRequest true "settings"
// @Success 200 {object} model.MeshSettings
// @Router /api/v5/mesh/settings [put]
func (h *MeshHandler) UpdateSettings(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshSettingsRequest
	if !meshBind(c, &request) {
		return
	}
	settings, err := s.UpdateSettings(meshregion.SettingsUpdate{
		NameSuffix: request.NameSuffix, HostGroupSupernet: request.HostGroupSupernet,
		HostGroupPrefix: request.HostGroupPrefix, ReservedSubnets: request.ReservedSubnets,
	})
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, settings)
}

type meshRegionRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	SortOrder   int    `json:"sort_order"`
	Subnet      string `json:"subnet"`
	ServerRange string `json:"server_range"`
	DevicePool  string `json:"device_pool"`
}

func (r meshRegionRequest) input() meshregion.RegionInput {
	return meshregion.RegionInput{Code: r.Code, Name: r.Name, SortOrder: r.SortOrder, Subnet: r.Subnet, ServerRange: r.ServerRange, DevicePool: r.DevicePool}
}

// CreateRegion godoc
// @Summary Add a region
// @Description A region is an independent mesh network with forward secrecy. code is its DNS label (1 to 16 lowercase letters, digits and inner hyphens); subnet a private IPv4 range of /8 to /24 no other region, the host group supernet or a reserved range overlaps; server_range (fixed addresses) and device_pool (addresses kept per device) two ranges inside it that do not overlap. The network name is generated. 400 invalid_request, 409 conflict (code taken, subnet overlaps a region).
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshRegionRequest true "region"
// @Success 201 {object} model.MeshRegion
// @Router /api/v5/mesh/regions [post]
func (h *MeshHandler) CreateRegion(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshRegionRequest
	if !meshBind(c, &request) {
		return
	}
	region, err := s.CreateRegion(request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, region)
}

// UpdateRegion godoc
// @Summary Change a region
// @Description Changes name, sort_order, subnet, server_range and device_pool; the code stays. The subnet changes only while the region has no member, and the device pool only while every pool member's address stays inside it. 404 not_found, 409 conflict.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "region id"
// @Param request body meshRegionRequest true "region"
// @Success 200 {object} model.MeshRegion
// @Router /api/v5/mesh/regions/{id} [put]
func (h *MeshHandler) UpdateRegion(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshRegionRequest
	if !meshBind(c, &request) {
		return
	}
	region, err := s.UpdateRegion(id, request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, region)
}

// DeleteRegion godoc
// @Summary Delete a region
// @Description Only a region without members and without pending changes to it. 404 not_found, 409 conflict.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Param id path int true "region id"
// @Success 200 {object} map[string]any
// @Router /api/v5/mesh/regions/{id} [delete]
func (h *MeshHandler) DeleteRegion(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	if err := s.DeleteRegion(id); err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id})
}

type meshMemberRequest struct {
	RegionID     uint   `json:"region_id"`
	NodeRef      string `json:"node_ref"`
	Hostname     string `json:"hostname"`
	Role         string `json:"role"`
	AddressMode  string `json:"address_mode"`
	Address      string `json:"address"`
	ProbeAddress string `json:"probe_address"`
}

func (r meshMemberRequest) input() meshregion.MemberInput {
	return meshregion.MemberInput{
		RegionID: r.RegionID, NodeRef: r.NodeRef, Hostname: r.Hostname, Role: r.Role,
		AddressMode: r.AddressMode, Address: r.Address, ProbeAddress: r.ProbeAddress,
	}
}

// Members godoc
// @Summary The mesh members
// @Description Every member, or those of region_id, by region and hostname, with region_code, fqdn (hostname.region.suffix) and warnings (no_address, pool_address_idle: unused for 30 days, so the next device that needs an address may take it).
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Param region_id query int false "region id"
// @Success 200 {array} meshregion.MemberView
// @Router /api/v5/mesh/members [get]
func (h *MeshHandler) Members(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	var regionID uint64
	if raw := c.Query("region_id"); raw != "" {
		var err error
		if regionID, err = strconv.ParseUint(raw, 10, 32); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", "region_id must be a positive integer")
			return
		}
	}
	members, err := s.Members(uint(regionID))
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, members)
}

// CreateMember godoc
// @Summary Add a member to a region
// @Description role is border, gateway, server or device. Border nodes, gateways and servers use a fixed address (address_mode fixed): the address given, inside the region's subnet and outside its device pool, or the lowest free address of the server range. A device uses the device pool by default (address_mode pool) and keeps its address until it has gone unused for 30 days. hostname is one DNS label, unique among the region's names. node_ref names the Agent node (proxy-<id> or forward-<id>); probe_address is a border node's public host:port. 400 invalid_request, 409 conflict or address_exhausted.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshMemberRequest true "member"
// @Success 201 {object} model.MeshMember
// @Router /api/v5/mesh/members [post]
func (h *MeshHandler) CreateMember(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshMemberRequest
	if !meshBind(c, &request) {
		return
	}
	member, err := s.CreateMember(request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, member)
}

// UpdateMember godoc
// @Summary Change a member
// @Description Changes hostname, role, address_mode, node_ref, probe_address and a fixed address within the member's region (region_id is ignored; a region change is a move). Changing the mode gives a new address. 400 invalid_request, 404 not_found, 409 conflict.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "member id"
// @Param request body meshMemberRequest true "member"
// @Success 200 {object} model.MeshMember
// @Router /api/v5/mesh/members/{id} [put]
func (h *MeshHandler) UpdateMember(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshMemberRequest
	if !meshBind(c, &request) {
		return
	}
	member, err := s.UpdateMember(id, request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, member)
}

// DeleteMember godoc
// @Summary Remove a member
// @Description Removes the member, the names it publishes and its region changes. A host group's gateway is refused (409 conflict) until the group has another gateway.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Param id path int true "member id"
// @Success 200 {object} map[string]any
// @Router /api/v5/mesh/members/{id} [delete]
func (h *MeshHandler) DeleteMember(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	if err := s.DeleteMember(id); err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id})
}

type meshMoveRequest struct {
	RegionID uint `json:"region_id"`
	// Address is the new fixed address; empty picks one.
	Address string `json:"address"`
	Note    string `json:"note"`
}

// MoveMember godoc
// @Summary Move a member to another region
// @Description An administrator's own move, recorded as an approved region change. The member gets a new address in the region (the fixed address given, or the lowest free one; a device's from the device pool): connections to its old address end. A host group's gateway moves only after the group's other gateway. A pending request of the member for the same region is approved by the move; other pending requests are rejected. 400 invalid_request, 404 not_found, 409 conflict or address_exhausted.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "member id"
// @Param request body meshMoveRequest true "target region"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v5/mesh/members/{id}/move [post]
func (h *MeshHandler) MoveMember(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshMoveRequest
	if !meshBind(c, &request) {
		return
	}
	change, err := s.Move(id, request.RegionID, meshregion.Decision{Address: request.Address, Note: request.Note, DecidedBy: meshActor(c)})
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, change)
}

type meshHostGroupRequest struct {
	Name            string `json:"name"`
	GatewayMemberID uint   `json:"gateway_member_id"`
	StandbyMemberID uint   `json:"standby_member_id"`
	Segment         string `json:"segment"`
}

func (r meshHostGroupRequest) input() meshregion.HostGroupInput {
	return meshregion.HostGroupInput{Name: r.Name, GatewayMemberID: r.GatewayMemberID, StandbyMemberID: r.StandbyMemberID, Segment: r.Segment}
}

// HostGroups godoc
// @Summary The host groups
// @Description Every host group by name, with its region (its gateway's), gateway and standby hostnames and how many names it has.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Success 200 {array} meshregion.HostGroupView
// @Router /api/v5/mesh/host-groups [get]
func (h *MeshHandler) HostGroups(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	groups, err := s.HostGroups()
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, groups)
}

// CreateHostGroup godoc
// @Summary Add a host group
// @Description The virtual machines of one physical host, represented by gateway_member_id (a gateway member) and an optional standby_member_id (another gateway of the same region). segment is the group's range in the host group supernet, of the host group prefix length; empty takes the first free one. 400 invalid_request, 409 conflict or address_exhausted.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshHostGroupRequest true "host group"
// @Success 201 {object} model.MeshHostGroup
// @Router /api/v5/mesh/host-groups [post]
func (h *MeshHandler) CreateHostGroup(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshHostGroupRequest
	if !meshBind(c, &request) {
		return
	}
	group, err := s.CreateHostGroup(request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, group)
}

// UpdateHostGroup godoc
// @Summary Change a host group
// @Description Changes the name and the gateways (in the group's region); the segment stays. The group's names follow the new gateway. 400 invalid_request, 404 not_found.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "host group id"
// @Param request body meshHostGroupRequest true "host group"
// @Success 200 {object} model.MeshHostGroup
// @Router /api/v5/mesh/host-groups/{id} [put]
func (h *MeshHandler) UpdateHostGroup(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshHostGroupRequest
	if !meshBind(c, &request) {
		return
	}
	group, err := s.UpdateHostGroup(id, request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, group)
}

// DeleteHostGroup godoc
// @Summary Delete a host group
// @Description Deletes the group and the names of its virtual machines. 404 not_found.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Param id path int true "host group id"
// @Success 200 {object} map[string]any
// @Router /api/v5/mesh/host-groups/{id} [delete]
func (h *MeshHandler) DeleteHostGroup(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	if err := s.DeleteHostGroup(id); err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id})
}

type meshDeviceNameRequest struct {
	MemberID    uint   `json:"member_id"`
	HostGroupID uint   `json:"host_group_id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
}

func (r meshDeviceNameRequest) input() meshregion.DeviceNameInput {
	return meshregion.DeviceNameInput{MemberID: r.MemberID, HostGroupID: r.HostGroupID, Name: r.Name, Address: r.Address}
}

// DeviceNames godoc
// @Summary The device names
// @Description The names administrators gave to machines that are not members: virtual machines of a host group and devices in a gateway's local network, by name.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.MeshDeviceName
// @Router /api/v5/mesh/device-names [get]
func (h *MeshHandler) DeviceNames(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	rows, err := s.DeviceNames()
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// CreateDeviceName godoc
// @Summary Name a virtual machine or a local device
// @Description With host_group_id, a virtual machine of the group: address in the group's segment; the group's gateway publishes it. Without, a device in the local network of the gateway member_id: a private address outside every region's subnet and the host group supernet. name is one DNS label, unique among the region's names. Devices are named by hand only; there is no network scan. 400 invalid_request, 409 conflict.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshDeviceNameRequest true "device name"
// @Success 201 {object} model.MeshDeviceName
// @Router /api/v5/mesh/device-names [post]
func (h *MeshHandler) CreateDeviceName(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshDeviceNameRequest
	if !meshBind(c, &request) {
		return
	}
	row, err := s.CreateDeviceName(request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, row)
}

// UpdateDeviceName godoc
// @Summary Change a device name
// @Description Same fields and checks as creating one. 400 invalid_request, 404 not_found, 409 conflict.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "device name id"
// @Param request body meshDeviceNameRequest true "device name"
// @Success 200 {object} model.MeshDeviceName
// @Router /api/v5/mesh/device-names/{id} [put]
func (h *MeshHandler) UpdateDeviceName(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshDeviceNameRequest
	if !meshBind(c, &request) {
		return
	}
	row, err := s.UpdateDeviceName(id, request.input())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, row)
}

// DeleteDeviceName godoc
// @Summary Delete a device name
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Param id path int true "device name id"
// @Success 200 {object} map[string]any
// @Router /api/v5/mesh/device-names/{id} [delete]
func (h *MeshHandler) DeleteDeviceName(c *gin.Context) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	if err := s.DeleteDeviceName(id); err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"id": id})
}

// Names godoc
// @Summary The intranet names
// @Description The names the panel publishes: every member with an address and every device name, as fqdn (name.region.suffix) with its address, source (member, host_group or device) and the member that publishes it, by full name. duplicate marks a name used twice in a region.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Success 200 {object} meshregion.NameList
// @Router /api/v5/mesh/names [get]
func (h *MeshHandler) Names(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	list, err := s.Names()
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, list)
}

// Changes godoc
// @Summary The region changes
// @Description The latest 500 region changes, newest first, or those of status (pending, approved or rejected), with the member's hostname and the region codes.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Param status query string false "pending, approved or rejected"
// @Success 200 {array} meshregion.ChangeView
// @Router /api/v5/mesh/region-changes [get]
func (h *MeshHandler) Changes(c *gin.Context) {
	s, ok := h.service(c)
	if !ok {
		return
	}
	changes, err := s.Changes(c.Query("status"))
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, changes)
}

type meshDecisionRequest struct {
	// Address is the new fixed address on approval; empty picks one.
	Address string `json:"address"`
	Note    string `json:"note"`
}

// ApproveChange godoc
// @Summary Approve a region change
// @Description Moves the member to the region it asked for, with a new address (the fixed address given, or the lowest free one; a device's from the device pool), and records old_address and new_address. 404 not_found, 409 conflict (already decided, the member moved meanwhile, a host group's other gateway) or address_exhausted.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "region change id"
// @Param request body meshDecisionRequest false "address and note"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v5/mesh/region-changes/{id}/approve [post]
func (h *MeshHandler) ApproveChange(c *gin.Context) {
	h.decide(c, true)
}

// RejectChange godoc
// @Summary Reject a region change
// @Description The member stays in its region. 404 not_found, 409 conflict (already decided).
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "region change id"
// @Param request body meshDecisionRequest false "note"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v5/mesh/region-changes/{id}/reject [post]
func (h *MeshHandler) RejectChange(c *gin.Context) {
	h.decide(c, false)
}

func (h *MeshHandler) decide(c *gin.Context, approve bool) {
	id, ok := meshID(c)
	if !ok {
		return
	}
	s, ok := h.service(c)
	if !ok {
		return
	}
	var request meshDecisionRequest
	if c.Request.ContentLength != 0 && !meshBind(c, &request) {
		return
	}
	decision := meshregion.Decision{Address: request.Address, Note: request.Note, DecidedBy: meshActor(c)}
	decide := s.Reject
	if approve {
		decide = s.Approve
	}
	change, err := decide(id, decision)
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, change)
}
