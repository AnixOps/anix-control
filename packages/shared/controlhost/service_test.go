package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

type bridgeStub struct {
	capability []byte
	operation  string
	payload    []byte
	response   packagebridgesdk.Response
	webSocket  *bridgeWebSocketStub
}

func (s *bridgeStub) Invoke(_ context.Context, capability []byte, operation string, payload []byte) (packagebridgesdk.Response, error) {
	s.capability = append([]byte(nil), capability...)
	s.operation = operation
	s.payload = append([]byte(nil), payload...)
	return s.response, nil
}

func (s *bridgeStub) OpenWebSocket(_ context.Context, capability []byte, operation string) (packagebridgesdk.WebSocketStream, error) {
	if s.webSocket == nil {
		return nil, io.EOF
	}
	s.webSocket.capability = append([]byte(nil), capability...)
	s.webSocket.operation = operation
	return s.webSocket, nil
}

func TestGenericServiceDispatchesOnlyThroughTheBridgeCapability(t *testing.T) {
	bridge := &bridgeStub{response: packagebridgesdk.Response{
		StatusCode: 200, Body: []byte(`{"items":["article-1"]}`),
	}}
	service := newGenericService(bridge, "lease-1")
	capability := make([]byte, 32)
	response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "knowledge.article.list", RequestBody: []byte(`{"page":1}`),
		BridgeCapability: capability, DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})

	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, "knowledge.article.list", bridge.operation)
	require.Equal(t, capability, bridge.capability)
	require.Equal(t, []byte(`{"page":1}`), bridge.payload)

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{RouteID: "knowledge.article.list"})
	require.Error(t, err)
}

func TestGenericServiceExplicitlyImplementsWebSocketPackage(t *testing.T) {
	service := newGenericService(&bridgeStub{}, "lease-1")
	_, supported := any(service).(pluginhostsdk.WebSocketPackage)
	require.True(t, supported)
}

func TestGenericServiceRelaysWebSocketOnlyThroughTheBridgeCapability(t *testing.T) {
	bridgeStream := newBridgeWebSocketStub()
	bridge := &bridgeStub{webSocket: bridgeStream}
	service := newGenericService(bridge, "lease-1")
	hostStream := newHostWebSocketStub()
	capability := make([]byte, 32)
	done := make(chan error, 1)
	go func() {
		done <- service.OpenWebSocket(context.Background(), pluginhostsdk.WebSocketOpen{
			RouteID: "telemetry.monitor.ws", BridgeCapability: capability,
		}, hostStream)
	}()

	hostStream.received <- pluginhostsdk.WebSocketFrame{Data: []byte("ping")}
	select {
	case frame := <-bridgeStream.sent:
		require.Equal(t, []byte("ping"), frame.Data)
		require.Nil(t, frame.Close)
	case <-time.After(time.Second):
		t.Fatal("package WebSocket frame did not reach the bridge")
	}
	require.Equal(t, capability, bridgeStream.capability)
	require.Equal(t, "telemetry.monitor.ws", bridgeStream.operation)

	bridgeStream.received <- packagebridgesdk.WebSocketFrame{Data: []byte("pong")}
	select {
	case frame := <-hostStream.sent:
		require.Equal(t, []byte("pong"), frame.Data)
		require.Nil(t, frame.Close)
	case <-time.After(time.Second):
		t.Fatal("bridge WebSocket frame did not reach the package host")
	}
	hostStream.received <- pluginhostsdk.WebSocketFrame{Close: &pluginhostsdk.WebSocketClose{Code: 1000, Reason: "done"}}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("package WebSocket relay did not finish")
	}
}

type bridgeWebSocketStub struct {
	capability []byte
	operation  string
	received   chan packagebridgesdk.WebSocketFrame
	sent       chan packagebridgesdk.WebSocketFrame
}

func newBridgeWebSocketStub() *bridgeWebSocketStub {
	return &bridgeWebSocketStub{
		received: make(chan packagebridgesdk.WebSocketFrame, 4),
		sent:     make(chan packagebridgesdk.WebSocketFrame, 4),
	}
}

func (s *bridgeWebSocketStub) Recv() (packagebridgesdk.WebSocketFrame, error) {
	frame, ok := <-s.received
	if !ok {
		return packagebridgesdk.WebSocketFrame{}, io.EOF
	}
	return frame, nil
}

func (s *bridgeWebSocketStub) Send(frame packagebridgesdk.WebSocketFrame) error {
	s.sent <- frame
	return nil
}

func (*bridgeWebSocketStub) CloseSend() error { return nil }

type hostWebSocketStub struct {
	received chan pluginhostsdk.WebSocketFrame
	sent     chan pluginhostsdk.WebSocketFrame
}

func newHostWebSocketStub() *hostWebSocketStub {
	return &hostWebSocketStub{
		received: make(chan pluginhostsdk.WebSocketFrame, 4),
		sent:     make(chan pluginhostsdk.WebSocketFrame, 4),
	}
}

func (s *hostWebSocketStub) Recv() (pluginhostsdk.WebSocketFrame, error) {
	frame, ok := <-s.received
	if !ok {
		return pluginhostsdk.WebSocketFrame{}, io.EOF
	}
	return frame, nil
}

func (s *hostWebSocketStub) Send(frame pluginhostsdk.WebSocketFrame) error {
	s.sent <- frame
	return nil
}
