# nftables Forward Reference Package

This directory is the reproducible package source for the dedicated-line
`nftables-forward` Control + Agent + WebUI package. It contains package
metadata, configuration, and a dependency-free WebUI module. The Agent
executable is an explicit build input and is never replaced with a placeholder.
The current contract proves packaging and signing only; production forwarding
still requires the real Agent runtime and network-namespace TCP/UDP evidence.

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

## Build

The input binary must be a non-empty regular executable file. The package
builder does not compile or download it.

```bash
python3 packages/nftables-forward/build.py build \
  --agent-binary /absolute/path/to/nftables-forward-agent \
  --goos linux \
  --goarch amd64 \
  --output-dir /tmp/nftables-forward-dist
```

The output directory contains:

- `nftables-forward-1.0.0.tar`: deterministic combined package artifact;
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
  --artifact /tmp/nftables-forward-dist/nftables-forward-1.0.0.tar \
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

bash packages/nftables-forward/tests/release_gate.sh
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
