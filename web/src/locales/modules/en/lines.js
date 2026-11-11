// Lines (src/views/admin/Lines.vue).
export default {
  lines: {
    title: 'Lines',
    description: 'A line is one way a subscriber connects: in at an entry, over relays, out to the internet at an exit.',
    loadFailed: 'Could not load the lines',
    emptyTitle: 'No lines yet',
    emptyDescription: 'Add a protocol to a device or create a forward route, and its line shows here.',
    legend: 'Dot colours are entry, relay and exit. The outlined device is where the protocol is served. A dashed link is forwarded in the kernel.',
    rules: '{n} domain rules',
    filter: { label: 'Filter', relay: 'Relayed', direct: 'Direct', fixed: 'Fixed targets', attention: 'Needs attention' },
    kind: { relay: 'Relayed line', direct: 'Direct line', fixed: 'Fixed targets' },
    columns: { line: 'Line', path: 'Entry → exit', rate: 'Rate', traffic: 'Traffic, last 24 hours' },
    skipped: {
      title: '{n} items cannot be read as lines yet',
      description: 'They work as before; they are only missing from the list above.'
    },
    detail: {
      path: 'Path',
      served: 'Served here',
      target: 'Fixed target',
      entryAndExit: 'Entry and exit',
      protocol: 'Protocol',
      noProtocol: 'Ports only',
      port: 'Entry port',
      hostname: 'Entry hostname',
      subscription: 'Subscription',
      shown: 'Offered in subscriptions',
      hidden: 'Not in subscriptions',
      rules: 'Domain rules',
      noRules: 'All traffic leaves by the default exit.',
      via: 'Out at {name}',
      openRoute: 'Open the forward route'
    }
  }
}
