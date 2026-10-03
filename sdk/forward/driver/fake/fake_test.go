package fake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/fake"
)

func TestFakeTrafficHealthAndFaults(t *testing.T) {
	ctx := context.Background()
	now := time.UnixMilli(1_700_000_000_000)
	host := fake.NewHost(func() time.Time { return now })
	d := fake.New(host, fake.Options{})
	b := conformance.Builder{Engine: d.Engine(), Caps: fake.NFTablesCapabilities(), Top: conformance.DefaultTopology()}
	hop := b.Simple(conformance.RouteA, 0)
	k := driver.KeyOf(hop)

	if err := host.AddTraffic(k, fake.Traffic{UpBytes: 1}); !errors.Is(err, driver.ErrNotFound) {
		t.Fatalf("traffic before apply: %v", err)
	}
	a, err := d.Render(conformance.State("forward-11", 1, hop))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Apply(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := host.AddTraffic(k, fake.Traffic{UpBytes: 10, DownBytes: 20, UpPackets: 1, DownPackets: 2, NewConns: 3, ActiveConns: 2}); err != nil {
		t.Fatal(err)
	}
	up := hop.GetUpstreams()[0]
	if err := host.SetHealth(k, &forwardv1.UpstreamHealth{Address: up.GetAddress(), Port: up.GetPort(), State: forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN, ConsecutiveFailures: 3}); err != nil {
		t.Fatal(err)
	}
	if err := host.SetHealth(k, &forwardv1.UpstreamHealth{Address: "198.51.100.1", Port: 1}); !errors.Is(err, driver.ErrNotFound) {
		t.Fatalf("health of unknown upstream: %v", err)
	}
	o, err := d.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c := o.Counters[0]
	if c.GetUpBytes() != 10 || c.GetDownBytes() != 20 || c.GetUpPackets() != 1 || c.GetDownPackets() != 2 || c.GetTotalConns() != 3 || c.GetActiveConns() != 2 || c.GetObservedAtUnixMs() != now.UnixMilli() {
		t.Fatalf("counters %v", c)
	}
	if len(o.Health) != 1 || o.Health[0].GetRouteId() != k.RouteID || o.Health[0].GetState() != forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN {
		t.Fatalf("health %v", o.Health)
	}

	// Health of an upstream that survives a changing apply is kept; a
	// dropped upstream's is not.
	hop2 := b.Simple(conformance.RouteA, 0)
	hop2.Upstreams = hop2.Upstreams[1:]
	a2, _ := d.Render(conformance.State("forward-11", 2, hop2))
	if _, err := d.Apply(ctx, a2); err != nil {
		t.Fatal(err)
	}
	if o, _ = d.Observe(ctx); len(o.Health) != 0 || o.Counters[0].GetUpBytes() != 10 {
		t.Fatalf("after apply: health %v counters %v", o.Health, o.Counters)
	}

	boom := errors.New("boom")
	for _, op := range []fake.Op{fake.OpCapabilities, fake.OpObserve, fake.OpSetUpstreams, fake.OpRemove} {
		host.FailNext(op, boom)
	}
	if _, err := d.Capabilities(ctx); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := d.Observe(ctx); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	u := hop2.GetUpstreams()[0]
	if err := d.SetUpstreams(ctx, k.RouteID, k.HopIndex, []driver.Upstream{{Address: u.GetAddress(), Port: u.GetPort()}}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if err := d.Remove(ctx); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if len(host.Owned()) == 0 {
		t.Fatal("failed Remove removed")
	}
	if err := d.Remove(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFakeImpostorOverOwnedPanics(t *testing.T) {
	host := fake.NewHost(nil)
	d := fake.New(host, fake.Options{})
	b := conformance.Builder{Engine: d.Engine(), Caps: fake.NFTablesCapabilities(), Top: conformance.DefaultTopology()}
	a, _ := d.Render(conformance.State("forward-11", 1, b.Simple(conformance.RouteA, 0)))
	if _, err := d.Apply(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	host.PlantImpostor()
}
