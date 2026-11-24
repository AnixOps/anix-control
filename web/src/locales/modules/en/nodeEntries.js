// The node page's Entries, under the Protocols tab: the addresses
// subscribers reach the node at (several lines, native IPv6, provider NAT
// port maps). Same keys as zh-CN/nodeEntries.js.
export default {
  nodeEntries: {
    title: 'Entries',
    description: 'Addresses users connect to. For multiple lines (such as CN2 and 9929) or native IPv6 plus NAT IPv4, add one entry each; subscriptions list each protocol once per entry.',
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
    uncovered: 'Not in subscriptions: {protocols}. No enabled entry covers these protocol ports. Add a port mapping or a direct entry.',
    listSeparator: ', ',
    saved: 'Entries of {name} saved; subscriptions updated',
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
      description: 'Saving replaces all entries and updates subscriptions immediately. The node configuration is unchanged.',
      empty: 'No entries. Subscriptions use the node address {host}.',
      entryN: 'Entry {n}',
      remove: 'Remove this entry',
      label: 'Name',
      labelPlaceholder: 'CN2 v6',
      labelHelp: 'Appended to the node name in subscriptions, for example "CN2 v6" or "9929 NAT".',
      kind: 'Kind',
      kindHelp: {
        direct: 'Users dial this address on the protocols\' own ports (for example the machine\'s own IPv6 address).',
        nat: 'Public ports are mapped to the machine. Enter the port map.',
        iwan: 'AnixOps Net connects to this node through a Panabit iWAN line. VMess, VLESS, Trojan and Shadowsocks only; offered to AnixOps Net only.'
      },
      line: 'iWAN line',
      linePlaceholder: 'Choose a line',
      noLines: 'Add a line under iWAN lines first',
      lineHelp: 'The shared line account is sent to every subscriber of this node.',
      iwanHostHelp: 'The node\'s address as the line\'s exit reaches it, usually the node address.',
      host: 'Address',
      portMap: 'Port map',
      portMapHelp: 'One range per line: public ports -> machine ports, for example 21000-21099 -> 30000-30099. Unmapped protocol ports are not offered.',
      pool: 'Entry pool',
      poolHelp: 'Empty: all subscribers. A pool name (such as shard-0 or canary-42) limits it to that pool.',
      badPool: 'Pool names use a-z, 0-9, _ and -',
      enabled: 'Enabled',
      cancel: 'Cancel',
      save: 'Save',
      required: 'Required',
      noPipe: "The name cannot contain {'|'}",
      duplicate: 'This name is used twice',
      badLine: 'Line {line} is invalid, or its two sides differ in length'
    },
    lines: {
      open: 'iWAN lines',
      title: 'Panabit iWAN lines',
      description: 'Third-party iWAN ingresses (such as a Panabit device and its IPLC) that AnixOps Net signs in to with a shared account. iWAN does not encrypt; the node protocol encrypts the path.',
      empty: 'No lines.',
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
      encryptHelp: 'XOR masking is not encryption.',
      badPort: 'A port is 1 to 65535',
      badMtu: '0, or 576 to 1600',
      saved: 'Line {name} saved; subscriptions updated',
      removeTitle: 'Delete the line {name}?',
      removeMessage: 'Subscriptions stop offering it. This cannot be undone.',
      removeAction: 'Delete line'
    }
  }
}
