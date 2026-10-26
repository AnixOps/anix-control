package parser

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestIWANFormatter(t *testing.T) {
	nodes := []*model.ParsedNode{
		{Type: "vless", Name: "skip", Server: "v.example.com", Port: 443, UUID: "u"},
		{Type: "iwan", Name: "HK\niWAN", Server: "hk.example.com", Port: 4500, Username: "7",
			Password: "c9a1e7b05b0e4a4b", MTU: 1380, PeerIP: "10.66.0.9", Settings: map[string]any{}},
		{Type: "iwan", Name: "SG", Server: "[2001:db8::1]", Port: 4501, Username: "7",
			Password: "c9a1e7b05b0e4a4b", Settings: map[string]any{"allow_encrypt": false}},
		{Type: "iwan", Name: "no password", Server: "x.example.com", Port: 1, Username: "7"},
	}
	output, err := (&IWANFormatter{}).Format(nodes, nil)
	require.NoError(t, err)
	want := "# Panabit iWAN (iwand). iWAN does not encrypt traffic; use it only on links you trust.\n" +
		"# HK iWAN\n# fixed address 10.66.0.9\n[iwan0]\nserver=hk.example.com\nport=4500\n" +
		"username=7\npassword=c9a1e7b05b0e4a4b\nmtu=1380\nencrypt=1\n\n" +
		"# SG\n[iwan1]\nserver=2001:db8::1\nport=4501\nusername=7\npassword=c9a1e7b05b0e4a4b\n" +
		"mtu=1400\nencrypt=0\n"
	require.Equal(t, want, string(output))

	empty, err := (&IWANFormatter{}).Format(nodes[:1], nil)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.Equal(t, "conf", (&IWANFormatter{}).FileExtension())
}

func TestFormattersSkipIWAN(t *testing.T) {
	node := &model.ParsedNode{Type: "iwan", Name: "HK", Server: "hk.example.com", Port: 4500,
		Username: "7", Password: "c9a1e7b05b0e4a4b", Settings: map[string]any{}}
	output, err := (&V2RayFormatter{}).Format([]*model.ParsedNode{node}, nil)
	require.NoError(t, err)
	require.Empty(t, output)
	require.Nil(t, (&ClashFormatter{}).buildProxy(node, nil))
}
