package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MeshHandler serves the intranet mesh regions and memberships
// (internal/meshregion). Every route answers 404 feature_disabled while the
// gated feature "mesh" is off.
type MeshHandler struct {
	db  func() *gorm.DB
	now func() time.Time
}

// NewMeshHandler reads the loaded database on each request.
func NewMeshHandler() *MeshHandler {
	return &MeshHandler{db: database.Get, now: time.Now}
}

type meshMemberRequest struct {
	// Node is proxy-<id> or forward-<id>; only when adding a node.
	Node     string `json:"node"`
	Boundary bool   `json:"boundary"`
	// RegionID moves the node to another region; 0 keeps it.
	RegionID uint `json:"region_id"`
}

// database answers the request's database, after answering an error itself.
func (h *MeshHandler) database(c *gin.Context) (*gorm.DB, bool) {
	if !features.Enabled(meshregion.Feature) {
		kernelError(c, http.StatusNotFound, "feature_disabled", "the mesh is not available in this build")
		return nil, false
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return nil, false
	}
	return db.WithContext(c.Request.Context()), true
}

func pathID(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil || id == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", name+" must be a positive integer")
		return 0, false
	}
	return uint(id), true
}

// audit records action in the operation log after the change; never a
// network key. A failed record is logged: the change stands.
func (h *MeshHandler) audit(c *gin.Context, db *gorm.DB, action string, targetID uint, content string) {
	var userID *uint
	if id := kernelActorID(c); id != 0 {
		userID = &id
	}
	if err := db.Create(&model.OperationLog{
		UserID: userID, Username: truncateText(c.GetString("email"), 100), Action: action, Module: "mesh",
		TargetType: "mesh", TargetID: &targetID, Content: content, IP: truncateText(c.ClientIP(), 45), Status: 1,
	}).Error; err != nil {
		slog.Warn("mesh: the operation log record failed", "action", action, "error", err)
	}
}

func meshError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, meshregion.ErrInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, meshregion.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, meshregion.ErrConflict):
		kernelError(c, http.StatusConflict, "mesh_conflict", err.Error())
	case errors.Is(err, meshregion.ErrFull):
		kernelError(c, http.StatusConflict, "segment_full", err.Error())
	case errors.Is(err, meshregion.ErrPoolFull):
		kernelError(c, http.StatusConflict, "pool_full", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// ListRegions godoc
// @Summary The intranet mesh regions
// @Description Every region in order (code, name, cidr, enabled, sort) with its members: node, fixed address, whether it is the node's home region and whether the node is a boundary node, the node's name and host, and the node's last mesh status as its Agent reported it. Network keys are never answered. 404 feature_disabled while the mesh is not available in this build.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} meshregion.Region
// @Router /api/v4/kernel/mesh/regions [get]
func (h *MeshHandler) ListRegions(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	regions, err := meshregion.List(c.Request.Context(), db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, regions)
}

// CreateRegion godoc
// @Summary Create a mesh region
// @Description A region is a network of its own: code (2 to 16 lowercase letters, digits or dashes, the network id the nodes see), name, cidr (an IPv4 network of /8 to /24 inside 10/8, 172.16/12, 192.168/16 or 100.64/10 that overlaps no other region; its lower half holds the fixed addresses, its upper half is the device pool), enabled and sort. Control generates its network key. Every boundary node gets an address in it. 409 mesh_conflict for a code or range another region has. Audited (module mesh, action mesh_region_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.RegionInput true "code, name, cidr, enabled, sort"
// @Success 201 {object} model.MeshRegion
// @Router /api/v4/kernel/mesh/regions [post]
func (h *MeshHandler) CreateRegion(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.RegionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "code, name, cidr, enabled and sort are expected")
		return
	}
	row, err := meshregion.CreateRegion(c.Request.Context(), db, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_region_create", row.ID, fmt.Sprintf(`{"code":%q,"cidr":%q,"enabled":%t}`, row.Code, row.CIDR, row.Enabled))
	kernelData(c, http.StatusCreated, row)
}

// UpdateRegion godoc
// @Summary Change a mesh region
// @Description Changes code, name, cidr, enabled and sort. The range of a region with members cannot change (409 mesh_conflict). The nodes of an enabled region get its network in their configuration at once; disabling it takes the network away. Audited (module mesh, action mesh_region_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "region id"
// @Param request body meshregion.RegionInput true "code, name, cidr, enabled, sort"
// @Success 200 {object} model.MeshRegion
// @Router /api/v4/kernel/mesh/regions/{id} [put]
func (h *MeshHandler) UpdateRegion(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var input meshregion.RegionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "code, name, cidr, enabled and sort are expected")
		return
	}
	row, err := meshregion.UpdateRegion(c.Request.Context(), db, id, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_region_update", row.ID, fmt.Sprintf(`{"code":%q,"cidr":%q,"enabled":%t}`, row.Code, row.CIDR, row.Enabled))
	kernelData(c, http.StatusOK, row)
}

// DeleteRegion godoc
// @Summary Delete a mesh region
// @Description Deletes a region that is no node's home region (409 mesh_conflict otherwise); the boundary nodes leave it. Audited (module mesh, action mesh_region_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "region id"
// @Success 204
// @Router /api/v4/kernel/mesh/regions/{id} [delete]
func (h *MeshHandler) DeleteRegion(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := meshregion.DeleteRegion(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_region_delete", id, fmt.Sprintf(`{"region_id":%d}`, id))
	c.Status(http.StatusNoContent)
}

// RotateRegionSecret godoc
// @Summary Give a mesh region a new network key
// @Description After a device token is revoked or a device deleted: the region's network gets a new key, pushed to every member at once and read by the devices with their next configuration, so the device's kept key no longer joins. Audited (module mesh, action mesh_region_rotate_secret).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path int true "region id"
// @Success 204
// @Router /api/v4/kernel/mesh/regions/{id}/rotate-secret [post]
func (h *MeshHandler) RotateRegionSecret(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if err := meshregion.RotateRegionSecret(c.Request.Context(), db, id, h.now()); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_region_rotate_secret", id, fmt.Sprintf(`{"region_id":%d}`, id))
	c.Status(http.StatusNoContent)
}

// AddMember godoc
// @Summary Add a node to a mesh region
// @Description Makes the region the node's home region with the lowest free fixed address of its server segment (the lower half of its range). A boundary node (boundary true) also gets an address in every other region. 409 mesh_conflict when the node is already in a region, segment_full when the segment has no free address. Audited (module mesh, action mesh_member_add).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "region id"
// @Param request body meshMemberRequest true "node and boundary"
// @Success 201 {object} model.MeshMember
// @Router /api/v4/kernel/mesh/regions/{id}/members [post]
func (h *MeshHandler) AddMember(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	regionID, ok := pathID(c, "id")
	if !ok {
		return
	}
	var request meshMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node and boundary are expected")
		return
	}
	node, err := agentcontrol.ParseAgentNode(request.Node)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node must be proxy-<id> or forward-<id>")
		return
	}
	row, err := meshregion.AddNode(c.Request.Context(), db, regionID, node, request.Boundary, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_member_add", row.ID, fmt.Sprintf(`{"node":%q,"region_id":%d,"address":%q,"boundary":%t}`, row.NodeRef, row.RegionID, row.Address, row.Boundary))
	kernelData(c, http.StatusCreated, row)
}

type meshAutoMemberRequest struct {
	// Node is proxy-<id> or forward-<id>.
	Node string `json:"node"`
}

// AddAutoMember godoc
// @Summary Add a node to the mesh with an automatic region
// @Description Adds an Agent node that is in no region and lets its measured latencies choose the region (appendix D.5): the node's configuration carries the region directory and no network until its mesh status reports region_probe.regions (region code and rtt_ms of each region it reached); the panel then chooses the enabled region of the lowest score (the latency, times 0.8 for the default preferred region of the mesh settings; ties by sort, then id), gives the node its fixed address of that region's server segment as AddMember does, with the basis latency, and removes the waiting entry. 409 mesh_conflict when the node is in a region or already waits. Audited (module mesh, action mesh_member_add_auto).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshAutoMemberRequest true "node"
// @Success 201 {object} model.MeshAutoNode
// @Router /api/v4/kernel/mesh/auto-members [post]
func (h *MeshHandler) AddAutoMember(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var request meshAutoMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node is expected")
		return
	}
	node, err := agentcontrol.ParseAgentNode(request.Node)
	if err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node must be proxy-<id> or forward-<id>")
		return
	}
	row, err := meshregion.AddAutoNode(c.Request.Context(), db, node, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_member_add_auto", row.ID, fmt.Sprintf(`{"node":%q}`, row.NodeRef))
	kernelData(c, http.StatusCreated, row)
}

// ListAutoMembers godoc
// @Summary The nodes waiting for an automatic region
// @Description The Agent nodes added with an automatic region that have not reported latencies of an enabled region yet, with the node's name.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} meshregion.AutoNodeView
// @Router /api/v4/kernel/mesh/auto-members [get]
func (h *MeshHandler) ListAutoMembers(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	rows, err := meshregion.AutoNodes(c.Request.Context(), db)
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, rows)
}

// RemoveAutoMember godoc
// @Summary Stop a node from waiting for an automatic region
// @Description Removes the waiting entry auto_id; the node's configuration loses the member mesh at once. 404 when it was placed or removed meanwhile. Audited (module mesh, action mesh_member_auto_remove).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param auto_id path int true "waiting entry id"
// @Success 204
// @Router /api/v4/kernel/mesh/auto-members/{auto_id} [delete]
func (h *MeshHandler) RemoveAutoMember(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "auto_id")
	if !ok {
		return
	}
	node, err := meshregion.RemoveAutoNode(c.Request.Context(), db, id)
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_member_auto_remove", id, fmt.Sprintf(`{"node":%q}`, node))
	c.Status(http.StatusNoContent)
}

// UpdateMember godoc
// @Summary Change a node's mesh membership
// @Description On the node's home membership: boundary makes it a boundary node (an address in every other region) or an ordinary member; region_id, when not 0, moves the node to that region, where it gets a new fixed address. Answers the node's home membership. 404 when member_id is not a home membership. Audited (module mesh, action mesh_member_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param member_id path int true "home membership id"
// @Param request body meshMemberRequest true "boundary and region_id"
// @Success 200 {object} model.MeshMember
// @Router /api/v4/kernel/mesh/members/{member_id} [put]
func (h *MeshHandler) UpdateMember(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "member_id")
	if !ok {
		return
	}
	var request meshMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "boundary and region_id are expected")
		return
	}
	row, err := meshregion.UpdateMember(c.Request.Context(), db, id, request.RegionID, request.Boundary, h.actor(c), h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_member_update", row.ID, fmt.Sprintf(`{"node":%q,"region_id":%d,"address":%q,"boundary":%t}`, row.NodeRef, row.RegionID, row.Address, row.Boundary))
	kernelData(c, http.StatusOK, row)
}

// RemoveMember godoc
// @Summary Take a node out of the mesh
// @Description Removes all memberships of the node of the home membership member_id; its configuration loses the member mesh at once. Audited (module mesh, action mesh_member_remove).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param member_id path int true "home membership id"
// @Success 204
// @Router /api/v4/kernel/mesh/members/{member_id} [delete]
func (h *MeshHandler) RemoveMember(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "member_id")
	if !ok {
		return
	}
	// The custom paths that name the node go with it; the audit counts
	// them.
	paths := 0
	var members []model.MeshMember
	if err := db.WithContext(c.Request.Context()).Where("id = ?", id).Limit(1).Find(&members).Error; err != nil {
		meshError(c, err)
		return
	}
	if len(members) > 0 {
		count, err := meshregion.MemberPaths(c.Request.Context(), db, meshregion.NodeEntry(members[0].NodeRef))
		if err != nil {
			meshError(c, err)
			return
		}
		paths = count
	}
	if err := meshregion.RemoveNode(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_member_remove", id, fmt.Sprintf(`{"member_id":%d,"paths_deleted":%d}`, id, paths))
	c.Status(http.StatusNoContent)
}
