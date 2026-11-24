### Changed

- Comments, documents and admin form text describe ANX only through its
  public boundary names: no private spec or roadmap references, no mechanism
  or rationale. The ANX form groups read 回落目标 / Fallback target, 用户入口 /
  User entry and 握手调优 / Handshake tuning; field names, stored settings and
  behavior are unchanged. CI step "Public wording"
  (`config/scripts/check_public_wording.sh`) keeps it that way.

### Security

- `github.com/gorilla/websocket` moves to the upstream fix of GO-2026-6278
  (weak mask key PRNG), `v1.5.4-0.20240701034025-d67f41855da4`.
