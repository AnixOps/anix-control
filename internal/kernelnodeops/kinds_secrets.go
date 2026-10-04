package kernelnodeops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// SecretDocuments executes PutSecretDocument (node-ops-service.md section
// 3.3, NO-5): the secret-keeping part of the protocol and raw
// configuration writes. In the document a package sends, a secret is a
// sealed handle (a value the administrator typed), the placeholder (keep
// the stored value) or empty (no value); a secret in clear is refused by
// the kind's check.
//
// The Preparer resolves every handle for the bound request, at the field
// the gateway sealed it from (the column's pointer followed by the
// secret's), and keeps the values for the executor. The executor stores the
// document as the legacy routes store it (service.PutNodeProtocolSecretDocumentTx,
// PutNodeRawConfigDocumentTx): the stored values at the placeholders, the
// legacy column and the split table in one transaction, and answers the
// redacted document the package keeps in its own row.
type SecretDocuments struct{}

// Register serves secrets.put on registry.
func (s *SecretDocuments) Register(registry *Registry) error {
	return registry.Register(KindSecretsPut, prepared{prepare: s.prepare, execute: s.put})
}

func init() {
	if err := (&SecretDocuments{}).Register(DefaultExecutors); err != nil {
		panic(err)
	}
}

// resolvedSecrets is what the Preparer keeps: each resolved handle's
// JSON-encoded value by pointer.
type resolvedSecrets map[string]string

// secretTarget is the sealed secret target of a document's owner.
func secretTarget(op *kernelnodeopsv1.PutSecretDocument) sealedsecrets.Target {
	if op.GetScope() == kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG {
		return sealedsecrets.Target{Kind: sealedsecrets.TargetProxy, ID: op.GetOwnerId()}
	}
	return sealedsecrets.Target{Kind: sealedsecrets.TargetProtocol, ID: op.GetOwnerId()}
}

// handlePositions finds the sealed handles of a document: the secret
// positions whose value is one.
func handlePositions(document string) []nodesecrets.Position {
	var handles []nodesecrets.Position
	for _, position := range nodesecrets.SecretPositions(document) {
		var value string
		if json.Unmarshal([]byte(position.Value), &value) == nil && sealedsecrets.IsHandle(value) {
			position.Value = value
			handles = append(handles, position)
		}
	}
	return handles
}

// prepare resolves the document's handles, all or none, for the bound
// request and the document's owner.
func (s *SecretDocuments) prepare(_ context.Context, submission *Submission) error {
	op := submission.Operation.GetPutSecretDocument()
	document := string(op.GetDocumentJson())
	handles := handlePositions(document)
	if len(handles) == 0 {
		if sealedsecrets.ContainsHandle(document) {
			return status.Error(codes.InvalidArgument, "document_json holds a sealed handle outside a secret position")
		}
		return nil
	}
	if submission.Request == nil {
		return status.Error(codes.PermissionDenied, "storing a typed secret needs the request binding its handles were sealed for")
	}
	uses := make([]sealedsecrets.Use, len(handles))
	target := secretTarget(op)
	for index, handle := range handles {
		uses[index] = sealedsecrets.Use{Handle: handle.Value, Target: target, Field: "/" + op.GetColumn() + handle.Pointer}
	}
	secrets, err := submission.Unseal(uses...)
	if err != nil {
		return err
	}
	resolved := make(resolvedSecrets, len(secrets))
	replacements := make(map[string]string, len(secrets))
	for index, secret := range secrets {
		encoded := secret.Value
		if !secret.JSON {
			raw, err := json.Marshal(secret.Value)
			if err != nil {
				return status.Error(codes.Internal, "a resolved secret cannot be encoded")
			}
			encoded = string(raw)
		}
		resolved[handles[index].Pointer] = encoded
		replacements[handles[index].Pointer] = `"` + SealedPrefix + `"`
	}
	// The operation keeps the bare prefix where each handle was, as its
	// canonical form does.
	stripped, err := nodesecrets.ReplacePositions(document, replacements)
	if err != nil {
		return status.Error(codes.InvalidArgument, "document_json is not a JSON document")
	}
	if sealedsecrets.ContainsHandle(stripped) {
		return status.Error(codes.InvalidArgument, "document_json holds a sealed handle outside a secret position")
	}
	op.DocumentJson = []byte(stripped)
	submission.Retain(resolved)
	return nil
}

// put executes secrets.put.
func (s *SecretDocuments) put(ctx context.Context, run *Run) Outcome {
	op := run.Operation.GetPutSecretDocument()
	id := nodeID(op.GetOwnerId())
	if outcome, ok := begin(ctx, run, kernelnodeopsv1.Channel_CHANNEL_KERNEL); !ok {
		return outcome
	}
	document := string(op.GetDocumentJson())
	resolved, _ := run.Prepared().(resolvedSecrets)
	replacements := make(map[string]string, len(resolved))
	for pointer, encoded := range resolved {
		replacements[pointer] = encoded
		var value string
		if json.Unmarshal([]byte(encoded), &value) == nil {
			run.UseSecret(value)
		}
	}
	full, err := nodesecrets.ReplacePositions(document, replacements)
	if err != nil {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL, "the resolved secrets do not fit the document", false)
	}
	if unresolved(full) {
		return Failed(kernelnodeopsv1.ErrorCode_ERROR_CODE_SECRET_HANDLE_INVALID,
			"the typed secrets are no longer held: the request ended or the kernel restarted since the submission; nothing was stored", false)
	}
	var outcome service.SecretDocumentOutcome
	failed, ok := transact(ctx, run, "storing the secret document", func(tx *gorm.DB) (err error) {
		if op.GetScope() == kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG {
			outcome, err = service.PutNodeRawConfigDocumentTx(tx, id, full)
		} else {
			outcome, err = service.PutNodeProtocolSecretDocumentTx(tx, id, op.GetColumn(), full)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_TARGET_GONE, fmt.Sprintf("the document's owner %d not found", id), false)
		}
		if err != nil {
			return err
		}
		if op.GetScope() == kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG {
			return validateStoredRawConfig(outcome.Full)
		}
		return validateStoredProtocol(op, outcome.Full)
	})
	if !ok {
		return failed
	}
	if op.GetScope() == kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG {
		// The node's row changed: the kernel's node cache forgets it, as
		// the legacy update does.
		service.DropNodeCache(id)
	}
	for _, position := range nodesecrets.SecretPositions(outcome.Full) {
		var value string
		if json.Unmarshal([]byte(position.Value), &value) == nil {
			run.UseSecret(value)
		}
	}
	return Succeeded(&kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_SecretDocument{SecretDocument: &kernelnodeopsv1.SecretDocumentResult{
		RedactedJson: []byte(outcome.Redacted), Stored: counter32(int64(len(resolved))), Kept: counter32(int64(outcome.Kept)), Cleared: counter32(int64(outcome.Cleared)),
	}}})
}

// validateStoredProtocol validates the protocol a package writes with the
// settings the operation stores (PutSecretDocument.protocol_json), as the
// protocol routes validate a write: the validator sees the typed secrets.
// A refusal fails the operation, and its transaction stores nothing.
func validateStoredProtocol(op *kernelnodeopsv1.PutSecretDocument, settings string) error {
	if op.GetScope() != kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL || op.GetColumn() != "settings" || len(op.GetProtocolJson()) == 0 {
		return nil
	}
	var protocol model.NodeProtocol
	if err := json.Unmarshal(op.GetProtocolJson(), &protocol); err != nil {
		return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "protocol_json is not a node protocol: "+err.Error(), false)
	}
	protocol.Settings = &settings
	if err := service.ValidateNodeProtocol(&protocol); err != nil {
		return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
	}
	return nil
}

// validateStoredRawConfig validates a raw configuration with the secrets it
// stores, as the raw configuration route validates it
// (service.ValidateWireGuardRuntimeConfig): the validator sees the typed
// secrets. A refusal fails the operation, and its transaction stores
// nothing.
func validateStoredRawConfig(document string) error {
	if strings.TrimSpace(document) == "" {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(document), &config); err != nil || config == nil {
		return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, "原始配置必须是 JSON 对象", false)
	}
	if err := service.ValidateWireGuardRuntimeConfig(config); err != nil {
		return fail(kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, err.Error(), false)
	}
	return nil
}

// unresolved reports whether a document still holds the bare prefix at a
// secret position: a handle whose value the executor no longer has.
func unresolved(document string) bool {
	for _, position := range nodesecrets.SecretPositions(document) {
		var value string
		if json.Unmarshal([]byte(position.Value), &value) == nil && strings.HasPrefix(value, SealedPrefix) {
			return true
		}
	}
	return false
}
