package kernelnodeops

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Operation states as the ledger stores them.
const (
	statePending     = "pending"
	stateDispatching = "dispatching"
	stateRunning     = "running"
	stateSucceeded   = "succeeded"
	stateFailed      = "failed"
	stateCancelled   = "cancelled"
	stateTimedOut    = "timed_out"
	stateSuperseded  = "superseded"
)

var (
	// activeStates have not ended.
	activeStates = []string{statePending, stateDispatching, stateRunning}
	// startedStates have a dispatch in flight.
	startedStates = []string{stateDispatching, stateRunning}
	stateEnums    = map[string]kernelnodeopsv1.OperationState{
		statePending: kernelnodeopsv1.OperationState_OPERATION_STATE_PENDING, stateDispatching: kernelnodeopsv1.OperationState_OPERATION_STATE_DISPATCHING,
		stateRunning: kernelnodeopsv1.OperationState_OPERATION_STATE_RUNNING, stateSucceeded: kernelnodeopsv1.OperationState_OPERATION_STATE_SUCCEEDED,
		stateFailed: kernelnodeopsv1.OperationState_OPERATION_STATE_FAILED, stateCancelled: kernelnodeopsv1.OperationState_OPERATION_STATE_CANCELLED,
		stateTimedOut: kernelnodeopsv1.OperationState_OPERATION_STATE_TIMED_OUT, stateSuperseded: kernelnodeopsv1.OperationState_OPERATION_STATE_SUPERSEDED,
	}
)

// Terminal reports whether a stored state has ended; it never changes again.
func Terminal(state string) bool {
	switch state {
	case stateSucceeded, stateFailed, stateCancelled, stateTimedOut, stateSuperseded:
		return true
	}
	return false
}

func stateName(state kernelnodeopsv1.OperationState) (string, bool) {
	for name, value := range stateEnums {
		if value == state {
			return name, true
		}
	}
	return "", false
}

// enumName is an enum value's name in the ledger: lower case, without its
// prefix ("CHANNEL_AGENT_CONTROL" is "agent_control").
func enumName(value fmt.Stringer, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(value.String(), prefix))
}

func channelName(channel kernelnodeopsv1.Channel) string {
	if channel == kernelnodeopsv1.Channel_CHANNEL_UNSPECIFIED {
		return ""
	}
	return enumName(channel, "CHANNEL_")
}

func channelEnum(name string) kernelnodeopsv1.Channel {
	if name == "" {
		return kernelnodeopsv1.Channel_CHANNEL_UNSPECIFIED
	}
	return kernelnodeopsv1.Channel(kernelnodeopsv1.Channel_value["CHANNEL_"+strings.ToUpper(name)])
}

func errorCodeName(code kernelnodeopsv1.ErrorCode) string {
	if code == kernelnodeopsv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return ""
	}
	return enumName(code, "ERROR_CODE_")
}

func errorCodeEnum(name string) kernelnodeopsv1.ErrorCode {
	if name == "" {
		return kernelnodeopsv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	return kernelnodeopsv1.ErrorCode(kernelnodeopsv1.ErrorCode_value["ERROR_CODE_"+strings.ToUpper(name)])
}

// ledgerLockKey is the PostgreSQL advisory lock that serializes the
// ledger's writes: event cursors then commit in order, so a watcher never
// steps over an event still being written, and the quota counts are exact.
const ledgerLockKey = 0x6b6e6f70_73303031 // "knops001"

// write runs fn in a transaction that holds the ledger's write lock.
func (e *Engine) write(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return service.WithRetryableTransaction(e.DB.WithContext(ctx), func(tx *gorm.DB) error {
		if err := lockLedger(tx); err != nil {
			return err
		}
		return fn(tx)
	})
}

func lockLedger(tx *gorm.DB) error {
	if tx.Name() == "postgres" {
		return tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(ledgerLockKey)).Error
	}
	// SQLite: a write statement takes the database's write lock now, so the
	// transaction never reads a snapshot another writer changes under it.
	return tx.Exec("UPDATE v4_kernel_node_operation_event SET id = id WHERE 1 = 0").Error
}

func appendEvent(tx *gorm.DB, op *model.KernelNodeOperation, now time.Time) error {
	return tx.Create(&model.KernelNodeOperationEvent{
		OperationID: op.OperationID, PackageID: op.PackageID, Family: op.Family, State: op.State, CreatedAt: now,
	}).Error
}

// ending is how an operation ends.
type ending struct {
	state   string
	result  *kernelnodeopsv1.OperationResult
	err     *kernelnodeopsv1.OperationError
	channel string
}

// maxResultBytes caps a stored result.
const maxResultBytes = 64 << 10

// endTx moves op from one of from to a terminal state, with its event and
// its fan-out parent's count, in tx. It reports whether it applied: a state
// already terminal never changes.
func (e *Engine) endTx(tx *gorm.DB, op *model.KernelNodeOperation, from []string, end ending) (bool, error) {
	now := e.now()
	updates := map[string]any{"state": end.state, "finished_at": now, "updated_at": now}
	if end.channel != "" && op.Channel == "" {
		updates["channel"] = end.channel
	}
	if end.err != nil {
		updates["error_code"] = errorCodeName(end.err.GetCode())
		updates["error_message"] = end.err.GetMessage()
		updates["error_retryable"] = end.err.GetRetryable()
	}
	if end.result != nil {
		encoded, err := storeJSON.Marshal(end.result)
		switch {
		case err != nil:
			return false, err
		case len(encoded) > maxResultBytes:
			updates["error_code"] = errorCodeName(kernelnodeopsv1.ErrorCode_ERROR_CODE_INTERNAL)
			updates["error_message"] = fmt.Sprintf("the result exceeded %d KiB and was not kept", maxResultBytes>>10)
			updates["error_retryable"] = false
		default:
			updates["result"] = string(encoded)
		}
	}
	result := tx.Model(&model.KernelNodeOperation{}).Where("operation_id = ? AND state IN ?", op.OperationID, from).Updates(updates)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}
	op.State = end.state
	if err := appendEvent(tx, op, now); err != nil {
		return false, err
	}
	if op.ParentOperationID != "" {
		if err := e.countChild(tx, op.ParentOperationID, end.state); err != nil {
			return false, err
		}
	}
	return true, nil
}

// countChild counts a child's end in its running parent, and ends the
// parent with its last child: CANCELLED when it was cancelled, SUCCEEDED
// when no child failed, else FAILED. A superseded child counts as
// succeeded: the operation that replaced it carries its work. A parent that
// ended already keeps its counts.
func (e *Engine) countChild(tx *gorm.DB, parentID, childState string) error {
	field := "fan_out_failed"
	if childState == stateSucceeded || childState == stateSuperseded {
		field = "fan_out_succeeded"
	}
	result := tx.Model(&model.KernelNodeOperation{}).
		Where("operation_id = ? AND state = ? AND fan_out_pending > 0", parentID, stateRunning).
		Updates(map[string]any{
			"fan_out_pending": gorm.Expr("fan_out_pending - 1"), field: gorm.Expr(field + " + 1"), "updated_at": e.now(),
		})
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	var parent model.KernelNodeOperation
	if err := tx.Where("operation_id = ?", parentID).Take(&parent).Error; err != nil {
		return err
	}
	if parent.FanOutPending > 0 {
		return nil
	}
	_, err := e.endTx(tx, &parent, []string{stateRunning}, fanOutEnding(&parent))
	return err
}

func fanOutEnding(parent *model.KernelNodeOperation) ending {
	switch {
	case parent.CancelRequestedAt != nil:
		return ending{state: stateCancelled, err: &kernelnodeopsv1.OperationError{
			Code: kernelnodeopsv1.ErrorCode_ERROR_CODE_CANCELLED, Message: cancelMessage(parent.CancelReason),
		}}
	case parent.FanOutFailed == 0:
		return ending{state: stateSucceeded}
	default:
		return ending{state: stateFailed, err: &kernelnodeopsv1.OperationError{
			Code:    kernelnodeopsv1.ErrorCode_ERROR_CODE_BACKEND_FAILED,
			Message: fmt.Sprintf("%d of %d operations failed", parent.FanOutFailed, parent.FanOutTotal), Retryable: true,
		}}
	}
}

func cancelMessage(reason string) string {
	if reason == "" {
		return "operation cancelled"
	}
	return "operation cancelled: " + reason
}

// submission is a checked operation ready to be recorded.
type submission struct {
	ownerID     string
	generation  uint64
	submittedBy string
	requestID   string
	kind        *kind
	operation   *kernelnodeopsv1.OperationSpec
	digest      string
	targets     []*kernelnodeopsv1.NodeRef
	resource    string
	reason      string
}

// errQuota is RESOURCE_EXHAUSTED for a package or node quota.
func errQuota(format string, args ...any) error {
	return status.Errorf(codes.ResourceExhausted, format, args...)
}

// record writes a submission: its row, targets and event, after replacing
// the pending operations it supersedes and checking the quotas. A request id
// recorded before answers its row with applied false.
func (e *Engine) record(ctx context.Context, s submission) (model.KernelNodeOperation, bool, error) {
	encoded, err := encodeOperation(s.operation)
	if err != nil {
		return model.KernelNodeOperation{}, false, err
	}
	var row model.KernelNodeOperation
	applied := false
	err = e.write(ctx, func(tx *gorm.DB) error {
		applied = false
		existing, found, err := loadByRequestID(tx, s.requestID)
		if err != nil || found {
			row = existing
			return err
		}
		if err := e.supersede(tx, s.kind, s.operation, s.resource, ""); err != nil {
			return err
		}
		if err := e.checkQuotas(tx, s.ownerID, s.targets); err != nil {
			return err
		}
		now := e.now()
		row = model.KernelNodeOperation{
			OperationID: newOperationID(), RequestID: s.requestID, Digest: s.digest, PackageID: s.ownerID,
			PackageGeneration: s.generation, SubmittedBy: s.submittedBy, Family: families[s.kind.family].name, Kind: s.kind.name,
			ResourceKey: s.resource, State: statePending, Operation: encoded, Reason: s.reason,
			CreatedAt: now, UpdatedAt: now, DeadlineAt: now.Add(e.timeout()),
		}
		if err := insertOperation(tx, &row, s.targets, now); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil && !applied {
		// A concurrent submission of the same request id by another package
		// won the unique index: answer its row.
		if existing, found, lookupErr := loadByRequestID(e.DB.WithContext(ctx), s.requestID); lookupErr == nil && found {
			return existing, false, nil
		}
	}
	return row, applied, err
}

func insertOperation(tx *gorm.DB, row *model.KernelNodeOperation, targets []*kernelnodeopsv1.NodeRef, now time.Time) error {
	if err := tx.Create(row).Error; err != nil {
		return err
	}
	if len(targets) > 0 {
		rows := make([]model.KernelNodeOperationTarget, 0, len(targets))
		for _, target := range targets {
			rows = append(rows, model.KernelNodeOperationTarget{
				OperationID: row.OperationID, NodeKind: nodeKindNames[target.GetKind()], NodeID: target.GetId(),
			})
		}
		if err := tx.CreateInBatches(&rows, 500).Error; err != nil {
			return err
		}
	}
	return appendEvent(tx, row, now)
}

// supersede ends the pending operations of k on resource that operation
// replaces (section 3.6): the last level-triggered operation wins. The
// children of sibling, a fan-out being recorded, never replace each other.
func (e *Engine) supersede(tx *gorm.DB, k *kind, operation *kernelnodeopsv1.OperationSpec, resource, sibling string) error {
	if k.supersedes == nil || resource == "" {
		return nil
	}
	query := tx.Where("resource_key = ? AND kind = ? AND state = ?", resource, k.name, statePending)
	if sibling != "" {
		query = query.Where("parent_operation_id <> ?", sibling)
	}
	var pending []model.KernelNodeOperation
	if err := query.Order("id").Find(&pending).Error; err != nil {
		return err
	}
	for i := range pending {
		older, err := decodeOperation(pending[i].Operation)
		if err != nil || !k.supersedes(operation, older) {
			continue
		}
		if _, err := e.endTx(tx, &pending[i], []string{statePending}, ending{state: stateSuperseded}); err != nil {
			return err
		}
	}
	return nil
}

// checkQuotas refuses an operation beyond its package's or a target node's
// quota of operations that have not ended. Fan-out children are not
// counted: their parent was.
func (e *Engine) checkQuotas(tx *gorm.DB, packageID string, targets []*kernelnodeopsv1.NodeRef) error {
	var count int64
	if err := tx.Model(&model.KernelNodeOperation{}).
		Where("package_id = ? AND parent_operation_id = '' AND state IN ?", packageID, activeStates).Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(e.packageQuota()) {
		return errQuota("package %s has %d node operations that have not ended (at most %d)", packageID, count, e.packageQuota())
	}
	for _, target := range targets {
		if err := tx.Model(&model.KernelNodeOperationTarget{}).
			Joins("JOIN v4_kernel_node_operation ON v4_kernel_node_operation.operation_id = v4_kernel_node_operation_target.operation_id").
			Where("v4_kernel_node_operation_target.node_kind = ? AND v4_kernel_node_operation_target.node_id = ?", nodeKindNames[target.GetKind()], target.GetId()).
			Where("v4_kernel_node_operation.parent_operation_id = '' AND v4_kernel_node_operation.state IN ?", activeStates).
			Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(e.nodeQuota()) {
			return errQuota("%s node %d has %d operations that have not ended (at most %d)",
				nodeKindNames[target.GetKind()], target.GetId(), count, e.nodeQuota())
		}
	}
	return nil
}

func loadByRequestID(db *gorm.DB, requestID string) (model.KernelNodeOperation, bool, error) {
	var rows []model.KernelNodeOperation
	if err := db.Where("request_id = ?", requestID).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return model.KernelNodeOperation{}, false, err
	}
	return rows[0], true, nil
}

func loadOperation(db *gorm.DB, operationID string) (model.KernelNodeOperation, bool, error) {
	var rows []model.KernelNodeOperation
	if err := db.Where("operation_id = ?", operationID).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return model.KernelNodeOperation{}, false, err
	}
	return rows[0], true, nil
}

// loadTargets returns the targets of operations, by operation id.
func loadTargets(db *gorm.DB, operationIDs []string) (map[string][]*kernelnodeopsv1.NodeRef, error) {
	targets := make(map[string][]*kernelnodeopsv1.NodeRef, len(operationIDs))
	if len(operationIDs) == 0 {
		return targets, nil
	}
	var rows []model.KernelNodeOperationTarget
	if err := db.Where("operation_id IN ?", operationIDs).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		targets[row.OperationID] = append(targets[row.OperationID], nodeRef(nodeKindByName(row.NodeKind), row.NodeID))
	}
	return targets, nil
}

func unixMillis(value *time.Time) int64 {
	if value == nil || value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

// toProto answers a ledger row as the contract's Operation.
func toProto(row *model.KernelNodeOperation, targets []*kernelnodeopsv1.NodeRef) *kernelnodeopsv1.Operation {
	operation := &kernelnodeopsv1.Operation{
		OperationId: row.OperationID, RequestId: row.RequestID, PackageId: row.PackageID, PackageGeneration: row.PackageGeneration,
		Family: familyByName(row.Family), Kind: row.Kind, Targets: targets, State: stateEnums[row.State],
		Channel: channelEnum(row.Channel), Attempt: row.Attempt, CreatedAtUnixMs: row.CreatedAt.UnixMilli(),
		AcceptedAtUnixMs: unixMillis(row.AcceptedAt), FinishedAtUnixMs: unixMillis(row.FinishedAt),
		DeadlineUnixMs: row.DeadlineAt.UnixMilli(), NodeRevision: row.NodeRevision, ParentOperationId: row.ParentOperationID,
	}
	if k := kinds[row.Kind]; (k != nil && k.fanOut) || row.FanOutTotal > 0 {
		operation.FanOut = &kernelnodeopsv1.FanOut{
			Total: row.FanOutTotal, Succeeded: row.FanOutSucceeded, Failed: row.FanOutFailed, Pending: row.FanOutPending,
		}
	}
	if row.Result != "" {
		result := &kernelnodeopsv1.OperationResult{}
		if err := loadJSON.Unmarshal([]byte(row.Result), result); err == nil {
			operation.Result = result
		}
	}
	if row.ErrorCode != "" || row.ErrorMessage != "" {
		operation.Error = &kernelnodeopsv1.OperationError{
			Code: errorCodeEnum(row.ErrorCode), Message: row.ErrorMessage, Retryable: row.ErrorRetryable,
		}
	}
	return operation
}

// answer loads row's targets and answers it.
func answer(db *gorm.DB, row *model.KernelNodeOperation) (*kernelnodeopsv1.Operation, error) {
	targets, err := loadTargets(db, []string{row.OperationID})
	if err != nil {
		return nil, err
	}
	return toProto(row, targets[row.OperationID]), nil
}

// Page tokens are the id of the last operation answered.
func encodePageToken(id uint64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatUint(id, 10)))
}

var errPageToken = errors.New("page token is invalid")

func decodePageToken(token string) (uint64, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, errPageToken
	}
	id, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil || id == 0 {
		return 0, errPageToken
	}
	return id, nil
}

// targetFilter limits query to the operations on node.
func targetFilter(query *gorm.DB, node *kernelnodeopsv1.NodeRef) *gorm.DB {
	return query.Where("operation_id IN (?)", query.Session(&gorm.Session{NewDB: true}).
		Model(&model.KernelNodeOperationTarget{}).Select("operation_id").
		Where("node_kind = ? AND node_id = ?", nodeKindNames[node.GetKind()], node.GetId()))
}
