package pluginhostv1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestControlHostDescriptorContainsOnlyLocalRPCs(t *testing.T) {
	service := File_api_pluginhost_v1_control_host_proto.Services().ByName("ControlPackageHost")
	require.NotNil(t, service)
	require.Equal(t, 5, service.Methods().Len())
	require.NotNil(t, service.Methods().ByName("Dispatch"))
	require.NotNil(t, service.Methods().ByName("OpenWebSocket"))
	require.NotNil(t, service.Methods().ByName("Migrate"))
	require.NotNil(t, service.Methods().ByName("Health"))
	require.NotNil(t, service.Methods().ByName("Drain"))
	require.Nil(t, service.Methods().ByName("Listen"))
}

func TestControlHostDescriptorContainsWebSocketRelay(t *testing.T) {
	service := File_api_pluginhost_v1_control_host_proto.Services().ByName("ControlPackageHost")
	require.NotNil(t, service)

	method := service.Methods().ByName("OpenWebSocket")
	require.NotNil(t, method)
	require.True(t, method.IsStreamingClient())
	require.True(t, method.IsStreamingServer())

	messages := File_api_pluginhost_v1_control_host_proto.Messages()
	require.NotNil(t, messages.ByName("WebSocketFrame"))
	require.NotNil(t, messages.ByName("WebSocketOpen"))
	require.NotNil(t, messages.ByName("WebSocketClose"))
	require.NotNil(t, messages.ByName("DispatchRequest").Fields().ByName("request_metadata_json"))
	require.NotNil(t, messages.ByName("WebSocketOpen").Fields().ByName("request_metadata_json"))
}
