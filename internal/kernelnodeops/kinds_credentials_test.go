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
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/AnixOps/anix-control/v5/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
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
	&model.ProtocolSecret{}, &model.NodeSecretSplit{}, &model.AgentEnrollment{}, &model.AgentCertificate{},
	&model.OperationLog{}, &model.ServiceCA{}, &model.WireGuardPeer{}, &model.SubscriptionGroup{}, &model.SystemConfig{},
}

func newCredentialHarness(t *testing.T, db *gorm.DB) *credentialHarness {
	t.Helper()
	require.NoError(t, db.AutoMigrate(credentialModels...))
	require.NoError(t, nodesecrets.MigrateForTest(db))
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
		KindNodeRetire: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG, KindProtocolRetire: kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG,
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
		case KindCredentialIssue, KindCleanAgentIssue, KindSecretsPut:
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

// Registration keys went with node registration: IssueRegistrationKey and
// RevokeRegistrationKey have no executor.
func TestRegistrationKeysWereRemoved(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(proxyHost, allFamilies())
		for requestID, operation := range map[string]*kernelnodeopsv1.OperationSpec{
			"regkey.issue:1": regKeyIssue("bootstrap", 0), "regkey.revoke:1": regKeyRevoke(1),
		} {
			_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: requestID, Operation: operation})
			require.Equal(t, codes.Unimplemented, status.Code(err), requestID)
		}
		require.Empty(t, h.rows(t))
	})
}

// The clean agent went with the flux runtime and node API keys, shared
// secrets and tokens are removed: IssueCleanAgent has no executor, and a
// clean agent's token, a node's key or a forward node's token is neither
// issued nor revoked.
func TestCleanAgentsAndNodeKeysWereRemoved(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newCredentialHarness(t, db)
		client := h.client(forwardHost, allFamilies())
		created := h.bindings.open(forwardHost, "forward.admin.forward.agents.post", nil, `{"name":"relay-agent","nodeId":10}`)
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "cleanagent.issue:1", Operation: cleanAgentIssue("relay-agent", 10), Request: created.binding()})
		require.Equal(t, codes.Unimplemented, status.Code(err))

		cleanAgent := nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, 20)
		for name, spec := range map[string]*kernelnodeopsv1.OperationSpec{
			"revoke a forward node token":  revokeSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN),
			"issue a node API key":         issueSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY, "", true),
			"issue without the enrollment": issueSpec(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, "", false),
		} {
			_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "removed:" + name, Operation: spec})
			require.Equal(t, codes.InvalidArgument, status.Code(err), name)
		}
		// A clean agent is no node a credential kind takes.
		for name, spec := range map[string]*kernelnodeopsv1.OperationSpec{
			"revoke a clean agent":      revokeSpec(cleanAgent, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED),
			"issue a clean agent token": issueSpec(cleanAgent, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_CLEAN_AGENT_TOKEN, "", true),
		} {
			_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "removed:" + name, Operation: spec})
			require.Equal(t, codes.PermissionDenied, status.Code(err), name)
		}
		// A credential the subject's kind does not hold is refused by the
		// kind's check.
		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "credential.revoke:wrong", Operation: revokeSpec(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY)})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}
