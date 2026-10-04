package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/packages/protocol-runtime/native/model"
	"github.com/gin-gonic/gin/binding"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The node protocol routes work on v2_node_protocol, which the package
// adopts once the node credential split finalized it
// (kernel.storage.adopt:v2_node_protocol): its secret positions then hold
// the placeholder, and the secrets are the kernel's. The package writes the
// rows; the secrets an administrator types reach the kernel only, as sealed
// handles the package passes to PutSecretDocument
// (docs/architecture/node-ops-service.md section 6.3).
const protocolTable = "v2_node_protocol"

// nodeStatusView shows whether a node exists, without its credentials.
const nodeStatusView = "kapi_node_status_v1"

// invalidNodeProtocol is the text of the kernel's service.ErrInvalidNodeProtocol.
const invalidNodeProtocol = "invalid node protocol"

// protocolsReady reports whether the package's lease adopts
// v2_node_protocol: false until the kernel finalized it, and the routes then
// answer from the legacy handler.
func (s *Service) protocolsReady(ctx context.Context) bool {
	return s.Leased != nil && s.Leased(ctx, protocolTable)
}

// writesReady is protocolsReady for a route that also needs KernelNodeOps
// and the request binding its sealed handles resolve for.
func (s *Service) writesReady(ctx context.Context, request pluginhostsdk.NativeRequest) bool {
	return s.protocolsReady(ctx) && s.NodeOps != nil && len(request.Binding) > 0
}

// message is the legacy handlers' c.JSON(code, gin.H{"message": text}).
func message(code int, text string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"message": text})
}

// messageError is c.JSON(code, gin.H{"message": text, "error": detail}).
func messageError(code int, text, detail string) (pluginhostsdk.NativeResponse, error) {
	return jsonAnswer(code, map[string]any{"message": text, "error": detail})
}

// pathUint parses a path parameter as the legacy handlers do.
func pathUint(request pluginhostsdk.NativeRequest, name string) (uint, bool) {
	id, err := strconv.ParseUint(request.Metadata.PathParams[name], 10, 32)
	return uint(id), err == nil
}

// redact masks a protocol for an administrator's answer, as the kernel's
// service.RedactNodeProtocol does.
func redact(protocol *model.NodeProtocol) {
	for _, column := range model.SecretColumns {
		field := protocol.Column(column)
		if *field != nil {
			redacted := v2compat.RedactNodeSecrets(**field)
			*field = &redacted
		}
	}
}

// GetProtocols is GET /api/v2/admin/nodes/:id/protocols: a node's
// protocols by sort order, their secrets masked.
func (s *Service) GetProtocols(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	nodeID, ok := pathUint(request, "id")
	if !ok {
		return message(http.StatusBadRequest, "无效的节点ID")
	}
	if !s.protocolsReady(ctx) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var protocols []model.NodeProtocol
	if err := db.Where("node_id = ?", nodeID).Order("sort ASC, id ASC").Find(&protocols).Error; err != nil {
		return message(http.StatusInternalServerError, "获取协议列表失败")
	}
	for i := range protocols {
		redact(&protocols[i])
	}
	return s.panel(protocols)
}

// sealedHandleIn reports whether a document carries a sealed handle.
func sealedHandleIn(document string) bool {
	return strings.Contains(document, v2compat.SealedHandlePrefix)
}

// hasSecretPositions reports whether a document has a secret position: a
// handle, the placeholder, or a value the masking would hide.
func hasSecretPositions(document string) bool {
	return sealedHandleIn(document) || strings.Contains(document, v2compat.NodeSecretPlaceholder) || v2compat.RedactNodeSecrets(document) != document
}

// storable reports whether a secret column's document can go through the
// kernel's PutSecretDocument: blank, the placeholder, or JSON. A document
// that is not JSON is a secret whole (the masking hides it), which the
// kernel stores only from its own writers: such a request is the legacy
// handler's.
func storable(document *string) bool {
	if document == nil {
		return true
	}
	text := strings.TrimSpace(*document)
	return text == "" || text == v2compat.NodeSecretPlaceholder || json.Valid([]byte(text))
}

// validateProtocol runs the kernel's protocol validator on the protocol the
// route writes (ValidateNodeConfig): sealed handles and placeholders stand
// in for the secrets. It answers the validator's error text, as the
// kernel's service returns it, or "" when the protocol is valid.
func (s *Service) validateProtocol(ctx context.Context, protocol *model.NodeProtocol) (string, error) {
	document, err := json.Marshal(protocol)
	if err != nil {
		return "", err
	}
	response, err := s.NodeOps.ValidateNodeConfig(ctx, &kernelnodeopsv1.ValidateNodeConfigRequest{
		Kind: kernelnodeopsv1.NodeConfigKind_NODE_CONFIG_KIND_PROTOCOL, DocumentJson: document,
	})
	if err != nil {
		return "", err
	}
	if response.GetValid() {
		return "", nil
	}
	if issues := response.GetIssues(); len(issues) > 0 {
		return issues[0].GetMessage(), nil
	}
	return response.GetMessage(), nil
}

// redactedRow is the protocol as the package sends it with its settings
// (PutSecretDocument.protocol_json): every secret column masked, so it
// carries no handle and no secret.
func redactedRow(protocol model.NodeProtocol) []byte {
	redact(&protocol)
	protocol.Node, protocol.SubscriptionGroups = nil, nil
	encoded, err := json.Marshal(protocol)
	if err != nil {
		return nil
	}
	return encoded
}

// putSecrets stores a column's document through the kernel: its handles
// resolve for the request, its placeholders keep the stored values, and
// the kernel writes the column (redacted, in a finalized table) and the
// split table. The settings carry the protocol, which the kernel validates
// with the typed secrets. It answers the failed operation, if any.
func (s *Service) putSecrets(ctx context.Context, request pluginhostsdk.NativeRequest, protocolID uint, column, document string, protocol []byte) (*kernelnodeopsv1.Operation, error) {
	put := &kernelnodeopsv1.PutSecretDocument{
		Scope: kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, OwnerId: uint64(protocolID), Column: column, DocumentJson: []byte(document),
	}
	if column == "settings" {
		put.ProtocolJson = protocol
	}
	operation, err := s.submit(ctx, request, s.requestID(request, fmt.Sprintf("secrets.put:protocol-%d:%s", protocolID, column), true),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_PutSecretDocument{PutSecretDocument: put}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, kernelWait)
	if err != nil {
		return nil, err
	}
	if !succeeded(operation) {
		return operation, nil
	}
	return nil, nil
}

// retireProtocol removes what the kernel holds for a protocol: its
// secrets, its users' WireGuard peers and its subscription group links
// (RetireProtocol). A protocol the kernel does not know is nothing to do.
func (s *Service) retireProtocol(ctx context.Context, request pluginhostsdk.NativeRequest, protocolID uint) error {
	operation, err := s.submit(ctx, request, fmt.Sprintf("protocol.retire:%d", protocolID),
		&kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RetireProtocol{RetireProtocol: &kernelnodeopsv1.RetireProtocol{ProtocolId: uint64(protocolID)}}},
		kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, kernelWait)
	if status.Code(err) == codes.NotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if !succeeded(operation) {
		return fmt.Errorf("protocol.retire %s: %s", operation.GetState(), operation.GetError().GetMessage())
	}
	return nil
}

// writeFailure answers a failed secret write: a refused protocol as the
// kernel's validator refuses it (400), anything else as a failed write.
func writeFailure(verb string, operation *kernelnodeopsv1.Operation, err error) (pluginhostsdk.NativeResponse, error) {
	if err != nil {
		return messageError(http.StatusInternalServerError, verb, err.Error())
	}
	if operation.GetError().GetCode() == kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED {
		return messageError(http.StatusBadRequest, verb, operation.GetError().GetMessage())
	}
	return messageError(http.StatusInternalServerError, verb, operation.GetError().GetMessage())
}

// CreateProtocol is POST /api/v2/admin/nodes/:id/protocols. The protocol is
// validated as the kernel validates it, inserted with its secret columns
// masked, and each column that carries a typed secret is stored by the
// kernel. A placeholder stands for nothing yet, so it is stored empty. A
// write that fails takes the protocol out again.
func (s *Service) CreateProtocol(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	const failed = "创建失败"
	nodeID, ok := pathUint(request, "id")
	if !ok {
		return message(http.StatusBadRequest, "无效的节点ID")
	}
	var protocol model.NodeProtocol
	if err := binding.JSON.BindBody(request.Body, &protocol); err != nil {
		return messageError(http.StatusBadRequest, "参数错误", bindError(err))
	}
	if !s.writesReady(ctx, request) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	for _, column := range model.SecretColumns {
		if !storable(*protocol.Column(column)) {
			return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
		}
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	protocol.NodeID = nodeID
	protocol.ID, protocol.Node, protocol.SubscriptionGroups = 0, nil, nil

	var nodes int64
	if err := db.Table(nodeStatusView).Where("id = ?", nodeID).Count(&nodes).Error; err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if nodes == 0 {
		return messageError(http.StatusInternalServerError, failed, "节点不存在")
	}
	for _, column := range model.SecretColumns {
		if field := protocol.Column(column); *field != nil {
			kept := v2compat.KeepNodeSecrets(**field, "")
			*field = &kept
		}
	}
	refusal, err := s.validateProtocol(ctx, &protocol)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if refusal != "" {
		return messageError(http.StatusBadRequest, failed, refusal)
	}

	typed := protocol
	row := protocol
	redact(&row)
	if err := db.Omit(clause.Associations).Create(&row).Error; err != nil {
		return messageError(http.StatusInternalServerError, failed, err.Error())
	}
	for _, column := range model.SecretColumns {
		document := *typed.Column(column)
		if document == nil || !sealedHandleIn(*document) {
			continue
		}
		operation, err := s.putSecrets(ctx, request, row.ID, column, *document, redactedRow(row))
		if err != nil || operation != nil {
			if undo := s.retireProtocol(ctx, request, row.ID); undo == nil {
				_ = db.Delete(&model.NodeProtocol{}, row.ID).Error
			}
			return writeFailure(failed, operation, err)
		}
	}
	return s.panel(row)
}

// UpdateProtocol is PUT /api/v2/admin/nodes/:id/protocols/:protocol_id.
// The update is checked as the kernel checks it: its keys resolved to
// columns, the protocol's id and node never changed, the merged protocol
// validated. Its columns without secrets are written first; then each
// secret column it names is stored by the kernel, the settings validated
// with the typed secrets. A refusal there puts the columns back.
func (s *Service) UpdateProtocol(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	const failed = "更新失败"
	protocolID, ok := pathUint(request, "protocol_id")
	if !ok {
		return message(http.StatusBadRequest, "无效的协议ID")
	}
	var updates map[string]any
	if err := binding.JSON.BindBody(request.Body, &updates); err != nil {
		return message(http.StatusBadRequest, "参数错误")
	}
	if !s.writesReady(ctx, request) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var current model.NodeProtocol
	if err := db.First(&current, protocolID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
		}
		return messageError(http.StatusInternalServerError, failed, err.Error())
	}
	resolved, err := columnUpdates(db, updates, "id", "node_id")
	if err != nil {
		return messageError(http.StatusBadRequest, failed, invalidNodeProtocol+": "+err.Error())
	}
	normalized, err := normalizeProtocolUpdates(resolved)
	if err != nil {
		return messageError(http.StatusInternalServerError, failed, err.Error())
	}
	for _, column := range model.SecretColumns {
		if text, ok := normalized[column].(string); ok && !storable(&text) {
			return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
		}
	}
	candidate, err := mergeProtocol(current, normalized)
	if err != nil {
		return messageError(http.StatusInternalServerError, failed, err.Error())
	}
	refusal, err := s.validateProtocol(ctx, &candidate)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if refusal != "" {
		return messageError(http.StatusBadRequest, failed, refusal)
	}

	// The kernel writes the secret columns it stores; the package writes
	// the others, and a secret column set to null.
	plain := map[string]any{}
	secret := map[string]string{}
	for column, value := range normalized {
		text, isText := value.(string)
		if !isSecretColumn(column) || !isText {
			plain[column] = value
			continue
		}
		stored := *current.Column(column)
		if hasSecretPositions(text) || (stored != nil && hasSecretPositions(*stored)) {
			secret[column] = text
			continue
		}
		plain[column] = value
	}
	// A column set to null drops the secrets the kernel holds for it.
	for column, value := range normalized {
		if value == nil && isSecretColumn(column) {
			if stored := *current.Column(column); stored != nil && hasSecretPositions(*stored) {
				if operation, err := s.putSecrets(ctx, request, protocolID, column, "{}", nil); err != nil || operation != nil {
					return writeFailure(failed, operation, err)
				}
			}
		}
	}
	if len(plain) > 0 {
		if err := db.Model(&model.NodeProtocol{}).Where("id = ?", protocolID).Updates(plain).Error; err != nil {
			return messageError(http.StatusInternalServerError, failed, err.Error())
		}
	}
	row := redactedRow(candidate)
	for _, column := range model.SecretColumns {
		document, ok := secret[column]
		if !ok {
			continue
		}
		operation, err := s.putSecrets(ctx, request, protocolID, column, document, row)
		if err != nil || operation != nil {
			if len(plain) > 0 {
				_ = db.Model(&model.NodeProtocol{}).Where("id = ?", protocolID).Updates(previousValues(db, current, plain)).Error
			}
			return writeFailure(failed, operation, err)
		}
	}
	return s.panel(map[string]any{"message": "更新成功"})
}

// DeleteProtocol is DELETE /api/v2/admin/nodes/:id/protocols/:protocol_id:
// the kernel retires what goes with the protocol (RetireProtocol), then the
// package deletes the row.
func (s *Service) DeleteProtocol(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	protocolID, ok := pathUint(request, "protocol_id")
	if !ok {
		return message(http.StatusBadRequest, "无效的协议ID")
	}
	if !s.protocolsReady(ctx) || s.NodeOps == nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	if err := s.retireProtocol(ctx, request, protocolID); err != nil {
		return message(http.StatusInternalServerError, "删除失败")
	}
	if err := db.Delete(&model.NodeProtocol{}, protocolID).Error; err != nil {
		return message(http.StatusInternalServerError, "删除失败")
	}
	return s.panel(map[string]any{"message": "删除成功"})
}

func isSecretColumn(column string) bool {
	for _, candidate := range model.SecretColumns {
		if candidate == column {
			return true
		}
	}
	return false
}

// previousValues are the current values of the columns an update wrote,
// to put them back.
func previousValues(db *gorm.DB, current model.NodeProtocol, written map[string]any) map[string]any {
	encoded, err := json.Marshal(current)
	if err != nil {
		return nil
	}
	var byName map[string]any
	if err := json.Unmarshal(encoded, &byName); err != nil {
		return nil
	}
	previous := make(map[string]any, len(written))
	for column := range written {
		if value, ok := byName[column]; ok {
			previous[column] = value
		}
	}
	return previous
}

// mergeProtocol is the protocol an update leaves, as the kernel merges it
// to validate it: the current row with the update's columns.
func mergeProtocol(current model.NodeProtocol, updates map[string]any) (model.NodeProtocol, error) {
	currentJSON, err := json.Marshal(current)
	if err != nil {
		return model.NodeProtocol{}, err
	}
	var merged map[string]any
	if err := json.Unmarshal(currentJSON, &merged); err != nil {
		return model.NodeProtocol{}, err
	}
	for key, value := range updates {
		merged[key] = value
	}
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return model.NodeProtocol{}, err
	}
	var candidate model.NodeProtocol
	if err := json.Unmarshal(mergedJSON, &candidate); err != nil {
		return model.NodeProtocol{}, fmt.Errorf("协议更新参数无效: %w", err)
	}
	candidate.ID = current.ID
	candidate.NodeID = current.NodeID
	return candidate, nil
}

// normalizeProtocolUpdates is the kernel's: a secret column's value is
// stored as JSON text, whatever JSON the update sent.
func normalizeProtocolUpdates(updates map[string]any) (map[string]any, error) {
	normalized := make(map[string]any, len(updates))
	for key, value := range updates {
		if !isSecretColumn(key) {
			normalized[key] = value
			continue
		}
		if value == nil {
			normalized[key] = nil
			continue
		}
		if text, ok := value.(string); ok {
			normalized[key] = text
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("字段 %s 不是有效 JSON: %w", key, err)
		}
		normalized[key] = string(encoded)
	}
	return normalized, nil
}

// columnUpdates is the kernel's: an update's keys resolved to the columns
// of v2_node_protocol, whatever spelling names them (column, field name,
// any case, underscores or not), associations left out, refused columns
// dropped, and a column named twice refused.
func columnUpdates(db *gorm.DB, updates map[string]any, refused ...string) (map[string]any, error) {
	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(&model.NodeProtocol{}); err != nil {
		return nil, err
	}
	columns := make(map[string]string)
	for _, field := range statement.Schema.Fields {
		if field.DBName != "" {
			columns[updateKey(field.DBName)] = field.DBName
			columns[updateKey(field.Name)] = field.DBName
		}
	}
	associations := make(map[string]bool)
	for name := range statement.Schema.Relationships.Relations {
		associations[updateKey(name)] = true
	}
	refusedColumns := make(map[string]bool, len(refused))
	for _, column := range refused {
		refusedColumns[column] = true
	}
	resolved := make(map[string]any, len(updates))
	for key, update := range updates {
		column, ok := columns[updateKey(key)]
		if !ok {
			if associations[updateKey(key)] {
				continue
			}
			column = key
		}
		if refusedColumns[column] {
			continue
		}
		if _, duplicate := resolved[column]; duplicate {
			return nil, fmt.Errorf("字段 %s 重复", column)
		}
		resolved[column] = update
	}
	return resolved, nil
}

func updateKey(key string) string {
	return strings.ToLower(strings.ReplaceAll(key, "_", ""))
}

// bindError is the message of the legacy handler's binding error. A body
// that is not a JSON object fails to decode as a whole, and encoding/json
// then names the Go type with its package, which is the kernel's model
// package for the kernel and this mirror (also named model) here: the
// same text.
func bindError(err error) string {
	return err.Error()
}
