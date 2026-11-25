package handler

import (
	"encoding/json"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/meshregion"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
)

// The mesh's external network bridges (internal/meshregion, R36): nodes
// that advertise chosen addresses of another overlay network in their
// home region to the members an administrator allows. Every route answers
// 404 feature_disabled while the gated feature "mesh" is off, and every
// change is audited (module mesh).

// bridgeAudit is the audit record of a bridge change.
func bridgeAudit(row model.MeshExternalBridge) string {
	content, _ := json.Marshal(map[string]any{"node": row.NodeRef, "interface": row.Interface, "routes": row.Routes, "allow": row.Allow})
	return string(content)
}

// ListMeshBridges godoc
// @Summary The mesh's external network bridges
// @Description Every external network bridge: its node and home region, the external interface, the routes it advertises, the allow list with what each entry expands to in the region, the list the node is given (expanded) and the one it last reported applied (applied, null before a report), and the addresses and segments held back until the node confirms it dropped them (held). Warnings: bridge_unconfirmed (the applied list differs from the expanded one, or addresses are held), bridge_unsupported (the node's kernel does not list mesh_external_bridge), allow_outside_region (an entry is not in the bridge's region).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {array} meshregion.BridgeView
// @Router /api/v4/kernel/mesh/bridges [get]
func (h *MeshHandler) ListMeshBridges(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	bridges, err := meshregion.Bridges(c.Request.Context(), db)
	if err != nil {
		meshError(c, err)
		return
	}
	kernelData(c, http.StatusOK, bridges)
}

// CreateMeshBridge godoc
// @Summary Make a node an external network bridge
// @Description node (a node in the mesh, not yet a bridge; it bridges in its home region), interface (its network interface on the external network), routes (1 to 64 external addresses, IPv4 /32 or IPv6 /128, which may lie in 100.64.0.0/10 or fd7a:115c:a1e0::/48, and subnets inside the private ranges outside those two; never ranges of the mesh) and allow (at most 256 sources in the node's home region: node:<node>, device:<id> or host_group:<id>). 409 mesh_conflict when a route overlaps a region, the host group supernet or a full node's subnets. Audited (module mesh, action mesh_bridge_create).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body meshregion.BridgeInput true "node, interface, routes, allow"
// @Success 201 {object} model.MeshExternalBridge
// @Router /api/v4/kernel/mesh/bridges [post]
func (h *MeshHandler) CreateMeshBridge(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	var input meshregion.BridgeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "node, interface, routes and allow are expected")
		return
	}
	row, err := meshregion.CreateBridge(c.Request.Context(), db, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_bridge_create", row.ID, bridgeAudit(row))
	kernelData(c, http.StatusCreated, row)
}

// UpdateMeshBridge godoc
// @Summary Change an external network bridge
// @Description Changes the interface, routes and allow list; the node never changes. An address or segment that leaves the list is not given to another member until the node reports a list without it applied. Audited (module mesh, action mesh_bridge_update).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param bridge_id path int true "bridge id"
// @Param request body meshregion.BridgeInput true "interface, routes, allow"
// @Success 200 {object} model.MeshExternalBridge
// @Router /api/v4/kernel/mesh/bridges/{bridge_id} [put]
func (h *MeshHandler) UpdateMeshBridge(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "bridge_id")
	if !ok {
		return
	}
	var input meshregion.BridgeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "interface, routes and allow are expected")
		return
	}
	row, err := meshregion.UpdateBridge(c.Request.Context(), db, id, input, h.now())
	if err != nil {
		meshError(c, err)
		return
	}
	h.audit(c, db, "mesh_bridge_update", row.ID, bridgeAudit(row))
	kernelData(c, http.StatusOK, row)
}

// DeleteMeshBridge godoc
// @Summary Remove an external network bridge
// @Description The node becomes an ordinary member. The addresses of its allow list stay held until it reports the bridge gone. Audited (module mesh, action mesh_bridge_delete).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param bridge_id path int true "bridge id"
// @Success 204
// @Router /api/v4/kernel/mesh/bridges/{bridge_id} [delete]
func (h *MeshHandler) DeleteMeshBridge(c *gin.Context) {
	db, ok := h.database(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "bridge_id")
	if !ok {
		return
	}
	if err := meshregion.DeleteBridge(c.Request.Context(), db, id); err != nil {
		meshError(c, err)
		return
	}
	content, _ := json.Marshal(map[string]any{"bridge_id": id})
	h.audit(c, db, "mesh_bridge_delete", id, string(content))
	c.Status(http.StatusNoContent)
}
