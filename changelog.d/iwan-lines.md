### Added

- Panabit iWAN lines: third-party iWAN ingresses (a real Panabit device
  carrying traffic over an IPLC, for example) that AnixOps Net logs into with
  each line's one shared account. Control runs nothing on a line.
  - `GET`/`POST /api/v4/kernel/iwan-lines` and
    `PUT`/`DELETE /api/v4/kernel/iwan-lines/{id}` manage them: name, host,
    port, username, password (never answered; an empty one on an update keeps
    it), MTU and XOR masking. A line that node entries use cannot be deleted.
    Writes are in the operation log (module `iwan_line`) without the login.
  - Node entries have a third kind, `iwan` (`iwan_line_id`): the node's
    VMess, VLESS, Trojan and Shadowsocks protocols are offered again,
    reached at the entry's host through the line, besides the node's host
    or its other entries. The node's own protocol encrypts the whole path;
    iWAN does not.
  - The node page's 入口 sheet offers the iWAN kind, and its "iWAN lines"
    sheet manages the lines.
- Subscription format `anixops` (AnixOps Net, chosen by its User-Agent
  `AnixOpsNet/...`): the V2Ray link list plus each line as an `iwan://` link
  and `via=<line name>` on the links reached through it. Other formats leave
  the lines and those links out. Older AnixOps Net versions skip `iwan://`
  links; a VMess link with `via` needs a version that reads it.

### Changed

- Database migration `0013_iwan_lines` adds `v5_iwan_line` and
  `v5_node_entry.iwan_line_id`.
