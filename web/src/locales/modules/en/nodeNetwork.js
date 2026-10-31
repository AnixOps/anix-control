// The node page's 网络 section on the protocols tab: the interface, source
// address and gateway automatic routes leave by, and the hop port (NAT and
// multi-homed machines). Same keys as zh-CN/nodeNetwork.js.
export default {
  nodeNetwork: {
    uplinks: {
      title: 'Uplinks (several NICs)',
      description: 'A dual-NIC entry (such as CN2 + 9929) lists each NIC here. Routes through this node then pick the NIC to each next node, or take the one with the lowest latency; each NIC probes each next node every 10 seconds. Each NIC needs its source address.',
      label: 'Label',
      name: 'Id',
      nameHint: 'Lower case letters and digits, at most 12',
      add: 'Add a NIC by hand',
      addDetected: 'Add NIC {name}',
      remove: 'Remove this NIC',
      default: 'Default uplink',
      defaultHint: 'Next nodes without a choice of their own take this.',
      auto: 'Auto (lowest latency)',
      follow: 'Follow the default',
      choice: 'This next node takes',
      hops: 'Latency to the next nodes',
      inUse: 'In use',
      rtt: '{ms} ms',
      down: 'Unreachable',
      waiting: 'Not probed yet',
      reason: 'Chosen automatically: {reason} ({time})'
    },
    title: 'Network',
    description: 'The interface and hop port forward routes use when they pass through this node automatically. By default they follow the default route; on NAT or multi-homed machines, name the egress interface, source address and gateway here. A NAT machine also needs a NAT entry under Entries whose port map covers the hop port: other nodes then dial the mapped public port.',
    loading: 'Loading network settings',
    loadFailed: 'Could not load network settings',
    interface: 'Egress interface',
    interfaceHint: 'Such as eth0 or ens18. Empty: the default route.',
    notDetected: 'Such as eth0 or ens18. Empty: the default route. The Agent has not reported its interfaces (Agent 5.0.0-rc.20 or later), so type them.',
    detectedAt: 'Interfaces the Agent detected (reported {time}). Default route: none named.',
    defaultRoute: 'Default route',
    none: 'None',
    manual: 'Type manually',
    pick: 'Pick a detected interface',
    sourceV4: 'IPv4 source address',
    gatewayV4: 'IPv4 gateway',
    sourceV6: 'IPv6 source address',
    gatewayV6: 'IPv6 gateway',
    hopPort: 'Hop port',
    hopPortHint: 'The port other nodes reach this one at over ANX. Empty: 24443.',
    save: 'Save',
    saved: 'Saved; the routes through this node were replanned.',
    saveFailed: 'Could not save',
    invalid: 'Some fields need changes'
  }
}
