// nodeDiagnostics: the node page's Diagnostics section (5.0, design D7).
export default {
  nodeDiagnostics: {
    title: 'Diagnostics',
    description: 'Run the allowed checks on the node’s Agent. The result shows here.',
    refresh: 'Refresh',
    loading: 'Loading diagnostics…',
    loadFailed: 'Could not load diagnostics',
    online: 'Agent online',
    notConnected: 'Agent offline',
    agentVersion: 'Agent {version}',
    supports: 'Supports: {list}',
    separator: ', ',
    offline: {
      title: 'The Agent is offline',
      description: 'Diagnostics run once the node’s Agent is connected to Control.'
    },
    unsupported: {
      title: 'This Agent does not run diagnostics',
      description: 'Upgrade the node’s Agent to use diagnostics.',
      descriptionVersion: 'Agent {version} does not run diagnostics. Upgrade the node’s Agent to use them.'
    },
    service: {
      title: 'Service {service}'
    },
    actions: {
      service_status: 'View service status',
      service_restart: 'Restart service',
      log_tail: 'View logs',
      forward_listen: 'Check listener',
      forward_port_conflict: 'Check port conflicts',
      forward_connect: 'Check TCP connect',
      forward_udp_probe: 'Probe UDP'
    },
    lines: {
      label: 'Log lines',
      option: 'Last {count} lines'
    },
    restartSuperOnly: 'Only a super administrator can restart a service.',
    restart: {
      title: 'Restart {service} on {name}?',
      message: '{service} stops for a moment and the connections through it drop.',
      confirm: 'Restart service'
    },
    forward: {
      title: 'Forward checks',
      hop: 'Hop to check',
      noHops: 'This node hosts no hop of a forward route yet.',
      unsupported: 'This Agent does not run forward checks. Upgrade it to use them.'
    },
    running: 'Running “{action}”, waiting up to {seconds} seconds…',
    result: {
      title: 'Result',
      success: 'Succeeded',
      failed: 'Failed',
      duration: '{ms} ms',
      output: 'Output',
      noOutput: '(no output)',
      rtt: '{ms} ms'
    },
    checkStates: {
      ok: 'OK',
      failed: 'Failed',
      inconclusive: 'Inconclusive',
      skipped: 'Skipped'
    },
    history: {
      title: 'Recent diagnostics',
      show: 'Show',
      pending: 'Running',
      none: 'Forward nodes keep no diagnostic history. Every run is in the operation log.'
    },
    errors: {
      node_offline: 'The node’s Agent is offline.',
      diagnostics_unsupported: 'This Agent does not run this diagnostic. Upgrade it to use it.',
      node_pending_approval: 'The node waits for approval. Approve it to run diagnostics.',
      diagnostic_busy: 'Another diagnostic of this node is running. Try again shortly.',
      diagnostic_timeout: 'The node returned no result within {seconds} seconds.',
      agent_error: 'The Agent refused this diagnostic: {message}',
      super_admin_required: 'Only a super administrator can restart a service.',
      action_not_allowed: 'This diagnostic is not allowed.',
      invalid_params: 'The diagnostic parameters are not valid: {message}',
      node_not_found: 'The node does not exist.',
      unknown: 'The diagnostic did not finish.'
    }
  }
}
