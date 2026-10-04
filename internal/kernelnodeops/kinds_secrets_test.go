package kernelnodeops

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const (
	protocolUpdateRoute = "protocol.admin.nodes.id.protocols.protocol_id.put"
	rawConfigRoute      = "proxy.admin.nodes.id.raw_config.put"
)

func putSecretSpec(scope kernelnodeopsv1.SecretScope, owner uint64, column string, document any) *kernelnodeopsv1.OperationSpec {
	raw, _ := json.Marshal(document)
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_PutSecretDocument{PutSecretDocument: &kernelnodeopsv1.PutSecretDocument{
		Scope: scope, OwnerId: owner, Column: column, DocumentJson: raw,
	}}}
}

// sealedDocument is the document the package reads at a top-level field of
// a sealed request: its secrets are handles.
func (r *request) sealedDocument(field string) map[string]any {
	document, _ := r.sealed[field].(map[string]any)
	return document
}

func protocolColumn(t *testing.T, db *gorm.DB, id uint, column string) string {
	t.Helper()
	var protocol model.NodeProtocol
	require.NoError(t, db.First(&protocol, id).Error)
	var value *string
	switch column {
	case "reality_settings":
		value = protocol.RealitySettings
	case "settings":
		value = protocol.Settings
	}
	if value == nil {
		return ""
	}
	return *value
}

func protocolSecret(t *testing.T, db *gorm.DB, owner uint64, column, pointer string) (string, bool) {
	t.Helper()
	var rows []model.ProtocolSecret
	require.NoError(t, db.Where("scope = ? AND owner_id = ? AND column_name = ? AND json_pointer = ?", nodesecrets.ScopeNodeProtocol, owner, column, pointer).Find(&rows).Error)
	if len(rows) == 0 {
		return "", false
	}
	return rows[0].Value, true
}

// A typed secret reaches the kernel only: the package sends the document
// with a handle, the kernel stores the value in the legacy column and the
// split table in one transaction and answers the redacted document.
// Placeholders keep the stored values; a secret left out is cleared.
func TestPutProtocolSecretDocument(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(protocolHost, allFamilies())
		require.NoError(t, db.Create(&model.NodeProtocol{ID: 6, NodeID: 1, Name: "other", Type: model.ProtocolVLESS, Port: 8443}).Error)

		typed := h.bindings.open(protocolHost, protocolUpdateRoute, map[string]string{"protocol_id": "5"},
			`{"name":"reality","reality_settings":{"dest":"www.example.test:443","private_key":"reality-private-typed","public_key":"pub","short_id":"6ba8"}}`)
		document := typed.sealedDocument("reality_settings")
		handle, _ := document["private_key"].(string)
		require.True(t, sealedsecrets.IsHandle(handle), "the package reads a handle")

		// Refusals: without the binding; for another protocol; a handle
		// outside a secret position; a secret in clear.
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "secrets.put:5:unbound",
			Operation: putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", document)})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "secrets.put:6:other", Request: typed.binding(),
			Operation: putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 6, "reality_settings", document)})
		require.Equal(t, codes.PermissionDenied, status.Code(err), "sealed for protocol 5, not 6")
		assert.NotContains(t, err.Error(), handle)
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "secrets.put:5:misplaced", Request: typed.binding(),
			Operation: putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", map[string]any{"dest": handle})})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "secrets.put:5:clear", Request: typed.binding(),
			Operation: putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", map[string]any{"private_key": "in-clear"})})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Empty(t, h.rows(t), "nothing recorded")
		require.Empty(t, protocolColumn(t, db, 5, "reality_settings"))

		operation, applied := submitBound(t, client, "secrets.put:5:reality", putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", document), typed)
		require.True(t, applied)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		result := operation.GetResult().GetSecretDocument()
		require.EqualValues(t, 1, result.GetStored())
		require.Zero(t, result.GetKept())
		require.Zero(t, result.GetCleared())
		stored := protocolColumn(t, db, 5, "reality_settings")
		require.Contains(t, stored, "reality-private-typed", "the legacy column holds the value")
		require.Equal(t, service.RedactNodeSecretsJSON(stored), string(result.GetRedactedJson()), "the package stores the redacted document")
		require.Contains(t, string(result.GetRedactedJson()), service.NodeSecretPlaceholder)
		value, ok := protocolSecret(t, db, 5, "reality_settings", "/private_key")
		require.True(t, ok)
		require.Equal(t, `"reality-private-typed"`, value, "the split table agrees")
		requireNoSecret(t, db, operation, "reality-private-typed")

		again, applied := submitBound(t, client, "secrets.put:5:reality", putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", document), typed)
		require.False(t, applied, "a repeat with the same handles is the first operation")
		require.Equal(t, operation.GetOperationId(), again.GetOperationId())

		// The administrator saves the masked answer back: the placeholder
		// keeps the stored secret, and a changed public field is written.
		kept := map[string]any{"dest": "other.example.test:443", "private_key": service.NodeSecretPlaceholder, "public_key": "pub-2", "short_id": "6ba8"}
		operation, _ = submitBound(t, client, "secrets.put:5:keep", putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", kept), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		result = operation.GetResult().GetSecretDocument()
		require.EqualValues(t, 1, result.GetKept())
		require.Zero(t, result.GetStored())
		stored = protocolColumn(t, db, 5, "reality_settings")
		require.Contains(t, stored, "reality-private-typed")
		require.Contains(t, stored, "other.example.test:443")
		require.NotContains(t, stored, service.NodeSecretPlaceholder)
		requireNoSecret(t, db, operation, "reality-private-typed")

		// A document without the secret clears it.
		operation, _ = submitBound(t, client, "secrets.put:5:clear", putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", map[string]any{"dest": "x"}), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.EqualValues(t, 1, operation.GetResult().GetSecretDocument().GetCleared())
		_, ok = protocolSecret(t, db, 5, "reality_settings", "/private_key")
		require.False(t, ok, "the split row went")
		require.NotContains(t, protocolColumn(t, db, 5, "reality_settings"), "reality-private-typed")
	})
}

// A raw configuration's secrets go the same way, for the node the handles
// were sealed for; a secret the kernel no longer holds (its request ended
// before the operation ran) stores nothing.
func TestPutRawConfigDocument(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(proxyHost, allFamilies())

		typed := h.bindings.open(proxyHost, rawConfigRoute, map[string]string{"id": "1"}, `{"raw_config":{"type":"vless","server_port":443,"tls_settings":{"private_key":"raw-private-typed","server_name":"edge"}}}`)
		document := typed.sealedDocument("raw_config")
		require.True(t, sealedsecrets.ContainsHandle(string(mustJSON(t, document))))
		operation, _ := submitBound(t, client, "secrets.put:raw-1", putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG, 1, "raw_config", document), typed)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		var node model.Node
		require.NoError(t, db.First(&node, 1).Error)
		require.NotNil(t, node.RawConfig)
		require.Contains(t, *node.RawConfig, "raw-private-typed")
		var secret model.ProtocolSecret
		require.NoError(t, db.Where("scope = ? AND owner_id = ? AND json_pointer = ?", nodesecrets.ScopeNodeRawConfig, 1, "/tls_settings/private_key").First(&secret).Error)
		require.Equal(t, `"raw-private-typed"`, secret.Value)
		require.Equal(t, service.RedactNodeSecretsJSON(*node.RawConfig), string(operation.GetResult().GetSecretDocument().GetRedactedJson()))
		requireNoSecret(t, db, operation, "raw-private-typed")

	})
}

// A typed secret is held until its request ends: an operation dispatched
// after that (the kernel restarted, or the request's deadline passed)
// stores nothing and says so.
func TestSecretsLostWithTheirRequestStoreNothing(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, db.AutoMigrate(credentialModels...))
		require.NoError(t, nodesecrets.EnsureSchema(db))
		b := newBindings(t)
		clock := time.Now()
		h := newHarness(t, db, func(e *Engine) {
			e.Secrets = b.store
			e.Now = func() time.Time { return clock }
		})
		require.NoError(t, (&SecretDocuments{}).Register(h.registry))
		client := h.client(proxyHost, allFamilies())
		later := b.open(proxyHost, rawConfigRoute, map[string]string{"id": "2"}, `{"raw_config":{"type":"vless","tls_settings":{"private_key":"raw-private-late"}}}`)
		receipt, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "secrets.put:raw-2", Request: later.binding(),
			Operation: putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG, 2, "raw_config", later.sealedDocument("raw_config"))})
		require.NoError(t, err)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_PENDING, receipt.GetOperation().GetState())
		clock = clock.Add(2 * time.Minute)
		h.start(t)
		ended := eventually(t, get(t, client, receipt.GetOperation().GetOperationId()), inState(kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED))
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, ended.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_SECRET_HANDLE_INVALID, ended.GetError().GetCode())
		var node model.Node
		require.NoError(t, db.First(&node, 2).Error)
		require.Nil(t, node.RawConfig, "nothing stored")
		requireNoSecret(t, db, ended, "raw-private-late")
	})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

// The legacy column and the split rows agree after a put in both phases,
// and the dual-read reader answers the stored secret.
func TestSecretDocumentsAgreeInBothPhases(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(protocolHost, allFamilies())
		for round, phase := range []string{nodesecrets.PhaseDualWrite, nodesecrets.PhaseDualRead} {
			if phase == nodesecrets.PhaseDualRead {
				dualRead(t, db)
			}
			typed := h.bindings.open(protocolHost, protocolUpdateRoute, map[string]string{"protocol_id": "5"}, `{"settings":{"password":"ss-`+phase+`","method":"aes"}}`)
			operation, _ := submitBound(t, client, "secrets.put:5:settings:"+phase, putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "settings", typed.sealedDocument("settings")), typed)
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
			stored := protocolColumn(t, db, 5, "settings")
			require.Contains(t, stored, "ss-"+phase)
			value, ok := protocolSecret(t, db, 5, "settings", "/password")
			require.True(t, ok)
			require.Equal(t, `"ss-`+phase+`"`, value, phase)
			var protocol model.NodeProtocol
			require.NoError(t, db.First(&protocol, 5).Error)
			nodesecrets.ResolveProtocol(db, &protocol)
			require.Equal(t, stored, *protocol.Settings, "the reader of %s answers the stored document", phase)
			// A keep-on-save round in this phase too.
			operation, _ = submitBound(t, client, "secrets.put:5:keep:"+phase, putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "settings", map[string]any{"password": service.NodeSecretPlaceholder, "method": "chacha"}), nil)
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
			require.Contains(t, protocolColumn(t, db, 5, "settings"), "ss-"+phase)
			_, err := nodesecrets.Backfill(context.Background(), db, nodesecrets.BackfillOptions{Restart: true})
			require.NoError(t, err)
			_, err = nodesecrets.Verify(context.Background(), db, nodesecrets.VerifyOptions{})
			require.NoError(t, err, "round %d (%s)", round, phase)
		}
	})
}

// With protocol_json, the settings a package stores are validated with the
// typed secrets, as the protocol routes validate a write: a WireGuard
// private key that does not match its public key fails VALIDATION_FAILED
// and stores nothing, where ValidateNodeConfig's stand-ins pass. The
// columns sent with it must be redacted.
func TestPutProtocolSettingsValidatesTheTypedSecrets(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(protocolHost, allFamilies())
		private, public, err := service.GenerateWireGuardKeypair()
		require.NoError(t, err)
		_, otherPublic, err := service.GenerateWireGuardKeypair()
		require.NoError(t, err)
		settings := func(publicKey string) string {
			return `{"settings":{"cidr":"10.8.0.0/24","server_address":"10.8.0.1/24","server_private_key":"` + private +
				`","server_public_key":"` + publicKey + `","tunnel_type":"quic"}}`
		}
		row := func(document map[string]any) []byte {
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			return mustJSON(t, map[string]any{"type": "wireguard", "enable": 0, "port": 51820, "settings": service.RedactNodeSecretsJSON(string(encoded))})
		}
		spec := func(document map[string]any, protocol []byte) *kernelnodeopsv1.OperationSpec {
			spec := putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "settings", document)
			spec.GetPutSecretDocument().ProtocolJson = protocol
			return spec
		}

		mismatched := h.bindings.open(protocolHost, protocolUpdateRoute, map[string]string{"protocol_id": "5"}, settings(otherPublic))
		document := mismatched.sealedDocument("settings")
		operation, _ := submitBound(t, client, "secrets.put:5:mismatched", spec(document, row(document)), mismatched)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, operation.GetError().GetCode())
		require.Contains(t, operation.GetError().GetMessage(), "server_public_key 与 server_private_key 不匹配")
		require.Empty(t, protocolColumn(t, db, 5, "settings"), "a refused document stores nothing")
		_, ok := protocolSecret(t, db, 5, "settings", "/server_private_key")
		require.False(t, ok)
		requireNoSecret(t, db, operation, private)

		matching := h.bindings.open(protocolHost, protocolUpdateRoute, map[string]string{"protocol_id": "5"}, settings(public))
		document = matching.sealedDocument("settings")
		operation, _ = submitBound(t, client, "secrets.put:5:matching", spec(document, row(document)), matching)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Contains(t, protocolColumn(t, db, 5, "settings"), private)

		// protocol_json carries the columns redacted, never a handle or a
		// secret in clear, and only with a protocol's settings.
		for name, protocol := range map[string][]byte{
			"a handle":        mustJSON(t, map[string]any{"type": "wireguard", "settings": mustJSONString(t, document)}),
			"a clear secret":  mustJSON(t, map[string]any{"type": "wireguard", "settings": `{"server_private_key":"` + private + `"}`}),
			"not an object":   []byte(`[1]`),
			"not JSON inside": mustJSON(t, map[string]any{"type": "wireguard", "settings": "not json"}),
		} {
			_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
				RequestId: "secrets.put:5:" + name, Request: matching.binding(), Operation: spec(map[string]any{"cidr": "10.8.0.0/24"}, protocol),
			})
			require.Equal(t, codes.InvalidArgument, status.Code(err), name)
		}
		other := putSecretSpec(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings", map[string]any{"dest": "x"})
		other.GetPutSecretDocument().ProtocolJson = row(map[string]any{})
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "secrets.put:5:reality-with-row", Operation: other})
		require.Equal(t, codes.InvalidArgument, status.Code(err), "only with settings")
	})
}

func mustJSONString(t *testing.T, value any) string {
	t.Helper()
	return string(mustJSON(t, value))
}
