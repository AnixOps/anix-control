package kernelnodeops

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// AdminQuery filters the administrator's listing of node operations
// (GET /api/v4/kernel/node-operations).
type AdminQuery struct {
	PackageID   string
	Family      string
	Kind        string
	State       string
	RequestID   string
	OperationID string
	Parent      string
	TargetKind  string
	TargetID    uint64
	// Before pages: operations listed before the one with this cursor.
	Before uint64
	Limit  int
}

// AdminTarget is one node an operation acts on.
type AdminTarget struct {
	Kind string `json:"kind"`
	ID   uint64 `json:"id"`
}

// AdminFanOut counts a fan-out's operations.
type AdminFanOut struct {
	Total     uint32 `json:"total"`
	Succeeded uint32 `json:"succeeded"`
	Failed    uint32 `json:"failed"`
	Pending   uint32 `json:"pending"`
}

// AdminError is how an operation failed.
type AdminError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// AdminOperation is one ledger row as administrators audit it: who asked
// for what, on which nodes, and how it went. It holds no secret: the
// operation is canonical, results and errors were scrubbed when stored.
type AdminOperation struct {
	Cursor              uint64          `json:"cursor"`
	OperationID         string          `json:"operation_id"`
	RequestID           string          `json:"request_id"`
	Digest              string          `json:"digest"`
	PackageID           string          `json:"package_id"`
	PackageGeneration   uint64          `json:"package_generation"`
	SubmittedBy         string          `json:"submitted_by"`
	Family              string          `json:"family"`
	Kind                string          `json:"kind"`
	State               string          `json:"state"`
	Terminal            bool            `json:"terminal"`
	Channel             string          `json:"channel"`
	Attempt             uint32          `json:"attempt"`
	NodeRevision        uint64          `json:"node_revision"`
	Resource            string          `json:"resource"`
	Targets             []AdminTarget   `json:"targets"`
	ParentOperationID   string          `json:"parent_operation_id"`
	FanOut              *AdminFanOut    `json:"fan_out,omitempty"`
	Reason              string          `json:"reason"`
	Operation           json.RawMessage `json:"operation"`
	Result              json.RawMessage `json:"result,omitempty"`
	Error               *AdminError     `json:"error,omitempty"`
	Evidence            json.RawMessage `json:"evidence,omitempty"`
	KernelOperationID   string          `json:"kernel_operation_id"`
	ForwardRuntimeJobID uint64          `json:"forward_runtime_job_id"`
	CancelRequestedAt   *time.Time      `json:"cancel_requested_at"`
	CancelReason        string          `json:"cancel_reason"`
	CreatedAt           time.Time       `json:"created_at"`
	AcceptedAt          *time.Time      `json:"accepted_at"`
	FinishedAt          *time.Time      `json:"finished_at"`
	DeadlineAt          time.Time       `json:"deadline_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// AdminPage is one page of the listing, newest first. NextBefore pages on;
// zero on the last page.
type AdminPage struct {
	Operations []AdminOperation `json:"operations"`
	NextBefore uint64           `json:"next_before"`
}

// QueryError is a filter the listing does not accept; its text says why.
type QueryError struct{ Message string }

func (e *QueryError) Error() string { return e.Message }

func queryError(message string) error { return &QueryError{Message: message} }

// ParseAdminQuery reads the listing's filters from query parameters.
func ParseAdminQuery(get func(string) string) (AdminQuery, error) {
	query := AdminQuery{
		PackageID: strings.TrimSpace(get("package_id")), Family: strings.TrimSpace(get("family")), Kind: strings.TrimSpace(get("kind")),
		State: strings.TrimSpace(get("state")), RequestID: strings.TrimSpace(get("request_id")),
		OperationID: strings.TrimSpace(get("operation_id")), Parent: strings.TrimSpace(get("parent_operation_id")),
		TargetKind: strings.TrimSpace(get("target_kind")),
	}
	for _, field := range []struct{ name, value string }{
		{"package_id", query.PackageID}, {"request_id", query.RequestID}, {"operation_id", query.OperationID}, {"parent_operation_id", query.Parent},
	} {
		if len(field.value) > maxRequestID {
			return query, queryError(field.name + " is too long")
		}
	}
	if query.Family != "" && familyByName(query.Family) == kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_UNSPECIFIED {
		return query, queryError("family must be forward, nodeconfig, diagnose, agents or credentials")
	}
	if query.Kind != "" && kinds[query.Kind] == nil {
		return query, queryError("kind is not an operation kind")
	}
	if _, known := stateEnums[query.State]; query.State != "" && !known {
		return query, queryError("state must be pending, dispatching, running, succeeded, failed, cancelled, timed_out or superseded")
	}
	if raw := strings.TrimSpace(get("target_id")); raw != "" || query.TargetKind != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 || nodeKindByName(query.TargetKind) == kernelnodeopsv1.NodeKind_NODE_KIND_UNSPECIFIED {
			return query, queryError("target_kind (proxy, forward or clean_agent) and target_id go together")
		}
		query.TargetID = id
	}
	if raw := strings.TrimSpace(get("before")); raw != "" {
		before, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || before == 0 {
			return query, queryError("before must be a positive cursor")
		}
		query.Before = before
	}
	if raw := strings.TrimSpace(get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxListLimit {
			return query, queryError("limit must be between 1 and 500")
		}
		query.Limit = limit
	}
	return query, nil
}

// AdminList lists every package's node operations, newest first.
func AdminList(ctx context.Context, db *gorm.DB, query AdminQuery) (AdminPage, error) {
	statement := db.WithContext(ctx).Model(&model.KernelNodeOperation{})
	for _, filter := range []struct{ column, value string }{
		{"package_id", query.PackageID}, {"family", query.Family}, {"kind", query.Kind}, {"state", query.State},
		{"request_id", query.RequestID}, {"operation_id", query.OperationID}, {"parent_operation_id", query.Parent},
	} {
		if filter.value != "" {
			statement = statement.Where(filter.column+" = ?", filter.value)
		}
	}
	if query.TargetID != 0 {
		statement = targetFilter(statement, nodeRef(nodeKindByName(query.TargetKind), query.TargetID))
	}
	if query.Before != 0 {
		statement = statement.Where("id < ?", query.Before)
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	var rows []model.KernelNodeOperation
	if err := statement.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return AdminPage{}, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.OperationID)
	}
	targets, err := loadTargets(db.WithContext(ctx), ids)
	if err != nil {
		return AdminPage{}, err
	}
	page := AdminPage{Operations: make([]AdminOperation, 0, len(rows))}
	for i := range rows {
		page.Operations = append(page.Operations, adminView(&rows[i], targets[rows[i].OperationID]))
	}
	if len(rows) == limit {
		page.NextBefore = rows[len(rows)-1].ID
	}
	return page, nil
}

func rawJSON(text string) json.RawMessage {
	if text == "" || !json.Valid([]byte(text)) {
		return nil
	}
	return json.RawMessage(text)
}

func adminView(row *model.KernelNodeOperation, targets []*kernelnodeopsv1.NodeRef) AdminOperation {
	view := AdminOperation{
		Cursor: row.ID, OperationID: row.OperationID, RequestID: row.RequestID, Digest: row.Digest, PackageID: row.PackageID,
		PackageGeneration: row.PackageGeneration, SubmittedBy: row.SubmittedBy, Family: row.Family, Kind: row.Kind,
		State: row.State, Terminal: Terminal(row.State), Channel: row.Channel, Attempt: row.Attempt, NodeRevision: row.NodeRevision,
		Resource: row.ResourceKey, Targets: make([]AdminTarget, 0, len(targets)), ParentOperationID: row.ParentOperationID,
		Reason: row.Reason, Operation: rawJSON(row.Operation), Result: rawJSON(row.Result), Evidence: rawJSON(row.Evidence),
		KernelOperationID: row.KernelOperationID, ForwardRuntimeJobID: row.ForwardRuntimeJobID,
		CancelRequestedAt: row.CancelRequestedAt, CancelReason: row.CancelReason, CreatedAt: row.CreatedAt,
		AcceptedAt: row.AcceptedAt, FinishedAt: row.FinishedAt, DeadlineAt: row.DeadlineAt, UpdatedAt: row.UpdatedAt,
	}
	if view.Operation == nil {
		view.Operation = json.RawMessage("{}")
	}
	for _, target := range targets {
		view.Targets = append(view.Targets, AdminTarget{Kind: nodeKindNames[target.GetKind()], ID: target.GetId()})
	}
	if k := kinds[row.Kind]; (k != nil && k.fanOut) || row.FanOutTotal > 0 {
		view.FanOut = &AdminFanOut{Total: row.FanOutTotal, Succeeded: row.FanOutSucceeded, Failed: row.FanOutFailed, Pending: row.FanOutPending}
	}
	if row.ErrorCode != "" || row.ErrorMessage != "" {
		view.Error = &AdminError{Code: row.ErrorCode, Message: row.ErrorMessage, Retryable: row.ErrorRetryable}
	}
	return view
}
