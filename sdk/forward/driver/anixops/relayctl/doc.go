// Package relayctl is the contract between the anixops forward driver
// (sdk/forward/driver/anixops, which runs in the Agent) and the relay process
// it controls (sdk/forward/driver/anixops/relayd, the `anixops-relay` unit):
// the relay's configuration document, which the driver renders and the relay
// loads, and the control API the driver reaches the relay with over a unix
// socket (docs/architecture/anixops-protocol.md section 6.1).
//
// It is plain data and a small client, so the Agent's driver does not link
// the transport library (QUIC and all) and the relay does not link the driver.
//
// # The configuration
//
// Config is what Render writes and the relay runs: the driver's ownership
// mark, where the link files are, and per hop its listener, ingress, admitted
// sources and peers, upstreams, strategy, breaker, limits and pause. Every
// field is a checked literal; the document is deterministic (fixed field
// order, hops sorted by route and index, upstreams in the state's order), so
// its digest is the artifact's digest.
//
// Fields split in two by what changing them costs a running relay:
//
//   - the listener of a hop (ListenerKey: address, port, protocols, ingress
//     security and carrier) is structural: a hop whose listener changes gets a
//     new listener, a new counter epoch, and its old carriers are closed (L1);
//   - everything else (upstreams, balance, breaker, limits, sources, peers,
//     pause) is hot: the relay changes it in place, keeping the listener,
//     established connections, carriers and the counter epoch.
//
// # The control API
//
// HTTP with JSON bodies on a unix socket (the socket's file permissions are
// its only key, as gost's web API's are):
//
//	GET  /v1/status       Status: the running instance, the digest of the
//	                      configuration it runs and whether each hop listens
//	PUT  /v1/config       Config: validate the whole document, pre-bind every
//	                      new listener, then change hop by hop; any failure
//	                      changes nothing (code "conflict" for a port another
//	                      process holds)
//	GET  /v1/observe      Observation: counters, rotation and upstream health
//	                      per hop, and the final counters of retired epochs
//	PUT  /v1/rotation     Rotation: the upstreams of one hop in rotation now
//	POST /v1/credentials  re-read the link files; no connection ends unless its
//	                      peer is no longer trusted
package relayctl
