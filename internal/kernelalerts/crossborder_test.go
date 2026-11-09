package kernelalerts

import (
	"encoding/json"
	"testing"
	"time"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

func crossBorderDoc(t *testing.T, db *gorm.DB) {
	t.Helper()
	doc := networkmodel.Document{
		Lines:     []networkmodel.Line{{ID: "hk-l", Node: "proxy-29"}, {ID: "jp-l", Node: "proxy-50"}, {ID: "de-l", Node: "proxy-57"}},
		Ingresses: []networkmodel.Ingress{{ID: "jp-hop", Line: "jp-l"}, {ID: "de-hop", Line: "de-l"}},
		Egresses:  []networkmodel.Egress{{ID: "hk-out", Line: "hk-l"}},
		Transports: []networkmodel.Transport{
			{ID: "t-jp", Kind: networkmodel.KindAnx, FromEgress: "hk-out", ToIngress: "jp-hop"},
			{ID: "t-de", Kind: networkmodel.KindAnx, FromEgress: "hk-out", ToIngress: "de-hop"},
			{ID: "t-line", Kind: "anx_line", FromEgress: "hk-out", ToIngress: "jp-hop", Trusted: true},
		},
	}
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.NetworkModel{ID: 1, Revision: 1, DocumentJSON: string(encoded), UpdatedAt: start}).Error)
}

func reportStatus(t *testing.T, db *gorm.DB, ref string, at time.Time, components ...*networkv1.ComponentStatus) {
	t.Helper()
	encoded, err := protojson.Marshal(&networkv1.NetworkStatus{Components: components})
	require.NoError(t, err)
	require.NoError(t, db.Save(&model.NetworkDevice{NodeRef: ref, StatusJSON: string(encoded), StatusAt: &at, UpdatedAt: at}).Error)
}

func component(id string, state networkv1.ComponentState, changed time.Time, detail string) *networkv1.ComponentStatus {
	return &networkv1.ComponentStatus{Id: id, State: state, ChangedAtUnixMs: changed.UnixMilli(), Detail: detail}
}

// A cross-border transport down for ten minutes on its device is critical
// and names the switch; once switched to a standby it is a warning; half of
// the transports down is the wide outage; a trusted line never alerts.
func TestCrossBorderLinkDown(t *testing.T) {
	db := openSQLite(t)
	require.NoError(t, db.AutoMigrate(&model.NetworkModel{}, &model.NetworkDevice{}, &model.L2Standby{}))
	crossBorderDoc(t, db)
	down := networkv1.ComponentState_COMPONENT_STATE_DOWN
	reportStatus(t, db, "proxy-29", start.Add(-time.Minute),
		component("t-jp", down, start.Add(-11*time.Minute), "rst_after_handshake"),
		component("t-de", networkv1.ComponentState_COMPONENT_STATE_APPLIED, start.Add(-time.Hour), ""),
		component("t-line", down, start.Add(-time.Hour), ""))

	found := kinds(scan(t, db, defaultSettings(), start))
	finding, ok := found[KindCrossBorderLinkDown+"/t-jp"]
	require.True(t, ok, found)
	require.Equal(t, model.KernelAlertCritical, finding.Severity)
	require.Equal(t, SubjectTransport, finding.SubjectKind)
	require.Contains(t, finding.Message, "standby/switch")
	require.Equal(t, "rst_after_handshake", finding.Detail["reasons"])
	require.NotContains(t, found, KindCrossBorderLinkDown+"/t-line")
	require.NotContains(t, found, KindCrossBorderLinkDown+"/t-de")
	// One of two is half: the wide outage too.
	require.Contains(t, found, KindCrossBorderWideOutage+"/all")

	// Switched to a standby: a warning that says to switch back.
	require.NoError(t, db.Create(&model.L2Standby{TransportID: "t-jp", Tier: model.L2StandbyTierReality, SourceNodeID: 29, ExitNodeID: 50,
		RelayPort: 31000, UUID: "u", Active: true, CreatedAt: start, UpdatedAt: start}).Error)
	finding = kinds(scan(t, db, defaultSettings(), start))[KindCrossBorderLinkDown+"/t-jp"]
	require.Equal(t, model.KernelAlertWarning, finding.Severity)
	require.Equal(t, model.L2StandbyTierReality, finding.Detail["active_tier"])

	// Too recent, or a stale status: nothing.
	reportStatus(t, db, "proxy-29", start.Add(-time.Minute), component("t-jp", down, start.Add(-5*time.Minute), ""))
	require.NotContains(t, kinds(scan(t, db, defaultSettings(), start)), KindCrossBorderLinkDown+"/t-jp")
	reportStatus(t, db, "proxy-29", start.Add(-time.Hour), component("t-jp", down, start.Add(-2*time.Hour), ""))
	require.NotContains(t, kinds(scan(t, db, defaultSettings(), start)), KindCrossBorderLinkDown+"/t-jp")
}

// Without the network model's tables the check finds nothing.
func TestCrossBorderWithoutNetworkModel(t *testing.T) {
	db := openSQLite(t)
	require.NotContains(t, kinds(scan(t, db, defaultSettings(), start)), KindCrossBorderWideOutage+"/all")
}
