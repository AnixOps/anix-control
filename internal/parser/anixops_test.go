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

func TestAnixOpsFormatterWritesANXLinks(t *testing.T) {
	anx := &model.ParsedNode{ID: "a", Name: "JP ANX", Type: "anx", Server: "198.51.100.7", Port: 8443,
		Password: strings.Repeat("ab", 32), PublicKey: "ANX1+key/text=", ServerName: "www.apple.com", ALPN: "h2,http/1.1"}
	noKey := &model.ParsedNode{ID: "b", Name: "no key", Type: "anx", Server: "198.51.100.8", Port: 8443,
		Password: strings.Repeat("ab", 32), ServerName: "www.apple.com"}
	content, err := (&AnixOpsFormatter{}).Format([]*model.ParsedNode{anx, noKey}, &model.TemplateRenderContext{})
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	require.NoError(t, err)
	require.Equal(t, "anx://"+strings.Repeat("ab", 32)+"@198.51.100.7:8443?alpn=h2%2Chttp%2F1.1&fp=chrome&pk=ANX1%2Bkey%2Ftext%3D&sni=www.apple.com#JP%20ANX", string(decoded))

	// The A2 no-SNI mode leaves sni out of the link.
	noSNI := &model.ParsedNode{ID: "c", Name: "SH ANX", Type: "anx", Server: "203.0.113.9", Port: 8443,
		Password: strings.Repeat("cd", 32), PublicKey: "ANX1key", Settings: map[string]any{"allow_no_sni": true}}
	content, err = (&AnixOpsFormatter{}).Format([]*model.ParsedNode{noSNI}, &model.TemplateRenderContext{})
	require.NoError(t, err)
	decoded, err = base64.StdEncoding.DecodeString(string(content))
	require.NoError(t, err)
	require.Equal(t, "anx://"+strings.Repeat("cd", 32)+"@203.0.113.9:8443?fp=chrome&pk=ANX1key#SH%20ANX", string(decoded))

	// Other formats leave ANX out.
	v2ray, err := (&V2RayFormatter{}).Format([]*model.ParsedNode{anx}, &model.TemplateRenderContext{})
	require.NoError(t, err)
	raw, _ := base64.StdEncoding.DecodeString(string(v2ray))
	require.Empty(t, strings.TrimSpace(string(raw)))
}
