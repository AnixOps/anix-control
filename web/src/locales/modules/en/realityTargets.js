// realityTargets: the node page's Targets section (Reality target probing).
export default {
  realityTargets: {
    title: 'Reality targets',
    description: "The node's own Agent probes candidate Reality targets (dest and SNI); they are ranked, one is recommended, and you can apply it to the node's Reality protocols.",
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
      unsupported: 'This Agent cannot probe targets. Upgrade the node’s Agent to use it.',
      unsupportedVersion: 'Agent {version} cannot probe targets. Upgrade the node’s Agent to use it.',
      candidates: 'Your candidates',
      candidatesHint: 'Host names or host:port, separated by spaces, commas or lines; probed before the built-in list.',
      withDefaults: 'Also probe the built-in list ({count})',
      run: 'Probe',
      running: 'The node is probing each candidate; this can take over a minute…'
    },
    recommended: {
      title: 'Recommended'
    },
    results: {
      title: 'Results',
      none: 'No probes yet.',
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
      description: 'Every 6 hours the node rechecks its current targets and the built-in candidates; a current target that fails twice in a row (the second check 15 minutes later) is replaced with the recommended one. A probe without an answer is not a failure, and a target that still works is never replaced for a better score. Every switch is in the operation log.',
      nextRun: 'Next check: {time}',
      nextRunSoon: 'Next check: within minutes',
      lastRun: 'Last check: {time}',
      outcome: 'Outcome: {outcome}',
      lastSwitch: 'Last automatic switch: {time}',
      confirmTitle: 'Turn on automatic selection?',
      confirmMessage: 'A switch changes the SNI and dest of this node\'s Reality protocols; clients must refresh their subscription to reach the new target.',
      confirm: 'Turn on'
    },
    apply: {
      button: 'Apply',
      toProtocol: 'Apply to {name}',
      inUse: 'In use',
      title: 'Change the target of {name} to {target}?',
      message: 'SNI and dest change from {from} to {target}; the private key and other settings stay. The node picks up the change itself; clients must refresh their subscription or they cannot connect.',
      confirm: 'Apply',
      done: 'The target of {name} is now {target}. Ask users to refresh their subscription.'
    },
    errors: {
      node_offline: "The node's Agent is offline.",
      reality_probe_unsupported: 'This Agent cannot probe targets.',
      probe_busy: 'This node is already probing; wait for it to finish.',
      target_not_probed: 'This node has not probed this target successfully yet. Probe it first.',
      protocol_not_found: 'The Reality protocol was not found.',
      invalid_params: 'Invalid parameters: {message}',
      unknown: 'The operation failed. Try again later.'
    }
  }
}
