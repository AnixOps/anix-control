// Package kernelnodeops serves the KernelNodeOps contract
// (sdk/api/kernelnodeops/v1, docs/architecture/node-ops-service.md) to
// official packages: typed, idempotent operations on nodes that the kernel
// records in its ledger (v4_kernel_node_operation), carries out with the
// executor of each kind and answers by polling and a watch stream.
//
// Every call is authorized against the calling host's current generation:
// a submission for its operation family's capability, the reads, watches
// and cancellations for any one of the five, and they answer only the
// operations the caller's package owns. A kind without an executor is not
// served: SubmitOperation answers UNIMPLEMENTED and records nothing.
package kernelnodeops

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/sealedsecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const (
	maxRequestID   = 128
	maxReasonText  = 255
	maxBindingSize = 4096
	// MaxWait bounds a SubmitOperation wait (wait_timeout_ms).
	MaxWait          = time.Minute
	defaultListLimit = 100
	maxListLimit     = 500
	watchBatch       = 500
	watchPoll        = time.Second
	// cancelWait is how long CancelOperation waits for a started operation
	// to end before it answers the operation as it is.
	cancelWait = 2 * time.Second
)

// watchReauthorize bounds how long a watch keeps streaming after its
// package lost every family or its generation was fenced.
var watchReauthorize = 30 * time.Second

// splitTables are the tables of the node credential split (section 4.1),
// as GetCapabilities answers them.
var splitTables = []string{
	"v2_node", "v2_authorized_key", "v2_forward_node", "v2_forward_clean_agent",
	"v2_forward_runtime_job", "v2_node_protocol", "v2_wireguard_peer",
}

// Authorizer admits a calling host to a capability.
type Authorizer interface {
	AuthorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string) error
}

// Server holds what every host's KernelNodeOps calls share.
type Server struct {
	Engine     *Engine
	Authorizer Authorizer
	// SplitPhases answers the node credential split's phase per table. Nil
	// answers LEGACY for every table: nothing is split before NO-2.
	SplitPhases func(ctx context.Context) ([]*kernelnodeopsv1.TableSplitState, error)
	// Agents are the live agent sessions the session RPCs answer; nil reads
	// the defaults registered with UseAgentStreams and UseWebSocketAgents.
	Agents *AgentSources
}

// For returns the KernelNodeOps server that host reaches; it has the shape
// of packagebridge.KernelNodeOpsProvider.
func (s *Server) For(host packagebridge.HostIdentity) kernelnodeopsv1.KernelNodeOpsServer {
	return &hostServer{server: s, host: host}
}

type hostServer struct {
	kernelnodeopsv1.UnimplementedKernelNodeOpsServer
	server *Server
	host   packagebridge.HostIdentity
}

func (h *hostServer) configured() error {
	if h.server == nil || h.server.Engine == nil || h.server.Engine.DB == nil || h.server.Authorizer == nil {
		return status.Error(codes.Unavailable, "kernel node operations are not configured")
	}
	return nil
}

func (h *hostServer) db(ctx context.Context) *gorm.DB {
	return h.server.Engine.DB.WithContext(ctx)
}

// allowed reports whether the host holds capability: a fenced generation is
// PermissionDenied, a missing capability false.
func (h *hostServer) allowed(ctx context.Context, capability string) (bool, error) {
	err := h.server.Authorizer.AuthorizeCapability(ctx, h.host, capability)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, packagebridge.ErrHostFenced):
		return false, status.Error(codes.PermissionDenied, "package host generation is fenced")
	case errors.Is(err, service.ErrCapabilityNotAuthorized):
		return false, nil
	default:
		return false, status.Error(codes.Unavailable, "kernel node operations authorization failed")
	}
}

// authorize requires one family's capability.
func (h *hostServer) authorize(ctx context.Context, family kernelnodeopsv1.OperationFamily) error {
	if err := h.configured(); err != nil {
		return err
	}
	capability := families[family].capability
	ok, err := h.allowed(ctx, capability)
	if err != nil {
		return err
	}
	if !ok {
		return status.Errorf(codes.PermissionDenied, "package is not authorized for %s", capability)
	}
	return nil
}

// authorizeAny requires any one of the five family capabilities.
func (h *hostServer) authorizeAny(ctx context.Context) error {
	if err := h.configured(); err != nil {
		return err
	}
	for _, family := range familyOrder {
		ok, err := h.allowed(ctx, families[family].capability)
		if err != nil || ok {
			return err
		}
	}
	return status.Error(codes.PermissionDenied, "package holds no kernel.nodeops capability")
}

func failure(operation string, err error) error {
	if status.Code(err) != codes.Unknown {
		return err
	}
	return status.Errorf(codes.Internal, "%s failed", operation)
}

func checkRequestID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxRequestID || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return "", status.Errorf(codes.InvalidArgument, "request_id is required and at most %d bytes of text", maxRequestID)
	}
	if strings.HasPrefix(value, fanOutRequestIDPrefix) {
		return "", status.Errorf(codes.InvalidArgument, "request ids starting %q are the kernel's", fanOutRequestIDPrefix)
	}
	return value, nil
}

func waitTimeout(milliseconds uint32) time.Duration {
	if milliseconds == 0 {
		return MaxWait
	}
	return min(time.Duration(milliseconds)*time.Millisecond, MaxWait)
}

// waitDone is the condition a wait mode waits for.
func waitDone(mode kernelnodeopsv1.WaitMode) func(*model.KernelNodeOperation) bool {
	switch mode {
	case kernelnodeopsv1.WaitMode_WAIT_MODE_ACCEPTED:
		return func(op *model.KernelNodeOperation) bool { return op.State == stateRunning || Terminal(op.State) }
	case kernelnodeopsv1.WaitMode_WAIT_MODE_TERMINAL:
		return func(op *model.KernelNodeOperation) bool { return Terminal(op.State) }
	}
	return nil
}

// SubmitOperation records one operation and starts it, or answers the
// operation its request id recorded before.
func (h *hostServer) SubmitOperation(ctx context.Context, request *kernelnodeopsv1.SubmitOperationRequest) (*kernelnodeopsv1.SubmitOperationResponse, error) {
	spec := request.GetOperation()
	if spec == nil {
		if err := h.configured(); err != nil {
			return nil, err
		}
		return nil, status.Error(codes.InvalidArgument, "operation is required")
	}
	k, err := kindOf(spec)
	if errors.Is(err, errUnknownKind) {
		if err := h.authorizeAny(ctx); err != nil {
			return nil, err
		}
		return nil, status.Error(codes.Unimplemented, "the operation's kind is not known to this kernel")
	}
	if err != nil {
		if configured := h.configured(); configured != nil {
			return nil, configured
		}
		return nil, err
	}
	if err := h.authorize(ctx, k.family); err != nil {
		return nil, err
	}
	if hasUnknownFields(spec.ProtoReflect()) {
		return nil, status.Error(codes.InvalidArgument, "the operation carries fields this kernel does not know")
	}
	requestID, err := checkRequestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	if err := checkText(request.GetReason(), "reason", maxReasonText, false); err != nil {
		return nil, err
	}
	if sealedsecrets.ContainsHandle(requestID) || sealedsecrets.ContainsHandle(request.GetReason()) {
		return nil, status.Error(codes.InvalidArgument, "request_id and reason never hold a sealed handle")
	}
	if _, known := kernelnodeopsv1.WaitMode_name[int32(request.GetWait())]; !known {
		return nil, status.Error(codes.InvalidArgument, "wait is unknown")
	}
	if len(request.GetRequest().GetBridgeCapability()) > maxBindingSize {
		return nil, status.Error(codes.InvalidArgument, "request binding is too large")
	}
	if err := k.check(spec); err != nil {
		return nil, err
	}
	operation := canonical(spec)
	dg, err := digest(k.name, operation)
	if err != nil {
		return nil, failure("submit operation", err)
	}
	db := h.db(ctx)
	existing, found, err := loadByRequestID(db, requestID)
	if err != nil {
		return nil, failure("submit operation", err)
	}
	if found {
		return h.replay(ctx, request, &existing, dg)
	}
	engine := h.server.Engine
	executor, served := engine.Executors.Lookup(k.name)
	if !served {
		return nil, status.Errorf(codes.Unimplemented, "this kernel does not execute %s operations yet; GetCapabilities lists the kinds it does", k.name)
	}
	bound, err := h.verifyBinding(ctx, request.GetRequest())
	if err != nil {
		return nil, err
	}
	resolved, err := k.resolve(db, operation)
	if err != nil {
		return nil, failure("submit operation", err)
	}
	preparer, prepares := executor.(Preparer)
	if prepares {
		prepared := &Submission{
			Host: h.host, Kind: k.name, Operation: proto.Clone(spec).(*kernelnodeopsv1.OperationSpec),
			Binding: request.GetRequest().GetBridgeCapability(), Request: bound, Targets: resolved.targets,
			store: engine.secretStore(), db: db,
		}
		if err := preparer.Prepare(ctx, prepared); err != nil {
			return nil, failure("prepare operation", err)
		}
		if preparedKind, err := kindOf(prepared.Operation); err != nil || preparedKind != k {
			return nil, status.Error(codes.Internal, "prepare operation changed its kind")
		}
		operation = canonical(prepared.Operation)
	}
	// The ledger never holds a sealed handle: only a Preparer resolves one.
	if holdsHandle(operation.ProtoReflect()) {
		if prepares {
			return nil, status.Error(codes.Internal, "prepare operation left a sealed handle")
		}
		return nil, status.Errorf(codes.InvalidArgument, "%s operations resolve no sealed handle", k.name)
	}
	retained := engine.retainBinding(h.host.PackageID, requestID, bound)
	row, applied, err := engine.record(ctx, submission{
		ownerID: h.host.PackageID, generation: h.host.Generation, submittedBy: submitter(h.host),
		requestID: requestID, kind: k, operation: operation, digest: dg, targets: resolved.targets,
		resource: resolved.resource, reason: request.GetReason(),
	})
	if err != nil || !applied {
		if retained {
			engine.dropBinding(h.host.PackageID, requestID)
		}
	}
	if err != nil {
		return nil, failure("submit operation", err)
	}
	if !applied {
		return h.replay(ctx, request, &row, dg)
	}
	engine.changed()
	engine.Wake()
	return h.answerSubmission(ctx, request, &row, true)
}

func submitter(host packagebridge.HostIdentity) string {
	return host.PackageID + "@" + strconv.FormatUint(host.Generation, 10)
}

// replay answers a request id recorded before: the first operation, as it
// is now, when it is the same operation of the same package; anything else
// is FAILED_PRECONDITION.
func (h *hostServer) replay(ctx context.Context, request *kernelnodeopsv1.SubmitOperationRequest, existing *model.KernelNodeOperation, dg string) (*kernelnodeopsv1.SubmitOperationResponse, error) {
	if existing.PackageID != h.host.PackageID || existing.Digest != dg {
		return nil, status.Errorf(codes.FailedPrecondition, "request_id %q was used for another operation", existing.RequestID)
	}
	return h.answerSubmission(ctx, request, existing, false)
}

func (h *hostServer) answerSubmission(ctx context.Context, request *kernelnodeopsv1.SubmitOperationRequest, row *model.KernelNodeOperation, applied bool) (*kernelnodeopsv1.SubmitOperationResponse, error) {
	if done := waitDone(request.GetWait()); done != nil {
		waited, err := h.server.Engine.waitFor(ctx, row.OperationID, waitTimeout(request.GetWaitTimeoutMs()), done)
		if err != nil {
			return nil, failure("submit operation", err)
		}
		row = &waited
	}
	operation, err := answer(h.db(ctx), row)
	if err != nil {
		return nil, failure("submit operation", err)
	}
	// The handles an operation minted are answered once, to a call bound
	// to the request they were minted for.
	if Terminal(row.State) && h.server.Engine.hasReveal(row.OperationID) {
		if bound, err := h.verifyBinding(ctx, request.GetRequest()); err == nil {
			if revealed := h.server.Engine.takeReveal(row.OperationID, bound); revealed != nil {
				operation.Result = revealed
			}
		}
	}
	return &kernelnodeopsv1.SubmitOperationResponse{Applied: applied, Operation: operation}, nil
}

// owned loads one of the caller's operations; another package's is
// NotFound.
func (h *hostServer) owned(ctx context.Context, column, value string) (model.KernelNodeOperation, error) {
	var rows []model.KernelNodeOperation
	if err := h.db(ctx).Where(column+" = ? AND package_id = ?", value, h.host.PackageID).Limit(1).Find(&rows).Error; err != nil {
		return model.KernelNodeOperation{}, failure("get operation", err)
	}
	if len(rows) == 0 {
		return model.KernelNodeOperation{}, status.Error(codes.NotFound, "operation not found")
	}
	return rows[0], nil
}

// GetOperation answers one of the caller's operations.
func (h *hostServer) GetOperation(ctx context.Context, request *kernelnodeopsv1.GetOperationRequest) (*kernelnodeopsv1.GetOperationResponse, error) {
	if err := h.authorizeAny(ctx); err != nil {
		return nil, err
	}
	var column, value string
	switch selector := request.GetSelector().(type) {
	case *kernelnodeopsv1.GetOperationRequest_OperationId:
		column, value = "operation_id", selector.OperationId
	case *kernelnodeopsv1.GetOperationRequest_RequestId:
		column, value = "request_id", selector.RequestId
	}
	if value == "" || len(value) > maxRequestID {
		return nil, status.Error(codes.InvalidArgument, "operation_id or request_id is required")
	}
	row, err := h.owned(ctx, column, value)
	if err != nil {
		return nil, err
	}
	operation, err := answer(h.db(ctx), &row)
	if err != nil {
		return nil, failure("get operation", err)
	}
	return &kernelnodeopsv1.GetOperationResponse{Operation: operation}, nil
}

func familyNames(values []kernelnodeopsv1.OperationFamily) ([]string, error) {
	names := make([]string, 0, len(values))
	for _, family := range values {
		entry, ok := families[family]
		if !ok {
			return nil, status.Error(codes.InvalidArgument, "families holds an unknown family")
		}
		names = append(names, entry.name)
	}
	return names, nil
}

// ListOperations pages through the caller's operations, newest first.
func (h *hostServer) ListOperations(ctx context.Context, request *kernelnodeopsv1.ListOperationsRequest) (*kernelnodeopsv1.ListOperationsResponse, error) {
	if err := h.authorizeAny(ctx); err != nil {
		return nil, err
	}
	query := h.db(ctx).Model(&model.KernelNodeOperation{}).Where("package_id = ?", h.host.PackageID)
	names, err := familyNames(request.GetFamilies())
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		query = query.Where("family IN ?", names)
	}
	if len(request.GetStates()) > 0 {
		states := make([]string, 0, len(request.GetStates()))
		for _, state := range request.GetStates() {
			name, ok := stateName(state)
			if !ok {
				return nil, status.Error(codes.InvalidArgument, "states holds an unknown state")
			}
			states = append(states, name)
		}
		query = query.Where("state IN ?", states)
	}
	if target := request.GetTarget(); target != nil {
		if _, known := nodeKindNames[target.GetKind()]; !known || checkID(target.GetId(), "target.id") != nil {
			return nil, status.Error(codes.InvalidArgument, "target needs a node kind and id")
		}
		query = targetFilter(query, target)
	}
	after, err := decodePageToken(request.GetPageToken())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if after > 0 {
		query = query.Where("id < ?", after)
	}
	limit := int(request.GetLimit())
	if limit <= 0 {
		limit = defaultListLimit
	}
	limit = min(limit, maxListLimit)
	var rows []model.KernelNodeOperation
	if err := query.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, failure("list operations", err)
	}
	operations, err := answerRows(h.db(ctx), rows)
	if err != nil {
		return nil, failure("list operations", err)
	}
	response := &kernelnodeopsv1.ListOperationsResponse{Operations: operations}
	if len(rows) == limit {
		response.NextPageToken = encodePageToken(rows[len(rows)-1].ID)
	}
	return response, nil
}

func answerRows(db *gorm.DB, rows []model.KernelNodeOperation) ([]*kernelnodeopsv1.Operation, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.OperationID)
	}
	targets, err := loadTargets(db, ids)
	if err != nil {
		return nil, err
	}
	operations := make([]*kernelnodeopsv1.Operation, 0, len(rows))
	for i := range rows {
		operations = append(operations, toProto(&rows[i], targets[rows[i].OperationID]))
	}
	return operations, nil
}

// WatchOperations streams every change of the caller's operations after a
// cursor, each with the operation as it is. A cursor the event log no
// longer holds (older than its retention, or past its end) gets RESYNC with
// the log's current cursor: the client lists again and watches from it.
func (h *hostServer) WatchOperations(request *kernelnodeopsv1.WatchOperationsRequest, stream grpc.ServerStreamingServer[kernelnodeopsv1.OperationEvent]) error {
	ctx := stream.Context()
	if err := h.authorizeAny(ctx); err != nil {
		return err
	}
	names, err := familyNames(request.GetFamilies())
	if err != nil {
		return err
	}
	engine := h.server.Engine
	cursor := request.GetAfterCursor()
	authorized := time.Now()
	for {
		if time.Since(authorized) > watchReauthorize {
			if err := h.authorizeAny(ctx); err != nil {
				return err
			}
			authorized = time.Now()
		}
		changed := engine.notify.wait()
		events, resync, latest, err := eventsAfter(h.db(ctx), h.host.PackageID, names, cursor)
		if err != nil {
			return failure("watch operations", err)
		}
		if resync {
			return stream.Send(&kernelnodeopsv1.OperationEvent{Cursor: latest, Kind: kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_RESYNC})
		}
		if err := h.sendEvents(ctx, stream, events); err != nil {
			return err
		}
		if len(events) > 0 {
			cursor = events[len(events)-1].ID
		}
		if latest > cursor {
			// Events of other packages or families: skip past them.
			cursor = latest
		}
		if len(events) == watchBatch {
			continue
		}
		timer := time.NewTimer(watchPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// sendEvents sends one CHANGED event per operation of events, at the
// cursor of its last event: each carries the operation as it is, so its
// earlier events add nothing.
func (h *hostServer) sendEvents(ctx context.Context, stream grpc.ServerStreamingServer[kernelnodeopsv1.OperationEvent], events []model.KernelNodeOperationEvent) error {
	if len(events) == 0 {
		return nil
	}
	last := map[string]uint64{}
	ids := make([]string, 0, len(events))
	for _, event := range events {
		if _, seen := last[event.OperationID]; !seen {
			ids = append(ids, event.OperationID)
		}
		last[event.OperationID] = event.ID
	}
	var rows []model.KernelNodeOperation
	if err := h.db(ctx).Where("operation_id IN ? AND package_id = ?", ids, h.host.PackageID).Find(&rows).Error; err != nil {
		return failure("watch operations", err)
	}
	operations, err := answerRows(h.db(ctx), rows)
	if err != nil {
		return failure("watch operations", err)
	}
	byID := make(map[string]*kernelnodeopsv1.Operation, len(operations))
	for _, operation := range operations {
		byID[operation.GetOperationId()] = operation
	}
	for _, event := range events {
		if last[event.OperationID] != event.ID {
			continue
		}
		operation, ok := byID[event.OperationID]
		if !ok {
			// Pruned since: nothing to say about it.
			continue
		}
		if err := stream.Send(&kernelnodeopsv1.OperationEvent{
			Cursor: event.ID, Kind: kernelnodeopsv1.OperationEventKind_OPERATION_EVENT_KIND_CHANGED, Operation: operation,
		}); err != nil {
			return err
		}
	}
	return nil
}

// eventsAfter returns up to watchBatch of owner's events after cursor and
// the log's latest cursor. resync is true when the log no longer holds the
// events after cursor: they were pruned, or cursor is past its end.
func eventsAfter(db *gorm.DB, owner string, families []string, cursor uint64) ([]model.KernelNodeOperationEvent, bool, uint64, error) {
	var bounds struct {
		Oldest uint64
		Latest uint64
	}
	if err := db.Model(&model.KernelNodeOperationEvent{}).Select("COALESCE(MIN(id), 0) AS oldest, COALESCE(MAX(id), 0) AS latest").
		Scan(&bounds).Error; err != nil {
		return nil, false, 0, err
	}
	if cursor > 0 && (cursor > bounds.Latest || (bounds.Oldest > 0 && bounds.Oldest > cursor+1)) {
		return nil, true, bounds.Latest, nil
	}
	query := db.Where("id > ? AND id <= ? AND package_id = ?", cursor, bounds.Latest, owner)
	if len(families) > 0 {
		query = query.Where("family IN ?", families)
	}
	var events []model.KernelNodeOperationEvent
	if err := query.Order("id").Limit(watchBatch).Find(&events).Error; err != nil {
		return nil, false, 0, err
	}
	if len(events) == watchBatch {
		bounds.Latest = events[len(events)-1].ID
	}
	return events, false, bounds.Latest, nil
}

// CancelOperation asks one of the caller's operations to stop.
func (h *hostServer) CancelOperation(ctx context.Context, request *kernelnodeopsv1.CancelOperationRequest) (*kernelnodeopsv1.CancelOperationResponse, error) {
	if err := h.authorizeAny(ctx); err != nil {
		return nil, err
	}
	id := request.GetOperationId()
	if id == "" || len(id) > maxRequestID {
		return nil, status.Error(codes.InvalidArgument, "operation_id is required")
	}
	if err := checkText(request.GetReason(), "reason", maxReasonText, false); err != nil {
		return nil, err
	}
	row, err := h.owned(ctx, "operation_id", id)
	if err != nil {
		return nil, err
	}
	engine := h.server.Engine
	if !Terminal(row.State) {
		if err := engine.cancel(ctx, row.OperationID, request.GetReason()); err != nil {
			return nil, failure("cancel operation", err)
		}
		row, err = engine.waitFor(ctx, row.OperationID, cancelWait, func(op *model.KernelNodeOperation) bool { return Terminal(op.State) })
		if err != nil {
			return nil, failure("cancel operation", err)
		}
	}
	operation, err := answer(h.db(ctx), &row)
	if err != nil {
		return nil, failure("cancel operation", err)
	}
	return &kernelnodeopsv1.CancelOperationResponse{Operation: operation}, nil
}

// GetCapabilities answers the families the caller holds, the kinds this
// kernel executes and the credential split's phase per table. It needs no
// family, but a fenced generation is refused.
func (h *hostServer) GetCapabilities(ctx context.Context, _ *kernelnodeopsv1.GetCapabilitiesRequest) (*kernelnodeopsv1.GetCapabilitiesResponse, error) {
	if err := h.configured(); err != nil {
		return nil, err
	}
	response := &kernelnodeopsv1.GetCapabilitiesResponse{Kinds: h.server.Engine.Executors.Kinds()}
	for _, family := range familyOrder {
		ok, err := h.allowed(ctx, families[family].capability)
		if err != nil {
			return nil, err
		}
		if ok {
			response.GrantedFamilies = append(response.GrantedFamilies, family)
		}
	}
	if h.server.SplitPhases != nil {
		tables, err := h.server.SplitPhases(ctx)
		if err != nil {
			return nil, failure("get capabilities", err)
		}
		response.Tables = tables
		return response, nil
	}
	for _, table := range splitTables {
		response.Tables = append(response.Tables, &kernelnodeopsv1.TableSplitState{
			Table: table, Phase: kernelnodeopsv1.SecretSplitPhase_SECRET_SPLIT_PHASE_LEGACY,
		})
	}
	return response, nil
}
