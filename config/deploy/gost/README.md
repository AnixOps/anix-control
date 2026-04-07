# gost Relay Deployment

This directory contains relay-side material for the `NodeX/gost` runtime path.

## Scope

Deploying gost on a relay host only prepares the relay side.

That alone does not mean the relay is already attached to the panel runtime.

Real attachment requires the full chain below:

1. `v2board` runtime is configured for `gost`
2. `v2board` can reach the NodeX control-plane
3. NodeX can reach the relay gost API
4. the relay gost API token matches the `ForwardNode.api_token`
5. a tunnel and forward are created and synchronized successfully

## Expected Runtime Chain

```text
v2board -> NodeX control-plane -> relay gost API
```

Current implementation does not use the relay host as the outer control-plane address.

`forward.runtime.nodex.base_url` must point to NodeX.

The relay `ForwardNode.host` and `ForwardNode.api_port` are used by NodeX only after `v2board` has already reached the NodeX control-plane.

## Minimal Relay Requirements

Each relay host should expose:

- a reachable host or IP
- a gost management API port
- a gost API token
- gost with dynamic config endpoints enabled
  - current verified version: `3.2.6`
  - older `3.0.0-rc10` was not compatible with the current dynamic config flow
- optional metrics port if you want Prometheus scraping

Known-good relay config rules from the verified run:

- use top-level `api:` in `gost.yml`
- do not bind both top-level `api:` and a separate `services: api` listener to the same port
- relay API auth should be `admin:<RELAY_API_TOKEN>`
- NodeX creates services with raw service JSON, not a wrapped `{"data": ...}` body

The corresponding `ForwardNode` in `v2board` should match:

- `host`
- `api_port`
- `api_token`

## Example gost API checks

```bash
curl -u admin:<RELAY_API_TOKEN> http://<RELAY_HOST>:<API_PORT>/api/config/services
curl -u admin:<RELAY_API_TOKEN> http://<RELAY_HOST>:<API_PORT>/api/config/limiters
```

## Verify End-To-End Attachment

After relay deployment, finish the panel side:

1. configure `FORWARD_RUNTIME_BACKEND=gost`
2. configure `FORWARD_RUNTIME_NODEX_BASE_URL`
3. configure `FORWARD_RUNTIME_NODEX_TOKEN`
4. create the relay `ForwardNode`
5. create a tunnel and forward
6. confirm:
   - `v2_forward_runtime_job.backend='gost'` is `success`
   - relay gost contains the expected service

Current verified single-UI split:

- NodeX control-plane: `18081`
- relay gost API: `18080`

`forward.runtime.nodex.base_url` must point to the NodeX control-plane, not to the relay gost API port.

## Related Docs

- [`../../../docs/reference/runtime.md`](../../../docs/reference/runtime.md)
- [`../../../docs/guide/forward-relay-onboarding.md`](../../../docs/guide/forward-relay-onboarding.md)
- [`../../../docs/guide/forward-tunnel-runtime-ops.md`](../../../docs/guide/forward-tunnel-runtime-ops.md)
