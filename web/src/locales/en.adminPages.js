// Messages for the admin pages other than the dashboard (src/views/admin
// and the forwarding area). src/i18n.js loads them before the first such
// page opens (router/index.js), so an admin's first visit, the
// dashboard, does not download them. The shell's messages are in
// src/locales/en.admin.js; the two groups share no message key.
import networkPages from './modules/en/networkPages'
import miscPages from './modules/en/miscPages'
import adminSupportPages from './modules/en/adminSupportPages'
import adminNodes from './modules/en/adminNodes'
import adminSubscriptionGroups from './modules/en/adminSubscriptionGroups'
import forwardV4 from './modules/en/forwardV4'
import forwardDns from './modules/en/forwardDns'

export default {
  ...adminSubscriptionGroups,
  ...forwardV4,
  ...forwardDns,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
  runtime: {
    shared: {
      reachability: 'Reachability',
      warnings: 'Warnings',
      powerShell: 'PowerShell',
      bash: 'Bash',
      bootstrapVerify: 'Bootstrap / Verify',
      references: 'References',
      doctorOutput: 'Doctor Output',
      doctorNotExecuted: 'Doctor has not been executed yet.',
      refreshStatus: 'Refresh status',
      runningDoctor: 'Running...',
      loading: 'Loading...',
      yes: 'Yes',
      no: 'No',
      present: 'Present',
      missing: 'Missing',
      notReady: 'Not ready',
      ready: 'Ready',
      unavailable: 'Unavailable',
      enabled: 'Enabled',
      disabled: 'Disabled'
    },
    localRuntime: {
      fields: {
        inventory: 'Inventory',
        command: 'Command',
        workingDir: 'Working dir'
      },
      cards: {
        commandFound: 'Command found'
      },
      backends: {
        nftables: {
          label: 'nftables / Ansible'
        }
      }
    },
    nodeX: {
      saveLoading: 'Saving...',
      save: 'Save NodeX Config',
      cards: {
        baseUrl: 'Base URL',
        tokenConfigured: 'Token configured',
        health: 'Health'
      },
      backends: {
        gost: 'gost / NodeX'
      }
    },
    ansibleMachines: {
      modal: {
        saveLoading: 'Saving...'
      }
    },
    workbench: {
      actions: {
        runDoctorActiveRuntime: 'Run doctor on active runtime'
      },
      localCard: {
        title: 'Local Ansible executor',
        description: 'Stateless panel-host execution. Inventory, playbooks and SSH access are managed separately from NodeX.',
        currentState: 'Current state'
      },
      nodeXCard: {
        title: 'Stateful gost control-plane',
        description: 'Panel talks to the internal NodeX control-plane. Real relay attachment only exists after gost runtime jobs succeed.'
      },
      state: {
        activeBackend: 'Active',
        standby: 'Standby'
      },
      references: {
        panelRuntimeDoc: 'Panel doc: docs/reference/runtime.md',
        panelRelayOnboarding: 'Panel doc: docs/guide/forward-relay-onboarding.md',
        nodeXRepo: 'NodeX repo: https://github.com/zdwtest/NodeX',
        panelNodeXOnboarding: 'Panel doc: docs/forward-runtime-relay-onboarding.md'
      },
      doctor: {
        title: 'Active Runtime Snapshot',
        description: 'Reachability only means the control plane or local executor can be contacted. It is not proof that a relay has already attached or that iptables rules already exist.',
        note: 'This workbench only shows the currently active backend. Use the dedicated Local Runtime and NodeX Runtime pages to edit config and run mode-specific probes.',
        summary: 'This workbench consolidates control-plane health, runtime diagnostics and one-click commands across both NodeX/gost and local Ansible execution paths.',
        loadingStatus: 'Fetching forward runtime status...'
      },
      cards: {
        backend: 'Backend',
        nodeXMode: 'NodeX Mode',
        attachment: 'Attachment',
        panelVerdict: 'Panel Verdict',
        nodeXSnapshot: 'NodeX Snapshot',
        baseUrlConfigured: 'Base URL configured',
        runtimeVersion: 'Runtime version',
        localAnsible: 'Local ansible',
        playbooks: 'Playbooks'
      },
      errors: {
        fetchStatusFailed: 'Failed to fetch forward runtime status',
        doctorFailed: 'Forward runtime doctor failed'
      }
    },
    nodeXTopology: {
      meta: {
        traffic: 'Upload / Download'
      },
      nodeModal: {
        saveLoading: 'Saving...'
      }
    },
    nodeXAgents: {
      title: 'NodeX Agents',
      subtitle: 'Only NodeX mode needs agents. Use this page for agent status, remote terminal, and task delivery.',
      tabs: {
        agents: 'Online Agents',
        terminal: 'Remote Terminal',
        tasks: 'Task History'
      },
      actions: {
        refresh: 'Refresh',
        execute: 'Run',
        send: 'Send',
        cancel: 'Cancel',
        monitor: 'Monitoring',
        openTerminal: 'Open in terminal'
      },
      table: {
        nodeId: 'Node',
        version: 'Version',
        system: 'System',
        lastSeen: 'Last Seen',
        status: 'Status',
        capabilities: 'Capabilities',
        taskId: 'Task ID',
        node: 'Node',
        command: 'Command / Action',
        duration: 'Duration',
        time: 'Time'
      },
      status: {
        online: 'Online',
        offline: 'Offline',
        connected: 'Connected',
        disconnected: 'Disconnected',
        success: 'Success',
        failed: 'Failed'
      },
      empty: {
        agents: 'No agents online',
        agentsDescription: 'An agent appears here when a NodeX node connects to Control.',
        tasks: 'No task history yet',
        tasksDescription: 'Commands you run in the terminal and tasks you send are listed here.'
      },
      terminal: {
        chooseNode: 'Choose node',
        nodeLabel: 'Node #{id}',
        chooseAction: 'Choose action',
        output: 'Terminal output',
        hint: 'Choose an online node and an action, then run it. Only the diagnostic actions Control allows can run.'
      },
      diagnosticActions: {
        service_status: 'Check service status',
        service_restart: 'Restart service',
        log_tail: 'Tail service log'
      },
      fields: {
        service: 'Service',
        lines: 'Lines'
      },
      services: {
        gost: 'GOST'
      },
      taskModal: {
        title: 'Send Task',
        action: 'Action',
        timeoutSeconds: 'Timeout (seconds)'
      },
      hints: {
        monitor: 'View monitoring data for node #{id}'
      },
      messages: {
        fetchFailed: 'Agents didn’t load',
        tasksFetchFailed: 'Task history didn’t load',
        taskIncomplete: 'Please fill in the required fields',
        taskSent: 'Task sent',
        taskSendFailed: 'Send failed: {message}',
        commandError: 'Error: {message}',
        selectActionFirst: 'Choose an action first'
      }
    }
  },
  admin: {
    nodes: adminNodes
  },
  adminPayment: {
    title: 'Payments',
    subtitle: 'Gateways users pay through, every payment, and how much came in.',
    tabs: {
      label: 'Payment sections',
      gateways: 'Gateways',
      records: 'Records',
      stats: 'Stats'
    },
    actions: {
      createGateway: 'New gateway',
      enable: 'Enable',
      disable: 'Disable',
      edit: 'Edit',
      details: 'Details'
    },
    gateways: {
      label: 'Payment gateways',
      table: {
        id: 'ID',
        name: 'Name',
        type: 'Type',
        feeRate: 'Fee Rate',
        minAmount: 'Min Amount',
        maxAmount: 'Max Amount',
        status: 'Status'
      },
      empty: 'No payment gateways yet',
      emptyDescription: 'Add a gateway so users can pay for plans.'
    },
    records: {
      label: 'Payment records',
      filters: {
        status: 'Filter by status',
        type: 'Filter by gateway'
      },
      table: {
        id: 'ID',
        tradeNo: 'Trade No.',
        userId: 'User ID',
        gateway: 'Gateway',
        amount: 'Amount',
        status: 'Status',
        createdAt: 'Created at'
      },
      detail: {
        title: 'Payment Details',
        tradeNo: 'Trade No.',
        amount: 'Amount',
        status: 'Status'
      },
      empty: 'No payments yet',
      emptyDescription: 'Payments appear here when users pay for an order.'
    },
    stats: {
      totalAmount: 'Total Revenue',
      totalOrders: 'Total Orders',
      successOrders: 'Successful Orders',
      successRate: 'Success Rate',
      gatewayDistribution: 'Gateway Distribution',
      empty: 'No gateway statistics yet'
    },
    modal: {
      createTitle: 'New gateway',
      editTitle: 'Edit Gateway',
      fields: {
        name: 'Name',
        type: 'Type',
        feeRate: 'Fee Rate',
        minAmount: 'Min Amount',
        maxAmount: 'Max Amount',
        configJson: 'Configuration (JSON)'
      },
      placeholders: {
        name: 'Gateway name',
        feeRate: 'Example: 0.01 = 1%',
        minAmount: 'Minimum payment amount',
        maxAmount: 'Maximum payment amount',
        configJson: "{'{'}\"app_id\": \"\", \"private_key\": \"\"{'}'}"
      }
    },
    types: {
      alipay: 'Alipay',
      wechat: 'WeChat Pay',
      stripe: 'Stripe',
      usdt: 'USDT',
      epay: 'EPay'
    },
    status: {
      enabled: 'Enabled',
      disabled: 'Disabled',
      pending: 'Pending',
      paid: 'Paid',
      failed: 'Failed',
      refunded: 'Refunded'
    },
    confirm: {
      deleteTitle: 'Delete payment gateway {name}?',
      deleteMessage: 'Users can no longer pay through this gateway. This can’t be undone.',
      deleteAction: 'Delete gateway'
    },
    messages: {
      fetchGatewaysFailed: 'Payment gateways didn’t load',
      fetchRecordsFailed: 'Payment records didn’t load',
      fetchStatsFailed: 'Payment statistics didn’t load',
      invalidConfigJson: 'Configuration JSON is invalid',
      gatewaySaveSuccess: 'Gateway saved successfully',
      gatewaySaveFailed: 'Failed to save gateway: {message}',
      gatewaySaveFailedShort: 'Save failed',
      toggleFailed: 'Failed to change gateway status: {message}',
      toggleFailedShort: 'Operation failed',
      gatewayEnabled: '{name} enabled',
      gatewayDisabled: '{name} disabled',
      gatewayDeleted: 'Gateway {name} deleted',
      deleteFailedShort: 'Delete failed'
    }
  },
  adminMfa: {
    config: {
      enabled: 'Enable multi-factor authentication',
      enabledHelp: 'Once enabled, users can choose to turn on MFA to protect account security.',
      required: 'Require MFA',
      requiredHelp: 'Require all users to enable MFA, otherwise they cannot use the service.',
      methods: 'Supported authentication methods',
      backupCodesCount: 'Recovery code count',
      backupCodesHelp: 'Number of recovery codes generated when users enable MFA.',
      maxAttempts: 'Maximum login attempts',
      maxAttemptsHelp: 'Accounts will be temporarily locked after exceeding the limit.',
      lockoutDuration: 'Lockout duration (minutes)',
      lockoutDurationHelp: 'Lockout duration after login failures exceed the limit.'
    },
    methods: {
      totp: 'TOTP (Google Authenticator / Authy)',
      sms: 'SMS verification code',
      email: 'Email verification code'
    },
    info: {
      totpBody: 'Time-based one-time passwords. Users can scan a QR code with apps such as Google Authenticator or Authy to bind MFA.',
      backupBody: 'When users cannot access their authenticator, they can use recovery codes to log in. Each recovery code can only be used once.',
      lockoutBody: 'Multiple consecutive MFA failures will trigger an account lockout to prevent brute-force attacks.',
      userOpsBody: 'Users turn two-factor authentication on or off and make new recovery codes on their own Account page.'
    },
    messages: {
      fetchFailed: 'Failed to load MFA configuration',
      saveSuccess: 'Saved successfully',
      saveFailed: 'Save failed: {message}',
      saveFailedShort: 'Save failed'
    }
  },
  adminTemplates: {
    title: 'Subscription templates',
    subtitle: 'Free subscription templates: traffic quota, speed and device limits, and the subscription groups they grant. Assign one to a user to apply it.',
    filters: {
      search: 'Search templates'
    },
    table: {
      label: 'Subscription templates'
    },
    actions: {
      create: 'New template'
    },
    empty: {
      title: 'No subscription templates yet',
      description: 'Create a template, then assign it to users to give them traffic and subscription groups.'
    },
    detail: {
      description: 'Template ID {id}'
    },
    labels: {
      noGroupsHint: 'This template grants no subscription groups yet.'
    },
    planModal: {
      createTitle: 'New subscription template',
      editTitle: 'Edit subscription template',
      fields: {
        name: 'Template name'
      },
      placeholders: {
        name: 'Enter template name'
      }
    },
    assignModal: {
      title: 'Assign subscription template'
    },
    groupModal: {
      title: 'Template groups - {name}',
      description: 'Select the subscription groups this template grants.'
    },
    confirm: {
      deleteTitle: 'Delete subscription template {name}?',
      deleteAction: 'Delete template'
    },
    messages: {
      loadFailed: 'Failed to load subscription templates',
      nameRequired: 'Please enter a template name',
      deleted: 'Subscription template {name} deleted',
      groupRemoved: 'Group {group} removed from template {plan}'
    }
  },
  adminPlans: {
    title: 'Plans',
    subtitle: 'What users can buy: traffic quota, speed and device limits, price, and the subscription groups each plan grants.',
    filters: {
      search: 'Search plans'
    },
    table: {
      label: 'Plans',
      name: 'Name',
      transfer: 'Traffic',
      limits: 'Limits',
      monthPrice: 'Monthly price',
      subscriptionGroups: 'Subscription groups'
    },
    actions: {
      create: 'Create plan',
      edit: 'Edit',
      delete: 'Delete',
      assign: 'Assign',
      manageGroups: 'Manage groups',
      removeGroupNamed: 'Remove group {name}'
    },
    empty: {
      title: 'No plans yet',
      description: 'Create a plan so users can buy traffic and get subscription groups.'
    },
    detail: {
      description: 'Plan ID {id}',
      limits: 'Quota and limits',
      actions: 'Actions'
    },
    planModal: {
      createTitle: 'Create Plan',
      editTitle: 'Edit Plan',
      fields: {
        name: 'Plan name',
        transfer: 'Traffic quota',
        speedLimit: 'Speed limit',
        deviceLimit: 'Device limit',
        monthPrice: 'Monthly price (cents)'
      },
      placeholders: {
        name: 'Enter plan name'
      },
      help: {
        zeroUnlimited: '0 means no limit.'
      }
    },
    assignModal: {
      title: 'Assign Plan',
      fields: {
        userId: 'User ID',
        expireAt: 'Expire time (Unix seconds)'
      },
      placeholders: {
        userId: 'Enter user ID'
      },
      help: {
        expireAt: 'Optional. Unix time in seconds.'
      }
    },
    groupModal: {
      title: 'Plan Groups - {name}',
      description: 'Select the subscription groups this plan can access.',
      empty: 'No subscription groups',
      noDescription: 'No description'
    },
    labels: {
      noSpeedLimit: 'No speed limit',
      noDeviceLimit: 'No device limit',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} devices',
      noGroups: 'None',
      noGroupsHint: 'This plan grants no subscription groups yet.'
    },
    confirm: {
      deleteTitle: 'Delete plan {name}?',
      deleteMessage: 'This can’t be undone.',
      deleteAction: 'Delete plan'
    },
    messages: {
      loadFailed: 'Failed to load plans',
      loadGroupsFailed: 'Failed to load subscription groups',
      deleted: 'Plan {name} deleted',
      deleteFailedShort: 'Delete failed',
      nameRequired: 'Please enter a plan name',
      saveFailed: 'Save failed: {message}',
      saveFailedShort: 'Save failed',
      saved: '{name} saved',
      userIdRequired: 'Please enter a user ID',
      assignSuccess: 'Assigned successfully',
      assignFailed: 'Assign failed: {message}',
      assignFailedShort: 'Assign failed',
      toggleGroupFailed: 'Failed to toggle group: {message}',
      toggleGroupFailedShort: 'Failed to toggle group',
      groupRemoved: 'Group {group} removed from plan {plan}',
      removeGroupFailed: 'Failed to remove group: {message}',
      removeGroupFailedShort: 'Remove failed'
    }
  },
  adminUsers: {
    title: 'Users',
    subtitle: 'Accounts, subscriptions and traffic. Select a user to see the details.',
    filters: {
      searchEmail: 'Search by email',
      label: 'Filter by status',
      exhaustedHint: '“Out of traffic” narrows the users on this page only; the server has no such filter.'
    },
    table: {
      label: 'Users',
      id: 'ID',
      email: 'Email',
      plan: 'Plan',
      subscriptionTemplate: 'Subscription template',
      traffic: 'Used / total',
      limits: 'Limits',
      expireAt: 'Expires at',
      status: 'Status',
      lastOnline: 'Last online',
      createdAt: 'Created at'
    },
    lastOnline: {
      never: 'Never',
      neverHint: 'No node has reported this user yet.',
      loading: 'Loading',
      unavailable: 'Unavailable'
    },
    status: {
      active: 'Active',
      expired: 'Expired',
      banned: 'Banned',
      exhausted: 'Out of traffic'
    },
    actions: {
      addUser: 'New user',
      editUser: 'Edit user',
      ban: 'Ban',
      unban: 'Unban',
      resetTraffic: 'Reset traffic',
      copySubscribe: 'Copy subscription link',
      resetSubscribe: 'Reset subscription link',
      viewTraffic: 'Traffic in the last 30 days'
    },
    empty: {
      title: 'No users yet',
      description: 'Add the first user to give them a subscription.'
    },
    detail: {
      description: 'ID {id} · joined {date}',
      subscription: 'Subscription',
      flowReset: 'Traffic resets',
      activity: 'Activity',
      actions: 'Actions',
      danger: 'Danger zone',
      dangerFooter: 'A new subscription link means every client has to import it again. Reset traffic can’t be undone.'
    },
    editModal: {
      title: 'Edit User',
      fields: {
        email: 'Email',
        balance: 'Balance (cents)',
        transfer: 'Traffic limit (bytes)',
        speedLimit: 'Speed limit (Mbps, 0 for unlimited)',
        deviceLimit: 'Device limit (0 for unlimited)',
        groupId: 'Subscription group',
        expiredAt: 'Expire time (Unix seconds)',
        flowResetTime: 'Flow reset day',
        remark: 'Remark'
      },
      groupOptions: {
        unassigned: 'Unassigned'
      }
    },
    createModal: {
      title: 'New user',
      submit: 'Create user',
      fields: {
        email: 'Email',
        password: 'Password',
        userType: 'User type',
        groupId: 'Subscription group',
        transferEnable: 'Traffic limit (bytes)',
        speedLimit: 'Speed limit (Mbps, 0 for unlimited)',
        deviceLimit: 'Device limit (0 for unlimited)',
        flowResetTime: 'Flow reset day'
      },
      placeholders: {
        email: 'Enter email',
        password: 'Enter password (min 6 chars)',
        transferEnable: 'Leave empty for default 0',
        speedLimit: '0 means no speed limit',
        deviceLimit: '0 means no device limit'
      },
      userTypes: {
        normal: 'Normal user',
        admin: 'Admin'
      }
    },
    confirm: {
      resetSubscribeTitle: 'Reset the subscription link of {email}?',
      resetSubscribeMessage: 'The old link stops working at once and the user has to import the subscription again. This can’t be undone.',
      resetSubscribeAction: 'Reset link'
    },
    copyDialog: {
      title: 'Copy subscription link',
      description: 'The link could not be copied automatically. Copy it below.',
      label: 'Subscription link'
    },
    resetFlow: {
      userTitle: 'Reset the traffic of {email}?',
      userMessage: 'Used traffic goes back to zero. This can’t be undone.',
      usedFlow: 'Used flow',
      quota: 'Quota',
      confirmAction: 'Reset traffic'
    },
    trafficModal: {
      title: 'Traffic Detail - {email}',
      subtitle: 'Daily totals and hourly detail for the last 30 days.',
      refresh: 'Refresh',
      dailyTitle: 'Daily Traffic',
      hourlyTitle: 'Hourly Traffic',
      empty: 'No traffic records',
      fetchFailed: 'Failed to load user traffic',
      summary: {
        total30d: '30-day total',
        dailyPeak: 'Daily peak',
        hourlyPeak: 'Hourly peak'
      },
      table: {
        date: 'Date',
        hour: 'Hour',
        traffic: 'Traffic'
      }
    },
    labels: {
      admin: 'Admin',
      noSpeedLimit: 'No speed limit',
      noDeviceLimit: 'No device limit',
      speedLimitMbps: '{value} Mbps',
      deviceLimitCount: '{value} devices',
      noReset: 'No reset',
      monthlyDay: 'Day {day} of every month',
      permanent: 'Permanent',
      trafficUnlimited: '{used} used · no limit'
    },
    bulk: {
      requestFailed: 'The bulk request failed: {message}',
      banPartial: 'Banned {done} of {total} users.',
      banNone: 'No user was banned.',
      unbanPartial: 'Unbanned {done} of {total} users.',
      unbanNone: 'No user was unbanned.',
      undoPartial: '{count} users could not be restored.',
      resetTitle: 'Reset the traffic of {count} users?',
      resetMessage: 'The used traffic of every selected user goes back to zero. This can’t be undone.',
      resetDone: 'Traffic reset for {count} users',
      resetPartial: 'Reset the traffic of {done} of {total} users.',
      resetNone: 'No user’s traffic was reset.'
    },
    messages: {
      actionFailed: 'Operation failed',
      fillEmailPassword: 'Please fill in email and password',
      passwordTooShort: 'Password must be at least 6 characters',
      userCreated: 'User created successfully',
      createFailed: 'Create failed',
      fetchUsersFailed: 'Users didn’t load',
      bulkBanned: '{count} users banned',
      bulkUnbanned: '{count} users unbanned',
      fetchStatsFailed: 'Failed to fetch statistics',
      saveFailed: 'Save failed: {message}',
      userSaved: '{email} saved',
      userBanned: '{email} banned',
      userUnbanned: '{email} unbanned',
      resetFailed: 'Reset failed',
      userFlowReset: 'User traffic reset successfully',
      fetchUserFailed: 'Failed to load the user',
      noToken: 'This user has no subscription token',
      subscribeCopied: 'Subscription link copied to clipboard',
      resetSubscribeSuccess: 'Subscription link reset',
      resetSubscribeFailed: 'Failed to reset subscription'
    }
  },
  control: {
    subtitle: 'Manage official signed packages, Control WebUI extensions, and lifecycle operations.',
    actions: {
      refresh: 'Refresh', refreshing: 'Refreshing...', importRelease: 'Import release', install: 'Install',
      configure: 'Configure', enable: 'Enable', disable: 'Disable', upgrade: 'Upgrade', update: 'Upgrade', rollback: 'Rollback', cancel: 'Cancel operation', installOfficialOnly: 'Official release required',
      saving: 'Saving...', newAssignment: 'New assignment'
    },
    pluginCenter: {
      filters: { search: 'Search plugins', health: 'Filter by health', target: 'Filter by target' },
      states: { healthy: 'Healthy', attention: 'Needs attention' },
      listLabel: 'Plugins',
      official: 'Official signed package',
      loadFailed: 'The plugin catalog didn’t load',
      empty: 'No plugins match the current filters',
      emptyCatalog: { title: 'No plugins yet', description: 'Import an official signed release to add its plugin to the catalog.' },
      detail: { versions: 'Versions and state' },
      operations: { title: 'Recent plugin operations', empty: 'No recent plugin operations' }
    },
    tabs: { assignments: 'Assignments', topologies: 'Topologies' },
    table: {
      plugin: 'Plugin', release: 'Release', installation: 'Installation', version: 'Version', state: 'State', actions: 'Actions',
      scope: 'Scope', description: 'Description', topology: 'Topology', activeRevision: 'Active revision', deployment: 'Deployment',
      operation: 'Operation', chain: 'Chain', revision: 'Revision', deadline: 'Deadline', target: 'Target', version: 'Version', role: 'Role', configRevision: 'Config revision', rolloutGroup: 'Rollout group'
    },
    labels: { releases: '{count} releases', desired: 'Desired', observed: 'Observed' },
    states: { catalogued: 'Catalogued', enabled: 'Enabled', disabled: 'Disabled', loading: 'Loading control state...' },
    empty: { assignments: 'No assignments for this node', topologies: 'No topologies', operations: 'No operations' },
    activity: { scoped: 'Selected activity', all: 'All activity', showAll: 'Show all activity', showScoped: 'Show selected activity', empty: 'No activity for the selected scope' },
    topology: {
      select: 'Topology', new: 'New topology', newTitle: 'Create topology', create: 'Create topology', name: 'Name', edit: 'Edit revision', status: 'View status', noDeployment: 'No deployment', editorTitle: 'Topology revision editor', revision: 'Revision',
      noRevisions: 'No revisions', failurePolicy: 'Failure policy', stopAndRollback: 'Stop and rollback', message: 'Revision message', graphJSON: 'Graph JSON',
      graphHelp: 'Use vertices and edges. Secrets must be referenced by secret_id; inline secret values are rejected.', diagnose: 'Validate / diagnose', validating: 'Validating...',
      unsavedChanges: 'This revision has unsaved changes. Save a new immutable revision before previewing or planning.',
      valid: 'Topology is valid', invalid: 'Topology has validation issues', invalidJSON: 'Topology JSON is invalid', saveRevision: 'Save revision', plan: 'Plan deployment',
      apply: 'Apply deployment', rollback: 'Rollback deployment', deployment: 'Deployment', applyConfirm: 'Apply {topology} (deployment #{deployment})?', rollbackConfirm: 'Request rollback for {topology} (deployment #{deployment})?',
      preview: 'Read-only deployment preview', previewAction: 'Preview', previewSteps: '{count} planned steps',
      emptyHint: 'Create a topology, describe its vertices and links, and save a revision to plan a deployment.',
      viewLabel: 'Edit as', viewJSON: 'JSON', viewGraph: 'Graph', graphLabel: 'Topology graph', graphSummary: '{nodes} vertices, {edges} links',
      graphEmpty: 'No vertices yet', graphEmptyHint: 'Add vertices to the JSON and the topology is drawn here.', graphInvalidHint: 'Fix the JSON and the topology is drawn here.', graphFailed: 'Couldn’t draw the topology'
    },
    extensions: { errorsTitle: 'WebUI extension loading failed' },
    assignments: {
      node: 'Node', noNodes: 'No nodes available', agentPlugin: 'Agent plugin', enabled: 'Assignment enabled',
      createTitle: 'Create node assignment', editTitle: 'Edit node assignment', deleteTitle: 'Delete {plugin} / {role} from this node?', deleteMessage: 'The node stops running this plugin role. This can’t be undone.', deleteAction: 'Delete assignment',
      emptyHint: 'Give this node a role: pick the plugin, scope and version.', roleHelp: 'Common roles: {roles}'
    },
    install: { title: 'Install official plugin', target: 'Runtime target', version: 'Release version', enableAfterInstall: 'Enable immediately after installation' },
    update: { title: 'Upgrade official plugin' },
    config: {
      title: 'Plugin configuration', loading: 'Loading configuration...', revision: 'Configuration revision: {revision}', mode: 'Configuration editor mode', formMode: 'Form', jsonMode: 'JSON',
      selectValue: 'Select a value', addItem: 'Add item', removeItem: 'Remove item', emptyArray: 'No items', item: 'Item', invalidJSON: 'Invalid JSON',
      schemaError: '{path} does not satisfy the schema', requiredError: '{path} is required'
    },
    releaseImport: {
      title: 'Import official signed release', manifest: 'Manifest JSON', signature: 'Signature', artifact: 'Package artifact',
      artifactOptional: 'Optional; a release without an uploaded artifact cannot be installed.'
    },
    messages: {
      actionQueued: '{action} was submitted for {plugin}', operationStatus: 'Operation {id} is {state}. Chain: {chain}.', installed: 'Installation intent was saved for {plugin}', configSaved: 'Configuration was saved for {plugin}',
      releaseImported: '{plugin} {version} was imported', cancelRequested: 'Operation cancellation was requested',
      assignmentSaved: '{plugin} assignment was saved', assignmentStateSaved: '{plugin} assignment state was saved', assignmentDeleted: '{plugin} assignment was deleted',
      topologyRevisionSaved: 'Topology revision {revision} was saved', topologyPlanned: 'Deployment #{id} was planned',
      topologyApplyRequested: 'Deployment apply was requested', topologyRollbackRequested: 'Deployment rollback was requested', topologyCreated: 'Topology {name} was created'
    },
    errors: {
      load: 'Unable to load control state', action: 'Plugin operation failed', install: 'Plugin installation failed', configLoad: 'Unable to load plugin configuration',
      configSave: 'Unable to save plugin configuration', releaseImport: 'Release import failed', cancel: 'Unable to cancel operation', poll: 'Unable to refresh operation state',
      nodesLoad: 'Unable to load nodes', assignmentsLoad: 'Unable to load node assignments', assignmentSave: 'Unable to save node assignment', assignmentDelete: 'Unable to delete node assignment',
      topologyLoad: 'Unable to load topology revisions', topologyValidate: 'Unable to validate topology', topologySave: 'Unable to save topology revision', topologyPlan: 'Unable to plan topology deployment',
      topologyPreview: 'Unable to preview topology deployment', topologyStatus: 'Unable to load deployment status', topologyApply: 'Unable to apply topology deployment', topologyRollback: 'Unable to roll back topology deployment', topologyCreate: 'Unable to create topology'
    }
  },
  routeModes: {
    open: 'Route modes',
    subtitle: 'Switch each package’s v2 routes between legacy, shadow and native, and roll back to legacy.',
    back: 'Plugin Center',
    package: 'Package',
    packageMeta: 'Version {version} · config revision {revision}',
    packageDisabled: 'Disabled',
    superAdminOnly: 'Only super admins can switch route modes.',
    loadFailed: 'Route modes didn’t load',
    empty: { title: 'No packages with v2 routes', description: 'Install and enable a package that declares v2 routes to manage its route modes.' },
    modes: { legacy: 'Legacy', shadow: 'Shadow', native: 'Native' },
    modeDefault: '{mode} (default)',
    sources: {
      stored: 'Set explicitly',
      default: 'Rehearsed default: no mode is stored',
      'kill-switch': 'Default off: package_routes.default_mode is legacy',
      'package-too-old': 'Default off: the package is older than the rehearsed release',
      'identity-authority': 'Follows the identity authority',
      unset: 'No mode stored'
    },
    defaults: {
      'kill-switch': 'Native defaults are off (package_routes.default_mode: legacy). Routes without a stored mode run legacy.',
      'package-too-old': 'Version {version} is older than {min}, the rehearsed release: its routes default to legacy.'
    },
    catalog: { 'native-flagged': 'Native-flagged', bridged: 'Bridged', 'kernel-owned': 'Kernel-owned', native: 'Native', none: 'Undeclared' },
    locked: { kernel_owned: 'Kernel-owned', identity_group_a: 'Identity cutover', websocket: 'WebSocket', not_declared: 'Not declared' },
    columns: { route: 'Route', endpoint: 'Method / path', catalog: 'Eligibility', configured: 'Configured', effective: 'Effective', shadow: 'Shadow (total / mismatch / errors)', mismatch: 'Mismatch rate', mode: 'Switch to' },
    routesLabel: 'Routes of {package}',
    routeMode: 'Mode of {route}',
    effectiveDiffers: 'Differs from configured',
    hostEffective: 'Host: {mode}',
    packageMode: 'Mode for the whole package',
    setPackage: 'Set whole package',
    rollback: 'Roll back to legacy',
    wholePackage: 'Whole package',
    wholePackageTarget: '{package} (whole package)',
    dialog: {
      setTitle: 'Switch {target} to {mode}?',
      setMessage: 'Routes that can’t take this mode are skipped. The change is recorded in the revision history.',
      nativeMessage: 'In native mode the package implementation answers instead of the legacy kernel handler. Confirm and give a reason for the audit trail.',
      rollbackTitle: 'Roll {package} back to legacy?',
      rollbackMessage: 'Every route of the package returns to legacy, routes native by default included.',
      reason: 'Reason',
      reasonHelp: 'Optional; stored with the revision.',
      reasonRequired: 'Required to switch to native.',
      confirmSet: 'Switch mode',
      confirmNative: 'Switch to native',
      confirmRollback: 'Roll back'
    },
    result: '{changed} routes changed, {skipped} skipped (config revision {revision})',
    errors: { switch: 'Couldn’t switch the route mode', rollback: 'Couldn’t roll back', revisions: 'Revision history didn’t load' },
    revisions: {
      title: 'Revision history',
      label: 'Route mode revisions',
      empty: 'No route mode changes yet',
      time: 'Time', route: 'Route', change: 'Change', actor: 'Actor', reason: 'Reason', revision: 'Revision',
      actions: { set: 'Set', rollback: 'Rollback' }
    },
    mismatches: {
      title: 'Shadow mismatch samples',
      label: 'Mismatch samples',
      open: 'Samples ({count})',
      lastSeen: 'Last {time}',
      privacy: 'Samples are masked before they are stored: tokens, passwords, keys, UUIDs and subscription links are hidden, e-mail addresses keep their first letter and domain, IP addresses their first two parts. Each route keeps its latest {max} samples for {days} days.',
      loadFailed: 'Mismatch samples didn’t load',
      empty: 'No samples stored',
      emptyDescription: 'Samples appear here when a shadow run answers differently from legacy.',
      observed: 'Observed',
      status: 'Status (legacy → native)',
      diffColumn: 'Differences',
      fields: '{count} fields',
      detail: 'Sample details',
      request: 'Request',
      requestID: 'Request ID',
      version: 'Package version',
      truncated: 'Diff shortened',
      diff: 'Differences (masked)',
      copyDiff: 'Copy diff'
    }
  },
  accessGroups: {
    subtitle: 'Manage independent service-scope memberships, resource grants, and plugin-owned quota policies.',
    actions: { refresh: 'Refresh', newGroup: 'New group', open: 'Open', editGroup: 'Edit name and description', enable: 'Enable', disable: 'Disable', add: 'Add', removeNamed: 'Remove {name}', addGrant: 'Add grant', saveQuota: 'Save quota', resolve: 'Resolve access' },
    filters: { label: 'Filter by service scope', allScopes: 'All service scopes' },
    table: { group: 'Access group', scope: 'Scope', state: 'State' },
    states: { enabled: 'Enabled', disabled: 'Disabled' },
    groups: { title: 'Access groups', empty: 'No access groups in this scope', emptyAll: 'No access groups yet', emptyDescription: 'An access group gives its users and plans resource grants and quota policies within one service scope.', noDescription: 'No description', directUnion: 'Direct and plan membership are combined as an allow-union.' },
    detail: { title: 'Access group', loading: 'Loading access group details…', group: 'Group', danger: 'Danger zone' },
    members: { title: 'User members', userID: 'User ID', empty: 'No direct user members' },
    plans: { title: 'Plan memberships', planID: 'Plan ID', empty: 'No plan memberships' },
    grants: { title: 'Resource grants', resourceType: 'Resource type', resourceID: 'Resource ID', permissions: 'Permissions JSON', empty: 'No resource grants' },
    quotas: { title: 'Quota policies', key: 'Policy key', policy: 'Policy JSON', empty: 'No quota policies' },
    resolver: { title: 'Effective access preview', description: 'Preview the server-side allow-union for one user, optional plan, and service scope.', userID: 'User ID', planID: 'Plan ID (optional)', scope: 'Service scope', result: '{count} enabled groups apply', none: 'No enabled groups apply', policySummary: '{grants} grants and {quotas} quota policies are effective.' },
    editor: { createTitle: 'Create access group', editTitle: 'Edit access group', name: 'Group name', description: 'Description', enabled: 'Group is enabled', scopeFixed: 'The scope of an existing group can’t change.' },
    messages: { groupCreated: 'Created access group {name}', groupSaved: 'Saved access group {name}', groupEnabled: 'Enabled access group {name}', groupDisabled: 'Disabled access group {name}', groupDeleted: 'Deleted access group {name}', memberAdded: 'Added user #{id}', memberRemoved: 'Removed user #{id}', planAdded: 'Added plan #{id}', planRemoved: 'Removed plan #{id}', grantAdded: 'Added resource grant', grantRemoved: 'Removed resource grant', quotaSaved: 'Saved quota policy', quotaRemoved: 'Removed quota policy' },
    confirm: { deleteGroupTitle: 'Delete access group {name}?', deleteGroup: 'Its memberships, resource grants and quota policies are deleted with it. This can’t be undone.', deleteGroupAction: 'Delete group', removeGrantTitle: 'Remove resource grant #{id}?', removeGrant: 'Members of this group lose access to {resource}. This can’t be undone.', removeGrantAction: 'Remove grant', removeQuotaTitle: 'Remove quota policy {key}?', removeQuota: 'The group is no longer limited by this quota. This can’t be undone.', removeQuotaAction: 'Remove policy' },
    errors: { load: 'Unable to load access control data', loadGroups: 'Unable to load access groups', loadDetail: 'Unable to load access group details', groupRequired: 'A service scope and group name are required', saveGroup: 'Unable to save access group', deleteGroup: 'Unable to delete access group', member: 'Unable to update user membership', plan: 'Unable to update plan membership', grant: 'Unable to update resource grant', quota: 'Unable to update quota policy', resolve: 'Unable to resolve effective access', invalidID: '{label} must be a positive integer', invalidJSON: '{label} must be valid JSON', scopeRequired: 'A service scope is required' }
  },
  agentUpgrades: {
    title: 'Agent upgrades',
    description: 'Control pushes Agent releases in canary batches (5%, 25%, 100%, at least 30 minutes each) and rolls a batch back when more than 5% of it fails.',
    empty: 'No Agent upgrade yet. A super administrator starts one with:',
    target: 'Agent {version}',
    batchOf: 'batch {batch} of {total}',
    batchEnds: 'next batch at {time} at the earliest',
    finished: 'ended {time}',
    batch: 'Batch {batch} · {percent}%',
    progress: '{done} / {total} settled',
    statuses: { running: 'Running', paused: 'Paused', rolling_back: 'Rolling back', succeeded: 'Succeeded', rolled_back: 'Rolled back', aborted: 'Aborted' },
    states: { pending: 'Pending', offered: 'Offered', upgrading: 'Upgrading', succeeded: 'Upgraded', failed: 'Failed', rolled_back: 'Rolled back', skipped: 'Skipped' },
    actions: { pause: 'Pause', resume: 'Resume', abort: 'Abort', rollback: 'Abort and roll back' },
    confirm: {
      abortTitle: 'Abort the upgrade to {version}?',
      abort: 'No further node is offered the upgrade. Nodes already upgraded keep the new release.',
      rollbackTitle: 'Roll back the upgrade to {version}?',
      rollback: 'The upgraded nodes of the current batch are told to reinstate their previous release, then the campaign ends.'
    },
    loadFailed: 'Unable to load Agent upgrades'
  },
  agentTransports: {
    open: 'Agent transports',
    subtitle: 'How each node’s agent reaches Control: the mTLS stream, or a legacy channel that v4.2 will refuse.',
    back: 'NodeX Agents',
    mode: 'agent_control.mtls: {mode}',
    modes: {
      off: 'Client certificates are off; legacy agents are served silently.',
      optional: 'Client certificates are verified when presented; legacy agents are served silently.',
      preferred: 'Legacy agents are still served, with deprecation signals.',
      required: 'Only enrolled agents (client certificates) are accepted on AnixOps Agent channels.'
    },
    sunset: 'Legacy sunset: {date}',
    notice: 'v4.2 makes agent_control.mtls: required the default. Before upgrading, every node marked Legacy must run an Agent that has enrolled; legacy API-key agents will be refused. UniProxy and v2board gRPC (third-party node software) are not affected.',
    guide: 'Upgrade guide',
    filterLabel: 'Filter by status',
    statuses: { all: 'All', mtls: 'mTLS', legacy: 'Legacy', 'third-party': 'Third-party', unseen: 'Not seen' },
    legacyHint: 'Refused under required',
    transports: {
      'mtls-stream': 'mTLS stream',
      'apikey-stream': 'API key stream',
      'http-legacy': 'Legacy HTTP',
      websocket: 'WebSocket',
      'clean-agent': 'Clean agent',
      uniproxy: 'UniProxy',
      'v2board-grpc': 'v2board gRPC'
    },
    columns: { node: 'Node', status: 'Status', transport: 'Last transport', version: 'Agent version', certificate: 'Certificate', lastSeen: 'Last seen', seenOn: 'Seen on' },
    certificateUntil: 'until {date}',
    noCertificate: 'Not enrolled',
    disabled: 'Disabled',
    tableLabel: 'Agent transports by node',
    empty: { title: 'No nodes', description: 'Proxy and forward nodes appear here once they exist.' },
    loadFailed: 'Unable to load agent transports'
  }
}
