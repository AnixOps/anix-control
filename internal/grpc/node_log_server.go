package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	pb "github.com/anixops/v2board/api/grpc/v2boardpb"
	"github.com/anixops/v2board/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"gorm.io/gorm"
)

const NodeLogService_ReportLogs_FullMethodName = "/v2board.NodeLogService/ReportLogs"

var (
	nodeLogEntryDesc protoreflect.MessageDescriptor
	nodeLogBatchDesc protoreflect.MessageDescriptor

	nodeLogBatchNodeIDField protoreflect.FieldDescriptor
	nodeLogBatchLogsField   protoreflect.FieldDescriptor

	nodeLogEntryLevelField      protoreflect.FieldDescriptor
	nodeLogEntrySourceField     protoreflect.FieldDescriptor
	nodeLogEntryMessageField    protoreflect.FieldDescriptor
	nodeLogEntryTimestampField  protoreflect.FieldDescriptor
	nodeLogEntryFieldsJSONField protoreflect.FieldDescriptor
	nodeLogEntryTraceIDField    protoreflect.FieldDescriptor
)

func init() {
	fileProto := &descriptorpb.FileDescriptorProto{
		Name:    strPtr("api/grpc/node_log_runtime.proto"),
		Package: strPtr("v2board"),
		Syntax:  strPtr("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: strPtr("NodeLogEntry"),
				Field: []*descriptorpb.FieldDescriptorProto{
					newStringField("level", 1),
					newStringField("source", 2),
					newStringField("message", 3),
					newInt64Field("timestamp", 4),
					newStringField("fields_json", 5),
					newStringField("trace_id", 6),
				},
			},
			{
				Name: strPtr("NodeLogBatchRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					newUint32Field("node_id", 1),
					newRepeatedMessageField("logs", 2, ".v2board.NodeLogEntry"),
				},
			},
		},
	}

	fileDesc, err := protodesc.NewFile(fileProto, nil)
	if err != nil {
		panic(err)
	}

	nodeLogEntryDesc = fileDesc.Messages().ByName("NodeLogEntry")
	nodeLogBatchDesc = fileDesc.Messages().ByName("NodeLogBatchRequest")

	nodeLogBatchNodeIDField = nodeLogBatchDesc.Fields().ByName("node_id")
	nodeLogBatchLogsField = nodeLogBatchDesc.Fields().ByName("logs")

	nodeLogEntryLevelField = nodeLogEntryDesc.Fields().ByName("level")
	nodeLogEntrySourceField = nodeLogEntryDesc.Fields().ByName("source")
	nodeLogEntryMessageField = nodeLogEntryDesc.Fields().ByName("message")
	nodeLogEntryTimestampField = nodeLogEntryDesc.Fields().ByName("timestamp")
	nodeLogEntryFieldsJSONField = nodeLogEntryDesc.Fields().ByName("fields_json")
	nodeLogEntryTraceIDField = nodeLogEntryDesc.Fields().ByName("trace_id")
}

func strPtr(v string) *string {
	return &v
}

func int32Ptr(v int32) *int32 {
	return &v
}

func labelPtr(v descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto_Label {
	return &v
}

func typePtr(v descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto_Type {
	return &v
}

func newStringField(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   strPtr(name),
		Number: int32Ptr(number),
		Label:  labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
		Type:   typePtr(descriptorpb.FieldDescriptorProto_TYPE_STRING),
	}
}

func newInt64Field(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   strPtr(name),
		Number: int32Ptr(number),
		Label:  labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
		Type:   typePtr(descriptorpb.FieldDescriptorProto_TYPE_INT64),
	}
}

func newUint32Field(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   strPtr(name),
		Number: int32Ptr(number),
		Label:  labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
		Type:   typePtr(descriptorpb.FieldDescriptorProto_TYPE_UINT32),
	}
}

func newRepeatedMessageField(name string, number int32, typeName string) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:     strPtr(name),
		Number:   int32Ptr(number),
		Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
		Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
		TypeName: strPtr(typeName),
	}
}

func newNodeLogBatchMessage() *dynamicpb.Message {
	return dynamicpb.NewMessage(nodeLogBatchDesc)
}

type NodeLogServiceClient interface {
	ReportLogs(ctx context.Context, in *dynamicpb.Message, opts ...grpc.CallOption) (*pb.StatusResponse, error)
}

type nodeLogServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewNodeLogServiceClient(cc grpc.ClientConnInterface) NodeLogServiceClient {
	return &nodeLogServiceClient{cc: cc}
}

func (c *nodeLogServiceClient) ReportLogs(ctx context.Context, in *dynamicpb.Message, opts ...grpc.CallOption) (*pb.StatusResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(pb.StatusResponse)
	if err := c.cc.Invoke(ctx, NodeLogService_ReportLogs_FullMethodName, in, out, cOpts...); err != nil {
		return nil, err
	}
	return out, nil
}

type NodeLogServiceServer interface {
	ReportLogs(context.Context, *dynamicpb.Message) (*pb.StatusResponse, error)
}

func RegisterNodeLogServiceServer(s grpc.ServiceRegistrar, srv NodeLogServiceServer) {
	s.RegisterService(&NodeLogService_ServiceDesc, srv)
}

func _NodeLogService_ReportLogs_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := newNodeLogBatchMessage()
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(NodeLogServiceServer).ReportLogs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: NodeLogService_ReportLogs_FullMethodName,
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(NodeLogServiceServer).ReportLogs(ctx, req.(*dynamicpb.Message))
	}
	return interceptor(ctx, in, info, handler)
}

var NodeLogService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v2board.NodeLogService",
	HandlerType: (*NodeLogServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ReportLogs",
			Handler:    _NodeLogService_ReportLogs_Handler,
		},
	},
	Metadata: "api/grpc/v2board.proto",
}

type NodeLogGRPCServer struct {
	nodeService    *service.NodeService
	nodeLogService *service.NodeLogService
}

func NewNodeLogGRPCServer() *NodeLogGRPCServer {
	return &NodeLogGRPCServer{
		nodeService:    service.NewNodeService(),
		nodeLogService: service.NewNodeLogService(),
	}
}

func normalizeLoggedAt(raw int64) *time.Time {
	if raw <= 0 {
		return nil
	}

	var t time.Time
	if raw > 1_000_000_000_000 {
		t = time.UnixMilli(raw)
	} else {
		t = time.Unix(raw, 0)
	}
	return &t
}

func (s *NodeLogGRPCServer) ReportLogs(ctx context.Context, req *dynamicpb.Message) (*pb.StatusResponse, error) {
	nodeID := uint(req.Get(nodeLogBatchNodeIDField).Uint())
	if nodeID == 0 {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	logValues := req.Get(nodeLogBatchLogsField).List()
	inputs := make([]service.NodeLogInput, 0, logValues.Len())
	for i := 0; i < logValues.Len(); i++ {
		entry := logValues.Get(i).Message()
		message := strings.TrimSpace(entry.Get(nodeLogEntryMessageField).String())
		if message == "" {
			continue
		}

		inputs = append(inputs, service.NodeLogInput{
			Level:      entry.Get(nodeLogEntryLevelField).String(),
			Source:     entry.Get(nodeLogEntrySourceField).String(),
			Message:    message,
			TraceID:    entry.Get(nodeLogEntryTraceIDField).String(),
			FieldsJSON: entry.Get(nodeLogEntryFieldsJSONField).String(),
			LoggedAt:   normalizeLoggedAt(entry.Get(nodeLogEntryTimestampField).Int()),
		})
	}

	var runtimeHealthy *bool
	runtimeError := ""
	for _, input := range inputs {
		if !strings.EqualFold(strings.TrimSpace(input.Source), "wireguard") || strings.TrimSpace(input.FieldsJSON) == "" {
			continue
		}
		var fields struct {
			RuntimeHealthy *bool  `json:"runtime_healthy"`
			RuntimeError   string `json:"runtime_error"`
		}
		if err := json.Unmarshal([]byte(input.FieldsJSON), &fields); err != nil || fields.RuntimeHealthy == nil {
			continue
		}
		healthy := *fields.RuntimeHealthy
		runtimeHealthy = &healthy
		runtimeError = fields.RuntimeError
	}

	if err := s.nodeLogService.RecordLogs(nodeID, inputs); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return nil, status.Error(codes.NotFound, "node not found")
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if runtimeHealthy != nil {
		if err := s.nodeService.UpdateRuntimeHealth(nodeID, *runtimeHealthy, runtimeError); err != nil {
			return nil, status.Error(codes.Internal, "failed to update runtime health")
		}
	}

	_ = s.nodeService.UpdateLastCheckAt(nodeID)

	return &pb.StatusResponse{
		Success: true,
		Message: "logs recorded",
		Code:    200,
	}, nil
}
