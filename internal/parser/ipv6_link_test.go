package parser

import (
	"encoding/base64"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

// A node entry on a native IPv6 address (internal/nodeentry) puts the
// address in brackets in share links, which would not parse without them.
func TestShareLinksBracketIPv6Servers(t *testing.T) {
	ctx := &model.TemplateRenderContext{UUID: "test-uuid"}
	nodes := []*model.ParsedNode{
		{Name: "SS v6", Type: "shadowsocks", Server: "2001:db8::10", Port: 30000, Cipher: "aes-256-gcm", Password: "pw"},
		{Name: "VLESS v6", Type: "vless", Server: "2001:db8::10", Port: 443, UUID: "u"},
		{Name: "SS v4", Type: "shadowsocks", Server: "198.51.100.7", Port: 21000, Cipher: "aes-256-gcm", Password: "pw"},
	}
	output, err := (&V2RayFormatter{}).Format(nodes, ctx)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	require.NoError(t, err)
	require.Contains(t, string(decoded), "@[2001:db8::10]:30000#")
	require.Contains(t, string(decoded), "@[2001:db8::10]:443?")
	require.Contains(t, string(decoded), "@198.51.100.7:21000#")
}
