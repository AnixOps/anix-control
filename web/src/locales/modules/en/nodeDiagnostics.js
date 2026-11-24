// nodeDiagnostics: the node page's Diagnostics section (5.0, design D7).
export default {
  nodeDiagnostics: {
    title: 'Diagnostics',
    description: 'Run checks on the node Agent.',
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
      description: 'Diagnostics require the Agent to be connected to Control.'
    },
    unsupported: {
      title: 'This Agent does not run diagnostics',
      description: 'Upgrade the Agent.',
      descriptionVersion: 'Agent {version} does not support diagnostics. Upgrade the Agent.'
    },
    service: {
      title: 'Service {service}',
      label: 'Service'
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
    restartSuperOnly: 'Only super administrators can restart services.',
    restart: {
      title: 'Restart {service} on {name}?',
      message: '{service} is briefly interrupted; existing connections drop.',
      confirm: 'Restart service'
    },
    forward: {
      title: 'Forward checks',
      hop: 'Hop to check',
      noHops: 'This node hosts no forward route hops.',
      unsupported: 'This Agent does not support forward checks. Upgrade it.'
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
      none: 'Forward nodes keep no diagnostic history; runs are in the operation log.'
    },
    errors: {
      node_offline: 'The node’s Agent is offline.',
      diagnostics_unsupported: 'This Agent does not support this diagnostic. Upgrade it.',
      node_pending_approval: 'The node is awaiting approval.',
      diagnostic_busy: 'Another diagnostic is running on this node. Try again later.',
      diagnostic_timeout: 'The node returned no result within {seconds} seconds.',
      agent_error: 'The Agent refused this diagnostic: {message}',
      super_admin_required: 'Only super administrators can restart services.',
      action_not_allowed: 'This diagnostic is not allowed.',
      invalid_params: 'The diagnostic parameters are not valid: {message}',
      node_not_found: 'The node does not exist.',
      unknown: 'The diagnostic did not finish.'
    }
  }
}
