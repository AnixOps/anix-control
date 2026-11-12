// Lines (src/views/admin/Lines.vue).
export default {
  lines: {
    title: 'Lines',
    description: 'A line is one way a subscriber connects: in at an entry, over relays, out to the internet at an exit.',
    loadFailed: 'Could not load the lines',
    emptyTitle: 'No lines yet',
    emptyDescription: 'Create a line from an entry to an exit, or add a protocol to a device.',
    legend: 'Dot colours are entry, relay and exit. The outlined device is where the protocol is served. A dashed link is forwarded in the kernel.',
    rules: '{n} domain rules',
    filter: { label: 'Filter', relay: 'Relayed', direct: 'Direct', fixed: 'Fixed targets', attention: 'Needs attention' },
    kind: { relay: 'Relayed line', direct: 'Direct line', fixed: 'Fixed targets' },
    columns: { line: 'Line', path: 'Entry → exit', rate: 'Rate', traffic: 'Traffic, last 24 hours' },
    skipped: {
      title: '{n} items cannot be read as lines yet',
      description: 'They work as before; they are only missing from the list above.'
    },
    actions: { edit: 'Edit', more: 'More actions', copy: 'Copy line', copyName: '{name} copy', delete: 'Delete line' },
    delete: {
      title: 'Delete line {name}?',
      message: 'Subscriptions that use it lose this line. This cannot be undone.',
      messageRules: 'Subscriptions that use it lose this line, and its {n} split rules are deleted with it. This cannot be undone.',
      confirm: 'Delete line'
    },
    toast: { created: 'Line {name} created', saved: 'Line {name} saved', deleted: 'Line {name} deleted' },
    form: {
      createTitle: 'Create line',
      editTitle: 'Edit {name}',
      description: 'Pick the entries, the devices it passes and the exit, then the protocol on the exit.',
      preview: 'Preview',
      previewEmpty: 'Pick an entry and an exit first.',
      entries: 'Entries',
      entriesHelp: 'Pick several and they back each other up.',
      via: 'Via',
      viaHelp: 'Optional; passed in the order you pick them.',
      exit: 'Exit',
      choose: 'Choose',
      protocol: 'Protocol',
      protocolPort: 'port {port}',
      noProtocols: 'This device has no protocol yet: add one on its node, or forward to fixed targets.',
      fixed: 'No protocol, forward to fixed targets',
      targets: 'Fixed targets',
      targetsHelp: 'One host:port per line.',
      name: 'Name',
      namePlaceholder: 'e.g. Shanghai → Tokyo',
      port: 'Entry port',
      portHelp: 'The port users connect to at the entry.',
      l4: 'Transport',
      enabled: 'Enabled',
      show: 'Offer in subscriptions',
      showHelp: 'The line appears in users\' subscriptions.',
      create: 'Create line',
      save: 'Save',
      refused: 'The line cannot be built as set:',
      conflict: 'The line was just changed elsewhere. Close and reopen it to edit.',
      errors: {
        required: 'Required',
        reused: 'The exit cannot also be an entry or a device it passes',
        target: 'Use host:port, with a port from 1 to 65535'
      }
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
