# nftables Forward Reference Package

This directory is the reproducible package source for the dedicated-line
`nftables-forward` Control + Agent + WebUI package. It contains package
metadata, configuration, and a dependency-free WebUI module. The Agent
executable is an explicit build input and is never replaced with a placeholder.
The Control release workflow builds that executable from the pinned
`AnixOps/anix-agent` source and publishes a signed package on release tags.
The pinned Agent repo includes privileged namespace acceptance for TCP DNAT,
UDP DNAT, plugin-created table rollback, and pre-existing nftables table
snapshot restoration. Production forwarding still requires staging restore
smoke, canary rollout records, legacy fallback rehearsal, and operator approval.

## Source Layout

```text
packages/nftables-forward/
|-- build.py
|-- verify_signature.py
|-- config.defaults.json
|-- config.schema.json
|-- manifest.template.json
|-- tests/release_gate.sh
|-- tests/test_build.py
|-- tests/test_verify_signature.py
|-- tests/webui_smoke.mjs
`-- webui/index.mjs
```

`build.py` creates one combined package archive. For a Linux AMD64 build the
archive contains:

```text
agent/linux-amd64/plugin
config.defaults.json
config.schema.json
package.json
webui/index.mjs
```

The generated signed-manifest input declares both `control` and `agent`
targets and maps `agent-linux-amd64` to
`agent/linux-amd64/plugin`. Other GOOS/GOARCH pairs use the same naming rule.

Version 1.2 uses the same configuration document end to end in the Control
form, topology preview, Agent Supervisor, and plugin runtime. The safe default
is `apply=false` with an empty `rules` array, so installing and enabling the
package does not claim traffic. An operator must add explicit TCP or UDP rules
and set `apply=true` before nftables changes are admitted. `rollback_on_exit`
is fixed to `true`; the Agent supplies a private `--anixops-state` path where
the runtime journals the pre-existing table snapshot before applying a plan.
The `plugin.runtime-state`, `kernel.observed-state`, and `plugin.cleanup`
capabilities let Supervisor recover that journal after a process or Agent
crash. Version 1.2 also exports a private, bounded live-kernel fingerprint and
per-rule packet/byte counters; the Supervisor binds those values to its own
version, config hash, and operation revisions before Control can use them for
topology promotion.

## Build

The input binary must be a non-empty regular executable file. The package
builder does not compile or download it; CI passes the pinned Agent
`cmd/nftables-forward` binary into this builder.

```bash
python3 packages/nftables-forward/build.py build \
  --agent-binary /absolute/path/to/nftables-forward-agent \
  --goos linux \
  --goarch amd64 \
  --output-dir /tmp/nftables-forward-dist
```

The output directory contains:

- `nftables-forward-1.2.0.tar`: deterministic combined package artifact;
- `manifest.json`: canonical manifest bytes to sign with the official Ed25519
  release key;
- `build-report.json`: input and output digests without timestamps or host
  paths;
- `SHA256SUMS.txt`: SHA-256 checksums for every generated output except itself.

The tar writer fixes entry order, ownership, permissions, timestamps, and
format. Identical source and Agent binary bytes therefore produce an identical
artifact and manifest. Reproducibility of the input Agent binary remains the
responsibility of its Go release build. The builder enforces the current
32 MiB Control/Agent artifact limit and 2 MiB WebUI bundle limit.

## Verify

Verification checks the outer checksums, manifest-to-artifact binding, WebUI
digest, architecture entrypoint, tar metadata, package file index, and every
inner file digest.

```bash
python3 packages/nftables-forward/build.py verify \
  --output-dir /tmp/nftables-forward-dist
```

The builder runs the same verification automatically before reporting a
successful build.

The release contract verifies an external Ed25519 signature without requiring
a private key in the package source tree. The public key may be PEM or the raw
Base64 value used by `plugins.official_public_key`:

```bash
python3 packages/nftables-forward/verify_signature.py \
  --manifest /tmp/nftables-forward-dist/manifest.json \
  --artifact /tmp/nftables-forward-dist/nftables-forward-1.2.0.tar \
  --signature /path/to/manifest.sig \
  --public-key /path/to/official-public-key.pem
```

## Test

The tests use a temporary executable fixture only to exercise the packer. No
fixture artifact is emitted into the source tree.

```bash
python3 -m unittest discover \
  -s packages/nftables-forward/tests \
  -p 'test_*.py' \
  -v

python3 packages/nftables-forward/build.py self-test

node packages/nftables-forward/tests/webui_smoke.mjs

GOEXPERIMENT=jsonv2 GOWORK=off \
  go -C /path/to/anix-agent build -o /tmp/nftables-forward-agent \
  ./cmd/nftables-forward
bash packages/nftables-forward/tests/release_gate.sh \
  --agent-binary /tmp/nftables-forward-agent
```

The runtime data-plane acceptance lives with the Agent source because it needs
to execute the real Linux plugin binary in privileged network namespaces:

```bash
GOEXPERIMENT=jsonv2 GOWORK=off \
  go -C /path/to/anix-agent build -o /tmp/nftables-forward-agent \
  ./cmd/nftables-forward

sudo bash /path/to/anix-agent/plugin/nftablesforward/namespace_acceptance.sh \
  --agent-binary /tmp/nftables-forward-agent
```

The test suite builds twice and compares every output byte, verifies archive
metadata and digests, rejects a tampered artifact, rejects a non-executable
Agent input, and enforces that `webui/index.mjs` has no static or dynamic
imports. The Node smoke test executes the factory with the versioned host API,
checks the namespace-constrained request, and renders a nftables response
without Vue or other installed dependencies.

## Signing Boundary

This directory never stores a release private key or emits a signature. Sign
the exact generated `manifest.json` bytes in the release pipeline, then submit
the Base64 signature with the manifest and artifact through the Control package
repository API.

`tests/release_gate.sh` is the CI contract: it builds twice, verifies the
unsigned package, creates only a temporary test key, checks PEM and raw
Base64 public-key verification, and rejects missing or tampered signatures.
The temporary signing key is deleted before the script exits and is never
uploaded.

The WebUI module does not embed its own SHA-256. Doing so would create an
unsatisfiable self-hash. The signed manifest binds the bundle path and digest,
and the Control and browser loaders verify those bytes before import.
