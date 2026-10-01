package sealedhandles

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realCredentials is a harness whose kernel serves NO-5's credential
// executors, with the split tables.
func realCredentials(t *testing.T) *harness {
	t.Helper()
	h := newHarnessWith(t, func(_ *harness, registry *kernelnodeops.Registry) {
		require.NoError(t, (&kernelnodeops.Credentials{}).Register(registry))
	})
	require.NoError(t, h.db.AutoMigrate(&model.NodeCredential{}, &model.ProtocolSecret{}, &model.NodeSecretSplit{}, &model.AgentEnrollment{}, &model.AgentCertificate{}))
	require.NoError(t, nodesecrets.EnsureSchema(h.db))
	return h
}

// nodeInsertion is a native POST /admin/nodes as proxy-node will serve it
// once it adopts v2_node: it inserts the row with the api_key tombstone
// (section 4.4), has the kernel issue the credentials, and shows them as
// handles.
func nodeInsertion(h *harness) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(request.Body, &body); err != nil {
			return pluginhostsdk.NativeResponse{}, err
		}
		node := &model.Node{Name: body.Name, APIKey: "!moved:" + uuid.NewString()}
		if err := h.db.Create(node).Error; err != nil {
			return pluginhostsdk.NativeResponse{}, err
		}
		submitted, err := h.nodeOps("proxy-node").SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: h.requestID("credential.issue:proxy-" + strconv.FormatUint(uint64(node.ID), 10)),
			Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueCredential{IssueCredential: &kernelnodeopsv1.IssueCredential{
				Subject: &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: uint64(node.ID)},
			}}},
			Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: request.Binding},
		})
		if err != nil {
			return refusal(err)
		}
		if submitted.GetOperation().GetState() != kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED {
			return refusal(context.DeadlineExceeded)
		}
		data := map[string]any{"node_id": node.ID}
		for _, handle := range submitted.GetOperation().GetResult().GetCredential().GetReveal() {
			data[handle.GetField()] = handle.GetHandle()
		}
		return panel(data)
	}
}

// Through the real gateway, bridge and executors: a token an administrator
// types is stored for the forward node it was typed for, and a new proxy
// node's generated key and secret are shown once, in that answer, and are
// what the node row and the split table hold. No secret or handle reaches
// the package host's view of the request, the ledger, or any later answer.
func TestTheCredentialExecutorsRoundTripThroughTheGateway(t *testing.T) {
	h := realCredentials(t)
	h.route(updateForwardNode, tokenUpdate(h, nil))
	h.route(createProxyNode, nodeInsertion(h))
	h.start(map[string]string{updateForwardNode: pluginhostsdk.RouteModeNative, createProxyNode: pluginhostsdk.RouteModeNative})

	recorder := h.do(http.MethodPut, "/api/v2/admin/forward/nodes/7", `{"name":"relay","api_token":"typed-token-e2e"}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	code, msg, data := decodePanel(t, recorder.Body.String())
	require.Zero(t, code, msg)
	assert.Equal(t, service.NodeSecretPlaceholder, data["api_token"])
	var forward model.ForwardNode
	require.NoError(t, h.db.First(&forward, forwardNodeID).Error)
	require.Equal(t, "typed-token-e2e", forward.APIToken, "the kernel stored what the administrator typed")
	var token model.NodeCredential
	require.NoError(t, h.db.Where("subject_kind = ? AND subject_id = ?", nodesecrets.SubjectForward, forwardNodeID).First(&token).Error)
	require.Equal(t, "typed-token-e2e", token.Value)
	received := h.received[updateForwardNode][0]
	assert.NotContains(t, string(received.Body), "typed-token-e2e")
	assert.Contains(t, string(received.Body), sealedsecrets.Prefix)

	recorder = h.do(http.MethodPost, "/api/v2/admin/nodes", `{"name":"edge-e2e"}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	code, msg, data = decodePanel(t, recorder.Body.String())
	require.Zero(t, code, msg)
	var node model.Node
	require.NoError(t, h.db.Where("name = ?", "edge-e2e").First(&node).Error)
	require.Len(t, node.APIKey, 64)
	require.Equal(t, node.APIKey, data["api_key"], "the administrator sees the key the row holds")
	require.Equal(t, node.Secret, data["secret"])
	var key model.NodeCredential
	require.NoError(t, h.db.Where("subject_kind = ? AND subject_id = ? AND kind = ?", nodesecrets.SubjectProxy, node.ID, nodesecrets.KindNodeAPIKey).First(&key).Error)
	require.Equal(t, node.APIKey, key.Value)
	require.Equal(t, node.APIKeyHash, key.KeyHash)

	ledger := h.ledgerText()
	for _, secret := range []string{"typed-token-e2e", node.APIKey, node.Secret} {
		assert.NotContains(t, ledger, secret)
	}
	assert.False(t, sealedsecrets.ContainsHandle(ledger), "the ledger never holds a handle")
	operations, err := h.nodeOps("proxy-node").ListOperations(context.Background(), &kernelnodeopsv1.ListOperationsRequest{})
	require.NoError(t, err)
	require.Len(t, operations.GetOperations(), 1)
	for _, handle := range operations.GetOperations()[0].GetResult().GetCredential().GetReveal() {
		assert.Empty(t, handle.GetHandle(), "a later read answers no handle")
	}
	assert.Zero(t, h.store.Pending(), "handles die with their requests")
}
