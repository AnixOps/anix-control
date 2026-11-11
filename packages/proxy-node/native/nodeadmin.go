package native

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v5/packages/proxy-node/native/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The node routes work on v2_node, which the package adopts once the node
// credential split finalized it (kernel.storage.adopt:v2_node, a grant the
// kernel honours only then): its credential columns hold tombstones and its
// raw configuration the placeholder at every secret position. A node's
// protocols come from kapi_node_protocol_public_v1, which the kernel grants
// once v2_node_protocol is finalized. Until the lease grants them, these
// routes answer from the legacy handler.
const (
	nodeTable           = "v2_node"
	nodeProtocolView    = "kapi_node_protocol_public_v1"
	kernelOperationWait = 30 * time.Second
	invalidNodeID       = "无效的节点ID"
	nodeNotFound        = "节点不存在"
)

// leased reports whether the package's lease adopts or grants every name.
func (s *Service) leased(ctx context.Context, names ...string) bool {
	if s.Leased == nil {
		return false
	}
	for _, name := range names {
		if !s.Leased(ctx, name) {
			return false
		}
	}
	return true
}

// message is the legacy handlers' c.JSON(code, gin.H{"message": text}).
func message(code int, text string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"message": text})
}

// messageError is c.JSON(code, gin.H{"message": text, "error": detail}).
func messageError(code int, text, detail string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"message": text, "error": detail})
}

// redactNode masks a node for an administrator's answer, as the kernel's
// service.RedactNode does: its raw configuration and its protocols'
// settings.
func redactNode(node *model.Node) {
	if node.RawConfig != nil {
		redacted := v2compat.RedactNodeSecrets(*node.RawConfig)
		node.RawConfig = &redacted
	}
	for i := range node.Protocols {
		protocol := &node.Protocols[i]
		for _, field := range []**string{&protocol.Settings, &protocol.TLSSettings, &protocol.TransportSettings, &protocol.RealitySettings, &protocol.CustomConfig} {
			if *field != nil {
				redacted := v2compat.RedactNodeSecrets(**field)
				*field = &redacted
			}
		}
	}
}

// NodeSortColumns are the columns the node list sorts by: the keys and the
// meaning of the kernel's service.NodeSortColumns. "sort" is the
// administrator's own order weight. A node's shown status is derived from
// its last check and its protocol count from another table, so neither is
// sortable; a node that never checked in (NULL last_check_at) sorts last
// ascending.
var NodeSortColumns = map[string]v2compat.SortColumn{
	"id":            {Expr: "id", Unique: true},
	"name":          {Expr: "name"},
	"host":          {Expr: "host"},
	"sort":          {Expr: "sort"},
	"created_at":    {Expr: "created_at"},
	"last_check_at": {Expr: "last_check_at", Nullable: true},
	"cpu_usage":     {Expr: "cpu_usage"},
	"online_users":  {Expr: "online_users"},
}

// NodeSortTiebreaker orders nodes that share a sorted value, so pages
// neither repeat nor skip a node.
const NodeSortTiebreaker = "id DESC"

// ListNodes is GET /api/v2/admin/nodes: a page of nodes by sort order (or by
// the sort and order query, the columns of NodeSortColumns), filtered by
// status, group and a search in the name and host, each with its protocols,
// masked; a node's status shows whether it checked in recently, unless it
// is disabled.
func (s *Service) ListNodes(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	if !s.leased(ctx, nodeTable, nodeProtocolView) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	page, pageSize := pagination(request)
	orderBy, err := v2compat.ParseListSort(query(request, "sort"), query(request, "order"), NodeSortColumns, NodeSortTiebreaker)
	if err != nil {
		return message(http.StatusBadRequest, err.Error())
	}
	if orderBy == "" {
		orderBy = "sort ASC, id DESC"
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	rows := db.Model(&model.Node{})
	if value, ok := getQuery(request, "status"); ok && value != "" {
		status, _ := strconv.Atoi(value)
		rows = rows.Where("status = ?", model.NodeStatus(status))
	}
	if value, ok := getQuery(request, "group_id"); ok && value != "" {
		group, _ := strconv.ParseUint(value, 10, 32)
		rows = rows.Where("group_id = ?", uint(group))
	}
	if search := query(request, "search"); search != "" {
		rows = rows.Where("name LIKE ? OR host LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := rows.Count(&total).Error; err != nil {
		return message(http.StatusInternalServerError, "获取节点列表失败")
	}
	var nodes []model.Node
	if err := rows.Preload("Protocols").Order(orderBy).Offset((page - 1) * pageSize).Limit(pageSize).Find(&nodes).Error; err != nil {
		return message(http.StatusInternalServerError, "获取节点列表失败")
	}
	now := s.now()
	for i := range nodes {
		node := &nodes[i]
		if node.Status != model.NodeStatusDisabled {
			if node.IsOnline(now) {
				node.Status = model.NodeStatusOnline
			} else if node.Status == model.NodeStatusOnline {
				node.Status = model.NodeStatusOffline
			}
		}
		redactNode(node)
	}
	if nodes == nil {
		nodes = []model.Node{}
	}
	return s.panel(map[string]any{"total": total, "list": nodes})
}

// GetNode is GET /api/v2/admin/nodes/:id: a node with its protocols,
// masked.
func (s *Service) GetNode(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return message(http.StatusBadRequest, invalidNodeID)
	}
	if !s.leased(ctx, nodeTable, nodeProtocolView) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var node model.Node
	if err := db.Preload("Protocols").First(&node, id).Error; err != nil {
		return message(http.StatusNotFound, nodeNotFound)
	}
	redactNode(&node)
	return s.panel(node)
}

// DeleteNode is DELETE /api/v2/admin/nodes/:id: the kernel retires what
// goes with the node (RetireNode: its protocols with their peers, links and
// secrets, its credentials and agent certificates), then the package
// deletes the row.
func (s *Service) DeleteNode(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return message(http.StatusBadRequest, invalidNodeID)
	}
	if !s.leased(ctx, nodeTable) || s.NodeOps == nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	operation, err := s.submit(ctx, request, fmt.Sprintf("node.retire:proxy-%d", id),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RetireNode{RetireNode: &kernelnodeopsv1.RetireNode{Node: proxyNode(id)}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, kernelOperationWait)
	switch {
	case status.Code(err) == codes.NotFound:
	case err != nil || !succeeded(operation):
		return message(http.StatusInternalServerError, "删除失败")
	}
	if err := db.Delete(&model.Node{}, id).Error; err != nil {
		return message(http.StatusInternalServerError, "删除失败")
	}
	return s.panel(map[string]any{"message": "删除成功"})
}
