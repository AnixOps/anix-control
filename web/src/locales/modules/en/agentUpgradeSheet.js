// agentUpgradeSheet: the Upgrade nodes sheet (5.0). Progress, pause and
// abort use the agentUpgrades messages.
export default {
  agentUpgradeSheet: {
    title: 'Upgrade nodes',
    description: 'Control pushes the Agent release it pins to the online nodes in batches, a small canary batch first.',
    loading: 'Loading upgrade details…',
    loadFailed: 'Could not load upgrade details',
    started: 'Upgrade to {version} started',
    start: {
      title: 'Start an upgrade',
      target: 'Target version',
      targetHelp: 'Defaults to the Agent release the Control install script pins. It cannot be newer than Control ({control}).',
      batches: 'Canary batches (cumulative %)',
      batchesHelp: 'Separate with commas. The first batch is at most {first}%, the last is 100%.',
      minimum: 'Minimum per batch',
      minimumValue: '{minutes} minutes',
      eligible: 'Will upgrade',
      eligibleValue: '{n} node | {n} nodes',
      rules: 'A batch rolls back when more than {percent}% of its nodes fail or do not reconnect with the new version within {minutes} minutes.',
      reason: 'Note (optional)',
      reasonPlaceholder: 'For example: upgrade to v4.2.0',
      submit: 'Start upgrade',
      noneEligible: 'No node can be upgraded: a node must be enabled and its Agent seen by Control.',
      active: 'An upgrade is running. Let it end, or abort it, before starting another.',
      superOnly: 'Only a super administrator can start an upgrade.',
      unconfigured: 'An upgrade cannot start now: {reason}'
    },
    confirm: {
      title: 'Upgrade {n} node to {version}? | Upgrade {n} nodes to {version}?',
      message: 'Pushed in batches: {batches}, at least {minutes} minutes each. You can pause, abort or roll back at any time.'
    },
    errors: {
      target: 'Write the target as a release tag, for example v4.2.0.',
      batchesFormat: 'Write the batches as integers separated by commas, for example 5, 25, 100.',
      batchesCount: 'At most {max} batches.',
      batchesOrder: 'Each percentage must be larger than the one before and at most 100.',
      batchesFirst: 'The first batch is at most {max}%.',
      batchesLast: 'The last batch must be 100%.',
      super_admin_required: 'Only a super administrator can start an upgrade.',
      agent_release_unverified: 'The files of this Agent release did not pass verification: {message}',
      agent_upgrade_active: 'An upgrade is running. Let it end, or abort it, before starting another.',
      agent_upgrade_no_nodes: 'No node can be upgraded.',
      agent_install_unconfigured: 'An upgrade cannot start now: {message}',
      invalid_request: 'The upgrade settings are not valid: {message}',
      unknown: 'The upgrade did not start.'
    }
  }
}
