package relayd_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayd"
)

func TestControlAPI(t *testing.T) {
	r := relayd.New(relayd.Options{})
	t.Cleanup(r.Close)
	socket := filepath.Join(t.TempDir(), "control.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.Serve(ctx, socket)
	}()
	t.Cleanup(func() { cancel(); <-done })
	client := relayctl.NewClient(socket)
	cctx := t.Context()
	waitFor(t, "the socket", func() bool { _, err := client.Status(cctx); return err == nil })

	st, err := client.Status(cctx)
	if err != nil || st.Instance == "" || st.Digest != "" || st.Applies != 0 {
		t.Fatalf("status of a fresh relay: %+v, %v", st, err)
	}

	host, port := echoTCP(t)
	entry := freePort(t)
	doc := relayctl.Config{Format: relayctl.Format, Owner: "test", Note: "test", Hops: []relayctl.Hop{rawHop(entry, true, false, target(host, port))}}
	content, err := relayctl.Encode(&doc)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := client.Apply(cctx, content)
	if err != nil || !reply.Changed || reply.Digest != relayctl.Digest(content) {
		t.Fatalf("apply: %+v, %v", reply, err)
	}
	if reply, err = client.Apply(cctx, content); err != nil || reply.Changed {
		t.Fatalf("the same document again: %+v, %v", reply, err)
	}
	conn := dialTCP(t, entry)
	roundTrip(t, conn, []byte("through the control-applied hop"))
	_ = conn.Close()
	st, err = client.Status(cctx)
	if err != nil || st.Digest != reply.Digest || st.Applies != 1 || len(st.Hops) != 1 || !st.Hops[0].Listening {
		t.Fatalf("status: %+v, %v", st, err)
	}
	waitFor(t, "the connection to end", func() bool {
		o, err := client.Observe(cctx, false)
		return err == nil && len(o.Hops) == 1 && o.Hops[0].Active == 0
	})
	o, err := client.Observe(cctx, true)
	if err != nil || len(o.Hops) != 1 || o.Hops[0].UpBytes == 0 || len(o.Hops[0].Rotation) != 1 {
		t.Fatalf("observe: %+v, %v", o, err)
	}

	// Refusals carry their codes.
	var e *relayctl.Error
	if _, err := client.Apply(cctx, []byte(`{"format":"nope"}`)); !errors.As(err, &e) || e.Code != relayctl.CodeInvalid {
		t.Fatalf("a malformed document: %v", err)
	}
	if _, err := client.Apply(cctx, []byte(`not json`)); !errors.As(err, &e) || e.Code != relayctl.CodeInvalid {
		t.Fatalf("a document that is not JSON: %v", err)
	}
	err = client.SetRotation(cctx, relayctl.Rotation{Route: "missing", Hop: 0, Active: []relayctl.RotationEntry{{Address: host, Port: port}}})
	if !errors.As(err, &e) || e.Code != relayctl.CodeNotFound {
		t.Fatalf("a rotation for an unknown hop: %v", err)
	}
	err = client.SetRotation(cctx, relayctl.Rotation{Route: route, Hop: 0})
	if !errors.As(err, &e) || e.Code != relayctl.CodeInvalid {
		t.Fatalf("an empty rotation: %v", err)
	}
	if err := client.ReloadCredentials(cctx); !errors.As(err, &e) || e.Code != relayctl.CodeInvalid {
		t.Fatalf("a credential reload without link files: %v", err)
	}

	// A conflict is its own code.
	other := doc
	other.Hops = append([]relayctl.Hop{}, doc.Hops...)
	clash := other.Hops[0]
	clash.Route = "01JF2A000000000000000000B1"
	clash.Listen.Port = freePort(t)
	hold := listenForeign(t, clash.Listen.Port)
	defer func() { _ = hold.Close() }()
	other.Hops = append(other.Hops, clash)
	b2, _ := relayctl.Encode(&other)
	if _, err := client.Apply(cctx, b2); !errors.As(err, &e) || e.Code != relayctl.CodeConflict {
		t.Fatalf("a port another process holds: %v", err)
	}
}

// TestManyConnectionsThroughTheChain runs hundreds of connections of random
// sizes at once over every carrier, with the race detector's help, and checks
// every byte and the books.
func TestManyConnectionsThroughTheChain(t *testing.T) {
	for _, carrier := range []string{relayctl.CarrierTLSTCP, relayctl.CarrierQUIC} {
		t.Run(carrier, func(t *testing.T) {
			c := newChain(t, carrier, relayd.Options{})
			const conns = 120
			var wg sync.WaitGroup
			var total uint64
			var mu sync.Mutex
			errs := make(chan error, conns)
			for i := range conns {
				wg.Add(1)
				go func() {
					defer wg.Done()
					size := 1 + (i*7919)%90_000
					conn, err := dialRaw(c.entryPort)
					if err != nil {
						errs <- err
						return
					}
					defer func() { _ = conn.Close() }()
					if err := echoCheck(conn, pattern(size)); err != nil {
						errs <- err
						return
					}
					mu.Lock()
					total += uint64(size) // #nosec G115 -- a small positive int
					mu.Unlock()
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
			waitFor(t, "every connection to end", func() bool {
				return c.entry.hopObs(route, 0).Active == 0 && c.mid.hopObs(route, 1).Active == 0 && c.exit.hopObs(route, 2).Active == 0
			})
			for _, h := range []struct {
				n   *node
				hop uint32
			}{{c.entry, 0}, {c.mid, 1}, {c.exit, 2}} {
				o := h.n.hopObs(route, h.hop)
				if o.UpBytes != total || o.DownBytes != total || o.Total != conns {
					t.Errorf("hop %d: up %d down %d total %d, want %d %d %d", h.hop, o.UpBytes, o.DownBytes, o.Total, total, total, conns)
				}
			}
		})
	}
}
