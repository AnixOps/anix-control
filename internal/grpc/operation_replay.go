package grpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentv1pb "github.com/AnixOps/anix-agent/sdk/api/grpc/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/protobuf/proto"
)

// kernelOperationEnvelopePayload mirrors the canonical payload emitted by the
// durable bridge. The session binding is transport-specific and must be
// regenerated when an operation is replayed on a new Agent connection.
type kernelOperationEnvelopePayload struct {
	Version        string          `json:"version"`
	OperationID    string          `json:"operation_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	SessionID      string          `json:"session_id"`
	Revision       uint64          `json:"revision"`
	PluginID       string          `json:"plugin_id"`
	TargetVersion  string          `json:"target_version"`
	ConfigHash     string          `json:"config_hash"`
	Config         json.RawMessage `json:"config"`
}

func rebindKernelDesiredSession(operation *agentv1pb.DesiredOperation, sessionID string) (*agentv1pb.DesiredOperation, error) {
	if operation == nil {
		return nil, errors.New("desired operation is nil")
	}
	cloned := proto.Clone(operation).(*agentv1pb.DesiredOperation)
	if len(cloned.PayloadJson) == 0 || !strings.HasPrefix(cloned.Kind, "plugin.") {
		return cloned, nil
	}
	var envelope kernelOperationEnvelopePayload
	if err := json.Unmarshal(cloned.PayloadJson, &envelope); err != nil {
		return nil, fmt.Errorf("decode plugin operation envelope for replay: %w", err)
	}
	if envelope.Version != service.KernelOperationEnvelopeVersion {
		return nil, fmt.Errorf("unsupported plugin operation envelope version %q", envelope.Version)
	}
	if envelope.OperationID != cloned.OperationId || envelope.Revision != cloned.Revision {
		return nil, errors.New("plugin operation envelope identity does not match desired operation")
	}
	envelope.SessionID = sessionID
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode plugin operation envelope for replay: %w", err)
	}
	cloned.PayloadJson = payload
	return cloned, nil
}
