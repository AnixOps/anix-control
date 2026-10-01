package kernelnodeops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

// The messages the validation route answers, which ValidateNodeConfig
// answers too.
const (
	validationNotObject   = "配置必须是 JSON 对象"
	validationWireGuard   = "WireGuard 配置无效"
	validationProtocol    = "协议无效"
	validationNotDocument = "文档不是 JSON"
)

// ValidateNodeConfig runs the kernel's raw configuration and protocol
// validators on a document (node-ops-service.md section 3.1, NO-5), as the
// validation and write routes run them. It is a read of the nodeconfig
// family and records nothing.
//
// A document may carry sealed handles or the placeholder where its secrets
// go: the validators check that a secret is present, not its value. At
// every secret position such a value is replaced by a well-formed stand-in
// of the secret's kind before the validators run (a WireGuard key pair,
// generated for the check), so a document an administrator saved with its
// secrets masked validates as the stored one would.
func (h *hostServer) ValidateNodeConfig(ctx context.Context, request *kernelnodeopsv1.ValidateNodeConfigRequest) (*kernelnodeopsv1.ValidateNodeConfigResponse, error) {
	if err := h.configured(); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG); err != nil {
		return nil, err
	}
	if err := checkJSON(request.GetDocumentJson(), "document_json", maxDocumentBytes); err != nil {
		return nil, err
	}
	if len(request.GetDocumentJson()) == 0 {
		return nil, invalid("document_json is required")
	}
	if err := checkText(request.GetProtocolType(), "protocol_type", maxNameBytes, false); err != nil {
		return nil, err
	}
	switch request.GetKind() {
	case kernelnodeopsv1.NodeConfigKind_NODE_CONFIG_KIND_RAW_CONFIG:
		return validateRawConfig(request.GetDocumentJson()), nil
	case kernelnodeopsv1.NodeConfigKind_NODE_CONFIG_KIND_PROTOCOL:
		return validateProtocol(request.GetProtocolType(), request.GetDocumentJson()), nil
	}
	return nil, invalid("kind must be RAW_CONFIG or PROTOCOL")
}

func refused(message string, err error, path string) *kernelnodeopsv1.ValidateNodeConfigResponse {
	response := &kernelnodeopsv1.ValidateNodeConfigResponse{Message: message}
	if err != nil {
		response.Issues = []*kernelnodeopsv1.ValidationIssue{{Path: path, Message: err.Error()}}
	}
	return response
}

// validateRawConfig validates a raw configuration as POST
// /admin/nodes/validate-config does: the configuration must be a JSON
// object, a WireGuard configuration must be valid, and one without
// server_port is valid with a warning.
func validateRawConfig(raw []byte) *kernelnodeopsv1.ValidateNodeConfigResponse {
	document, err := standIns(string(raw))
	if err != nil {
		return refused(validationNotObject, nil, "")
	}
	validated, err := service.ValidateRawNodeConfig(document)
	if errors.Is(err, service.ErrInvalidNodeProtocol) {
		return refused(validationWireGuard, err, "")
	}
	if err != nil {
		return refused(validationNotObject, nil, "")
	}
	response := &kernelnodeopsv1.ValidateNodeConfigResponse{Valid: true}
	for _, warning := range validated.Warnings {
		response.Issues = append(response.Issues, &kernelnodeopsv1.ValidationIssue{Path: "/server_port", Message: warning})
	}
	return response
}

// validateProtocol validates a protocol row's document, as the protocol
// routes do (service.ValidateNodeProtocol): the document is the row as the
// package writes it (type, port, enable, settings, custom_config, ...), and
// protocol_type, when given, is its type.
func validateProtocol(protocolType string, raw []byte) *kernelnodeopsv1.ValidateNodeConfigResponse {
	document, err := standIns(string(raw))
	if err != nil {
		return refused(validationNotDocument, nil, "")
	}
	var protocol model.NodeProtocol
	if err := json.Unmarshal([]byte(document), &protocol); err != nil {
		return refused(validationProtocol, err, "")
	}
	if protocolType != "" {
		protocol.Type = model.ProtocolType(strings.ToLower(strings.TrimSpace(protocolType)))
	}
	if err := service.ValidateNodeProtocol(&protocol); err != nil {
		return refused(validationProtocol, err, "/settings")
	}
	return &kernelnodeopsv1.ValidateNodeConfigResponse{Valid: true}
}

// standIns replaces every sealed handle and placeholder at a secret
// position of a document (inline, or in the JSON strings a protocol row's
// columns hold) by a stand-in the validators accept: a WireGuard key pair
// for server_private_key and its public key, the placeholder elsewhere.
// A document that is not JSON is an error.
func standIns(document string) (string, error) {
	var root any
	if err := json.Unmarshal([]byte(document), &root); err != nil {
		return "", err
	}
	var private, public string
	keys := func() (string, string) {
		if private == "" {
			private, public, _ = service.GenerateWireGuardKeypair()
		}
		return private, public
	}
	replaced := standIn(root, "", keys)
	encoded, err := json.Marshal(replaced)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func masked(value string) bool {
	return value == service.NodeSecretPlaceholder || sealedsecrets.IsHandle(value) || strings.HasPrefix(value, SealedPrefix)
}

// standIn walks a decoded document.
func standIn(value any, key string, keys func() (string, string)) any {
	switch typed := value.(type) {
	case map[string]any:
		pairPublic := false
		for child, item := range typed {
			if text, ok := item.(string); ok && nodesecrets.IsSecretKey(child) && masked(text) {
				if child == "server_private_key" {
					typed[child], _ = keys()
					pairPublic = true
				} else {
					typed[child] = service.NodeSecretPlaceholder
				}
				continue
			}
			typed[child] = standIn(item, child, keys)
		}
		if pairPublic {
			_, public := keys()
			for _, name := range []string{"server_public_key", "public_key"} {
				if _, present := typed[name]; present {
					typed[name] = public
				}
			}
		}
		return typed
	case []any:
		for index, item := range typed {
			typed[index] = standIn(item, key, keys)
		}
		return typed
	case string:
		// A protocol column holds its document as a JSON string.
		if strings.HasPrefix(strings.TrimSpace(typed), "{") && (strings.Contains(typed, service.NodeSecretPlaceholder) || strings.Contains(typed, SealedPrefix)) {
			if inner, err := standIns(typed); err == nil {
				return inner
			}
		}
		return typed
	}
	return value
}
