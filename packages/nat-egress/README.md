# NAT Egress Package

This directory is the reproducible Control + Agent + WebUI package source for
the 3.3 `nat-egress` plugin. The package takes the real Agent executable as an
explicit build input; it never substitutes a placeholder into a release
artifact.

The Agent runtime is implemented as a Linux plugin process supervised over a
private Unix socket. Its signed JSON configuration controls:

- nftables `inet` postrouting masquerade for IPv4 and/or IPv6;
- fwmark policy rules and copied default routes in an isolated routing table;
- marked, interface-bound TCP health probes reported through the versioned
  local gRPC health service;
- snapshot-aware rollback of nftables state and removal only of policy state
  created by the plugin.

The signed manifest declares `plugin.runtime-state` and `plugin.cleanup`, so the
Agent Supervisor supplies a stable private ownership journal across process and
Agent restarts and invokes the signed cleanup entrypoint after crashes, disable,
update, and rollback transitions. The package also declares
`forward.mark.consume`: it consumes the configured fwmark but does not assign it.
An activated topology must pair it with an upstream plugin or host rule that
declares the matching mark-producer contract.

The default configuration is observation-safe: `apply=false` prevents network
mutation, while `rollback_on_exit=true` protects an applied canary. Applying the
runtime requires the namespaced `nat-egress.network-admin` permission. The WebUI
receives only `nat-egress.view` and reads the namespaced
`/api/v3/plugins/nat-egress/status` route.

Before `apply=true`, the host must enable the selected address-family forwarding
sysctl, permit the intended traffic through its FORWARD firewall policy, reserve
the configured mark/table/priority, and provide exactly one default route on the
selected egress interface. The runtime validates forwarding and policy-route
conflicts but intentionally does not weaken an existing host firewall.

The package contract proves deterministic archive and manifest generation,
public-key-only signature verification, tamper rejection, strict configuration
schema binding, and dependency-free `anixops.webui/v1` registration. Production
signing remains disabled until the Control release workflow pins the Agent
runtime revision and privileged namespace NAT/policy-routing evidence is
recorded. Canary and rollback approval remain separate operational gates.

## Test

```bash
python3 -m unittest discover -s packages/nat-egress/tests -p 'test_*.py' -v
python3 packages/nat-egress/build.py self-test
GOWORK=off go test ./packages/nat-egress/tests
node packages/nat-egress/tests/webui_smoke.mjs
bash packages/nat-egress/tests/release_gate.sh
```
