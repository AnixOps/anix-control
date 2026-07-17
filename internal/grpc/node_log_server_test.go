package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func nodeLogRequestForTest(nodeID uint32, entries ...*dynamicpb.Message) *dynamicpb.Message {
	req := newNodeLogBatchMessage()
	req.Set(nodeLogBatchNodeIDField, protoreflect.ValueOfUint32(nodeID))

	logs := req.Mutable(nodeLogBatchLogsField).List()
	for _, entry := range entries {
		logs.Append(protoreflect.ValueOfMessage(entry))
	}
	return req
}

func nodeLogEntryForTest(message string, timestamp int64) *dynamicpb.Message {
	entry := dynamicpb.NewMessage(nodeLogEntryDesc)
	entry.Set(nodeLogEntryLevelField, protoreflect.ValueOfString("warn"))
	entry.Set(nodeLogEntrySourceField, protoreflect.ValueOfString("unit-test"))
	entry.Set(nodeLogEntryMessageField, protoreflect.ValueOfString(message))
	entry.Set(nodeLogEntryTimestampField, protoreflect.ValueOfInt64(timestamp))
	entry.Set(nodeLogEntryFieldsJSONField, protoreflect.ValueOfString(`{"pid":123}`))
	entry.Set(nodeLogEntryTraceIDField, protoreflect.ValueOfString("trace-1"))
	return entry
}

func TestNodeLogDynamicMessageAndTimestampNormalization(t *testing.T) {
	const (
		nodeID    = uint32(42)
		unixSec   = int64(1_700_000_000)
		unixMilli = int64(1_700_000_000_123)
	)

	entry := nodeLogEntryForTest("  started  ", unixMilli)
	req := nodeLogRequestForTest(nodeID, entry)

	assert.Equal(t, uint64(nodeID), req.Get(nodeLogBatchNodeIDField).Uint())
	logs := req.Get(nodeLogBatchLogsField).List()
	require.Equal(t, 1, logs.Len())
	gotEntry := logs.Get(0).Message()
	assert.Equal(t, "warn", gotEntry.Get(nodeLogEntryLevelField).String())
	assert.Equal(t, "unit-test", gotEntry.Get(nodeLogEntrySourceField).String())
	assert.Equal(t, "  started  ", gotEntry.Get(nodeLogEntryMessageField).String())
	assert.Equal(t, unixMilli, gotEntry.Get(nodeLogEntryTimestampField).Int())

	assert.Nil(t, normalizeLoggedAt(0))
	assert.Equal(t, unixSec, normalizeLoggedAt(unixSec).Unix())
	assert.Equal(t, unixMilli, normalizeLoggedAt(unixMilli).UnixMilli())
}

func TestNodeLogGRPCServerReportLogsValidationAndNotFound(t *testing.T) {
	requireInMemoryDatabase(t)
	defer requireDatabaseClosed(t)
	requireAutoMigrate(t, &model.Node{}, &model.NodeLog{})

	srv := NewNodeLogGRPCServer()

	_, err := srv.ReportLogs(context.Background(), nodeLogRequestForTest(0))
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	req := nodeLogRequestForTest(404, nodeLogEntryForTest("node is missing", time.Now().Unix()))
	_, err = srv.ReportLogs(context.Background(), req)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestNodeLogGRPCServerReportLogsPersistsNonBlankEntries(t *testing.T) {
	requireInMemoryDatabase(t)
	defer requireDatabaseClosed(t)
	requireAutoMigrate(t, &model.Node{}, &model.NodeLog{})

	node := model.Node{Name: "node-log-test", Host: "127.0.0.1", Port: 8443}
	require.NoError(t, database.Get().Create(&node).Error)

	srv := NewNodeLogGRPCServer()
	req := nodeLogRequestForTest(
		uint32(node.ID),
		nodeLogEntryForTest("  persisted log  ", 1_700_000_000_123),
		nodeLogEntryForTest("   ", time.Now().Unix()),
	)

	resp, err := srv.ReportLogs(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "logs recorded", resp.Message)

	var logs []model.NodeLog
	require.NoError(t, database.Get().Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, node.ID, logs[0].NodeID)
	assert.Equal(t, "warning", logs[0].Level)
	assert.Equal(t, "unit-test", logs[0].Source)
	assert.Equal(t, "persisted log", logs[0].Message)
	assert.Equal(t, `{"pid":123}`, logs[0].FieldsJSON)
	require.NotNil(t, logs[0].LoggedAt)
	assert.Equal(t, int64(1_700_000_000_123), logs[0].LoggedAt.UnixMilli())
}

func TestNodeLogGRPCServerReportLogsUpdatesWireGuardRuntimeHealth(t *testing.T) {
	requireInMemoryDatabase(t)
	defer requireDatabaseClosed(t)
	requireAutoMigrate(t, &model.Node{}, &model.NodeLog{})

	node := model.Node{Name: "wireguard-health-test", Host: "127.0.0.1", Port: 8443}
	require.NoError(t, database.Get().Create(&node).Error)

	entry := nodeLogEntryForTest("wireguard runtime unhealthy", time.Now().Unix())
	entry.Set(nodeLogEntrySourceField, protoreflect.ValueOfString("wireguard"))
	entry.Set(nodeLogEntryFieldsJSONField, protoreflect.ValueOfString(`{"runtime_healthy":false,"runtime_error":"gost exited"}`))
	_, err := NewNodeLogGRPCServer().ReportLogs(context.Background(), nodeLogRequestForTest(uint32(node.ID), entry))
	require.NoError(t, err)

	var updated model.Node
	require.NoError(t, database.Get().First(&updated, node.ID).Error)
	assert.False(t, updated.RuntimeHealthy)
	assert.Equal(t, "gost exited", updated.RuntimeError)
	assert.NotNil(t, updated.RuntimeCheckedAt)
}

type stubNodeLogServiceServer struct {
	called bool
	resp   *pb.StatusResponse
	err    error
}

func (s *stubNodeLogServiceServer) ReportLogs(context.Context, *dynamicpb.Message) (*pb.StatusResponse, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

func TestNodeLogServiceHandlerDecodeAndInterceptorPaths(t *testing.T) {
	decodeErr := errors.New("decode failed")
	_, err := _NodeLogService_ReportLogs_Handler(
		&stubNodeLogServiceServer{},
		context.Background(),
		func(any) error { return decodeErr },
		nil,
	)
	require.ErrorIs(t, err, decodeErr)

	stub := &stubNodeLogServiceServer{resp: &pb.StatusResponse{Success: true, Code: 200}}
	resp, err := _NodeLogService_ReportLogs_Handler(
		stub,
		context.Background(),
		func(v any) error {
			v.(*dynamicpb.Message).Set(nodeLogBatchNodeIDField, protoreflect.ValueOfUint32(7))
			return nil
		},
		nil,
	)
	require.NoError(t, err)
	assert.True(t, stub.called)
	assert.Equal(t, stub.resp, resp)

	stub = &stubNodeLogServiceServer{resp: &pb.StatusResponse{Success: true, Message: "intercepted"}}
	resp, err = _NodeLogService_ReportLogs_Handler(
		stub,
		context.Background(),
		func(v any) error {
			v.(*dynamicpb.Message).Set(nodeLogBatchNodeIDField, protoreflect.ValueOfUint32(9))
			return nil
		},
		func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			assert.Equal(t, NodeLogService_ReportLogs_FullMethodName, info.FullMethod)
			msg, ok := req.(*dynamicpb.Message)
			require.True(t, ok)
			assert.Equal(t, uint64(9), msg.Get(nodeLogBatchNodeIDField).Uint())
			return handler(ctx, req)
		},
	)
	require.NoError(t, err)
	assert.True(t, stub.called)
	assert.Equal(t, stub.resp, resp)
}

type fakeNodeLogClientConn struct {
	invoked bool
	method  string
	resp    *pb.StatusResponse
	err     error
}

func (f *fakeNodeLogClientConn) Invoke(_ context.Context, method string, _ any, reply any, _ ...grpc.CallOption) error {
	f.invoked = true
	f.method = method
	if f.err != nil {
		return f.err
	}
	out := reply.(*pb.StatusResponse)
	out.Success = f.resp.Success
	out.Message = f.resp.Message
	out.Code = f.resp.Code
	return nil
}

func (f *fakeNodeLogClientConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("streaming is not implemented in fakeNodeLogClientConn")
}

func TestNodeLogServiceClientReportLogs(t *testing.T) {
	conn := &fakeNodeLogClientConn{resp: &pb.StatusResponse{Success: true, Message: "ok", Code: 200}}
	client := NewNodeLogServiceClient(conn)

	resp, err := client.ReportLogs(context.Background(), nodeLogRequestForTest(1))
	require.NoError(t, err)
	assert.True(t, conn.invoked)
	assert.Equal(t, NodeLogService_ReportLogs_FullMethodName, conn.method)
	assert.Equal(t, conn.resp, resp)

	invokeErr := errors.New("invoke failed")
	conn.err = invokeErr
	resp, err = client.ReportLogs(context.Background(), nodeLogRequestForTest(1))
	require.ErrorIs(t, err, invokeErr)
	assert.Nil(t, resp)
}
