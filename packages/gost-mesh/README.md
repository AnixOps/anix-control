# GOST Mesh Package

This directory is the reproducible Control + Agent + WebUI package source for
the 3.3 `gost-mesh` plugin. A release build requires two real executable inputs:

- the AnixOps `gost-mesh` Agent plugin executable;
- the official GOST v3.2.6 executable whose platform SHA-256 is pinned in
  `runtime-contract.json`.

Both executables are covered by the signed package artifact. The manifest
declares `agent-<goos>-<goarch>` and
`runtime-gost-<goos>-<goarch>` entrypoints. The latter points to
`runtime/<goos>-<goarch>/gost`; `package.json` and `build-report.json` bind its
version, source-archive checksum, binary checksum, path, size, and executable
mode. A host-provided `gost` found through `PATH` is not an acceptable release
runtime.

GOST Mesh v1 implements WSS and QUIC only. TUIC and combined protocol profiles
are deliberately absent because the pinned GOST runtime does not implement
TUIC. The signed capabilities also declare tunnel health, policy routing,
`plugin.runtime-state`, and `plugin.cleanup`. The last two capabilities require
the Agent Supervisor to retain a private ownership journal and invoke the signed
cleanup entrypoint after crashes, disable, update, rollback, or Agent restart.

## Configuration

The signed configuration is one aggregate `anixops.gost-mesh/v1` document:

- `apply=false` with `tunnels=[]` is the observation-safe default;
- `apply=true` requires at least one tunnel;
- every tunnel has one `entry` or `exit` role and one `wss` or `quic`
  transport;
- entry tunnels require a remote endpoint, source CIDRs, verified server name,
  CA file, and a client certificate/key;
- exit tunnels require a listen endpoint, route CIDRs, server certificate/key,
  and a client CA;
- TUN addresses and routing CIDRs are IPv4-only in v1;
- each package document contains at most 128 unique tunnel objects; entry
  routing tables are in `1..252` and priorities are in `1..32765`; exit
  `routing.table` and `routing.priority` are unused and must be omitted or `0`;
- the complete configuration JSON is limited to 256 KiB before schema or
  runtime validation, and TUN MTU is in `576..9000`;
- WSS requires an absolute path beginning with `/`; QUIC requires an empty
  `wss_path`;
- `rollback_on_exit` is fixed to `true`.

WSS and QUIC both use mandatory mutual TLS. The entry verifies the exit with
`ca_file` plus `server_name` and presents `cert_file`/`key_file`; the exit uses
its certificate/key and requires entry certificates signed by `ca_file`.
Leaving the relay reachable without client authentication is not a supported
v1 mode. Enabled health checks also require `health.source_address`, which must
belong to an entry `routing.source_cidrs`; the probe therefore follows the same
source-policy route as business traffic.

Before apply, Linux must already have `net.ipv4.ip_forward=1` and reverse-path
filtering disabled (`net.ipv4.conf.*.rp_filter=0`). The plugin validates these
conditions and fails closed instead of changing host-wide sysctls.

The JSON Schema rejects unsafe field shapes before deployment. The Control
semantic validator enforces the 256 KiB limit on the complete JSON document and
rejects constraints that JSON Schema cannot express: canonical and
non-overlapping CIDRs, TUN peer membership, unique IDs, TUN names, entry routing
tables and priority ranges, `health.timeout_seconds < health.interval_seconds`,
health source containment, safe literal health targets, clean absolute TLS
paths, distinct certificate/key files, canonical enum strings, and integer JSON
lexemes for integer fields (for example, `443.0` is rejected). The Agent
independently enforces the same semantic boundary before any process or network
operation.

TLS fields are paths to private files already materialized on the Agent. They
are not secret values and the manifest therefore declares no `secret_fields`.
Secure Secret ID to private-file materialization, renewal, deletion, and audit
must be complete before a stable production rollout. Until then, execution
remains canary-gated even though the package contract is reproducible.

The runtime needs the namespaced `gost-mesh.network-admin` and
`gost-mesh.process-exec` permissions. The WebUI remains read-only and receives
only `gost-mesh.view` for the namespaced
`/api/v3/plugins/gost-mesh/status` route.

## Test

```bash
python3 -m unittest discover -s packages/gost-mesh/tests -p 'test_*.py' -v
python3 packages/gost-mesh/build.py self-test
GOWORK=off go test ./packages/gost-mesh/tests
node packages/gost-mesh/tests/webui_smoke.mjs
bash packages/gost-mesh/tests/release_gate.sh \
  --agent-binary /path/to/gost-mesh-agent \
  --gost /path/to/pinned/gost-v3.2.6
```

The builder self-test uses a hermetic executable fixture to test archive and
tamper mechanics only. The release gate has no fixture fallback and accepts
only a real versioned Agent binary plus the exact pinned GOST binary. It also
runs every case in `tests/semantic_contract_cases.json` through the Agent's
`--anixops-validate` mode. Valid cases include 128 semantically non-conflicting
tunnels; invalid cases cover duplicate objects, canonical CIDRs, TUN peers,
health timing and source containment, certificate/key separation, reserved
literal targets, exit route allocation, and the 256 KiB document boundary.
Validation mode must not start GOST or mutate network state.
