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

// Mesh devices without an Agent (internal/meshregion/enroll.go): the
// administrator's one-time enrollment credentials and device revocation
// under /api/v4/kernel/mesh, and the devices' own API under
// /api/v5/mesh/device (docs/design/mesh-regions/agent-contract.md,
// section 4).

// ListMeshEnrollments godoc
// @Summary The mesh enrollment credentials
// @Description The newest 200 one-time credentials: region_id (0 = the device's lowest latency chooses), hostname, role (access or full), a full node's fixed address (empty: the lowest free one when it enrolls), expires_at, used_at and the device_id that used it. Tokens are never answered. 404 feature_disabled while the mesh is not available in this build.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} meshregion.EnrollmentView
// @Router /api/v4/kernel/mesh/enrollments [get]
func (h *MeshHandler) ListMeshEnrollments(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	rows, err := meshregion.Enrollments(c.Request.Context(), db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// CreateMeshEnrollment godoc
// @Summary Issue a mesh enrollment credential
// @Description Issues a one-time credential for a device without an Agent: hostname (one DNS label, the device's name), region_id (0 lets the device's reported latencies choose among the enabled regions) and hours (1 to 168, 24 when 0). role access (default) enrolls a personal device with a pool address; role full enrolls a full node without an Agent (a router or a server), which needs region_id and gets a fixed address of the region's server segment: address, kept for the credential until it is used or withdrawn, or the lowest free one when empty (409 mesh_conflict when taken). A full node listens and advertises only the subnets an administrator confirms. With device_id the credential enrolls that existing device again (its name, region, role and address stay; hostname and region_id are ignored, role and address are refused); a device key another device has is refused otherwise. The token (`mse_` and 26 characters) is answered only here; only its SHA-256 is stored. Audited (module mesh, action mesh_enrollment_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.EnrollmentInput true "region_id, hostname, hours, role, address"
// @Success 201 {object} meshregion.IssuedEnrollment
// @Router /api/v4/kernel/mesh/enrollments [post]
func (h *MeshHandler) CreateMeshEnrollment(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.EnrollmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "region_id, hostname, hours, role and address are expected")
		return
	}
	row, err := meshregion.CreateEnrollment(c.Request.Context(), db, input, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_enrollment_create", row.ID, fmt.Sprintf(`{"region_id":%d,"hostname":%q,"role":%q,"address":%q}`, row.RegionID, row.Hostname, row.Role, row.Address))
	kernelData(c, http.StatusCreated, row)
}

// DeleteMeshEnrollment godoc
// @Summary Withdraw a mesh enrollment credential
// @Description Deletes an unused credential; 409 mesh_conflict for a used one, which stays as the record of the device it enrolled. Audited (module mesh, action mesh_enrollment_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param enrollment_id path int true "credential id"
// @Success 204
// @Router /api/v4/kernel/mesh/enrollments/{enrollment_id} [delete]
func (h *MeshHandler) DeleteMeshEnrollment(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "enrollment_id")
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

// RevokeMeshDevice godoc
// @Summary Revoke an enrolled device's token
// @Description The device's token is refused from now on and its static key leaves its region's native_keys; the device and its address stay until removed. Enrolling again needs a new credential. 404 when the device holds no token. Audited (module mesh, action mesh_device_revoke).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param device_id path int true "device id"
// @Success 204
// @Router /api/v4/kernel/mesh/devices/{device_id}/revoke [post]
func (h *MeshHandler) RevokeMeshDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "device_id")
	if !ok {
		return
	}
	if err := meshregion.RevokeDevice(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_device_revoke", id, fmt.Sprintf(`{"device_id":%d}`, id))
	c.Status(http.StatusNoContent)
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

// meshDeviceRequestBytes bounds the body of an enrollment and of a region
// change request.
const meshDeviceRequestBytes = 16 << 10

func tooLarge(err error) bool {
	var maxBytes *http.MaxBytesError
	return errors.As(err, &maxBytes)
}

func meshDeviceError(c *gin.Context, err error) {
	if errors.Is(err, meshregion.ErrUnauthorized) {
		kernelError(c, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	meshError(c, err)
}

// EnrollMeshDevice godoc
// @Summary Enroll a mesh device
// @Description A device without an Agent exchanges a one-time credential (token) for its device token, sent as `Authorization: Bearer <device_token>` from then on, and its networks (config, the shape of the Agent's member mesh). device_key (16 to 128 base64 characters) is the identity the device keeps. A key another device has is refused (409) unless the credential was issued for that device, which then keeps its region and address. Within 10 minutes of a use the same credential and key enroll again with a new token, for an answer that was lost. latencies (milliseconds by region code, at most 64) choose the region when the credential names none. static_key, when the device has one already, is its static public key in the region's network (32 bytes in standard base64 with padding), which the region's native_keys list; without it the device's first report names it, and it changes only with another enrollment. The body is at most 16 KiB (413). 401 for an unknown, used or expired credential. Rate limited; 404 feature_disabled while the mesh is not available in this build.
// @Tags Mesh
// @Accept json
// @Produce json
// @Param request body meshregion.EnrollRequest true "token, device_key, latencies, static_key"
// @Success 201 {object} meshregion.Enrolled
// @Router /api/v5/mesh/device/enroll [post]
func (h *MeshHandler) EnrollMeshDevice(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var request meshregion.EnrollRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, meshDeviceRequestBytes)
	if err := c.ShouldBindJSON(&request); err != nil {
		if tooLarge(err) {
			kernelError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "the request is too large")
			return
		}
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

// maxFeaturesQuery bounds the features a device names on a configuration
// read.
const maxFeaturesQuery = 4 << 10

// MeshDeviceConfig godoc
// @Summary A mesh device's networks
// @Description The device's networks in the shape of the Agent's member mesh: its region's network, kind managed with the static keys of its members and devices as native_keys, as an access member (its pool address, no listeners, the region's boundary nodes as peers) while the region is enabled, none otherwise. The networks follow the capabilities of the device's kernel, from features (comma separated) or else its latest report's features: without mesh_native_keys no network is answered, and dns.resolver_address only with mesh_resolver_address. Every read uses the device's address. A device reads it every minute. 401 for an unknown or revoked device token.
// @Tags Mesh
// @Produce json
// @Security BearerAuth
// @Param features query string false "the kernel's capabilities, comma separated"
// @Success 200 {object} object
// @Router /api/v5/mesh/device/config [get]
func (h *MeshHandler) MeshDeviceConfig(c *gin.Context) {
	device, ok := h.device(c)
	if !ok {
		return
	}
	var features []string
	if raw, given := c.GetQuery("features"); given {
		if len(raw) > maxFeaturesQuery || strings.Count(raw, ",") >= meshregion.MaxFeatures {
			kernelError(c, http.StatusBadRequest, "invalid_request", "features: at most 64 names in 4 KiB")
			return
		}
		features = []string{}
		for _, name := range strings.Split(raw, ",") {
			if name = strings.TrimSpace(name); name != "" {
				features = append(features, name)
			}
		}
	}
	config, err := meshregion.DeviceConfig(c.Request.Context(), h.db(), device, features, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, config)
}

// MeshDeviceReport godoc
// @Summary Report a mesh device's status
// @Description The read-only status of the device, the payload of the Agent's mesh.status report (a JSON object with a networks array, at most the package report size). Answered 204 with no instructions; the administrator sees the latest report. A device without a static key gets the one it reports for its region's network (networks[].static_key of the network whose id is the region code); later reports cannot change it. 401 for an unknown or revoked device token.
// @Tags Mesh
// @Accept json
// @Security BearerAuth
// @Param request body object true "mesh.status payload"
// @Success 204
// @Router /api/v5/mesh/device/report [post]
func (h *MeshHandler) MeshDeviceReport(c *gin.Context) {
	device, ok := h.device(c)
	if !ok {
		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, agentcontrol.MaxPackageReportPayloadBytes))
	if err != nil {
		kernelError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "the report is too large")
		return
	}
	token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if err := meshregion.RecordDeviceStatus(c.Request.Context(), h.db(), device, token, payload, h.now()); err != nil {
		meshError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type meshDeviceChangeRequest struct {
	// Region is the code of the region to move to.
	Region string `json:"region"`
	Reason string `json:"reason"`
}

// MeshDeviceChange godoc
// @Summary Ask to move a mesh device to another region
// @Description Records the device's request to move to the region of code region, with an optional reason (at most 500 characters), for an administrator to approve or reject on the region changes. 409 mesh_conflict when a request is already pending. 401 for an unknown or revoked device token.
// @Tags Mesh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshDeviceChangeRequest true "region, reason"
// @Success 201 {object} model.MeshRegionChange
// @Router /api/v5/mesh/device/region-change [post]
func (h *MeshHandler) MeshDeviceChange(c *gin.Context) {
	device, ok := h.device(c)
	if !ok {
		return
	}
	var request meshDeviceChangeRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, meshDeviceRequestBytes)
	if err := c.ShouldBindJSON(&request); err != nil {
		if tooLarge(err) {
			kernelError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "the request is too large")
			return
		}
		kernelError(c, http.StatusBadRequest, "invalid_request", "region and reason are expected")
		return
	}
	row, err := meshregion.RequestDeviceChange(c.Request.Context(), h.db(), device, request.Region, request.Reason, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusCreated, row)
}

type meshDeviceSubnetRequest struct {
	// CIDRs are the subnets to advertise; empty withdraws the request.
	CIDRs []string `json:"cidrs"`
}

// MeshDeviceSubnetRequest godoc
// @Summary Ask to advertise subnets from a mesh full node
// @Description Records the subnets a full node without an Agent asks to advertise (cidrs: private IPv4 networks with no host bits, at most 16) for an administrator to approve or reject on the device. A new request replaces the pending one; an empty cidrs withdraws it. Approval makes them the node's confirmed subnets, which it reads with its next configuration; rejection keeps the confirmed subnets. 202 with no body. 400 invalid_request for an invalid list, 409 mesh_conflict for an access device, 401 for an unknown or revoked device token.
// @Tags Mesh
// @Accept json
// @Security BearerAuth
// @Param request body meshDeviceSubnetRequest true "cidrs"
// @Success 202
// @Router /api/v5/mesh/device/subnet-request [post]
func (h *MeshHandler) MeshDeviceSubnetRequest(c *gin.Context) {
	device, ok := h.device(c)
	if !ok {
		return
	}
	var request meshDeviceSubnetRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, meshDeviceRequestBytes)
	if err := c.ShouldBindJSON(&request); err != nil {
		if tooLarge(err) {
			kernelError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "the request is too large")
			return
		}
		kernelError(c, http.StatusBadRequest, "invalid_request", "cidrs is expected")
		return
	}
	if request.CIDRs == nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "cidrs is expected")
		return
	}
	if err := meshregion.RequestDeviceSubnets(c.Request.Context(), h.db(), device, request.CIDRs, h.now()); err != nil {
		meshError(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}
