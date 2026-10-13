### Changed

- The root Go module is `github.com/AnixOps/anix-control/v5` (it was `.../v4`): `go.mod`, every import, the
  protobuf `go_package` of `api/grpc/v2board.proto` (regenerated), the CI package filters and the gates that
  name the path. Only the import path changes; the product version and every wire contract stay as they are.
  The SDK and identity modules keep their paths.
