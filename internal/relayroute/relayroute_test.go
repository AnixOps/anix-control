package relayroute

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCleanRelays(t *testing.T) {
	clean, err := CleanRelays([]string{" relay-a.example.cn:50051 ", "", "[2001:db8::1]:50051", "relay-a.example.cn:50051", "203.0.113.7:443"})
	require.NoError(t, err)
	assert.Equal(t, []string{"relay-a.example.cn:50051", "[2001:db8::1]:50051", "203.0.113.7:443"}, clean)

	for _, bad := range []string{"relay.example.cn", "relay.example.cn:0", "relay.example.cn:65536", "http://relay.example.cn:1",
		"relay example.cn:1", "relay.example.cn:1;rm", "-relay.example.cn:1", "$(id):1", "2001:db8::1:50051",
		// The installer refuses these (re-audit G5): only plain ports.
		"relay.example.cn:+443", "relay.example.cn:0443", "relay.example.cn:00001"} {
		_, err := CleanRelays([]string{bad})
		assert.ErrorIs(t, err, ErrInvalid, bad)
	}
	many := make([]string, 0, MaxRelays+1)
	for i := 0; i <= MaxRelays; i++ {
		many = append(many, "relay-"+string(rune('a'+i))+".example.cn:50051")
	}
	_, err = CleanRelays(many)
	assert.ErrorIs(t, err, ErrInvalid)
	empty, err := CleanRelays(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{}, empty)
}

func TestCleanCaches(t *testing.T) {
	clean, err := CleanCaches([]string{"https://relay-a.example.cn:8443/", "https://relay-a.example.cn:8443", "https://[2001:db8::1]:8443/cache"})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://relay-a.example.cn:8443", "https://[2001:db8::1]:8443/cache"}, clean)

	for _, bad := range []string{"http://relay.example.cn", "https://user@relay.example.cn", "https://relay.example.cn/?x=1",
		"https://relay.example.cn/#f", "https://relay.example.cn:0", "https://relay.example.cn:08443", "https://relay.example.cn/a%20b", "https://relay.example.cn/a'b", "relay.example.cn"} {
		_, err := CleanCaches([]string{bad})
		assert.ErrorIs(t, err, ErrInvalid, bad)
	}
}

func TestEncodeDecode(t *testing.T) {
	assert.Equal(t, "", Encode(nil))
	assert.Equal(t, `["a:1","b:2"]`, Encode([]string{"a:1", "b:2"}))
	assert.Equal(t, []string{"a:1", "b:2"}, Decode(`["a:1","b:2"]`))
	assert.Equal(t, []string{}, Decode(""))
	assert.Equal(t, []string{}, Decode("not json"))
}

func TestForNodeAndDesiredMember(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.ForwardNode{}, &model.NodeGroup{}))

	routed := model.NodeGroup{Name: "CN", JoinTokenHash: "a", Relays: `["relay-a.example.cn:50051"]`, ReleaseCaches: `["https://relay-a.example.cn:8443"]`}
	require.NoError(t, db.Create(&routed).Error)
	plain := model.NodeGroup{Name: "HK", JoinTokenHash: "b"}
	require.NoError(t, db.Create(&plain).Error)
	proxy := model.Node{Name: "cn-1", NodeGroupID: &routed.ID}
	require.NoError(t, db.Create(&proxy).Error)
	forward := model.ForwardNode{Name: "cn-f1", NodeGroupID: &routed.ID}
	require.NoError(t, db.Create(&forward).Error)
	other := model.Node{Name: "hk-1", NodeGroupID: &plain.ID}
	require.NoError(t, db.Create(&other).Error)
	alone := model.Node{Name: "solo"}
	require.NoError(t, db.Create(&alone).Error)

	want := Route{Relays: []string{"relay-a.example.cn:50051"}, ReleaseCaches: []string{"https://relay-a.example.cn:8443"}}
	for _, node := range []agentcontrol.AgentNode{{Kind: agentcontrol.NodeKindProxy, ID: uint32(proxy.ID)}, {Kind: agentcontrol.NodeKindForward, ID: uint32(forward.ID)}} {
		route, err := ForNode(db, node)
		require.NoError(t, err)
		assert.Equal(t, want, route, node.String())
		member, err := DesiredMember(db, node)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"relays": want.Relays, "release_caches": want.ReleaseCaches}, member)
	}
	for _, node := range []agentcontrol.AgentNode{
		{Kind: agentcontrol.NodeKindProxy, ID: uint32(other.ID)}, {Kind: agentcontrol.NodeKindProxy, ID: uint32(alone.ID)}, {Kind: agentcontrol.NodeKindProxy, ID: 9999},
	} {
		route, err := ForNode(db, node)
		require.NoError(t, err)
		assert.True(t, route.Empty(), node.String())
		member, err := DesiredMember(db, node)
		require.NoError(t, err)
		assert.Nil(t, member, "no member without a route")
	}
}
