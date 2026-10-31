### Removed

- `POST /api/v4/kernel/network/migrate-gost` and the "Move gost routes to ANX" dialog on the Routes page. FR-test and the Agents hold no gost routes any more; a stored route that still has an ENGINE_GOST hop stays unplanned (flagged `gost_removed`) until it is edited onto the network model or deleted.
