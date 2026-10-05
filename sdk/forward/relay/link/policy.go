package link

import (
	"errors"
	"sync/atomic"
)

// policy is what a listener admits, shared by every kind of listener (the
// TCP Listener and the QUICListener) so that the two cannot drift apart: the
// credentials its handshakes run under, the ALPN protocol, the hop's
// ingress_sources and, on an encrypted listener, its ingress_peers. Sources and
// peers are replaced in place and read on every connection; the credentials
// change through Credentials.Reload.
type policy struct {
	creds    *Credentials // nil on a plaintext listener
	protocol string

	sources atomic.Pointer[Sources]
	peers   atomic.Pointer[peerSet]
}

// init validates and stores the policy. A plaintext policy (creds nil) has
// sources only; an encrypted one also needs the protocol and at least one
// peer, since "admit everybody" is not a state.
func (p *policy) init(creds *Credentials, protocol string, sources, peers []string) error {
	s, err := ParseSources(sources)
	if err != nil {
		return err
	}
	if creds != nil {
		ps, err := newPeerSet(peers)
		if err != nil {
			return err
		}
		p.peers.Store(ps)
	}
	p.creds, p.protocol = creds, protocol
	p.sources.Store(&s)
	return nil
}

func (p *policy) setSources(items []string) error {
	s, err := ParseSources(items)
	if err != nil {
		return err
	}
	p.sources.Store(&s)
	return nil
}

func (p *policy) setPeers(ids []string) (removed []string, err error) {
	if p.creds == nil {
		return nil, errors.New("link: a plaintext listener has no peer identities")
	}
	next, err := newPeerSet(ids)
	if err != nil {
		return nil, err
	}
	prev := p.peers.Swap(next)
	return prev.without(next), nil
}

func (p *policy) peerAllowed(id string) bool { return p.peers.Load().has(id) }
