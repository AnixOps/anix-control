### Removed

- UniProxy (`/api/v1/server/UniProxy/*`, `/api/v2/server/UniProxy/*`), the
  `v2board.*` gRPC services and the old V2Board subscription route
  `/api/v1/client/subscribe` are gone and answer 404 (gRPC `Unimplemented`).
  Nodes connect only over the mTLS Agent Control stream; third-party node
  software (V2bX, XrayR) can no longer connect. Clients use
  `/{subscribe_path}/<token>` (default `/s/<token>`).
- `grpc.api_token` (`ANIX_CONTROL_GRPC_API_TOKEN`) is ignored; remove it.
- `anix-control agents transports` no longer has a `third-party` status: a
  node that never connected over the stream is `unseen`.
- `scripts/install-agent.sh` (the V2bX migration installer, which could not
  download any Agent release) and the V2bX/XrayR client compatibility guide
  are removed.

### Changed

- The node configuration snapshot (`anixops.nodeconfig/v1`) carries the
  runtime configuration under `runtime`. `legacy_pull` still carries the
  same map for Agents of the previous release and is removed in the next
  one.
