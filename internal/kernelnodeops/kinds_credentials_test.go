package kernelnodeops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// The field list of these tests: the listed routes the credential kinds
// serve, and a route that answers an agent enrollment credential, which
// no listed route does yet.
const testSecretFields = `{"format":"anixops.node-secret-fields/v1","routes":[
 {"route_id":"proxy.admin.nodes.post","target":{"kind":"proxy","new":true},"request":[{"pointer":"/raw_config","kind":"document"}],"answer":[{"pointer":"/data/api_key","name":"api_key"},{"pointer":"/data/secret","name":"secret"}]},
 {"route_id":"proxy.admin.nodes.id.raw_config.put","target":{"kind":"proxy","path_param":"id"},"request":[{"pointer":"/raw_config","kind":"document"}]},
 {"route_id":"forward.admin.forward.nodes.id.put","target":{"kind":"forward","path_param":"id"},"request":[{"pointer":"/api_token","kind":"value"}]},
 {"route_id":"forward.admin.forward.nodes.post","target":{"kind":"forward","new":true},"request":[{"pointer":"/api_token","kind":"value"}],"answer":[{"pointer":"/data/api_token","name":"api_token"}]},
 {"route_id":"proxy.admin.auth_keys.post","target":{"kind":"registration_key","new":true},"answer":[{"pointer":"/data/key","name":"key"}]},
 {"route_id":"forward.admin.forward.agents.post","target":{"kind":"clean_agent","new":true},"answer":[{"pointer":"/data/token","name":"token"}]},
 {"route_id":"protocol.admin.nodes.id.protocols.protocol_id.put","target":{"kind":"protocol","path_param":"protocol_id"},"request":[{"pointer":"/settings","kind":"document"},{"pointer":"/reality_settings","kind":"document"}]},
 {"route_id":"test.agents.enroll","target":{"kind":"proxy","path_param":"id"},"answer":[{"pointer":"/data/credential","name":"credential"}]}
]}`

// bindings seals requests as the gateway does and answers the kernel's
// binding verification for them (the boundRequest seam).
type bindings struct {
	t      *testing.T
	store  *sealedsecrets.Store
	sealer *sealedsecrets.Sealer
	mu     sync.Mutex
	live   map[string]bound
	count  int
}

type bound struct {
	host    packagebridge.HostIdentity
	request packagebridge.Request
}

// boundRequestSeam is one process-wide seam for these tests, which run in
// sequence.
var (
	seamMu     sync.Mutex
	seamActive *bindings
)

func newBindings(t *testing.T) *bindings {
	t.Helper()
	table, err := sealedsecrets.ParseTable([]byte(testSecretFields))
	require.NoError(t, err)
	store := sealedsecrets.NewStore(nil)
	b := &bindings{t: t, store: store, sealer: sealedsecrets.NewSealer(table, nil, store), live: map[string]bound{}}
	seamMu.Lock()
	previous := boundRequest
	seamActive = b
	boundRequest = func(_ context.Context, raw []byte) (packagebridge.HostIdentity, packagebridge.Request, error) {
		seamMu.Lock()
		active := seamActive
		seamMu.Unlock()
		if active == nil {
			return packagebridge.HostIdentity{}, packagebridge.Request{}, packagebridge.ErrBindingRejected
		}
		active.mu.Lock()
		defer active.mu.Unlock()
		entry, ok := active.live[string(raw)]
		if !ok {
			return packagebridge.HostIdentity{}, packagebridge.Request{}, packagebridge.ErrBindingRejected
		}
		return entry.host, entry.request, nil
	}
	seamMu.Unlock()
	t.Cleanup(func() {
		seamMu.Lock()
		boundRequest = previous
		seamActive = nil
		seamMu.Unlock()
	})
	return b
}

// request is one administrator request a package serves: its binding
// (the bridge capability), the sealed body the package reads, and the key
// its handles live under.
type request struct {
	capability []byte
	body       []byte
	key        string
	routeID    string
	sealed     map[string]any
}

// open seals a request of host on routeID and binds it for the kernel.
func (b *bindings) open(host packagebridge.HostIdentity, routeID string, params map[string]string, body string) *request {
	b.t.Helper()
	b.mu.Lock()
	b.count++
	requestID := "req-" + strconv.Itoa(b.count)
	b.mu.Unlock()
	deadline := time.Now().Add(time.Minute)
	sealed, err := b.sealer.SealRequest(sealedsecrets.RequestInput{
		PackageID: host.PackageID, Generation: host.Generation, RequestID: requestID, RouteID: routeID,
		Deadline: deadline, Body: []byte(body), PathParams: params,
	})
	require.NoError(b.t, err)
	capability := make([]byte, 32)
	_, err = rand.Read(capability)
	require.NoError(b.t, err)
	principal, err := json.Marshal(map[string]any{"actor_id": 1, "admin": true, "package_id": host.PackageID})
	require.NoError(b.t, err)
	b.mu.Lock()
	b.live[string(capability)] = bound{host: host, request: packagebridge.Request{
		RequestID: requestID, RouteID: routeID, Method: "POST", Body: []byte(body), PrincipalJSON: principal,
		Deadline: deadline, SealedRequest: sealed.Key,
	}}
	b.mu.Unlock()
	r := &request{capability: capability, body: sealed.Body, key: sealed.Key, routeID: routeID}
	if len(sealed.Body) > 0 {
		require.NoError(b.t, json.Unmarshal(sealed.Body, &r.sealed))
	}
	b.t.Cleanup(func() { b.sealer.Release(sealed.Key) })
	return r
}

// handle returns the handle sealed at a top-level field of the body.
func (r *request) handle(field string) string {
	value, _ := r.sealed[field].(string)
	return value
}

// binding is the RequestBinding a package passes.
func (r *request) binding() *kernelnodeopsv1.RequestBinding {
	return &kernelnodeopsv1.RequestBinding{BridgeCapability: r.capability}
}

// shown expands the handles of reveal in the request's answer, as the
// gateway does, and returns each secret by its name.
func (b *bindings) shown(r *request, reveal []*kernelnodeopsv1.SecretHandle) map[string]string {
	b.t.Helper()
	data := map[string]any{}
	for _, handle := range reveal {
		require.True(b.t, sealedsecrets.IsHandle(handle.GetHandle()), "a handle, not a value")
		require.True(b.t, b.store.Live(handle.GetHandle()))
		data[handle.GetField()] = handle.GetHandle()
	}
	answer, err := json.Marshal(map[string]any{"code": 0, "data": data})
	require.NoError(b.t, err)
	expanded, count, err := b.sealer.ExpandAnswer(r.key, r.routeID, answer, nil)
	require.NoError(b.t, err)
	require.Equal(b.t, len(reveal), count, "every handle expanded in the bound answer")
	var decoded struct {
		Data map[string]string `json:"data"`
	}
	require.NoError(b.t, json.Unmarshal(expanded, &decoded))
	return decoded.Data
}

// submitBound submits an operation bound to r and waits for its end.
func submitBound(t *testing.T, client kernelnodeopsv1.KernelNodeOpsServer, requestID string, spec *kernelnodeopsv1.OperationSpec, r *request) (*kernelnodeopsv1.Operation, bool) {
	t.Helper()
	submit := &kernelnodeopsv1.SubmitOperationRequest{RequestId: requestID, Operation: spec, Wait: kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL, WaitTimeoutMs: 20000}
	if r != nil {
		submit.Request = r.binding()
	}
	response, err := client.SubmitOperation(context.Background(), submit)
	require.NoError(t, err)
	return response.GetOperation(), response.GetApplied()
}

// credentialHarness is an engine serving every NO-5 kind on the seeded
// database with the split tables, the agent PKI tables and a test CA.
type credentialHarness struct {
	*harness
	bindings  *bindings
	authority *modulepki.Authority
}

var credentialModels = []any{
	&model.NodeCredential{}, &model.ProtocolSecret{}, &model.NodeSecretSplit{}, &model.AgentEnrollment{}, &model.AgentCertificate{},
	&model.OperationLog{}, &model.ServiceCA{}, &model.WireGuardPeer{}, &model.SubscriptionGroup{}, &model.SystemConfig{},
}

func newCredentialHarness(t *testing.T, db *gorm.DB) *credentialHarness {
	t.Helper()
	require.NoError(t, db.AutoMigrate(credentialModels...))
	require.NoError(t, nodesecrets.EnsureSchema(db))
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "test", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(context.Background()))
	b := newBindings(t)
	h := newHarness(t, db, func(e *Engine) { e.Secrets = b.store })
	credentials := &Credentials{PKI: func(_ context.Context, tx *gorm.DB) (*agentpki.Service, error) {
		return agentpki.New(agentpki.Options{DB: tx, Authority: authority})
	}}
	require.NoError(t, credentials.Register(h.registry))
	require.NoError(t, (&Retirements{}).Register(h.registry))
	require.NoError(t, (&SecretDocuments{}).Register(h.registry))
	h.start(t)
	return &credentialHarness{harness: h, bindings: b, authority: authority}
}

// ledgerText is everything the ledger holds: operations, events, targets.
func ledgerText(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var text strings.Builder
	for _, rows := range []any{&[]model.KernelNodeOperation{}, &[]model.KernelNodeOperationEvent{}, &[]model.KernelNodeOperationTarget{}} {
		require.NoError(t, db.Find(rows).Error)
		encoded, err := json.Marshal(rows)
		require.NoError(t, err)
		text.Write(encoded)
	}
	return text.String()
}

// requireNoSecret checks that neither the ledger nor the operation as
// answered holds any of the values, nor a handle.
func requireNoSecret(t *testing.T, db *gorm.DB, operation *kernelnodeopsv1.Operation, values ...string) {
	t.Helper()
	ledger := ledgerText(t, db)
	for _, value := range values {
		if value == "" {
			continue
		}
		assert.NotContains(t, ledger, value, "the ledger never holds a secret")
	}
	assert.False(t, sealedsecrets.ContainsHandle(ledger), "the ledger never holds a handle")
	if operation != nil {
		encoded, err := json.Marshal(operation)
		require.NoError(t, err)
		for _, value := range values {
			if value != "" {
				assert.NotContains(t, string(encoded), value, "the answer never shows a value in clear")
			}
		}
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// credentialRow is a subject's current credential in the split table.
func credentialRow(t *testing.T, db *gorm.DB, subjectKind string, subjectID uint64, kind string) (model.NodeCredential, bool) {
	t.Helper()
	var rows []model.NodeCredential
	require.NoError(t, db.Where("subject_kind = ? AND subject_id = ? AND kind = ? AND status <> ?", subjectKind, subjectID, kind, nodesecrets.StatusRetired).
		Order("version DESC").Find(&rows).Error)
	if len(rows) == 0 {
		return model.NodeCredential{}, false
	}
	return rows[0], true
}

// dualRead moves every split table to dual_read: backfill, verify, phase.
func dualRead(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := nodesecrets.Backfill(ctx, db, nodesecrets.BackfillOptions{Restart: true})
	require.NoError(t, err)
	_, err = nodesecrets.Verify(ctx, db, nodesecrets.VerifyOptions{})
	require.NoError(t, err)
	changes, err := nodesecrets.SetPhase(ctx, db, nodesecrets.PhaseOptions{Phase: nodesecrets.PhaseDualRead, Actor: "test"})
	require.NoError(t, err, "%+v", changes)
	require.Equal(t, nodesecrets.PhaseDualRead, nodesecrets.ReadPhase(db, nodesecrets.TableNode))
}

func issueSpec(subject *kernelnodeopsv1.NodeRef, kind kernelnodeopsv1.CredentialKind, handle string, replace bool) *kernelnodeopsv1.OperationSpec {
	spec := issueCredential(subject, kind, handle)
	spec.GetIssueCredential().Replace = replace
	return spec
}

func revokeSpec(subject *kernelnodeopsv1.NodeRef, kind kernelnodeopsv1.CredentialKind) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RevokeCredential{RevokeCredential: &kernelnodeopsv1.RevokeCredential{Subject: subject, Kind: kind}}}
}

func regKeyIssue(name string, expires int64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueRegistrationKey{IssueRegistrationKey: &kernelnodeopsv1.IssueRegistrationKey{Name: name, ExpiresAtUnix: expires}}}
}

func regKeyRevoke(id uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_RevokeRegistrationKey{RevokeRegistrationKey: &kernelnodeopsv1.RevokeRegistrationKey{KeyId: id}}}
}

func cleanAgentIssue(name string, forwardNode uint64) *kernelnodeopsv1.OperationSpec {
	return &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueCleanAgent{IssueCleanAgent: &kernelnodeopsv1.IssueCleanAgent{Name: name, ForwardNodeId: forwardNode}}}
}

func TestTheKernelServesTheCredentialSecretAndRetirementKinds(t *testing.T) {
	served := map[string]kernelnodeopsv1.OperationFamily{
		KindCredentialIssue: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS, KindCredentialRevoke: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS,
		KindRegKeyIssue: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS, KindRegKeyRevoke: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS,
		KindCleanAgentIssue: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS,
		KindNodeRetire:      kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG, KindProtocolRetire: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG,
		KindSecretsPut: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG,
	}
	for kindName, family := range served {
		require.Contains(t, DefaultExecutors.Kinds(), kindName)
		require.Equal(t, family, FamilyOfKind(kindName))
		_, isPreparer := func() (Executor, bool) {
			e, _ := DefaultExecutors.Lookup(kindName)
			_, ok := e.(Preparer)
			return e, ok
		}()
		switch kindName {
		case KindCredentialIssue, KindRegKeyIssue, KindCleanAgentIssue, KindSecretsPut:
			require.True(t, isPreparer, "%s acts in the submitting call", kindName)
		default:
			require.False(t, isPreparer, kindName)
		}
	}
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		engine := &Engine{DB: db, Executors: DefaultExecutors}
		capabilities, err := (&Server{Engine: engine, Authorizer: allow(service.CapabilityNodeOpsCredentials)}).For(proxyHost).
			GetCapabilities(context.Background(), &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		for kindName := range served {
			require.Contains(t, capabilities.GetKinds(), kindName)
		}
		registry := NewRegistry()
		require.NoError(t, (&Credentials{}).Register(registry))
		require.Error(t, (&Credentials{}).Register(registry), "a kind is served once")
	})
}

// A proxy node's credentials are generated by the kernel and shown once,
// as handles, in the bound answer: the administrator sees the key and
// secret the node row and the split table hold, and nothing else does.
func TestIssueProxyNodeCredentials(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(proxyHost, allFamilies())
		created := h.bindings.open(proxyHost, "proxy.admin.nodes.post", nil, `{"name":"edge"}`)

		// Node 1 holds a key (seeded): issuing both without replace is
		// refused in the submitting call, and nothing is recorded.
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:proxy-1:a", Operation: issueSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", false), Request: created.binding(),
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		require.Empty(t, h.rows(t))

		// Without a binding nobody would be shown the secret: refused.
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:proxy-1:b", Operation: issueSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", true),
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Empty(t, h.rows(t))

		// The unknown node is NOT_FOUND.
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:proxy-9", Operation: issueSpec(nodeRef(proxyKind, 9), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", true), Request: created.binding(),
		})
		require.Equal(t, codes.NotFound, status.Code(err))

		operation, applied := submitBound(t, client, "credential.issue:proxy-1:c", issueSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", true), created)
		require.True(t, applied)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Equal(t, kernelnodeopsv1.Channel_CHANNEL_KERNEL, operation.GetChannel())
		result := operation.GetResult().GetCredential()
		require.Equal(t, uint64(1), result.GetSubject().GetId())
		require.Len(t, result.GetReveal(), 2)
		shown := h.bindings.shown(created, result.GetReveal())
		var node model.Node
		require.NoError(t, db.First(&node, 1).Error)
		require.Len(t, shown["api_key"], 64)
		require.Equal(t, node.APIKey, shown["api_key"], "the administrator is shown the key the row holds")
		require.Equal(t, node.Secret, shown["secret"])
		require.Equal(t, sha256Hex(node.APIKey), node.APIKeyHash)
		key, ok := credentialRow(t, db, nodesecrets.SubjectProxy, 1, nodesecrets.KindNodeAPIKey)
		require.True(t, ok)
		require.Equal(t, node.APIKey, key.Value, "the split table agrees")
		require.Equal(t, node.APIKeyHash, key.KeyHash)
		require.Equal(t, 1, key.Version, "the seeded key was never dual-written: the first split row")
		require.Equal(t, key.ID, result.GetCredentialId())
		require.EqualValues(t, 1, result.GetVersion())
		secret, ok := credentialRow(t, db, nodesecrets.SubjectProxy, 1, nodesecrets.KindNodeSharedSecret)
		require.True(t, ok)
		require.Equal(t, node.Secret, secret.Value)
		requireNoSecret(t, db, nil, node.APIKey, node.Secret, "node-key-1")
		read := get(t, client, operation.GetOperationId())()
		for _, handle := range read.GetResult().GetCredential().GetReveal() {
			assert.Empty(t, handle.GetHandle(), "a later read answers no handle")
			assert.NotEmpty(t, handle.GetField())
		}

		// The same request id again is the first operation, applied once:
		// the node keeps its credentials, and the handles were taken.
		again, applied := submitBound(t, client, "credential.issue:proxy-1:c", issueSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", true), created)
		require.False(t, applied)
		require.Equal(t, operation.GetOperationId(), again.GetOperationId())
		var repeated model.Node
		require.NoError(t, db.First(&repeated, 1).Error)
		require.Equal(t, node.APIKey, repeated.APIKey)
		require.Len(t, h.rows(t), 1)
	})
}

// A forward node's token is what the administrator typed (a handle sealed
// from the request) or what the kernel generated; replacing one revokes
// the node's agent certificates, as the forward node update does.
func TestIssueForwardNodeToken(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(forwardHost, allFamilies())
		require.NoError(t, db.Create(&model.AgentCertificate{Serial: "cert-10", NodeKind: "forward", NodeID: 10, Cluster: "test", EnrollmentID: "e", IssuerKeyID: "k", NotAfter: time.Now().Add(time.Hour)}).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = ?", 10).Updates(map[string]any{"api_port": 18080}).Error)

		typed := h.bindings.open(forwardHost, "forward.admin.forward.nodes.id.put", map[string]string{"id": "10"}, `{"name":"relay","api_token":"typed-token-1"}`)
		handle := typed.handle("api_token")
		require.True(t, sealedsecrets.IsHandle(handle), "the package reads a handle")

		// The handle sealed for node 10 does not store for node 11.
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:forward-11:token", Request: typed.binding(),
			Operation: issueSpec(nodeRef(forwardKind, 11), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, handle, false),
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.NotContains(t, err.Error(), handle)
		var other model.ForwardNode
		require.NoError(t, db.First(&other, 11).Error)
		require.Empty(t, other.APIToken)
		// A handle that is none, or another request's binding, is refused.
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:forward-10:bad", Request: typed.binding(),
			Operation: issueSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, liveHandle, false),
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Empty(t, h.rows(t), "nothing recorded")

		operation, _ := submitBound(t, client, "credential.issue:forward-10:token", issueSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, handle, false), typed)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Empty(t, operation.GetResult().GetCredential().GetReveal(), "a typed secret is never echoed")
		var node model.ForwardNode
		require.NoError(t, db.First(&node, 10).Error)
		require.Equal(t, "typed-token-1", node.APIToken)
		row, ok := credentialRow(t, db, nodesecrets.SubjectForward, 10, nodesecrets.KindForwardNodeToken)
		require.True(t, ok)
		require.Equal(t, "typed-token-1", row.Value)
		require.Equal(t, "198.51.100.10:18080", row.Endpoint, "the token is pinned to the node's endpoint")
		require.Equal(t, row.ID, operation.GetResult().GetCredential().GetCredentialId())
		var certificate model.AgentCertificate
		require.NoError(t, db.First(&certificate, "serial = ?", "cert-10").Error)
		require.Nil(t, certificate.RevokedAt, "a first token revokes nothing")
		requireNoSecret(t, db, operation, "typed-token-1")

		// The handle resolved once: a second operation with it is refused.
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:forward-10:token:again", Request: typed.binding(),
			Operation: issueSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, handle, true),
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))

		// A rotation without replace is refused; with it the kernel
		// generates a token, shows it once and revokes the certificates.
		rotation := h.bindings.open(forwardHost, "forward.admin.forward.nodes.post", nil, `{"name":"relay"}`)
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:forward-10:rotate:a", Request: rotation.binding(),
			Operation: issueSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, "", false),
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		operation, _ = submitBound(t, client, "credential.issue:forward-10:rotate:b", issueSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", true), rotation)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		shown := h.bindings.shown(rotation, operation.GetResult().GetCredential().GetReveal())
		require.NoError(t, db.First(&node, 10).Error)
		require.Len(t, node.APIToken, 32)
		require.Equal(t, node.APIToken, shown["api_token"])
		row, ok = credentialRow(t, db, nodesecrets.SubjectForward, 10, nodesecrets.KindForwardNodeToken)
		require.True(t, ok)
		require.Equal(t, node.APIToken, row.Value)
		require.Equal(t, 2, row.Version)
		require.NoError(t, db.First(&certificate, "serial = ?", "cert-10").Error)
		require.NotNil(t, certificate.RevokedAt, "replacing the token revokes the agent's certificate")
		require.Equal(t, agentpki.RevokeReasonCredentialsReplaced, certificate.RevokeReason)
		requireNoSecret(t, db, nil, "typed-token-1", node.APIToken)
	})
}

// An agent enrollment credential is minted through internal/agentpki, with
// its audit entry, and shown as a handle; a route whose answer shows none
// gets nothing issued.
func TestIssueAgentEnrollmentCredential(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(proxyHost, allFamilies())
		enroll := h.bindings.open(proxyHost, "test.agents.enroll", map[string]string{"id": "1"}, `{}`)
		spec := issueSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT, "", false)
		operation, _ := submitBound(t, client, "credential.issue:proxy-1:enroll", spec, enroll)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		shown := h.bindings.shown(enroll, operation.GetResult().GetCredential().GetReveal())
		credential := shown["credential"]
		require.True(t, strings.HasPrefix(credential, "anixagt_"), "an anixagt_ credential")
		var enrollment model.AgentEnrollment
		require.NoError(t, db.Where("node_kind = ? AND node_id = ?", "proxy", 1).First(&enrollment).Error)
		require.NotNil(t, enrollment.CredentialHash)
		require.Equal(t, sha256Hex(credential), *enrollment.CredentialHash, "stored hashed")
		var audits []model.OperationLog
		require.NoError(t, db.Where("action = ?", agentpki.AuditActionTokenIssue).Find(&audits).Error)
		require.Len(t, audits, 1, "the legacy audit entry")
		require.Contains(t, audits[0].Username, AgentEnrollmentActor)
		require.NotContains(t, audits[0].Content, credential)
		requireNoSecret(t, db, nil, credential)

		// No listed route of today's field list answers an enrollment
		// credential: nothing is issued for one.
		created := h.bindings.open(proxyHost, "proxy.admin.nodes.post", nil, `{"name":"edge"}`)
		operation, _ = submitBound(t, client, "credential.issue:proxy-2:enroll", issueSpec(nodeRef(proxyKind, 2), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT, "", false), created)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_SECRET_HANDLE_INVALID, operation.GetError().GetCode())
		var count int64
		require.NoError(t, db.Model(&model.AgentEnrollment{}).Where("node_id = ?", 2).Count(&count).Error)
		require.Zero(t, count, "rolled back")
		require.NoError(t, db.Model(&model.OperationLog{}).Count(&count).Error)
		require.EqualValues(t, 1, count, "no audit entry for what was not issued")

		// Revoking the enrollments revokes the credential.
		operation, _ = submitBound(t, client, "credential.revoke:proxy-1:enroll", revokeSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.NoError(t, db.First(&enrollment, "id = ?", enrollment.ID).Error)
		require.NotNil(t, enrollment.RevokedAt)
		require.Equal(t, agentpki.RevokeReasonCredentialsRevoked, enrollment.RevokeReason)
	})
}

// Registration keys: issued as the auth key routes issue them, shown once,
// revoked with their split row.
func TestRegistrationKeys(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(proxyHost, allFamilies())
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "regkey.issue:unbound", Operation: regKeyIssue("bootstrap", 0)})
		require.Equal(t, codes.PermissionDenied, status.Code(err), "a key is issued in an administrator's request")
		require.Empty(t, h.rows(t))

		created := h.bindings.open(proxyHost, "proxy.admin.auth_keys.post", nil, `{"name":"bootstrap"}`)
		expires := time.Now().Add(24 * time.Hour).Unix()
		operation, applied := submitBound(t, client, "regkey.issue:1", regKeyIssue("bootstrap", expires), created)
		require.True(t, applied)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		result := operation.GetResult().GetRegistrationKey()
		require.Equal(t, "bootstrap", result.GetName())
		require.Equal(t, expires, result.GetExpiresAtUnix())
		shown := h.bindings.shown(created, result.GetReveal())
		var key model.AuthorizedKey
		require.NoError(t, db.First(&key, result.GetKeyId()).Error)
		require.Len(t, key.Key, 64)
		require.Equal(t, key.Key, shown["key"])
		require.Equal(t, sha256Hex(key.Key), key.KeyHash)
		require.NotNil(t, key.ExpireAt)
		require.Equal(t, expires, *key.ExpireAt)
		row, ok := credentialRow(t, db, nodesecrets.SubjectRegistrationKey, uint64(key.ID), nodesecrets.KindRegistrationKey)
		require.True(t, ok)
		require.Equal(t, key.Key, row.Value)
		requireNoSecret(t, db, operation, key.Key)

		again, applied := submitBound(t, client, "regkey.issue:1", regKeyIssue("bootstrap", expires), created)
		require.False(t, applied, "a repeat issues no second key")
		require.Equal(t, operation.GetOperationId(), again.GetOperationId())
		var keys int64
		require.NoError(t, db.Model(&model.AuthorizedKey{}).Count(&keys).Error)
		require.EqualValues(t, 2, keys, "the seeded key and the issued one")

		// Revocation needs no binding; a key gone since is nothing to do.
		operation, _ = submitBound(t, client, "regkey.revoke:"+strconv.FormatUint(result.GetKeyId(), 10), regKeyRevoke(result.GetKeyId()), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.Equal(t, "bootstrap", operation.GetResult().GetRegistrationKey().GetName())
		require.NoError(t, db.Model(&model.AuthorizedKey{}).Where("id = ?", result.GetKeyId()).Count(&keys).Error)
		require.Zero(t, keys)
		_, ok = credentialRow(t, db, nodesecrets.SubjectRegistrationKey, result.GetKeyId(), nodesecrets.KindRegistrationKey)
		require.False(t, ok, "the split row went with it")
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "regkey.revoke:9", Operation: regKeyRevoke(9)})
		require.Equal(t, codes.NotFound, status.Code(err), "an unknown key")
		operation, applied = submitBound(t, client, "regkey.revoke:"+strconv.FormatUint(result.GetKeyId(), 10), regKeyRevoke(result.GetKeyId()), nil)
		require.False(t, applied)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState())
	})
}

// Clean agents: created as the administrator's agent creation creates
// them, the token shown once; revoked as the revocation route revokes
// them. A node's key or token is never revoked in place.
func TestCleanAgentsAndRevocation(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(forwardHost, allFamilies())
		created := h.bindings.open(forwardHost, "forward.admin.forward.agents.post", nil, `{"name":"relay-agent","nodeId":10}`)
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "cleanagent.issue:unbound", Operation: cleanAgentIssue("relay-agent", 10)})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "cleanagent.issue:gone", Operation: cleanAgentIssue("relay-agent", 99), Request: created.binding()})
		require.Equal(t, codes.NotFound, status.Code(err))

		operation, _ := submitBound(t, client, "cleanagent.issue:1", cleanAgentIssue("relay-agent", 10), created)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		result := operation.GetResult().GetCredential()
		require.Equal(t, kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, result.GetSubject().GetKind())
		require.Equal(t, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_CLEAN_AGENT_TOKEN, result.GetKind())
		shown := h.bindings.shown(created, result.GetReveal())
		var agent model.ForwardCleanAgent
		require.NoError(t, db.First(&agent, result.GetSubject().GetId()).Error)
		require.Equal(t, "relay-agent", agent.Name)
		require.Equal(t, uint(10), *agent.NodeID)
		require.True(t, strings.HasPrefix(agent.Token, "v2fa_"))
		require.Equal(t, agent.Token, shown["token"])
		row, ok := credentialRow(t, db, nodesecrets.SubjectCleanAgent, uint64(agent.ID), nodesecrets.KindCleanAgentToken)
		require.True(t, ok)
		require.Equal(t, agent.Token, row.Value)
		require.Equal(t, row.ID, result.GetCredentialId())
		requireNoSecret(t, db, operation, agent.Token)

		// Revoke it, as the route does; the split row follows.
		subject := nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, uint64(agent.ID))
		operation, _ = submitBound(t, client, "credential.revoke:clean-agent", revokeSpec(subject, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
		require.NoError(t, db.First(&agent, agent.ID).Error)
		require.Equal(t, model.ForwardCleanAgentStatusRevoked, agent.Status)
		require.NotNil(t, agent.RevokedAt)
		row, ok = credentialRow(t, db, nodesecrets.SubjectCleanAgent, uint64(agent.ID), nodesecrets.KindCleanAgentToken)
		require.True(t, ok)
		require.Equal(t, nodesecrets.StatusRevoked, row.Status)
		again, applied := submitBound(t, client, "credential.revoke:clean-agent", revokeSpec(subject, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED), nil)
		require.False(t, applied)
		require.Equal(t, operation.GetOperationId(), again.GetOperationId())

		// A clean agent's token is not issued with IssueCredential, and a
		// node's token is not revoked in place.
		operation, _ = submitBound(t, client, "credential.issue:clean-agent", issueSpec(subject, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_CLEAN_AGENT_TOKEN, "", true), created)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, operation.GetError().GetCode())
		operation, _ = submitBound(t, client, "credential.revoke:forward-10", revokeSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN), nil)
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, operation.GetState())
		require.Equal(t, kernelnodeopsv1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, operation.GetError().GetCode())
		require.Contains(t, operation.GetError().GetMessage(), "rotate")
		// A credential the subject's kind does not hold is refused by the
		// kind's check.
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "credential.revoke:wrong", Operation: revokeSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY)})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// Every issuing executor leaves the legacy column and the split row in
// agreement, in dual_write and in dual_read.
func TestIssuedCredentialsAgreeInBothPhases(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		for round, phase := range []string{nodesecrets.PhaseDualWrite, nodesecrets.PhaseDualRead} {
			if phase == nodesecrets.PhaseDualRead {
				dualRead(t, db)
			}
			suffix := ":" + phase
			proxy := h.client(proxyHost, allFamilies())
			forward := h.client(forwardHost, allFamilies())
			proxyID := uint64(round + 1)
			forwardID := uint64(10 + round)

			created := h.bindings.open(proxyHost, "proxy.admin.nodes.post", nil, `{}`)
			operation, _ := submitBound(t, proxy, "credential.issue:proxy"+suffix, issueSpec(nodeRef(proxyKind, proxyID), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", true), created)
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
			var node model.Node
			require.NoError(t, db.First(&node, proxyID).Error)
			key, ok := credentialRow(t, db, nodesecrets.SubjectProxy, proxyID, nodesecrets.KindNodeAPIKey)
			require.True(t, ok)
			require.Equal(t, node.APIKey, key.Value, phase)
			require.Equal(t, node.APIKeyHash, key.KeyHash, phase)
			require.Equal(t, node.APIKey, nodesecrets.NodeAPIKey(db, &node), "the reader of this phase answers it")
			secret, ok := credentialRow(t, db, nodesecrets.SubjectProxy, proxyID, nodesecrets.KindNodeSharedSecret)
			require.True(t, ok)
			require.Equal(t, node.Secret, secret.Value, phase)

			typed := h.bindings.open(forwardHost, "forward.admin.forward.nodes.id.put", map[string]string{"id": strconv.FormatUint(forwardID, 10)}, `{"api_token":"typed`+suffix+`"}`)
			operation, _ = submitBound(t, forward, "credential.issue:forward"+suffix, issueSpec(nodeRef(forwardKind, forwardID), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, typed.handle("api_token"), true), typed)
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
			var forwardNode model.ForwardNode
			require.NoError(t, db.First(&forwardNode, forwardID).Error)
			require.Equal(t, "typed"+suffix, forwardNode.APIToken)
			token, ok := credentialRow(t, db, nodesecrets.SubjectForward, forwardID, nodesecrets.KindForwardNodeToken)
			require.True(t, ok)
			require.Equal(t, forwardNode.APIToken, token.Value, phase)
			require.True(t, nodesecrets.ForwardNodeTokenMatches(db, &forwardNode, "typed"+suffix))

			keyRequest := h.bindings.open(proxyHost, "proxy.admin.auth_keys.post", nil, `{}`)
			operation, _ = submitBound(t, proxy, "regkey.issue"+suffix, regKeyIssue("key"+suffix, 0), keyRequest)
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
			var authKey model.AuthorizedKey
			require.NoError(t, db.First(&authKey, operation.GetResult().GetRegistrationKey().GetKeyId()).Error)
			regKey, ok := credentialRow(t, db, nodesecrets.SubjectRegistrationKey, uint64(authKey.ID), nodesecrets.KindRegistrationKey)
			require.True(t, ok)
			require.Equal(t, authKey.Key, regKey.Value, phase)
			require.Equal(t, authKey.KeyHash, regKey.KeyHash, phase)

			agentRequest := h.bindings.open(forwardHost, "forward.admin.forward.agents.post", nil, `{}`)
			operation, _ = submitBound(t, forward, "cleanagent.issue"+suffix, cleanAgentIssue("agent"+suffix, forwardID), agentRequest)
			require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED, operation.GetState(), operation.GetError())
			var agent model.ForwardCleanAgent
			require.NoError(t, db.First(&agent, operation.GetResult().GetCredential().GetSubject().GetId()).Error)
			agentToken, ok := credentialRow(t, db, nodesecrets.SubjectCleanAgent, uint64(agent.ID), nodesecrets.KindCleanAgentToken)
			require.True(t, ok)
			require.Equal(t, agent.Token, agentToken.Value, phase)
			found, err := nodesecrets.CleanAgentByToken(db, agent.Token)
			require.NoError(t, err)
			require.Equal(t, agent.ID, found.ID)

			// The seeded rows were never dual-written; the backfill copies
			// them and leaves the executors' rows as they are.
			_, err = nodesecrets.Backfill(context.Background(), db, nodesecrets.BackfillOptions{Restart: true})
			require.NoError(t, err)
			_, err = nodesecrets.Verify(context.Background(), db, nodesecrets.VerifyOptions{})
			require.NoError(t, err, "the old and new forms agree after every executor, in %s", phase)
		}
	})
}
