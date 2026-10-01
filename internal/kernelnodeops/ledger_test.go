package kernelnodeops

import (
	"context"
	"strings"
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"gorm.io/gorm"
)

const (
	forwardAction = kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UPDATE
	proxyKind     = kernelnodeopsv1.NodeKind_NODE_KIND_PROXY
	forwardKind   = kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD
)

// A repeat of a request id answers the first receipt, as it is now; the
// same id with another operation, or from another package, is refused.
func TestARepeatAnswersTheFirstReceipt(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindForwardApply, newScript(succeed))
		h.serve(KindNodeSync, newScript(succeed))
		forward := h.client(forwardHost, allFamilies())

		first := submit(t, forward, "forward.apply:40:update:d1", applyForward(40, forwardAction))
		require.True(t, first.GetApplied())
		require.Equal(t, kernelnodeopsv1.OperationState_OPERATION_STATE_PENDING, first.GetOperation().GetState())
		require.Equal(t, "forward", first.GetOperation().GetPackageId())
		require.EqualValues(t, 3, first.GetOperation().GetPackageGeneration())
		require.Equal(t, KindForwardApply, first.GetOperation().GetKind())
		require.Equal(t, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_FORWARD, first.GetOperation().GetFamily())
		require.Equal(t, []*kernelnodeopsv1.NodeRef{nodeRef(forwardKind, 10), nodeRef(forwardKind, 11)}, first.GetOperation().GetTargets())

		repeat := submit(t, forward, "forward.apply:40:update:d1", applyForward(40, forwardAction))
		require.False(t, repeat.GetApplied())
		require.Equal(t, first.GetOperation().GetOperationId(), repeat.GetOperation().GetOperationId())
		require.Len(t, h.rows(t), 1, "a repeat records nothing")

		_, err := forward.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "forward.apply:40:update:d1", Operation: applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_PAUSE),
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err), "the same id with another operation")
		_, err = forward.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "forward.apply:40:update:d1", Operation: syncNode(forwardKind, 10, false),
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err), "the same id with another kind")

		proxy := h.client(proxyHost, allFamilies())
		_, err = proxy.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "forward.apply:40:update:d1", Operation: applyForward(40, forwardAction),
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err), "the same operation from another package")
		require.Len(t, h.rows(t), 1)

		// The ledger keeps the digest and the canonical operation.
		row := h.rows(t)[0]
		require.Len(t, row.Digest, 64)
		require.Equal(t, "forward@3", row.SubmittedBy)
		require.Equal(t, "forward:40", row.ResourceKey)
		require.JSONEq(t, `{"apply_forward":{"forward_id":"40","action":"FORWARD_ACTION_UPDATE"}}`, row.Operation)
	})
}

// A retry of the same intent carries other sealed handles (they are minted
// per request): it is the same operation.
func TestSealedHandlesDoNotChangeTheDigest(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindCredentialIssue, newScript(succeed))
		h.serve(KindSecretsPut, newScript(succeed))
		proxy := h.client(proxyHost, allFamilies())
		credential := nodeRef(forwardKind, 10)
		first := submit(t, proxy, "credential.issue:forward-10:token:d", issueCredential(credential, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, SealedPrefix+"aaaa"))
		repeat := submit(t, proxy, "credential.issue:forward-10:token:d", issueCredential(credential, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, SealedPrefix+"bbbb"))
		require.True(t, first.GetApplied())
		require.False(t, repeat.GetApplied())
		require.Equal(t, first.GetOperation().GetOperationId(), repeat.GetOperation().GetOperationId())

		protocol := h.client(protocolHost, allFamilies())
		document := func(handle string) *kernelnodeopsv1.OperationSpec {
			return putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "reality_settings",
				`{"private_key":"`+handle+`","public_key":"pub","short_id":"ab"}`)
		}
		first = submit(t, protocol, "secrets.put:5:reality:d", document(SealedPrefix+"one"))
		repeat = submit(t, protocol, "secrets.put:5:reality:d", document(SealedPrefix+"two"))
		require.Equal(t, first.GetOperation().GetOperationId(), repeat.GetOperation().GetOperationId())
		for _, row := range h.rows(t) {
			require.NotContains(t, row.Operation, "aaaa", "handles are not stored")
			require.NotContains(t, row.Operation, "one")
		}
	})
}

// A target that does not exist is NOT_FOUND and records nothing; a repeat
// of a retirement after the package deleted its row still answers the
// first receipt.
func TestMissingTargetsRecordNothing(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		for _, kindName := range KnownKinds() {
			h.serve(kindName, newScript(succeed))
		}
		forward := h.client(forwardHost, allFamilies())
		ctx := context.Background()
		for name, spec := range map[string]*kernelnodeopsv1.OperationSpec{
			"forward":          applyForward(99, forwardAction),
			"tunnel":           applyTunnel(99),
			"forward node":     syncNode(forwardKind, 99, false),
			"proxy node":       retireNode(proxyKind, 99),
			"endpoint":         checkEndpoints(nodeRef(forwardKind, 10), nodeRef(forwardKind, 99)),
			"agent node":       agentOperation(99, "agent.ping"),
			"protocol":         {Operation: &kernelnodeopsv1.OperationSpec_RetireProtocol{RetireProtocol: &kernelnodeopsv1.RetireProtocol{ProtocolId: 99}}},
			"registration":     {Operation: &kernelnodeopsv1.OperationSpec_RevokeRegistrationKey{RevokeRegistrationKey: &kernelnodeopsv1.RevokeRegistrationKey{KeyId: 99}}},
			"clean agent":      {Operation: &kernelnodeopsv1.OperationSpec_RevokeCredential{RevokeCredential: &kernelnodeopsv1.RevokeCredential{Subject: nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, 99)}}},
			"legacy rule":      {Operation: &kernelnodeopsv1.OperationSpec_ApplyLegacyRule{ApplyLegacyRule: &kernelnodeopsv1.ApplyLegacyRule{RuleId: 99, Action: forwardAction}}},
			"clean agent node": {Operation: &kernelnodeopsv1.OperationSpec_IssueCleanAgent{IssueCleanAgent: &kernelnodeopsv1.IssueCleanAgent{Name: "n", ForwardNodeId: 99}}},
		} {
			_, err := forward.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "missing:" + name, Operation: spec})
			require.Equal(t, codes.NotFound, status.Code(err), name)
		}
		require.Empty(t, h.rows(t))

		first := submit(t, forward, "node.retire:forward-12", retireNode(forwardKind, 12))
		require.NoError(t, db.Delete(&model.ForwardNode{}, 12).Error)
		repeat := submit(t, forward, "node.retire:forward-12", retireNode(forwardKind, 12))
		require.False(t, repeat.GetApplied())
		require.Equal(t, first.GetOperation().GetOperationId(), repeat.GetOperation().GetOperationId())
	})
}

// A kind with no executor is UNIMPLEMENTED and records nothing, so a retry
// after the kernel gained the executor applies. GetCapabilities lists only
// the kinds with one.
func TestKindsWithoutAnExecutorAreUnimplemented(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		forward := h.client(forwardHost, allFamilies())
		ctx := context.Background()
		capabilities, err := forward.GetCapabilities(ctx, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Empty(t, capabilities.GetKinds(), "NO-1 executes no kind")

		for _, kindName := range KnownKinds() {
			_, err := forward.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "r:" + kindName, Operation: sampleOperation(kindName)})
			require.Equal(t, codes.Unimplemented, status.Code(err), kindName)
			require.Contains(t, status.Convert(err).Message(), kindName)
		}
		require.Empty(t, h.rows(t), "nothing is recorded, nothing is applied")

		h.serve(KindNodeSync, newScript(succeed))
		applied := submit(t, forward, "r:"+KindNodeSync, sampleOperation(KindNodeSync))
		require.True(t, applied.GetApplied(), "the retry is not answered a stale refusal")
		capabilities, err = forward.GetCapabilities(ctx, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Equal(t, []string{KindNodeSync}, capabilities.GetKinds())
	})
}

// sampleOperation is a valid operation of each kind on the seeded rows.
func sampleOperation(kindName string) *kernelnodeopsv1.OperationSpec {
	samples := map[string]*kernelnodeopsv1.OperationSpec{
		KindForwardApply:       applyForward(40, forwardAction),
		KindForwardTunnel:      applyTunnel(30),
		KindForwardSyncBackend: {Operation: &kernelnodeopsv1.OperationSpec_SyncForwardBackend{SyncForwardBackend: &kernelnodeopsv1.SyncForwardBackend{}}},
		KindForwardLegacyRule:  {Operation: &kernelnodeopsv1.OperationSpec_ApplyLegacyRule{ApplyLegacyRule: &kernelnodeopsv1.ApplyLegacyRule{RuleId: 50, Action: forwardAction}}},
		KindNodeSync:           syncNode(proxyKind, 1, true),
		KindNodeRetire:         retireNode(proxyKind, 2),
		KindProtocolRetire:     {Operation: &kernelnodeopsv1.OperationSpec_RetireProtocol{RetireProtocol: &kernelnodeopsv1.RetireProtocol{ProtocolId: 5}}},
		KindSecretsPut:         putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG, 1, "raw_config", `{"private_key":"********"}`),
		KindDiagnoseEndpoints:  checkEndpoints(nodeRef(forwardKind, 10), nodeRef(forwardKind, 11)),
		KindDiagnoseNodeStats:  {Operation: &kernelnodeopsv1.OperationSpec_CollectNodeStats{CollectNodeStats: &kernelnodeopsv1.CollectNodeStats{Node: nodeRef(forwardKind, 10)}}},
		KindDiagnoseForward:    {Operation: &kernelnodeopsv1.OperationSpec_DiagnoseForward{DiagnoseForward: &kernelnodeopsv1.DiagnoseForward{ForwardId: 40}}},
		KindDiagnoseTunnel:     {Operation: &kernelnodeopsv1.OperationSpec_DiagnoseTunnel{DiagnoseTunnel: &kernelnodeopsv1.DiagnoseTunnel{TunnelId: 30}}},
		KindDiagnoseForwardBackend: {Operation: &kernelnodeopsv1.OperationSpec_TestForwardBackend{TestForwardBackend: &kernelnodeopsv1.TestForwardBackend{
			Host: "198.51.100.10", ApiPort: 18080, Token: &kernelnodeopsv1.SecretRef{Handle: SealedPrefix + "x"},
		}}},
		KindAgentDiagnostic:  {Operation: &kernelnodeopsv1.OperationSpec_RunAgentDiagnostic{RunAgentDiagnostic: &kernelnodeopsv1.RunAgentDiagnostic{NodeId: 1, Action: "ping", ParamsJson: []byte(`{"host":"1.1.1.1"}`)}}},
		KindAgentOperation:   agentOperation(1, "node.reload"),
		KindCredentialIssue:  issueCredential(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, ""),
		KindCredentialRevoke: {Operation: &kernelnodeopsv1.OperationSpec_RevokeCredential{RevokeCredential: &kernelnodeopsv1.RevokeCredential{Subject: nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, 20)}}},
		KindRegKeyIssue:      {Operation: &kernelnodeopsv1.OperationSpec_IssueRegistrationKey{IssueRegistrationKey: &kernelnodeopsv1.IssueRegistrationKey{Name: "edge"}}},
		KindRegKeyRevoke:     {Operation: &kernelnodeopsv1.OperationSpec_RevokeRegistrationKey{RevokeRegistrationKey: &kernelnodeopsv1.RevokeRegistrationKey{KeyId: 60}}},
		KindCleanAgentIssue:  {Operation: &kernelnodeopsv1.OperationSpec_IssueCleanAgent{IssueCleanAgent: &kernelnodeopsv1.IssueCleanAgent{Name: "agent", ForwardNodeId: 10}}},
	}
	return samples[kindName]
}

// Every kind of the contract has a sample, and every sample is valid and
// found: the kinds' checks and target lookups accept what the routes send.
func TestEveryKindAcceptsItsSample(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db, func(e *Engine) { e.NodeQuota = 1000 })
		for _, kindName := range KnownKinds() {
			h.serve(kindName, newScript(succeed))
		}
		require.Len(t, KnownKinds(), 20)
		forward := h.client(forwardHost, allFamilies())
		for _, kindName := range KnownKinds() {
			response := submit(t, forward, "sample:"+kindName, sampleOperation(kindName))
			require.Equal(t, kindName, response.GetOperation().GetKind())
			require.Equal(t, FamilyOfKind(kindName), response.GetOperation().GetFamily())
		}
	})
}

// The request id, the reason, the wait mode and every operation's fields
// are checked before anything is recorded.
func TestMalformedSubmissionsAreRefused(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		for _, kindName := range KnownKinds() {
			h.serve(kindName, newScript(succeed))
		}
		client := h.client(forwardHost, allFamilies())
		ctx := context.Background()
		refuse := func(name string, code codes.Code, request *kernelnodeopsv1.SubmitOperationRequest) {
			t.Helper()
			_, err := client.SubmitOperation(ctx, request)
			require.Equal(t, code, status.Code(err), name)
		}
		valid := applyForward(40, forwardAction)
		refuse("no request id", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{Operation: valid})
		refuse("long request id", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: strings.Repeat("r", 129), Operation: valid})
		refuse("control character", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "a\nb", Operation: valid})
		refuse("the kernel's prefix", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "fanout:x:1", Operation: valid})
		refuse("no operation", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "r"})
		refuse("empty operation", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "r", Operation: &kernelnodeopsv1.OperationSpec{}})
		refuse("long reason", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "r", Operation: valid, Reason: strings.Repeat("x", 256)})
		refuse("unknown wait", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "r", Operation: valid, Wait: 9})
		refuse("large binding", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "r", Operation: valid,
			Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: make([]byte, 4097)}})

		invalid := map[string]*kernelnodeopsv1.OperationSpec{
			"forward id":         applyForward(0, forwardAction),
			"forward id range":   applyForward(1<<33, forwardAction),
			"forward action":     applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UNSPECIFIED),
			"unknown action":     applyForward(40, 99),
			"tunnel reasons":     {Operation: &kernelnodeopsv1.OperationSpec_ApplyTunnel{ApplyTunnel: &kernelnodeopsv1.ApplyTunnel{TunnelId: 30, Reasons: []string{""}}}},
			"sync node kind":     syncNode(kernelnodeopsv1.NodeKind_NODE_KIND_UNSPECIFIED, 1, false),
			"sync node unknown":  syncNode(9, 1, false),
			"no endpoints":       checkEndpoints(),
			"repeated endpoint":  checkEndpoints(nodeRef(forwardKind, 10), nodeRef(forwardKind, 10)),
			"agent op kind":      agentOperation(1, "shell.exec"),
			"agent payload":      {Operation: &kernelnodeopsv1.OperationSpec_AgentControlOperation{AgentControlOperation: &kernelnodeopsv1.AgentControlOperation{NodeId: 1, Kind: "agent.ping", PayloadJson: []byte("{")}}},
			"agent timeout":      {Operation: &kernelnodeopsv1.OperationSpec_AgentControlOperation{AgentControlOperation: &kernelnodeopsv1.AgentControlOperation{NodeId: 1, Kind: "agent.ping", TimeoutSeconds: 3601}}},
			"diagnostic action":  {Operation: &kernelnodeopsv1.OperationSpec_RunAgentDiagnostic{RunAgentDiagnostic: &kernelnodeopsv1.RunAgentDiagnostic{NodeId: 1}}},
			"secret scope":       putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_UNSPECIFIED, 5, "settings", `{}`),
			"secret column":      putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "name", `{}`),
			"raw config column":  putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG, 1, "settings", `{}`),
			"document not JSON":  putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG, 1, "raw_config", `private_key=x`),
			"secret in clear":    putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "settings", `{"users":[{"password":"hunter2"}]}`),
			"array secret":       putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "settings", `{"psk":["k1"]}`),
			"credential handle":  issueCredential(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, "plain-token"),
			"credential kind":    issueCredential(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY, ""),
			"one value for all":  issueCredential(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED, SealedPrefix+"x"),
			"enrollment value":   issueCredential(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT, SealedPrefix+"x"),
			"registration name":  {Operation: &kernelnodeopsv1.OperationSpec_IssueRegistrationKey{IssueRegistrationKey: &kernelnodeopsv1.IssueRegistrationKey{Name: strings.Repeat("n", 101)}}},
			"registration time":  {Operation: &kernelnodeopsv1.OperationSpec_IssueRegistrationKey{IssueRegistrationKey: &kernelnodeopsv1.IssueRegistrationKey{ExpiresAtUnix: -1}}},
			"clean agent name":   {Operation: &kernelnodeopsv1.OperationSpec_IssueCleanAgent{IssueCleanAgent: &kernelnodeopsv1.IssueCleanAgent{ForwardNodeId: 10}}},
			"empty sealed value": issueCredential(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, SealedPrefix),
			"backend name":       {Operation: &kernelnodeopsv1.OperationSpec_SyncForwardBackend{SyncForwardBackend: &kernelnodeopsv1.SyncForwardBackend{Backend: "docker"}}},
			"test host":          {Operation: &kernelnodeopsv1.OperationSpec_TestForwardBackend{TestForwardBackend: &kernelnodeopsv1.TestForwardBackend{ApiPort: 1}}},
			"test token clear":   {Operation: &kernelnodeopsv1.OperationSpec_TestForwardBackend{TestForwardBackend: &kernelnodeopsv1.TestForwardBackend{Host: "h", ApiPort: 1, Token: &kernelnodeopsv1.SecretRef{Handle: "typed"}}}},
		}
		for name, spec := range invalid {
			refuse(name, codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "invalid:" + name, Operation: spec})
		}

		// A field this kernel does not know is refused rather than ignored;
		// an operation kind it does not know is UNIMPLEMENTED.
		withUnknown := applyForward(40, forwardAction)
		withUnknown.GetApplyForward().ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 9, protowire.VarintType), 1))
		refuse("unknown field", codes.InvalidArgument, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "unknown-field", Operation: withUnknown})
		newKind := &kernelnodeopsv1.OperationSpec{}
		newKind.ProtoReflect().SetUnknown(protowire.AppendBytes(protowire.AppendTag(nil, 60, protowire.BytesType), nil))
		refuse("unknown kind", codes.Unimplemented, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "unknown-kind", Operation: newKind})
		require.Empty(t, h.rows(t))
	})
}

// The canonical form and the digest ignore handles only: every other field
// changes the digest.
func TestDigestCoversTheOperation(t *testing.T) {
	digestOf := func(spec *kernelnodeopsv1.OperationSpec) string {
		k, err := kindOf(spec)
		require.NoError(t, err)
		value, err := digest(k.name, canonical(spec))
		require.NoError(t, err)
		return value
	}
	base := digestOf(applyForward(40, forwardAction))
	require.Equal(t, base, digestOf(applyForward(40, forwardAction)))
	require.NotEqual(t, base, digestOf(applyForward(41, forwardAction)))
	require.NotEqual(t, base, digestOf(applyForward(40, kernelnodeopsv1.ForwardAction_FORWARD_ACTION_SYNC)))
	require.NotEqual(t, digestOf(syncNode(proxyKind, 1, false)), digestOf(syncNode(proxyKind, 1, true)))
	require.NotEqual(t, digestOf(syncNode(proxyKind, 1, false)), digestOf(syncNode(forwardKind, 1, false)))
	document := func(text string) string {
		return digestOf(putSecretDocument(kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL, 5, "settings", text))
	}
	require.Equal(t, document(`{"a":1,"password":"`+SealedPrefix+`x"}`), document(`{"password":"`+SealedPrefix+`y", "a":1}`))
	require.NotEqual(t, document(`{"a":1,"password":"`+SealedPrefix+`x"}`), document(`{"a":2,"password":"`+SealedPrefix+`x"}`))
	require.NotEqual(t, document(`{"password":"`+SealedPrefix+`x"}`), document(`{"password":"********"}`))
}

// Concurrent submissions of one request id record one operation; every
// caller gets it, one of them with applied true.
func TestConcurrentRepeatsRecordOnce(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindNodeSync, newScript(succeed))
		client := h.client(protocolHost, allFamilies())
		const callers = 8
		type answer struct {
			response *kernelnodeopsv1.SubmitOperationResponse
			err      error
		}
		answers := make(chan answer, callers)
		start := make(chan struct{})
		for range callers {
			go func() {
				<-start
				response, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
					RequestId: "node.sync:proxy-1:race", Operation: syncNode(proxyKind, 1, true),
				})
				answers <- answer{response, err}
			}()
		}
		close(start)
		applied := 0
		ids := map[string]bool{}
		for range callers {
			got := <-answers
			require.NoError(t, got.err)
			if got.response.GetApplied() {
				applied++
			}
			ids[got.response.GetOperation().GetOperationId()] = true
		}
		require.Equal(t, 1, applied)
		require.Len(t, ids, 1)
		require.Len(t, h.rows(t), 1)
	})
}
