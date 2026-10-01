package kernelnodeops

import (
	"context"
	"fmt"
	"testing"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func validateRequest(kind kernelnodeopsv1.NodeConfigKind, protocolType, document string) *kernelnodeopsv1.ValidateNodeConfigRequest {
	return &kernelnodeopsv1.ValidateNodeConfigRequest{Kind: kind, ProtocolType: protocolType, DocumentJson: []byte(document)}
}

// ValidateNodeConfig runs the validators of the raw configuration and
// protocol routes and answers what they answer; a secret sent as a handle
// or the placeholder counts as present.
func TestValidateNodeConfig(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		h := newHarness(t, db)
		ctx := context.Background()
		raw := kernelnodeopsv1.NodeConfigKind_NODE_CONFIG_KIND_RAW_CONFIG
		protocol := kernelnodeopsv1.NodeConfigKind_NODE_CONFIG_KIND_PROTOCOL

		_, err := h.client(forwardHost, allow(service.CapabilityNodeOpsForward)).ValidateNodeConfig(ctx, validateRequest(raw, "", `{}`))
		require.Equal(t, codes.PermissionDenied, status.Code(err), "the nodeconfig family")
		client := h.client(proxyHost, allow(service.CapabilityNodeOpsNodeConfig))
		_, err = client.ValidateNodeConfig(ctx, validateRequest(raw, "", ``))
		require.Equal(t, codes.InvalidArgument, status.Code(err), "a document is required")
		_, err = client.ValidateNodeConfig(ctx, validateRequest(kernelnodeopsv1.NodeConfigKind_NODE_CONFIG_KIND_UNSPECIFIED, "", `{}`))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		_, err = client.ValidateNodeConfig(ctx, validateRequest(raw, "", `{"a":`))
		require.Equal(t, codes.InvalidArgument, status.Code(err), "not JSON")

		answer, err := client.ValidateNodeConfig(ctx, validateRequest(raw, "", `{"type":"vless","server_port":443}`))
		require.NoError(t, err)
		require.True(t, answer.GetValid())
		require.Empty(t, answer.GetIssues())
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(raw, "", `{"type":"vless"}`))
		require.NoError(t, err)
		require.True(t, answer.GetValid())
		require.Len(t, answer.GetIssues(), 1)
		require.Equal(t, "/server_port", answer.GetIssues()[0].GetPath())
		require.Equal(t, "缺少 server_port 字段", answer.GetIssues()[0].GetMessage())
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(raw, "", `[1]`))
		require.NoError(t, err)
		require.False(t, answer.GetValid())
		require.Equal(t, "配置必须是 JSON 对象", answer.GetMessage())

		wireGuard := `{"type":"wireguard","node_type":"wireguard","server_port":51820,"cidr":"10.9.0.0/24","server_address":"10.9.0.1/24","relay":{"backend":"gost","role":"entry","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"},"server_private_key":%q}`
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(raw, "", fmt.Sprintf(wireGuard, "not-a-key")))
		require.NoError(t, err)
		require.False(t, answer.GetValid())
		require.Equal(t, "WireGuard 配置无效", answer.GetMessage())
		require.Len(t, answer.GetIssues(), 1)
		require.Contains(t, answer.GetIssues()[0].GetMessage(), "server_private_key")
		for _, masked := range []string{service.NodeSecretPlaceholder, liveHandle} {
			answer, err = client.ValidateNodeConfig(ctx, validateRequest(raw, "", fmt.Sprintf(wireGuard, masked)))
			require.NoError(t, err)
			require.True(t, answer.GetValid(), "a secret sent masked counts as present: %s %v", answer.GetMessage(), answer.GetIssues())
		}
		private, public, err := service.GenerateWireGuardKeypair()
		require.NoError(t, err)
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(raw, "", fmt.Sprintf(`{"type":"wireguard","server_port":51820,"cidr":"10.9.0.0/24","server_address":"10.9.0.1/24","relay":{"backend":"gost","role":"entry","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"},"server_private_key":%q,"server_public_key":%q}`, private, public)))
		require.NoError(t, err)
		require.True(t, answer.GetValid(), "%s %v", answer.GetMessage(), answer.GetIssues())

		// A protocol row, as the package writes it; its settings column
		// is a JSON string.
		settings := fmt.Sprintf(`{"cidr":"10.9.0.0/24","server_address":"10.9.0.1/24","relay":{"backend":"gost","role":"entry","server":"exit.example.com","server_port":8443,"entry_tun_address":"172.31.77.2/24","exit_tun_address":"172.31.77.1/24"},"server_private_key":%q,"server_public_key":%q}`, service.NodeSecretPlaceholder, "pub")
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(protocol, "wireguard", fmt.Sprintf(`{"port":51820,"enable":1,"settings":%q}`, settings)))
		require.NoError(t, err)
		require.True(t, answer.GetValid(), "%s %v", answer.GetMessage(), answer.GetIssues())
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(protocol, "", fmt.Sprintf(`{"type":"wireguard","port":51820,"enable":1,"settings":%q}`, `{"cidr":"not-a-cidr"}`)))
		require.NoError(t, err)
		require.False(t, answer.GetValid())
		require.Equal(t, "协议无效", answer.GetMessage())
		require.Contains(t, answer.GetIssues()[0].GetMessage(), "CIDR")
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(protocol, "vless", `{"port":443,"reality_settings":"{\"private_key\":\"********\"}"}`))
		require.NoError(t, err)
		require.True(t, answer.GetValid())
		answer, err = client.ValidateNodeConfig(ctx, validateRequest(protocol, "", `"text"`))
		require.NoError(t, err)
		require.False(t, answer.GetValid())
		require.Empty(t, h.rows(t), "a read records nothing")
	})
}
