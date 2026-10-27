// The node page's Entries, under the Protocols tab: the addresses
// subscribers reach the node at (several lines, native IPv6, provider NAT
// port maps). Same keys as zh-CN/nodeEntries.js.
export default {
  nodeEntries: {
    title: 'Entries',
    description: 'The addresses users reach this node at. When the machine has several lines (such as CN2 and 9929), or is reached over native IPv6 and a provider NAT IPv4 address, add an entry for each: subscriptions offer every protocol once per entry, named after it.',
    edit: 'Edit entries',
    add: 'Add entry',
    tableLabel: 'Entries of {name}',
    loadFailed: 'Could not load the entries',
    emptyTitle: 'No extra entries',
    emptyDescription: 'Subscriptions use the node address {host}.',
    enabled: 'Enabled',
    disabled: 'Disabled',
    samePorts: 'Same as the protocols',
    viaLine: 'Through {name}',
    uncovered: 'Left out of subscriptions: {protocols}. A node with entries is offered only through them, and no enabled entry offers these protocol ports (a NAT map\'s machine ports must be the protocol ports). Map them, or add a direct entry.',
    listSeparator: ', ',
    saved: 'Saved the entries of {name}; subscriptions use them now',
    columns: {
      label: 'Name',
      kind: 'Kind',
      host: 'Address',
      family: 'Address type',
      ports: 'Ports',
      status: 'Status'
    },
    kinds: {
      direct: 'Direct',
      nat: 'NAT mapping',
      iwan: 'Through an iWAN line'
    },
    families: {
      ipv4: 'IPv4',
      ipv6: 'IPv6',
      domain: 'Domain'
    },
    sheet: {
      title: 'Edit the entries of {name}',
      description: 'Saving replaces the whole list, and subscriptions use the new entries at once; the node\'s own configuration does not change. A relay entered over IPv6 that leaves over IPv4 needs a userspace forwarder such as gost listening on [::] (nftables DNAT does not cross IPv4 and IPv6).',
      empty: 'No entries yet; subscriptions use the node address {host}.',
      entryN: 'Entry {n}',
      remove: 'Remove this entry',
      label: 'Name',
      labelPlaceholder: 'CN2 v6',
      labelHelp: 'Shown after the node name in subscriptions, for example "CN2 v6" or "9929 NAT".',
      kind: 'Kind',
      kindHelp: {
        direct: 'Users dial this address on the protocols\' own ports (for example the machine\'s own IPv6 address).',
        nat: 'The provider maps ports of a public address to the machine; fill in the port map.',
        iwan: 'AnixOps Net logs into a Panabit iWAN line and reaches this node through it. Offered besides the node address and the other entries, for VMess, VLESS, Trojan and Shadowsocks, and only to AnixOps Net.'
      },
      line: 'iWAN line',
      linePlaceholder: 'Choose a line',
      noLines: 'Add a line under iWAN lines first',
      lineHelp: 'The line\'s shared account goes to every subscriber of this node.',
      iwanHostHelp: 'The node\'s address as the line\'s exit reaches it, usually the node address.',
      host: 'Address',
      portMap: 'Port map',
      portMapHelp: 'One range per line: public ports -> machine ports, for example 21000-21099 -> 30000-30099. A protocol port the map leaves out is not offered through this entry.',
      enabled: 'Enabled',
      cancel: 'Cancel',
      save: 'Save',
      required: 'Required',
      noPipe: "The name cannot contain {'|'}",
      duplicate: 'This name is used twice',
      badLine: 'Line {line} does not parse, or its two sides differ in length'
    },
    lines: {
      open: 'iWAN lines',
      title: 'Panabit iWAN lines',
      description: 'Third-party iWAN ingresses (a Panabit device and its IPLC, say) that AnixOps Net logs into with the line\'s one shared account. Nothing is installed on them. iWAN does not encrypt; the node\'s own protocol encrypts the whole path.',
      empty: 'No lines yet.',
      used: '{n} entries use it',
      add: 'Add line',
      edit: 'Edit {name}',
      remove: 'Delete {name}',
      close: 'Done',
      name: 'Name',
      nameHelp: 'Shown in subscriptions as a node; unique.',
      host: 'Server',
      port: 'Port',
      username: 'Username',
      password: 'Password',
      passwordKeep: 'Leave empty to keep the stored password.',
      mtu: 'MTU',
      mtuHelp: 'Empty or 0 uses the client\'s default; otherwise 576 to 1600.',
      encrypt: 'XOR masking',
      encryptHelp: 'Hides the payload from casual inspection; it is not encryption.',
      badPort: 'A port is 1 to 65535',
      badMtu: '0, or 576 to 1600',
      saved: 'Saved the line {name}; subscriptions use it now',
      removeTitle: 'Delete the line {name}?',
      removeMessage: 'Subscriptions stop offering it. This cannot be undone.',
      removeAction: 'Delete line'
    }
  }
}
