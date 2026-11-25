package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
)

// The mesh's addresses and names (internal/meshregion): the global
// settings, the personal devices and their pool addresses, the host groups,
// the names gateway nodes publish, the intranet name list and the region
// changes. Every route answers 404 feature_disabled while the gated feature
// "mesh" is off, and every change is audited (module mesh).

type meshDecisionRequest struct {
	Note string `json:"note"`
}

type meshDeviceMoveRequest struct {
	RegionID uint `json:"region_id"`
}

func (h *MeshHandler) actor(c *gin.Context) *uint {
	if id := kernelActorID(c); id != 0 {
		return &id
	}
	return nil
}

// MeshSummary godoc
// @Summary The mesh settings, device pools and pending region changes
// @Description The global settings (intranet domain suffix, host group supernet and segment length, reserved ranges; the defaults until saved), each region's device pool (size, addresses in use, low when less than a tenth is free) and how many region changes wait for a decision.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} meshregion.Summary
// @Router /api/v4/kernel/mesh/summary [get]
func (h *MeshHandler) MeshSummary(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	summary, err := meshregion.LoadSummary(c.Request.Context(), db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, summary)
}

// SaveMeshSettings godoc
// @Summary Change the mesh settings
// @Description name_suffix (DNS labels, such as mesh.internal: a name is name.region-code.suffix), host_group_supernet (a private network of /8 to /24 outside every region) and host_group_prefix (the segment length; both change only while no host group exists), reserved_ranges (at most 32 networks no region or host group may overlap). 409 mesh_conflict when a range overlaps a region. Audited (module mesh, action mesh_settings_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.SettingsInput true "settings"
// @Success 200 {object} meshregion.Settings
// @Router /api/v4/kernel/mesh/settings [put]
func (h *MeshHandler) SaveMeshSettings(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.SettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "name_suffix, host_group_supernet, host_group_prefix and reserved_ranges are expected")
		return
	}
	settings, err := meshregion.SaveSettings(c.Request.Context(), db, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_settings_update", 1, fmt.Sprintf(`{"name_suffix":%q,"host_group_supernet":%q,"host_group_prefix":%d,"reserved_ranges":%d}`,
		settings.NameSuffix, settings.HostGroupSupernet, settings.HostGroupPrefix, len(settings.ReservedRanges)))
	kernelData(c, http.StatusOK, settings)
}

// ListMeshDevices godoc
// @Summary The mesh's personal devices
// @Description Every device (or those of region_id) with its region, pool address, when the address was last used, its intranet name and its warnings: no_address (the address went to another device), address_idle (unused for 30 days; it may go to another device when the pool is full).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param region_id query int false "region id"
// @Success 200 {array} meshregion.DeviceView
// @Router /api/v4/kernel/mesh/devices [get]
func (h *MeshHandler) ListMeshDevices(c *gin.Context) {
	db, ok := h.database(c)
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
	devices, err := meshregion.Devices(c.Request.Context(), db, uint(regionID), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, devices)
}

// CreateMeshDevice godoc
// @Summary Add a personal device to a mesh region
// @Description The device gets the lowest free address of the region's device pool (the upper half of its range), or else the address unused longest if unused for 30 days, which its device loses. hostname is one DNS label; device_key, the device's identity key, may be empty. 409 pool_full when the pool has neither. Audited (module mesh, action mesh_device_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "region id"
// @Param request body meshregion.DeviceInput true "hostname and device_key"
// @Success 201 {object} model.MeshDevice
// @Router /api/v4/kernel/mesh/regions/{id}/devices [post]
func (h *MeshHandler) CreateMeshDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	regionID, ok := pathID(c, "id")
	if !ok {
		return
	}
	var input meshregion.DeviceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "hostname and device_key are expected")
		return
	}
	row, err := meshregion.CreateDevice(c.Request.Context(), db, regionID, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_create", row.ID, fmt.Sprintf(`{"hostname":%q,"region_id":%d,"address":%q}`, row.Hostname, row.RegionID, meshText(row.Address)))
	kernelData(c, http.StatusCreated, row)
}

// UpdateMeshDevice godoc
// @Summary Change a personal device
// @Description Changes hostname and device_key; the region changes only through a region change. Audited (module mesh, action mesh_device_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Param request body meshregion.DeviceInput true "hostname and device_key"
// @Success 200 {object} model.MeshDevice
// @Router /api/v4/kernel/mesh/devices/{device_id} [put]
func (h *MeshHandler) UpdateMeshDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	var input meshregion.DeviceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "hostname and device_key are expected")
		return
	}
	row, err := meshregion.UpdateDevice(c.Request.Context(), db, id, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_update", row.ID, fmt.Sprintf(`{"hostname":%q,"device_key":%t}`, row.Hostname, row.DeviceKey != nil))
	kernelData(c, http.StatusOK, row)
}

// DeleteMeshDevice godoc
// @Summary Remove a personal device
// @Description Its address becomes free; its pending region change is rejected. Audited (module mesh, action mesh_device_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Success 204
// @Router /api/v4/kernel/mesh/devices/{device_id} [delete]
func (h *MeshHandler) DeleteMeshDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	if err := meshregion.DeleteDevice(c.Request.Context(), db, id, h.now()); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_delete", id, fmt.Sprintf(`{"device_id":%d}`, id))
	c.Status(http.StatusNoContent)
}

// MoveMeshDevice godoc
// @Summary Move a personal device to another region
// @Description At once: the device gets an address of the other region's device pool, recorded as an administrator's approved region change; its pending change is settled. Audited (module mesh, action mesh_device_move).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Param request body meshDeviceMoveRequest true "region_id"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v4/kernel/mesh/devices/{device_id}/move [post]
func (h *MeshHandler) MoveMeshDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	var request meshDeviceMoveRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.RegionID == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "region_id is expected")
		return
	}
	row, err := meshregion.MoveDevice(c.Request.Context(), db, id, request.RegionID, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_move", id, fmt.Sprintf(`{"device_id":%d,"from_region_id":%d,"to_region_id":%d,"address":%q}`, id, row.FromRegionID, row.ToRegionID, row.NewAddress))
	kernelData(c, http.StatusOK, row)
}

// AssignMeshDeviceAddress godoc
// @Summary Give a personal device a new pool address
// @Description For a device whose address went to another device after 30 days unused: the lowest free address of its region's device pool; for a full node, the lowest free address of the server segment. Audited (module mesh, action mesh_device_address).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Success 200 {object} model.MeshDevice
// @Router /api/v4/kernel/mesh/devices/{device_id}/address [post]
func (h *MeshHandler) AssignMeshDeviceAddress(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	row, err := meshregion.AssignDeviceAddress(c.Request.Context(), db, id, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_address", id, fmt.Sprintf(`{"device_id":%d,"address":%q}`, id, *row.Address))
	kernelData(c, http.StatusOK, row)
}

// meshSubnetsInput is the body of SetMeshDeviceSubnets.
type meshSubnetsInput struct {
	Subnets []string `json:"subnets"`
}

// SetMeshDeviceSubnets godoc
// @Summary Confirm the subnets a full node advertises
// @Description Replaces the local subnets a full node without an Agent advertises in its network (subnets.advertise of its configuration): private IPv4 networks, at most 32, that overlap no region, the host group supernet, a reserved range or another full node's subnets; an empty list advertises none; the body is at most 16 KiB (413 payload_too_large). The node reads them with its next configuration. 400 invalid_request for an access device. Audited (module mesh, action mesh_device_subnets).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Param request body meshSubnetsInput true "subnets"
// @Success 200 {array} string
// @Router /api/v4/kernel/mesh/devices/{device_id}/subnets [put]
func (h *MeshHandler) SetMeshDeviceSubnets(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	var input meshSubnetsInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, meshDeviceRequestBytes)
	if err := c.ShouldBindJSON(&input); err != nil {
		if tooLarge(err) {
			kernelError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "the request is too large")
			return
		}
		kernelError(c, http.StatusBadRequest, "invalid_request", "subnets is expected")
		return
	}
	subnets, err := meshregion.SetDeviceSubnets(c.Request.Context(), db, id, input.Subnets, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	detail, _ := json.Marshal(map[string]any{"device_id": id, "subnets": subnets})
	h.audit(c, db, "mesh_device_subnets", id, string(detail))
	kernelData(c, http.StatusOK, subnets)
}

// ApproveMeshDeviceSubnets godoc
// @Summary Approve a full node's subnet request
// @Description Confirms the subnets a full node without an Agent asked to advertise (its subnet_request on the devices) as its subnets, with the checks of the subnets route (409 mesh_conflict on an overlap). subnets repeats the requested list as the administrator saw it; 409 mesh_conflict when the node has changed its request since. 404 not_found when nothing is pending. Audited (module mesh, action mesh_device_subnets_approve).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Param request body meshSubnetsInput true "subnets"
// @Success 200 {array} string
// @Router /api/v4/kernel/mesh/devices/{device_id}/subnet-request/approve [post]
func (h *MeshHandler) ApproveMeshDeviceSubnets(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	var input meshSubnetsInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, meshDeviceRequestBytes)
	if err := c.ShouldBindJSON(&input); err != nil {
		if tooLarge(err) {
			kernelError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "the request is too large")
			return
		}
		kernelError(c, http.StatusBadRequest, "invalid_request", "subnets is expected")
		return
	}
	subnets, err := meshregion.ApproveDeviceSubnets(c.Request.Context(), db, id, input.Subnets, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	detail, _ := json.Marshal(map[string]any{"device_id": id, "subnets": subnets})
	h.audit(c, db, "mesh_device_subnets_approve", id, string(detail))
	kernelData(c, http.StatusOK, subnets)
}

// RejectMeshDeviceSubnets godoc
// @Summary Reject a full node's subnet request
// @Description Removes the pending subnet request of a full node without an Agent; its confirmed subnets stay. 404 not_found when nothing is pending. Audited (module mesh, action mesh_device_subnets_reject).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Success 204
// @Router /api/v4/kernel/mesh/devices/{device_id}/subnet-request [delete]
func (h *MeshHandler) RejectMeshDeviceSubnets(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	subnets, err := meshregion.RejectDeviceSubnets(c.Request.Context(), db, id, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	detail, _ := json.Marshal(map[string]any{"device_id": id, "subnets": subnets})
	h.audit(c, db, "mesh_device_subnets_reject", id, string(detail))
	c.Status(http.StatusNoContent)
}

// ListMeshHostGroups godoc
// @Summary The mesh's host groups
// @Description Every host group by segment, with its gateway node, standby node and the gateway's home region.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} meshregion.HostGroupView
// @Router /api/v4/kernel/mesh/host-groups [get]
func (h *MeshHandler) ListMeshHostGroups(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	groups, err := meshregion.HostGroups(c.Request.Context(), db)
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, groups)
}

// CreateMeshHostGroup godoc
// @Summary Add a host group
// @Description A host group is the virtual machines or containers behind a gateway node, on a segment of the host group supernet: segment (empty for the next free one), gateway and standby (nodes in the mesh; the standby, which may be empty, has the gateway's home region). Audited (module mesh, action mesh_host_group_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.HostGroupInput true "name, segment, gateway, standby"
// @Success 201 {object} model.MeshHostGroup
// @Router /api/v4/kernel/mesh/host-groups [post]
func (h *MeshHandler) CreateMeshHostGroup(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.HostGroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "name, segment, gateway and standby are expected")
		return
	}
	row, err := meshregion.CreateHostGroup(c.Request.Context(), db, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_host_group_create", row.ID, fmt.Sprintf(`{"segment":%q,"gateway":%q,"standby":%q}`, row.Segment, row.GatewayRef, meshText(row.StandbyRef)))
	kernelData(c, http.StatusCreated, row)
}

// UpdateMeshHostGroup godoc
// @Summary Change a host group
// @Description Changes the name, gateway and standby; the segment never changes. The names published for its hosts follow the new gateway. Audited (module mesh, action mesh_host_group_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param group_id path int true "host group id"
// @Param request body meshregion.HostGroupInput true "name, gateway, standby"
// @Success 200 {object} model.MeshHostGroup
// @Router /api/v4/kernel/mesh/host-groups/{group_id} [put]
func (h *MeshHandler) UpdateMeshHostGroup(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "group_id")
	if !ok {
		return
	}
	var input meshregion.HostGroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "name, gateway and standby are expected")
		return
	}
	row, err := meshregion.UpdateHostGroup(c.Request.Context(), db, id, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_host_group_update", row.ID, fmt.Sprintf(`{"segment":%q,"gateway":%q,"standby":%q}`, row.Segment, row.GatewayRef, meshText(row.StandbyRef)))
	kernelData(c, http.StatusOK, row)
}

// DeleteMeshHostGroup godoc
// @Summary Remove a host group
// @Description Removes the host group and the names of its hosts. Audited (module mesh, action mesh_host_group_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param group_id path int true "host group id"
// @Success 204
// @Router /api/v4/kernel/mesh/host-groups/{group_id} [delete]
func (h *MeshHandler) DeleteMeshHostGroup(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "group_id")
	if !ok {
		return
	}
	if err := meshregion.DeleteHostGroup(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_host_group_delete", id, fmt.Sprintf(`{"host_group_id":%d}`, id))
	c.Status(http.StatusNoContent)
}

// ListMeshDeviceNames godoc
// @Summary The names gateway nodes publish
// @Description Every published name by gateway: a host of a host group (host_group_id) or a device of the gateway's local network.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.MeshDeviceName
// @Router /api/v4/kernel/mesh/device-names [get]
func (h *MeshHandler) ListMeshDeviceNames(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	names, err := meshregion.DeviceNames(c.Request.Context(), db)
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, names)
}

// CreateMeshDeviceName godoc
// @Summary Publish a name for a host behind a gateway
// @Description name is one DNS label. With host_group_id: an address in the group's segment, published by the group's gateway. Without: gateway (a node in the mesh) and a private address outside every region and the host group supernet (a device of the gateway's local network). Audited (module mesh, action mesh_device_name_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.DeviceNameInput true "gateway, host_group_id, name, address"
// @Success 201 {object} model.MeshDeviceName
// @Router /api/v4/kernel/mesh/device-names [post]
func (h *MeshHandler) CreateMeshDeviceName(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.DeviceNameInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "gateway, host_group_id, name and address are expected")
		return
	}
	row, err := meshregion.CreateDeviceName(c.Request.Context(), db, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_name_create", row.ID, fmt.Sprintf(`{"name":%q,"address":%q,"gateway":%q}`, row.Name, row.Address, row.GatewayRef))
	kernelData(c, http.StatusCreated, row)
}

// UpdateMeshDeviceName godoc
// @Summary Change a published name
// @Description As when publishing it. Audited (module mesh, action mesh_device_name_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param name_id path int true "device name id"
// @Param request body meshregion.DeviceNameInput true "gateway, host_group_id, name, address"
// @Success 200 {object} model.MeshDeviceName
// @Router /api/v4/kernel/mesh/device-names/{name_id} [put]
func (h *MeshHandler) UpdateMeshDeviceName(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "name_id")
	if !ok {
		return
	}
	var input meshregion.DeviceNameInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "gateway, host_group_id, name and address are expected")
		return
	}
	row, err := meshregion.UpdateDeviceName(c.Request.Context(), db, id, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_name_update", row.ID, fmt.Sprintf(`{"name":%q,"address":%q,"gateway":%q}`, row.Name, row.Address, row.GatewayRef))
	kernelData(c, http.StatusOK, row)
}

// DeleteMeshDeviceName godoc
// @Summary Remove a published name
// @Description Audited (module mesh, action mesh_device_name_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param name_id path int true "device name id"
// @Success 204
// @Router /api/v4/kernel/mesh/device-names/{name_id} [delete]
func (h *MeshHandler) DeleteMeshDeviceName(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "name_id")
	if !ok {
		return
	}
	if err := meshregion.DeleteDeviceName(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_name_delete", id, fmt.Sprintf(`{"device_name_id":%d}`, id))
	c.Status(http.StatusNoContent)
}

// ListMeshNames godoc
// @Summary The intranet name list
// @Description Every node in the mesh, every device with an address and every published name as name.region-code.suffix (the node's or the gateway's home region), by name. A name that occurs more than once is answered every time, marked duplicate. unnamed lists the nodes whose name is no DNS label.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} meshregion.NameList
// @Router /api/v4/kernel/mesh/names [get]
func (h *MeshHandler) ListMeshNames(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	names, err := meshregion.Names(c.Request.Context(), db)
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, names)
}

// ListMeshChanges godoc
// @Summary The region changes
// @Description The latest 500 region changes, pending first, or those of one status (pending, approved, rejected), with their regions' codes.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param status query string false "pending, approved or rejected"
// @Success 200 {array} meshregion.ChangeView
// @Router /api/v4/kernel/mesh/changes [get]
func (h *MeshHandler) ListMeshChanges(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	changes, err := meshregion.Changes(c.Request.Context(), db, c.Query("status"))
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, changes)
}

// CreateMeshChange godoc
// @Summary Record a region change request
// @Description Records, for a node (node) or a device (device_id), a pending request to move to to_region_id, on the member's behalf; it moves when approved. A member has at most one pending request (409 mesh_conflict). Audited (module mesh, action mesh_change_request).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.ChangeInput true "node or device_id, to_region_id, reason"
// @Success 201 {object} model.MeshRegionChange
// @Router /api/v4/kernel/mesh/changes [post]
func (h *MeshHandler) CreateMeshChange(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.ChangeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node or device_id, to_region_id and reason are expected")
		return
	}
	row, err := meshregion.RequestChange(c.Request.Context(), db, input, model.MeshChangeAdmin, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_change_request", row.ID, fmt.Sprintf(`{"node":%q,"device_id":%d,"from_region_id":%d,"to_region_id":%d}`, meshText(row.NodeRef), meshID(row.DeviceID), row.FromRegionID, row.ToRegionID))
	kernelData(c, http.StatusCreated, row)
}

// ApproveMeshChange godoc
// @Summary Approve a region change
// @Description Moves the node or device to the requested region, where it gets a new address, and records the decision with the optional note. 409 mesh_conflict when the change is no longer pending or the member has moved since (reject it then). Audited (module mesh, action mesh_change_approve).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param change_id path int true "region change id"
// @Param request body meshDecisionRequest false "note"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v4/kernel/mesh/changes/{change_id}/approve [post]
func (h *MeshHandler) ApproveMeshChange(c *gin.Context) {
	h.decideMeshChange(c, true)
}

// RejectMeshChange godoc
// @Summary Reject a region change
// @Description Records the rejection with the optional note; nothing moves. Audited (module mesh, action mesh_change_reject).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param change_id path int true "region change id"
// @Param request body meshDecisionRequest false "note"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v4/kernel/mesh/changes/{change_id}/reject [post]
func (h *MeshHandler) RejectMeshChange(c *gin.Context) {
	h.decideMeshChange(c, false)
}

func (h *MeshHandler) decideMeshChange(c *gin.Context, approve bool) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "change_id")
	if !ok {
		return
	}
	var request meshDecisionRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			kernelError(c, http.StatusBadRequest, "invalid_request", "note is expected")
			return
		}
	}
	decide, action := meshregion.RejectChange, "mesh_change_reject"
	if approve {
		decide, action = meshregion.ApproveChange, "mesh_change_approve"
	}
	row, err := decide(c.Request.Context(), db, id, request.Note, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, action, row.ID, fmt.Sprintf(`{"node":%q,"device_id":%d,"to_region_id":%d,"address":%q}`, meshText(row.NodeRef), meshID(row.DeviceID), row.ToRegionID, row.NewAddress))
	kernelData(c, http.StatusOK, row)
}

func meshText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func meshID(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}
