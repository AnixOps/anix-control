package pluginhostv1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestControlHostDescriptorContainsOnlyLocalDispatchRPCs(t *testing.T) {
	service := File_api_pluginhost_v1_control_host_proto.Services().ByName("ControlPackageHost")
	require.NotNil(t, service)
	require.Equal(t, 4, service.Methods().Len())
	require.NotNil(t, service.Methods().ByName("Dispatch"))
	require.NotNil(t, service.Methods().ByName("Migrate"))
	require.NotNil(t, service.Methods().ByName("Health"))
	require.NotNil(t, service.Methods().ByName("Drain"))
	require.Nil(t, service.Methods().ByName("Listen"))
}
