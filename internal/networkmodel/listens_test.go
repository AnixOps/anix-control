package networkmodel

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListenConflicts(t *testing.T) {
	s := newService(t)
	require.NoError(t, s.DB.AutoMigrate(&model.NodeProtocol{}, &model.EgressRoute{}))
	_, err := s.Replace(context.Background(), twoHops(), nil, Actor{})
	require.NoError(t, err)
	require.NoError(t, s.DB.Create(&model.NodeProtocol{ID: 5, NodeID: 1, Port: 9000, Enable: 1}).Error)
	require.NoError(t, s.DB.Create(&model.NodeProtocol{ID: 6, NodeID: 1, Port: 9001}).Error)
	require.NoError(t, s.DB.Model(&model.NodeProtocol{ID: 6}).Update("enable", 0).Error)
	require.NoError(t, s.DB.Create(&model.EgressRoute{ID: 4, Name: "e", SourceNodeID: 1, ExitNodeID: 2, RelayPort: 9100, Method: "m", Enabled: true}).Error)

	others := []Forward{
		{ID: "fw-a", Entries: []string{"proxy-1"}, Listen: Endpoint{Address: "::", Port: 7000}},
		{ID: "fw-paused", Entries: []string{"proxy-1"}, Listen: Endpoint{Address: "::", Port: 7001}, Paused: true},
		{ID: "fw-k", Kernel: []KernelHop{{Nodes: []string{"proxy-2"}, Listen: Endpoint{Address: "::", Port: 7002}}}, Entries: []string{"proxy-1"}, Listen: Endpoint{Address: "::", Port: 7003}},
		{ID: "rp-1", Entries: []string{"proxy-2"}, Listen: Endpoint{Address: "::", Port: 7004}, ReverseProxy: &ReverseProxySpec{}},
	}
	conflicts := func(listen Endpoint, node string) []string {
		t.Helper()
		s.Forwards = func(*gorm.DB) ([]Forward, error) {
			return append([]Forward{{ID: "fw-new", Entries: []string{node}, Listen: listen}}, others...), nil
		}
		found, err := s.ListenConflicts(s.DB, "fw-new")
		require.NoError(t, err)
		return found
	}

	require.Equal(t, []string{"listen [::]:7000 on proxy-1 is taken by forward fw-a"}, conflicts(Endpoint{Address: "::", Port: 7000}, "proxy-1"))
	require.Equal(t, []string{"listen [::]:7002 on proxy-2 is taken by kernel forward fw-k"}, conflicts(Endpoint{Address: "::", Port: 7002}, "proxy-2"))
	require.Equal(t, []string{"listen 192.0.2.1:7004 on proxy-2 is taken by reverse proxy rp-1"}, conflicts(Endpoint{Address: "192.0.2.1", Port: 7004}, "proxy-2"))
	require.Equal(t, []string{"listen [::]:8388 on proxy-1 is taken by ingress in-ss"}, conflicts(Endpoint{Address: "::", Port: 8388}, "proxy-1"))
	require.Equal(t, []string{"listen 203.0.113.20:443 on proxy-2 is taken by ingress in-hop"}, conflicts(Endpoint{Address: "::ffff:203.0.113.20", Port: 443}, "proxy-2"))
	require.Equal(t, []string{"listen 0.0.0.0:9000 on proxy-1 is taken by protocol 5"}, conflicts(Endpoint{Address: "0.0.0.0", Port: 9000}, "proxy-1"))
	require.Equal(t, []string{"listen [::]:9100 on proxy-2 is taken by egress relay 4"}, conflicts(Endpoint{Address: "::", Port: 9100}, "proxy-2"))

	// Another node, another specific address, a paused owner, a disabled
	// protocol: no conflict.
	require.Empty(t, conflicts(Endpoint{Address: "::", Port: 7000}, "proxy-2"))
	require.Empty(t, conflicts(Endpoint{Address: "203.0.113.21", Port: 443}, "proxy-2"))
	require.Empty(t, conflicts(Endpoint{Address: "::", Port: 7001}, "proxy-1"))
	require.Empty(t, conflicts(Endpoint{Address: "::", Port: 9001}, "proxy-1"))
}
