# GOST Mesh Reference Package

This directory is the reproducible package source for the 3.3 tunnel-mesh
package. It follows the same software-package + WebUI plugin contract as the
3.2 reference packages, but it is not wired into production release signing yet
because the real Agent runtime and WSS/TUIC/QUIC namespace traffic evidence are
still pending.

The package currently proves:

- deterministic archive and manifest generation;
- public-key-only signature verification and tamper rejection;
- dependency-free `anixops.webui/v1` WebUI bundle registration;
- namespaced `/api/v3/plugins/gost-mesh/status` backend contract.

Runtime execution must remain feature-gated until the Agent-side `gost-mesh`
runtime, WSS/TUIC/QUIC integration evidence, NAT composition tests, and canary
records exist.

## Test

```bash
python3 -m unittest discover -s packages/gost-mesh/tests -p 'test_*.py' -v
node packages/gost-mesh/tests/webui_smoke.mjs
bash packages/gost-mesh/tests/release_gate.sh
```
