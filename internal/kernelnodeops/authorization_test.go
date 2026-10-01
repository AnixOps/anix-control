package kernelnodeops

import (
	"context"
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Each kind needs its family's capability; reads, watches and cancellations
// need any one of the five, and GetCapabilities none.
func TestEachFamilyNeedsItsCapability(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		for _, kindName := range KnownKinds() {
			h.serve(kindName, newScript(succeed))
		}
		ctx := context.Background()
		for _, kindName := range KnownKinds() {
			capability := families[FamilyOfKind(kindName)].capability
			holder := h.client(forwardHost, allow(capability))
			response, err := holder.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "family:" + kindName, Operation: sampleOperation(kindName)})
			require.NoError(t, err, kindName)
			require.True(t, response.GetApplied())
			for _, other := range service.NodeOpsCapabilities() {
				if other == capability {
					continue
				}
				_, err := h.client(forwardHost, allow(other)).SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{
					RequestId: "other:" + kindName, Operation: sampleOperation(kindName),
				})
				require.Equal(t, codes.PermissionDenied, status.Code(err), "%s with %s", kindName, other)
			}
		}
		require.Len(t, h.rows(t), len(KnownKinds()))

		// Any one family reads, lists, watches and cancels.
		diagnose := h.client(forwardHost, allow(service.CapabilityNodeOpsDiagnose))
		list, err := diagnose.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{})
		require.NoError(t, err)
		require.Len(t, list.GetOperations(), len(KnownKinds()), "a package lists its operations of every family")
		_, err = diagnose.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: list.GetOperations()[0].GetOperationId()})
		require.NoError(t, err)

		none := h.client(forwardHost, allow())
		_, err = none.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_RequestId{RequestId: "family:" + KindNodeSync}})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = none.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = none.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: list.GetOperations()[0].GetOperationId()})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		capabilities, err := none.GetCapabilities(ctx, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err, "GetCapabilities needs no family")
		require.Empty(t, capabilities.GetGrantedFamilies())

		two := h.client(forwardHost, allow(service.CapabilityNodeOpsForward, service.CapabilityNodeOpsCredentials))
		capabilities, err = two.GetCapabilities(ctx, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.NoError(t, err)
		require.Equal(t, []kernelnodeopsv1.OperationFamily{
			kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_FORWARD, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS,
		}, capabilities.GetGrantedFamilies())
		require.Equal(t, KnownKinds(), capabilities.GetKinds())
		require.Len(t, capabilities.GetTables(), 7)
		for _, table := range capabilities.GetTables() {
			require.Equal(t, kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_LEGACY, table.GetPhase(), table.GetTable())
		}
	})
}

// A fenced generation can do nothing, not even ask for its capabilities:
// an old instance cannot start operations after an upgrade. What it
// submitted belongs to its package, and the new generation sees it.
func TestFencedGenerationsAreRefused(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindNodeSync, newScript(succeed))
		authorizer := allFamilies()
		old := h.client(proxyHost, authorizer)
		operation := submit(t, old, "node.sync:before-upgrade", syncNode(proxyKind, 1, false)).GetOperation()

		authorizer.set("fenced", true)
		ctx := context.Background()
		_, err := old.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.sync:after", Operation: syncNode(proxyKind, 1, false)})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), "fenced")
		_, err = old.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "node.sync:before-upgrade", Operation: syncNode(proxyKind, 1, false)})
		require.Equal(t, codes.PermissionDenied, status.Code(err), "not even a repeat")
		_, err = old.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_OperationId{OperationId: operation.GetOperationId()}})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = old.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = old.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: operation.GetOperationId()})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = old.GetCapabilities(ctx, &kernelnodeopsv1.GetCapabilitiesRequest{})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Len(t, h.rows(t), 1)

		next := proxyHost
		next.Generation++
		current := (&Server{Engine: h.engine, Authorizer: allFamilies()}).For(next)
		seen := get(t, current, operation.GetOperationId())()
		require.EqualValues(t, proxyHost.Generation, seen.GetPackageGeneration(), "the new generation sees its package's operations")
		repeat := submit(t, current, "node.sync:before-upgrade", syncNode(proxyKind, 1, false))
		require.False(t, repeat.GetApplied())
		require.Equal(t, operation.GetOperationId(), repeat.GetOperation().GetOperationId())
	})
}

// A family cannot reach another domain's nodes: each kind takes only its
// node kinds.
func TestKindsTakeOnlyTheirNodeKinds(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		for _, kindName := range KnownKinds() {
			h.serve(kindName, newScript(succeed))
		}
		client := h.client(forwardHost, allFamilies())
		cleanAgent := kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT
		for name, spec := range map[string]*kernelnodeopsv1.OperationSpec{
			"sync a clean agent":         syncNode(cleanAgent, 20, false),
			"retire a clean agent":       retireNode(cleanAgent, 20),
			"check a proxy endpoint":     checkEndpoints(nodeRef(forwardKind, 10), nodeRef(proxyKind, 1)),
			"proxy node statistics":      {Operation: &kernelnodeopsv1.OperationSpec_CollectNodeStats{CollectNodeStats: &kernelnodeopsv1.CollectNodeStats{Node: nodeRef(proxyKind, 1)}}},
			"clean agent statistics":     {Operation: &kernelnodeopsv1.OperationSpec_CollectNodeStats{CollectNodeStats: &kernelnodeopsv1.CollectNodeStats{Node: nodeRef(cleanAgent, 20)}}},
			"check a clean agent":        checkEndpoints(nodeRef(cleanAgent, 20)),
			"a credential for no node":   issueCredential(nil, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY, ""),
			"an unknown kind of subject": issueCredential(nodeRef(7, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY, ""),
		} {
			_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "kind:" + name, Operation: spec})
			require.Contains(t, []codes.Code{codes.PermissionDenied, codes.InvalidArgument}, status.Code(err), name)
		}
		_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "kind:sync", Operation: syncNode(cleanAgent, 20, false)})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), "does not take clean_agent nodes")
		// Credentials must be ones the node kind holds.
		for name, spec := range map[string]*kernelnodeopsv1.OperationSpec{
			"api key of a forward node":     issueCredential(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY, ""),
			"forward token of a proxy node": issueCredential(nodeRef(proxyKind, 1), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, ""),
			"enrollment of a clean agent":   issueCredential(nodeRef(cleanAgent, 20), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT, ""),
		} {
			_, err := client.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{RequestId: "credential:" + name, Operation: spec})
			require.Equal(t, codes.InvalidArgument, status.Code(err), name)
		}
		require.Empty(t, h.rows(t))
		// What each kind takes is accepted.
		submit(t, client, "ok:sync forward", syncNode(forwardKind, 10, false))
		submit(t, client, "ok:retire proxy", retireNode(proxyKind, 2))
		submit(t, client, "ok:revoke clean agent", sampleOperation(KindCredentialRevoke))
		submit(t, client, "ok:enroll forward", issueCredential(nodeRef(forwardKind, 11), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT, ""))
	})
}

// A package sees only its own operations: another's is NOT_FOUND to get or
// cancel, and absent from its list.
func TestPackagesSeeOnlyTheirOwnOperations(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindNodeSync, newScript(succeed))
		h.serve(KindForwardApply, newScript(succeed))
		proxy := h.client(proxyHost, allFamilies())
		forward := h.client(forwardHost, allFamilies())
		mine := submit(t, proxy, "node.sync:mine", syncNode(proxyKind, 1, false)).GetOperation()
		submit(t, forward, "forward.apply:theirs", applyForward(40, forwardAction))
		ctx := context.Background()
		_, err := forward.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_OperationId{OperationId: mine.GetOperationId()}})
		require.Equal(t, codes.NotFound, status.Code(err))
		_, err = forward.GetOperation(ctx, &kernelnodeopsv1.GetOperationRequest{Selector: &kernelnodeopsv1.GetOperationRequest_RequestId{RequestId: "node.sync:mine"}})
		require.Equal(t, codes.NotFound, status.Code(err))
		_, err = forward.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: mine.GetOperationId()})
		require.Equal(t, codes.NotFound, status.Code(err))
		require.Equal(t, pending, get(t, proxy, mine.GetOperationId())().GetState())
		list, err := forward.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{})
		require.NoError(t, err)
		require.Len(t, list.GetOperations(), 1)
		require.Equal(t, "forward", list.GetOperations()[0].GetPackageId())
	})
}

// ListOperations filters by family, state and target, newest first, in
// pages.
func TestListOperationsFiltersAndPages(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		for _, kindName := range KnownKinds() {
			h.serve(kindName, newScript(succeed))
		}
		client := h.client(forwardHost, allFamilies())
		for _, kindName := range KnownKinds() {
			submit(t, client, "list:"+kindName, sampleOperation(kindName))
		}
		ctx := context.Background()
		var all []*kernelnodeopsv1.Operation
		token := ""
		for {
			page, err := client.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{Limit: 4, PageToken: token})
			require.NoError(t, err)
			all = append(all, page.GetOperations()...)
			if token = page.GetNextPageToken(); token == "" {
				break
			}
		}
		require.Len(t, all, len(KnownKinds()))
		require.Equal(t, "list:"+KnownKinds()[len(KnownKinds())-1], all[0].GetRequestId(), "newest first")

		credentials, err := client.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{
			Families: []kernelnodeopsv1.OperationFamily{kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS},
		})
		require.NoError(t, err)
		require.Len(t, credentials.GetOperations(), 5)

		onNode, err := client.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{Target: nodeRef(forwardKind, 12)})
		require.NoError(t, err)
		var kindsOnNode []string
		for _, operation := range onNode.GetOperations() {
			kindsOnNode = append(kindsOnNode, operation.GetKind())
		}
		require.ElementsMatch(t, []string{KindForwardLegacyRule}, kindsOnNode)

		_, err = h.engine.end(ctx, all[0].GetOperationId(), activeStates, ending{state: stateSucceeded})
		require.NoError(t, err)
		ended, err := client.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{States: []kernelnodeopsv1.OperationState{succeeded}})
		require.NoError(t, err)
		require.Len(t, ended.GetOperations(), 1)
		active, err := client.ListOperations(ctx, &kernelnodeopsv1.ListOperationsRequest{States: []kernelnodeopsv1.OperationState{pending}})
		require.NoError(t, err)
		require.Len(t, active.GetOperations(), len(KnownKinds())-1)

		for name, request := range map[string]*kernelnodeopsv1.ListOperationsRequest{
			"family": {Families: []kernelnodeopsv1.OperationFamily{0}},
			"state":  {States: []kernelnodeopsv1.OperationState{0}},
			"target": {Target: nodeRef(0, 1)},
			"token":  {PageToken: "%%%"},
		} {
			_, err := client.ListOperations(ctx, request)
			require.Equal(t, codes.InvalidArgument, status.Code(err), name)
		}
	})
}

// Each package may have 256 operations that have not ended, and each node
// 32; beyond is RESOURCE_EXHAUSTED and records nothing. Ended operations
// free their place.
func TestQuotas(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db, func(e *Engine) { e.PackageQuota = 4; e.NodeQuota = 2 })
		h.serve(KindAgentOperation, newScript(succeed))
		h.serve(KindRegKeyIssue, newScript(succeed))
		client := h.client(protocolHost, allFamilies())
		ctx := context.Background()
		first := submit(t, client, "agent:1:a", agentOperation(1, "agent.ping")).GetOperation()
		submit(t, client, "agent:1:b", agentOperation(1, "agent.ping"))
		_, err := client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "agent:1:c", Operation: agentOperation(1, "agent.ping")})
		require.Equal(t, codes.ResourceExhausted, status.Code(err), "node 1 has 2")
		require.Contains(t, status.Convert(err).Message(), "proxy node 1")
		submit(t, client, "agent:2:a", agentOperation(2, "agent.ping"))
		submit(t, client, "regkey:a", sampleOperation(KindRegKeyIssue))
		_, err = client.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "regkey:b", Operation: sampleOperation(KindRegKeyIssue)})
		require.Equal(t, codes.ResourceExhausted, status.Code(err), "the package has 4")
		require.Len(t, h.rows(t), 4)

		// Another package has its own quota; the node's is shared.
		other := h.client(proxyHost, allFamilies())
		submit(t, other, "regkey:other", sampleOperation(KindRegKeyIssue))
		_, err = other.SubmitOperation(ctx, &kernelnodeopsv1.SubmitOperationRequest{RequestId: "agent:1:other", Operation: agentOperation(1, "agent.ping")})
		require.Equal(t, codes.ResourceExhausted, status.Code(err))

		// A repeat of a recorded request is answered, quota or not.
		repeat := submit(t, client, "agent:1:a", agentOperation(1, "agent.ping"))
		require.False(t, repeat.GetApplied())

		_, err = client.CancelOperation(ctx, &kernelnodeopsv1.CancelOperationRequest{OperationId: first.GetOperationId()})
		require.NoError(t, err)
		submit(t, client, "agent:1:c", agentOperation(1, "agent.ping"))
	})
}
