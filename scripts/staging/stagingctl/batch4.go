package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// Batch 4: subscription + forward + proxy-node (+ gost-mesh, wireguard).
//
// Background workers that touch these tables run on every Control of the
// stack (cmd/server/singleton_workers.go). The seed keeps them inert or
// deterministic:
//   - forward nodes have no metrics port, so the gost stats worker collects
//     nothing; every forward runs on the gost backend, so the Ansible stats
//     worker has nothing either; no runtime job is seeded;
//   - the forward flow reset worker resets permissions whose
//     flow_reset_time is today's day: every permission has 0 (never);
//     it also pauses the active forwards of expired members and disables
//     expired permissions, so the expired member's forward and permission
//     are seeded paused/disabled already and every other expiry is far away;
//   - the latency prober dials the forward targets (TEST-NET, unreachable)
//     and appends buckets with the clock's time: v2_forward_latency_bucket
//     is never compared, and the seeded tunnel_node buckets use the
//     prober's own target keys (node id, host, port), so each node keeps
//     one key and the multi-ingress answer stays deterministic.

// Subscription group indexes in World.IDs["sub_group"].
const (
	sgDefault = iota
	sgPremium
	sgStreaming
	sgRetired
	sgEmpty
	sgJapan
	sgScratch
)

// Forward node indexes in World.IDs["fwd_node"].
const (
	fnRelay0 = iota // relay-hk-01
	fnRelay1        // relay-hk-02
	fnRelay2        // relay-sh-03, disabled
	fnRelay3        // Relay-GZ-04, type "Relay" (mixed case)
	fnExit0         // exit-jp-01
	fnExit1         // exit-us-02, offline
	fnExit2         // exit-sg-03
)

// Tunnel indexes in World.IDs["tunnel"]; b4Tunnels holds their names.
const (
	tnHKJP      = iota // relay0 -> exit0, tunnel forwarding, forwards and permissions
	tnHKPort           // relay0, port forwarding, the user's forward
	tnDisabled         // relay1, disabled, a permission of the user
	tnUS               // relay1 -> exit1, mTLS, user2 and members
	tnSG               // relay3, port forwarding, user2
	tnUnusedA          // relay0, nothing uses it (deleted by the replay)
	tnUnusedB          // relay1 -> exit2, nothing uses it
	tnPermOnly         // relay0, a permission only
	tnNoIngress        // no entry node, a member's forward
)

var b4Tunnels = []string{
	"港日 HK-JP 隧道", "HK 直连 port", "SH 停用 disabled", "US mTLS tunnel", "SG 端口 port",
	"Unused 未使用 A", "Unused 未使用 B", "Permission only 仅授权", "No ingress 无入口",
}

// Speed limit indexes in World.IDs["speed_limit"].
const (
	slFast   = iota // tunnel HK-JP, the user's permission
	slSlow          // tunnel US, user2's permission
	slIdle          // tunnel SG, unused
	slOff           // disabled tunnel, inactive, unused
	slMember        // tunnel HK-JP, members' permissions
)

// b4Expiry is a permission expiry far in the future (2030-01-01, ms).
const b4Expiry = int64(1893456000000)

func init() {
	registerSeed(80, "proxy nodes, protocols, logs and load balancers", seedProxyNodes)
	registerSeed(81, "subscription groups, templates and links", seedSubscriptionGroups)
	registerSeed(82, "forward nodes, tunnels, permissions, forwards and speed limits", seedForwarding)

	registerTables("proxy-node", TableSpec{Table: "v2_load_balancer", Ignore: timestamps()})
	registerTables("subscription",
		TableSpec{Table: "v2_subscription_group", Ignore: timestamps()},
		TableSpec{Table: "v2_subscription_template", Ignore: timestamps()},
		TableSpec{Table: "v2_subscription_group_node_protocols", Key: "subscription_group_id,node_protocol_id"},
		TableSpec{Table: "v2_plan_subscription_group", Ignore: map[string]string{"created_at": "set from the handler's clock"}},
		TableSpec{Table: "v2_user_subscription_group", Ignore: map[string]string{"created_at": "set from the handler's clock"}},
		// The subscriber request ledger and change log are written by the
		// membership routes (KernelSubscriber) and by the forward package's
		// traffic reset; they are compared once, here.
		TableSpec{Table: "v4_kernel_subscriber_request", Key: "method,user_id,result", Ignore: map[string]string{
			"request_id": "derived from the request's Idempotency-Key or X-Request-ID, else a random UUID per twin (the replayer sends neither); rows are keyed by method, user and result instead",
			"created_at": "set from the handler's clock",
		}},
		TableSpec{Table: "v4_kernel_subscriber_change", Ignore: map[string]string{"created_at": "set from the handler's clock"}},
	)
	registerTables("forward",
		TableSpec{Table: "v2_forward", Ignore: timestamps()},
		TableSpec{Table: "v2_forward_tunnel", Ignore: timestamps()},
		TableSpec{Table: "v2_forward_user_tunnel", Ignore: timestamps()},
		TableSpec{Table: "v2_speed_limit", Ignore: map[string]string{
			"created_time": "milliseconds from the handler's clock",
			"updated_time": "milliseconds from the handler's clock",
		}},
		// POST /api/v2/user/reset type 1 zeroes u and d through
		// KernelSubscriber.ResetTraffic.
		TableSpec{Table: "v2_user", Ignore: map[string]string{
			"updated_at":    "set from the handler's clock",
			"last_login_at": "the replayer logs every persona in on each twin",
		}},
		// A guard: no batch-4 write route may queue a runtime job (none is
		// seeded, so both twins must stay empty).
		TableSpec{Table: "v2_forward_runtime_job", Ignore: timestamps(
			"started_at", "set by the job executor's clock",
			"claimed_at", "set by the job executor's clock",
			"completed_at", "set by the job executor's clock",
		)},
	)

	registerSpecs(subscriptionSpecs()...)
	registerSpecs(forwardSpecs()...)
	registerSpecs(proxyNodeSpecs()...)
	registerSpecs(gostMeshSpecs()...)
	registerSpecs(wireguardSpecs()...)
}

// ---------------------------------------------------------------------------
// Seed.

func (s *Seeder) exec(statement string, args ...any) {
	if err := s.DB.Exec(statement, args...).Error; err != nil {
		panic(fmt.Errorf("seed: %s: %w", statement, err))
	}
}

// updateColumns writes columns GORM's Create replaces with their defaults
// (a zero value over a column default).
func (s *Seeder) updateColumns(value any, id uint, columns map[string]any) {
	if err := s.DB.Model(value).Where("id = ?", id).UpdateColumns(columns).Error; err != nil {
		panic(fmt.Errorf("seed: update %T %d: %w", value, id, err))
	}
}

func fakeHex(s *Seeder, words int) string {
	var b strings.Builder
	for i := 0; i < words; i++ {
		fmt.Fprintf(&b, "%016x", s.Rand.Uint64())
	}
	return b.String()
}

// seedProxyNodes writes proxy nodes in every status with protocols of
// every type (fake keys), runtime logs of three nodes and load balancers.
func seedProxyNodes(s *Seeder) {
	groups := s.World.IDs["node_group"]
	type shape struct {
		name, host string
		status     model.NodeStatus
		checked    bool
	}
	shapes := []shape{
		{"香港 HK-01", "hk-01.nodes.example.com", model.NodeStatusOnline, true},
		{"香港 HK-02", "192.0.2.12", model.NodeStatusOnline, true},
		{"日本 JP-01", "jp-01.nodes.example.net", model.NodeStatusOnline, true},
		{"日本 JP-02", "192.0.2.22", model.NodeStatusOffline, true},
		{"美国 US-01", "us-01.nodes.example.org", model.NodeStatusOnline, true},
		{"新加坡 SG-01", "198.51.100.31", model.NodeStatusOnline, true},
		{"Pending 待审核", "pending.nodes.example.com", model.NodeStatusPending, false},
		{"Disabled 已禁用", "203.0.113.41", model.NodeStatusDisabled, true},
		{"Offline 离线", "offline.nodes.example.net", model.NodeStatusOffline, true},
		{"Relay child 中转", "192.0.2.50", model.NodeStatusOnline, true},
	}
	nodes := make([]model.Node, len(shapes))
	for i, sh := range shapes {
		created := s.At(Days(-160 + i))
		node := model.Node{
			Name: sh.name, Host: sh.host, Port: 8443 + i, APIKey: fmt.Sprintf("stg-node-%02d-%016x", i+1, s.Rand.Uint64()),
			APIKeyHash: fakeHex(s, 4), Secret: fmt.Sprintf("fake-staging-node-secret-%02d", i+1), Status: sh.status,
			GroupID: ptr(groups[i%len(groups)]), Rate: 1 + float64(i%3)*0.5, TrafficRate: 1, Sort: i % 4, Show: 1,
			MonthlyResetDay: 1 + i%28, ServerIP: ptr(fmt.Sprintf("192.0.2.%d", 100+i)), ServerVersion: ptr(fmt.Sprintf("1.8.%d", i%5)),
			ServerOS: ptr("Debian GNU/Linux 12"), CPUUsage: float64(s.Rand.Intn(900)) / 10, MemoryUsage: float64(s.Rand.Intn(900)) / 10,
			DiskUsage: float64(s.Rand.Intn(900)) / 10, Uptime: int64(s.Rand.Intn(5000000)), OnlineUsers: s.Rand.Intn(80),
			RuntimeHealthy: true, TotalUpload: int64(s.Rand.Intn(4000)) << 30, TotalDownload: int64(s.Rand.Intn(12000)) << 30,
			CreatedAt: created, UpdatedAt: created.Add(time.Hour),
		}
		if i%4 == 1 {
			node.Tags = ptr(`["streaming","ipv6"]`)
		}
		if i == 2 {
			node.MonthlyLimit = ptr(int64(2000) << 30)
		}
		if sh.checked {
			node.LastCheckAt = ptr(s.At(-time.Duration(i*7) * time.Minute).Unix())
			node.RuntimeCheckedAt = node.LastCheckAt
		}
		if i == 9 {
			node.ParentID = &nodes[0].ID
		}
		s.Create(&node)
		nodes[i] = node
		s.World.Add("node", node.ID)
		if i < 3 {
			s.World.Add("node.logs", node.ID)
		} else {
			s.World.Add("node.quiet", node.ID)
		}
	}
	// Show and RuntimeHealthy default to on: the disabled node is hidden
	// and unhealthy after its creation.
	s.updateColumns(&model.Node{}, nodes[7].ID, map[string]any{"show": 0, "runtime_healthy": false, "runtime_error": "xray: exit status 1"})

	type proto struct {
		kind              model.ProtocolType
		name              string
		tls               int
		transport         string
		settings, reality string
	}
	protos := []proto{
		{model.ProtocolVLESS, "vless-reality", 2, "tcp", `{"flow":"xtls-rprx-vision"}`,
			`{"private_key":"FAKE-staging-reality-private-key-%02d","short_id":"%08x","dest":"www.example.com:443"}`},
		{model.ProtocolVMess, "vmess-ws", 1, "ws", `{}`, ""},
		{model.ProtocolTrojan, "trojan", 1, "tcp", `{}`, ""},
		{model.ProtocolShadowsocks, "ss2022", 0, "tcp", `{"method":"2022-blake3-aes-128-gcm","password":"RkFLRS1zdGFnaW5nLXNzLWtleQ=="}`, ""},
		{model.ProtocolHysteria2, "hy2", 1, "udp", `{"up_mbps":100,"down_mbps":100,"obfs":"salamander","obfs-password":"fake-obfs"}`, ""},
		{model.ProtocolWireGuard, "wireguard", 0, "udp",
			`{"cidr":"10.66.0.0/24","server_address":"10.66.0.1/24","private_key":"RkFLRS1zdGFnaW5nLXdnLXByaXZhdGUta2V5LTAwMDA=","server_public_key":"RkFLRS1zdGFnaW5nLXdnLXB1YmxpYy1rZXktMDAwMDA=","mtu":1280}`, ""},
	}
	for i, node := range nodes {
		count := 2 + i%2
		for j := 0; j < count; j++ {
			p := protos[(i+j)%len(protos)]
			created := node.CreatedAt.Add(time.Duration(j+1) * time.Hour)
			protocol := model.NodeProtocol{
				NodeID: node.ID, Name: fmt.Sprintf("%s-%02d", p.name, i+1), Type: p.kind, Port: 443 + j*1000 + i, Enable: 1, Show: 1,
				Sort: j, TLS: p.tls, Settings: ptr(p.settings), Transport: ptr(p.transport), CreatedAt: created, UpdatedAt: created,
			}
			if p.tls == 1 {
				protocol.TLSSettings = ptr(fmt.Sprintf(`{"server_name":"%s","allowInsecure":false}`, node.Host))
				protocol.ALPN = ptr("h2,http/1.1")
			}
			if p.reality != "" {
				protocol.RealitySettings = ptr(fmt.Sprintf(p.reality, i+1, s.Rand.Uint32()))
			}
			if p.transport == "ws" {
				protocol.TransportSettings = ptr(`{"path":"/staging-ws","headers":{"Host":"cdn.example.com"}}`)
			}
			if j == 1 && i%3 == 0 {
				protocol.Host = ptr("edge.example.org")
			}
			s.Create(&protocol)
			if j == 2 {
				// Show defaults to on: a hidden protocol after its creation.
				s.updateColumns(&model.NodeProtocol{}, protocol.ID, map[string]any{"show": 0})
			}
			s.World.Add("node_protocol", protocol.ID)
		}
	}

	levels := []string{"info", "warning", "error", "debug"}
	sources := []string{"xray", "agent", "sing-box", "wireguard"}
	messages := []string{
		"started inbound vless-reality on :443", "slow upstream <edge> 203.0.113.9", "config reload failed: invalid json",
		"heartbeat ok", "peer added 10.66.0.%d", "用户连接数超限 user limit reached",
	}
	fields := []string{`{"pid":%d,"version":"1.8.4"}`, `[1,2,3]`, `12345678901234567890`, `not json`, ``}
	for n, count := range []int{45, 10, 3} {
		for i := 0; i < count; i++ {
			message := messages[(i+n)%len(messages)]
			if strings.Contains(message, "%d") {
				message = fmt.Sprintf(message, 2+i)
			}
			field := fields[i%len(fields)]
			if strings.Contains(field, "%d") {
				field = fmt.Sprintf(field, 1000+i)
			}
			created := s.At(-Days(3) + time.Duration(i*7)*time.Minute)
			entry := model.NodeLog{
				NodeID: nodes[n].ID, Level: levels[i%len(levels)], Source: sources[(i/2)%len(sources)], Message: message,
				FieldsJSON: field, CreatedAt: created, UpdatedAt: created,
			}
			if i%3 != 2 {
				entry.TraceID = fmt.Sprintf("trace-%02d-%04d", n, i)
			}
			if i%5 != 4 {
				// The node's own time, a little before the time it was stored;
				// some logs share it, so the id breaks the tie.
				entry.LoggedAt = ptr(created.Add(-time.Duration(30+(i%2)*30) * time.Second))
			}
			s.Create(&entry)
		}
	}

	type balancer struct {
		name, strategy, weights string
		group, interval         int
		off                     bool
	}
	balancers := []balancer{
		{"lb-hk 香港", "round-robin", fmt.Sprintf(`{"%d":3,"%d":1}`, nodes[0].ID, nodes[1].ID), 0, 60, false},
		{"lb-jp 日本", "weight", "not json", 3, 30, false},
		{"lb-us", "latency", `[1,2]`, 1, 120, false},
		{"lb-sg", "random", "", 1, 60, false},
		{"lb-disabled 停用", "least-load", fmt.Sprintf(`{"%d":2}`, nodes[5].ID), 2, 15, true},
		{"lb-weighted", "weighted-random", fmt.Sprintf(`{"%d":5,"%d":5,"%d":1}`, nodes[2].ID, nodes[3].ID, nodes[4].ID), 3, 45, false},
	}
	for i, b := range balancers {
		created := s.At(Days(-90+i) + time.Duration(i)*time.Hour)
		row := model.LoadBalancer{
			Name: b.name, GroupID: groups[b.group%len(groups)], Strategy: b.strategy, HealthCheck: true, CheckInterval: b.interval,
			CheckTimeout: 5 + i, NodeWeights: b.weights, Enabled: true, CreatedAt: created, UpdatedAt: created.Add(time.Minute),
		}
		s.Create(&row)
		if b.off {
			s.updateColumns(&model.LoadBalancer{}, row.ID, map[string]any{"health_check": false, "enabled": false})
		}
		s.World.Add("loadbalancer", row.ID)
	}
}

// seedSubscriptionGroups writes subscription groups (one disabled, one
// empty, one the replay deletes), templates of every protocol type with fake
// keys, links to node protocols and plans, and memberships of the personas
// and other members, expiring, expired and without expiry.
func seedSubscriptionGroups(s *Seeder) {
	type group struct {
		name        string
		description *string
		priority    int
	}
	groups := []group{
		{"默认 Default", nil, 0},
		{"高级 Premium", ptr("fast nodes 高速节点"), 10},
		{"流媒体 Streaming", ptr("Netflix / Disney+"), 5},
		{"停用 Retired", nil, 5},
		{"空分组 Empty", nil, 10},
		{"日本 Japan", ptr("東京・大阪"), 3},
		{"Scratch 待删除", ptr("deleted by the rehearsal"), 1},
	}
	ids := make([]uint, len(groups))
	for i, g := range groups {
		created := s.At(Days(-140 + i))
		row := model.SubscriptionGroup{Name: g.name, Description: g.description, Priority: g.priority, Enable: 1, CreatedAt: created, UpdatedAt: created}
		s.Create(&row)
		ids[i] = row.ID
		s.World.Add("sub_group", row.ID)
	}
	s.updateColumns(&model.SubscriptionGroup{}, ids[sgRetired], map[string]any{"enable": 0})

	type template struct {
		group             int
		name, kind, host  string
		port, tls, sort   int
		transport         string
		transportSettings *string
		off               bool
	}
	templates := []template{
		{sgDefault, "HK vless reality", "vless", "hk.sub.example.com", 443, 2, 2, "tcp", nil, false},
		{sgDefault, "JP vmess ws", "vmess", "jp.sub.example.com", 443, 1, 1, "ws", ptr(`{"path":"/ws"}`), false},
		{sgDefault, "HK trojan", "trojan", "192.0.2.80", 8443, 1, 1, "tcp", nil, false},
		{sgPremium, "US hysteria2", "hysteria2", "us.sub.example.net", 443, 1, 0, "udp", nil, false},
		{sgPremium, "SG shadowsocks", "shadowsocks", "198.51.100.81", 8388, 0, 0, "tcp", nil, false},
		{sgPremium, "HK tuic", "tuic", "hk2.sub.example.com", 443, 1, 3, "udp", nil, false},
		{sgStreaming, "Netflix vless", "vless", "nf.sub.example.org", 443, 2, 0, "grpc", ptr(`{"serviceName":"grpc"}`), false},
		{sgStreaming, "Disney anytls", "anytls", "203.0.113.82", 443, 1, 1, "tcp", nil, false},
		{sgRetired, "old vmess", "vmess", "old.sub.example.com", 80, 0, 0, "tcp", nil, true},
		{sgJapan, "Tokyo vless", "vless", "tyo.sub.example.net", 443, 2, 0, "xhttp", ptr(`{"path":"/x","mode":"auto"}`), false},
		{sgJapan, "Osaka trojan", "trojan", "osa.sub.example.net", 443, 1, 0, "ws", ptr(`{"path":"/trojan"}`), false},
		{sgScratch, "scratch vless", "vless", "scratch.sub.example.com", 443, 2, 0, "tcp", nil, false},
		{sgScratch, "scratch ss", "shadowsocks", "192.0.2.83", 8389, 0, 1, "tcp", nil, false},
	}
	for i, t := range templates {
		created := s.At(Days(-130+i) + time.Duration(i)*time.Minute)
		row := model.SubscriptionTemplate{
			GroupID: ids[t.group], Name: t.name, Type: t.kind, Enable: 1, Sort: t.sort, Server: t.host, Port: t.port,
			TLS: t.tls, Transport: t.transport, TransportSettings: t.transportSettings, CreatedAt: created, UpdatedAt: created,
			TemplateJSON: `{"name":"{{.Email}} ` + t.name + `"}`,
		}
		if t.tls > 0 {
			row.ServerName = ptr(t.host)
			row.TLSFingerprint = ptr("chrome")
			row.ALPN = ptr("h2,http/1.1")
		}
		if t.tls == 2 {
			row.RealityPublicKey = ptr(fmt.Sprintf("FAKE-staging-reality-pbk-%02d", i))
			row.RealityShortID = ptr(fmt.Sprintf("%08x", s.Rand.Uint32()))
			row.RealityDest = ptr("www.example.com:443")
			row.Flow = ptr("xtls-rprx-vision")
		}
		if t.kind == "shadowsocks" {
			row.SSCipher = ptr("2022-blake3-aes-128-gcm")
			row.SSServerKey = ptr("RkFLRS1zdGFnaW5nLXNzLXNlcnZlcg==")
			row.Tags = ptr(`["ss","sg"]`)
		}
		if t.kind == "hysteria2" {
			row.ProtocolSettings = ptr(`{"up_mbps":100,"down_mbps":200}`)
		}
		s.Create(&row)
		if t.off {
			s.updateColumns(&model.SubscriptionTemplate{}, row.ID, map[string]any{"enable": 0})
		}
		if t.group == sgScratch {
			s.World.Add("sub_template.scratch", row.ID)
		} else {
			s.World.Add("sub_template", row.ID)
		}
	}

	// Node protocol links: the first protocols of some nodes per group.
	protocols := s.World.IDs["node_protocol"]
	links := map[int][]int{
		sgDefault: {0, 1, 3}, sgPremium: {4, 5, 6, 8}, sgStreaming: {10}, sgRetired: {20}, sgJapan: {6, 7, 9}, sgScratch: {0, 12},
	}
	for _, g := range []int{sgDefault, sgPremium, sgStreaming, sgRetired, sgJapan, sgScratch} {
		for _, p := range links[g] {
			s.exec("INSERT INTO v2_subscription_group_node_protocols (subscription_group_id, node_protocol_id) VALUES (?, ?)",
				ids[g], protocols[p%len(protocols)])
		}
	}

	plans := s.World.IDs["plan"]
	planLinks := []struct{ plan, group int }{
		{0, sgDefault}, {1, sgDefault}, {1, sgPremium}, {2, sgPremium}, {2, sgStreaming}, {5, sgJapan}, {3, sgScratch},
	}
	for i, l := range planLinks {
		s.Create(&model.PlanSubscriptionGroup{PlanID: plans[l.plan], GroupID: ids[l.group], CreatedAt: s.At(Days(-120 + i))})
	}

	member := func(user uint, g int, expire *int64, transfer *int64, renew *int64, offset int) {
		s.Create(&model.UserSubscriptionGroup{
			UserID: user, GroupID: ids[g], ExpireAt: expire, TransferEnable: transfer, NextRenewPrice: renew,
			CreatedAt: s.At(Days(-100 + offset)),
		})
	}
	unix := func(days int) *int64 { return ptr(s.At(Days(days)).Unix()) }
	userID, user2ID := s.World.PersonaID(User), s.World.PersonaID(User2)
	member(userID, sgDefault, nil, nil, nil, 0)
	member(userID, sgPremium, unix(365), ptr(int64(200)<<30), nil, 1)
	member(userID, sgStreaming, unix(-30), nil, nil, 2)
	member(userID, sgScratch, nil, nil, nil, 3)
	member(user2ID, sgPremium, unix(180), nil, ptr(int64(1990)), 4)
	member(s.World.PersonaID(Banned), sgPremium, unix(-1), nil, nil, 5)
	member(s.World.PersonaID(Expired), sgDefault, nil, nil, nil, 6)
	members := s.World.IDs["user.member"]
	extra := []int{sgPremium, sgStreaming, sgJapan, sgScratch}
	for i := 2; i < 22 && i < len(members); i++ {
		member(members[i], sgDefault, nil, nil, nil, 10+i)
		var expire *int64
		if i%3 != 0 {
			expire = unix(30 + 10*i)
		}
		member(members[i], extra[i%len(extra)], expire, nil, nil, 30+i)
	}
	if len(members) > 22 {
		member(members[22], sgRetired, nil, nil, nil, 60)
	}
}

// seedForwarding writes the Flux-style forward data: relay and exit nodes,
// tunnels of both types (one disabled, two unused, one without an entry
// node), speed limits, tunnel permissions, forwards of the user and user2
// personas and of other members, legacy forward rules and latency buckets.
func seedForwarding(s *Seeder) {
	type node struct {
		name, kind, host string
		status           int
	}
	nodeShapes := []node{
		{"relay-hk-01", "relay", "198.51.100.11", 1},
		{"relay-hk-02", "relay", "198.51.100.12", 1},
		{"relay-sh-03", "relay", "198.51.100.13", 1},
		{"Relay-GZ-04", "Relay", "relay-gz.example.net", 1},
		{"exit-jp-01", "exit", "203.0.113.21", 1},
		{"exit-us-02", "exit", "203.0.113.22", 0},
		{"exit-sg-03", "exit", "exit-sg.example.org", 1},
	}
	nodes := make([]model.ForwardNode, len(nodeShapes))
	regions := []string{"HK", "HK", "SH", "GZ", "JP", "US", "SG"}
	for i, n := range nodeShapes {
		created := s.At(Days(-110 + i))
		nodes[i] = model.ForwardNode{
			Name: n.name, Type: n.kind, Host: n.host, Port: 8001 + i, APIPort: 9001 + i,
			APIToken: fmt.Sprintf("fake-gost-api-token-%02d", i+1), Region: regions[i], ISP: "Example ISP", Datacenter: "dc-" + strings.ToLower(regions[i]),
			Bandwidth: int64(1000 * (1 + i%3)), Status: n.status, LastCheck: s.At(-time.Duration(i+1) * time.Hour), Latency: 10 + 7*i,
			Load: float64(i) / 10, Uptime: 99.9 - float64(i), Tags: `["staging"]`, Weight: 1 + i%3, MaxConn: 1000,
			Enabled: true, TotalUpload: int64(i+1) << 33, TotalDownload: int64(i+1) << 35, CurrentConn: 3 * i,
			CreatedAt: created, UpdatedAt: created,
		}
		s.Create(&nodes[i])
		s.World.Add("fwd_node", nodes[i].ID)
	}
	s.updateColumns(&model.ForwardNode{}, nodes[fnRelay2].ID, map[string]any{"enabled": false})

	type tunnel struct {
		in, out    int // node indexes, -1 for none
		kind, flow int
		protocol   string
		off        bool
	}
	tunnelShapes := []tunnel{
		tnHKJP:      {fnRelay0, fnExit0, 2, 2, "tls", false},
		tnHKPort:    {fnRelay0, fnRelay0, 1, 1, "tcp", false},
		tnDisabled:  {fnRelay1, fnRelay1, 1, 2, "tcp", true},
		tnUS:        {fnRelay1, fnExit1, 2, 2, "mtls", false},
		tnSG:        {fnRelay3, fnRelay3, 1, 2, "tcp", false},
		tnUnusedA:   {fnRelay0, fnRelay0, 1, 2, "tcp", false},
		tnUnusedB:   {fnRelay1, fnExit2, 2, 1, "tls", false},
		tnPermOnly:  {fnRelay0, fnRelay0, 1, 2, "tcp", false},
		tnNoIngress: {-1, fnRelay0, 1, 2, "tcp", false},
	}
	tunnels := make([]model.ForwardTunnel, len(tunnelShapes))
	for i, t := range tunnelShapes {
		created := s.At(Days(-100) + time.Duration(i)*time.Hour)
		row := model.ForwardTunnel{
			Name: b4Tunnels[i], Type: t.kind, Flow: t.flow, Protocol: t.protocol, TrafficRatio: 1 + float64(i%3)*0.5,
			InterfaceName: "eth0", TCPListenAddr: "0.0.0.0", UDPListenAddr: "[::]", Status: 1,
			InNodePortSta: ptr(20000 + i*1000), InNodePortEnd: ptr(20999 + i*1000), CreatedAt: created, UpdatedAt: created,
		}
		if t.in >= 0 {
			row.InNodeID, row.InIP = nodes[t.in].ID, nodes[t.in].Host
		}
		row.OutNodeID, row.OutIP = &nodes[t.out].ID, nodes[t.out].Host
		s.Create(&row)
		if t.off {
			s.updateColumns(&model.ForwardTunnel{}, row.ID, map[string]any{"status": 0})
		}
		tunnels[i] = row
		s.World.Add("tunnel", row.ID)
	}

	type limit struct {
		name   string
		tunnel int
		speed  int64
		off    bool
	}
	limits := []limit{
		slFast: {"fast 极速 50M", tnHKJP, 51200, false}, slSlow: {"slow 5M", tnUS, 5120, false},
		slIdle: {"idle 空闲", tnSG, 1024, false}, slOff: {"off 停用", tnDisabled, 512, true},
		slMember: {"member 10M", tnHKJP, 10240, false},
	}
	limitIDs := make([]uint, len(limits))
	for i, l := range limits {
		at := s.At(Days(-90 + i)).UnixMilli()
		row := model.SpeedLimit{CreatedTime: at, UpdatedTime: at + 60000, Status: 1, Name: l.name, Speed: l.speed,
			TunnelID: tunnels[l.tunnel].ID, TunnelName: tunnels[l.tunnel].Name}
		s.Create(&row)
		if l.off {
			s.updateColumns(&model.SpeedLimit{}, row.ID, map[string]any{"status": 0})
		}
		limitIDs[i] = row.ID
		s.World.Add("speed_limit", row.ID)
	}

	permission := func(key string, user uint, t int, speed *uint, flow int64, inFlow int64, exp int64, off bool, offset int) {
		created := s.At(Days(-80 + offset))
		row := model.ForwardUserTunnel{
			UserID: user, TunnelID: tunnels[t].ID, Flow: flow, Num: 5 + offset%5, InFlow: inFlow, OutFlow: inFlow / 2,
			ExpTime: exp, SpeedID: speed, Status: 1, CreatedAt: created, UpdatedAt: created,
		}
		s.Create(&row)
		if off {
			s.updateColumns(&model.ForwardUserTunnel{}, row.ID, map[string]any{"status": 0})
		}
		s.World.Add("user_tunnel", row.ID)
		if key != "" {
			s.World.Add(key, row.ID)
		}
	}
	userID, user2ID, expiredID := s.World.PersonaID(User), s.World.PersonaID(User2), s.World.PersonaID(Expired)
	// The user's HK-JP permission has less traffic than its forwards, so
	// the permission list raises it.
	permission("user_tunnel.user", userID, tnHKJP, &limitIDs[slFast], 500, 1<<30, b4Expiry, false, 0)
	permission("user_tunnel.user", userID, tnHKPort, nil, 100, 0, 0, true, 1)
	permission("user_tunnel.user", userID, tnDisabled, nil, 50, 0, b4Expiry, false, 2)
	permission("user_tunnel.user2", user2ID, tnUS, &limitIDs[slSlow], 200, 900000, b4Expiry, false, 3)
	permission("user_tunnel.user2", user2ID, tnSG, nil, 0, 0, 0, false, 4)
	// Expired before the data set's time and already disabled, as the flow
	// reset worker leaves it.
	permission("user_tunnel.expired", expiredID, tnHKJP, nil, 10, 0, s.At(-Days(10)).UnixMilli(), true, 5)
	members := s.World.IDs["user.member"]
	for i := 2; i < 10; i++ {
		if i%2 == 0 {
			permission("", members[i], tnHKJP, &limitIDs[slMember], int64(100*i), int64(i)<<28, b4Expiry, false, 6+i)
		} else {
			permission("", members[i], tnUS, nil, int64(50*i), int64(i)<<27, 0, false, 6+i)
		}
	}
	permission("", members[10], tnPermOnly, nil, 10, 0, b4Expiry, false, 20)

	emails := map[uint]string{}
	var users []model.User
	if err := s.DB.Select("id", "email").Find(&users).Error; err != nil {
		panic(err)
	}
	for _, u := range users {
		emails[u.ID] = u.Email
	}
	index := 0
	forward := func(key string, owner uint, t int, name, remote, strategy string, inx int, off bool) {
		created := s.At(Days(-70) + time.Duration(index)*time.Hour)
		synced := created.Add(30 * time.Minute)
		row := model.Forward{
			UserID: owner, UserName: emails[owner], Name: name, TunnelID: tunnels[t].ID, InPort: 21000 + index, OutPort: 0,
			RemoteAddr: remote, InterfaceName: "eth1", Strategy: strategy, Status: 1, RuntimeBackend: "gost", RuntimeStatus: 2,
			RuntimeMessage: "synced", RuntimeLastSyncAt: &synced, InFlow: int64(index+1) << 29, OutFlow: int64(index+1) << 26, Inx: inx,
			CreatedAt: created, UpdatedAt: created,
		}
		if index%5 == 4 {
			row.RuntimeLastSyncAt, row.RuntimeStatus, row.RuntimeMessage = nil, 0, ""
		}
		s.Create(&row)
		if off {
			s.updateColumns(&model.Forward{}, row.ID, map[string]any{"status": 0})
		}
		index++
		s.World.Add("forward", row.ID)
		s.World.Add(key, row.ID)
	}
	forward("forward.user", userID, tnHKJP, "web 网站 443", "203.0.113.50:443", "fifo", 0, false)
	forward("forward.user", userID, tnHKJP, "multi 多目标", "203.0.113.51:443,203.0.113.52:443", "round", 1, false)
	forward("forward.user", userID, tnHKPort, "ssh", "203.0.113.53:22", "fifo", 0, false)
	forward("forward.user", userID, tnHKJP, "game udp 游戏", "203.0.113.54:27015", "fifo", 2, true)
	forward("forward.user2", user2ID, tnUS, "user2 api", "203.0.113.60:8443", "fifo", 0, false)
	forward("forward.user2", user2ID, tnSG, "user2 sg", "203.0.113.61:443,203.0.113.62:443", "hash", 1, false)
	forward("forward.expired", expiredID, tnHKJP, "expired member", "203.0.113.63:443", "fifo", 0, true)
	for i := 2; i < 10; i++ {
		t := tnHKJP
		if i%2 == 1 {
			t = tnUS
		}
		forward("forward.other", members[i], t, fmt.Sprintf("member %d 转发", i), fmt.Sprintf("203.0.113.%d:%d", 100+i, 8000+i), "fifo", i%3, false)
	}
	forward("forward.noingress", members[11], tnNoIngress, "no ingress 无入口", "198.51.100.90:443", "fifo", 0, false)

	expires := s.At(Days(90))
	rule := func(key string, owner *uint, relay, exit int, port int) {
		row := model.ForwardRule{
			Name: fmt.Sprintf("rule-%d", port), Enabled: true, RelayNodeID: nodes[relay].ID, ListenPort: port, Protocol: "tcp",
			ExitNodeID: nodes[exit].ID, TargetHost: "203.0.113.9", TargetPort: 8443, UserID: owner, AllowedIPs: "198.51.100.0/24",
			SpeedLimit: ptr(int64(512)), TrafficLimit: ptr(int64(1) << 30), ExpireTime: &expires, Upload: int64(port), Download: int64(port) * 3,
			Connections: 1, TotalConns: 9, Remark: "synthetic rule", CreatedAt: s.At(Days(-60)), UpdatedAt: s.At(Days(-60)),
		}
		s.Create(&row)
		if key != "" {
			s.World.Add(key, row.ID)
		}
	}
	rule("forward_rule.user", &userID, fnRelay0, fnExit0, 30001)
	rule("forward_rule.user", &userID, fnRelay1, fnExit1, 30002)
	rule("forward_rule.user2", &user2ID, fnRelay0, fnExit2, 30003)
	rule("", nil, fnRelay3, fnExit0, 30004)

	// Latency buckets with the prober's own target keys.
	bucket := func(kind string, id uint, label, host string, port int, at time.Time, success int, avg float64) {
		s.Create(&model.ForwardLatencyBucket{
			TargetKey: fmt.Sprintf("%s:%d:%s:%d", kind, id, host, port), TargetType: kind, TargetID: id, Label: label, Host: host, Port: port,
			BucketAt: at, IntervalSeconds: 60, SampleCount: 5, SuccessCount: success, MinRTT: avg / 2, AvgRTT: avg, MaxRTT: avg * 2,
			P95RTT: avg * 1.8, LossPct: float64(5-success) * 20, CreatedAt: at, UpdatedAt: at,
		})
	}
	for i, n := range nodes {
		if i == fnRelay2 {
			continue
		}
		for b := 0; b < 2; b++ {
			at := s.At(-time.Duration(2-b) * time.Hour)
			success := 5 - b*(i%2)*5
			bucket("tunnel_node", n.ID, n.Name+" 入口", n.Host, n.Port, at, success, 12.5+float64(i))
			bucket("forward_node", n.ID, n.Name, n.Host, n.Port, at, success, 11+float64(i))
		}
	}
	bucket("forward", s.World.ID("forward.user", 0), "web 网站 443", "203.0.113.50", 443, s.At(-time.Hour), 4, 30.25)
}

// ---------------------------------------------------------------------------
// Request helpers.

// flux is a Flux-style POST request (JSON body, code/msg/ts/data envelope).
func flux(persona, path string, body any, label string, mask ...string) Req {
	return Req{Persona: persona, Path: path, Body: body, Label: label, Mask: mask}
}

// fluxForbidden is a member and an anonymous caller on an administrator
// route.
func fluxForbidden(path string, body any) []Req {
	return []Req{
		{Persona: User, Path: path, Body: body, Label: "member on admin route"},
		{Persona: Anon, Path: path, Body: body, Label: "anonymous"},
	}
}

func anyMap(pairs ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}

// ---------------------------------------------------------------------------
// Subscription.

func subscriptionSpecs() []RouteSpec {
	stamped := []string{"data.created_at", "data.updated_at"}
	updated := []string{"data.updated_at"}
	group := func(id any) string { return fill("/api/v2/admin/subscription/groups/:id", id) }
	templates := func(id any) string { return fill("/api/v2/admin/subscription/groups/:id/templates", id) }
	template := func(id any) string { return fill("/api/v2/admin/subscription/templates/:id", id) }
	protocols := func(id any) string { return fill("/api/v2/admin/subscription/groups/:id/protocols", id) }
	planGroups := func(id any) string { return fill("/api/v2/admin/subscription/plans/:plan_id/groups", id) }
	planGroup := func(plan, g any) string {
		return fill("/api/v2/admin/subscription/plans/:plan_id/groups/:group_id", plan, g)
	}
	userGroups := func(id any) string { return fill("/api/v2/admin/subscription/users/:user_id/groups", id) }
	userGroup := func(user, g any) string {
		return fill("/api/v2/admin/subscription/users/:user_id/groups/:group_id", user, g)
	}
	sg := func(w *World, i int) uint { return w.ID("sub_group", i) }
	// A fixed expiry a year after the data set's time.
	later := func(w *World) int64 { return w.Base.Add(Days(400)).Unix() }

	return []RouteSpec{
		{RouteID: "subscription.user.subscription.get", Reads: func(w *World) []Req {
			// refresh=true is left out: both sides rebuild the summary and
			// answer their own cached_at, which a shadow comparison would
			// count as a mismatch (subscriptioncompat masks data.cached_at).
			path := "/api/v2/user/subscription"
			return []Req{
				{Persona: User, Path: path, Label: "plan, traffic and expiry"},
				{Persona: User2, Path: path, Label: "another member"},
				{Persona: Fresh, Path: path, Label: "no plan, no transfer"},
				{Persona: Expired, Path: path, Label: "expired"},
				{Persona: Admin, Path: path, Label: "administrator without a plan"},
				{Persona: User, Path: path, Query: q("refresh", "1"), Label: "refresh=1 is not a refresh"},
				{Persona: User, Path: path, Query: q("refresh", "false"), Label: "refresh=false"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
		{RouteID: "subscription.admin.subscription.formats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/subscription/formats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "formats"},
				{Persona: Staff, Path: path, Query: q("format", "clash"), Label: "ignored query"},
			}, forbidden(path)...)
		}},
		{RouteID: "subscription.admin.subscription.protocols.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/subscription/protocols"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "protocol types"},
				{Persona: Admin2, Path: path, Label: "another administrator"},
			}, forbidden(path)...)
		}},
		{RouteID: "subscription.admin.subscription.groups.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/subscription/groups"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "by priority then id"},
				{Persona: Staff, Path: path, Query: q("page", "2"), Label: "ignored pagination"},
			}, forbidden(path)...)
		}},
		{RouteID: "subscription.admin.subscription.groups.id.get", Reads: func(w *World) []Req {
			return append([]Req{
				{Persona: Admin, Path: group(sg(w, sgDefault)), Label: "with templates and protocols"},
				{Persona: Admin, Path: group(sg(w, sgPremium)), Label: "with a description"},
				{Persona: Admin, Path: group(sg(w, sgRetired)), Label: "disabled"},
				{Persona: Admin, Path: group(sg(w, sgEmpty)), Label: "without templates"},
				{Persona: Staff, Path: group(sg(w, sgJapan)), Label: "staff"},
				{Persona: Admin, Path: group(Missing), Label: "not found"},
				{Persona: Admin, Path: group(0), Label: "zero"},
				{Persona: Admin, Path: group("x"), Label: "invalid id"},
				{Persona: Admin, Path: group("1%20OR%201=1"), Label: "a condition"},
				{Persona: Admin, Path: group("4294967297"), Label: "beyond 32 bits"},
			}, forbidden(group(sg(w, sgDefault)))...)
		}},
		// The protocol pool answers natively only once v2_node and
		// v2_node_protocol are finalized (node-ops-service.md section 4.3);
		// before that the host answers from the legacy handler and the
		// shadow comparison is skipped.
		{RouteID: "subscription.admin.subscription.groups.id.protocols.get", Reads: func(w *World) []Req {
			protocols := func(id any) string { return fmt.Sprintf("/api/v2/admin/subscription/groups/%v/protocols", id) }
			return append([]Req{
				{Persona: Admin, Path: protocols(sg(w, sgDefault)), Label: "linked protocols, secrets redacted"},
				{Persona: Admin, Path: protocols(sg(w, sgPremium)), Label: "several nodes"},
				{Persona: Admin, Path: protocols(sg(w, sgEmpty)), Label: "none"},
				{Persona: Admin, Path: protocols(Missing), Label: "unknown group"},
				{Persona: Admin, Path: protocols("x"), Label: "invalid id"},
			}, forbidden(protocols(sg(w, sgDefault)))...)
		}},
		{RouteID: "subscription.admin.subscription.protocols.available.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/subscription/protocols/available"
			return append([]Req{{Persona: Admin, Path: path, Label: "the pool with nodes and groups"}}, forbidden(path)...)
		}},
		{RouteID: "subscription.admin.subscription.groups.id.templates.get", Reads: func(w *World) []Req {
			return append([]Req{
				{Persona: Admin, Path: templates(sg(w, sgDefault)), Label: "by sort then id"},
				{Persona: Admin, Path: templates(sg(w, sgPremium)), Label: "settings and keys"},
				{Persona: Admin, Path: templates(sg(w, sgEmpty)), Label: "none"},
				{Persona: Admin, Path: templates(Missing), Label: "unknown group"},
				{Persona: Admin, Path: templates("x"), Label: "invalid id"},
			}, forbidden(templates(sg(w, sgDefault)))...)
		}},
		{RouteID: "subscription.admin.subscription.templates.id.get", Reads: func(w *World) []Req {
			reqs := []Req{}
			for i := 0; i < 6; i++ {
				reqs = append(reqs, Req{Persona: Admin, Path: template(w.ID("sub_template", i*2)), Label: "a template"})
			}
			return append(append(reqs,
				Req{Persona: Admin, Path: template(Missing), Label: "not found"},
				Req{Persona: Admin, Path: template("-1"), Label: "negative id"},
				Req{Persona: Admin, Path: template("x"), Label: "invalid id"},
			), forbidden(template(w.ID("sub_template", 0)))...)
		}},
		{RouteID: "subscription.admin.subscription.plans.plan_id.groups.get", Reads: func(w *World) []Req {
			return append([]Req{
				{Persona: Admin, Path: planGroups(w.ID("plan", 1)), Label: "two groups"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 0)), Label: "one group"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 4)), Label: "none (hidden plan)"},
				{Persona: Staff, Path: planGroups(w.ID("plan", 5)), Label: "staff"},
				{Persona: Admin, Path: planGroups(Missing), Label: "unknown plan"},
				{Persona: Admin, Path: planGroups(0), Label: "plan zero"},
				{Persona: Admin, Path: planGroups("x"), Label: "invalid id"},
			}, forbidden(planGroups(w.ID("plan", 1)))...)
		}},
		{RouteID: "subscription.admin.subscription.users.user_id.groups.get", Reads: func(w *World) []Req {
			return append([]Req{
				{Persona: Admin, Path: userGroups(w.PersonaID(User)), Label: "unexpired and without expiry, an expired one left out"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Label: "an expiring group with a renewal price"},
				{Persona: Admin, Path: userGroups(w.PersonaID(Banned)), Label: "banned member, expired membership"},
				{Persona: Admin, Path: userGroups(w.PersonaID(Expired)), Label: "expired member"},
				{Persona: Admin, Path: userGroups(w.ID("user.member", 22)), Label: "a disabled group is listed"},
				{Persona: Staff, Path: userGroups(w.ID("user.member", 5)), Label: "another member"},
				{Persona: Admin, Path: userGroups(w.PersonaID(Fresh)), Label: "none"},
				{Persona: Admin, Path: userGroups(Missing), Label: "unknown user"},
				{Persona: Admin, Path: userGroups("x"), Label: "invalid id"},
			}, forbidden(userGroups(w.PersonaID(User)))...)
		}},
		{RouteID: "subscription.admin.subscription.stats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/subscription/stats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "members, traffic, templates, protocols, online nodes and plans"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},

		{RouteID: "subscription.admin.subscription.groups.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/subscription/groups"
			return []Req{
				{Persona: Admin, Path: path, Body: `{"name":"Staging 新分组","description":"d","priority":3,"enable":1}`, Mask: stamped, Label: "a group"},
				{Persona: Admin, Path: path, Body: `{"name":"Staging zero enable","enable":0}`, Mask: stamped, Label: "zero enable takes the default"},
				{Persona: Admin, Path: path, Body: fmt.Sprintf(`{"id":%d,"name":"Staging explicit id","created_at":"2020-01-02T03:04:05Z","updated_at":"2020-01-02T03:04:05Z"}`, Missing+10),
					Mask: updated, Label: "an explicit id and times"},
				{Persona: Staff, Path: path, Mask: stamped, Label: "nested associations are dropped",
					Body: `{"name":"Staging nested","protocols":[{"id":1},{"name":"x","node":{"name":"n"}}],"templates":[{"id":1},{"name":"t"}]}`},
				{Persona: Admin, Path: path, Body: `{"name":"默认 Default"}`, Label: "a duplicate name"},
				{Persona: Admin, Path: path, Body: `{"name":1}`, Label: "a name of the wrong type"},
				{Persona: Admin, Path: path, Body: `{"name":"x","templates":"x"}`, Label: "templates of the wrong type"},
				{Persona: Admin, Path: path, Body: `{"name":"x","templates":[{"created_at":"x"}]}`, Label: "a nested time that does not parse"},
				{Persona: Admin, Path: path, Body: `{"name":`, Label: "invalid JSON"},
				{Persona: Admin, Path: path, Label: "no body"},
				{Persona: Admin, Path: path, Body: `[1]`, Label: "a body that is not an object"},
				{Persona: User, Path: path, Body: `{"name":"member"}`, Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.groups.id.protocols.post", Writes: func(w *World) []Req {
			p := func(i int) uint { return w.ID("node_protocol", i) }
			return []Req{
				{Persona: Admin, Path: protocols(sg(w, sgPremium)), Body: anyMap("protocol_ids", []uint{p(1), p(3), p(5)}), Label: "replace some"},
				{Persona: Admin, Path: protocols(sg(w, sgDefault)), Body: anyMap("protocol_ids", []uint{p(3), p(3), p(2)}), Label: "duplicates"},
				{Persona: Admin, Path: protocols(sg(w, sgDefault)), Body: anyMap("protocol_ids", []uint{p(3), p(2)}), Label: "the same set"},
				{Persona: Admin, Path: protocols(sg(w, sgStreaming)), Body: `{"protocol_ids":[]}`, Label: "clear"},
				{Persona: Admin, Path: protocols(sg(w, sgEmpty)), Body: anyMap("protocol_ids", []uint{p(4)}), Label: "a group without links"},
				{Persona: Admin, Path: protocols(sg(w, sgJapan)), Body: anyMap("protocol_ids", []uint{p(1), Missing}), Label: "an unknown protocol"},
				{Persona: Admin, Path: protocols(Missing), Body: anyMap("protocol_ids", []uint{p(1)}), Label: "an unknown group"},
				{Persona: Admin, Path: protocols(0), Body: anyMap("protocol_ids", []uint{p(1)}), Label: "group zero"},
				{Persona: Admin, Path: protocols(sg(w, sgJapan)), Body: `{}`, Label: "no list"},
				{Persona: Admin, Path: protocols(sg(w, sgJapan)), Body: `{"protocol_ids":null}`, Label: "a null list"},
				{Persona: Admin, Path: protocols(sg(w, sgJapan)), Body: `{"protocol_ids":[-1]}`, Label: "a negative id"},
				{Persona: Admin, Path: protocols(sg(w, sgJapan)), Body: `{"protocol_ids":[1.5]}`, Label: "a fractional id"},
				{Persona: Admin, Path: protocols(sg(w, sgJapan)), Body: `{"protocol_ids":[18446744073709551615]}`, Label: "an id the database refuses"},
				{Persona: Admin, Path: protocols("x"), Body: anyMap("protocol_ids", []uint{p(1)}), Label: "invalid id"},
				{Persona: Admin, Path: protocols(sg(w, sgJapan)), Label: "no body"},
				{Persona: User, Path: protocols(sg(w, sgJapan)), Body: anyMap("protocol_ids", []uint{p(1)}), Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.groups.id.templates.post", Writes: func(w *World) []Req {
			full := `{"name":"TW trojan","type":"trojan","server":"tw.sub.example.com","port":8443,"server_name":"tw.sub.example.com","tls":1,` +
				`"tls_fingerprint":"chrome","alpn":"h2,http/1.1","transport":"ws","transport_settings":"{\"path\":\"/ws\"}","enable":1,"sort":3,` +
				`"protocol_settings":"{\"flow\":\"\"}","template_json":"{\"name\":\"{{.Email}}\"}","tags":"[\"tw\"]"}`
			return []Req{
				{Persona: Admin, Path: templates(sg(w, sgDefault)), Body: full, Mask: stamped, Label: "every field"},
				{Persona: Admin, Path: templates(sg(w, sgPremium)), Body: `{"name":"min"}`, Mask: stamped, Label: "defaults"},
				{Persona: Staff, Path: templates(sg(w, sgEmpty)), Body: fmt.Sprintf(`{"name":"path wins","group_id":%d}`, sg(w, sgDefault)), Mask: stamped,
					Label: "the path group wins over the body"},
				{Persona: Admin, Path: templates(sg(w, sgPremium)), Mask: stamped, Label: "a nested group is dropped",
					Body: `{"name":"nested","group":{"id":1,"name":"default","protocols":[{"name":"p","node":{"name":"n"}}]}}`},
				{Persona: Admin, Path: templates(Missing), Label: "an unknown group (foreign key)",
					Body: `{"name":"orphan","created_at":"2020-01-02T03:04:05Z","updated_at":"2020-01-02T03:04:05Z"}`},
				{Persona: Admin, Path: templates(sg(w, sgDefault)), Body: `{"port":"443"}`, Label: "a port of the wrong type"},
				{Persona: Admin, Path: templates(sg(w, sgDefault)), Body: `{"group":"default"}`, Label: "a nested group of the wrong type"},
				{Persona: Admin, Path: templates("x"), Body: full, Label: "invalid id"},
				{Persona: Admin, Path: templates(sg(w, sgDefault)), Label: "no body"},
				{Persona: User, Path: templates(sg(w, sgDefault)), Body: `{"name":"m"}`, Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.plans.plan_id.groups.post", Writes: func(w *World) []Req {
			body := func(g any) map[string]any { return anyMap("group_id", g) }
			return []Req{
				{Persona: Admin, Path: planGroups(w.ID("plan", 4)), Body: body(sg(w, sgPremium)), Mask: stamped, Label: "a new link"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 7)), Body: body(sg(w, sgEmpty)), Mask: stamped, Label: "a new link to an empty group"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 1)), Body: body(sg(w, sgPremium)), Mask: stamped, Label: "an existing link is kept once"},
				{Persona: Admin, Path: planGroups(Missing), Body: body(sg(w, sgPremium)), Label: "unknown plan"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 0)), Body: body(Missing), Label: "unknown group"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 0)), Body: `{}`, Label: "no group"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 0)), Body: `{"group_id":"2"}`, Label: "a group of the wrong type"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 0)), Body: `{"group_id":18446744073709551615}`, Label: "a group id the database refuses"},
				{Persona: Admin, Path: planGroups("x"), Body: body(sg(w, sgPremium)), Label: "invalid id"},
				{Persona: Admin, Path: planGroups(w.ID("plan", 0)), Label: "no body"},
				{Persona: User, Path: planGroups(w.ID("plan", 0)), Body: body(sg(w, sgPremium)), Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.users.user_id.groups.post", Writes: func(w *World) []Req {
			// Without an Idempotency-Key or X-Request-ID every request is a
			// grant of its own, with a random ledger id on each twin.
			return []Req{
				{Persona: Admin, Path: userGroups(w.PersonaID(Fresh)), Mask: stamped, Label: "a new group with every field",
					Body: fmt.Sprintf(`{"group_id":%d,"expire_at":%d,"transfer_enable":1073741824,"next_renew_price":1500}`, sg(w, sgPremium), later(w))},
				{Persona: Admin, Path: userGroups(w.ID("user.fresh", 1)), Body: anyMap("group_id", sg(w, sgEmpty)), Mask: stamped, Label: "a new group without fields"},
				{Persona: Admin, Path: userGroups(w.ID("user.fresh", 1)), Body: anyMap("group_id", sg(w, sgRetired)), Mask: stamped, Label: "a disabled group"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: anyMap("group_id", sg(w, sgPremium), "next_renew_price", 500), Mask: stamped,
					Label: "an existing group: only the given fields are set"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User)), Body: anyMap("group_id", sg(w, sgStreaming), "expire_at", later(w)), Mask: stamped,
					Label: "an existing expired group: a new expiry"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: `{"group_id":` + fmt.Sprint(sg(w, sgPremium)) + `,"expire_at":null,"transfer_enable":null}`,
					Mask: stamped, Label: "an existing group: null fields are kept"},
				{Persona: Admin, Path: userGroups(w.PersonaID(Banned)), Body: anyMap("group_id", sg(w, sgPremium), "expire_at", later(w)), Mask: stamped,
					Label: "an expired membership of a banned member"},
				{Persona: Staff, Path: userGroups(w.PersonaID(Expired)), Body: anyMap("group_id", sg(w, sgJapan)), Mask: stamped, Label: "an expired member"},
				{Persona: Admin, Path: userGroups(w.ID("user.member", 30)), Body: anyMap("group_id", sg(w, sgJapan), "expire_at", -1, "transfer_enable", -1, "next_renew_price", -5),
					Mask: stamped, Label: "negative values are stored as given"},
				{Persona: Admin, Path: userGroups(Missing), Body: anyMap("group_id", sg(w, sgDefault)), Label: "unknown user"},
				{Persona: Admin, Path: userGroups(0), Body: anyMap("group_id", sg(w, sgDefault)), Label: "user zero"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: anyMap("group_id", Missing), Label: "unknown group"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: `{"group_id":4294967297}`, Label: "a group beyond 32 bits"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: `{"group_id":18446744073709551615}`, Label: "a group id the database refuses"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: `{}`, Label: "no group"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: `{"group_id":0}`, Label: "group zero"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: `{"group_id":"2"}`, Label: "a group of the wrong type"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: fmt.Sprintf(`{"group_id":%d,"expire_at":"soon"}`, sg(w, sgPremium)), Label: "an expiry of the wrong type"},
				{Persona: Admin, Path: userGroups("x"), Body: anyMap("group_id", sg(w, sgDefault)), Label: "invalid id"},
				{Persona: Admin, Path: userGroups("4294967297"), Body: anyMap("group_id", sg(w, sgDefault)), Label: "user beyond 32 bits"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Body: `{"group_id":`, Label: "invalid JSON"},
				{Persona: Admin, Path: userGroups(w.PersonaID(User2)), Label: "no body"},
				{Persona: User, Path: userGroups(w.PersonaID(User)), Body: anyMap("group_id", sg(w, sgPremium)), Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.groups.id.put", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: group(sg(w, sgPremium)), Body: `{"name":"高级 Premium+","description":null,"priority":11,"enable":1}`, Mask: updated, Label: "every column"},
				{Persona: Admin, Path: group(sg(w, sgStreaming)), Body: `{"name":"流媒体 Streaming"}`, Mask: updated, Label: "missing fields become zero"},
				{Persona: Staff, Path: group(sg(w, sgJapan)), Body: fmt.Sprintf(`{"id":%d,"name":"日本 Japan renamed"}`, sg(w, sgDefault)), Mask: updated, Label: "the path id wins"},
				{Persona: Admin, Path: group(Missing + 20), Body: `{"name":"Staging created by PUT","enable":1}`, Mask: stamped, Label: "an unknown id creates the group"},
				{Persona: Admin, Path: group(sg(w, sgPremium)), Mask: updated, Label: "nested associations are dropped",
					Body: `{"name":"高级 Premium+","protocols":[{"id":3}],"templates":[{"id":1,"name":"moved"}]}`},
				{Persona: Admin, Path: group(sg(w, sgPremium)), Body: `{"name":"默认 Default"}`, Label: "a duplicate name"},
				{Persona: Admin, Path: group(sg(w, sgPremium)), Body: `{"priority":"high"}`, Label: "a priority of the wrong type"},
				{Persona: Admin, Path: group("x"), Body: `{"name":"x"}`, Label: "invalid id"},
				{Persona: Admin, Path: group("x"), Body: `{"name":`, Label: "an invalid body before the id"},
				{Persona: Admin, Path: group(sg(w, sgDefault)), Label: "no body"},
				{Persona: User, Path: group(sg(w, sgDefault)), Body: `{"name":"m"}`, Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.templates.id.put", Writes: func(w *World) []Req {
			t := func(i int) string { return template(w.ID("sub_template", i)) }
			return []Req{
				{Persona: Admin, Path: t(3), Body: `{"name":"US hysteria2 v2","port":8443,"tls":true,"enable":false}`, Mask: updated, Label: "some fields"},
				{Persona: Admin, Path: t(0), Body: `{"reality_public_key":null}`, Mask: updated, Label: "a pointer field to null"},
				{Persona: Admin, Path: t(4), Body: anyMap("group_id", sg(w, sgDefault)), Mask: updated, Label: "move to another group"},
				{Persona: Admin, Path: t(5), Body: `{"sort":2.7}`, Mask: updated, Label: "a fractional sort"},
				{Persona: Staff, Path: t(6), Mask: updated, Label: "the id and times are kept",
					Body: `{"name":"x","id":9,"ID":9,"Id":9,"created_at":"2001-01-01T00:00:00Z","CreatedAt":"2001-01-01T00:00:00Z","UPDATED_AT":"2001-01-01T00:00:00Z"}`},
				{Persona: Admin, Path: t(7), Body: `{"id":9}`, Mask: updated, Label: "only protected keys"},
				{Persona: Admin, Path: t(1), Body: `{"Name":"by field name"}`, Mask: updated, Label: "a field name"},
				{Persona: Admin, Path: t(1), Body: `{"bogus":1}`, Label: "an unknown column"},
				{Persona: Admin, Path: t(1), Body: `{"Group":{"name":"x"}}`, Label: "the group relation"},
				{Persona: Admin, Path: t(1), Body: `{}`, Label: "an empty object"},
				{Persona: Admin, Path: t(1), Body: `null`, Label: "null"},
				{Persona: Admin, Path: template(Missing), Body: `{"name":"x"}`, Label: "not found"},
				{Persona: Admin, Path: template("x"), Body: `{"name":"x"}`, Label: "invalid id"},
				{Persona: Admin, Path: t(1), Body: `[1]`, Label: "a body that is not an object"},
				{Persona: Admin, Path: t(1), Label: "no body"},
				{Persona: User, Path: t(1), Body: `{"name":"m"}`, Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.groups.id.delete", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: group(sg(w, sgScratch)), Label: "members, templates, plan and protocol links"},
				{Persona: Admin, Path: group(sg(w, sgScratch)), Label: "again: the group is gone"},
				{Persona: Admin, Path: group(sg(w, sgEmpty)), Label: "a group without members or links"},
				{Persona: Admin, Path: group(Missing), Label: "not found"},
				{Persona: Admin, Path: group(0), Label: "zero"},
				{Persona: Admin, Path: group("x"), Label: "invalid id"},
				{Persona: Admin, Path: group("1%20OR%201=1"), Label: "a condition"},
				{Persona: Admin, Path: group("4294967297"), Label: "beyond 32 bits"},
				{Persona: User, Path: group(sg(w, sgRetired)), Label: "member on admin route"},
				{Persona: Anon, Path: group(sg(w, sgRetired)), Label: "anonymous"},
			}
		}},
		{RouteID: "subscription.admin.subscription.plans.plan_id.groups.group_id.delete", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: planGroup(w.ID("plan", 1), sg(w, sgDefault)), Label: "a link"},
				{Persona: Admin, Path: planGroup(w.ID("plan", 1), sg(w, sgDefault)), Label: "again: no such link"},
				{Persona: Admin, Path: planGroup(w.ID("plan", 0), sg(w, sgPremium)), Label: "no such link"},
				{Persona: Admin, Path: planGroup(Missing, sg(w, sgDefault)), Label: "unknown plan"},
				{Persona: Admin, Path: planGroup(w.ID("plan", 0), Missing), Label: "unknown group"},
				{Persona: Admin, Path: planGroup("x", sg(w, sgDefault)), Label: "plan not a number"},
				{Persona: Admin, Path: planGroup(w.ID("plan", 0), "x"), Label: "group not a number"},
				{Persona: User, Path: planGroup(w.ID("plan", 0), sg(w, sgDefault)), Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.templates.id.delete", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: template(w.ID("sub_template", 2)), Label: "a template"},
				{Persona: Admin, Path: template(w.ID("sub_template", 2)), Label: "again: not found"},
				{Persona: Admin, Path: template(w.ID("sub_template.scratch", 0)), Label: "a template of a deleted group"},
				{Persona: Admin, Path: template(Missing), Label: "not found"},
				{Persona: Admin, Path: template(0), Label: "zero"},
				{Persona: Admin, Path: template("x"), Label: "invalid id"},
				{Persona: User, Path: template(w.ID("sub_template", 1)), Label: "member on admin route"},
			}
		}},
		{RouteID: "subscription.admin.subscription.users.user_id.groups.group_id.delete", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: userGroup(w.PersonaID(User), sg(w, sgDefault)), Label: "a group without expiry"},
				{Persona: Admin, Path: userGroup(w.PersonaID(User2), sg(w, sgPremium)), Label: "an expiring group"},
				{Persona: Admin, Path: userGroup(w.PersonaID(Banned), sg(w, sgPremium)), Label: "an expired membership of a banned member"},
				{Persona: Staff, Path: userGroup(w.PersonaID(Expired), sg(w, sgDefault)), Label: "an expired member"},
				{Persona: Admin, Path: userGroup(w.ID("user.member", 22), sg(w, sgRetired)), Label: "a disabled group"},
				{Persona: Admin, Path: userGroup(w.PersonaID(User), sg(w, sgDefault)), Label: "again: no such membership"},
				{Persona: Admin, Path: userGroup(w.ID("user.fresh", 4), sg(w, sgDefault)), Label: "no such membership"},
				{Persona: Admin, Path: userGroup(Missing, sg(w, sgDefault)), Label: "unknown user"},
				{Persona: Admin, Path: userGroup(w.PersonaID(User2), Missing), Label: "unknown group"},
				{Persona: Admin, Path: userGroup(0, sg(w, sgDefault)), Label: "user zero"},
				{Persona: Admin, Path: userGroup(w.PersonaID(User2), 0), Label: "group zero"},
				{Persona: Admin, Path: userGroup("x", sg(w, sgDefault)), Label: "user not a number"},
				{Persona: Admin, Path: userGroup(w.PersonaID(User2), "x"), Label: "group not a number"},
				{Persona: Admin, Path: userGroup(w.PersonaID(User2), "4294967297"), Label: "group beyond 32 bits"},
				{Persona: User, Path: userGroup(w.PersonaID(User), sg(w, sgPremium)), Label: "member on admin route"},
			}
		}},
	}
}

// ---------------------------------------------------------------------------
// Forward (Flux clone: POST routes with JSON bodies, many of them reads).

func forwardSpecs() []RouteSpec {
	created := []string{"data.createdTime", "data.updatedTime"}
	listTimes := []string{"data.*.createdTime", "data.*.updatedTime"}
	tn := func(w *World, i int) uint { return w.ID("tunnel", i) }
	fn := func(w *World, i int) uint { return w.ID("fwd_node", i) }
	sl := func(w *World, i int) uint { return w.ID("speed_limit", i) }

	forwardList := func(admin bool) func(w *World) []Req {
		return func(w *World) []Req {
			path := "/api/v2/forward/list"
			if admin {
				path = "/api/v2/admin/forward/list"
				return append([]Req{
					flux(Admin, path, nil, "every forward", listTimes...),
					flux(Admin2, path, map[string]any{}, "every forward, empty body", listTimes...),
					flux(Staff, path, map[string]any{"page": 2}, "staff, ignored body", listTimes...),
				}, fluxForbidden(path, nil)...)
			}
			return []Req{
				flux(User, path, nil, "own forwards in order", listTimes...),
				flux(User2, path, map[string]any{}, "own forwards", listTimes...),
				flux(Fresh, path, nil, "none", listTimes...),
				flux(Expired, path, nil, "a paused forward", listTimes...),
				flux(Admin, path, nil, "an administrator sees every forward", listTimes...),
				flux(Anon, path, nil, "anonymous"),
			}
		}
	}
	order := func(admin bool) func(w *World) []Req {
		return func(w *World) []Req {
			f := func(key string, i int) uint { return w.ID(key, i) }
			entry := func(id uint, inx int) map[string]any { return anyMap("id", id, "inx", inx) }
			if admin {
				path := "/api/v2/admin/forward/update-order"
				return append([]Req{
					flux(Admin, path, anyMap("forwards", []any{entry(f("forward.user", 0), 5), entry(f("forward.user2", 0), 1)}), "anyone's forwards"),
					flux(Staff, path, anyMap("forwards", []any{entry(f("forward.other", 2), 4)}), "staff"),
					flux(Admin, path, anyMap("forwards", []any{entry(Missing, 1)}), "an unknown forward"),
					flux(Admin, path, `{"forwards":[]}`, "an empty list"),
					flux(Admin, path, `{}`, "no list"),
					flux(Admin, path, `{"forwards":{"id":1}}`, "a list that is not a list"),
					flux(Admin, path, nil, "no body"),
					flux(Admin, path, `{"forwards":`, "invalid JSON"),
				}, fluxForbidden(path, anyMap("forwards", []any{entry(f("forward.user", 0), 1)}))...)
			}
			path := "/api/v2/forward/update-order"
			return []Req{
				flux(User, path, anyMap("forwards", []any{entry(f("forward.user", 0), 3), entry(f("forward.user", 1), -1)}), "own forwards"),
				flux(User, path, anyMap("forwards", []any{entry(f("forward.user", 2), 2), entry(f("forward.user2", 1), 1)}), "another member's forward"),
				flux(User2, path, anyMap("forwards", []any{entry(f("forward.user2", 1), 9)}), "own forward"),
				flux(Admin, path, anyMap("forwards", []any{entry(f("forward.other", 0), 7), entry(f("forward.user", 2), 6)}), "an administrator orders anyone's"),
				flux(User, path, anyMap("forwards", []any{entry(f("forward.user", 0), 1), entry(f("forward.user", 0), 2)}), "a forward twice"),
				flux(User, path, anyMap("forwards", []any{entry(Missing, 1)}), "an unknown forward"),
				flux(Fresh, path, anyMap("forwards", []any{entry(f("forward.user", 0), 1)}), "a member without forwards"),
				flux(User, path, `{"forwards":[]}`, "an empty list"),
				flux(User, path, `{}`, "no list"),
				flux(User, path, `{"forwards":{"id":1}}`, "a list that is not a list"),
				flux(User, path, nil, "no body"),
				flux(User, path, `{"forwards":`, "invalid JSON"),
				flux(Anon, path, anyMap("forwards", []any{entry(f("forward.user", 0), 1)}), "anonymous"),
			}
		}
	}
	tunnelsFor := func(admin bool) func(w *World) []Req {
		return func(w *World) []Req {
			if admin {
				path := "/api/v2/admin/tunnel/user/tunnel"
				return append([]Req{
					flux(Admin, path, nil, "every active tunnel the backend can run"),
					flux(Staff, path, map[string]any{}, "staff"),
				}, fluxForbidden(path, nil)...)
			}
			path := "/api/v2/tunnel/user/tunnel"
			return []Req{
				flux(User, path, nil, "own tunnels (a disabled one left out)"),
				flux(User2, path, map[string]any{}, "own tunnels"),
				flux(Fresh, path, nil, "none (or the replay's new permission)"),
				flux(Expired, path, nil, "a disabled permission"),
				flux(Admin, path, nil, "an administrator"),
				flux(Anon, path, nil, "anonymous"),
			}
		}
	}
	assign := func(admin bool) func(w *World) []Req {
		return func(w *World) []Req {
			path, fresh, member, other := "/api/v2/tunnel/user/assign", w.ID("user.fresh", 2), w.ID("user.member", 32), w.ID("user.fresh", 3)
			if admin {
				path, fresh, member, other = "/api/v2/admin/tunnel/user/assign", w.PersonaID(Fresh), w.ID("user.member", 30), w.ID("user.fresh", 1)
			}
			body := func(user, tunnel any, extra ...any) map[string]any {
				return anyMap(append([]any{"userId", user, "tunnelId", tunnel}, extra...)...)
			}
			return append([]Req{
				flux(Admin, path, body(fresh, tn(w, tnHKJP), "flow", 100, "num", 3, "flowResetTime", 0, "expTime", b4Expiry, "speedId", sl(w, slFast)), "a permission"),
				flux(Admin, path, body(other, tn(w, tnUS), "speedId", 0), "without a speed limit"),
				flux(Staff, path, body(member, tn(w, tnDisabled)), "on a disabled tunnel"),
				flux(Admin, path, body(w.PersonaID(User), tn(w, tnHKJP)), "twice"),
				flux(Admin, path, body(w.PersonaID(Fresh), tn(w, tnHKJP)), "the fresh member's permission (exists after the admin route)"),
				flux(Admin, path, body(Missing, tn(w, tnHKJP)), "an unknown user"),
				flux(Admin, path, body(member, Missing), "an unknown tunnel"),
				flux(Admin, path, body(member, tn(w, tnHKJP), "speedId", Missing), "an unknown speed limit"),
				flux(Admin, path, body(member, tn(w, tnHKJP), "speedId", sl(w, slSlow)), "another tunnel's speed limit"),
				flux(Admin, path, anyMap("tunnelId", tn(w, tnHKJP)), "no user"),
				flux(Admin, path, anyMap("userId", member), "no tunnel"),
				flux(Admin, path, fmt.Sprintf(`{"userId":"%d","tunnelId":%d}`, member, tn(w, tnHKJP)), "a user id that is a string"),
				flux(Admin, path, nil, "no body"),
			}, fluxForbidden(path, body(w.PersonaID(User), tn(w, tnSG)))...)
		}
	}
	userTunnels := func(admin bool) func(w *World) []Req {
		return func(w *World) []Req {
			path := "/api/v2/tunnel/user/list"
			if admin {
				path = "/api/v2/admin/tunnel/user/list"
			}
			return append([]Req{
				flux(Admin, path, anyMap("userId", w.PersonaID(User)), "the user's permissions (traffic catches up)"),
				flux(Admin, path, anyMap("userId", w.PersonaID(User2)), "user2's"),
				flux(Staff, path, anyMap("userId", w.ID("user.member", 4)), "a member's"),
				flux(Admin, path, anyMap("userId", w.PersonaID(Fresh)), "the fresh member (a new permission)"),
				flux(Admin, path, anyMap("userId", w.ID("user.fresh", 4)), "a member without permissions"),
				flux(Admin, path, anyMap("userId", Missing), "a user without a row"),
				flux(Admin, path, `{}`, "no user"),
				flux(Admin, path, fmt.Sprintf(`{"userId":"%d"}`, w.PersonaID(User)), "a user id that is a string"),
				flux(Admin, path, nil, "no body"),
			}, fluxForbidden(path, anyMap("userId", w.PersonaID(User)))...)
		}
	}

	return []RouteSpec{
		{RouteID: "forward.admin.forward.list.post", Writes: forwardList(true)},
		{RouteID: "forward.forward.list.post", Writes: forwardList(false)},
		{RouteID: "forward.admin.forward.update_order.post", Writes: order(true)},
		{RouteID: "forward.forward.update_order.post", Writes: order(false)},
		{RouteID: "forward.admin.tunnel.user.tunnel.post", Writes: tunnelsFor(true)},
		{RouteID: "forward.tunnel.user.tunnel.post", Writes: tunnelsFor(false)},
		{RouteID: "forward.admin.tunnel.user.assign.post", Writes: assign(true)},
		{RouteID: "forward.tunnel.user.assign.post", Writes: assign(false)},
		{RouteID: "forward.admin.tunnel.user.list.post", Writes: userTunnels(true)},
		{RouteID: "forward.tunnel.user.list.post", Writes: userTunnels(false)},
		{RouteID: "forward.admin.tunnel.list.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/tunnel/list"
			// Runs after the tunnel creations: their times are the clock's.
			return append([]Req{
				flux(Admin, path, nil, "every tunnel by name", listTimes...),
				flux(Staff, path, map[string]any{}, "staff", listTimes...),
			}, fluxForbidden(path, nil)...)
		}},
		{RouteID: "forward.admin.tunnel.create.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/tunnel/create"
			ok := func(label string, body map[string]any) Req { return flux(Admin, path, body, label, created...) }
			no := func(label string, body any) Req { return flux(Admin, path, body, label) }
			return append([]Req{
				ok("port forwarding", anyMap("name", " Staging 新建端口 ", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 1, "interfaceName", " eth9 ", "tcpListenAddr", " 0.0.0.0 ")),
				ok("tunnel forwarding", anyMap("name", "Staging 新建隧道", "inNodeId", fn(w, fnRelay1), "outNodeId", fn(w, fnExit0), "type", 2, "flow", 2, "trafficRatio", 2.5)),
				ok("tunnel forwarding with a protocol, relay type in another case", anyMap("name", "Staging mTLS", "inNodeId", fn(w, fnRelay3), "outNodeId", fn(w, fnExit2), "type", 2, "flow", 2, "protocol", " mtls ")),
				ok("a ratio of zero is one", anyMap("name", "Staging ratio zero", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 2, "trafficRatio", 0)),
				ok("the largest ratio", anyMap("name", "Staging ratio max", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 2, "trafficRatio", 100)),
				no("a ratio too large", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 2, "trafficRatio", 100.5)),
				no("a negative ratio", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 2, "trafficRatio", -1)),
				no("the same node twice", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "outNodeId", fn(w, fnRelay0), "type", 2, "flow", 2)),
				no("no exit node", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "type", 2, "flow", 2)),
				no("an exit node that is a relay", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "outNodeId", fn(w, fnRelay1), "type", 2, "flow", 2)),
				no("an entry node that is an exit", anyMap("name", "omega", "inNodeId", fn(w, fnExit0), "type", 1, "flow", 2)),
				no("a disabled entry node", anyMap("name", "omega", "inNodeId", fn(w, fnRelay2), "type", 1, "flow", 2)),
				no("an unknown entry node", anyMap("name", "omega", "inNodeId", Missing, "type", 1, "flow", 2)),
				no("an unknown exit node", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "outNodeId", Missing, "type", 2, "flow", 2)),
				no("no entry node", anyMap("name", "omega", "type", 1, "flow", 2)),
				no("a name in use", anyMap("name", " "+b4Tunnels[tnHKJP]+" ", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 2)),
				no("no name", anyMap("name", "  ", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 2)),
				no("an unknown type", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "type", 3, "flow", 2)),
				no("an unknown flow", anyMap("name", "omega", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 0)),
				no("a type that is a string", fmt.Sprintf(`{"name":"omega","inNodeId":%d,"type":"1","flow":2}`, fn(w, fnRelay0))),
				no("no body", nil),
				no("invalid JSON", `{"name":`),
			}, fluxForbidden(path, anyMap("name", "member", "inNodeId", fn(w, fnRelay0), "type", 1, "flow", 2))...)
		}},
		{RouteID: "forward.admin.tunnel.delete.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/tunnel/delete"
			id := func(v any) map[string]any { return anyMap("id", v) }
			return append([]Req{
				flux(Admin, path, id(tn(w, tnUnusedA)), "an unused tunnel"),
				flux(Admin, path, id(tn(w, tnUnusedA)), "again: not found"),
				flux(Admin, path, id(tn(w, tnHKJP)), "a tunnel with forwards"),
				flux(Admin, path, id(tn(w, tnPermOnly)), "a tunnel with a permission only"),
				flux(Staff, path, id(tn(w, tnDisabled)), "a disabled tunnel with a permission"),
				flux(Admin, path, id(Missing), "an unknown tunnel"),
				flux(Admin, path, id(0), "tunnel zero"),
				flux(Admin, path, fmt.Sprintf(`{"id":"%d"}`, tn(w, tnUnusedB)), "an id that is a string"),
				flux(Admin, path, `{"id":-5}`, "a negative id"),
				flux(Admin, path, nil, "no body"),
			}, fluxForbidden(path, id(tn(w, tnUnusedB)))...)
		}},
		{RouteID: "forward.speed_limit.create.post", Writes: func(w *World) []Req {
			path := "/api/v2/speed-limit/create"
			body := func(name string, speed any, tunnel any, tunnelName string) map[string]any {
				return anyMap("name", name, "speed", speed, "tunnelId", tunnel, "tunnelName", tunnelName)
			}
			return append([]Req{
				flux(Admin, path, body(" turbo 涡轮 ", 90000, tn(w, tnHKJP), " "+b4Tunnels[tnHKJP]+" "), "a limit"),
				flux(Admin, path, body("disabled tunnel", 1, tn(w, tnDisabled), b4Tunnels[tnDisabled]), "a limit on a disabled tunnel"),
				flux(Staff, path, body("fast 极速 50M", 51200, tn(w, tnHKJP), b4Tunnels[tnHKJP]), "a second limit with a name in use"),
				flux(Admin, path, body("unused tunnel", 2048, tn(w, tnUnusedB), b4Tunnels[tnUnusedB]), "a limit on an unused tunnel"),
				flux(Admin, path, anyMap("name", "x", "speed", 2, "tunnelId", tn(w, tnUS), "tunnelName", b4Tunnels[tnUS], "status", 0, "id", 9), "unknown fields are ignored"),
				flux(Admin, path, body("x", 2, tn(w, tnHKJP), b4Tunnels[tnUS]), "a tunnel name that does not match"),
				flux(Admin, path, body("x", 2, tn(w, tnHKJP), strings.ToUpper(b4Tunnels[tnHKJP])), "a tunnel name in another case"),
				flux(Admin, path, body("x", 2, Missing, b4Tunnels[tnHKJP]), "an unknown tunnel"),
				flux(Admin, path, anyMap("name", "x", "speed", 2, "tunnelName", b4Tunnels[tnHKJP]), "no tunnel id"),
				flux(Admin, path, body("x", 2, tn(w, tnHKJP), "  "), "a blank tunnel name"),
				flux(Admin, path, body(" ", 2, tn(w, tnHKJP), b4Tunnels[tnHKJP]), "a blank name"),
				flux(Admin, path, body("x", 0, tn(w, tnHKJP), b4Tunnels[tnHKJP]), "a zero speed"),
				flux(Admin, path, body("x", -1, tn(w, tnHKJP), b4Tunnels[tnHKJP]), "a negative speed"),
				flux(Admin, path, `{"name":"","speed":0,"tunnelId":0,"tunnelName":""}`, "the tunnel is checked first"),
				flux(Admin, path, fmt.Sprintf(`{"name":"x","speed":"2","tunnelId":%d,"tunnelName":"x"}`, tn(w, tnHKJP)), "a speed that is a string"),
				flux(Admin, path, `{"name":"x","speed":2,"tunnelId":-1,"tunnelName":"x"}`, "a negative tunnel id"),
				flux(Admin, path, `[1]`, "a body that is not an object"),
				flux(Admin, path, `{"name":`, "invalid JSON"),
				flux(Admin, path, nil, "no body"),
			}, fluxForbidden(path, body("member", 2, tn(w, tnHKJP), b4Tunnels[tnHKJP]))...)
		}},
		{RouteID: "forward.speed_limit.delete.post", Writes: func(w *World) []Req {
			path := "/api/v2/speed-limit/delete"
			id := func(v any) map[string]any { return anyMap("id", v) }
			return append([]Req{
				flux(Admin, path, id(sl(w, slIdle)), "an unused limit"),
				flux(Admin, path, id(sl(w, slOff)), "an unused inactive limit"),
				flux(Admin, path, id(sl(w, slFast)), "a limit the user's permission names"),
				flux(Staff, path, id(sl(w, slSlow)), "a limit user2's permission names"),
				flux(Admin, path, id(Missing), "an unknown limit"),
				flux(Admin, path, id(sl(w, slIdle)), "deleting twice"),
				flux(Admin, path, id(0), "limit zero"),
				flux(Admin, path, `{}`, "no id"),
				flux(Admin, path, fmt.Sprintf(`{"id":"%d"}`, sl(w, slMember)), "an id that is a string"),
				flux(Admin, path, `{"id":-3}`, "a negative id"),
				flux(Admin, path, `{"id":4294967299}`, "an id beyond 32 bits"),
				flux(Admin, path, nil, "no body"),
			}, fluxForbidden(path, id(sl(w, slMember)))...)
		}},
		{RouteID: "forward.speed_limit.list.post", Writes: func(w *World) []Req {
			path := "/api/v2/speed-limit/list"
			// Runs after the creations: their times are the clock's.
			return append([]Req{
				flux(Admin, path, nil, "every limit by id", listTimes...),
				flux(Admin, path, anyMap("tunnelId", tn(w, tnHKJP)), "a body is ignored", listTimes...),
				flux(Staff, path, map[string]any{}, "staff", listTimes...),
			}, fluxForbidden(path, nil)...)
		}},
		{RouteID: "forward.speed_limit.tunnels.post", Writes: func(w *World) []Req {
			path := "/api/v2/speed-limit/tunnels"
			return append([]Req{
				flux(Admin, path, nil, "every active tunnel"),
				flux(Staff, path, map[string]any{}, "staff"),
				flux(Admin2, path, anyMap("tunnelId", tn(w, tnHKJP)), "a body is ignored"),
			}, fluxForbidden(path, nil)...)
		}},
		{RouteID: "forward.user.reset.post", Writes: func(w *World) []Req {
			// Type 1 goes through KernelSubscriber.ResetTraffic; without an
			// Idempotency-Key each request is a reset of its own.
			path := "/api/v2/user/reset"
			return append([]Req{
				flux(Admin, path, anyMap("id", w.ID("user.member", 20), "type", 1), "a subscriber's traffic"),
				flux(Staff, path, anyMap("id", w.ID("user.expired", 3), "type", 1), "an expired subscriber's traffic"),
				flux(Admin, path, anyMap("id", Missing, "type", 1), "an unknown subscriber"),
				flux(Admin, path, anyMap("id", w.ID("user_tunnel.user", 0), "type", 2), "a permission's traffic and its forwards'"),
				flux(Admin, path, anyMap("id", w.ID("user_tunnel.user2", 0), "type", 2), "another permission"),
				flux(Admin, path, anyMap("id", Missing, "type", 2), "an unknown permission"),
				flux(Admin, path, anyMap("id", w.ID("user_tunnel", 0), "type", 3), "an unknown type"),
				flux(Admin, path, anyMap("id", w.ID("user_tunnel", 0), "type", -1), "a negative type"),
				flux(Admin, path, anyMap("id", w.ID("user_tunnel", 0)), "no type"),
				flux(Admin, path, anyMap("id", 0, "type", 1), "id zero"),
				flux(Admin, path, fmt.Sprintf(`{"id":"%d","type":1}`, w.ID("user.member", 20)), "an id that is a string"),
				flux(Admin, path, nil, "no body"),
			}, fluxForbidden(path, anyMap("id", w.PersonaID(User), "type", 1))...)
		}},

		{RouteID: "forward.admin.forward.observability.multi_ingress.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/forward/observability/multi-ingress"
			target := func(v any) url.Values { return q("targetId", fmt.Sprint(v)) }
			return append([]Req{
				{Persona: Admin, Path: path, Query: target(w.ID("forward.user", 0)), Label: "a forward's ingress latencies"},
				{Persona: Admin, Path: path, Query: target(w.ID("forward.user", 2)), Label: "a port forwarding tunnel's exit (several tunnels)"},
				{Persona: Staff, Path: path, Query: target(w.ID("forward.user2", 0)), Label: "an offline exit node"},
				{Persona: Admin, Path: path, Query: target(w.ID("forward.noingress", 0)), Label: "a tunnel without an entry node"},
				{Persona: Admin, Path: path, Query: target(Missing), Label: "an unknown forward"},
				{Persona: Admin, Path: path, Label: "no target"},
				{Persona: Admin, Path: path, Query: target(0), Label: "target zero"},
				{Persona: Admin, Path: path, Query: target("x"), Label: "a target that is not a number"},
				{Persona: Admin, Path: path, Query: target("4294967297"), Label: "a target beyond 32 bits"},
			}, forbidden(path)...)
		}},
		{RouteID: "forward.admin.forward.stats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/forward/stats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "enabled relay and exit nodes"},
				{Persona: Staff, Path: path, Query: q("type", "relay"), Label: "ignored filter"},
			}, forbidden(path)...)
		}},
		{RouteID: "forward.user.forward.rules.get", Reads: func(w *World) []Req {
			path := "/api/v2/user/forward/rules"
			return []Req{
				{Persona: User, Path: path, Label: "own rules with their nodes, no API token"},
				{Persona: User2, Path: path, Label: "own rule"},
				{Persona: Fresh, Path: path, Label: "none"},
				{Persona: Expired, Path: path, Label: "expired member, none"},
				{Persona: Admin, Path: path, Label: "an administrator's own (none)"},
				{Persona: Anon, Path: path, Label: "anonymous"},
			}
		}},
	}
}

// ---------------------------------------------------------------------------
// Proxy nodes.

func proxyNodeSpecs() []RouteSpec {
	stamped := []string{"data.created_at", "data.updated_at"}
	lb := func(id any) string { return fill("/api/v2/admin/loadbalancers/:id", id) }
	logs := func(id any) string { return fill("/api/v2/admin/nodes/:id/logs", id) }
	return []RouteSpec{
		{RouteID: "proxy.loadbalancer.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/loadbalancers"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "newest first"},
				{Persona: Admin, Path: path, Query: q("group_id", fmt.Sprint(w.ID("node_group", 1))), Label: "one group"},
				{Persona: Staff, Path: path, Query: q("group_id", fmt.Sprint(w.ID("node_group", 0))), Label: "another group"},
				{Persona: Admin, Path: path, Query: q("group_id", fmt.Sprint(Missing)), Label: "a group without load balancers"},
				{Persona: Admin, Path: path, Query: q("group_id", "-4"), Label: "a negative group is every group"},
				{Persona: Admin, Path: path, Query: q("group_id", "abc"), Label: "a group that is not a number"},
			}, forbidden(path)...)
		}},
		{RouteID: "proxy.loadbalancer.id.get", Reads: func(w *World) []Req {
			var reqs []Req
			for i := 0; i < 6; i++ {
				reqs = append(reqs, Req{Persona: Admin, Path: lb(w.ID("loadbalancer", i)), Label: "a load balancer (weights object, text, array, none)"})
			}
			return append(append(reqs,
				Req{Persona: Admin, Path: lb(Missing), Label: "not found"},
				Req{Persona: Admin, Path: lb("abc"), Label: "invalid id"},
				Req{Persona: Admin, Path: lb("4294967297"), Label: "beyond 32 bits"},
			), forbidden(lb(w.ID("loadbalancer", 0)))...)
		}},
		{RouteID: "proxy.admin.nodes.stats.get", Reads: func(w *World) []Req {
			path := "/api/v2/admin/nodes/stats"
			return append([]Req{
				{Persona: Admin, Path: path, Label: "every status"},
				{Persona: Staff, Path: path, Label: "staff"},
			}, forbidden(path)...)
		}},
		{RouteID: "proxy.admin.nodes.id.logs.get", Reads: func(w *World) []Req {
			busy := w.ID("node.logs", 0)
			return append([]Req{
				{Persona: Admin, Path: logs(busy), Label: "newest first"},
				{Persona: Admin, Path: logs(busy), Query: q("page", "2", "page_size", "10"), Label: "page 2"},
				{Persona: Admin, Path: logs(busy), Query: q("page", "9", "page_size", "10"), Label: "page past the end"},
				{Persona: Admin, Path: logs(busy), Query: q("page_size", "1000"), Label: "page size above the limit"},
				{Persona: Admin, Path: logs(busy), Query: q("page", "-1", "page_size", "0"), Label: "page and page size below one"},
				{Persona: Admin, Path: logs(busy), Query: q("page", "x", "page_size", "y"), Label: "pagination that is not a number"},
				{Persona: Admin, Path: logs(busy), Query: q("level", "WARN"), Label: "level as reported"},
				{Persona: Admin, Path: logs(busy), Query: q("level", " fatal "), Label: "level fatal is error"},
				{Persona: Admin, Path: logs(busy), Query: q("level", "trace"), Label: "unknown level"},
				{Persona: Admin, Path: logs(busy), Query: q("source", " agent "), Label: "source"},
				{Persona: Admin, Path: logs(busy), Query: q("search", "agent"), Label: "search in message, source and trace"},
				{Persona: Admin, Path: logs(busy), Query: q("search", "e", "level", "info", "page_size", "5"), Label: "search, level and page size"},
				{Persona: Staff, Path: logs(w.ID("node.logs", 1)), Label: "another node"},
				{Persona: Admin, Path: logs(w.ID("node.logs", 2)), Query: q("search", "用户"), Label: "a search in Chinese"},
				{Persona: Admin, Path: logs(w.ID("node.quiet", 0)), Label: "a node without logs"},
				{Persona: Admin, Path: logs(Missing), Label: "unknown node"},
				{Persona: Admin, Path: logs("abc"), Label: "invalid id"},
			}, forbidden(logs(busy))...)
		}},
		{RouteID: "proxy.loadbalancer.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/loadbalancers"
			n := func(i int) uint { return w.ID("node", i) }
			return []Req{
				{Persona: Admin, Path: path, Mask: stamped, Label: "full body with weights",
					Body: fmt.Sprintf(`{"name":"lb-staging","group_id":%d,"strategy":"least-load","health_check":true,"check_interval":15,"check_timeout":3,"enabled":true,"weights":{"%d": 2, "%d":1}}`,
						w.ID("node_group", 2), n(0), n(1))},
				{Persona: Admin, Path: path, Body: fmt.Sprintf(`{"name":"lb-text","node_weights":"{\"%d\":2}"}`, n(2)), Mask: stamped, Label: "weights as text"},
				{Persona: Staff, Path: path, Body: `{"name":"lb-both","weights":{"1":1},"node_weights":"{\"2\":2}"}`, Mask: stamped, Label: "text weights win over weights"},
				{Persona: Admin, Path: path, Body: `{"name":"lb-null","weights":null}`, Mask: stamped, Label: "null weights"},
				{Persona: Admin, Path: path, Body: `{"name":"lb-off","health_check":false,"enabled":false}`, Mask: stamped, Label: "flags off are written as their defaults"},
				{Persona: Admin, Path: path, Body: `{}`, Mask: stamped, Label: "empty object"},
				{Persona: Admin, Path: path, Body: `{"name":"x","strategy":"fastest"}`, Label: "unknown strategy"},
				{Persona: Admin, Path: path, Body: fmt.Sprintf(`{"name":"%0256d"}`, 0), Label: "name too long"},
				{Persona: Admin, Path: path, Body: `{"name":"x","check_interval":-5}`, Label: "negative interval"},
				{Persona: Admin, Path: path, Body: `{"name":"x","group_id":-1}`, Label: "negative group"},
				{Persona: Admin, Path: path, Body: `{"name":`, Label: "invalid JSON"},
				{Persona: Admin, Path: path, Label: "no body"},
				{Persona: User, Path: path, Body: `{"name":"member"}`, Label: "member on admin route"},
			}
		}},
		{RouteID: "proxy.loadbalancer.id.put", Writes: func(w *World) []Req {
			updated := []string{"data.updated_at"}
			l := func(i int) string { return lb(w.ID("loadbalancer", i)) }
			return []Req{
				{Persona: Admin, Path: l(0), Body: fmt.Sprintf(`{"name":"lb-hk 香港 v2","group_id":%d,"strategy":"weighted-random"}`, w.ID("node_group", 3)), Mask: updated, Label: "rename and regroup"},
				{Persona: Admin, Path: l(1), Body: `{"health_check":false,"enabled":false}`, Mask: updated, Label: "turn both flags off"},
				{Persona: Staff, Path: l(4), Body: `{"health_check":true,"enabled":true}`, Mask: updated, Label: "turn both flags on"},
				{Persona: Admin, Path: l(2), Body: fmt.Sprintf(`{"weights":{"%d":9}}`, w.ID("node", 3)), Mask: updated, Label: "replace weights"},
				{Persona: Admin, Path: l(3), Body: `{"name":"","group_id":0,"check_interval":0,"check_timeout":0,"weights":null}`, Mask: updated, Label: "zero values keep the stored ones"},
				{Persona: Admin, Path: l(5), Body: `{}`, Mask: updated, Label: "empty object"},
				{Persona: Admin, Path: l(0), Body: `{"strategy":"fastest"}`, Label: "unknown strategy"},
				{Persona: Admin, Path: l(0), Body: `[`, Label: "invalid JSON"},
				{Persona: Admin, Path: lb(Missing), Body: `{"name":"x"}`, Label: "not found"},
				{Persona: Admin, Path: lb("x"), Body: `{"name":"x"}`, Label: "invalid id"},
				{Persona: User, Path: l(0), Body: `{"name":"m"}`, Label: "member on admin route"},
			}
		}},
		{RouteID: "proxy.loadbalancer.id.delete", Writes: func(w *World) []Req {
			return []Req{
				{Persona: Admin, Path: lb(w.ID("loadbalancer", 2)), Label: "existing"},
				{Persona: Admin, Path: lb(w.ID("loadbalancer", 2)), Label: "again"},
				{Persona: Admin, Path: lb(Missing), Label: "missing succeeds"},
				{Persona: Admin, Path: lb("-1"), Label: "invalid id"},
				{Persona: User, Path: lb(w.ID("loadbalancer", 3)), Label: "member on admin route"},
				{Persona: Anon, Path: lb(w.ID("loadbalancer", 3)), Label: "anonymous"},
			}
		}},
	}
}

// ---------------------------------------------------------------------------
// gost-mesh: NodeX diagnosis and the gost API connection test.

func gostMeshSpecs() []RouteSpec {
	// The staging stack has no forward_runtime configuration: NodeX has no
	// address, so both sides answer the same configuration error at once.
	// With an address configured the answer carries checkedAt, the clock's
	// time, which a shadow comparison would count (gostmeshcompat masks
	// data.checkedAt).
	nodeX := func(path string) func(w *World) []Req {
		return func(w *World) []Req {
			return append([]Req{
				{Persona: Admin, Path: path, Label: "runtime diagnosis"},
				{Persona: Staff, Path: path, Label: "staff"},
				{Persona: Admin2, Path: path, Query: q("verbose", "1"), Label: "ignored query"},
			}, forbidden(path)...)
		}
	}
	return []RouteSpec{
		{RouteID: "gost.admin.forward.nodex.status.get", Reads: nodeX("/api/v2/admin/forward/nodex/status")},
		{RouteID: "gost.admin.forward.nodex.doctor.get", Reads: nodeX("/api/v2/admin/forward/nodex/doctor")},
		{RouteID: "gost.admin.forward.test_connection.post", Writes: func(w *World) []Req {
			// TEST-NET hosts: the internal staging network has no route, so
			// the dial fails at once (network unreachable) or, at worst,
			// after the gost client's 10s timeout, the same way on both
			// sides. The native side records a KernelNodeOps operation
			// (v4 ledger) the legacy handler does not: that table is not
			// compared.
			path := "/api/v2/admin/forward/test-connection"
			req := func(host string, port int, token string) string {
				return fmt.Sprintf(`{"host":%q,"api_port":%d,"api_token":%q}`, host, port, token)
			}
			return append([]Req{
				flux(Admin, path, req("192.0.2.10", 9001, "fake-gost-api-token"), "an unreachable gost API"),
				flux(Staff, path, req("198.51.100.11", 9001, ""), "an unreachable gost API without a token"),
				flux(Admin, path, req("gost host", 9001, "fake-gost-api-token"), "a host that is not a host name"),
				flux(Admin, path, req("203.0.113.21", -1, "fake-gost-api-token"), "a negative port"),
				flux(Admin, path, req("192.0.2.10:9001/x?", 9001, "fake-gost-api-token"), "a host with a path"),
				flux(Admin, path, nil, "no body"),
				flux(Admin, path, `{"host":`, "invalid JSON"),
				flux(Admin, path, `{"api_port":8080}`, "no host"),
				flux(Admin, path, `{"host":"192.0.2.10"}`, "no port"),
				flux(Admin, path, `{"host":"192.0.2.10","api_port":0}`, "port zero"),
				flux(Admin, path, `{"host":"192.0.2.10","api_port":"8080"}`, "a port that is a string"),
				flux(Admin, path, `{"host":7,"api_port":8080}`, "a host that is a number"),
				flux(Admin, path, `[1]`, "a body that is an array"),
				flux(Admin, path, `"192.0.2.10"`, "a body that is a string"),
				flux(Admin, path, `null`, "a body that is null"),
			}, fluxForbidden(path, req("192.0.2.10", 9001, "x"))...)
		}},
	}
}

// ---------------------------------------------------------------------------
// WireGuard: the administrator's key pair, random on each side.

func wireguardSpecs() []RouteSpec {
	return []RouteSpec{
		{RouteID: "wireguard.admin.wireguard.keypair.post", Writes: func(w *World) []Req {
			path := "/api/v2/admin/wireguard/keypair"
			keys := []string{"data.private_key", "data.public_key"}
			return append([]Req{
				flux(Admin, path, nil, "no body", keys...),
				flux(Admin, path, `{"private_key":"chosen","public_key":"chosen"}`, "a body, ignored", keys...),
				flux(Staff, path, "not json", "a body that is not JSON, ignored", keys...),
			}, fluxForbidden(path, nil)...)
		}},
	}
}
