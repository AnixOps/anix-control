# V2Board AnixOps Guide

This folder is the deep-dive layer of the documentation tree.

Start elsewhere first when possible:

- overview and boundary: [`../intro/README.md`](../intro/README.md)
- startup and config examples: [`../reference/README.md`](../reference/README.md)
- repository ownership: [`../reference/repository-layout.md`](../reference/repository-layout.md)
- runtime entrypoint: [`../reference/runtime.md`](../reference/runtime.md)

Use this folder when you need implementation detail, clone contracts, runtime operations, or smoke-test checklists.

## Documents

| Document | Purpose |
|------|------|
| [Flux-panel Clone Guide](flux-panel-clone.md) | Source-of-truth workflow for future `flux-panel` 1:1 cloning work |
| [Flux Forward Contract](flux-forward-contract.md) | Concrete forward/tunnel endpoint mapping, DTO fields, auth scope and known gaps |
| [Flux-panel Clone Workstream](flux-panel-workstream.md) | Current module status, reference mapping, implementation order and definition of done |
| [Node Management](node-management.md) | Node registration, heartbeat, protocol config and operations |
| [Client Compatibility](client-compatibility.md) | V2bX/XrayR and related compatibility notes |
| [API Reference](api-reference.md) | Existing project API overview |
| [Subscription System](subscription-system.md) | Subscription groups, templates and formatting |

## Recommended Reading Order

If the task is to continue cloning `flux-panel`:

1. Read [Flux-panel Clone Guide](flux-panel-clone.md).
2. Read [Flux Forward Contract](flux-forward-contract.md).
3. Read [Flux-panel Clone Workstream](flux-panel-workstream.md).
4. Inspect the local reference repo at `C:\Users\z7299\AppData\Local\Temp\flux-panel`.
5. Only then start changing routes, services, DTOs or pages.

If the task is not related to `flux-panel`, use the other topic-specific guides.
