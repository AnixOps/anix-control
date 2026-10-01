package kernelnodeops

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func parse(t *testing.T, raw string) (AdminQuery, error) {
	t.Helper()
	values, err := url.ParseQuery(raw)
	require.NoError(t, err)
	return ParseAdminQuery(values.Get)
}

// The administrator's listing shows every package's operations with what
// an audit needs, filtered and paged, and holds no secret.
func TestAdminListing(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindNodeSync, newScript(succeed))
		h.serve(KindForwardApply, newScript(succeed))
		h.serve(KindCredentialIssue, newScript(succeed))
		proxy := h.client(proxyHost, allFamilies())
		forward := h.client(forwardHost, allFamilies())
		sync, err := proxy.SubmitOperation(context.Background(), &kernelnodeopsv1.SubmitOperationRequest{
			RequestId: "node.sync:proxy-1:audit", Operation: syncNode(proxyKind, 1, true), Reason: "protocol changed",
		})
		require.NoError(t, err)
		submit(t, forward, "forward.apply:40:audit", applyForward(40, forwardAction))
		submit(t, forward, "credential.issue:audit", issueCredential(nodeRef(forwardKind, 10), kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, SealedPrefix+"secret-handle"))
		_, err = h.engine.end(context.Background(), sync.GetOperation().GetOperationId(), activeStates, ending{
			state: stateFailed, err: &kernelnodeopsv1.OperationError{Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_NODE_OFFLINE, Message: "offline", Retryable: true},
		})
		require.NoError(t, err)

		all, err := AdminList(context.Background(), db, AdminQuery{})
		require.NoError(t, err)
		require.Len(t, all.Operations, 3)
		require.Zero(t, all.NextBefore)
		require.Equal(t, KindCredentialIssue, all.Operations[0].Kind, "newest first")
		encoded, err := json.Marshal(all)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret-handle")

		query, err := parse(t, "package_id=proxy-node&state=failed")
		require.NoError(t, err)
		page, err := AdminList(context.Background(), db, query)
		require.NoError(t, err)
		require.Len(t, page.Operations, 1)
		view := page.Operations[0]
		require.Equal(t, "node.sync:proxy-1:audit", view.RequestID)
		require.Equal(t, "proxy-node@5", view.SubmittedBy)
		require.Equal(t, "nodeconfig", view.Family)
		require.True(t, view.Terminal)
		require.Equal(t, "protocol changed", view.Reason)
		require.Equal(t, "nodeconfig:proxy-1", view.Resource)
		require.Equal(t, []AdminTarget{{Kind: "proxy", ID: 1}}, view.Targets)
		require.Equal(t, &AdminError{Code: "node_offline", Message: "offline", Retryable: true}, view.Error)
		require.JSONEq(t, `{"sync_node":{"node":{"kind":"NODE_KIND_PROXY","id":"1"},"force":true}}`, string(view.Operation))
		require.NotNil(t, view.FinishedAt)

		query, err = parse(t, "target_kind=forward&target_id=11")
		require.NoError(t, err)
		page, err = AdminList(context.Background(), db, query)
		require.NoError(t, err)
		require.Len(t, page.Operations, 1)
		require.Equal(t, KindForwardApply, page.Operations[0].Kind)

		query, err = parse(t, "family=credentials&kind=credential.issue")
		require.NoError(t, err)
		page, err = AdminList(context.Background(), db, query)
		require.NoError(t, err)
		require.Len(t, page.Operations, 1)

		first, err := AdminList(context.Background(), db, AdminQuery{Limit: 2})
		require.NoError(t, err)
		require.Len(t, first.Operations, 2)
		require.NotZero(t, first.NextBefore)
		rest, err := AdminList(context.Background(), db, AdminQuery{Limit: 2, Before: first.NextBefore})
		require.NoError(t, err)
		require.Len(t, rest.Operations, 1)
		require.Equal(t, "node.sync:proxy-1:audit", rest.Operations[0].RequestID)
	})
}

func TestAdminQueryValidation(t *testing.T) {
	for _, raw := range []string{
		"family=everything", "kind=shell", "state=done", "target_kind=proxy", "target_id=3", "target_kind=moon&target_id=3",
		"target_kind=proxy&target_id=0", "before=-1", "before=x", "limit=0", "limit=501", "limit=x",
	} {
		_, err := parse(t, raw)
		require.Error(t, err, raw)
		var queryError *QueryError
		require.ErrorAs(t, err, &queryError)
	}
	query, err := parse(t, "family=forward&kind=forward.apply&state=running&target_kind=clean_agent&target_id=4&before=9&limit=50")
	require.NoError(t, err)
	require.Equal(t, AdminQuery{Family: "forward", Kind: "forward.apply", State: "running", TargetKind: "clean_agent", TargetID: 4, Before: 9, Limit: 50}, query)
}

// Ended operations are kept 90 days and events 7; operations that have not
// ended are never pruned. A pruned request id applies again.
func TestPruneKeepsTheRetention(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		h.serve(KindNodeSync, newScript(succeed))
		client := h.client(protocolHost, allFamilies())
		old := submit(t, client, "node.sync:old", syncNode(proxyKind, 1, false)).GetOperation()
		recent := submit(t, client, "node.sync:recent", syncNode(proxyKind, 2, false)).GetOperation()
		active := submit(t, client, "node.sync:active", syncNode(forwardKind, 10, false)).GetOperation()
		for _, id := range []string{old.GetOperationId(), recent.GetOperationId()} {
			_, err := h.engine.end(context.Background(), id, activeStates, ending{state: stateSucceeded})
			require.NoError(t, err)
		}
		now := time.Now()
		require.NoError(t, db.Model(&model.KernelNodeOperation{}).Where("operation_id = ?", old.GetOperationId()).
			Update("finished_at", now.Add(-OperationRetention-time.Hour)).Error)
		require.NoError(t, db.Model(&model.KernelNodeOperation{}).Where("operation_id = ?", active.GetOperationId()).
			Update("created_at", now.Add(-OperationRetention-time.Hour)).Error)
		require.NoError(t, db.Model(&model.KernelNodeOperationEvent{}).Where("operation_id = ?", recent.GetOperationId()).
			Update("created_at", now.Add(-EventRetention-time.Hour)).Error)

		removed, err := h.engine.Prune(context.Background(), now)
		require.NoError(t, err)
		require.EqualValues(t, 1, removed)
		var ids []string
		require.NoError(t, db.Model(&model.KernelNodeOperation{}).Order("id").Pluck("operation_id", &ids).Error)
		require.Equal(t, []string{recent.GetOperationId(), active.GetOperationId()}, ids)
		var targets int64
		require.NoError(t, db.Model(&model.KernelNodeOperationTarget{}).Where("operation_id = ?", old.GetOperationId()).Count(&targets).Error)
		require.Zero(t, targets)
		require.Empty(t, h.states(t, recent.GetOperationId()), "its events are older than theirs")
		require.NotEmpty(t, h.states(t, active.GetOperationId()))

		again := submit(t, client, "node.sync:old", syncNode(proxyKind, 1, false))
		require.True(t, again.GetApplied(), "a pruned request id applies again")
		require.NotEqual(t, old.GetOperationId(), again.GetOperation().GetOperationId())
	})
}
