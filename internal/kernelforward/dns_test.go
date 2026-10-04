package kernelforward

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/forwardddns"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	testKEK   = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	otherKEK  = "HxwdHh8AAQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRo="
	cfSecret  = "cf-token-very-secret"
	hostnameA = "hk.example.com"
)

var entry2 = agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 14}

// fakeDNS is a provider that records its record sets.
type fakeDNS struct {
	mu      sync.Mutex
	sets    map[string][]string
	calls   []string
	failing error
}

func (p *fakeDNS) Kind() string { return "fake" }

func (p *fakeDNS) SetRecords(_ context.Context, set forwardddns.RecordSet) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(set.Values) == 0 {
		panic("empty record set published")
	}
	p.calls = append(p.calls, "set "+set.Name+" "+set.Type+" "+strings.Join(set.Values, ","))
	if p.failing != nil {
		return p.failing
	}
	p.sets[set.Name+" "+set.Type] = set.Values
	return nil
}

func (p *fakeDNS) DeleteRecords(_ context.Context, _, name, rtype string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "delete "+name+" "+rtype)
	if p.failing != nil {
		return p.failing
	}
	delete(p.sets, name+" "+rtype)
	return nil
}

func (p *fakeDNS) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	calls := p.calls
	p.calls = nil
	return calls
}

// dnsFixture is the forwarding fixture with a second entry node
// (forward-14, 192.0.2.14), a route with both entries, a fake provider,
// and the controller with a settable set of connected Agents.
type dnsFixture struct {
	*fixture
	dns     *fakeDNS
	ha      *EntryHA
	route   *forwardv1.Route
	online  map[string]bool
	seenKEK *string
}

func newDNSFixture(t *testing.T, db *gorm.DB) *dnsFixture {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	f := newFixture(t, db)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 14, Name: "entry2", Type: "relay", Host: "192.0.2.14", Port: 7000, Enabled: true}).Error)
	f.hello(entry2, nftCaps())
	kek := testKEK
	d := &dnsFixture{fixture: f, dns: &fakeDNS{sets: map[string][]string{}}, online: map[string]bool{"forward-11": true, "forward-14": true}, seenKEK: &kek}
	f.service.DNSKEK = func() string { return *d.seenKEK }
	f.service.NewDNSProvider = func(kind string, _, credentials map[string]string) (forwardddns.Provider, error) {
		if kind != forwardddns.KindCloudflare || credentials[forwardddns.CredentialAPIToken] != cfSecret {
			return nil, errors.New("unexpected provider")
		}
		return d.dns, nil
	}
	d.ha = &EntryHA{Service: f.service, LocalSession: func(node agentcontrol.AgentNode) bool { return d.online[node.String()] }}
	route := twoHop(31000)
	route.Hops[0].NodeRefs = []string{"forward-11", "forward-14"}
	route.Listen.EntryHostname = "HK.example.com"
	d.route = f.create("route-ha", route)
	return d
}

func (d *dnsFixture) provider() *forwardv1.DnsProvider {
	d.t.Helper()
	provider, err := d.service.CreateDNSProvider(d.ctx, "provider-1", &forwardv1.DnsProvider{
		Name: "cf", Kind: forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE,
	}, map[string]string{forwardddns.CredentialAPIToken: cfSecret})
	require.NoError(d.t, err)
	return provider
}

func (d *dnsFixture) bind(providerID uint64, types ...forwardv1.DnsRecordType) *forwardv1.DnsBinding {
	d.t.Helper()
	binding, err := d.service.CreateDNSBinding(d.ctx, "binding-1", &forwardv1.DnsBinding{
		RouteId: d.route.GetId(), ProviderId: providerID, Zone: "example.com", RecordTypes: types,
	})
	require.NoError(d.t, err)
	return binding
}

// report stores a fresh, applied report of node at its desired generation.
func (d *dnsFixture) report(node agentcontrol.AgentNode, hopErr bool) {
	d.t.Helper()
	state := d.state(node)
	report := &forwardv1.NodeForwardReport{NodeRef: node.String(), Generation: state.GetGeneration(), StateHash: state.GetStateHash(), Applied: true}
	if hopErr {
		report.Errors = []*forwardv1.HopError{{RouteId: d.route.GetId(), HopIndex: 0, Message: "listen: address in use"}}
	}
	encoded, err := jsonWrite.Marshal(report)
	require.NoError(d.t, err)
	now := d.clock.Now()
	require.NoError(d.t, d.db.Save(&model.KernelForwardNodeReport{
		NodeRef: node.String(), Generation: report.Generation, StateHash: report.StateHash, Applied: true,
		HopErrors: len(report.Errors), ReportJSON: string(encoded), ObservedAt: now, ReceivedAt: now,
	}).Error)
}

// tick advances 10 s, refreshes the reports of the healthy nodes and
// evaluates.
func (d *dnsFixture) tick(healthy ...agentcontrol.AgentNode) {
	d.t.Helper()
	d.clock.Advance(DNSEvaluateInterval)
	for _, node := range healthy {
		d.report(node, false)
	}
	require.NoError(d.t, d.ha.Evaluate(d.ctx))
}

func (d *dnsFixture) status() *forwardv1.RouteDnsStatus {
	d.t.Helper()
	status, err := d.service.RouteDNS(d.ctx, d.route.GetId())
	require.NoError(d.t, err)
	return status
}

func (d *dnsFixture) audits(action string) int64 {
	var count int64
	require.NoError(d.t, d.db.Model(&model.OperationLog{}).Where("action = ? AND username = ?", action, dnsAuditActor).Count(&count).Error)
	return count
}

func TestDNSProviderCredentialsAreSealedAndWriteOnly(t *testing.T) {
	runDNSProviderCredentials(t, openSQLite(t))
}

func TestPostgresDNSProviderCredentials(t *testing.T) {
	runDNSProviderCredentials(t, openPostgres(t))
}

func runDNSProviderCredentials(t *testing.T, db *gorm.DB) {
	d := newDNSFixture(t, db)
	provider := d.provider()
	require.Equal(t, []string{forwardddns.CredentialAPIToken}, provider.GetCredentialNames())
	encoded, err := jsonWrite.Marshal(provider)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), cfSecret)

	var row model.KernelForwardDNSProvider
	require.NoError(t, db.First(&row, provider.GetId()).Error)
	require.NotEmpty(t, row.SealedCredentials)
	require.NotContains(t, row.SealedCredentials, cfSecret)
	require.NotContains(t, row.ConfigJSON+row.CredentialNames, cfSecret)

	// A retry answers the same provider once.
	again, err := d.service.CreateDNSProvider(d.ctx, "provider-1", &forwardv1.DnsProvider{
		Name: "cf", Kind: forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE,
	}, map[string]string{forwardddns.CredentialAPIToken: cfSecret})
	require.NoError(t, err)
	require.Equal(t, provider.GetId(), again.GetId())

	// The placeholder keeps the stored secret; the provider still builds.
	_, err = d.service.UpdateDNSProvider(d.ctx, "provider-2", &forwardv1.DnsProvider{Id: provider.GetId(), Name: "cf-main"},
		map[string]string{forwardddns.CredentialAPIToken: service.SensitiveSystemConfigPlaceholder})
	require.NoError(t, err)
	built, err := d.service.dnsProvider(db, provider.GetId())
	require.NoError(t, err)
	require.Equal(t, d.dns, built)

	// Another key-encryption key cannot open the credentials.
	other := otherKEK
	d.seenKEK = &other
	_, err = d.service.dnsProvider(db, provider.GetId())
	require.ErrorContains(t, err, "wrong key-encryption key")

	// Without a key-encryption key nothing is stored.
	empty := ""
	d.seenKEK = &empty
	_, err = d.service.CreateDNSProvider(d.ctx, "provider-3", &forwardv1.DnsProvider{
		Name: "cf2", Kind: forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE,
	}, map[string]string{forwardddns.CredentialAPIToken: cfSecret})
	var refusal *DNSRefusedError
	require.ErrorAs(t, err, &refusal)
	require.True(t, refusal.Precondition)
	require.Equal(t, CodeSecretStoreUnavailable, refusal.Violations[0].GetCode())

	// Missing credentials and unknown settings are refused.
	kek := testKEK
	d.seenKEK = &kek
	_, err = d.service.CreateDNSProvider(d.ctx, "provider-4", &forwardv1.DnsProvider{
		Name: "ali", Kind: forwardv1.DnsProviderKind_DNS_PROVIDER_KIND_ALIDNS, Config: map[string]string{"region": "cn"},
	}, map[string]string{forwardddns.CredentialAccessKeyID: "id"})
	require.ErrorAs(t, err, &refusal)
	require.False(t, refusal.Precondition)
	require.Len(t, refusal.Violations, 2)

	// A provider in use cannot be deleted.
	d.bind(provider.GetId())
	err = d.service.DeleteDNSProvider(d.ctx, "provider-5", provider.GetId())
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, CodeProviderInUse, refusal.Violations[0].GetCode())
	require.Equal(t, d.route.GetId(), refusal.Violations[0].GetRouteId())
	listed, err := d.service.ListDNSProviders(d.ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.EqualValues(t, 1, listed[0].GetBindings())
	require.Equal(t, "cf-main", listed[0].GetName())
}

func TestDNSBindingValidation(t *testing.T) {
	d := newDNSFixture(t, openSQLite(t))
	provider := d.provider()
	var refusal *DNSRefusedError

	_, err := d.service.CreateDNSBinding(d.ctx, "b-1", &forwardv1.DnsBinding{
		RouteId: d.route.GetId(), ProviderId: provider.GetId(), Zone: "example.com", RecordName: "other.example.com",
	})
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, CodeHostnameMismatch, refusal.Violations[0].GetCode())

	_, err = d.service.CreateDNSBinding(d.ctx, "b-2", &forwardv1.DnsBinding{
		RouteId: "missing", ProviderId: 99, Zone: "example.net", Mode: forwardv1.DnsBindingMode_DNS_BINDING_MODE_CNAME, RecordName: "x.example.com", Ttl: DNSMaxTTL + 1,
	})
	require.ErrorAs(t, err, &refusal)
	codes := map[string]bool{}
	for _, v := range refusal.Violations {
		codes[v.GetCode()] = true
	}
	require.True(t, codes[CodeUnknownRoute] && codes[CodeUnknownProvider] && codes[CodeInvalidFormat], codes)

	// CNAME mode: Control manages its own name.
	binding, err := d.service.CreateDNSBinding(d.ctx, "b-3", &forwardv1.DnsBinding{
		RouteId: d.route.GetId(), ProviderId: provider.GetId(), Zone: "ha.example.net", Mode: forwardv1.DnsBindingMode_DNS_BINDING_MODE_CNAME,
		RecordName: "R1.ha.example.net.", RecordTypes: []forwardv1.DnsRecordType{forwardv1.DnsRecordType_DNS_RECORD_TYPE_AAAA, forwardv1.DnsRecordType_DNS_RECORD_TYPE_A},
	})
	require.NoError(t, err)
	require.Equal(t, "r1.ha.example.net", binding.GetRecordName())
	require.EqualValues(t, DNSDefaultTTL, binding.GetTtl())
	require.Len(t, binding.GetRecordTypes(), 2)
	status := d.status()
	require.Equal(t, "r1.ha.example.net", status.GetCnameTarget())
	require.Equal(t, hostnameA, status.GetEntryHostname())

	// One binding per route.
	_, err = d.service.CreateDNSBinding(d.ctx, "b-4", &forwardv1.DnsBinding{RouteId: d.route.GetId(), ProviderId: provider.GetId(), Zone: "example.com"})
	require.ErrorAs(t, err, &refusal)
	require.True(t, refusal.Precondition)
	require.Equal(t, CodeBindingExists, refusal.Violations[0].GetCode())

	// Only record types, TTL and paused change.
	_, err = d.service.UpdateDNSBinding(d.ctx, "b-5", &forwardv1.DnsBinding{Id: binding.GetId(), Zone: "example.org"})
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, CodeImmutable, refusal.Violations[0].GetCode())
	updated, err := d.service.UpdateDNSBinding(d.ctx, "b-6", &forwardv1.DnsBinding{Id: binding.GetId(), Ttl: 300, Paused: true})
	require.NoError(t, err)
	require.EqualValues(t, 300, updated.GetTtl())
	require.True(t, updated.GetPaused())
	require.Equal(t, []forwardv1.DnsRecordType{forwardv1.DnsRecordType_DNS_RECORD_TYPE_A}, updated.GetRecordTypes())

	// A route without entry_hostname cannot be bound.
	single := d.create("route-single", twoHop(31001))
	_, err = d.service.CreateDNSBinding(d.ctx, "b-7", &forwardv1.DnsBinding{RouteId: single.GetId(), ProviderId: provider.GetId(), Zone: "example.com"})
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, CodeEntryHostnameRequired, refusal.Violations[0].GetCode())

	_, err = d.service.RouteDNS(d.ctx, "missing")
	require.ErrorIs(t, err, ErrNotFound)
	unbound, err := d.service.RouteDNS(d.ctx, single.GetId())
	require.NoError(t, err)
	require.Equal(t, dnsStateUnbound, unbound.GetState())
}

func TestEntryHAHysteresisAndNeverEmpty(t *testing.T) {
	runEntryHA(t, openSQLite(t))
}

func TestPostgresEntryHA(t *testing.T) {
	runEntryHA(t, openPostgres(t))
}

func runEntryHA(t *testing.T, db *gorm.DB) {
	d := newDNSFixture(t, db)
	provider := d.provider()
	d.bind(provider.GetId())

	// Before any report nothing is published.
	require.NoError(t, d.ha.Evaluate(d.ctx))
	require.Empty(t, d.dns.take())
	require.Equal(t, dnsStatePending, d.status().GetState())

	// First publication: every healthy entry joins at once.
	d.tick(entry, entry2)
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11,192.0.2.14"}, d.dns.take())
	status := d.status()
	require.Equal(t, dnsStateOK, status.GetState())
	require.Equal(t, []string{"192.0.2.11", "192.0.2.14"}, status.GetRecords()[0].GetPublished())
	require.EqualValues(t, 1, d.audits("forward.dns_publish"))

	// forward-14 reports a hop error on the entry hop: it stays in
	// rotation for two evaluations and leaves on the third.
	d.report(entry2, true)
	d.tick(entry)
	d.tick(entry)
	require.Empty(t, d.dns.take())
	nodes := d.status().GetNodes()
	require.Equal(t, dnsReasonHopError, nodes[1].GetReason())
	require.True(t, nodes[1].GetInRotation())
	require.EqualValues(t, 2, nodes[1].GetBadStreak())
	d.tick(entry)
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11"}, d.dns.take())

	// forward-11's Agent goes offline: with no entry left in rotation the
	// published record is kept (never empty) and the binding degrades.
	d.online["forward-11"] = false
	for i := 0; i < 5; i++ {
		d.tick(entry)
	}
	require.Empty(t, d.dns.take())
	status = d.status()
	require.Equal(t, dnsStateDegraded, status.GetState())
	require.Equal(t, []string{"192.0.2.11"}, status.GetRecords()[0].GetPublished())
	require.Empty(t, status.GetRecords()[0].GetDesired())
	require.Equal(t, dnsReasonOffline, status.GetNodes()[0].GetReason())
	require.EqualValues(t, 1, d.audits("forward.dns_degraded"))

	// forward-14 recovers: it needs three good evaluations to rejoin.
	d.tick(entry, entry2)
	d.tick(entry, entry2)
	require.Empty(t, d.dns.take())
	d.tick(entry, entry2)
	require.Equal(t, []string{"set hk.example.com A 192.0.2.14"}, d.dns.take())
	require.Equal(t, dnsStateOK, d.status().GetState())
	require.EqualValues(t, 1, d.audits("forward.dns_recovered"))

	// A stale report counts as down too.
	d.online["forward-11"] = true
	for i := 0; i < 3; i++ {
		d.tick(entry, entry2)
	}
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11,192.0.2.14"}, d.dns.take())
	d.clock.Advance(DNSReportStaleAfter)
	for i := 0; i < 3; i++ {
		d.tick(entry)
	}
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11"}, d.dns.take())
	require.Equal(t, dnsReasonReportStale, d.status().GetNodes()[1].GetReason())

	// Paused routes and bindings change nothing.
	_, err := d.service.UpdateDNSBinding(d.ctx, "pause", &forwardv1.DnsBinding{Id: d.status().GetBinding().GetId(), Paused: true})
	require.NoError(t, err)
	d.online["forward-11"] = false
	for i := 0; i < 4; i++ {
		d.tick(entry)
	}
	require.Empty(t, d.dns.take())
	require.Equal(t, dnsStatePaused, d.status().GetState())

	// Purging deletes what was published.
	require.NoError(t, d.service.DeleteDNSBinding(d.ctx, "delete", d.status().GetBinding().GetId(), true))
	require.Equal(t, []string{"delete hk.example.com A"}, d.dns.take())
	require.Equal(t, dnsStateUnbound, d.status().GetState())
	require.EqualValues(t, 1, d.audits("forward.dns_binding_delete"))
}

func TestEntryHABackoffAndRateLimit(t *testing.T) {
	d := newDNSFixture(t, openSQLite(t))
	provider := d.provider()
	d.bind(provider.GetId())
	d.dns.failing = errors.New("cloudflare: HTTP 429 10000: rate limited")
	d.tick(entry, entry2)
	require.Len(t, d.dns.take(), 1)
	status := d.status()
	require.Equal(t, dnsStateError, status.GetState())
	require.Contains(t, status.GetLastError(), "rate limited")
	require.Equal(t, d.clock.Now().Add(DNSRetryBase).UnixMilli(), status.GetNextAttemptAtUnixMs())

	// Within the back-off nothing is called.
	d.tick(entry, entry2)
	d.tick(entry, entry2)
	require.Empty(t, d.dns.take())
	d.tick(entry, entry2)
	require.Len(t, d.dns.take(), 1)
	// The second failure doubles the back-off.
	require.Equal(t, d.clock.Now().Add(2*DNSRetryBase).UnixMilli(), d.status().GetNextAttemptAtUnixMs())

	d.dns.failing = nil
	d.clock.Advance(2 * DNSRetryBase)
	// The provider's tokens are spent: the call waits.
	require.True(t, d.ha.limiter.take(provider.GetId(), DNSProviderBurst, d.clock.Now().Add(DNSEvaluateInterval)))
	d.tick(entry, entry2)
	require.Empty(t, d.dns.take())
	require.Equal(t, dnsStateRateLimited, d.status().GetState())
	d.clock.Advance(DNSProviderRefill)
	d.tick(entry, entry2)
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11,192.0.2.14"}, d.dns.take())
	require.Equal(t, dnsStateOK, d.status().GetState())
	require.Empty(t, d.status().GetLastError())

	// A TTL change republishes; the resync repeats the set after an hour.
	binding := d.status().GetBinding()
	_, err := d.service.UpdateDNSBinding(d.ctx, "ttl", &forwardv1.DnsBinding{Id: binding.GetId(), Ttl: 120})
	require.NoError(t, err)
	d.clock.Advance(DNSMinPublishInterval)
	d.tick(entry, entry2)
	require.Len(t, d.dns.take(), 1)
	d.tick(entry, entry2)
	require.Empty(t, d.dns.take())
	d.clock.Advance(DNSResyncInterval)
	d.tick(entry, entry2)
	require.Len(t, d.dns.take(), 1)

	// Dropping a record type deletes its records; AAAA has no addresses.
	_, err = d.service.UpdateDNSBinding(d.ctx, "types", &forwardv1.DnsBinding{Id: binding.GetId(), Ttl: 120,
		RecordTypes: []forwardv1.DnsRecordType{forwardv1.DnsRecordType_DNS_RECORD_TYPE_AAAA}})
	require.NoError(t, err)
	d.clock.Advance(DNSMinPublishInterval)
	d.tick(entry, entry2)
	require.Equal(t, []string{"delete hk.example.com A"}, d.dns.take())
	require.Equal(t, dnsStatePending, d.status().GetState())
	require.Equal(t, dnsReasonNoAddress, d.status().GetNodes()[0].GetReason())
}

func TestEntryHAPresenceFromAgentTransports(t *testing.T) {
	d := newDNSFixture(t, openSQLite(t))
	require.NoError(t, d.db.AutoMigrate(&model.AgentTransport{}))
	d.ha.LocalSession = nil
	provider := d.provider()
	d.bind(provider.GetId())
	seen := func(id uint, at time.Time) {
		require.NoError(t, d.db.Save(&model.AgentTransport{NodeKind: agentcontrol.NodeKindForward, NodeID: id, Transport: model.AgentTransportMTLSStream,
			FirstSeenAt: at, LastSeenAt: at}).Error)
	}
	seen(11, d.clock.Now())
	seen(14, d.clock.Now().Add(-DNSPresenceStaleAfter))
	d.tick(entry, entry2)
	require.Equal(t, []string{"set hk.example.com A 192.0.2.11"}, d.dns.take())
	require.Equal(t, dnsReasonOffline, d.status().GetNodes()[1].GetReason())
}

func TestEvaluateEntry(t *testing.T) {
	now := start
	report := func(mutate func(*forwardv1.NodeForwardReport)) *forwardv1.NodeForwardReport {
		r := &forwardv1.NodeForwardReport{Generation: 4, Applied: true}
		if mutate != nil {
			mutate(r)
		}
		return r
	}
	good := entryFacts{InInventory: true, Addresses: []string{"192.0.2.1"}, Report: report(nil), ReceivedAt: now, Desired: 4, Online: true}
	cases := []struct {
		name   string
		facts  func(entryFacts) entryFacts
		want   verdict
		reason string
	}{
		{"healthy", func(f entryFacts) entryFacts { return f }, verdictGood, dnsReasonHealthy},
		{"not in inventory", func(f entryFacts) entryFacts { f.InInventory = false; return f }, verdictBad, dnsReasonNoInventory},
		{"no address", func(f entryFacts) entryFacts { f.Addresses = nil; return f }, verdictBad, dnsReasonNoAddress},
		{"never reported", func(f entryFacts) entryFacts { f.Report = nil; return f }, verdictBad, dnsReasonNeverReported},
		{"stale", func(f entryFacts) entryFacts { f.ReceivedAt = now.Add(-DNSReportStaleAfter - time.Second); return f }, verdictBad, dnsReasonReportStale},
		{"offline", func(f entryFacts) entryFacts { f.Online = false; return f }, verdictBad, dnsReasonOffline},
		{"hop error elsewhere", func(f entryFacts) entryFacts {
			f.Report = report(func(r *forwardv1.NodeForwardReport) {
				r.Errors = []*forwardv1.HopError{{RouteId: "r", HopIndex: 1}, {RouteId: "other", HopIndex: 0}}
			})
			return f
		}, verdictGood, dnsReasonHealthy},
		{"hop error", func(f entryFacts) entryFacts {
			f.Report = report(func(r *forwardv1.NodeForwardReport) { r.Errors = []*forwardv1.HopError{{RouteId: "r"}} })
			return f
		}, verdictBad, dnsReasonHopError},
		{"some upstreams down", func(f entryFacts) entryFacts {
			f.Report = report(func(r *forwardv1.NodeForwardReport) {
				r.Health = []*forwardv1.UpstreamHealth{{RouteId: "r", State: forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN}, {RouteId: "r", State: forwardv1.HealthState_HEALTH_STATE_HEALTHY}}
			})
			return f
		}, verdictGood, dnsReasonHealthy},
		{"every upstream down", func(f entryFacts) entryFacts {
			f.Report = report(func(r *forwardv1.NodeForwardReport) {
				r.Health = []*forwardv1.UpstreamHealth{{RouteId: "r", State: forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN}, {RouteId: "r", State: forwardv1.HealthState_HEALTH_STATE_UNHEALTHY}}
			})
			return f
		}, verdictBad, dnsReasonUpstreamsDown},
		{"converging", func(f entryFacts) entryFacts { f.Desired = 5; return f }, verdictNeutral, dnsReasonConverging},
		{"not applied", func(f entryFacts) entryFacts {
			f.Report = report(func(r *forwardv1.NodeForwardReport) { r.Applied = false })
			return f
		}, verdictNeutral, dnsReasonConverging},
	}
	for _, c := range cases {
		v, reason := evaluateEntry("r", c.facts(good), now)
		require.Equal(t, c.want, v, c.name)
		require.Equal(t, c.reason, reason, c.name)
	}
}

func TestPublicAddressesAndTokenBuckets(t *testing.T) {
	require.Equal(t, []string{"203.0.113.5", "2001:db8::1"},
		publicAddresses([]string{"10.0.0.1", "203.0.113.5", "::ffff:203.0.113.5", "fe80::1", "2001:db8::1", "127.0.0.1", "bad"}, []string{"A", "AAAA"}))
	require.Equal(t, []string{"2001:db8::1"}, publicAddresses([]string{"203.0.113.5", "2001:db8::1"}, []string{"AAAA"}))

	var buckets tokenBuckets
	require.True(t, buckets.take(1, DNSProviderBurst, start))
	require.False(t, buckets.take(1, 1, start))
	require.True(t, buckets.take(2, 1, start))
	require.True(t, buckets.take(1, 1, start.Add(DNSProviderRefill)))
	require.False(t, buckets.take(1, 1, start.Add(DNSProviderRefill)))

	node := model.KernelForwardDNSNode{}
	step(&node, verdictGood, false)
	step(&node, verdictGood, false)
	require.False(t, node.InRotation)
	step(&node, verdictNeutral, false)
	step(&node, verdictGood, false)
	require.True(t, node.InRotation)
	step(&node, verdictBad, false)
	step(&node, verdictGood, false)
	step(&node, verdictBad, false)
	step(&node, verdictBad, false)
	require.True(t, node.InRotation)
	step(&node, verdictBad, false)
	require.False(t, node.InRotation)
}

func TestDNSPrometheus(t *testing.T) {
	d := newDNSFixture(t, openSQLite(t))
	d.bind(d.provider().GetId())
	d.tick(entry, entry2)
	var body strings.Builder
	WritePrometheus(&body, d.db)
	require.Contains(t, body.String(), `anixops_forward_dns_bindings{state="ok"} 1`)
	require.Contains(t, body.String(), `anixops_forward_dns_updates_total{provider="fake",result="published"}`)
}
