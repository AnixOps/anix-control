package localgrpc

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type testService struct {
	testgrpc.UnimplementedTestServiceServer
}

func (testService) UnaryCall(ctx context.Context, request *testgrpc.SimpleRequest) (*testgrpc.SimpleResponse, error) {
	if string(request.GetPayload().GetBody()) == "panic" {
		panic("boom")
	}
	incoming, _ := metadata.FromIncomingContext(ctx)
	body := append(request.GetPayload().GetBody(), []byte(incoming.Get("x-test")[0])...)
	return &testgrpc.SimpleResponse{Payload: &testgrpc.Payload{Body: body}}, nil
}

func (testService) StreamingInputCall(stream testgrpc.TestService_StreamingInputCallServer) error {
	total := 0
	for {
		request, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&testgrpc.StreamingInputCallResponse{AggregatedPayloadSize: int32(total)})
		}
		if err != nil {
			return err
		}
		total += len(request.GetPayload().GetBody())
	}
}

func (testService) StreamingOutputCall(request *testgrpc.StreamingOutputCallRequest, stream testgrpc.TestService_StreamingOutputCallServer) error {
	for _, parameters := range request.GetResponseParameters() {
		if err := stream.Send(&testgrpc.StreamingOutputCallResponse{Payload: &testgrpc.Payload{Body: make([]byte, parameters.GetSize())}}); err != nil {
			return err
		}
	}
	return status.Error(codes.Aborted, "done with an error")
}

func newClient() testgrpc.TestServiceClient {
	channel := &Channel{}
	testgrpc.RegisterTestServiceServer(channel, testService{})
	return testgrpc.NewTestServiceClient(channel)
}

func TestUnaryCallCopiesMessagesAndMetadata(t *testing.T) {
	client := newClient()
	body := []byte("hello ")
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-test", "world")
	response, err := client.UnaryCall(ctx, &testgrpc.SimpleRequest{Payload: &testgrpc.Payload{Body: body}})
	require.NoError(t, err)
	require.Equal(t, "hello world", string(response.GetPayload().GetBody()))
	require.Equal(t, "hello ", string(body), "the caller's message is not shared")
}

func TestUnaryPanicIsInternal(t *testing.T) {
	_, err := newClient().UnaryCall(context.Background(), &testgrpc.SimpleRequest{Payload: &testgrpc.Payload{Body: []byte("panic")}})
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestClientStreamReturnsOneReply(t *testing.T) {
	stream, err := newClient().StreamingInputCall(context.Background())
	require.NoError(t, err)
	for _, body := range []string{"ab", "cde"} {
		require.NoError(t, stream.Send(&testgrpc.StreamingInputCallRequest{Payload: &testgrpc.Payload{Body: []byte(body)}}))
	}
	response, err := stream.CloseAndRecv()
	require.NoError(t, err)
	require.EqualValues(t, 5, response.GetAggregatedPayloadSize())
}

func TestServerStreamDeliversEveryMessageThenTheStatus(t *testing.T) {
	stream, err := newClient().StreamingOutputCall(context.Background(), &testgrpc.StreamingOutputCallRequest{
		ResponseParameters: []*testgrpc.ResponseParameters{{Size: 1}, {Size: 2}},
	})
	require.NoError(t, err)
	for _, size := range []int{1, 2} {
		response, err := stream.Recv()
		require.NoError(t, err)
		require.Len(t, response.GetPayload().GetBody(), size)
	}
	_, err = stream.Recv()
	require.Equal(t, codes.Aborted, status.Code(err))
}

func TestUnknownMethodIsUnimplemented(t *testing.T) {
	_, err := newClient().EmptyCall(context.Background(), &testgrpc.Empty{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	err = (&Channel{}).Invoke(context.Background(), "/no.Service/Call", &testgrpc.Empty{}, &testgrpc.Empty{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestCancelledCallerEndsTheStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := newClient().StreamingInputCall(ctx)
	require.NoError(t, err)
	cancel()
	_, err = stream.CloseAndRecv()
	require.Equal(t, codes.Canceled, status.Code(err))
}
