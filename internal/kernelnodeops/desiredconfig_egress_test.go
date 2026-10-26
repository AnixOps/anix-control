package kernelnodeops

import (
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A proxy node's egress routes are the member "egress" of the source and
// the exit node; a node without any has no such member, so its hash does
// not change when another node's route does.
func TestProxyDesiredConfigCarriesEgressRoutes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		source := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
		exit := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 2}
		before, err := BuildDesiredConfig(db, source)
		require.NoError(t, err)
		require.NotContains(t, before.Document, "egress")

		require.NoError(t, db.Create(&model.EgressRoute{
			ID: 7, Name: "ai", SourceNodeID: 1, ExitNodeID: 2, ExitHost: "203.0.113.2", RelayPort: 24680,
			Method: model.EgressRouteMethod, RelayKey: "a2V5", Domains: `["openai.com"]`, Enabled: true,
		}).Error)
		withRoute, err := BuildDesiredConfig(db, source)
		require.NoError(t, err)
		require.NotEqual(t, before.Hash, withRoute.Hash)
		var document struct {
			Egress struct {
				Routes []struct {
					ID      uint64   `json:"id"`
					Domains []string `json:"domains"`
					Server  struct {
						Protocol string `json:"protocol"`
						Host     string `json:"host"`
						Port     int    `json:"port"`
					} `json:"server"`
				} `json:"routes"`
				Relays []json.RawMessage `json:"relays"`
			} `json:"egress"`
		}
		require.NoError(t, json.Unmarshal(withRoute.JSON, &document))
		require.Len(t, document.Egress.Routes, 1)
		require.Equal(t, uint64(7), document.Egress.Routes[0].ID)
		require.Equal(t, []string{"openai.com"}, document.Egress.Routes[0].Domains)
		require.Equal(t, "203.0.113.2", document.Egress.Routes[0].Server.Host)
		require.Equal(t, "shadowsocks", document.Egress.Routes[0].Server.Protocol)
		require.Empty(t, document.Egress.Relays)

		exitConfig, err := BuildDesiredConfig(db, exit)
		require.NoError(t, err)
		require.Contains(t, string(exitConfig.JSON), `"relays":[{"id":7,"method":"2022-blake3-aes-128-gcm","password":"a2V5","port":24680,"protocol":"shadowsocks"}]`)
	})
}
