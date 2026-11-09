// The node area's cross-border standby page: switch a blocked cross-border
// link to a public-protocol standby channel and back in one click. Same
// keys as zh-CN/standby.js.
export default {
  standby: {
    nav: 'Standby',
    page: {
      title: 'Cross-border standby',
      description: 'Each cross-border link (ANX-A) can have two public-protocol channels ready: tier 1 VLESS + REALITY + Vision, tier 2 VLESS + XHTTP behind a real TLS front on our own domain. Switch a blocked link over in one click, and back once it recovers.',
      prepare: 'Prepare channels…',
      allReality: 'All to tier 1',
      allXhttp: 'All to tier 2',
      allOff: 'All back',
      loadFailed: 'Could not load the cross-border links',
      emptyTitle: 'The network model has no cross-border link yet',
      emptyDescription: 'Add a transport of kind anx that is not trusted under Network and it shows here.',
      note: 'Both nodes\' Agents apply a switch within a minute: the source node routes every domain to the exit node\'s VLESS relay; traffic to bare IPs keeps the link. The drill manual is docs/guide/cross-border-standby.md.',
      stale: 'Channels no cross-border link uses: {n} (the next prepare removes the inactive ones)',
      skipped: 'A path whose ends are not proxy nodes has no channel',
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
      description: 'Makes a channel for each node pair of each cross-border link (Control generates the uuid, REALITY keys and port and never shows them). Preparing switches nothing; the nodes do not change.',
      serverName: 'Tier 1 borrowed site (SNI)',
      serverNameHelp: 'A large foreign site, e.g. www.microsoft.com. Empty prepares no tier 1.',
      dest: 'Borrowed site address',
      destHelp: 'host:port; SNI:443 when empty.',
      domains: 'Tier 2 own domains',
      domainsHelp: 'One "exit node id=domain" per line, e.g. 50=edge.example.net. The domain must resolve to the exit node, with the TLS front set up there. A node without a domain gets no tier 2.',
      save: 'Prepare'
    },
    confirm: {
      title: 'Switch {what} to {tier}?',
      message: 'Both nodes switch within a minute; open connections drop and reconnect.',
      all: 'every cross-border link',
      ok: 'Switch'
    },
    toast: {
      prepared: 'Prepared: {created} created, {updated} updated, {removed} removed',
      switched: '{n} link(s) now on {tier}'
    }
  }
}
