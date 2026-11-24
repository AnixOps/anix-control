// The forwarding area's Egress page: some domains a node's users open leave
// through another node. Same keys as zh-CN/egressRoutes.js.
export default {
  egressRoutes: {
    nav: 'Egress',
    page: {
      title: 'Domain egress',
      description: 'Route traffic for selected domains from a source node through another node. Other traffic is unchanged.',
      add: 'New egress route',
      loadFailed: 'Could not load the egress routes',
      emptyTitle: 'No egress routes',
      emptyDescription: 'Select the source node, exit node and domains. Changes apply within a minute.',
      note: 'Nodes connect over a Shadowsocks 2022 relay with a generated key that is never shown. The exit node firewall must allow the relay port.',
      domainCount: '{n} domain | {n} domains',
      disabled: 'Disabled',
      edit: 'Edit…',
      enable: 'Enable',
      disable: 'Disable',
      delete: 'Delete…',
      nodeMissing: 'Node #{id} was deleted',
      fallbacks: '(backup {names})',
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
      description: 'Traffic from the source node to these domains leaves through the exit node.',
      name: 'Name',
      namePlaceholder: 'AI via Japan',
      source: 'Source node',
      exit: 'Exit node',
      exitHost: 'Exit address',
      exitHostHelp: 'Address the source node connects to; empty uses the exit node address.',
      relayPort: 'Relay port',
      relayPortHelp: 'Exit node listening port; must not clash with its protocol or other relay ports.',
      domains: 'Domains',
      domainsHelp: 'One per line; subdomains are included. full:, keyword: and regexp: prefixes are supported.',
      enabled: 'Enabled',
      rotateKey: 'Replace the relay key',
      rotateKeyHelp: 'Both nodes switch to the new key.',
      presetAi: 'Add common AI domains',
      create: 'Create',
      save: 'Save',
      required: 'Required',
      sameNode: 'The exit node must be another node',
      fallbacks: 'Backup exits',
      fallbacksHelp: 'When the exit is unreachable, backup exits are used in order; a failed exit is skipped for 30 seconds and restored after recovery. Each backup listens on its own relay port.',
      fallbackNode: 'Backup exit {n}',
      addFallback: 'Add backup exit',
      removeFallback: 'Remove',
      fallbackRepeated: 'The source node, the exit node and the backup exits must all differ'
    },
    delete: {
      title: 'Delete {name}?',
      message: 'The route is removed within a minute; the domains leave from the source node again.',
      confirm: 'Delete'
    },
    toast: {
      created: 'Created {name}',
      saved: 'Saved {name}',
      deleted: 'Deleted {name}'
    }
  }
}
