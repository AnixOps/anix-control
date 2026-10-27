package parser

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestAnixOpsFormatterWritesIWANLinksAndVia(t *testing.T) {
	nodes := []*model.ParsedNode{
		{ID: "iwan-line-3", Name: "HK iWAN", Type: "iwan", Server: "2001:db8::1", Port: 4567, Username: "shared user", Password: "p@ss", MTU: 1400, Settings: map[string]any{"allow_encrypt": false}},
		{ID: "t", Name: "US | via HK", Type: "trojan", Server: "198.51.100.20", Port: 443, Password: "pw", TLSMode: 1, Via: "HK iWAN"},
		{ID: "v", Name: "JP", Type: "vmess", Server: "198.51.100.40", Port: 443, UUID: "00000000-0000-0000-0000-000000000001", Via: "HK iWAN"},
	}
	content, err := (&AnixOpsFormatter{}).Format(nodes, &model.TemplateRenderContext{})
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	require.NoError(t, err)
	lines := strings.Split(string(decoded), "\n")
	require.Len(t, lines, 3)
	require.Equal(t, "iwan://shared%20user:p%40ss@[2001:db8::1]:4567?encrypt=0&mtu=1400#HK%20iWAN", lines[0])
	require.Contains(t, lines[1], "&via=HK%20iWAN#")
	require.True(t, strings.HasPrefix(lines[2], "vmess://"))
	require.True(t, strings.HasSuffix(lines[2], "?via=HK%20iWAN"))
}
