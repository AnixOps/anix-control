### Removed

- The `nftables-forward` and `nat-egress` official packages (扩展 → nftables
  Forward and NAT Egress) are gone. Relays run on the network model (ANX), and
  a node's egress NIC, source and gateway are set on its node page (网络), so
  both plugins did the same work twice. On the first start of this release
  Control disables any installation of them (the rows and releases stay for the
  record); nothing else needs to be done. The nftables Ansible playbooks and
  `v2b_forward_nft.sh` are removed with them; `forward_legacy_cleanup` still
  deletes the old nftables tables from a host.
