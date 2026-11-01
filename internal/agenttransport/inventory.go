package agenttransport

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"gorm.io/gorm"
)

// UpgradeGuideURL is where the inventory points an operator at enrolling
// agents: the UPGRADE section that walks a node to an enrolled Agent.
const UpgradeGuideURL = "https://github.com/AnixOps/anix-control/blob/main/docs/UPGRADE.md#agent-transports-preparing-for-v42"

// Node statuses of the inventory: how the node's agent reaches Control on
// the AnixOps Agent channels.
const (
	// StatusMTLS: the node's latest AnixOps Agent channel was the mTLS
	// stream. Ready for agent_control.mtls: required.
	StatusMTLS = "mtls"
	// StatusUnseen: no sighting since this inventory exists.
	StatusUnseen = "unseen"
)

// TransportSeen is one transport a node was seen on.
type TransportSeen struct {
	Transport    string    `json:"transport"`
	AgentVersion string    `json:"agent_version,omitempty"`
	Identity     string    `json:"identity,omitempty"`
	FirstSeenAt  time.Time `json:"first_seen_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

// Certificate is an agent certificate of a node, from the record the agent
// PKI keeps of every certificate it issued (no key, no certificate bytes).
type Certificate struct {
	Serial   string    `json:"serial"`
	NotAfter time.Time `json:"not_after"`
	SPIFFEID string    `json:"spiffe_id,omitempty"`
	// IssuedAt is when the PKI issued it, and RenewAfter when its Agent
	// renews it: two thirds of the lifetime, the renew_after_unix the Agent
	// was told (modulepki.RenewAfter). A valid certificate past RenewAfter
	// means the Agent did not renew in time.
	IssuedAt   time.Time `json:"issued_at"`
	RenewAfter time.Time `json:"renew_after"`
	// RevokedAt and RevokeReason are set on a revoked certificate: a
	// disabled or deleted node, replaced or revoked credentials.
	RevokedAt    *time.Time `json:"revoked_at"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
}

// Certificate states of a node (NodeTransports.CertificateState).
const (
	// CertificateValid: the node holds an unexpired, unrevoked certificate.
	CertificateValid = "valid"
	// CertificateRevoked: it holds none, and its newest record is revoked.
	// The Agent must enroll again.
	CertificateRevoked = "revoked"
	// CertificateExpired: it holds none, and its newest record expired
	// without being revoked (the Agent was away longer than the lifetime).
	CertificateExpired = "expired"
	// CertificateNone: no record. The node never enrolled, or its records
	// were pruned (a day after they expired).
	CertificateNone = "none"
)

// Connection types of a node (Connection.Type).
const (
	// ConnectionMTLSStream: an Agent Control stream authenticated by the
	// agent client certificate.
	ConnectionMTLSStream = "mtls_stream"
	// ConnectionOffline: nothing was seen within ConnectionWindow.
	ConnectionOffline = "offline"
)

// ConnectionWindow is how recent the last sighting of a transport must be
// for a node to count as connected on it: the five minutes that make a
// node's heartbeat current (model.Node.IsOnline).
const ConnectionWindow = 5 * time.Minute

// Connection is how a node's agent reaches Control right now.
type Connection struct {
	// Type is ConnectionMTLSStream or ConnectionOffline.
	Type string `json:"type"`
	// Transport is the transport the type was read from (mtls-stream),
	// empty when offline.
	Transport string `json:"transport,omitempty"`
	// LastSeenAt is when that transport was last seen; nil when offline.
	LastSeenAt *time.Time `json:"last_seen_at"`
}

// NodeTransports is one node of the inventory.
type NodeTransports struct {
	// Node is proxy-<id> or forward-<id>.
	Node    string `json:"node"`
	Kind    string `json:"kind"`
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Status is StatusMTLS or StatusUnseen.
	Status string `json:"status"`
	// Transport is the transport of the latest sighting, empty if unseen.
	Transport    string `json:"transport"`
	AgentVersion string `json:"agent_version,omitempty"`
	// KernelVersion is the anx-node release the node's live session
	// reported, "" without a live session or when the Agent reports none
	// (no kernel installed, or an Agent that predates the report).
	KernelVersion string     `json:"kernel_version,omitempty"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
	// Certificate is the newest valid certificate, nil when there is none:
	// nil also means never enrolled to the readiness check.
	Certificate *Certificate `json:"certificate"`
	// CertificateState is CertificateValid, CertificateRevoked,
	// CertificateExpired or CertificateNone.
	CertificateState string `json:"certificate_state"`
	// LastCertificate is the newest certificate record in any state, so a
	// revoked or expired one still shows its expiry and why it ended; it is
	// Certificate while that exists. Nil with CertificateNone.
	LastCertificate *Certificate `json:"last_certificate"`
	// Connection is how the agent is connected now.
	Connection Connection `json:"connection"`
	// Transports lists every transport seen, newest first.
	Transports []TransportSeen `json:"transports"`
	// Session is the node's live Agent Control stream session in this
	// process, nil when it has none (or the caller reads no sessions).
	Session *LiveSession `json:"session"`
}

// LiveSession is a node's live Agent Control stream session: how it
// authenticated and what it negotiated. It carries no credential.
type LiveSession struct {
	SessionID string `json:"session_id"`
	// Authentication is mtls; Identity the SPIFFE ID.
	Authentication string `json:"authentication"`
	Identity       string `json:"identity"`
	// Certificate is the client certificate of an mtls session: serial,
	// expiry and URI SAN.
	Certificate  *agentstreams.SessionCertificate `json:"certificate"`
	AgentVersion string                           `json:"agent_version,omitempty"`
	// KernelVersion is the anx-node release the session's Hello named.
	KernelVersion string    `json:"kernel_version,omitempty"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	// NegotiatedCapabilities are the data-plane capabilities in use on the
	// session (name.version, such as config.v1).
	NegotiatedCapabilities []string `json:"negotiated_capabilities"`
	// AgentMetrics are the Agent's own health metrics (agent_control_*,
	// agent_identity_*, agent_dataplane_*: stream, certificate, spool depth
	// and drops, apply failures) from its latest heartbeat that carried any.
	AgentMetrics   map[string]float64 `json:"agent_metrics"`
	AgentMetricsAt *time.Time         `json:"agent_metrics_at"`
}

// Summary counts the inventory's nodes by status, and tells whether any
// enabled node has not enrolled.
type Summary struct {
	Total  int `json:"total"`
	MTLS   int `json:"mtls"`
	Unseen int `json:"unseen"`
	// ReadyForRequired is true when no enabled node is unseen without an
	// agent certificate. It is computed over every node.
	ReadyForRequired bool `json:"ready_for_required"`
	// RequiredReasons explains a false ReadyForRequired, empty otherwise.
	RequiredReasons []string `json:"required_reasons"`
	// RequiredBlockers lists the enabled nodes required would refuse.
	RequiredBlockers []RequiredBlocker `json:"required_blockers"`
}

// Why agent_control.mtls: required would refuse a node (RequiredBlocker).
const (
	// BlockerNeverEnrolled: the node was never seen and holds no valid
	// agent certificate, so its agent never enrolled.
	BlockerNeverEnrolled = "never_enrolled"
)

// RequiredBlocker is an enabled node agent_control.mtls: required would
// refuse.
type RequiredBlocker struct {
	Node   string `json:"node"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
	// Transport and LastSeenAt are empty and nil for never_enrolled.
	Transport  string     `json:"transport,omitempty"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

// requiredBlocker tells whether required would refuse node, and why. Only
// enabled nodes count: a disabled node's agent is refused in every mode.
// An unseen node with a valid certificate has enrolled.
func requiredBlocker(node NodeTransports) (RequiredBlocker, bool) {
	if !node.Enabled {
		return RequiredBlocker{}, false
	}
	blocker := RequiredBlocker{Node: node.Node, Name: node.Name}
	switch {
	case node.Status == StatusUnseen && node.Certificate == nil:
		blocker.Reason = BlockerNeverEnrolled
	default:
		return RequiredBlocker{}, false
	}
	return blocker, true
}

// requiredReasons explains the blockers.
func requiredReasons(summary Summary) []string {
	reasons := []string{}
	if len(summary.RequiredBlockers) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d enabled node(s) never enrolled (no agent certificate, never seen): install or enroll their Agents, or disable the nodes", len(summary.RequiredBlockers)))
	}
	return reasons
}

// Inventory is the transport inventory.
type Inventory struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Summary     Summary          `json:"summary"`
	Nodes       []NodeTransports `json:"nodes"`
	// UpgradeGuide is where the operator reads how to move legacy nodes.
	UpgradeGuide string `json:"upgrade_guide"`
}

// inventoryProxyNode and inventoryForwardNode read only the columns the
// inventory shows: never a node's credentials.
type inventoryProxyNode struct {
	ID            uint
	Name          string
	Status        model.NodeStatus
	ServerVersion *string
}

type inventoryForwardNode struct {
	ID      uint
	Name    string
	Enabled bool
}

// Options select what Build returns.
type Options struct {
	// Live overlays sightings of this process not yet written (the API);
	// the CLI, another process, reads the table only.
	Live *Recorder
	// Now is the inventory's clock; zero means time.Now.
	Now time.Time
	// Sessions lists this process's live agent sessions; their Agent
	// Control stream sessions are shown with their nodes. Nil shows none
	// (the CLI, another process).
	Sessions func() []agentstreams.Session
	// Nodes keeps the listed nodes only (the detail page asks for one);
	// empty keeps every node. It narrows the list and its counts, not the
	// readiness fields of the summary, which cover every node.
	Nodes []agentcontrol.AgentNode
}

// Build reads the transport inventory: every proxy and forward node with
// its transports, newest sighting, agent version and certificate.
func Build(ctx context.Context, db *gorm.DB, options Options) (Inventory, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	inventory := Inventory{GeneratedAt: now, UpgradeGuide: UpgradeGuideURL, Nodes: []NodeTransports{},
		Summary: Summary{RequiredReasons: []string{}, RequiredBlockers: []RequiredBlocker{}}}
	db = db.WithContext(ctx)

	var proxies []inventoryProxyNode
	if err := db.Model(&model.Node{}).Select("id, name, status, server_version").Order("id").Scan(&proxies).Error; err != nil {
		return Inventory{}, err
	}
	var forwards []inventoryForwardNode
	if err := db.Model(&model.ForwardNode{}).Select("id, name, enabled").Order("id").Scan(&forwards).Error; err != nil {
		return Inventory{}, err
	}
	var rows []model.AgentTransport
	if err := db.Find(&rows).Error; err != nil {
		return Inventory{}, err
	}
	rows = overlay(rows, options.Live.Snapshot())
	// Every record, valid or not: the PKI prunes them a day after they
	// expired, so the table holds a few per node.
	var certificates []model.AgentCertificate
	if err := db.Order("not_after DESC, created_at DESC").Find(&certificates).Error; err != nil {
		return Inventory{}, err
	}
	type nodeKey struct {
		kind string
		id   uint
	}
	seen := map[nodeKey][]TransportSeen{}
	for _, row := range rows {
		// Rows of channels removed earlier may remain in the table: they
		// are not shown.
		if row.Transport != model.AgentTransportMTLSStream {
			continue
		}
		key := nodeKey{row.NodeKind, row.NodeID}
		seen[key] = append(seen[key], TransportSeen{
			Transport: row.Transport, AgentVersion: row.AgentVersion,
			Identity: row.Identity, FirstSeenAt: row.FirstSeenAt.UTC(), LastSeenAt: row.LastSeenAt.UTC(),
		})
	}
	// The records come newest expiry first: the first valid one of a node is
	// its certificate, the first of any state its latest one.
	newestCertificate := map[nodeKey]*Certificate{}
	latestCertificate := map[nodeKey]*Certificate{}
	for _, record := range certificates {
		key := nodeKey{record.NodeKind, record.NodeID}
		certificate := certificateOf(record)
		if _, ok := latestCertificate[key]; !ok {
			latestCertificate[key] = certificate
		}
		if _, ok := newestCertificate[key]; !ok && record.RevokedAt == nil && record.NotAfter.After(now) {
			newestCertificate[key] = certificate
		}
	}

	liveSessions := map[nodeKey]*LiveSession{}
	if options.Sessions != nil {
		for _, session := range options.Sessions() {
			if session.Transport != agentstreams.TransportControlStream {
				continue
			}
			negotiated := append([]string{}, session.NegotiatedCapabilities...)
			liveSessions[nodeKey{session.Node.Kind, uint(session.Node.ID)}] = &LiveSession{
				SessionID: session.SessionID, Authentication: session.Authentication, Identity: session.Identity,
				Certificate: session.Certificate, AgentVersion: session.AgentVersion, KernelVersion: session.KernelVersion, ConnectedAt: session.ConnectedAt.UTC(),
				LastSeenAt: session.LastSeen.UTC(), NegotiatedCapabilities: negotiated,
				AgentMetrics: session.AgentMetrics, AgentMetricsAt: session.AgentMetricsAt,
			}
		}
	}

	wanted := map[nodeKey]bool{}
	for _, node := range options.Nodes {
		wanted[nodeKey{node.Kind, uint(node.ID)}] = true
	}
	add := func(kind string, id uint, name string, enabled bool, fallbackVersion string) {
		key := nodeKey{kind, id}
		node := NodeTransports{
			Node: agentcontrol.AgentNode{Kind: kind, ID: uint32(id)}.String(), Kind: kind, ID: id, Name: name, // #nosec G115 -- node ids are uint32 on every agent channel.
			Enabled: enabled, Certificate: newestCertificate[key], LastCertificate: latestCertificate[key],
			Transports: seen[key], Session: liveSessions[key],
		}
		if node.Transports == nil {
			node.Transports = []TransportSeen{}
		}
		if node.Session != nil {
			node.KernelVersion = node.Session.KernelVersion
		}
		classify(&node, fallbackVersion)
		node.CertificateState = certificateState(node.Certificate, node.LastCertificate)
		node.Connection = connectionOf(node, now, options.Sessions != nil)
		if blocker, ok := requiredBlocker(node); ok {
			inventory.Summary.RequiredBlockers = append(inventory.Summary.RequiredBlockers, blocker)
		}
		if len(wanted) > 0 && !wanted[key] {
			return
		}
		inventory.Nodes = append(inventory.Nodes, node)
	}
	for _, proxy := range proxies {
		version := ""
		if proxy.ServerVersion != nil {
			version = *proxy.ServerVersion
		}
		add(agentcontrol.NodeKindProxy, proxy.ID, proxy.Name, proxy.Status != model.NodeStatusDisabled, version)
	}
	for _, forward := range forwards {
		add(agentcontrol.NodeKindForward, forward.ID, forward.Name, forward.Enabled, "")
	}
	for _, node := range inventory.Nodes {
		inventory.Summary.Total++
		switch node.Status {
		case StatusMTLS:
			inventory.Summary.MTLS++
		default:
			inventory.Summary.Unseen++
		}
	}
	inventory.Summary.RequiredReasons = requiredReasons(inventory.Summary)
	inventory.Summary.ReadyForRequired = len(inventory.Summary.RequiredBlockers) == 0
	return inventory, nil
}

// certificateOf renders a certificate record. RenewAfter is derived from the
// recorded issue and expiry times, as the Agent was told it.
func certificateOf(record model.AgentCertificate) *Certificate {
	node := agentcontrol.AgentNode{Kind: record.NodeKind, ID: uint32(record.NodeID)} // #nosec G115 -- node ids are uint32 on every agent channel.
	issuedAt := record.CreatedAt.UTC()
	certificate := &Certificate{
		Serial: record.Serial, NotAfter: record.NotAfter.UTC(),
		SPIFFEID: agentcontrol.AgentIdentity{Cluster: record.Cluster, Node: node}.String(),
		IssuedAt: issuedAt, RenewAfter: modulepki.RenewAfter(issuedAt, record.NotAfter.UTC()),
		RevokeReason: record.RevokeReason,
	}
	if record.RevokedAt != nil {
		revokedAt := record.RevokedAt.UTC()
		certificate.RevokedAt = &revokedAt
	}
	return certificate
}

// certificateState names where a node stands: valid when it holds a valid
// certificate, else by its newest record.
func certificateState(valid, latest *Certificate) string {
	switch {
	case valid != nil:
		return CertificateValid
	case latest == nil:
		return CertificateNone
	case latest.RevokedAt != nil:
		return CertificateRevoked
	default:
		return CertificateExpired
	}
}

// connectionOf tells how node's agent is connected at now.
//   - A live Agent Control stream session of this process decides: mTLS or
//     mTLS.
//   - Otherwise the newest transport seen within ConnectionWindow does: the
//     mTLS stream counts only when no session provider was read (the CLI),
//     since with one the stream is not connected.
//   - Else the node is offline.
//
// Sessions are in memory of the process that holds the stream, as the
// inventory's session already is: the Control behind the API serves every
// stream today (one replica, container-deployment.md).
func connectionOf(node NodeTransports, now time.Time, sessionsRead bool) Connection {
	if session := node.Session; session != nil {
		if session.Authentication == agentstreams.AuthenticationMTLS {
			lastSeen := session.LastSeenAt
			return Connection{Type: ConnectionMTLSStream, Transport: model.AgentTransportMTLSStream, LastSeenAt: &lastSeen}
		}
	}
	for _, seen := range node.Transports {
		if now.Sub(seen.LastSeenAt) > ConnectionWindow {
			// Newest first: the rest is older.
			break
		}
		if seen.Transport == model.AgentTransportMTLSStream && !sessionsRead {
			lastSeen := seen.LastSeenAt
			return Connection{Type: ConnectionMTLSStream, Transport: seen.Transport, LastSeenAt: &lastSeen}
		}
	}
	return Connection{Type: ConnectionOffline}
}

// classify orders a node's transports newest first and derives its status,
// latest transport and agent version.
func classify(node *NodeTransports, fallbackVersion string) {
	sort.SliceStable(node.Transports, func(i, j int) bool {
		return node.Transports[i].LastSeenAt.After(node.Transports[j].LastSeenAt)
	})
	node.Status = StatusUnseen
	if len(node.Transports) > 0 {
		latest := node.Transports[0]
		node.Transport = latest.Transport
		lastSeen := latest.LastSeenAt
		node.LastSeenAt = &lastSeen
		node.Status = StatusMTLS
	}
	// The stream reports the agent's version in Hello; other transports
	// fall back to what the node row recorded.
	for _, transport := range node.Transports {
		if transport.AgentVersion != "" {
			node.AgentVersion = transport.AgentVersion
			return
		}
	}
	node.AgentVersion = fallbackVersion
}

// overlay merges this process's in-memory sightings over the table's rows:
// the newer last sighting wins.
func overlay(rows, live []model.AgentTransport) []model.AgentTransport {
	if len(live) == 0 {
		return rows
	}
	type key struct {
		kind, transport string
		id              uint
	}
	index := map[key]int{}
	for i, row := range rows {
		index[key{row.NodeKind, row.Transport, row.NodeID}] = i
	}
	for _, row := range live {
		k := key{row.NodeKind, row.Transport, row.NodeID}
		i, ok := index[k]
		if !ok {
			index[k] = len(rows)
			rows = append(rows, row)
			continue
		}
		if row.LastSeenAt.After(rows[i].LastSeenAt) {
			first := rows[i].FirstSeenAt
			rows[i] = row
			if !first.IsZero() && first.Before(row.FirstSeenAt) {
				rows[i].FirstSeenAt = first
			}
		}
	}
	return rows
}
