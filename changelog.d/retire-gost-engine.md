### Removed

- The gost forward engine is retired. Control refuses a route with an
  `ENGINE_GOST` hop on create and update. A stored route still on gost is
  no longer planned: it shows "gost removed" and keeps its ports until it
  is moved to ANX (`POST /api/v4/kernel/network/migrate-gost`) or deleted.
  The SDK's gost driver (`sdk/forward/driver/gost`), its rendered goldens
  and the gost end-to-end tests are gone. The route editor offers nftables
  and ANX, and no longer suggests inserting a gost relay. The SDK still
  validates and plans `ENGINE_GOST`, so the contract fixtures stay readable.
  The cross-repo Agent end-to-end suite runs its forward route on nftables
  in the netns lane only; the main lane skips it.
