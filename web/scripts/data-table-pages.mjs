// Pages moved to the list page template in UI U6 (plan §7.1): their tables
// are UiDataTable, never a bare <table>. ESLint (vue/no-restricted-html-elements)
// and src/__tests__/dataTableGuard.test.js read this list; add a page when it
// migrates (U7 and U8 add theirs).
export const DATA_TABLE_PAGES = [
  'src/views/admin/Users.vue',
  'src/views/admin/Tickets.vue',
  'src/views/admin/Plugins.vue',
  'src/views/admin/Agent.vue',
  'src/views/admin/Orders.vue',
  'src/views/admin/Coupons.vue',
  'src/views/admin/Plans.vue',
  'src/views/admin/Payment.vue',
  'src/views/admin/Knowledge.vue',
  'src/views/admin/InviteCodes.vue',
  'src/views/admin/AccessGroups.vue',
  'src/views/admin/Invite.vue',
  'src/views/admin/AnsibleMachines.vue',
  // U7: nodes (list, node page and its table sections)
  'src/views/admin/Nodes.vue',
  'src/views/admin/NodeDetail.vue',
  'src/views/admin/nodes/NodeProtocolsSection.vue',
  'src/views/admin/nodes/NodeLogsSection.vue',
  // U7: forward nodes
  'src/views/admin/ForwardNodes.vue',
  'src/views/admin/forward-nodes/NodeXRulesPanel.vue',
  'src/views/admin/forward-nodes/RuntimeJobsTable.vue',
  // U7: forward suite
  'src/views/admin/Forward.vue',
  'src/views/admin/Tunnel.vue',
  'src/views/admin/LimitI18n.vue',
  // U7: settings, subscription groups, notifications, security
  'src/views/admin/System.vue',
  'src/views/admin/system/SettingsGeneral.vue',
  'src/views/admin/system/SettingsRuntime.vue',
  'src/views/admin/system/SettingsBackup.vue',
  'src/views/admin/system/SettingsBalancer.vue',
  'src/views/admin/system/SettingsAudit.vue',
  'src/views/admin/Subscriptions.vue',
  'src/views/admin/SubscriptionGroup.vue',
  'src/views/admin/subscriptions/GroupTemplates.vue',
  'src/views/admin/subscriptions/GroupProtocols.vue',
  'src/views/admin/Notifications.vue',
  'src/views/admin/notifications/NotifyTelegram.vue',
  'src/views/admin/notifications/NotifyTemplates.vue',
  'src/views/admin/notifications/NotifyLogs.vue',
  'src/views/admin/Security.vue',
  // U8: deployments (the operation timeline is a list, shared with Plugins)
  'src/views/admin/Deployments.vue',
  'src/views/admin/deployments/DeploymentTopologiesPanel.vue',
  'src/views/admin/deployments/DeploymentAssignmentsPanel.vue',
  'src/components/admin/OperationTimeline.vue'
]
