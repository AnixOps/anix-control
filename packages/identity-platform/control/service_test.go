package main

import (
	"context"
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
}

func (s *bridgeStub) Invoke(_ context.Context, capability []byte, operation string, payload []byte) (packagebridgesdk.Response, error) {
	s.capability = append([]byte(nil), capability...)
	s.operation = operation
	s.payload = append([]byte(nil), payload...)
	return s.response, nil
}

func TestIdentityServiceDispatchesLoginOnlyThroughBridgeCapability(t *testing.T) {
	bridge := &bridgeStub{response: packagebridgesdk.Response{
		StatusCode: 200,
		Body:       []byte(`{"code":0,"msg":"操作成功","ts":1,"data":{"token":"issued"}}`),
		Headers:    []packagebridgesdk.Header{{Name: "Retry-After", Value: "1"}},
	}}
	service := newIdentityService(bridge, "lease-1")
	capability := make([]byte, 32)
	response, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "identity.auth.login", RequestBody: []byte(`{"email":"u@example.test","password":"secret"}`),
		BridgeCapability: capability, DeadlineUnixMillis: time.Now().Add(time.Second).UnixMilli(),
	})

	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, "identity.auth.login", bridge.operation)
	require.Equal(t, capability, bridge.capability)
	require.Equal(t, []byte(`{"email":"u@example.test","password":"secret"}`), bridge.payload)
	require.Equal(t, []pluginhostsdk.Header{{Name: "Retry-After", Value: "1"}}, response.Headers)
}

func TestIdentityServiceRejectsUnknownRoutesAndMissingCapability(t *testing.T) {
	service := newIdentityService(&bridgeStub{}, "lease-1")
	_, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{RouteID: "identity.admin.users.list"})
	require.Error(t, err)

	_, err = service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{RouteID: "identity.auth.login"})
	require.Error(t, err)
}

func TestIdentityServiceDispatchesDeclaredAdministrativeRouteThroughBridge(t *testing.T) {
	bridge := &bridgeStub{response: packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"code":0,"msg":"操作成功","ts":1,"data":[]}`)}}
	service := newIdentityService(bridge, "lease-1")
	capability := make([]byte, 32)
	_, err := service.Dispatch(context.Background(), pluginhostsdk.DispatchRequest{
		RouteID: "identity.admin.users.get", BridgeCapability: capability,
	})

	require.NoError(t, err)
	require.Equal(t, "identity.admin.users.get", bridge.operation)
}

func TestNilIdentityServiceHealthIsUnhealthyWithoutPanicking(t *testing.T) {
	var service *identityService

	response, err := service.Health(context.Background())

	require.NoError(t, err)
	require.False(t, response.Healthy)
	require.Empty(t, response.LeaseID)
}

func TestIdentityServiceRunsMigrationOnlyThroughBridgeCapability(t *testing.T) {
	bridge := &bridgeStub{response: packagebridgesdk.Response{StatusCode: 200, Body: []byte(`{"checkpoint":"identity-platform/001","validation_digest":"migration-digest","complete":true}`)}}
	service := newIdentityService(bridge, "lease-1")
	capability := make([]byte, 32)

	response, err := service.Migrate(context.Background(), pluginhostsdk.MigrationRequest{
		MigrationID: "001_identity_platform", BridgeCapability: capability,
	})

	require.NoError(t, err)
	require.True(t, response.Complete)
	require.Equal(t, "identity-platform/001", response.Checkpoint)
	require.Equal(t, "migration-digest", response.ValidationDigest)
	require.Equal(t, "migration.identity-platform.001_identity_platform", bridge.operation)
	require.Equal(t, capability, bridge.capability)
}
