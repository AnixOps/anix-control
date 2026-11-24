// The node area's cross-border standby page: switch an unreachable cross-border
// link to a public-protocol standby channel and back in one click. Same
// keys as zh-CN/standby.js.
export default {
  standby: {
    nav: 'Standby',
    page: {
      title: 'Cross-border standby',
      description: 'Two standby channels per cross-border link (ANX-A): tier 1 VLESS + REALITY + Vision, tier 2 VLESS + XHTTP behind a TLS front on an own domain. Switch when the link is unreachable and back after recovery.',
      prepare: 'Prepare channels…',
      allReality: 'All to tier 1',
      allXhttp: 'All to tier 2',
      allOff: 'All back',
      loadFailed: 'Could not load the cross-border links',
      emptyTitle: 'No cross-border links',
      emptyDescription: 'Links appear after adding an untrusted transport of kind anx under Network.',
      note: 'Switches apply within a minute: domain traffic moves to the VLESS relay of the exit node; IP traffic keeps the link. Drill steps: docs/guide/cross-border-standby.md.',
      stale: 'Channels without a cross-border link: {n} (inactive ones are removed at the next prepare)',
      skipped: 'Paths whose ends are not proxy nodes have no channel',
      columns: {
        transport: 'Link',
        pairs: 'Node pairs',
        active: 'Now on',
        ready: 'Ready',
        channels: 'Channels'
      },
      tier: {
        reality: 'Tier 1 REALITY',
        xhttp: 'Tier 2 XHTTP',
        off: 'The link'
      },
      none: 'None',
      toReality: 'Switch to tier 1',
      toXhttp: 'Switch to tier 2',
      toOff: 'Switch back'
    },
    sheet: {
      title: 'Prepare standby channels',
      description: 'Creates channels for each node pair of each cross-border link with generated credentials that are never shown. Preparing does not switch.',
      serverName: 'Tier 1 borrowed site (SNI)',
      serverNameHelp: 'An overseas site, such as www.microsoft.com. Empty skips tier 1.',
      dest: 'Borrowed site address',
      destHelp: 'host:port; SNI:443 when empty.',
      domains: 'Tier 2 own domains',
      domainsHelp: 'One "exit node id=domain" per line, such as 50=edge.example.net. The domain must resolve to the exit node with a TLS front configured. Nodes without a domain skip tier 2.',
      save: 'Prepare'
    },
    confirm: {
      title: 'Switch {what} to {tier}?',
      message: 'Switches within a minute; open connections drop and reconnect.',
      all: 'every cross-border link',
      ok: 'Switch'
    },
    toast: {
      prepared: 'Prepared: {created} created, {updated} updated, {removed} removed',
      switched: '{n} link(s) now on {tier}'
    }
  }
}
