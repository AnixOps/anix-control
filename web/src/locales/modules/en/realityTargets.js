// realityTargets: the node page's Targets section (Reality target probing).
export default {
  realityTargets: {
    title: 'Reality targets',
    description: "The node Agent probes candidate Reality targets (dest and SNI), ranks them and recommends one to apply to the node's Reality protocols.",
    refresh: 'Refresh',
    loading: 'Loading targets…',
    loadFailed: 'Could not load the targets',
    online: 'Agent online',
    notConnected: 'Agent offline',
    agentVersion: 'Agent {version}',
    nodeAddress: 'Node address {address}',
    separator: ', ',
    score: 'Score {score}',
    current: {
      title: 'Current target',
      none: 'This node has no Reality protocol.',
      dest: 'dest {dest}'
    },
    probe: {
      title: 'Probe',
      offline: "Probing needs the node's Agent connected to Control.",
      unsupported: 'This Agent does not support target probing. Upgrade it.',
      unsupportedVersion: 'Agent {version} does not support target probing. Upgrade it.',
      candidates: 'Your candidates',
      candidatesHint: 'Host names or host:port, separated by spaces, commas or new lines. Probed before the built-in list.',
      withDefaults: 'Also probe the built-in list ({count})',
      run: 'Probe',
      running: 'Probing; this can take about a minute…'
    },
    recommended: {
      title: 'Recommended'
    },
    results: {
      title: 'Results',
      none: 'No probes.',
      handshake: 'Handshake {ms} ms',
      cdn: 'CDN: {cdn}',
      usedBy: 'Used by {count} other nodes'
    },
    verdicts: {
      ok: 'Usable',
      failed: 'Not usable',
      error: 'Not finished'
    },
    failures: {
      resolve: 'does not resolve',
      private_address: 'resolves to a private address',
      connect: 'unreachable',
      tls_handshake: 'TLS handshake failed',
      tls13: 'no TLS 1.3',
      h2: 'no h2',
      x25519: 'no X25519',
      certificate: 'invalid certificate',
      http: 'HTTP request failed',
      redirect: 'redirects to another host',
      reality: 'breaks the REALITY handshake'
    },
    auto: {
      title: 'Automatic selection',
      description: 'Every 6 hours, the current targets and built-in candidates are rechecked. A current target that fails twice in a row (15 minutes apart) is replaced with the recommended one. Switches are recorded in the operation log.',
      nextRun: 'Next check: {time}',
      nextRunSoon: 'Next check: within minutes',
      lastRun: 'Last check: {time}',
      outcome: 'Outcome: {outcome}',
      lastSwitch: 'Last automatic switch: {time}',
      confirmTitle: 'Turn on automatic selection?',
      confirmMessage: 'Switching changes the SNI and dest of the Reality protocols on this node. Clients must refresh their subscription.',
      confirm: 'Turn on'
    },
    apply: {
      button: 'Apply',
      toProtocol: 'Apply to {name}',
      inUse: 'In use',
      title: 'Change the target of {name} to {target}?',
      message: 'SNI and dest change from {from} to {target}; other settings are unchanged. Clients must refresh their subscription.',
      confirm: 'Apply',
      done: 'The target of {name} is now {target}. Users must refresh their subscription.'
    },
    errors: {
      node_offline: "The node's Agent is offline.",
      reality_probe_unsupported: 'This Agent cannot probe targets.',
      probe_busy: 'This node is already probing. Try again later.',
      target_not_probed: 'This target has not been probed successfully. Probe it first.',
      protocol_not_found: 'The Reality protocol was not found.',
      invalid_params: 'Invalid parameters: {message}',
      unknown: 'The operation failed. Try again later.'
    }
  }
}
