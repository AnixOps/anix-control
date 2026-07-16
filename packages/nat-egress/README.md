# NAT Egress Reference Package

This directory is the reproducible package source for the 3.3 NAT egress
package. It follows the same software-package + WebUI plugin contract as the
3.2 reference packages, but it is not wired into production release signing yet
because the real Agent runtime and NAT/policy-routing namespace traffic evidence
are still pending.

The package currently proves:

- deterministic archive and manifest generation;
- public-key-only signature verification and tamper rejection;
- dependency-free `anixops.webui/v1` WebUI bundle registration;
- namespaced `/api/v3/plugins/nat-egress/status` backend contract.

Runtime execution must remain feature-gated until the Agent-side `nat-egress`
runtime, policy routing/NAT integration evidence, composed tunnel tests, and
canary records exist.

## Test

```bash
python3 -m unittest discover -s packages/nat-egress/tests -p 'test_*.py' -v
node packages/nat-egress/tests/webui_smoke.mjs
bash packages/nat-egress/tests/release_gate.sh
```
