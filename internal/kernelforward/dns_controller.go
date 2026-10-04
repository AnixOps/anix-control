package kernelforward

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/forwardddns"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The entry HA controller (forward-sdk.md section 7.4, L2). Every
// DNSEvaluateInterval the singleton worker evaluates each binding's entry
// nodes and keeps the binding's records on the addresses of the nodes in
// rotation:
//
//   - A node is healthy when it is in the inventory with a public address
//     of a published family, its latest forward report arrived within
//     DNSReportStaleAfter, its Agent is online (a session in this process,
//     or a stream seen within DNSPresenceStaleAfter by any process), the
//     report has no hop error on the route's entry hop, and not every
//     upstream of the entry hop is down. A node still converging (its
//     report behind its desired generation, or not applied) is neither
//     healthy nor unhealthy: its streaks do not move.
//   - Hysteresis: a node leaves rotation after DNSRemoveAfter unhealthy
//     evaluations in a row and joins after DNSAddAfter healthy ones; before
//     the binding's first publication a healthy node joins at once.
//   - Never empty: when no node is in rotation for a record type, the last
//     published values are kept, the binding is degraded, a warning is
//     logged and the change audited.
//   - Provider calls are rate limited per provider (a token bucket of
//     DNSProviderBurst calls refilled one per DNSProviderRefill), spaced at
//     least DNSMinPublishInterval apart per binding, retried with an
//     exponential back-off from DNSRetryBase to DNSRetryMax after a
//     failure, and repeated every DNSResyncInterval to undo drift.
//   - Every publication is audited (system/forward-dns) and counted.
//
// The state lives in the binding and node tables, so a new lease holder
// carries on where the last one stopped.
const (
	DNSEvaluateInterval    = 10 * time.Second
	DNSRemoveAfter         = 3
	DNSAddAfter            = 3
	DNSReportStaleAfter    = 150 * time.Second
	DNSPresenceStaleAfter  = 180 * time.Second
	DNSMinPublishInterval  = 30 * time.Second
	DNSResyncInterval      = time.Hour
	DNSRetryBase           = 30 * time.Second
	DNSRetryMax            = 15 * time.Minute
	DNSProviderBurst       = 10
	DNSProviderRefill      = 6 * time.Second
	dnsCallTimeout         = 30 * time.Second
	dnsAuditActor          = "system/forward-dns"
	dnsMaxErrorText        = 500
	dnsAuditTargetBinding  = "forward_dns_binding"
	dnsReasonHealthy       = "healthy"
	dnsReasonConverging    = "converging"
	dnsReasonNoInventory   = "not_in_inventory"
	dnsReasonNoAddress     = "no_address"
	dnsReasonNeverReported = "never_reported"
	dnsReasonReportStale   = "report_stale"
	dnsReasonOffline       = "offline"
	dnsReasonHopError      = "hop_error"
	dnsReasonUpstreamsDown = "upstreams_down"
)

// Binding states (RouteDnsStatus.state).
const (
	dnsStateUnbound          = "unbound"
	dnsStatePaused           = "paused"
	dnsStatePending          = "pending"
	dnsStateOK               = "ok"
	dnsStateDegraded         = "degraded"
	dnsStateError            = "error"
	dnsStateRateLimited      = "rate_limited"
	dnsStateRouteMissing     = "route_missing"
	dnsStateHostnameMismatch = "hostname_mismatch"
)

// EntryHA is the entry HA controller on one database.
type EntryHA struct {
	Service *Service
	// LocalSession reports whether this process holds node's Agent
	// Control session; nil when it holds none.
	LocalSession func(node agentcontrol.AgentNode) bool

	mu        sync.Mutex
	providers map[uint64]cachedProvider
	limiter   tokenBuckets
}

type cachedProvider struct {
	updatedAt time.Time
	provider  forwardddns.Provider
}

// Run evaluates every DNSEvaluateInterval until ctx ends. Only the
// singleton-worker lease holder runs it.
func (e *EntryHA) Run(ctx context.Context) {
	ticker := time.NewTicker(DNSEvaluateInterval)
	defer ticker.Stop()
	for {
		if err := e.Evaluate(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("forward entry HA evaluation failed", "component", "kernel-forward", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// dnsProvider builds the client of a stored provider.
func (s *Service) dnsProvider(db *gorm.DB, id uint64) (forwardddns.Provider, error) {
	row, err := loadProvider(db, id)
	if err != nil {
		return nil, err
	}
	return s.buildDNSProvider(row)
}

func (s *Service) buildDNSProvider(row model.KernelForwardDNSProvider) (forwardddns.Provider, error) {
	sealer, err := s.credentialSealer()
	if err != nil {
		return nil, err
	}
	credentials, err := sealer.open(row)
	if err != nil {
		return nil, err
	}
	if s.NewDNSProvider != nil {
		return s.NewDNSProvider(row.Kind, decodeMap(row.ConfigJSON), credentials)
	}
	return forwardddns.New(row.Kind, decodeMap(row.ConfigJSON), credentials, forwardddns.Options{})
}

func (e *EntryHA) provider(db *gorm.DB, id uint64) (forwardddns.Provider, error) {
	row, err := loadProvider(db, id)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	cached, ok := e.providers[id]
	e.mu.Unlock()
	if ok && cached.updatedAt.Equal(row.UpdatedAt) {
		return cached.provider, nil
	}
	provider, err := e.Service.buildDNSProvider(row)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	if e.providers == nil {
		e.providers = map[uint64]cachedProvider{}
	}
	e.providers[id] = cachedProvider{updatedAt: row.UpdatedAt, provider: provider}
	e.mu.Unlock()
	return provider, nil
}

// ---------------------------------------------------------------------------
// Health

// verdict is one evaluation of an entry node.
type verdict int

const (
	verdictNeutral verdict = iota
	verdictGood
	verdictBad
)

// entryFacts are what one evaluation knows of an entry node.
type entryFacts struct {
	InInventory bool
	// Addresses are its public addresses of the published families.
	Addresses []string
	// Report is its latest report; ReceivedAt when it arrived.
	Report     *forwardv1.NodeForwardReport
	ReceivedAt time.Time
	// Desired is its desired generation.
	Desired uint64
	Online  bool
}

// evaluateEntry judges an entry node of routeID at now.
func evaluateEntry(routeID string, facts entryFacts, now time.Time) (verdict, string) {
	switch {
	case !facts.InInventory:
		return verdictBad, dnsReasonNoInventory
	case len(facts.Addresses) == 0:
		return verdictBad, dnsReasonNoAddress
	case facts.Report == nil:
		return verdictBad, dnsReasonNeverReported
	case now.Sub(facts.ReceivedAt) > DNSReportStaleAfter:
		return verdictBad, dnsReasonReportStale
	case !facts.Online:
		return verdictBad, dnsReasonOffline
	}
	for _, hopErr := range facts.Report.GetErrors() {
		if hopErr.GetRouteId() == routeID && hopErr.GetHopIndex() == 0 {
			return verdictBad, dnsReasonHopError
		}
	}
	upstreams, down := 0, 0
	for _, health := range facts.Report.GetHealth() {
		if health.GetRouteId() != routeID || health.GetHopIndex() != 0 {
			continue
		}
		upstreams++
		if state := health.GetState(); state == forwardv1.HealthState_HEALTH_STATE_UNHEALTHY || state == forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN {
			down++
		}
	}
	if upstreams > 0 && down == upstreams {
		return verdictBad, dnsReasonUpstreamsDown
	}
	if facts.Report.GetGeneration() < facts.Desired || !facts.Report.GetApplied() {
		return verdictNeutral, dnsReasonConverging
	}
	return verdictGood, dnsReasonHealthy
}

// step moves a node's streaks and rotation by one evaluation; bootstrap
// (nothing published yet) lets a healthy node join at once.
func step(node *model.KernelForwardDNSNode, v verdict, bootstrap bool) {
	switch v {
	case verdictGood:
		node.Healthy = true
		node.GoodStreak++
		node.BadStreak = 0
		if !node.InRotation && (bootstrap || node.GoodStreak >= DNSAddAfter) {
			node.InRotation = true
		}
	case verdictBad:
		node.Healthy = false
		node.BadStreak++
		node.GoodStreak = 0
		if node.InRotation && node.BadStreak >= DNSRemoveAfter {
			node.InRotation = false
		}
	}
}

// publicAddresses keeps the global unicast addresses of the record types,
// in their canonical form, in order.
func publicAddresses(addresses []string, types []string) []string {
	wantV4, wantV6 := false, false
	for _, t := range types {
		wantV4 = wantV4 || t == forwardddns.TypeA
		wantV6 = wantV6 || t == forwardddns.TypeAAAA
	}
	var out []string
	seen := map[string]bool{}
	for _, raw := range addresses {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		addr = addr.Unmap()
		if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.Zone() != "" {
			continue
		}
		if (addr.Is4() && !wantV4) || (addr.Is6() && !wantV6) || seen[addr.String()] {
			continue
		}
		seen[addr.String()] = true
		out = append(out, addr.String())
	}
	return out
}

func addressType(address string) string {
	if addr, err := netip.ParseAddr(address); err == nil && addr.Is6() {
		return forwardddns.TypeAAAA
	}
	return forwardddns.TypeA
}

// ---------------------------------------------------------------------------
// Rate limit

// tokenBuckets limits provider calls per provider.
type tokenBuckets struct {
	mu      sync.Mutex
	buckets map[uint64]*bucket
}

type bucket struct {
	tokens float64
	at     time.Time
}

// take spends n tokens of provider id at now, or none when fewer remain.
func (t *tokenBuckets) take(id uint64, n int, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.buckets == nil {
		t.buckets = map[uint64]*bucket{}
	}
	b, ok := t.buckets[id]
	if !ok {
		b = &bucket{tokens: DNSProviderBurst, at: now}
		t.buckets[id] = b
	}
	if elapsed := now.Sub(b.at); elapsed > 0 {
		b.tokens = min(DNSProviderBurst, b.tokens+float64(elapsed)/float64(DNSProviderRefill))
		b.at = now
	}
	if b.tokens < float64(n) {
		return false
	}
	b.tokens -= float64(n)
	return true
}

// ---------------------------------------------------------------------------
// Metrics

// dnsCounters counts provider calls by provider kind and result.
type dnsCounters struct {
	mu     sync.Mutex
	counts map[[2]string]uint64
}

var dnsMetrics dnsCounters

func (c *dnsCounters) add(kind, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[[2]string]uint64{}
	}
	c.counts[[2]string{kind, result}]++
}

func (c *dnsCounters) snapshot() map[[2]string]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[[2]string]uint64, len(c.counts))
	for key, value := range c.counts {
		out[key] = value
	}
	return out
}

// writeDNSPrometheus renders the entry HA gauges and counters.
func writeDNSPrometheus(body *strings.Builder, db *gorm.DB) {
	var rows []struct {
		State string
		Count int64
	}
	if err := db.Model(&model.KernelForwardDNSBinding{}).Select("state, count(*) as count").Group("state").Scan(&rows).Error; err != nil {
		return
	}
	byState := map[string]int64{}
	for _, row := range rows {
		byState[row.State] += row.Count
	}
	name := "anixops_forward_dns_bindings"
	body.WriteString("# HELP " + name + " Forward entry HA DNS bindings by state (ok, degraded: no healthy entry and the last records kept, error, pending, paused, rate_limited, route_missing, hostname_mismatch).\n")
	body.WriteString("# TYPE " + name + " gauge\n")
	for _, state := range []string{dnsStateOK, dnsStateDegraded, dnsStateError, dnsStatePending, dnsStatePaused, dnsStateRateLimited, dnsStateRouteMissing, dnsStateHostnameMismatch} {
		body.WriteString(name + `{state="` + state + `"} ` + strconv.FormatInt(byState[state], 10) + "\n")
	}
	name = "anixops_forward_dns_updates_total"
	body.WriteString("# HELP " + name + " DNS provider calls of forward entry HA in this process, by provider kind and result (published, deleted, error, rate_limited).\n")
	body.WriteString("# TYPE " + name + " counter\n")
	counts := dnsMetrics.snapshot()
	keys := make([][2]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
	for _, key := range keys {
		body.WriteString(name + `{provider="` + key[0] + `",result="` + key[1] + `"} ` + strconv.FormatUint(counts[key], 10) + "\n")
	}
}

// ---------------------------------------------------------------------------
// Audit

func auditDNS(tx *gorm.DB, action string, binding model.KernelForwardDNSBinding, detail map[string]any) error {
	content := map[string]any{"binding_id": binding.ID, "route_id": binding.RouteID, "zone": binding.Zone, "record_name": binding.RecordName}
	for key, value := range detail {
		content[key] = value
	}
	encoded, _ := json.Marshal(content)
	return service.NewOperationLogService(tx).Record(&service.OperationLogInput{
		Username: dnsAuditActor, Action: action, Module: "forward", TargetType: dnsAuditTargetBinding, Content: string(encoded),
	})
}

// ---------------------------------------------------------------------------
// Evaluation

// evaluation is what one pass reads once for every binding.
type evaluation struct {
	now       time.Time
	inventory map[string][]string
	reports   map[string]model.KernelForwardNodeReport
	desired   map[string]uint64
	lastSeen  map[string]time.Time
}

// Evaluate runs one pass over every binding.
func (e *EntryHA) Evaluate(ctx context.Context) error {
	if e == nil || e.Service == nil {
		return errors.New("kernel forward: entry HA has no service")
	}
	db, err := e.Service.db(ctx)
	if err != nil {
		return err
	}
	var bindings []model.KernelForwardDNSBinding
	if err := db.Order("id").Find(&bindings).Error; err != nil {
		return fmt.Errorf("kernel forward: load dns bindings: %w", err)
	}
	if len(bindings) == 0 {
		return nil
	}
	pass, err := e.load(db)
	if err != nil {
		return err
	}
	var failed error
	for _, binding := range bindings {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := e.evaluateBinding(ctx, db, pass, binding); err != nil {
			slog.Warn("forward entry HA binding evaluation failed", "component", "kernel-forward", "binding_id", binding.ID, "route_id", binding.RouteID, "error", err)
			failed = err
		}
	}
	return failed
}

func (e *EntryHA) load(db *gorm.DB) (*evaluation, error) {
	pass := &evaluation{
		now: e.Service.now(), inventory: map[string][]string{}, reports: map[string]model.KernelForwardNodeReport{},
		desired: map[string]uint64{}, lastSeen: map[string]time.Time{},
	}
	inv, err := loadInventory(db)
	if err != nil {
		return nil, err
	}
	for _, info := range inv.nodes {
		pass.inventory[info.GetNodeRef()] = info.GetAddresses()
	}
	var reports []model.KernelForwardNodeReport
	if err := db.Find(&reports).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load reports: %w", err)
	}
	for _, report := range reports {
		pass.reports[report.NodeRef] = report
	}
	var states []model.KernelForwardNodeState
	if err := db.Select("node_ref", "generation").Find(&states).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load states: %w", err)
	}
	for _, state := range states {
		pass.desired[state.NodeRef] = state.Generation
	}
	var seen []model.AgentTransport
	if err := db.Where("transport IN ?", []string{model.AgentTransportMTLSStream, model.AgentTransportAPIKeyStream}).Find(&seen).Error; err != nil {
		// A database without the transport table relies on reports.
		slog.Debug("kernel forward: agent transports could not be read", "component", "kernel-forward", "error", err)
	}
	for _, row := range seen {
		ref := agentcontrol.AgentNode{Kind: row.NodeKind, ID: uint32(min(row.NodeID, 1<<32-1))}.String() // #nosec G115 -- bounded above.
		if row.LastSeenAt.After(pass.lastSeen[ref]) {
			pass.lastSeen[ref] = row.LastSeenAt
		}
	}
	return pass, nil
}

// online reports whether ref's Agent is connected to some Control process.
func (e *EntryHA) online(pass *evaluation, ref string) bool {
	if e.LocalSession != nil {
		if node, err := agentcontrol.ParseAgentNode(ref); err == nil && e.LocalSession(node) {
			return true
		}
	}
	seen, ok := pass.lastSeen[ref]
	return ok && pass.now.Sub(seen) <= DNSPresenceStaleAfter
}

func (e *EntryHA) facts(pass *evaluation, ref string, types []string) entryFacts {
	addresses, inInventory := pass.inventory[ref]
	facts := entryFacts{InInventory: inInventory, Addresses: publicAddresses(addresses, types), Desired: pass.desired[ref], Online: e.online(pass, ref)}
	if row, ok := pass.reports[ref]; ok {
		report := &forwardv1.NodeForwardReport{}
		if err := jsonRead.Unmarshal([]byte(row.ReportJSON), report); err == nil {
			// The row's columns are authoritative for the generation.
			report.Generation, report.Applied = row.Generation, row.Applied
			facts.Report, facts.ReceivedAt = report, row.ReceivedAt
		}
	}
	return facts
}

// saveState writes the controller's columns of a binding, leaving the
// fields an administrator edits alone.
func saveState(db *gorm.DB, binding model.KernelForwardDNSBinding) error {
	return db.Model(&model.KernelForwardDNSBinding{}).Where("id = ?", binding.ID).UpdateColumns(map[string]any{
		"state": binding.State, "published_json": binding.PublishedJSON, "published_at": binding.PublishedAt,
		"published_ttl": binding.PublishedTTL, "desired_json": binding.DesiredJSON, "evaluated_at": binding.EvaluatedAt,
		"degraded": binding.Degraded, "failures": binding.Failures, "last_error": binding.LastError,
		"last_error_at": binding.LastErrorAt, "next_attempt_at": binding.NextAttemptAt,
	}).Error
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func clipError(err error) string {
	text := err.Error()
	if len(text) > dnsMaxErrorText {
		text = text[:dnsMaxErrorText] + "..."
	}
	return text
}

func (e *EntryHA) evaluateBinding(ctx context.Context, db *gorm.DB, pass *evaluation, binding model.KernelForwardDNSBinding) error {
	now := pass.now
	binding.EvaluatedAt = &now
	var routes []model.KernelForwardRoute
	if err := db.Where("id = ?", binding.RouteID).Limit(1).Find(&routes).Error; err != nil {
		return err
	}
	if len(routes) == 0 {
		binding.State = dnsStateRouteMissing
		return saveState(db, binding)
	}
	route, err := decodeRoute(routes[0])
	if err != nil {
		return err
	}
	if binding.Paused || route.GetPaused() || routes[0].Enforced != "" {
		binding.State = dnsStatePaused
		return saveState(db, binding)
	}
	if binding.Mode == DNSModeDDNS && forwardddns.NormalizeName(route.GetListen().GetEntryHostname()) != binding.RecordName {
		binding.State = dnsStateHostnameMismatch
		return saveState(db, binding)
	}
	var entry []string
	if hops := route.GetHops(); len(hops) > 0 {
		entry = hops[0].GetNodeRefs()
	}
	types := bindingTypes(binding)

	// Health and hysteresis.
	var stored []model.KernelForwardDNSNode
	if err := db.Where("binding_id = ?", binding.ID).Find(&stored).Error; err != nil {
		return err
	}
	byRef := map[string]model.KernelForwardDNSNode{}
	for _, node := range stored {
		byRef[node.NodeRef] = node
	}
	bootstrap := binding.PublishedAt == nil
	desired := map[string][]string{}
	var nodes []model.KernelForwardDNSNode
	for _, ref := range entry {
		node, ok := byRef[ref]
		if !ok {
			node = model.KernelForwardDNSNode{BindingID: binding.ID, NodeRef: ref}
		}
		facts := e.facts(pass, ref, types)
		v, reason := evaluateEntry(binding.RouteID, facts, now)
		step(&node, v, bootstrap)
		node.Reason, node.EvaluatedAt = reason, now
		nodes = append(nodes, node)
		if node.InRotation {
			for _, address := range facts.Addresses {
				desired[addressType(address)] = append(desired[addressType(address)], address)
			}
		}
		delete(byRef, ref)
	}
	for t := range desired {
		sort.Strings(desired[t])
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for ref := range byRef {
			if err := tx.Delete(&model.KernelForwardDNSNode{}, "binding_id = ? AND node_ref = ?", binding.ID, ref).Error; err != nil {
				return err
			}
		}
		if len(nodes) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&nodes).Error
	}); err != nil {
		return fmt.Errorf("kernel forward: save dns nodes: %w", err)
	}

	// What to publish.
	published := decodePublished(binding.PublishedJSON)
	wanted := map[string]bool{}
	var sets []forwardddns.RecordSet
	degraded := false
	for _, t := range types {
		wanted[t] = true
		if len(desired[t]) == 0 {
			// Never publish an empty set: keep what is published.
			if len(published[t]) > 0 {
				degraded = true
			}
			continue
		}
		resync := binding.PublishedAt == nil || now.Sub(*binding.PublishedAt) >= DNSResyncInterval
		if resync || binding.PublishedTTL != binding.TTL || !sameStrings(desired[t], published[t]) {
			sets = append(sets, forwardddns.RecordSet{Zone: binding.Zone, Name: binding.RecordName, Type: t, Values: desired[t], TTL: binding.TTL})
		}
	}
	var removals []string
	for _, t := range sortedTypes(published) {
		if !wanted[t] {
			removals = append(removals, t)
		}
	}
	binding.DesiredJSON = encodeJSON(desired)
	if degraded != binding.Degraded {
		action := "forward.dns_degraded"
		if !degraded {
			action = "forward.dns_recovered"
		} else {
			slog.Warn("forward entry HA: no entry node is healthy, keeping the published records", "component", "kernel-forward",
				"binding_id", binding.ID, "route_id", binding.RouteID, "record_name", binding.RecordName)
		}
		if err := auditDNS(db, action, binding, map[string]any{"published": published}); err != nil {
			slog.Warn("forward entry HA: audit failed", "component", "kernel-forward", "error", err)
		}
		binding.Degraded = degraded
	}

	switch {
	case len(sets) == 0 && len(removals) == 0:
		binding.State = settledState(binding, published, degraded)
		return saveState(db, binding)
	case binding.NextAttemptAt != nil && now.Before(*binding.NextAttemptAt):
		binding.State = dnsStateError
		return saveState(db, binding)
	case binding.PublishedAt != nil && now.Sub(*binding.PublishedAt) < DNSMinPublishInterval:
		binding.State = settledState(binding, published, degraded)
		return saveState(db, binding)
	}
	provider, err := e.provider(db, binding.ProviderID)
	if err != nil {
		return e.failed(db, binding, "provider", err)
	}
	if !e.limiter.take(binding.ProviderID, len(sets)+len(removals), now) {
		dnsMetrics.add(provider.Kind(), "rate_limited")
		binding.State = dnsStateRateLimited
		return saveState(db, binding)
	}
	callCtx, cancel := context.WithTimeout(ctx, dnsCallTimeout)
	defer cancel()
	changes := map[string]any{}
	for _, set := range sets {
		if err := provider.SetRecords(callCtx, set); err != nil {
			dnsMetrics.add(provider.Kind(), "error")
			return e.failed(db, binding, set.Type, err)
		}
		dnsMetrics.add(provider.Kind(), "published")
		changes[set.Type] = map[string]any{"from": published[set.Type], "to": set.Values, "ttl": set.TTL}
		published[set.Type] = set.Values
	}
	for _, t := range removals {
		if err := provider.DeleteRecords(callCtx, binding.Zone, binding.RecordName, t); err != nil {
			dnsMetrics.add(provider.Kind(), "error")
			return e.failed(db, binding, t, err)
		}
		dnsMetrics.add(provider.Kind(), "deleted")
		changes[t] = map[string]any{"from": published[t], "to": []string{}}
		delete(published, t)
	}
	binding.PublishedJSON, binding.PublishedAt, binding.PublishedTTL = encodeJSON(published), &now, binding.TTL
	binding.Failures, binding.LastError, binding.LastErrorAt, binding.NextAttemptAt = 0, "", nil, nil
	binding.State = settledState(binding, published, degraded)
	if err := saveState(db, binding); err != nil {
		return err
	}
	if err := auditDNS(db, "forward.dns_publish", binding, map[string]any{"provider_id": binding.ProviderID, "changes": changes}); err != nil {
		slog.Warn("forward entry HA: audit failed", "component", "kernel-forward", "error", err)
	}
	slog.Info("forward entry HA published", "component", "kernel-forward", "binding_id", binding.ID, "route_id", binding.RouteID,
		"record_name", binding.RecordName, "types", len(changes))
	return nil
}

func settledState(binding model.KernelForwardDNSBinding, published map[string][]string, degraded bool) string {
	switch {
	case degraded:
		return dnsStateDegraded
	case len(published) == 0 || binding.PublishedAt == nil:
		return dnsStatePending
	}
	return dnsStateOK
}

// failed records a provider failure and backs off.
func (e *EntryHA) failed(db *gorm.DB, binding model.KernelForwardDNSBinding, what string, cause error) error {
	now := *binding.EvaluatedAt
	binding.Failures++
	backoff := DNSRetryBase << min(binding.Failures-1, 10)
	if backoff > DNSRetryMax || backoff <= 0 {
		backoff = DNSRetryMax
	}
	next := now.Add(backoff)
	binding.State, binding.LastError, binding.LastErrorAt, binding.NextAttemptAt = dnsStateError, what+": "+clipError(cause), &now, &next
	if err := saveState(db, binding); err != nil {
		return err
	}
	slog.Warn("forward entry HA: DNS provider call failed", "component", "kernel-forward", "binding_id", binding.ID,
		"route_id", binding.RouteID, "failures", binding.Failures, "retry_in", backoff.String(), "error", binding.LastError)
	return nil
}
