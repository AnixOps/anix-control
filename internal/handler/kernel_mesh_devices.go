package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
)

// Mesh devices without an Agent (internal/meshregion/device.go): the
// administrator's enrollment credentials, devices and region change
// requests under /api/v4/kernel/mesh, and the devices' own API under
// /api/v5/mesh/device (docs/design/mesh-regions/agent-contract.md,
// section 4).

func (h *MeshHandler) actor(c *gin.Context) *uint {
	if id := kernelActorID(c); id != 0 {
		return &id
	}
	return nil
}

// ListEnrollments godoc
// @Summary The mesh enrollment credentials
// @Description The newest 200 one-time credentials: region_id (0 = the device's lowest latency chooses), kind (node or personal), name, expires_at, used_at and the device_id that used it. Tokens are never answered. 404 feature_disabled while the mesh is not available in this build.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.MeshEnrollment
// @Router /api/v4/kernel/mesh/enrollments [get]
func (h *MeshHandler) ListEnrollments(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	rows, err := meshregion.ListEnrollments(c.Request.Context(), db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// CreateEnrollment godoc
// @Summary Issue a mesh enrollment credential
// @Description Issues a one-time credential for a device without an Agent: kind node (a full node with a fixed address from the server segment) or personal (an access device with an address from the device pool), name (1 to 64 characters), region_id (0 lets the device's reported latencies choose among the enabled regions) and hours (1 to 168, 24 when 0). The token (`mse_` and 26 characters) is answered only here; only its SHA-256 is stored. Audited (module mesh, action mesh_enrollment_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.EnrollmentInput true "region_id, kind, name, hours"
// @Success 201 {object} meshregion.IssuedEnrollment
// @Router /api/v4/kernel/mesh/enrollments [post]
func (h *MeshHandler) CreateEnrollment(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.EnrollmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "region_id, kind, name and hours are expected")
		return
	}
	row, err := meshregion.CreateEnrollment(c.Request.Context(), db, input, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_enrollment_create", row.ID, fmt.Sprintf(`{"region_id":%d,"kind":%q,"name":%q}`, row.RegionID, row.Kind, row.Name))
	kernelData(c, http.StatusCreated, row)
}

// DeleteEnrollment godoc
// @Summary Withdraw a mesh enrollment credential
// @Description Deletes the credential; a device that already used it stays enrolled. Audited (module mesh, action mesh_enrollment_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "credential id"
// @Success 204
// @Router /api/v4/kernel/mesh/enrollments/{id} [delete]
func (h *MeshHandler) DeleteEnrollment(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := meshregion.DeleteEnrollment(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_enrollment_delete", id, fmt.Sprintf(`{"enrollment_id":%d}`, id))
	c.Status(http.StatusNoContent)
}

type meshDeviceRequest struct {
	RegionID uint `json:"region_id"`
}

// MoveDevice godoc
// @Summary Move a mesh device to another region
// @Description The device gets an address in region_id (from the server segment for a node, from the device pool for a personal device) and reads its new network on its next configuration read. 409 segment_full when the region has no free address. Audited (module mesh, action mesh_device_move).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "device id"
// @Param request body meshDeviceRequest true "region_id"
// @Success 200 {object} model.MeshDevice
// @Router /api/v4/kernel/mesh/devices/{id} [put]
func (h *MeshHandler) MoveDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var request meshDeviceRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.RegionID == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "region_id is expected")
		return
	}
	row, err := meshregion.MoveDevice(c.Request.Context(), db, id, request.RegionID, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_move", row.ID, fmt.Sprintf(`{"region_id":%d,"address":%q}`, row.RegionID, row.Address))
	kernelData(c, http.StatusOK, row)
}

// RevokeDevice godoc
// @Summary Revoke a mesh device
// @Description The device's token is refused from now on and its address is released. Enrolling again needs a new credential. Audited (module mesh, action mesh_device_revoke).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "device id"
// @Success 204
// @Router /api/v4/kernel/mesh/devices/{id} [delete]
func (h *MeshHandler) RevokeDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := meshregion.RevokeDevice(c.Request.Context(), db, id, h.now()); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_revoke", id, fmt.Sprintf(`{"device_id":%d}`, id))
	c.Status(http.StatusNoContent)
}

// ListRegionChanges godoc
// @Summary The mesh region change requests
// @Description The newest 200 requests of devices to move to another region: device_id, from_region_id, to_region_id, reason, state (pending, approved or rejected), decided_by and decided_at.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.MeshRegionChange
// @Router /api/v4/kernel/mesh/region-changes [get]
func (h *MeshHandler) ListRegionChanges(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	rows, err := meshregion.ListRegionChanges(c.Request.Context(), db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// ApproveRegionChange godoc
// @Summary Approve a mesh region change request
// @Description The device moves to the requested region and gets an address there. 404 when the request is not pending. Audited (module mesh, action mesh_region_change_approve).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "request id"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v4/kernel/mesh/region-changes/{id}/approve [post]
func (h *MeshHandler) ApproveRegionChange(c *gin.Context) { h.decide(c, true) }

// RejectRegionChange godoc
// @Summary Reject a mesh region change request
// @Description The device stays in its region. 404 when the request is not pending. Audited (module mesh, action mesh_region_change_reject).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "request id"
// @Success 200 {object} model.MeshRegionChange
// @Router /api/v4/kernel/mesh/region-changes/{id}/reject [post]
func (h *MeshHandler) RejectRegionChange(c *gin.Context) { h.decide(c, false) }

func (h *MeshHandler) decide(c *gin.Context, approve bool) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	row, err := meshregion.DecideRegionChange(c.Request.Context(), db, id, approve, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	action := "mesh_region_change_reject"
	if approve {
		action = "mesh_region_change_approve"
	}
	h.audit(c, db, action, row.ID,
		fmt.Sprintf(`{"device_id":%d,"from_region_id":%d,"to_region_id":%d}`, row.DeviceID, row.FromRegionID, row.ToRegionID))
	kernelData(c, http.StatusOK, row)
}

// device authenticates the request's device token.
func (h *MeshHandler) device(c *gin.Context) (model.MeshDevice, bool) {
	db, ok := h.database(c)
	if !ok {
		return model.MeshDevice{}, false
	}
	token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	device, err := meshregion.Authenticate(c.Request.Context(), db, token, h.now())
	if err != nil {
		meshDeviceError(c, err)
		return device, false
	}
	return device, true
}

func meshDeviceError(c *gin.Context, err error) {
	if errors.Is(err, meshregion.ErrUnauthorized) {
		kernelError(c, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	meshError(c, err)
}

// EnrollDevice godoc
// @Summary Enroll a mesh device
// @Description A device without an Agent exchanges a one-time credential (token) for its device token, sent as `Authorization: Bearer <device_token>` from then on, and its networks (config, the shape of the Agent's member mesh). device_key (8 to 128 letters, digits or `._:-`) is the identity the device keeps: enrolling the same key again keeps the device and, in the same region, its address. latencies (milliseconds by region code) choose the region when the credential names none. 401 for an unknown, used or expired credential. Rate limited; 404 feature_disabled while the mesh is not available in this build.
// @Tags Mesh
// @Accept json
// @Produce json
// @Param request body meshregion.EnrollRequest true "token, device_key, name, latencies"
// @Success 201 {object} meshregion.Enrolled
// @Router /api/v5/mesh/device/enroll [post]
func (h *MeshHandler) EnrollDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var request meshregion.EnrollRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "token and device_key are expected")
		return
	}
	enrolled, err := meshregion.Enroll(c.Request.Context(), db, request, h.now())
	if err != nil {
		meshDeviceError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, enrolled)
}

// DeviceConfig godoc
// @Summary A mesh device's networks
// @Description The device's networks in the shape of the Agent's member mesh: its region's network while the region is enabled (role full for a node, access for a personal device; the region's boundary nodes as peers), none otherwise. A device reads it every minute. 401 for an unknown or revoked device token.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object
// @Router /api/v5/mesh/device/config [get]
func (h *MeshHandler) DeviceConfig(c *gin.Context) {
	device, ok := h.device(c)
	if !ok {
		return
	}
	config, err := meshregion.DeviceConfig(c.Request.Context(), h.db().WithContext(c.Request.Context()), device, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, config)
}

// DeviceReport godoc
// @Summary Report a mesh device's status
// @Description The read-only status of the device, the payload of the Agent's mesh.status report (a JSON object with a networks array, at most the package report size). The administrator sees the latest one. 401 for an unknown or revoked device token.
// @Tags Mesh
// @Accept json
// @Security BearerAuth
// @Param request body object true "mesh.status payload"
// @Success 204
// @Router /api/v5/mesh/device/report [post]
func (h *MeshHandler) DeviceReport(c *gin.Context) {
	device, ok := h.device(c)
	if !ok {
		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, agentcontrol.MaxPackageReportPayloadBytes))
	if err != nil {
		kernelError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "the report is too large")
		return
	}
	now := h.now()
	if err := meshregion.RecordStatus(c.Request.Context(), h.db().WithContext(c.Request.Context()), meshregion.DeviceRef(device.ID), payload, now, now); err != nil {
		meshError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type meshRegionChangeRequest struct {
	// Region is the code of the region to move to.
	Region string `json:"region"`
	Reason string `json:"reason"`
}

// DeviceRegionChange godoc
// @Summary Ask to move a mesh device to another region
// @Description Records the device's request to move to the region of code region, with an optional reason (at most 200 characters), for an administrator to approve or reject. 409 mesh_conflict when the device is in that region already or has a pending request. 401 for an unknown or revoked device token.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshRegionChangeRequest true "region, reason"
// @Success 201 {object} model.MeshRegionChange
// @Router /api/v5/mesh/device/region-change [post]
func (h *MeshHandler) DeviceRegionChange(c *gin.Context) {
	device, ok := h.device(c)
	if !ok {
		return
	}
	var request meshRegionChangeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "region and reason are expected")
		return
	}
	row, err := meshregion.RequestRegionChange(c.Request.Context(), h.db().WithContext(c.Request.Context()), device, request.Region, request.Reason, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, row)
}
