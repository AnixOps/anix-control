package gost

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// ReloadCredentials makes the running gost use the link certificate files
// (Config.LinkCert, LinkKey, LinkCA) again; the Agent calls it after it
// renewed them in place (owner decision H28). The configuration file
// names the files, not their contents, so it does not change, and Apply
// would see nothing to do.
//
// gost 3.2.6 has no certificate hot-reload: it reads a TLS certificate
// only when it builds the object that uses it, a listener's when it
// creates the service and a dialer's when it creates the hop. So
// ReloadCredentials, through the web API as Apply changes structure
// (F4c), deletes and creates again every service whose listener is
// encrypted (tls, wss, grpc) and replaces every hop whose dialers are,
// keeping the upstreams a SetUpstreams put in rotation; every other
// service runs on untouched. It then waits, as Apply does, until gost
// holds every listener and runs every service again. A re-created
// service's established connections run on with the old certificate
// until they close (gRPC's end with its listener and their peers dial
// again). The hops whose services were re-created start a new counter
// epoch, recorded in the state file as Apply records its own
// re-creations, and the WithRetiredCounters hook gets their counters
// read just before the first change; hops whose dialers alone use the
// certificate keep theirs.
//
// A mux (mtls, mwss) or QUIC listener cannot be re-created that way
// (linkObjects), so when the node has one ReloadCredentials restarts gost
// on the recorded file instead, as it does when the web API does not
// answer or gost refuses a change: every established connection ends,
// every hop starts a new counter epoch (the state file records the
// start), the hook gets every hop's counters when the web API answers,
// and every rendered upstream is back in rotation. A peer's QUIC carrier
// to the restarted gost gets no close and is dropped at its idle timeout
// (30 s). A reload (SIGHUP) would not do: the file is the one gost
// loaded, so whether a reload took could not be seen, and it would strand
// mux carriers as re-creating them does.
//
// It does nothing when nothing is applied, gost does not run (a start
// reads the files) or no route uses the link certificate. The files must
// load (the certificate with its key, a CA in PEM) before anything
// changes; a configuration file that is not the recorded one is
// ErrConflict (Apply puts the recorded one back, and gost loads the files
// with it). It is serialised with Apply, SetUpstreams and Remove; Observe
// runs between them.
func (d *Driver) ReloadCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.cfg.linkTLS() {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	h, err := d.readHost()
	if err != nil {
		return err
	}
	st := h.state
	if !h.owned || !st.applied() || h.config == nil {
		return nil
	}
	if driver.Digest(h.config) != st.Digest {
		return fmt.Errorf("%w: %s is not the configuration the driver applied; Apply puts it back", driver.ErrConflict, d.cfg.configPath())
	}
	cfg, err := decodeConfig(h.config)
	if err != nil {
		return err
	}
	touched, restart := linkObjects(cfg)
	if len(touched) == 0 {
		return nil
	}
	if err := d.cfg.loadLink(); err != nil {
		return err
	}
	if err := d.sup.Check(ctx); err != nil {
		return err
	}
	status, err := d.status(ctx)
	if err != nil {
		return err
	}
	if !status.Running {
		return nil
	}
	p, err := parseContent(driver.Artifact{Content: h.config, Hops: keysOf(st.Hops)})
	if err != nil {
		return err
	}

	var before []*forwardv1.Counters
	if !restart {
		_, live := d.apiReady(ctx, h, h.config)
		if err := ctx.Err(); err != nil {
			return err
		}
		if live != nil {
			if d.retired != nil && status.Instance != "" {
				before = d.hopCounters(st, live, status.Instance)
			}
			apiErr := d.reloadAPI(ctx, h, cfg, live, touched, before)
			if apiErr == nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return errors.Join(apiErr, d.recoverPrevious(ctx, h))
			}
		}
	}
	if before == nil && d.retired != nil && status.Instance != "" {
		if l, err := d.readLive(ctx); err == nil {
			before = d.hopCounters(st, l, status.Instance)
		}
	}

	// Restart gost on the recorded file. A reload (SIGHUP) would not do:
	// the file is the one gost loaded, so whether it took could not be
	// seen, and it would strand mux carriers as re-creating does.
	if err := d.restart(ctx, p); err != nil {
		return errors.Join(fmt.Errorf("gost driver: reload credentials: %w", err), d.recoverPrevious(ctx, h))
	}
	next := *st
	next.Loads++
	next.Served = p.metricsPath
	if err := d.writeState(&next); err != nil {
		return err
	}
	if len(before) > 0 {
		d.retired(before)
	}
	return nil
}

// reloadAPI re-creates the services and replaces the hops in touched
// through the web API, waits until gost serves the recorded configuration
// again, and records the re-created services' hops as starting a new
// counter epoch, handing their counters in before to the hook. On error
// the caller restarts gost.
func (d *Driver) reloadAPI(ctx context.Context, h *host, cfg *gostConfig, live *liveConfig, touched map[string]bool, before []*forwardv1.Counters) error {
	st := h.state
	target, err := objects(cfg)
	if err != nil {
		return err
	}
	// A hop keeps the selection a SetUpstreams put in rotation.
	for i, o := range target {
		if o.kind != kindHops || !touched[o.key()] {
			continue
		}
		rot, ok := liveRotation(live.hop(o.name))
		if !ok {
			continue
		}
		for j := range st.Hops {
			mh := &st.Hops[j]
			if hopName(mh.key()) != o.name {
				continue
			}
			weights := map[string]uint32{}
			for _, u := range rot {
				weights[fmt.Sprintf("%s|%d", u.Address, u.Port)] = u.Weight
			}
			body, err := d.selectionHop(h, mh, weights)
			if err != nil {
				return err
			}
			target[i].body = body
		}
	}

	log := newSyncLog()
	if err := d.sync(ctx, target, touched, live.names(), log); err != nil {
		return fmt.Errorf("gost driver: reload credentials: %w", err)
	}
	p, err := parseContent(driver.Artifact{Content: h.config, Hops: keysOf(st.Hops)})
	if err != nil {
		return err
	}
	if err := d.waitAPI(ctx, st, p); err != nil {
		return err
	}

	hops := map[string]bool{}
	for _, mh := range st.Hops {
		for _, s := range mh.Services {
			if log.services[s] {
				hops[hopName(mh.key())] = true
			}
		}
	}
	next := *st
	next.recreated(hops)
	if err := d.writeState(&next); err != nil {
		return err
	}
	if d.retired != nil && len(before) == len(st.Hops) {
		var out []*forwardv1.Counters
		for i, mh := range st.Hops {
			if hops[hopName(mh.key())] {
				out = append(out, before[i])
			}
		}
		if len(out) > 0 {
			d.retired(out)
		}
	}
	return nil
}

// waitAPI waits until gost serves p as st records it again (serves):
// every listener held and every service and hop running.
func (d *Driver) waitAPI(ctx context.Context, st *hostState, p *parsed) error {
	deadline := time.Now().Add(d.cfg.ReadyTimeout)
	for {
		ok, _, err := d.serves(ctx, st, p)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gost driver: gost did not serve the configuration within %v", d.cfg.ReadyTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// linkObjects answers the objects of a configuration that hold the link
// certificate, as "kind/name": the services with an encrypted listener
// (or a forwarder node with an encrypted dialer) and the hops with an
// encrypted dialer. restart is set when re-creating one of those services
// through the web API would not do (measured with gost 3.2.6): a mux
// listener (mtls, mwss) closes its listening socket only, so the carriers
// it accepted run on, peers keep opening streams on them that nothing
// accepts any more, and only a new gost process closes them; a QUIC
// listener keeps its UDP socket while its connections live, so the new
// service cannot bind.
func linkObjects(c *gostConfig) (touched map[string]bool, restart bool) {
	touched = map[string]bool{}
	tlsNode := func(nodes []node) bool {
		for _, n := range nodes {
			if n.Dialer != nil && n.Dialer.TLS != nil || n.Connector != nil && n.Connector.TLS != nil {
				return true
			}
		}
		return false
	}
	for _, s := range c.Services {
		if s.Listener.TLS != nil || tlsNode(s.Forwarder.Nodes) {
			touched[kindServices+"/"+s.Name] = true
			if s.Listener.TLS != nil && slices.Contains([]string{"mtls", "mwss", "quic"}, s.Listener.Type) {
				restart = true
			}
		}
	}
	for _, h := range c.Hops {
		if tlsNode(h.Nodes) {
			touched[kindHops+"/"+h.Name] = true
		}
	}
	return touched, restart
}

// loadLink checks that the link certificate files load as gost loads
// them: the certificate with its key, and at least one CA certificate.
func (c Config) loadLink() error {
	if _, err := tls.LoadX509KeyPair(c.LinkCert, c.LinkKey); err != nil {
		return fmt.Errorf("gost driver: link certificate: %w", err)
	}
	ca, err := os.ReadFile(c.LinkCA)
	if err != nil {
		return fmt.Errorf("gost driver: link CA: %w", err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(ca) {
		return fmt.Errorf("gost driver: link CA: %s holds no PEM certificate", c.LinkCA)
	}
	return nil
}
