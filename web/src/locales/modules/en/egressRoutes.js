// The forwarding area's Egress page: some domains a node's users open leave
// through another node. Same keys as zh-CN/egressRoutes.js.
export default {
  egressRoutes: {
    nav: 'Egress',
    page: {
      title: 'Domain egress',
      description: 'Send what a node\'s users open on chosen domains out through another node (for example AI services from Hong Kong through Japan). Other traffic is unchanged.',
      add: 'New egress route',
      loadFailed: 'Could not load the egress routes',
      emptyTitle: 'No egress routes yet',
      emptyDescription: 'Create one: pick the source node, the exit node and the domains. Both nodes\' Agents apply it within a minute.',
      note: 'The two nodes talk over a Shadowsocks 2022 relay with a key Control generates; it is never shown and never in a subscription. The exit node\'s firewall must allow the relay port.',
      domainCount: '{n} domain | {n} domains',
      disabled: 'Disabled',
      edit: 'Edit…',
      enable: 'Enable',
      disable: 'Disable',
      delete: 'Delete…',
      nodeMissing: 'Node #{id} was deleted',
      columns: {
        name: 'Name',
        path: 'Source → exit',
        domains: 'Domains',
        relay: 'Relay port',
        state: 'State'
      },
      state: {
        applied: 'Applied',
        pending: 'Pending',
        failed: 'Failed'
      },
      stateSource: 'Source',
      stateExit: 'Exit'
    },
    sheet: {
      createTitle: 'New egress route',
      editTitle: 'Edit {name}',
      description: 'When the source node\'s users open these domains, the traffic leaves through the exit node. Only protocols on the Xray core take it.',
      name: 'Name',
      namePlaceholder: 'AI via Japan',
      source: 'Source node',
      exit: 'Exit node',
      exitHost: 'Exit address',
      exitHostHelp: 'The address the source node dials; the exit node\'s address when empty.',
      relayPort: 'Relay port',
      relayPortHelp: 'The port the exit node listens on; not one of its protocols\' or another relay\'s.',
      domains: 'Domains',
      domainsHelp: 'One per line. openai.com matches its subdomains too; full:, keyword: and regexp: entries work as well.',
      enabled: 'Enabled',
      rotateKey: 'Replace the relay key',
      rotateKeyHelp: 'Both nodes switch to the new key together.',
      presetAi: 'Add common AI domains',
      create: 'Create',
      save: 'Save',
      required: 'Required',
      sameNode: 'The exit node must be another node'
    },
    delete: {
      title: 'Delete {name}?',
      message: 'Both nodes drop the route within a minute; the domains leave from the source node again.',
      confirm: 'Delete'
    },
    toast: {
      created: 'Created {name}',
      saved: 'Saved {name}',
      deleted: 'Deleted {name}'
    }
  }
}
