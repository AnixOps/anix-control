package sealedhandles

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// updateNodeSecret and enrollNode are fixture routes (harness_test.go,
// fieldTable).
const (
	updateNodeSecret = "proxy.fixture.nodes.id.secret.put"
	enrollNode       = "proxy.fixture.nodes.id.enroll.post"
)

// secretPut is a fake native PUT /admin/fixture/nodes/:id/secret: it
// stores the token an administrator typed through PutSecretDocument, with
// the handle it was sent. use may change what it submits, as a misbehaving
// package would.
func secretPut(h *harness, use func(request pluginhostsdk.NativeRequest, submit *kernelnodeopsv1.SubmitOperationRequest)) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		var body struct {
			APIToken string `json:"api_token"`
		}
		if err := json.Unmarshal(request.Body, &body); err != nil {
			return pluginhostsdk.NativeResponse{}, err
		}
		id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 64)
		if err != nil {
			return pluginhostsdk.NativeResponse{}, err
		}
		submit := &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: h.requestID("secrets.put:proxy-" + strconv.FormatUint(id, 10)),
			Operation: secretDocument(id, body.APIToken),
			Wait:      kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: request.Binding},
		}
		if use != nil {
			use(request, submit)
		}
		if _, err := h.nodeOps("proxy-node").SubmitOperation(ctx, submit); err != nil {
			return refusal(err)
		}
		return panel(map[string]any{"id": id, "api_token": service.NodeSecretPlaceholder})
	}
}

// secretDocument is a PutSecretDocument of node id's raw configuration
// holding handle as its token.
func secretDocument(id uint64, handle string) *kernelnodeopsv1.OperationSpec {
	document, _ := json.Marshal(map[string]string{"token": handle})
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_PutSecretDocument{PutSecretDocument: &kernelnodeopsv1.PutSecretDocument{
		Scope: kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG, OwnerId: id, Column: "raw_config", DocumentJson: document,
	}}}
}

// enrollment is a fake native POST /admin/fixture/nodes/:id/enroll: the
// kernel generates the node's Agent enrollment credential and the answer
// shows it, as a handle.
func enrollment(h *harness, answer func(reveal []*kernelnodeopsv1.SecretHandle) map[string]any) pluginhostsdk.NativeHandler {
	return func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		submitted, err := h.nodeOps("proxy-node").SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: h.requestID("credential.issue:proxy-3"),
			Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueCredential{IssueCredential: &kernelnodeopsv1.IssueCredential{
				Subject: &kernelnodeopsv1.NodeRef{Kind: kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, Id: proxyNodeID},
				Kind:    kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT,
			}}},
			Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: request.Binding},
		})
		if err != nil {
			return refusal(err)
		}
		reveal := submitted.GetOperation().GetResult().GetCredential().GetReveal()
		if answer != nil {
			return panel(answer(reveal))
		}
		data := map[string]any{"node_id": proxyNodeID}
		for _, handle := range reveal {
			data[handle.GetField()] = handle.GetHandle()
		}
		return panel(data)
	}
}

func decodePanel(t *testing.T, body string) (int, string, map[string]any) {
	t.Helper()
	var answer struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &answer), body)
	return answer.Code, answer.Msg, answer.Data
}

// A secret an administrator types reaches the kernel only: the package
// handles a handle, the kernel resolves it for the bound request, and
// nothing of either is in the ledger.
func TestATypedSecretReachesOnlyTheKernel(t *testing.T) {
	h := newHarness(t)
	h.route(updateNodeSecret, secretPut(h, nil))
	h.start(map[string]string{updateNodeSecret: pluginhostsdk.RouteModeNative})

	recorder := h.do(http.MethodPut, "/api/v2/admin/fixture/nodes/7/secret", `{"name":"relay","api_token":"typed-token-1"}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	code, _, data := decodePanel(t, recorder.Body.String())
	require.Zero(t, code, recorder.Body.String())
	assert.Equal(t, service.NodeSecretPlaceholder, data["api_token"])

	require.Len(t, h.received[updateNodeSecret], 1)
	received := h.received[updateNodeSecret][0]
	assert.NotContains(t, string(received.Body), "typed-token-1", "a secret never reaches the package")
	assert.Contains(t, string(received.Body), sealedsecrets.Prefix)
	assert.Equal(t, "typed-token-1", h.resolved["proxy-7"], "the kernel resolved it for the bound request")
	assert.Empty(t, h.legacy[updateNodeSecret], "native mode")
	ledger := h.ledgerText()
	assert.NotContains(t, ledger, "typed-token-1")
	assert.False(t, sealedsecrets.ContainsHandle(ledger), "the ledger never holds a handle")
	assert.Zero(t, h.store.Pending(), "handles die with their request")
}

// A handle resolves only for its own request, route, target and field;
// a package that tries anything else is refused and stores nothing.
func TestHandlesAreRefusedOutsideTheirBinding(t *testing.T) {
	h := newHarness(t)
	var stashed pluginhostsdk.NativeRequest
	misuse := ""
	h.route(updateNodeSecret, secretPut(h, func(request pluginhostsdk.NativeRequest, submit *kernelnodeopsv1.SubmitOperationRequest) {
		put := submit.GetOperation().GetPutSecretDocument()
		switch misuse {
		case "":
			stashed = request
		case "another target":
			put.OwnerId = secretNodeID + 1
		case "another request's handle":
			var body struct {
				APIToken string `json:"api_token"`
			}
			require.NoError(t, json.Unmarshal(stashed.Body, &body))
			submit.Operation = secretDocument(put.GetOwnerId(), body.APIToken)
		case "another request's binding":
			submit.Request.BridgeCapability = stashed.Binding
		case "another field":
			var body struct {
				Nested struct {
					Secret string `json:"secret"`
				} `json:"nested"`
			}
			require.NoError(t, json.Unmarshal(request.Body, &body))
			submit.Operation = secretDocument(put.GetOwnerId(), body.Nested.Secret)
		case "no binding":
			submit.Request = nil
		case "used twice":
			if _, err := h.nodeOps("proxy-node").SubmitOperation(context.Background(), proto(submit, h.requestID("first"))); err != nil {
				t.Errorf("the first use failed: %v", err)
			}
		}
	}))
	h.start(map[string]string{updateNodeSecret: pluginhostsdk.RouteModeNative})

	recorder := h.do(http.MethodPut, "/api/v2/admin/fixture/nodes/7/secret", `{"api_token":"first-token"}`)
	_, msg, _ := decodePanel(t, recorder.Body.String())
	require.Equal(t, "操作成功", msg)

	for _, name := range []string{"another target", "another request's handle", "another request's binding", "another field", "no binding", "used twice"} {
		misuse = name
		delete(h.resolved, "proxy-8")
		token := "token-" + strings.ReplaceAll(name, " ", "-")
		recorder := h.do(http.MethodPut, "/api/v2/admin/fixture/nodes/7/secret", `{"api_token":"`+token+`","nested":{"secret":"nested-secret"}}`)
		code, msg, _ := decodePanel(t, recorder.Body.String())
		require.Equal(t, -1, code, name)
		assert.Equal(t, codes.PermissionDenied.String(), msg, name)
		assert.Empty(t, h.resolved["proxy-8"], "%s: nothing was stored for another target", name)
		assert.NotContains(t, recorder.Body.String(), sealedsecrets.Prefix, name)
		want := "first-token"
		if name == "used twice" {
			want = "token-used-twice"
		}
		assert.Equal(t, want, h.resolved["proxy-7"], "%s: only bound uses resolved", name)
	}
	assert.Zero(t, h.store.Pending())
}

// proto copies a submission under another request id.
func proto(submit *kernelnodeopsv1.SubmitOperationRequest, requestID string) *kernelnodeopsv1.SubmitOperationRequest {
	return &kernelnodeopsv1.SubmitOperationRequest{RequestId: requestID, Operation: submit.GetOperation(), Wait: submit.GetWait(), Request: submit.GetRequest()}
}

// A secret the kernel generates is shown once, in the answer to the request
// it was generated for: the package handles handles, the ledger keeps none,
// and a handle in any other answer is refused.
func TestGeneratedSecretsAreShownOnlyInTheBoundAnswer(t *testing.T) {
	h := newHarness(t)
	var earlier []*kernelnodeopsv1.SecretHandle
	replay := false
	h.route(enrollNode, enrollment(h, func(reveal []*kernelnodeopsv1.SecretHandle) map[string]any {
		data := map[string]any{"node_id": proxyNodeID}
		shown := reveal
		if replay {
			shown = earlier
		}
		earlier = reveal
		for _, handle := range shown {
			data[handle.GetField()] = handle.GetHandle()
		}
		return data
	}))
	h.start(map[string]string{enrollNode: pluginhostsdk.RouteModeNative})

	recorder := h.do(http.MethodPost, "/api/v2/admin/fixture/nodes/3/enroll", `{}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	_, _, data := decodePanel(t, recorder.Body.String())
	assert.Equal(t, "generated-credential-3", data["credential"])
	require.Len(t, earlier, 1)
	for _, handle := range earlier {
		assert.True(t, sealedsecrets.IsHandle(handle.GetHandle()), "the package handled a handle")
	}

	ledger := h.ledgerText()
	assert.NotContains(t, ledger, "generated-")
	assert.False(t, sealedsecrets.ContainsHandle(ledger), "the ledger never holds a handle")
	operations, err := h.nodeOps("proxy-node").ListOperations(context.Background(), &kernelnodeopsv1.ListOperationsRequest{})
	require.NoError(t, err)
	require.Len(t, operations.GetOperations(), 1)
	for _, handle := range operations.GetOperations()[0].GetResult().GetCredential().GetReveal() {
		assert.Empty(t, handle.GetHandle(), "a later read is answered no handle")
		assert.NotEmpty(t, handle.GetField())
	}

	replay = true
	recorder = h.do(http.MethodPost, "/api/v2/admin/fixture/nodes/3/enroll", `{}`)
	require.Equal(t, http.StatusBadGateway, recorder.Code, "another request's handles do not expand")
	assert.NotContains(t, recorder.Body.String(), sealedsecrets.Prefix)
	assert.NotContains(t, recorder.Body.String(), "generated-")
	assert.Zero(t, h.store.Pending())
}

// In legacy mode the host relays: the legacy handler reads the request as
// sent, and the host only ever held handles.
func TestLegacyModeReadsTheOriginalRequest(t *testing.T) {
	h := newHarness(t)
	h.route(updateNodeSecret, secretPut(h, nil))
	h.start(nil)

	recorder := h.do(http.MethodPut, "/api/v2/admin/fixture/nodes/7/secret", `{"api_token":"typed-token-2"}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"legacy":true`)
	assert.Equal(t, []string{`{"api_token":"typed-token-2"}`}, h.legacy[updateNodeSecret])
	require.Len(t, h.dispatched, 1)
	assert.NotContains(t, string(h.dispatched[0].Body), "typed-token-2", "the host is sent handles in every mode")
	assert.Empty(t, h.received[updateNodeSecret])
}

// A request that cannot be sealed is served by the kernel's legacy handler
// without the host, which never sees it.
func TestUnsealableRequestsFallBackToLegacy(t *testing.T) {
	h := newHarness(t)
	h.route(updateNodeSecret, secretPut(h, nil))
	h.start(map[string]string{updateNodeSecret: pluginhostsdk.RouteModeNative})

	body := `{"api_token":["typed-token-3"]}`
	recorder := h.do(http.MethodPut, "/api/v2/admin/fixture/nodes/7/secret", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"legacy":true`)
	assert.Empty(t, h.dispatched, "the host never saw the request")
	assert.Empty(t, h.received[updateNodeSecret])
	assert.Equal(t, []string{body}, h.legacy[updateNodeSecret])
	require.Len(t, h.viaLegacy, 1)
	var rendered strings.Builder
	require.NoError(t, h.metrics.WritePrometheus(&rendered))
	assert.Contains(t, rendered.String(), `route="proxy.fixture.nodes.id.secret.put",stage="request",result="legacy_fallback",reason="value_not_string"} 1`)
}

// A shadow run is never bound: the router gives it no binding, and the
// capability of its request was consumed by the legacy answer, so even a
// package that kept it resolves nothing.
func TestShadowRunsAreNeverBound(t *testing.T) {
	h := newHarness(t)
	type attempt struct {
		binding []byte
		code    codes.Code
	}
	attempts := make(chan attempt, 2)
	h.route("proxy.admin.nodes.id.get", func(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
		h.mu.Lock()
		kept := append([]byte(nil), h.lastMinted...)
		h.mu.Unlock()
		for _, binding := range [][]byte{request.Binding, kept} {
			_, err := h.nodeOps("proxy-node").SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
				RequestId: h.requestID("secrets.put:proxy-3:shadow"),
				Operation: secretDocument(proxyNodeID, sealedsecrets.Prefix+strings.Repeat("A", 43)),
				Request:   &kernelnodeopsv1.RequestBinding{BridgeCapability: binding},
			})
			attempts <- attempt{binding: binding, code: status.Code(err)}
		}
		return panel(map[string]any{"legacy": true})
	})
	h.start(map[string]string{"proxy.admin.nodes.id.get": pluginhostsdk.RouteModeShadow})

	recorder := h.do(http.MethodGet, "/api/v2/admin/nodes/3", "")
	require.Equal(t, http.StatusOK, recorder.Code)
	first, second := <-attempts, <-attempts
	assert.Nil(t, first.binding, "the router binds no shadow run")
	assert.Equal(t, codes.PermissionDenied, first.code, "and a handle resolves only with a binding")
	assert.NotEmpty(t, second.binding)
	assert.Equal(t, codes.PermissionDenied, second.code, "the legacy answer consumed the capability")
	assert.Empty(t, h.resolved)
}

// The walk: through the gateway, the bridge and the router, no value under
// a key IsNodeSecretKey marks, and no listed field's value, reaches a
// package on any listed route, in any document form or depth.
func TestNoNodeSecretReachesAPackageOnAnyListedRoute(t *testing.T) {
	h := newHarness(t)
	table := fieldTable(t)
	ids := table.RouteIDs()
	sort.Strings(ids)
	modes := map[string]string{}
	var walked []string
	for _, id := range ids {
		route, _ := table.Route(id)
		if len(route.Request) == 0 {
			continue
		}
		walked = append(walked, id)
		h.route(id, func(context.Context, pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
			return panel(map[string]any{"ok": true})
		})
		modes[id] = pluginhostsdk.RouteModeNative
	}
	h.start(modes)
	rows := extraction(t)
	keys := []string{"private_key", "privateKey", "server_private_key", "preshared_key", "server_key", "psk", "password", "obfs-password",
		"auth", "seed", "pass", "passwd", "token", "secret", "credential", "api_key", "api_token", "x25519_private_keys"}
	for _, id := range walked {
		route, _ := table.Route(id)
		var values []string
		secrets := func(prefix string) map[string]any {
			document := map[string]any{"public_key": "pub", "nested": map[string]any{}, "peers": []any{map[string]any{}}}
			for index, key := range keys {
				require.True(t, service.IsNodeSecretKey(key), key)
				top, deep, peer := fmt.Sprintf("%s-%d-top", prefix, index), fmt.Sprintf("%s-%d-deep", prefix, index), fmt.Sprintf("%s-%d-peer", prefix, index)
				document[key] = top
				document["nested"].(map[string]any)[key] = deep
				document["peers"].([]any)[0].(map[string]any)[key] = peer
				values = append(values, top, deep, peer)
			}
			return document
		}
		body := secrets(id + "-body")
		for index, field := range route.Request {
			name := strings.TrimPrefix(field.Pointer, "/")
			switch field.Kind {
			case sealedsecrets.FieldValue:
				body[name] = id + "-typed"
				values = append(values, id+"-typed")
			case sealedsecrets.FieldDocument:
				document := secrets(fmt.Sprintf("%s-%s", id, name))
				if index%2 == 0 {
					encoded, err := json.Marshal(document)
					require.NoError(t, err)
					body[name] = string(encoded)
				} else {
					body[name] = document
				}
			}
		}
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		row := rows[id]
		path := strings.NewReplacer(":protocol_id", "9", ":id", strconv.Itoa(proxyNodeID)).Replace(row.Path)
		recorder := h.do(row.Method, path, string(encoded))
		require.Equal(t, http.StatusOK, recorder.Code, "%s: %s", id, recorder.Body.String())
		require.Len(t, h.received[id], 1, id)
		received := h.received[id][0]
		for _, value := range values {
			require.NotContains(t, string(received.Body), value, "%s: a node secret reached the package", id)
			for name, param := range received.Metadata.PathParams {
				require.NotContains(t, param, value, "%s: %s", id, name)
			}
		}
		assert.Contains(t, string(received.Body), `pub`, id)
	}
	assert.Len(t, walked, 7, "the listed request routes and the fixture route")
	assert.Zero(t, h.store.Pending())
	assert.Empty(t, h.viaLegacy, "every walked request was sealed, none fell back")
}
