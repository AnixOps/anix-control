# Forward Runtime Work Plan

This document turns the current dual-runtime state into concrete parallel work packages.

It is intentionally implementation-oriented so multiple agents can work in parallel without stepping on each other.

## Program Goals

The next stage should achieve all of these:

1. operators can tell whether a relay is only saved or actually attached
2. NodeX mode and local Ansible mode stay explicitly separated in UI copy, backend validation, and diagnostics
3. the Flux-cloned `/admin/forward*` surface stays clean while proprietary execution controls stay discoverable
4. onboarding, doctor, version, upgrade, and smoke-test flows become copyable and predictable

## Work Package 1: Runtime UX Copy And Navigation

Area:
- `web/src/views/admin/System.vue`
- runtime-related admin copy
- docs navigation

Goal:
- make operators understand the mode split without reading source code

Tasks:
- rename ambiguous labels such as generic "entry node" when the mode is local Ansible
- add direct links or help copy to relay onboarding and smoke-test docs
- ensure runtime pages keep clear ownership:
  - `NodeX Topology` for relay/exit semantics
  - `Ansible Machines` for stateless execution-machine records
  - `Local Runtime` for panel-host ansible executor settings

Dependencies:
- existing runtime docs and onboarding docs

Definition of done:
- no UI copy implies that every forward node is already attached
- no UI copy suggests ansible always goes through NodeX
- operator can navigate from system/runtime screen to onboarding and diagnostics in one hop

## Work Package 2: Forward And Tunnel Form Semantics

Area:
- `web/src/views/admin/Forward.vue`
- `web/src/views/admin/Tunnel.vue`
- matching request/response DTO handling

Goal:
- keep Flux clone fidelity while fixing mode-specific semantic confusion

Tasks:
- review whether ingress-node selectors are shown only where they are semantically valid
- make execution-node vs ingress-node semantics explicit in validations and helper text
- ensure Flux-compatible fields stay unchanged while proprietary mode hints remain outside the clone contract

Dependencies:
- Work Package 1 copy rules
- existing Flux contract docs

Definition of done:
- NodeX mode keeps ingress semantics
- ansible mode does not falsely require ingress semantics
- clone DTOs and response envelopes remain aligned

## Work Package 3: Backend Runtime Truth And Health Semantics

Area:
- `internal/service/forward_panel_runtime_service.go`
- `internal/service/forward_node_service.go`
- runtime diagnostics handlers and DTOs

Goal:
- stop the backend from implying that TCP reachability equals full attachment

Tasks:
- separate coarse reachability from runtime attachment state in diagnostics responses
- surface mode-specific readiness hints:
  - NodeX base URL/token
  - relay gost API reachability
  - ansible command/inventory readiness
- review whether `ForwardNode` health output needs explicit labels such as `reachability` vs `runtimeReady`

Dependencies:
- none; can start immediately

Definition of done:
- API consumers can distinguish TCP-up from runtime-attached
- NodeX and ansible failure causes are diagnosable without reading logs first

## Work Package 4: NodeX Relay Onboarding And Packaging

Area:
- `NodeX/control-plane`
- `NodeX/docs`
- install or wrapper tooling

Goal:
- make the NodeX side feel like a real operator product instead of scattered internal notes

Tasks:
- standardize control-plane startup examples around `--forward-api-token`
- improve relay onboarding docs with concrete gost deployment recipes and acceptance checks
- add or refine helper commands for:
  - version
  - runtime status
  - doctor
  - relay onboarding checks

Dependencies:
- current NodeX docs baseline

Definition of done:
- a fresh operator can start NodeX and verify relay attachment from one doc chain
- every command in the onboarding flow is copyable

## Work Package 5: Local Ansible Operator Path (`nftables_ansible` Default)

Area:
- `config/deploy/ansible/`
- runtime executor docs
- installation/bootstrap scripts

Goal:
- make stateless ansible execution as turnkey as NodeX mode, with `nftables_ansible` as the recommended default and `iptables_ansible` as legacy compatibility

Tasks:
- document key-login and password-login inventory examples under the YAML-only runtime flow
- standardize the minimal password-login and key-login examples
- define exact executor-host prerequisites:
  - `ansible-playbook`
  - inventory path
  - sudo/become behavior
- verify one-click bootstrap still matches:
  - `forward_runtime.nftables_ansible.*` (recommended)
  - `forward_runtime.iptables_ansible.*` (legacy compatibility)

Dependencies:
- existing ansible assets

Definition of done:
- operators can tell exactly where SSH credentials live
- no doc suggests `ForwardNode` stores SSH secrets
- ansible smoke path is executable end to end

## Work Package 6: Smoke Tests And SOP Automation

Area:
- docs
- helper scripts
- lightweight verification commands

Goal:
- make manual verification cheap and consistent

Tasks:
- add minimal smoke command sets for:
  - NodeX mode
  - ansible mode
- standardize SQL checks against `v2_forward_runtime_job`
- define operator pass/fail criteria for relay attachment
- if useful, add wrapper scripts that collect:
  - NodeX status
  - relay API response
  - job status snapshot

Dependencies:
- Work Packages 3, 4, and 5 are helpful but not mandatory

Definition of done:
- every runtime path has a short smoke checklist and a clear acceptance result

## Work Package 7: Flux Clone Gap Closure

Area:
- forward/tunnel/limit/user-tunnel pages
- associated handlers/services/tests

Goal:
- continue Flux clone work without polluting the clone surface with proprietary runtime controls

Tasks:
- finish remaining runtime semantics that matter to Flux behavior:
  - diagnose path semantics
  - quota and expiry side effects
  - runtime-side speed-limit propagation
- finish last-mile UI parity for `user.tsx` and `limit.tsx`

Dependencies:
- keep proprietary runtime docs separate from clone docs

Definition of done:
- clone progress can move forward independently of internal runtime packaging

## Suggested Parallel Split

If you open five agents, use this split:

1. `v2board` docs and operator navigation
2. `v2board` backend diagnostics and health semantics
3. `v2board` forward/tunnel UI semantics and copy
4. `NodeX` onboarding, doctor, and control-plane docs/tooling
5. `ansible` bootstrap, inventory path, and smoke-test SOP

## Shared Preconditions

Before any agent starts editing code, keep these truths fixed:

1. creating a `ForwardNode` is not the same as runtime attachment
2. `NodeX/gost` means `v2board -> NodeX -> relay gost API`
3. `nftables_ansible` means local stateless `v2board` executor by default; `iptables_ansible` is legacy compatibility on the same path
4. panel forward-node online state is only coarse `host:port` TCP reachability

If a task changes one of those truths, update these docs first:

- [`forward-relay-onboarding.md`](forward-relay-onboarding.md)
- [`forward-tunnel-runtime-ops.md`](forward-tunnel-runtime-ops.md)
- [`forward-tunnel-smoke-test.md`](forward-tunnel-smoke-test.md)
- [`../reference/runtime.md`](../reference/runtime.md)

## First Five-Agent Kickoff

If the next round starts immediately, use this order:

1. Agent A
   - tighten backend diagnostics and health semantics
   - target `internal/service/forward_panel_runtime_*` and related DTOs
2. Agent B
   - clean up tunnel and forward form semantics
   - target `web/src/views/admin/Forward.vue` and `web/src/views/admin/Tunnel.vue`
3. Agent C
   - improve NodeX relay bootstrap and doctor flow
   - target `NodeX/control-plane`, `NodeX/tools`, and NodeX docs
4. Agent D
   - harden ansible bootstrap and inventory behavior
   - target `config/deploy/ansible/`, bootstrap scripts, and executor docs
5. Agent E
   - maintain docs, SOP, and smoke automation
   - target workstream docs, onboarding docs, and helper smoke scripts

## Source Of Truth Docs

Before starting any work package, read:

- [`forward-relay-onboarding.md`](forward-relay-onboarding.md)
- [`forward-tunnel-runtime-ops.md`](forward-tunnel-runtime-ops.md)
- [`forward-tunnel-smoke-test.md`](forward-tunnel-smoke-test.md)
- [`../reference/runtime.md`](../reference/runtime.md)
- [`../reference/startup-config.md`](../reference/startup-config.md)
