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
import adminApiTokens from './modules/en/adminApiTokens'
import adminLicenses from './modules/en/adminLicenses'
import forwardV4 from './modules/en/forwardV4'
import forwardDns from './modules/en/forwardDns'
import egressRoutes from './modules/en/egressRoutes'
import standby from './modules/en/standby'
import topology from './modules/en/topology'
import lines from './modules/en/lines'
import mesh from './modules/en/mesh'
import reverseProxies from './modules/en/reverseProxies'
import nodeDiagnostics from './modules/en/nodeDiagnostics'
import realityTargets from './modules/en/realityTargets'
import nodeEntries from './modules/en/nodeEntries'
import nodeNetwork from './modules/en/nodeNetwork'
import nodeForwardPorts from './modules/en/nodeForwardPorts'
import nodeExitPolicy from './modules/en/nodeExitPolicy'
import nodeTemporaryProxies from './modules/en/nodeTemporaryProxies'
import nodeReach from './modules/en/nodeReach'
import agentUpgradeSheet from './modules/en/agentUpgradeSheet'

export default {
  ...adminSubscriptionGroups,
  ...adminApiTokens,
  ...adminLicenses,
  ...forwardV4,
  ...forwardDns,
  ...egressRoutes,
  ...topology,
  ...lines,
  ...mesh,
  ...standby,
  ...reverseProxies,
  ...nodeDiagnostics,
  ...realityTargets,
  ...nodeEntries,
  ...nodeNetwork,
  ...nodeForwardPorts,
  ...nodeExitPolicy,
  ...nodeTemporaryProxies,
  ...nodeReach,
  ...agentUpgradeSheet,
  ...networkPages,
  ...miscPages,
  ...adminSupportPages,
  admin: {
    nodes: adminNodes
  },
  adminPayment: {
    title: 'Payments',
    subtitle: 'Payment gateways, payment records and revenue statistics.',
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
      empty: 'No payment gateways',
      emptyDescription: 'Users can pay for plans once a gateway is added.'
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
      empty: 'No payments',
      emptyDescription: 'Order payments appear here.'
    },
    stats: {
      totalAmount: 'Total Revenue',
      totalOrders: 'Total Orders',
      successOrders: 'Successful Orders',
      successRate: 'Success Rate',
      gatewayDistribution: 'Gateway Distribution',
      empty: 'No gateway statistics'
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
      deleteMessage: 'The gateway can no longer accept payments. This cannot be undone.',
      deleteAction: 'Delete gateway'
    },
    messages: {
      fetchGatewaysFailed: 'Could not load the payment gateways',
      fetchRecordsFailed: 'Could not load the payment records',
      fetchStatsFailed: 'Could not load the payment statistics',
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
      totpBody: 'Time-based one-time passwords, set up by scanning a QR code in an app such as Google Authenticator or Authy.',
      backupBody: 'Recovery codes sign a user in without the authenticator. Each code works once.',
      lockoutBody: 'Repeated MFA failures lock the account.',
      userOpsBody: 'Users turn two-factor authentication on or off and create new recovery codes on the Account page.'
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
    subtitle: 'Free subscription templates: traffic quota, speed and device limits, and the subscription groups they grant. A template applies once assigned to a user.',
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
      title: 'No subscription templates',
      description: 'Users assigned a template receive its traffic and subscription groups.'
    },
    detail: {
      description: 'Template ID {id}'
    },
    labels: {
      noGroupsHint: 'This template grants no subscription groups.'
    },
    planModal: {
      createTitle: 'New subscription template',
      editTitle: 'Edit subscription template',
      fields: {
        name: 'Template name'
      },
      placeholders: {
        name: 'Enter template name'
      },
      help: {
        show: 'When off, users do not see this template.'
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
    subtitle: 'Purchasable plans: traffic quota, speed and device limits, price, and the subscription groups each plan grants.',
    filters: {
      search: 'Search plans'
    },
    table: {
      label: 'Plans',
      name: 'Name',
      transfer: 'Traffic',
      limits: 'Limits',
      monthPrice: 'Monthly price',
      subscriptionGroups: 'Subscription groups',
      status: 'Status'
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
      title: 'No plans',
      description: 'Users who buy a plan receive its traffic and subscription groups.'
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
        monthPrice: 'Monthly price (cents)',
        show: 'On sale'
      },
      placeholders: {
        name: 'Enter plan name'
      },
      help: {
        zeroUnlimited: '0 means no limit.',
        show: 'When off, the plan is hidden on the purchase page. Existing holders are not affected.'
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
      noGroupsHint: 'This plan grants no subscription groups.',
      onSale: 'On sale',
      offSale: 'Off sale'
    },
    confirm: {
      deleteTitle: 'Delete plan {name}?',
      deleteMessage: 'This cannot be undone.',
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
    subtitle: 'Accounts, subscriptions and traffic. Select a user to view details.',
    filters: {
      searchEmail: 'Search by email',
      label: 'Filter by status',
      exhaustedHint: '“Out of traffic” filters the users on this page only.'
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
      neverHint: 'No node has reported this user.',
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
      title: 'No users',
      description: 'Subscriptions can be assigned once users are added.'
    },
    detail: {
      description: 'ID {id} · joined {date}',
      subscription: 'Subscription',
      flowReset: 'Traffic resets',
      activity: 'Activity',
      actions: 'Actions',
      danger: 'Danger zone',
      dangerFooter: 'After a subscription link reset, every client must import it again. A traffic reset cannot be undone.'
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
      resetSubscribeMessage: 'The old link stops working immediately and the user must import the subscription again. This cannot be undone.',
      resetSubscribeAction: 'Reset link'
    },
    copyDialog: {
      title: 'Copy subscription link',
      description: 'Automatic copy failed. Copy the link below.',
      label: 'Subscription link'
    },
    resetFlow: {
      userTitle: 'Reset the traffic of {email}?',
      userMessage: 'Used traffic is reset to zero. This cannot be undone.',
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
      resetMessage: 'Used traffic of all selected users is reset to zero. This cannot be undone.',
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
      fetchUsersFailed: 'Could not load the users',
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
    config: {
      title: 'Plugin configuration', loading: 'Loading configuration...', revision: 'Configuration revision: {revision}', mode: 'Configuration editor mode', formMode: 'Form', jsonMode: 'JSON',
      selectValue: 'Select a value', addItem: 'Add item', removeItem: 'Remove item', emptyArray: 'No items', item: 'Item', invalidJSON: 'Invalid JSON',
      schemaError: '{path} does not satisfy the schema', requiredError: '{path} is required'
    },
    messages: { configSaved: 'Configuration was saved for {plugin}' },
    errors: { configLoad: 'Unable to load plugin configuration', configSave: 'Unable to save plugin configuration' }
  },
  accessGroups: {
    subtitle: 'Manage independent service-scope memberships, resource grants, and plugin-owned quota policies.',
    actions: { refresh: 'Refresh', newGroup: 'New group', open: 'Open', editGroup: 'Edit name and description', enable: 'Enable', disable: 'Disable', add: 'Add', removeNamed: 'Remove {name}', addGrant: 'Add grant', saveQuota: 'Save quota', resolve: 'Resolve access' },
    filters: { label: 'Filter by service scope', allScopes: 'All service scopes' },
    table: { group: 'Access group', scope: 'Scope', state: 'State' },
    states: { enabled: 'Enabled', disabled: 'Disabled' },
    groups: { title: 'Access groups', empty: 'No access groups in this scope', emptyAll: 'No access groups', emptyDescription: 'An access group gives its users and plans resource grants and quota policies within one service scope.', noDescription: 'No description', directUnion: 'Direct and plan membership are combined as an allow-union.' },
    detail: { title: 'Access group', loading: 'Loading access group details…', group: 'Group', danger: 'Danger zone' },
    members: { title: 'User members', userID: 'User ID', empty: 'No direct user members' },
    plans: { title: 'Plan memberships', planID: 'Plan ID', empty: 'No plan memberships' },
    grants: { title: 'Resource grants', resourceType: 'Resource type', resourceID: 'Resource ID', permissions: 'Permissions JSON', empty: 'No resource grants' },
    quotas: { title: 'Quota policies', key: 'Policy key', policy: 'Policy JSON', empty: 'No quota policies' },
    resolver: { title: 'Effective access preview', description: 'Preview the effective access of a user, plan and service scope.', userID: 'User ID', planID: 'Plan ID (optional)', scope: 'Service scope', result: '{count} enabled groups apply', none: 'No enabled groups apply', policySummary: '{grants} grants and {quotas} quota policies are effective.' },
    editor: { createTitle: 'Create access group', editTitle: 'Edit access group', name: 'Group name', description: 'Description', enabled: 'Group is enabled', scopeFixed: 'The scope of an existing group cannot change.' },
    messages: { groupCreated: 'Created access group {name}', groupSaved: 'Saved access group {name}', groupEnabled: 'Enabled access group {name}', groupDisabled: 'Disabled access group {name}', groupDeleted: 'Deleted access group {name}', memberAdded: 'Added user #{id}', memberRemoved: 'Removed user #{id}', planAdded: 'Added plan #{id}', planRemoved: 'Removed plan #{id}', grantAdded: 'Added resource grant', grantRemoved: 'Removed resource grant', quotaSaved: 'Saved quota policy', quotaRemoved: 'Removed quota policy' },
    confirm: { deleteGroupTitle: 'Delete access group {name}?', deleteGroup: 'Its memberships, resource grants and quota policies are deleted with it. This cannot be undone.', deleteGroupAction: 'Delete group', removeGrantTitle: 'Remove resource grant #{id}?', removeGrant: 'Members of this group lose access to {resource}. This cannot be undone.', removeGrantAction: 'Remove grant', removeQuotaTitle: 'Remove quota policy {key}?', removeQuota: 'The group is no longer limited by this quota. This cannot be undone.', removeQuotaAction: 'Remove policy' },
    errors: { load: 'Unable to load access control data', loadGroups: 'Unable to load access groups', loadDetail: 'Unable to load access group details', groupRequired: 'A service scope and group name are required', saveGroup: 'Unable to save access group', deleteGroup: 'Unable to delete access group', member: 'Unable to update user membership', plan: 'Unable to update plan membership', grant: 'Unable to update resource grant', quota: 'Unable to update quota policy', resolve: 'Unable to resolve effective access', invalidID: '{label} must be a positive integer', invalidJSON: '{label} must be valid JSON', scopeRequired: 'A service scope is required' }
  },
  agentUpgrades: {
    title: 'Agent upgrades',
    description: 'Agent releases roll out in batches of 5%, 25% and 100%, at least 30 minutes each. A batch rolls back when more than 5% of it fails.',
    empty: 'No Agent upgrades. A super administrator starts one with:',
    emptyShort: 'No Agent upgrades.',
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
      abort: 'No more nodes receive the upgrade. Upgraded nodes keep the new release.',
      rollbackTitle: 'Roll back the upgrade to {version}?',
      rollback: 'Upgraded nodes in the current batch return to their previous release, and the upgrade ends.'
    },
    loadFailed: 'Unable to load Agent upgrades'
  },
  // The channel names of the node page's Agent connection (the page that
  // listed every node's channel is gone).
  agentTransports: {
    transports: {
      'mtls-stream': 'mTLS stream'
    }
  }
}
