### Added

- `grpc.edge_proxies` (`ANIX_CONTROL_GRPC_EDGE_PROXIES`): a reverse proxy on
  another machine terminates the nodes' TLS with the public certificate and
  sends their gRPC to Control in plain HTTP/2 over the private network. The
  gRPC listener admits the proxy's addresses only (any other connection is
  closed before the gRPC layer), takes the node's client certificate from
  `x-anix-client-cert` and its address from `x-forwarded-for`, and reads
  those headers from those addresses only. Control verifies the certificate
  itself and holds no gRPC certificate in this mode. Requires
  `tls.mode: external`; `grpc.tls_cert_file` and `tls.cert_file` must be unset.
- Deployment guides: [two-host deployment](docs/guide/deploy-two-hosts.md)
  (the panel on machine A, an nginx edge on machine B that holds every public
  certificate) and [single-host deployment](docs/guide/deploy-single-host.md).
  The nginx example is `config/deploy/examples/nginx/edge-two-hosts.conf`.

### Changed

- The configuration is ten operator keys: `grpc.edge_proxies` is read only in
  the edge mode.
- `init-secrets.sh` no longer warns about a missing gRPC certificate when
  `ANIX_CONTROL_GRPC_EDGE_PROXIES` is set.
