// Package realitytarget picks REALITY targets (dest and SNI) per proxy node
// (design: "Reality 目标站探测", phases 1 and 2). The node's Agent probes each
// candidate from its own network (reality.probe, kernelnodeops.RealityProbes);
// this package holds the candidates, scores the answers, keeps the latest
// probe of each candidate (v5_reality_target_probe), recommends the best
// one, and applies a target an administrator picks to a Reality protocol
// (Apply).
//
// Everything here is kernel-neutral: a probe and a target are a host name
// and a port, whatever core serves the node's REALITY inbound.
package realitytarget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// MaxCandidates bounds the candidates of one run: a node probes a short
	// list, never scans (RealiTLScanner warns that scanning from a VPS may
	// get it flagged).
	MaxCandidates = 24
	// Concurrency is how many probes of one node run at once.
	Concurrency = 8
	// DefaultTimeout is a probe's own time on the Agent.
	DefaultTimeout = 5 * time.Second
	// MinTimeout and MaxTimeout bound it (the Agent's bounds).
	MinTimeout = 100 * time.Millisecond
	MaxTimeout = 10 * time.Second
)

// DefaultCandidates are probed when a run names none of its own, or with
// them: large sites on different providers known to answer TLS 1.3 and h2.
// The probe decides; the list only seeds it. Names the GFW blocks by SNI
// (Google, and so on) are left out on purpose: the SNI travels in clear.
var DefaultCandidates = []string{
	"www.microsoft.com",
	"www.bing.com",
	"www.apple.com",
	"swdist.apple.com",
	"gateway.icloud.com",
	"addons.mozilla.org",
	"www.mozilla.org",
	"www.lovelive-anime.jp",
	"www.nvidia.com",
	"www.amd.com",
	"www.samsung.com",
	"www.dell.com",
	"www.oracle.com",
	"www.sap.com",
	"www.cisco.com",
	"s0.awsstatic.com",
	"d1.awsstatic.com",
	"m.media-amazon.com",
	"www.tesla.com",
	"www.ikea.com",
}

// Candidate is one target to probe.
type Candidate struct {
	ServerName string `json:"server_name"`
	Port       int    `json:"port"`
}

func (c Candidate) String() string { return net.JoinHostPort(c.ServerName, strconv.Itoa(c.Port)) }

// ParseCandidate reads "host" or "host:port" (port 443 when absent). A
// candidate is a host name: an address is refused, because the SNI is a
// name and the Agent resolves it itself.
func ParseCandidate(raw string) (Candidate, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	host, port := raw, 443
	if h, p, err := net.SplitHostPort(raw); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return Candidate{}, fmt.Errorf("%q: the port must be from 1 to 65535", raw)
		}
		host, port = h, n
	}
	host = strings.TrimSuffix(host, ".")
	if !validHostName(host) {
		return Candidate{}, fmt.Errorf("%q is not a host name", raw)
	}
	return Candidate{ServerName: host, Port: port}, nil
}

func validHostName(host string) bool {
	if len(host) == 0 || len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

// Candidates builds a run's list: extra first, then the defaults when
// withDefaults, without duplicates, at most MaxCandidates.
func Candidates(extra []string, withDefaults bool) ([]Candidate, error) {
	var list []Candidate
	seen := map[Candidate]bool{}
	add := func(raw string) error {
		candidate, err := ParseCandidate(raw)
		if err != nil {
			return err
		}
		if !seen[candidate] && len(list) < MaxCandidates {
			seen[candidate] = true
			list = append(list, candidate)
		}
		return nil
	}
	if len(extra) > MaxCandidates {
		return nil, fmt.Errorf("at most %d candidates", MaxCandidates)
	}
	for _, raw := range extra {
		if err := add(raw); err != nil {
			return nil, err
		}
	}
	if withDefaults {
		for _, raw := range DefaultCandidates {
			_ = add(raw)
		}
	}
	if len(list) == 0 {
		return nil, errors.New("no candidates to probe")
	}
	return list, nil
}

// ProbeResult is the Agent's answer to reality.probe (anix-agent
// realityprobe.Result).
type ProbeResult struct {
	ServerName     string     `json:"server_name"`
	Port           int        `json:"port"`
	Address        string     `json:"address,omitempty"`
	Verdict        string     `json:"verdict"`
	Failures       []string   `json:"failures,omitempty"`
	Error          string     `json:"error,omitempty"`
	ConnectMS      int64      `json:"connect_ms,omitempty"`
	HandshakeMS    int64      `json:"handshake_ms,omitempty"`
	TLSVersion     string     `json:"tls_version,omitempty"`
	ALPN           string     `json:"alpn,omitempty"`
	Curve          string     `json:"curve,omitempty"`
	CertNames      []string   `json:"cert_names,omitempty"`
	CertIssuer     string     `json:"cert_issuer,omitempty"`
	CertNotAfter   *time.Time `json:"cert_not_after,omitempty"`
	CertChainBytes int        `json:"cert_chain_bytes,omitempty"`
	OCSPStapled    bool       `json:"ocsp_stapled,omitempty"`
	HTTPStatus     int        `json:"http_status,omitempty"`
	RedirectHost   string     `json:"redirect_host,omitempty"`
	ServerHeader   string     `json:"server_header,omitempty"`
	CDN            string     `json:"cdn,omitempty"`
	// RealityMS is the Agent's loopback REALITY handshake through the target.
	RealityMS int64 `json:"reality_ms,omitempty"`
}

// VerdictOK is a target that passed every hard requirement; VerdictFailed
// one that did not; VerdictError a probe that gave no answer (offline,
// timeout), which is not stored.
const (
	VerdictOK     = "ok"
	VerdictFailed = "failed"
	VerdictError  = "error"
)

// The weights of Score (the design's table). Kept in one place to be tuned.
const (
	weightSameNarrowPrefix = 30 // same IPv4 /24 or IPv6 /48 as the node
	weightSameWidePrefix   = 15 // same IPv4 /16 or IPv6 /32
	weightLatencyFast      = 30 // handshake within 20 ms
	weightLatencyNear      = 20 // within 50 ms
	weightLatencyFair      = 10 // within 150 ms
	weightPostQuantum      = 10 // X25519MLKEM768
	weightOCSP             = 5
	weightLargeChain       = 5   // chain of 3500 bytes or more (mldsa65 needs it)
	penaltyCDN             = -30 // the node would forward for the CDN
	penaltyPerReuse        = -5  // per other node already using the target
	largeChainBytes        = 3500
	postQuantumCurve       = "X25519MLKEM768"
)

// ScoreInput is what Score weighs besides the probe.
type ScoreInput struct {
	// NodeAddr is the node's public address; invalid when unknown.
	NodeAddr netip.Addr
	// UsedBy is how many other nodes use the target now.
	UsedBy int
}

// Score ranks a target that passed, 0 to 100; a failed one scores 0.
func Score(result ProbeResult, input ScoreInput) int {
	if result.Verdict != VerdictOK {
		return 0
	}
	score := 0
	if target, err := netip.ParseAddrPort(result.Address); err == nil && input.NodeAddr.IsValid() {
		switch {
		case samePrefix(target.Addr(), input.NodeAddr, 24, 48):
			score += weightSameNarrowPrefix
		case samePrefix(target.Addr(), input.NodeAddr, 16, 32):
			score += weightSameWidePrefix
		}
	}
	switch handshake := result.HandshakeMS; {
	case handshake <= 20:
		score += weightLatencyFast
	case handshake <= 50:
		score += weightLatencyNear
	case handshake <= 150:
		score += weightLatencyFair
	}
	if result.Curve == postQuantumCurve {
		score += weightPostQuantum
	}
	if result.OCSPStapled {
		score += weightOCSP
	}
	if result.CertChainBytes >= largeChainBytes {
		score += weightLargeChain
	}
	if result.CDN != "" {
		score += penaltyCDN
	}
	score += penaltyPerReuse * input.UsedBy
	return min(max(score, 0), 100)
}

func samePrefix(a, b netip.Addr, bits4, bits6 int) bool {
	a, b = a.Unmap(), b.Unmap()
	if a.Is4() != b.Is4() {
		return false
	}
	bits := bits6
	if a.Is4() {
		bits = bits4
	}
	pa, err := a.Prefix(bits)
	if err != nil {
		return false
	}
	return pa.Contains(b)
}

// Prober runs reality.probe on a node (kernelnodeops.RealityProbes).
type Prober interface {
	Probe(ctx context.Context, node agentcontrol.AgentNode, serverName string, port int, timeout time.Duration) (json.RawMessage, error)
}

// Target is one candidate's latest probe, as answered.
type Target struct {
	ServerName string       `json:"server_name"`
	Port       int          `json:"port"`
	Verdict    string       `json:"verdict"`
	Score      int          `json:"score"`
	UsedBy     int          `json:"used_by"`
	ProbedAt   *time.Time   `json:"probed_at,omitempty"`
	Error      string       `json:"error,omitempty"`
	Result     *ProbeResult `json:"result,omitempty"`
}

// CurrentTarget is the target one of the node's Reality protocols uses now.
type CurrentTarget struct {
	ProtocolID uint   `json:"protocol_id"`
	Name       string `json:"name"`
	ServerName string `json:"server_name"`
	Dest       string `json:"dest,omitempty"`
}

// Report is a node's targets: what its Reality protocols use now, the
// candidates ranked (passed first by score, then failed), and the best one.
type Report struct {
	NodeID      uint            `json:"node_id"`
	NodeAddress string          `json:"node_address,omitempty"`
	Current     []CurrentTarget `json:"current"`
	Recommended *Target         `json:"recommended"`
	Targets     []Target        `json:"targets"`
}

// Service runs and reads the probes of the proxy nodes.
type Service struct {
	DB     *gorm.DB
	Prober Prober
	// Lookup resolves a node host that is a name; nil uses the default
	// resolver.
	Lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// Now defaults to time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Run probes the candidates from node nodeID (Concurrency at a time),
// stores each answer and answers the node's report with this run's
// errors. A probe that gave no answer keeps the candidate's earlier row.
func (s *Service) Run(ctx context.Context, nodeID uint, candidates []Candidate, timeout time.Duration) (*Report, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(max(timeout, MinTimeout), MaxTimeout)
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(nodeID)} // #nosec G115 -- node ids are uint32 on the wire.

	errorsByTarget := make(map[Candidate]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, Concurrency)
	answers := make(map[Candidate]ProbeResult, len(candidates))
	for _, candidate := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				errorsByTarget[candidate] = ctx.Err().Error()
				mu.Unlock()
				return
			}
			defer func() { <-slots }()
			raw, err := s.Prober.Probe(ctx, node, candidate.ServerName, candidate.Port, timeout)
			var result ProbeResult
			if err == nil {
				err = json.Unmarshal(raw, &result)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errorsByTarget[candidate] = err.Error()
				return
			}
			result.ServerName, result.Port = candidate.ServerName, candidate.Port
			if result.Verdict != VerdictOK {
				result.Verdict = VerdictFailed
			}
			answers[candidate] = result
		}()
	}
	wg.Wait()

	nodeAddr := s.nodeAddr(ctx, nodeID)
	usedBy, err := s.usage(nodeID)
	if err != nil {
		return nil, err
	}
	probedAt := s.now().UTC()
	for candidate, result := range answers {
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		row := model.RealityTargetProbe{
			NodeID: nodeID, ServerName: candidate.ServerName, Port: candidate.Port, Verdict: result.Verdict,
			Score: Score(result, ScoreInput{NodeAddr: nodeAddr, UsedBy: usedBy[candidate.ServerName]}), Result: string(encoded), ProbedAt: probedAt,
		}
		err = s.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "node_id"}, {Name: "server_name"}, {Name: "port"}},
			DoUpdates: clause.AssignmentColumns([]string{"verdict", "score", "result", "probed_at", "updated_at"}),
		}).Create(&row).Error
		if err != nil {
			return nil, fmt.Errorf("store the probe of %s: %w", candidate, err)
		}
	}
	report, err := s.Report(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	for candidate, message := range errorsByTarget {
		found := false
		for i := range report.Targets {
			if report.Targets[i].ServerName == candidate.ServerName && report.Targets[i].Port == candidate.Port {
				report.Targets[i].Error, found = message, true
			}
		}
		if !found {
			report.Targets = append(report.Targets, Target{ServerName: candidate.ServerName, Port: candidate.Port, Verdict: VerdictError, Error: message, UsedBy: usedBy[candidate.ServerName]})
		}
	}
	sortTargets(report.Targets)
	return report, nil
}

// Report answers the node's stored probes, ranked, with its current
// targets and the recommendation.
func (s *Service) Report(ctx context.Context, nodeID uint) (*Report, error) {
	var rows []model.RealityTargetProbe
	if err := s.DB.WithContext(ctx).Where("node_id = ?", nodeID).Find(&rows).Error; err != nil {
		return nil, err
	}
	usedBy, err := s.usage(nodeID)
	if err != nil {
		return nil, err
	}
	current, err := s.current(nodeID)
	if err != nil {
		return nil, err
	}
	report := &Report{NodeID: nodeID, Current: current, Targets: make([]Target, 0, len(rows))}
	if addr := s.nodeAddr(ctx, nodeID); addr.IsValid() {
		report.NodeAddress = addr.String()
	}
	for _, row := range rows {
		target := Target{ServerName: row.ServerName, Port: row.Port, Verdict: row.Verdict, Score: row.Score, UsedBy: usedBy[row.ServerName]}
		probedAt := row.ProbedAt.UTC()
		target.ProbedAt = &probedAt
		var result ProbeResult
		if json.Unmarshal([]byte(row.Result), &result) == nil {
			target.Result = &result
		}
		report.Targets = append(report.Targets, target)
	}
	sortTargets(report.Targets)
	if len(report.Targets) > 0 && report.Targets[0].Verdict == VerdictOK {
		best := report.Targets[0]
		report.Recommended = &best
	}
	return report, nil
}

// sortTargets puts passed targets first by score, then failed, then
// errors, each by name.
func sortTargets(targets []Target) {
	rank := map[string]int{VerdictOK: 0, VerdictFailed: 1, VerdictError: 2}
	sort.SliceStable(targets, func(i, j int) bool {
		a, b := targets[i], targets[j]
		if rank[a.Verdict] != rank[b.Verdict] {
			return rank[a.Verdict] < rank[b.Verdict]
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Result != nil && b.Result != nil && a.Result.HandshakeMS != b.Result.HandshakeMS {
			return a.Result.HandshakeMS < b.Result.HandshakeMS
		}
		if a.ServerName != b.ServerName {
			return a.ServerName < b.ServerName
		}
		return a.Port < b.Port
	})
}

// nodeAddr is the node's public address: the one its Agent last joined
// from, else its host when that is an address or resolves to one.
func (s *Service) nodeAddr(ctx context.Context, nodeID uint) netip.Addr {
	var node model.Node
	if err := s.DB.WithContext(ctx).Select("id", "host", "last_public_address").First(&node, nodeID).Error; err != nil {
		return netip.Addr{}
	}
	for _, raw := range []string{node.LastPublicAddress, node.Host} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if addrPort, err := netip.ParseAddrPort(raw); err == nil {
			return addrPort.Addr().Unmap()
		}
		if addr, err := netip.ParseAddr(strings.Trim(raw, "[]")); err == nil {
			return addr.Unmap()
		}
	}
	if host := strings.TrimSpace(node.Host); validHostName(strings.ToLower(host)) {
		lookup := s.Lookup
		if lookup == nil {
			lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
				return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			}
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if addrs, err := lookup(lookupCtx, host); err == nil && len(addrs) > 0 {
			return addrs[0].Unmap()
		}
	}
	return netip.Addr{}
}

// realityNames reads the target names of a protocol's Reality settings:
// server_name, server_names and dest's host.
func realityNames(settings *string) (serverName, dest string, names []string) {
	if settings == nil || strings.TrimSpace(*settings) == "" {
		return "", "", nil
	}
	var doc map[string]any
	if json.Unmarshal([]byte(*settings), &doc) != nil {
		return "", "", nil
	}
	seen := map[string]bool{}
	addName := func(name string) {
		name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	serverName, _ = doc["server_name"].(string)
	addName(serverName)
	if list, ok := doc["server_names"].([]any); ok {
		for _, item := range list {
			if name, ok := item.(string); ok {
				addName(name)
				if serverName == "" {
					serverName = name
				}
			}
		}
	}
	dest, _ = doc["dest"].(string)
	if host, _, err := net.SplitHostPort(dest); err == nil {
		addName(host)
	} else if dest != "" {
		addName(dest)
	}
	return serverName, dest, names
}

// realityProtocols are the Reality protocols (tls 2) of all nodes.
func (s *Service) realityProtocols() ([]model.NodeProtocol, error) {
	var protocols []model.NodeProtocol
	err := s.DB.Select("id", "node_id", "name", "reality_settings").Where("tls = ?", 2).Order("id").Find(&protocols).Error
	return protocols, err
}

// usage counts, per target name, the other nodes whose Reality protocols
// use it.
func (s *Service) usage(nodeID uint) (map[string]int, error) {
	protocols, err := s.realityProtocols()
	if err != nil {
		return nil, err
	}
	nodes := map[string]map[uint]bool{}
	for _, protocol := range protocols {
		if protocol.NodeID == nodeID {
			continue
		}
		_, _, names := realityNames(protocol.RealitySettings)
		for _, name := range names {
			if nodes[name] == nil {
				nodes[name] = map[uint]bool{}
			}
			nodes[name][protocol.NodeID] = true
		}
	}
	counts := make(map[string]int, len(nodes))
	for name, set := range nodes {
		counts[name] = len(set)
	}
	return counts, nil
}

// current lists the node's Reality protocols and their targets.
func (s *Service) current(nodeID uint) ([]CurrentTarget, error) {
	protocols, err := s.realityProtocols()
	if err != nil {
		return nil, err
	}
	current := []CurrentTarget{}
	for _, protocol := range protocols {
		if protocol.NodeID != nodeID {
			continue
		}
		serverName, dest, _ := realityNames(protocol.RealitySettings)
		current = append(current, CurrentTarget{ProtocolID: protocol.ID, Name: protocol.Name, ServerName: serverName, Dest: dest})
	}
	return current, nil
}

// ErrNotRealityProtocol refuses an apply to a protocol that is not one of
// the node's Reality protocols; ErrTargetNotProbed one to a target the node
// has not probed with verdict ok (unless forced).
var (
	ErrNotRealityProtocol = errors.New("the protocol is not a Reality protocol of this node")
	ErrTargetNotProbed    = errors.New("this node has no passing probe of the target: probe it first")
)

// ApplyRequest names the target to write into one Reality protocol.
type ApplyRequest struct {
	ProtocolID uint
	Target     Candidate
	// Force applies a target the node has not probed with verdict ok.
	Force bool
}

// ApplyResult is the protocol's target before and after.
type ApplyResult struct {
	Previous CurrentTarget `json:"previous"`
	Applied  CurrentTarget `json:"applied"`
}

// UpdateProtocol writes protocol columns the way the admin protocol route
// does (service.NodeService.UpdateProtocol): validated, secrets kept.
type UpdateProtocol func(protocolID uint, updates map[string]any) error

// Apply writes target into a Reality protocol of node nodeID: reality
// settings dest (host:port), server_name and, where the settings list them,
// server_names; and tls settings server_name, which the subscription links
// carry as the SNI. The rest of the settings, the private key included,
// stays as stored. Clients pick up the new SNI with their next subscription
// refresh.
func (s *Service) Apply(ctx context.Context, nodeID uint, request ApplyRequest, update UpdateProtocol) (*ApplyResult, error) {
	var protocol model.NodeProtocol
	err := s.DB.WithContext(ctx).Where("id = ? AND node_id = ?", request.ProtocolID, nodeID).First(&protocol).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && protocol.TLS != 2 {
		return nil, ErrNotRealityProtocol
	}
	if err != nil {
		return nil, err
	}
	if !request.Force {
		var passed int64
		err := s.DB.WithContext(ctx).Model(&model.RealityTargetProbe{}).
			Where("node_id = ? AND server_name = ? AND port = ? AND verdict = ?", nodeID, request.Target.ServerName, request.Target.Port, VerdictOK).
			Count(&passed).Error
		if err != nil {
			return nil, err
		}
		if passed == 0 {
			return nil, ErrTargetNotProbed
		}
	}
	nodesecrets.ResolveProtocol(s.DB, &protocol)
	serverName, dest, _ := realityNames(protocol.RealitySettings)
	result := &ApplyResult{Previous: CurrentTarget{ProtocolID: protocol.ID, Name: protocol.Name, ServerName: serverName, Dest: dest}}

	reality, err := settingsObject(protocol.RealitySettings, "reality_settings")
	if err != nil {
		return nil, err
	}
	reality["dest"] = request.Target.String()
	reality["server_name"] = request.Target.ServerName
	if _, listed := reality["server_names"]; listed {
		reality["server_names"] = []string{request.Target.ServerName}
	}
	tlsSettings, err := settingsObject(protocol.TLSSettings, "tls_settings")
	if err != nil {
		return nil, err
	}
	tlsSettings["server_name"] = request.Target.ServerName
	realityJSON, err := json.Marshal(reality)
	if err != nil {
		return nil, err
	}
	tlsJSON, err := json.Marshal(tlsSettings)
	if err != nil {
		return nil, err
	}
	if err := update(protocol.ID, map[string]any{"reality_settings": string(realityJSON), "tls_settings": string(tlsJSON)}); err != nil {
		return nil, err
	}
	result.Applied = CurrentTarget{ProtocolID: protocol.ID, Name: protocol.Name, ServerName: request.Target.ServerName, Dest: request.Target.String()}
	return result, nil
}

// settingsObject decodes a protocol's JSON settings column; empty is {}.
func settingsObject(column *string, name string) (map[string]any, error) {
	object := map[string]any{}
	if column == nil || strings.TrimSpace(*column) == "" {
		return object, nil
	}
	if err := json.Unmarshal([]byte(*column), &object); err != nil || object == nil {
		return nil, fmt.Errorf("the protocol's %s is not a JSON object", name)
	}
	return object, nil
}
