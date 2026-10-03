package fake_test

import (
	"errors"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/fake"
)

// env adapts a fake Host to the conformance suite, with every optional
// interface.
type env struct {
	host *fake.Host
	opts fake.Options
}

func (e *env) NewDriver(testing.TB) driver.Driver { return fake.New(e.host, e.opts) }
func (e *env) Owned(testing.TB) []string          { return e.host.Owned() }
func (e *env) Foreign(testing.TB) []string        { return e.host.Foreign() }
func (e *env) PlantForeign(testing.TB) {
	e.host.PlantForeign("table ip nat", "docker")
	e.host.PlantForeign("table inet filter", "chain prerouting")
	e.host.PlantForeign("unit wg-quick@wg0", "running")
}
func (e *env) Damage(t testing.TB) {
	if !e.host.Damage() {
		t.Fatal("nothing to damage")
	}
}
func (e *env) Traffic(t testing.TB, k driver.HopKey) {
	if err := e.host.AddTraffic(k, fake.Traffic{UpBytes: 1000, DownBytes: 4000, UpPackets: 10, DownPackets: 20, NewConns: 1, ActiveConns: 1}); err != nil {
		t.Fatal(err)
	}
}
func (e *env) FailNextApply(testing.TB) {
	e.host.FailNext(fake.OpApply, errors.New("nft: transaction refused"))
}
func (e *env) PlantConflict(_ testing.TB, port uint32) {
	e.host.PlantConflict(port, "table ip nat")
}
func (e *env) PlantImpostor(testing.TB) { e.host.PlantImpostor() }
func (e *env) Applies(testing.TB) int   { return e.host.Applies() }

var (
	_ conformance.Damager         = (*env)(nil)
	_ conformance.TrafficSource   = (*env)(nil)
	_ conformance.ApplyFaulter    = (*env)(nil)
	_ conformance.ConflictPlanter = (*env)(nil)
	_ conformance.ImpostorPlanter = (*env)(nil)
	_ conformance.ApplyCounter    = (*env)(nil)
)

func factory(opts fake.Options) conformance.Factory {
	return func(*testing.T) conformance.Env { return &env{host: fake.NewHost(nil), opts: opts} }
}

// TestConformanceNFTables runs the suite on the default, nftables-like fake.
func TestConformanceNFTables(t *testing.T) {
	conformance.Run(t, factory(fake.Options{}))
}

// TestConformanceGost runs it on a gost-like fake: encrypted links, no
// exact quota.
func TestConformanceGost(t *testing.T) {
	caps := fake.NFTablesCapabilities()
	caps.LinkSecurities = []forwardv1.LinkSecurity{
		forwardv1.LinkSecurity_LINK_SECURITY_RAW,
		forwardv1.LinkSecurity_LINK_SECURITY_TLS,
		forwardv1.LinkSecurity_LINK_SECURITY_WSS,
		forwardv1.LinkSecurity_LINK_SECURITY_QUIC,
		forwardv1.LinkSecurity_LINK_SECURITY_GRPC,
	}
	caps.Quota = false
	conformance.Run(t, factory(fake.Options{Engine: forwardv1.Engine_ENGINE_GOST, Capabilities: caps}))
}

// TestConformanceMinimal runs it on a fake with few capabilities, so the
// state cases skip and the unsupported cases run.
func TestConformanceMinimal(t *testing.T) {
	caps := &forwardv1.EngineCapabilities{
		Version:        "fake minimal",
		Available:      true,
		Strategies:     []forwardv1.BalanceStrategy{forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER},
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
	}
	conformance.Run(t, factory(fake.Options{Capabilities: caps}))
}

// bareEnv has only the required Env methods: the optional scenarios skip.
type bareEnv struct{ host *fake.Host }

func (e *bareEnv) NewDriver(testing.TB) driver.Driver { return fake.New(e.host, fake.Options{}) }
func (e *bareEnv) Owned(testing.TB) []string          { return e.host.Owned() }
func (e *bareEnv) Foreign(testing.TB) []string        { return e.host.Foreign() }
func (e *bareEnv) PlantForeign(testing.TB)            { e.host.PlantForeign("table ip nat", "docker") }

func TestConformanceBareEnv(t *testing.T) {
	conformance.Run(t, func(*testing.T) conformance.Env { return &bareEnv{host: fake.NewHost(nil)} })
}

// TestConformanceUnavailable: an unavailable driver passes "capabilities"
// and skips the rest.
func TestConformanceUnavailable(t *testing.T) {
	caps := &forwardv1.EngineCapabilities{Available: false, UnavailableReason: "nft not installed"}
	conformance.Run(t, factory(fake.Options{Capabilities: caps}))
}
