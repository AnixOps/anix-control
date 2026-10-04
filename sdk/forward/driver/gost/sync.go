package gost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// The kinds of objects Apply changes through gost's web API, as the API's
// paths name them.
const (
	kindServices   = "services"
	kindChains     = "chains"
	kindHops       = "hops"
	kindAdmissions = "admissions"
	kindLimiters   = "limiters"
	kindCLimiters  = "climiters"
)

// apiObject is one object of a configuration as the web API takes it.
type apiObject struct {
	kind, name string
	body       any
}

func (o apiObject) key() string { return o.kind + "/" + o.name }

// objects answers every object of a configuration that the web API
// changes, in the order the configuration lists them: services, chains
// and the hot objects. Hops carry the API's spelling (failTimeout in
// nanoseconds).
func objects(c *gostConfig) ([]apiObject, error) {
	var out []apiObject
	for _, s := range c.Services {
		out = append(out, apiObject{kindServices, s.Name, s})
	}
	for _, ch := range c.Chains {
		out = append(out, apiObject{kindChains, ch.Name, ch})
	}
	for _, h := range c.Hops {
		body, err := toAPIHop(h)
		if err != nil {
			return nil, err
		}
		out = append(out, apiObject{kindHops, h.Name, body})
	}
	for _, a := range c.Admissions {
		out = append(out, apiObject{kindAdmissions, a.Name, a})
	}
	for _, l := range c.Limiters {
		out = append(out, apiObject{kindLimiters, l.Name, l})
	}
	for _, l := range c.CLimiters {
		out = append(out, apiObject{kindCLimiters, l.Name, l})
	}
	return out, nil
}

// globals answers the parts of a configuration the web API cannot
// change: the log, the API and where metrics are served (not the path,
// which names the structure gost loaded). Configurations whose globals
// differ need a reload.
func globals(c *gostConfig) ([]byte, error) {
	g := struct {
		Log     *logConfig `json:"log"`
		API     *apiBlock  `json:"api"`
		Metrics string     `json:"metrics"`
	}{Log: c.Log, API: c.API}
	if c.Metrics != nil {
		g.Metrics = c.Metrics.Addr
	}
	return json.Marshal(g)
}

// changes answers the objects Apply changes to take a running gost from
// prev to next: every service, chain, admission and limiter that was
// added, removed or changed, and every hop (putting each hop again also
// puts back every upstream SetUpstreams took out of rotation, as a
// changing apply must). Objects that did not change, an admission
// EnforceQuotas closed among them, are left alone.
func changes(prev, next []apiObject) map[string]bool {
	enc := func(objs []apiObject) map[string][]byte {
		m := map[string][]byte{}
		for _, o := range objs {
			b, _ := json.Marshal(o.body)
			m[o.key()] = b
		}
		return m
	}
	p, n := enc(prev), enc(next)
	out := map[string]bool{}
	for k, b := range n {
		if old, ok := p[k]; !ok || !bytes.Equal(old, b) {
			out[k] = true
		}
	}
	for k := range p {
		if _, ok := n[k]; !ok {
			out[k] = true
		}
	}
	for _, o := range next {
		if o.kind == kindHops {
			out[o.key()] = true
		}
	}
	return out
}

// syncLog records what sync did, for its caller and for a rollback.
type syncLog struct {
	// touched are the objects sync changed, or may have changed (a request
	// that failed in transport); a request gost refused changed nothing
	// and is not in it.
	touched map[string]bool
	// services are the services sync deleted or created: their hops'
	// counter epochs end.
	services map[string]bool
}

func newSyncLog() *syncLog {
	return &syncLog{touched: map[string]bool{}, services: map[string]bool{}}
}

// sync makes the running gost, whose objects running names ("kind/name"),
// run target for the objects in touched, through the web API, in an
// order that keeps every reference resolvable and frees listen ports
// before they are taken again:
//
//  1. delete the touched services that run (a changed service is
//     deleted and created again: gost's PUT closes the old service first
//     and leaves it closed when the new one fails);
//  2. create or replace the touched admissions, limiters, hops and chains
//     of target;
//  3. create the touched services of target;
//  4. delete the touched chains, hops, admissions and limiters target no
//     longer has.
//
// It stops at the first error; log says what it did until then.
func (d *Driver) sync(ctx context.Context, target []apiObject, touched, running map[string]bool, log *syncLog) error {
	do := func(method string, o apiObject) error {
		path := "/config/" + o.kind
		var body any
		switch method {
		case http.MethodPost:
			body = o.body
		case http.MethodPut:
			path += "/" + url.PathEscape(o.name)
			body = o.body
		case http.MethodDelete:
			path += "/" + url.PathEscape(o.name)
		}
		err := d.call(ctx, method, path, body, nil)
		if err == nil || !refused(err) {
			log.touched[o.key()] = true
			if o.kind == kindServices && err == nil {
				log.services[o.name] = true
			}
		}
		if err == nil {
			running[o.key()] = method != http.MethodDelete
		}
		return err
	}
	inTarget := map[string]bool{}
	for _, o := range target {
		inTarget[o.key()] = true
	}
	// Touched objects that run, in a stable order, by kind.
	stale := func(kind string) []apiObject {
		var out []apiObject
		for k := range touched {
			if name, ok := strings.CutPrefix(k, kind+"/"); ok && running[k] {
				out = append(out, apiObject{kind: kind, name: name})
			}
		}
		slices.SortFunc(out, func(a, b apiObject) int { return strings.Compare(a.name, b.name) })
		return out
	}

	for _, o := range stale(kindServices) {
		if err := do(http.MethodDelete, o); err != nil {
			return err
		}
	}
	for _, kind := range []string{kindAdmissions, kindLimiters, kindCLimiters, kindHops, kindChains} {
		for _, o := range target {
			if o.kind != kind || !touched[o.key()] {
				continue
			}
			method := http.MethodPost
			if running[o.key()] {
				method = http.MethodPut
			}
			if err := do(method, o); err != nil {
				return err
			}
		}
	}
	for _, o := range target {
		if o.kind == kindServices && touched[o.key()] {
			if err := do(http.MethodPost, o); err != nil {
				return err
			}
		}
	}
	for _, kind := range []string{kindChains, kindHops, kindAdmissions, kindLimiters, kindCLimiters} {
		for _, o := range stale(kind) {
			if inTarget[o.key()] {
				continue
			}
			if err := do(http.MethodDelete, o); err != nil {
				return err
			}
		}
	}
	return nil
}

// rollback puts back the objects a failed sync touched as prev has them,
// re-reading what runs first. It answers what it did in log.
func (d *Driver) rollback(ctx context.Context, prev []apiObject, done, log *syncLog) error {
	live, err := d.readLive(ctx)
	if err != nil {
		return fmt.Errorf("gost driver: roll back: %w", err)
	}
	if err := d.sync(ctx, prev, done.touched, live.names(), log); err != nil {
		return fmt.Errorf("gost driver: roll back: %w", err)
	}
	return nil
}

// applyStrands reports whether taking the running gost from prev to
// content through the web API would delete or re-create a running
// service whose listener is a mux or QUIC carrier (muxCarrier): its
// accepted carriers would outlive it, so its peers would keep opening
// streams nothing accepts (or, for QUIC, the new service could not bind
// while they live). Apply restarts gost instead. A service only added
// has no carriers yet and is created through the web API.
func applyStrands(prev *gostConfig, content []byte, live *liveConfig) bool {
	next, err := decodeConfig(content)
	if err != nil {
		return true
	}
	from, err1 := objects(prev)
	to, err2 := objects(next)
	if err1 != nil || err2 != nil {
		return true
	}
	running := live.names()
	touched := changes(from, to)
	for _, s := range prev.Services {
		k := kindServices + "/" + s.Name
		if touched[k] && running[k] && muxCarrier(s.Listener.Type) {
			return true
		}
	}
	return false
}

// hasMuxListener reports whether the configuration content, the file a
// running gost loaded, has a mux or QUIC listener, or cannot be read (so
// what gost runs is unknown).
func hasMuxListener(content []byte) bool {
	if content == nil {
		return true
	}
	c, err := decodeConfig(content)
	if err != nil {
		return true
	}
	for _, s := range c.Services {
		if muxCarrier(s.Listener.Type) {
			return true
		}
	}
	return false
}
