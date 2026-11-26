package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
)

// The mesh's custom paths (internal/meshregion): the path of one source
// member of a region to a peer or a subnet through waypoint members. Every
// route answers 404 feature_disabled while the gated feature "mesh" is off,
// and every change is audited (module mesh).

// pathAudit is the audit record of a path change.
func pathAudit(row model.MeshPath) string {
	content, _ := json.Marshal(map[string]any{"source": row.SourceRef, "target": row.Target, "via": row.Via})
	return string(content)
}

// ListMeshPaths godoc
// @Summary The mesh's custom paths
// @Description Every custom path: its source member (node:<node> or device:<id>) with name and region, the target (a member entry or a subnet) with its name, the waypoints in order with name and current address, and the state the source last reported for it (status: state, reason, unreachable as reported; null without a report). Warnings: path_member_missing (the source, the target or a waypoint does not resolve to an address in the source's region, so the path is not delivered), path_unsupported (the source is a device without an Agent whose last report does not list mesh_paths).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} meshregion.PathView
// @Router /api/v4/kernel/mesh/paths [get]
func (h *MeshHandler) ListMeshPaths(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	paths, err := meshregion.Paths(c.Request.Context(), db)
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, paths)
}

// CreateMeshPath godoc
// @Summary Add a custom path
// @Description source (node:<node> of a node in the mesh, or device:<id> of a full node device; its region is the source's home region), target (a member of that region, node:<node> or device:<id>, or a private IPv4 subnet in CIDR form outside the region's range and not a part of a host group's segment) and via (1 to 6 distinct full nodes of the region, never the source or the target). A source has at most one path per target: 409 mesh_conflict otherwise. Audited (module mesh, action mesh_path_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.PathInput true "source, target, via"
// @Success 201 {object} model.MeshPath
// @Router /api/v4/kernel/mesh/paths [post]
func (h *MeshHandler) CreateMeshPath(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.PathInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "source, target and via are expected")
		return
	}
	row, err := meshregion.CreatePath(c.Request.Context(), db, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_path_create", row.ID, pathAudit(row))
	kernelData(c, http.StatusCreated, row)
}

// UpdateMeshPath godoc
// @Summary Change a custom path
// @Description Changes the target and the waypoints; the source never changes (400 when source differs). Audited (module mesh, action mesh_path_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param path_id path int true "path id"
// @Param request body meshregion.PathInput true "source (unchanged, may be left out), target, via"
// @Success 200 {object} model.MeshPath
// @Router /api/v4/kernel/mesh/paths/{path_id} [put]
func (h *MeshHandler) UpdateMeshPath(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "path_id")
	if !ok {
		return
	}
	var input meshregion.PathInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "target and via are expected")
		return
	}
	row, err := meshregion.UpdatePath(c.Request.Context(), db, id, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_path_update", row.ID, pathAudit(row))
	kernelData(c, http.StatusOK, row)
}

// DeleteMeshPath godoc
// @Summary Remove a custom path
// @Description Audited (module mesh, action mesh_path_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param path_id path int true "path id"
// @Success 204
// @Router /api/v4/kernel/mesh/paths/{path_id} [delete]
func (h *MeshHandler) DeleteMeshPath(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "path_id")
	if !ok {
		return
	}
	if err := meshregion.DeletePath(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	content, _ := json.Marshal(map[string]any{"path_id": id})
	h.audit(c, db, "mesh_path_delete", id, string(content))
	c.Status(http.StatusNoContent)
}
