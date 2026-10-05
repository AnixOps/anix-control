package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/packages/proxy-node/native/model"
	"github.com/gin-gonic/gin/binding"
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
	nodeTable            = "v2_node"
	nodeProtocolView     = "kapi_node_protocol_public_v1"
	kernelOperationWait  = 30 * time.Second
	invalidNodeID        = "无效的节点ID"
	nodeNotFound         = "节点不存在"
	rawConfigInvalidJSON = "节点原始配置 JSON 无效"
	rawConfigNotObject   = "原始配置必须是 JSON 对象"
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

// GetRawConfig is GET /api/v2/admin/nodes/:id/raw-config: the node's raw
// configuration, its secrets masked; one that is not JSON is refused, as
// it would read as the placeholder alone.
func (s *Service) GetRawConfig(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return message(http.StatusBadRequest, invalidNodeID)
	}
	if !s.leased(ctx, nodeTable) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var node model.Node
	if err := db.First(&node, id).Error; err != nil {
		return message(http.StatusNotFound, nodeNotFound)
	}
	var config any
	if node.RawConfig != nil && *node.RawConfig != "" {
		if err := json.Unmarshal([]byte(v2compat.RedactNodeSecrets(*node.RawConfig)), &config); err != nil {
			return message(http.StatusBadRequest, rawConfigInvalidJSON)
		}
	}
	return s.panel(map[string]any{"node_id": node.ID, "name": node.Name, "raw_config": config})
}

// decodeRawConfig is the kernel's service.DecodeRawNodeConfig: a raw
// configuration sent inline or as a JSON string, which must be a JSON
// object, and its normalized encoding.
func decodeRawConfig(raw any) ([]byte, error) {
	var (
		encoded []byte
		err     error
	)
	if text, ok := raw.(string); ok {
		encoded = []byte(text)
	} else if encoded, err = json.Marshal(raw); err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(encoded, &config); err != nil || config == nil {
		return nil, errors.New("raw config must be an object")
	}
	return json.Marshal(config)
}

// UpdateRawConfig is PUT /api/v2/admin/nodes/:id/raw-config. The kernel
// stores the configuration (PutSecretDocument): the secrets an
// administrator types arrive as sealed handles, a placeholder keeps the
// stored secret, the configuration is validated with the secrets it holds,
// and the kernel's node cache forgets the node. Null clears it.
func (s *Service) UpdateRawConfig(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathID(request)
	if !ok {
		return message(http.StatusBadRequest, invalidNodeID)
	}
	var req struct {
		RawConfig any `json:"raw_config" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return messageError(http.StatusBadRequest, "参数错误", err.Error())
	}
	if !s.leased(ctx, nodeTable) || s.NodeOps == nil || len(request.Binding) == 0 {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	document := "{}"
	if req.RawConfig != nil {
		normalized, err := decodeRawConfig(req.RawConfig)
		if err != nil {
			return message(http.StatusBadRequest, rawConfigNotObject)
		}
		document = string(normalized)
		if strings.Contains(document, v2compat.NodeSecretPlaceholder) {
			var nodes int64
			if err := db.Model(&model.Node{}).Where("id = ?", id).Count(&nodes).Error; err != nil {
				return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
			}
			if nodes == 0 {
				return message(http.StatusNotFound, nodeNotFound)
			}
		}
	}
	operation, err := s.submit(ctx, request, s.requestID(request, fmt.Sprintf("secrets.put:proxy-%d:raw_config", id), true),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_PutSecretDocument{PutSecretDocument: &kernelnodeopsv1.PutSecretDocument{
			Scope: kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG, OwnerId: uint64(id), Column: "raw_config", DocumentJson: []byte(document),
		}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, kernelOperationWait)
	switch {
	case status.Code(err) == codes.NotFound:
		// The kernel's update of a node that does not exist writes nothing
		// and succeeds.
		return s.panel(map[string]any{"message": "配置更新成功"})
	case err != nil:
		return message(http.StatusInternalServerError, "更新失败")
	case !succeeded(operation):
		if operation.GetError().GetCode() == kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED {
			return messageError(http.StatusBadRequest, "WireGuard 配置无效", operation.GetError().GetMessage())
		}
		return message(http.StatusInternalServerError, "更新失败")
	}
	if req.RawConfig == nil {
		if err := db.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{"raw_config": nil}).Error; err != nil {
			return message(http.StatusInternalServerError, "更新失败")
		}
	}
	return s.panel(map[string]any{"message": "配置更新成功"})
}
