package netline

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/routesubscription"
	"github.com/stretchr/testify/require"
)

func TestWriterStoresLinesAsRoutes(t *testing.T) {
	db := openDB(t)
	require.NoError(t, db.AutoMigrate(append(model.KernelForwardModels(), &model.OperationLog{}, &model.ForwardNode{})...))
	for id, name := range map[uint]string{1: "sh", 2: "tencent", 3: "jp"} {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: name, Host: name + ".example.com", Rate: 1}).Error)
	}
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 7, NodeID: 3, Name: "JP VLESS", Type: model.ProtocolVLESS, Port: 443, Enable: 1, Show: 1}).Error)
	require.NoError(t, db.Create(&model.NodeNetwork{NodeRef: "proxy-2", ScanCloud: true}).Error)
	ctx := context.Background()
	forward := kernelforward.New(db)
	caps := &forwardv1.NodeCapabilities{Engines: []*forwardv1.EngineCapabilities{{
		Engine: forwardv1.Engine_ENGINE_NFTABLES, Available: true, Udp: true,
		Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN},
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
	}}}
	for _, id := range []uint32{1, 2, 3} {
		_, _, err := forward.RecordHello(ctx, agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: id}, caps, "5.0.5")
		require.NoError(t, err)
	}
	w := &Writer{DB: db, Forward: forward, Offers: routesubscription.New(db)}

	draft := Draft{Name: "tencent-jp", Enabled: true, Show: true, Port: 31000, L4: []string{"tcp", "udp"},
		Chain: []Hop{{Devices: []string{"proxy-2"}}, {Devices: []string{"proxy-1"}}, {Devices: []string{"proxy-3"}}}, ProtocolID: 7}
	id, err := w.Create(ctx, "r-1", draft)
	require.NoError(t, err)
	s, err := Import(db)
	require.NoError(t, err)
	var line Line
	for _, l := range s.Lines {
		if l.ID == id {
			line = l
		}
	}
	require.Equal(t, "JP VLESS | tencent-jp", line.Name)
	require.True(t, line.Show)
	require.Equal(t, uint(7), line.Protocol.ID)
	require.Equal(t, []string{"tcp", "udp"}, line.L4)
	require.Empty(t, line.Chain[1].Transport, "the kernel hop links raw")
	require.NotEmpty(t, line.Chain[2].Transport)
	route, err := forward.GetRoute(ctx, id)
	require.NoError(t, err)
	require.Equal(t, forwardv1.Engine_ENGINE_NFTABLES, route.GetHops()[0].GetEngine())
	require.Equal(t, forwardv1.Engine_ENGINE_NETWORK, route.GetHops()[1].GetEngine())

	draft.Show, draft.Name, draft.Revision = false, "renamed", line.Revision
	require.NoError(t, w.Update(ctx, "r-2", id, draft))
	require.ErrorIs(t, w.Update(ctx, "r-3", id, draft), kernelforward.ErrRevisionMismatch)
	status, err := w.Offers.Get(ctx, id)
	require.NoError(t, err)
	require.False(t, status.Published)

	draft.Chain = draft.Chain[:2]
	_, err = w.Create(ctx, "r-4", draft)
	require.ErrorIs(t, err, kernelforward.ErrInvalidRequest, "the protocol is not on the exit")

	require.NoError(t, w.Delete(ctx, "r-5", id))
	_, err = forward.GetRoute(ctx, id)
	require.ErrorIs(t, err, kernelforward.ErrNotFound)
}
