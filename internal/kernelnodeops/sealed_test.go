package kernelnodeops

import (
	"context"
	"strings"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var liveHandle = sealedsecrets.Prefix + strings.Repeat("A", 43)

func TestTheSealedPrefixIsTheGatewaysGrammar(t *testing.T) {
	require.Equal(t, sealedsecrets.Prefix, SealedPrefix)
	require.True(t, sealedsecrets.IsHandle(liveHandle))
}

// A binding that names no live request of the calling package is refused
// before anything is prepared or recorded; a submission without one is not
// bound, and its handles do not resolve.
func TestUnverifiableBindingsAreRefused(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		prepared := 0
		h.serve(KindCredentialIssue, preparing{prepare: func(_ context.Context, submission *Submission) error {
			prepared++
			_, err := submission.Unseal(sealedsecrets.Use{Handle: liveHandle, Target: NodeTarget(nodeRef(forwardKind, 10)), Field: "/api_token"})
			return err
		}})
		client := h.client(forwardHost, allFamilies())
		spec := issueCredential(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, liveHandle)
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "credential.issue:forward-10:token:a", Operation: spec,
			Request: &kernelnodeopsv1.RequestBinding{BridgeCapability: make([]byte, 32)},
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Zero(t, prepared, "refused before Prepare")
		assert.NotContains(t, err.Error(), liveHandle)

		_, err = client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "credential.issue:forward-10:token:b", Operation: spec})
		require.Equal(t, codes.PermissionDenied, status.Code(err), "without a binding a handle resolves nothing")
		assert.Equal(t, 1, prepared)
		assert.Empty(t, h.rows(t), "nothing was recorded")
	})
}

// The ledger never holds a handle: not in the request id or reason, and not
// in an operation whose kind resolves none.
func TestTheLedgerNeverHoldsAHandle(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindCleanAgentIssue, newScript(succeed))
		h.serve(KindNodeSync, newScript(succeed))
		client := h.client(forwardHost, allFamilies())
		ctx := context.Background()
		_, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.sync:" + liveHandle, Operation: syncNode(forwardKind, 10, false)})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		_, err = client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.sync:forward-10", Operation: syncNode(forwardKind, 10, false), Reason: "key " + liveHandle})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		_, err = client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "cleanagent.issue:1",
			Operation: &kernelnodeopsv1.OperationSpec{Operation: &kernelnodeopsv1.OperationSpec_IssueCleanAgent{IssueCleanAgent: &kernelnodeopsv1.IssueCleanAgent{
				Name: liveHandle, ForwardNodeId: 10,
			}}}})
		require.Equal(t, codes.InvalidArgument, status.Code(err), "a kind without a Preparer resolves no handle")
		for _, err := range []error{err} {
			assert.NotContains(t, err.Error(), liveHandle)
		}
		assert.Empty(t, h.rows(t))
	})
}

// A result's handles are kept out of the ledger: the stored copy keeps each
// SecretHandle's field and expiry, and the handles are answered once, to a
// call bound to the request they were minted for.
func TestResultsAreStoredWithoutTheirHandles(t *testing.T) {
	result := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: &kernelnodeopsv1.CredentialResult{
		Version: 2, Reveal: []*kernelnodeopsv1.SecretHandle{{Field: "api_key", Handle: liveHandle, ExpiresAtUnixMs: 99}},
	}}}
	stored, revealed := withoutHandles(result)
	require.Same(t, result, revealed)
	require.Len(t, stored.GetCredential().GetReveal(), 1)
	assert.Empty(t, stored.GetCredential().GetReveal()[0].GetHandle())
	assert.Equal(t, "api_key", stored.GetCredential().GetReveal()[0].GetField())
	assert.EqualValues(t, 99, stored.GetCredential().GetReveal()[0].GetExpiresAtUnixMs())
	assert.Equal(t, liveHandle, result.GetCredential().GetReveal()[0].GetHandle(), "the executor's result is not changed")
	assert.False(t, holdsHandle(stored.ProtoReflect()))

	plain := &kernelnodeopsv1.OperationResult{Result: &kernelnodeopsv1.OperationResult_Credential{Credential: &kernelnodeopsv1.CredentialResult{Version: 1}}}
	stored, revealed = withoutHandles(plain)
	require.Same(t, plain, stored)
	require.Nil(t, revealed)
	stored, revealed = withoutHandles(nil)
	require.Nil(t, stored)
	require.Nil(t, revealed)

	engine := &Engine{}
	bound := &BoundRequest{Deadline: time.Now().Add(time.Minute), binding: sealedsecrets.Binding{Key: "key-1"}}
	engine.keepReveal("op-1", result, bound)
	assert.Nil(t, engine.takeReveal("op-1", nil))
	assert.Nil(t, engine.takeReveal("op-1", &BoundRequest{Deadline: bound.Deadline, binding: sealedsecrets.Binding{Key: "key-2"}}), "another request")
	assert.Same(t, result, engine.takeReveal("op-1", bound))
	assert.Nil(t, engine.takeReveal("op-1", bound), "once")
	expired := &BoundRequest{Deadline: time.Now().Add(-time.Second), binding: sealedsecrets.Binding{Key: "key-1"}}
	engine.keepReveal("op-2", result, expired)
	assert.Nil(t, engine.takeReveal("op-2", expired), "after its request")
}

// An executor reveals only for a live binding, and what it reveals is
// scrubbed from its texts.
func TestRevealNeedsALiveBinding(t *testing.T) {
	run := &Run{engine: &Engine{Secrets: sealedsecrets.NewStore(nil)}}
	_, err := run.Reveal("api_key", "generated-secret")
	require.ErrorIs(t, err, ErrNoRequestBinding)
	assert.Equal(t, []string{"generated-secret"}, run.usedSecrets())
	run.Request = &BoundRequest{Deadline: time.Now().Add(-time.Second)}
	_, err = run.Reveal("api_key", "generated-secret")
	require.ErrorIs(t, err, ErrNoRequestBinding)
	run.Request = &BoundRequest{Deadline: time.Now().Add(time.Minute), binding: sealedsecrets.Binding{Key: "unknown"}}
	_, err = run.Reveal("api_key", "generated-secret")
	require.Error(t, err, "a request the gateway did not seal")

	var submission *Submission
	_, err = submission.Unseal()
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestNodeTargets(t *testing.T) {
	assert.Equal(t, sealedsecrets.Target{Kind: sealedsecrets.TargetProxy, ID: 1}, NodeTarget(nodeRef(proxyKind, 1)))
	assert.Equal(t, sealedsecrets.Target{Kind: sealedsecrets.TargetForward, ID: 10}, NodeTarget(nodeRef(forwardKind, 10)))
	assert.Equal(t, sealedsecrets.Target{Kind: sealedsecrets.TargetCleanAgent, ID: 20}, NodeTarget(nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT, 20)))
	assert.Equal(t, sealedsecrets.Target{}, NodeTarget(nil))
}

// preparing is an executor with a Preparer.
type preparing struct {
	prepare func(context.Context, *Submission) error
}

func (p preparing) Prepare(ctx context.Context, submission *Submission) error {
	return p.prepare(ctx, submission)
}

func (preparing) Execute(context.Context, *Run) Outcome { return Succeeded(nil) }
