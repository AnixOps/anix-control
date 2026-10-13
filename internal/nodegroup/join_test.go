package nodegroup

import (
	"crypto/ecdh"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proxyTemplate is a template of every protocol kind that needs a per-node
// secret, and both port forms.
const proxyTemplate = `[
	{"name": "VLESS Reality", "type": "vless", "port": 443, "tls": "reality",
	 "reality": {"server_name": "www.example.com", "dest": "www.example.com:443"}, "settings": {"flow": "xtls-rprx-vision"}},
	{"name": "SS 2022", "type": "shadowsocks", "port_range": [20000, 20003], "settings": {"method": "2022-blake3-aes-128-gcm"}},
	{"name": "VMess WS", "type": "vmess", "port_range": [20000, 20003], "transport": "ws", "transport_settings": {"path": "/v"}},
	{"name": "WireGuard", "type": "wireguard", "port": 51820,
	 "settings": {"cidr": "10.66.0.0/24", "server_address": "10.66.0.1/24", "relay": {"server": "198.51.100.1", "server_port": 8421}}}
]`

func decodeDocument(t *testing.T, document *string) map[string]any {
	t.Helper()
	require.NotNil(t, document)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(*document), &decoded))
	return decoded
}

func TestJoinCreatesAProxyNodeWithTheTemplate(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		rate := 2.0
		group, token := f.createGroup(t, GroupInput{
			Name: "HK", Kind: "proxy", Rate: &rate, Tags: []string{"hk"}, MonthlyLimitBytes: 1 << 40, ResetDay: 9, Sort: 4,
			ProtocolTemplate:     json.RawMessage(proxyTemplate),
			SubscriptionGroupIDs: []uint{f.subscriptions[0].ID, f.subscriptions[1].ID},
		})
		print := fingerprint("a")
		joined, err := f.join(t, JoinRequest{
			Token: token, MachineFingerprint: print, Hostname: "host-a", RequestedName: "hk-01",
			PublicAddresses: []string{"203.0.113.5", "2001:db8::5"}, Arch: "amd64", OS: "linux", AgentVersion: "v5.0.0",
		})
		require.NoError(t, err)
		assert.True(t, joined.Created)
		assert.Equal(t, agentcontrol.NodeKindProxy, joined.Node.Kind)
		assert.Equal(t, "hk-01", joined.NodeName)
		assert.Equal(t, "HK", joined.GroupName)
		assert.Equal(t, group.ID, joined.GroupID)
		assert.Equal(t, model.NodeApprovalApproved, joined.Approval)
		assert.Equal(t, "spiffe://anixops/test/agent/"+joined.Node.String(), joined.Issued.Identity.String())

		// The node: the group's defaults, the join's name, host and
		// fingerprint.
		var node model.Node
		require.NoError(t, f.db.First(&node, joined.Node.ID).Error)
		assert.Equal(t, "hk-01", node.Name)
		assert.Equal(t, "203.0.113.5", node.Host, "the first public address when no connect host is given")
		assert.Equal(t, 2.0, node.Rate)
		require.NotNil(t, node.Tags)
		assert.JSONEq(t, `["hk"]`, *node.Tags)
		require.NotNil(t, node.MonthlyLimit)
		assert.Equal(t, int64(1<<40), *node.MonthlyLimit)
		assert.Equal(t, 9, node.MonthlyResetDay)
		assert.Equal(t, 4, node.Sort)
		require.NotNil(t, node.NodeGroupID)
		assert.Equal(t, group.ID, *node.NodeGroupID)
		assert.Equal(t, print, node.MachineFingerprint)
		assert.Equal(t, model.NodeApprovalApproved, node.Approval)
		assert.Equal(t, "203.0.113.5", node.LastPublicAddress)
		assert.Equal(t, model.NodeStatusPending, node.Status, "online once its Agent connects")
		assert.NotEmpty(t, node.APIKeyHash, "every proxy node has a credential until they are removed")
		require.NotNil(t, node.ServerOS)
		assert.Equal(t, "linux/amd64", *node.ServerOS)

		// The protocols: the template's, with distinct ports and the node's
		// own secrets, in the group's subscription groups.
		var protocols []model.NodeProtocol
		require.NoError(t, f.db.Preload("SubscriptionGroups").Where("node_id = ?", node.ID).Order("sort").Find(&protocols).Error)
		require.Len(t, protocols, 4)
		ports := map[int]bool{}
		for _, protocol := range protocols {
			assert.False(t, ports[protocol.Port], "port %d twice", protocol.Port)
			ports[protocol.Port] = true
			assert.Equal(t, 1, protocol.Enable)
			ids := []uint{}
			for _, subscription := range protocol.SubscriptionGroups {
				ids = append(ids, subscription.ID)
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			assert.Equal(t, []uint{f.subscriptions[0].ID, f.subscriptions[1].ID}, ids, protocol.Name)
			assert.Empty(t, service.ValidateProtocolSecrets(&protocol), protocol.Name)
		}
		reality := protocols[0]
		assert.Equal(t, model.ProtocolVLESS, reality.Type)
		assert.Equal(t, 443, reality.Port)
		assert.Equal(t, 2, reality.TLS)
		realitySettings := decodeDocument(t, reality.RealitySettings)
		private, err := base64.RawURLEncoding.DecodeString(realitySettings["private_key"].(string))
		require.NoError(t, err)
		key, err := ecdh.X25519().NewPrivateKey(private)
		require.NoError(t, err)
		assert.Equal(t, base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), realitySettings["public_key"], "the public key is the private key's")
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{16}$`), realitySettings["short_id"])
		assert.Equal(t, "www.example.com:443", realitySettings["dest"])
		assert.Equal(t, "www.example.com", decodeDocument(t, reality.TLSSettings)["server_name"])
		assert.Equal(t, "xtls-rprx-vision", decodeDocument(t, reality.Settings)["flow"])

		shadowsocks := protocols[1]
		assert.GreaterOrEqual(t, shadowsocks.Port, 20000)
		assert.LessOrEqual(t, shadowsocks.Port, 20003)
		serverKey, err := base64.StdEncoding.DecodeString(decodeDocument(t, shadowsocks.Settings)["server_key"].(string))
		require.NoError(t, err)
		assert.Len(t, serverKey, 16, "2022-blake3-aes-128-gcm takes a 16-byte key")

		vmess := protocols[2]
		require.NotNil(t, vmess.Transport)
		assert.Equal(t, "ws", *vmess.Transport)
		assert.JSONEq(t, `{"path":"/v"}`, *vmess.TransportSettings)

		wireguard := decodeDocument(t, protocols[3].Settings)
		assert.NotEmpty(t, wireguard["server_private_key"])
		assert.NotEmpty(t, wireguard["server_public_key"])

		// The enrollment and certificate, as Enroll records them.
		var enrollment model.AgentEnrollment
		require.NoError(t, f.db.Where("id = ?", joined.Issued.EnrollmentID).First(&enrollment).Error)
		assert.Equal(t, model.AgentEnrollmentMethodJoinToken, enrollment.Method)
		assert.Equal(t, uint(joined.Node.ID), enrollment.NodeID)
		assert.Nil(t, enrollment.CredentialHash)
		assert.Equal(t, "v5.0.0", enrollment.AgentVersion)
		var certificate model.AgentCertificate
		require.NoError(t, f.db.Where("serial = ?", joined.Issued.Serial).First(&certificate).Error)
		assert.Equal(t, enrollment.ID, certificate.EnrollmentID)
		leaf, err := x509.ParseCertificate(joined.Issued.CertificateDER)
		require.NoError(t, err)
		identity, err := agentcontrol.AgentIdentityFromCertificate(leaf)
		require.NoError(t, err)
		assert.Equal(t, joined.Node, identity.Node)

		// Audited, without the token.
		joins := f.audits(t, agentpki.AuditActionJoin)
		require.Len(t, joins, 1)
		assert.Contains(t, joins[0].Content, `"group":"HK"`)
		assert.Contains(t, joins[0].Content, `"created":true`)
		assert.NotContains(t, joins[0].Content, token)
		assert.Equal(t, "198.51.100.200", joins[0].IP)

		// A second machine gets its own secrets.
		second, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: fingerprint("b"), Hostname: "host-b", PublicAddresses: []string{"203.0.113.6"}, ConnectHost: "b.nodes.example.com"})
		require.NoError(t, err)
		var secondNode model.Node
		require.NoError(t, f.db.First(&secondNode, second.Node.ID).Error)
		assert.Equal(t, "b.nodes.example.com", secondNode.Host, "--host wins over the detected address")
		var secondReality model.NodeProtocol
		require.NoError(t, f.db.Where("node_id = ? AND type = ?", second.Node.ID, "vless").First(&secondReality).Error)
		assert.NotEqual(t, realitySettings["private_key"], decodeDocument(t, secondReality.RealitySettings)["private_key"])
		assert.NotEqual(t, realitySettings["short_id"], decodeDocument(t, secondReality.RealitySettings)["short_id"])

		view, err := f.svc.Get(t.Context(), group.ID)
		require.NoError(t, err)
		assert.Equal(t, NodeCounts{Live: 2}, view.Nodes)
	})
}

func TestJoinCreatesAForwardNode(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		group, token := f.createGroup(t, GroupInput{Name: "edge", Kind: "forward", Tags: []string{"iepl", "ansible-machine"}})
		joined, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: fingerprint("f"), Hostname: "relay-1", PublicAddresses: []string{"203.0.113.9"}})
		require.NoError(t, err)
		assert.Equal(t, agentcontrol.NodeKindForward, joined.Node.Kind)
		assert.Equal(t, "relay-1", joined.NodeName)
		assert.True(t, joined.Created)

		var node model.ForwardNode
		require.NoError(t, f.db.First(&node, joined.Node.ID).Error)
		assert.Equal(t, "relay-1", node.Name)
		assert.Equal(t, "203.0.113.9", node.Host)
		assert.True(t, node.Enabled)
		assert.NotEmpty(t, node.APIToken, "created as the forward inventory creates an Agent node")
		require.NotNil(t, node.NodeGroupID)
		assert.Equal(t, group.ID, *node.NodeGroupID)
		assert.Equal(t, fingerprint("f"), node.MachineFingerprint)
		assert.Equal(t, model.NodeApprovalApproved, node.Approval)
		assert.Equal(t, "203.0.113.9", node.LastPublicAddress)
		assert.JSONEq(t, `["iepl"]`, node.Tags, "never the Ansible machine mark")

		// The forward plan ran in the join's transaction.
		var plan model.KernelForwardPlan
		require.NoError(t, f.db.First(&plan).Error)
		assert.Equal(t, "node_created", plan.Reason)
		var protocols int64
		require.NoError(t, f.db.Model(&model.NodeProtocol{}).Count(&protocols).Error)
		assert.Zero(t, protocols)
		assert.Equal(t, "spiffe://anixops/test/agent/"+joined.Node.String(), joined.Issued.Identity.String())
	})
}

func TestJoinNamesAreUniqueWithinTheGroup(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		_, token := f.createGroup(t, GroupInput{Name: "g", Kind: "proxy"})
		_, other := f.createGroup(t, GroupInput{Name: "h", Kind: "proxy"})
		var names []string
		for i := 0; i < 3; i++ {
			joined, err := f.join(t, JoinRequest{Token: token, Hostname: "hk", PublicAddresses: []string{"203.0.113." + strconv.Itoa(i+1)}})
			require.NoError(t, err)
			names = append(names, joined.NodeName)
		}
		assert.Equal(t, []string{"hk", "hk-2", "hk-3"}, names)
		requested, err := f.join(t, JoinRequest{Token: token, Hostname: "hk", RequestedName: "tokyo", PublicAddresses: []string{"203.0.113.9"}})
		require.NoError(t, err)
		assert.Equal(t, "tokyo", requested.NodeName, "--name wins over the host name")
		elsewhere, err := f.join(t, JoinRequest{Token: other, Hostname: "hk", PublicAddresses: []string{"203.0.113.10"}})
		require.NoError(t, err)
		assert.Equal(t, "hk", elsewhere.NodeName, "names are unique within a group only")
		unnamed, err := f.join(t, JoinRequest{Token: other, PublicAddresses: []string{"203.0.113.11"}})
		require.NoError(t, err)
		assert.Equal(t, "node", unnamed.NodeName)
	})
}

func TestJoinRefusalsAreOneAnswerAndAudited(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		past := f.clock.Now().Add(time.Hour)
		capped, cappedToken := f.createGroup(t, GroupInput{Name: "capped", Kind: "proxy", MaxNodes: 1})
		expiring, expiringToken := f.createGroup(t, GroupInput{Name: "expiring", Kind: "proxy", TokenExpiresAt: &past})
		deleted, deletedToken := f.createGroup(t, GroupInput{Name: "deleted", Kind: "proxy"})
		require.NoError(t, f.svc.Delete(t.Context(), deleted.ID, f.admin))
		_, err := f.join(t, JoinRequest{Token: cappedToken, Hostname: "a", PublicAddresses: []string{"203.0.113.1"}})
		require.NoError(t, err)
		f.clock.Advance(2 * time.Hour)

		unknown, err := newJoinToken()
		require.NoError(t, err)
		for name, token := range map[string]string{
			"malformed": "anixjoin_short", "enrollment credential": "anixagt_" + unknown[9:], "unknown": unknown,
			"at max_nodes": cappedToken, "expired": expiringToken, "deleted": deletedToken,
		} {
			_, err := f.join(t, JoinRequest{Token: token, Hostname: "x", PublicAddresses: []string{"203.0.113.50"}, PeerAddress: "198.51.100.77"})
			assert.ErrorIs(t, err, ErrJoinRejected, name)
			assert.Equal(t, ErrJoinRejected.Error(), err.Error(), "%s: no detail", name)
		}
		var nodes int64
		require.NoError(t, f.db.Model(&model.Node{}).Count(&nodes).Error)
		assert.Equal(t, int64(1), nodes, "a refused join creates nothing")

		reasons := map[string]int{}
		for _, entry := range f.audits(t, AuditActionJoinRejected) {
			assert.Equal(t, "198.51.100.77", entry.IP)
			assert.Equal(t, 2, entry.Status)
			var content map[string]any
			require.NoError(t, json.Unmarshal([]byte(entry.Content), &content))
			reasons[content["reason"].(string)]++
			switch content["reason"] {
			case refusalMaxNodes:
				assert.EqualValues(t, capped.ID, content["group_id"])
			case refusalExpiredToken:
				assert.EqualValues(t, expiring.ID, content["group_id"])
			case refusalDeletedGroup:
				assert.EqualValues(t, deleted.ID, content["group_id"])
			}
		}
		assert.Equal(t, map[string]int{refusalUnknownToken: 3, refusalMaxNodes: 1, refusalExpiredToken: 1, refusalDeletedGroup: 1}, reasons)
	})
}

func TestJoinRefusesMalformedRequests(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	_, token := f.createGroup(t, GroupInput{Name: "g", Kind: "proxy"})
	for name, request := range map[string]JoinRequest{
		"fingerprint":      {Token: token, MachineFingerprint: "abc", PublicAddresses: []string{"203.0.113.1"}},
		"address":          {Token: token, PublicAddresses: []string{"not-an-ip"}},
		"zone":             {Token: token, PublicAddresses: []string{"fe80::1%eth0"}},
		"too many":         {Token: token, PublicAddresses: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3", "1.1.1.4", "1.1.1.5", "1.1.1.6", "1.1.1.7", "1.1.1.8", "1.1.1.9"}},
		"connect host":     {Token: token, ConnectHost: "a b", PublicAddresses: []string{"203.0.113.1"}},
		"requested name":   {Token: token, RequestedName: "a\nb", PublicAddresses: []string{"203.0.113.1"}},
		"long name":        {Token: token, RequestedName: string(make([]byte, 65)), PublicAddresses: []string{"203.0.113.1"}},
		"no csr":           {Token: token, CSRDER: []byte{}, PublicAddresses: []string{"203.0.113.1"}},
		"no host anywhere": {Token: token, PeerAddress: " "},
	} {
		if request.CSRDER != nil && len(request.CSRDER) == 0 {
			_, err := f.svc.Join(t.Context(), request)
			assert.ErrorIs(t, err, ErrInvalidJoin, name)
			continue
		}
		if name == "no host anywhere" {
			request.CSRDER = csr(t)
			_, err := f.svc.Join(t.Context(), request)
			assert.ErrorIs(t, err, ErrInvalidJoin, name)
			continue
		}
		_, err := f.join(t, request)
		assert.ErrorIs(t, err, ErrInvalidJoin, name)
	}
	// A CSR that does not parse is the PKI's refusal.
	_, err := f.join(t, JoinRequest{Token: token, CSRDER: []byte("garbage"), PublicAddresses: []string{"203.0.113.1"}})
	require.Error(t, err)
	var nodes int64
	require.NoError(t, f.db.Model(&model.Node{}).Count(&nodes).Error)
	assert.Zero(t, nodes, "a failed issuance rolls the node back")
}

func TestJoinRejoinRule(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		group, token := f.createGroup(t, GroupInput{Name: "g", Kind: "proxy", MaxNodes: 1, ProtocolTemplate: json.RawMessage(`[{"type":"vmess","port":8080}]`)})
		print := fingerprint("m")
		first, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: print, Hostname: "m", PublicAddresses: []string{"203.0.113.7", "2001:db8::7"}})
		require.NoError(t, err)
		var protocolsBefore int64
		require.NoError(t, f.db.Model(&model.NodeProtocol{}).Count(&protocolsBefore).Error)

		// All four conditions hold: the machine rejoins its node, although
		// the group is at its cap. The old certificate is revoked.
		f.clock.Advance(10 * time.Minute)
		rejoined, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: print, Hostname: "m", PublicAddresses: []string{"2001:db8::99", "203.0.113.7"}})
		require.NoError(t, err)
		assert.False(t, rejoined.Created)
		assert.Equal(t, first.Node, rejoined.Node)
		assert.Equal(t, "m", rejoined.NodeName)
		assert.NotEqual(t, first.Issued.Serial, rejoined.Issued.Serial)
		var old model.AgentCertificate
		require.NoError(t, f.db.Where("serial = ?", first.Issued.Serial).First(&old).Error)
		require.NotNil(t, old.RevokedAt)
		assert.Equal(t, agentpki.RevokeReasonRejoined, old.RevokeReason)
		var fresh model.AgentCertificate
		require.NoError(t, f.db.Where("serial = ?", rejoined.Issued.Serial).First(&fresh).Error)
		assert.Nil(t, fresh.RevokedAt)
		var protocolsAfter int64
		require.NoError(t, f.db.Model(&model.NodeProtocol{}).Count(&protocolsAfter).Error)
		assert.Equal(t, protocolsBefore, protocolsAfter, "a rejoin creates no protocol")

		// Each failing condition creates a node instead; the group's cap
		// is lifted for that.
		_, err = f.svc.Update(t.Context(), group.ID, GroupInput{Name: "g", ProtocolTemplate: json.RawMessage(`[{"type":"vmess","port":8080}]`)}, f.admin)
		require.NoError(t, err)
		f.clock.Advance(10 * time.Minute)
		createsANode := func(name string, request JoinRequest) {
			t.Helper()
			request.Token, request.Hostname = token, "m"
			joined, err := f.join(t, request)
			require.NoError(t, err, name)
			assert.True(t, joined.Created, name)
			assert.NotEqual(t, first.Node, joined.Node, name)
		}
		createsANode("reset", JoinRequest{MachineFingerprint: print, PublicAddresses: []string{"203.0.113.7"}, Reset: true})
		createsANode("another fingerprint", JoinRequest{MachineFingerprint: fingerprint("n"), PublicAddresses: []string{"203.0.113.7"}})
		createsANode("no fingerprint", JoinRequest{PublicAddresses: []string{"203.0.113.7"}})
		createsANode("another address", JoinRequest{MachineFingerprint: print, PublicAddresses: []string{"203.0.113.8"}})

		// Seen recently on the stream (its transport sighting), or by a
		// heartbeat: never handed to the joining machine. (A group and a
		// machine of their own: the nodes above share this machine's
		// fingerprint and address.)
		_, seenToken := f.createGroup(t, GroupInput{Name: "seen", Kind: "proxy"})
		seenPrint := fingerprint("s")
		online, err := f.join(t, JoinRequest{Token: seenToken, MachineFingerprint: seenPrint, Hostname: "s", PublicAddresses: []string{"203.0.113.20"}})
		require.NoError(t, err)
		f.clock.Advance(10 * time.Minute)
		require.NoError(t, f.db.Create(&model.AgentTransport{
			NodeKind: "proxy", NodeID: uint(online.Node.ID), Transport: model.AgentTransportMTLSStream,
			FirstSeenAt: f.clock.Now(), LastSeenAt: f.clock.Now().Add(-time.Minute),
		}).Error)
		seen, err := f.join(t, JoinRequest{Token: seenToken, MachineFingerprint: seenPrint, Hostname: "s", PublicAddresses: []string{"203.0.113.20"}})
		require.NoError(t, err)
		assert.True(t, seen.Created, "seen on the stream")
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", seen.Node.ID).Update("approval", model.NodeApprovalRejected).Error)
		require.NoError(t, f.db.Where("node_id = ?", online.Node.ID).Delete(&model.AgentTransport{}).Error)
		heartbeat := f.clock.Now().Add(-2 * time.Minute).Unix()
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", online.Node.ID).Update("last_check_at", heartbeat).Error)
		seen, err = f.join(t, JoinRequest{Token: seenToken, MachineFingerprint: seenPrint, Hostname: "s", PublicAddresses: []string{"203.0.113.20"}})
		require.NoError(t, err)
		assert.True(t, seen.Created, "seen by a heartbeat")
		require.NoError(t, f.db.Model(&model.Node{}).Where("id = ?", seen.Node.ID).Update("approval", model.NodeApprovalRejected).Error)
		f.clock.Advance(10 * time.Minute)
		quiet, err := f.join(t, JoinRequest{Token: seenToken, MachineFingerprint: seenPrint, Hostname: "s", PublicAddresses: []string{"203.0.113.20"}})
		require.NoError(t, err)
		assert.False(t, quiet.Created, "quiet for five minutes: rejoined")
		assert.Equal(t, online.Node, quiet.Node)

		// Later, quiet again: but the nodes created meanwhile share the
		// fingerprint and the address, so the newest quiet one is rejoined,
		// never a node of another group.
		f.clock.Advance(10 * time.Minute)
		again, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: print, Hostname: "m", PublicAddresses: []string{"203.0.113.7"}})
		require.NoError(t, err)
		assert.False(t, again.Created)
		_, otherToken := f.createGroup(t, GroupInput{Name: "other", Kind: "proxy"})
		elsewhere, err := f.join(t, JoinRequest{Token: otherToken, MachineFingerprint: print, Hostname: "m", PublicAddresses: []string{"203.0.113.7"}})
		require.NoError(t, err)
		assert.True(t, elsewhere.Created, "a fingerprint never rejoins across groups")

		// A rejected node is never rejoined.
		var latest model.Node
		require.NoError(t, f.db.First(&latest, again.Node.ID).Error)
		require.NoError(t, f.db.Model(&model.Node{}).Where("node_group_id = ?", group.ID).Updates(map[string]any{"approval": model.NodeApprovalRejected}).Error)
		fresh2, err := f.join(t, JoinRequest{Token: token, MachineFingerprint: print, Hostname: "m", PublicAddresses: []string{"203.0.113.7"}})
		require.NoError(t, err)
		assert.True(t, fresh2.Created)
		assert.Equal(t, "m", fresh2.NodeName, "rejected nodes do not hold their names")
	})
}

func TestJoinPendingApprovalAndDecisions(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, f *fixture) {
		_, token := f.createGroup(t, GroupInput{Name: "g", Kind: "proxy", RequireApproval: true})
		_, forwardToken := f.createGroup(t, GroupInput{Name: "f", Kind: "forward", RequireApproval: true})
		joined, err := f.join(t, JoinRequest{Token: token, Hostname: "p", PublicAddresses: []string{"203.0.113.1"}})
		require.NoError(t, err)
		assert.Equal(t, model.NodeApprovalPending, joined.Approval)
		pending, err := PendingApproval(t.Context(), f.db, joined.Node)
		require.NoError(t, err)
		assert.True(t, pending)
		forward, err := f.join(t, JoinRequest{Token: forwardToken, Hostname: "f", PublicAddresses: []string{"203.0.113.2"}})
		require.NoError(t, err)
		assert.Equal(t, model.NodeApprovalPending, forward.Approval)

		heard := make(chan agentcontrol.AgentNode, 4)
		cancel := OnApproval(func(node agentcontrol.AgentNode) { heard <- node })
		defer cancel()

		decision, err := f.svc.Approve(t.Context(), joined.Node, f.admin)
		require.NoError(t, err)
		assert.Equal(t, NodeDecision{Node: joined.Node.String(), Name: "p", Approval: model.NodeApprovalApproved}, decision)
		select {
		case node := <-heard:
			assert.Equal(t, joined.Node, node)
		case <-time.After(time.Second):
			t.Fatal("the approval was not announced")
		}
		pending, err = PendingApproval(t.Context(), f.db, joined.Node)
		require.NoError(t, err)
		assert.False(t, pending)
		_, err = f.svc.Approve(t.Context(), joined.Node, f.admin)
		assert.ErrorIs(t, err, ErrNotPending)
		_, err = f.svc.Reject(t.Context(), joined.Node, f.admin)
		assert.ErrorIs(t, err, ErrNotPending, "an approved node is not rejected")
		_, err = f.svc.Approve(t.Context(), agentcontrol.AgentNode{Kind: "proxy", ID: 4242}, f.admin)
		assert.ErrorIs(t, err, ErrNodeNotFound)

		// Rejecting disables the node and revokes its certificates.
		decision, err = f.svc.Reject(t.Context(), forward.Node, f.admin)
		require.NoError(t, err)
		assert.Equal(t, model.NodeApprovalRejected, decision.Approval)
		var node model.ForwardNode
		require.NoError(t, f.db.First(&node, forward.Node.ID).Error)
		assert.False(t, node.Enabled)
		assert.Equal(t, model.NodeApprovalRejected, node.Approval)
		var certificate model.AgentCertificate
		require.NoError(t, f.db.Where("serial = ?", forward.Issued.Serial).First(&certificate).Error)
		require.NotNil(t, certificate.RevokedAt)
		assert.Equal(t, agentpki.RevokeReasonNodeRejected, certificate.RevokeReason)
		revoked, err := f.pki.IsRevoked(t.Context(), forward.Issued.Serial, forward.Node)
		require.NoError(t, err)
		assert.True(t, revoked)
		select {
		case node := <-heard:
			t.Fatalf("a rejection was announced as an approval: %s", node)
		default:
		}

		// A proxy node rejected: disabled.
		other, err := f.join(t, JoinRequest{Token: token, Hostname: "q", PublicAddresses: []string{"203.0.113.3"}})
		require.NoError(t, err)
		_, err = f.svc.Reject(t.Context(), other.Node, f.admin)
		require.NoError(t, err)
		var proxy model.Node
		require.NoError(t, f.db.First(&proxy, other.Node.ID).Error)
		assert.Equal(t, model.NodeStatusDisabled, proxy.Status)
		assert.Len(t, f.audits(t, AuditActionNodeApprove), 1)
		assert.Len(t, f.audits(t, AuditActionNodeReject), 2)
	})
}

func TestJoinNotifiesAfterTheCommit(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	_, token := f.createGroup(t, GroupInput{Name: "HK", Kind: "proxy", RequireApproval: true})
	type notice struct {
		joined Joined
		stored bool
	}
	notices := make(chan notice, 1)
	f.svc.OnJoin = func(joined Joined) {
		// The notifier runs after the commit: the node is there.
		var count int64
		err := f.db.Model(&model.Node{}).Where("id = ?", joined.Node.ID).Count(&count).Error
		notices <- notice{joined: joined, stored: err == nil && count == 1}
	}
	joined, err := f.join(t, JoinRequest{Token: token, RequestedName: "hk-01", PublicAddresses: []string{"203.0.113.5"}})
	require.NoError(t, err)
	select {
	case got := <-notices:
		assert.True(t, got.stored)
		assert.Equal(t, joined.Node, got.joined.Node)
		title, content, data := got.joined.Notification()
		assert.Equal(t, fmt.Sprintf("节点待审批：hk-01 (%s)", joined.Node), title)
		assert.Contains(t, content, "节点组 [HK]")
		assert.Contains(t, content, "203.0.113.5")
		assert.Contains(t, content, "批准前")
		assert.Equal(t, joined.Node.String(), data["node"])
		assert.NotContains(t, content, token)
	case <-time.After(5 * time.Second):
		t.Fatal("no notification")
	}

	// A refused join notifies no one.
	unknown, err := newJoinToken()
	require.NoError(t, err)
	_, err = f.join(t, JoinRequest{Token: unknown, PublicAddresses: []string{"203.0.113.6"}})
	require.ErrorIs(t, err, ErrJoinRejected)
	select {
	case <-notices:
		t.Fatal("a refused join was notified")
	case <-time.After(100 * time.Millisecond):
	}
	approved := Joined{Node: joined.Node, NodeName: "hk-01", GroupName: "HK", Created: false, Approval: model.NodeApprovalApproved, PeerAddress: "198.51.100.1"}
	title, content, _ := approved.Notification()
	assert.Equal(t, fmt.Sprintf("节点已重新接入：hk-01 (%s)", joined.Node), title)
	assert.Contains(t, content, "198.51.100.1")
}
