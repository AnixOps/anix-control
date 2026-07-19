package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"

	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
)

type bridgeClient interface {
	Invoke(context.Context, []byte, string, []byte) (packagebridgesdk.Response, error)
}

type webSocketBridgeClient interface {
	OpenWebSocket(context.Context, []byte, string) (packagebridgesdk.WebSocketStream, error)
}

type genericService struct {
	bridge   bridgeClient
	leaseID  string
	draining atomic.Bool
}

func newGenericService(bridge bridgeClient, leaseID string) *genericService {
	return &genericService{bridge: bridge, leaseID: leaseID}
}

func (s *genericService) Dispatch(ctx context.Context, request pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	if s == nil || s.bridge == nil || s.draining.Load() {
		return pluginhostsdk.DispatchResponse{}, errors.New("package is unavailable")
	}
	if strings.TrimSpace(request.RouteID) == "" || len(request.BridgeCapability) != 32 {
		return pluginhostsdk.DispatchResponse{}, errors.New("package bridge capability is required")
	}
	response, err := s.bridge.Invoke(ctx, request.BridgeCapability, request.RouteID, request.RequestBody)
	if err != nil {
		return pluginhostsdk.DispatchResponse{}, err
	}
	headers := make([]pluginhostsdk.Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = pluginhostsdk.Header{Name: header.Name, Value: header.Value}
	}
	return pluginhostsdk.DispatchResponse{
		StatusCode: response.StatusCode, ResponseBody: response.Body, Headers: headers,
	}, nil
}

func (s *genericService) OpenWebSocket(ctx context.Context, open pluginhostsdk.WebSocketOpen, stream pluginhostsdk.WebSocketStream) error {
	if s == nil || s.bridge == nil || s.draining.Load() || stream == nil {
		return errors.New("package is unavailable")
	}
	if strings.TrimSpace(open.RouteID) == "" || len(open.BridgeCapability) != 32 {
		return errors.New("package WebSocket bridge capability is required")
	}
	bridge, ok := s.bridge.(webSocketBridgeClient)
	if !ok {
		return errors.New("package WebSocket bridge is unavailable")
	}
	relayContext, cancel := context.WithCancel(ctx)
	defer cancel()
	bridgeStream, err := bridge.OpenWebSocket(relayContext, open.BridgeCapability, open.RouteID)
	if err != nil {
		return err
	}
	return relayWebSocketBridge(relayContext, stream, bridgeStream)
}

func relayWebSocketBridge(ctx context.Context, host pluginhostsdk.WebSocketStream, bridge packagebridgesdk.WebSocketStream) error {
	result := make(chan error, 2)
	go func() {
		for {
			frame, err := host.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					_ = bridge.CloseSend()
					result <- nil
					return
				}
				result <- err
				return
			}
			if frame.Close != nil && len(frame.Data) != 0 {
				result <- errors.New("package WebSocket frame cannot contain data and close")
				return
			}
			bridgeFrame := packagebridgesdk.WebSocketFrame{Data: append([]byte(nil), frame.Data...)}
			if frame.Close != nil {
				bridgeFrame.Close = &packagebridgesdk.WebSocketClose{Code: frame.Close.Code, Reason: frame.Close.Reason}
			}
			if err := bridge.Send(bridgeFrame); err != nil {
				result <- err
				return
			}
			if frame.Close != nil {
				_ = bridge.CloseSend()
				result <- nil
				return
			}
		}
	}()
	go func() {
		for {
			frame, err := bridge.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					result <- nil
					return
				}
				result <- err
				return
			}
			if frame.Close != nil && len(frame.Data) != 0 {
				result <- errors.New("bridge WebSocket frame cannot contain data and close")
				return
			}
			hostFrame := pluginhostsdk.WebSocketFrame{Data: append([]byte(nil), frame.Data...)}
			if frame.Close != nil {
				hostFrame.Close = &pluginhostsdk.WebSocketClose{Code: frame.Close.Code, Reason: frame.Close.Reason}
			}
			if err := host.Send(hostFrame); err != nil {
				result <- err
				return
			}
			if frame.Close != nil {
				result <- nil
				return
			}
		}
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *genericService) Migrate(ctx context.Context, request pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	if s == nil || s.bridge == nil || s.draining.Load() {
		return pluginhostsdk.MigrationResponse{}, errors.New("package is unavailable")
	}
	if strings.TrimSpace(request.MigrationID) == "" || len(request.BridgeCapability) != 32 {
		return pluginhostsdk.MigrationResponse{}, errors.New("package migration bridge capability is required")
	}
	operation := "migration." + packageID + "." + request.MigrationID
	response, err := s.bridge.Invoke(ctx, request.BridgeCapability, operation, nil)
	if err != nil {
		return pluginhostsdk.MigrationResponse{}, err
	}
	var result struct {
		Checkpoint       string `json:"checkpoint"`
		ValidationDigest string `json:"validation_digest"`
		Complete         bool   `json:"complete"`
	}
	if response.StatusCode != 200 || json.Unmarshal(response.Body, &result) != nil || result.Checkpoint == "" || result.ValidationDigest == "" || !result.Complete {
		return pluginhostsdk.MigrationResponse{}, errors.New("package migration bridge response is invalid")
	}
	return pluginhostsdk.MigrationResponse{
		Checkpoint: result.Checkpoint, ValidationDigest: result.ValidationDigest, Complete: result.Complete,
	}, nil
}

func (s *genericService) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	if s == nil {
		return pluginhostsdk.HealthResponse{Healthy: false}, nil
	}
	if s.bridge == nil || s.draining.Load() {
		return pluginhostsdk.HealthResponse{Healthy: false, LeaseID: s.leaseID}, nil
	}
	return pluginhostsdk.HealthResponse{Healthy: true, LeaseID: s.leaseID, DetailsJSON: `{"bridge":"required"}`}, nil
}

func (s *genericService) Drain(context.Context) (pluginhostsdk.DrainResponse, error) {
	if s == nil {
		return pluginhostsdk.DrainResponse{}, errors.New("package is unavailable")
	}
	s.draining.Store(true)
	return pluginhostsdk.DrainResponse{Drained: true}, nil
}
