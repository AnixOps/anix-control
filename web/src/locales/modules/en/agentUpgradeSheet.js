// agentUpgradeSheet: the Upgrade nodes sheet (5.0). Progress, pause and
// abort use the agentUpgrades messages.
export default {
  agentUpgradeSheet: {
    title: 'Upgrade nodes',
    description: 'Pushes the Agent release pinned by Control to online nodes in batches.',
    loading: 'Loading upgrade details…',
    loadFailed: 'Could not load upgrade details',
    started: 'Upgrade to {version} started',
    kernelStarted: 'Kernel upgrade to anx-node {version} started',
    start: {
      title: 'Start an upgrade',
      target: 'Target version',
      targetHelp: 'Defaults to the Agent release pinned by Control. It must match the major.minor of Control ({control}).',
      lineMismatch: 'The default target {target} is not on the version line of Control ({control}). Place an Agent release of that line in agent_install.artifact_dir.',
      nodeIncompatible: 'incompatible',
      nodeWarning: 'check version',
      batches: 'Canary batches (cumulative %)',
      batchesHelp: 'Comma separated. The first batch is at most {first}%; the last is 100%.',
      minimum: 'Minimum per batch',
      minimumValue: '{minutes} minutes',
      eligible: 'Will upgrade',
      eligibleValue: '{n} node | {n} nodes',
      rules: 'A batch rolls back when more than {percent}% of its nodes fail or do not reconnect with the new version within {minutes} minutes.',
      reason: 'Note (optional)',
      reasonPlaceholder: 'For example: upgrade to v4.2.0',
      nodes: 'Nodes',
      nodeVersion: 'Agent {version} now',
      nodeKernel: 'kernel anx-node {version}',
      nodeKernelUnknown: 'kernel not reported',
      kernelTarget: 'Installs anx-node {version} (latest kernel). The Agent version is unchanged; it restarts once.',
      kernelMissing: 'No anx-node release on Control. Upload one below or copy one into agent_install.artifact_dir.',
      kernelSubmit: 'Upgrade kernel',
      mode: 'Upgrade mode',
      modes: {
        staged: 'Staged',
        manual: 'Choose nodes',
        kernel: 'Kernel only',
        stagedHelp: 'Upgrades all eligible nodes in batches.',
        manualHelp: 'Upgrades the selected nodes at once.',
        kernelHelp: 'Upgrades only the anx-node kernel of the selected nodes, which must run the same Agent version.'
      },
      submit: 'Start upgrade',
      noneEligible: 'No eligible nodes. A node must be enabled and its Agent must have connected to Control.',
      active: 'An upgrade is in progress. Wait for it to finish or abort it first.',
      superOnly: 'Only super administrators can start upgrades.',
      unconfigured: 'An upgrade cannot start now: {reason}'
    },
    upload: {
      label: 'Manual kernel install: choose SHA256SUMS, SHA256SUMS.sig and the anx-node packages of one release',
      submit: 'Upload kernel',
      missing: 'Choose SHA256SUMS, SHA256SUMS.sig and at least one anx-node .tar.gz package.',
      done: 'anx-node {version} uploaded'
    },
    confirm: {
      title: 'Upgrade {n} node to {version}? | Upgrade {n} nodes to {version}?',
      message: 'Pushed in batches: {batches}, at least {minutes} minutes each. Can be paused, aborted or rolled back at any time.',
      kernelTitle: 'Upgrade the kernel of {n} node to anx-node {version}? | Upgrade the kernel of {n} nodes to anx-node {version}?',
      kernelMessage: 'Installs the kernel on these nodes now: {nodes}. Each Agent restarts once; no reconnection within 10 minutes counts as failure.',
      manualMessage: 'Upgrades these nodes now: {nodes}. Nodes that fail or do not reconnect with the new version within 10 minutes are rolled back.'
    },
    errors: {
      target: 'The target must be a release tag, for example v4.2.0.',
      batchesFormat: 'Batches must be comma-separated integers, for example 5, 25, 100.',
      batchesCount: 'At most {max} batches.',
      batchesOrder: 'Percentages must increase and not exceed 100.',
      batchesFirst: 'The first batch is at most {max}%.',
      batchesLast: 'The last batch must be 100%.',
      super_admin_required: 'Only super administrators can start upgrades.',
      agent_release_unverified: 'Agent release verification failed: {message}',
      agent_upgrade_active: 'An upgrade is in progress. Wait for it to finish or abort it first.',
      agent_upgrade_no_nodes: 'No node can be upgraded.',
      kernel_release_missing: 'Control holds no anx-node release to install.',
      kernel_release_unverified: 'The anx-node release files are incomplete: {message}',
      agent_install_unconfigured: 'An upgrade cannot start now: {message}',
      kernel_release_invalid: 'Kernel release verification against the release key failed: {message}',
      kernel_release_conflict: 'Another release of this anx-node version is already stored: {message}',
      invalid_request: 'Invalid upgrade settings: {message}',
      unknown: 'The upgrade did not start.'
    }
  }
}
