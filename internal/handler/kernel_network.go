package handler

import (
	"errors"
	"net/http"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

// NetworkModelHandler is the administrator API of the device network model
// (docs/design/network-model, internal/networkmodel): lines on nodes,
// ingresses and egresses on lines, ANX-Line transports between devices and
// paths from user ingresses to internet egresses, declared as one document
// and turned into each device's plan. Every write is in the operation log
// (module network).
type NetworkModelHandler struct {
	db func() *gorm.DB
}

// NewNetworkModelHandler reads the database on each request.
func NewNetworkModelHandler() *NetworkModelHandler {
	return &NetworkModelHandler{db: database.Get}
}

type networkModelRequest struct {
	// Revision, when set, must be the stored revision (409 otherwise).
	Revision *uint64               `json:"revision"`
	Document networkmodel.Document `json:"document"`
}

func (h *NetworkModelHandler) modelError(c *gin.Context, err error) {
	var invalid *networkmodel.Invalid
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": invalid.Error(), "violations": invalid.Violations}})
	case errors.Is(err, networkmodel.ErrConflict):
		kernelError(c, http.StatusConflict, "revision_conflict", err.Error())
	case errors.Is(err, networkmodel.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, networkmodel.ErrUnknownNode):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, networkmodel.ErrNoKEK):
		kernelError(c, http.StatusConflict, "kek_required", err.Error())
	default:
		kernelDBError(c, err)
	}
}

func (h *NetworkModelHandler) actor(c *gin.Context) networkmodel.Actor {
	return networkmodel.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

// Get godoc
// @Summary The device network model
// @Description The network model document (lines on nodes; user and hop ingresses and internet and hop egresses on lines; ANX-Line transports from a hop egress to another device's hop ingress; paths from a user ingress through transports to an internet egress) with its revision, the paths not planned yet and why (pending: a transport's accepting device has not reported its ANX-Line server key), and each device's plan generation and hash, whether its Agent negotiated network.v1, and which public keys it reported, and the devices a route may use (nodes: every enabled node whose Agent connected, by node reference and name).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/network [get]
func (h *NetworkModelHandler) Get(c *gin.Context) {
	view, err := networkmodel.New(h.db()).Get(c.Request.Context())
	if err != nil {
		h.modelError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// Replace godoc
// @Summary Replace the device network model
// @Description Makes the document the network model. With revision, the stored revision must match (409 revision_conflict). Every device's plan must pass the planner (sdk/network.Check and the cross-device rules: segments chain from device to device, a transport joins two devices and carries one path); otherwise 400 with violations. A new anx_line transport gets a link key sealed under module_runtime.ca_kek (409 kek_required without it); a removed one loses its key. Each device whose plan changed gets a new generation. Answers as GET does. Audited (network_model_replace).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body networkModelRequest true "the document"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/network [put]
func (h *NetworkModelHandler) Replace(c *gin.Context) {
	var request networkModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := networkmodel.New(h.db()).Replace(c.Request.Context(), &request.Document, request.Revision, h.actor(c))
	if err != nil {
		h.modelError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// RotateLinkKey godoc
// @Summary Rotate an ANX-Line transport's link key
// @Description Gives the transport a new random link key and replans its two devices. Answers as GET does. Audited (network_link_key_rotate).
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "transport id"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/network/transports/{id}/rotate-key [post]
func (h *NetworkModelHandler) RotateLinkKey(c *gin.Context) {
	view, err := networkmodel.New(h.db()).RotateLinkKey(c.Request.Context(), c.Param("id"), h.actor(c))
	if err != nil {
		h.modelError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// DevicePlan godoc
// @Summary A device's network plan
// @Description The node's current NetworkPlan (anixops.network.v1, protojson with proto field names) with its generation and hash, link keys redacted. 404 when the model gives the node nothing.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id> or forward-<id>"
// @Success 200 {object} map[string]any
// @Router /api/v4/kernel/network/devices/{id}/plan [get]
func (h *NetworkModelHandler) DevicePlan(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	plan, found, err := networkmodel.New(h.db()).DevicePlan(c.Request.Context(), node.String())
	if err != nil {
		h.modelError(c, err)
		return
	}
	if !found {
		kernelError(c, http.StatusNotFound, "not_found", "the network model gives this node no plan")
		return
	}
	encoded, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(networkmodel.Redacted(plan))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", append(append([]byte(`{"data":`), encoded...), '}'))
}

// NodeNetwork godoc
// @Summary A device's network settings
// @Description The interface the network model's automatic egresses of the node leave by (interface; source_v4 and gateway_v4, source_v6 and gateway_v6 on it; empty: the default route) and the port its automatic hop ingress listens on (hop_port; 0: 24443). A node behind NAT is reached at the NAT entry (GET /api/v4/kernel/nodes/{id}/entries) whose port map covers hop_port. detected lists the interfaces the node's Agent last reported (name, addresses_v4, addresses_v6, gateway_v4, gateway_v6; empty before an Agent that reports them), detected_at when. 404 for an unknown node.
// @Tags Kernel
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id> or forward-<id>"
// @Success 200 {object} networkmodel.NodeNetworkView
// @Router /api/v4/kernel/nodes/{id}/network [get]
func (h *NetworkModelHandler) NodeNetwork(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	view, err := networkmodel.New(h.db()).NodeNetwork(c.Request.Context(), node.String())
	if err != nil {
		h.settingsError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

// SetNodeNetwork godoc
// @Summary Set a device's network settings
// @Description Replaces the node's network settings (see GET) and replans: the automatic lines, egresses, hop ingresses and transports of the routes through the node follow them. A source address needs the interface, a gateway its source address; 400 with violations otherwise, 404 for an unknown node. Answers like GET. Audited (network_node_settings).
// @Tags Kernel
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "proxy-<id> or forward-<id>"
// @Param request body networkmodel.NodeSettings true "the settings"
// @Success 200 {object} networkmodel.NodeNetworkView
// @Router /api/v4/kernel/nodes/{id}/network [put]
func (h *NetworkModelHandler) SetNodeNetwork(c *gin.Context) {
	node, ok := parseNodeRef(c)
	if !ok {
		return
	}
	var request networkmodel.NodeSettings
	if err := c.ShouldBindJSON(&request); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	service := networkmodel.New(h.db())
	if _, err := service.SetNodeSettings(c.Request.Context(), node.String(), request, h.actor(c)); err != nil {
		h.settingsError(c, err)
		return
	}
	view, err := service.NodeNetwork(c.Request.Context(), node.String())
	if err != nil {
		h.settingsError(c, err)
		return
	}
	kernelData(c, http.StatusOK, view)
}

func (h *NetworkModelHandler) settingsError(c *gin.Context, err error) {
	if errors.Is(err, networkmodel.ErrUnknownNode) {
		kernelError(c, http.StatusNotFound, "not_found", err.Error())
		return
	}
	h.modelError(c, err)
}
