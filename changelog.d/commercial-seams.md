### Changed

- The commercial set links into Control through files of its own: the package
  host table (`builtin.Register`), the `/api/v2` payment, order, coupon and
  invite routes (`router_commercial.go`), the payment gateways and order
  reconciler, and the KernelOrder provider. Behaviour is unchanged; a build
  without those files is the community edition. The v2 route inventory reads
  every `router_*.go` next to `router.go`.
