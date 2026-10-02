// Sections of the pages built on the settings template (UI U7). The
// sidebar shows only the page; the command palette lists every section
// under it, so 访问组 or 审计日志 can still be found by name.
export const ADMIN_PAGE_SECTIONS = Object.freeze({
  // 流量与监控 (UI U8): 实时节点 is the page itself (/admin/monitor).
  monitor: [
    { id: 'monitor-live', to: '/admin/monitor/live', labelKey: 'adminMonitor.sections.live' },
    { id: 'monitor-traffic', to: '/admin/monitor/traffic', labelKey: 'adminMonitor.sections.traffic' },
    { id: 'monitor-latency', to: '/admin/monitor/latency', labelKey: 'adminMonitor.sections.latency' },
    { id: 'monitor-forward', to: '/admin/monitor/forward', labelKey: 'adminMonitor.sections.forward' }
  ],
  settings: [
    { id: 'settings-general', to: '/admin/system/general', labelKey: 'adminSettings.sections.general' },
    { id: 'settings-runtime', to: '/admin/system/runtime', labelKey: 'adminSettings.sections.runtime' },
    { id: 'settings-backup', to: '/admin/system/backup', labelKey: 'adminSettings.sections.backup' },
    { id: 'settings-balancer', to: '/admin/system/balancer', labelKey: 'adminSettings.sections.balancer' },
    { id: 'settings-audit', to: '/admin/system/audit', labelKey: 'adminSettings.sections.audit' },
    { id: 'settings-about', to: '/admin/system/about', labelKey: 'adminSettings.sections.about' }
  ],
  security: [
    { id: 'security-mfa', to: '/admin/security/mfa', labelKey: 'adminSecurity.sections.mfa' },
    { id: 'security-access-groups', to: '/admin/security/access-groups', labelKey: 'adminSecurity.sections.accessGroups' }
  ],
  notifications: [
    { id: 'notifications-email', to: '/admin/notifications/email', labelKey: 'adminNotify.channels.email' },
    { id: 'notifications-telegram', to: '/admin/notifications/telegram', labelKey: 'adminNotify.channels.telegram' },
    { id: 'notifications-templates', to: '/admin/notifications/templates', labelKey: 'adminNotify.channels.templates' },
    { id: 'notifications-logs', to: '/admin/notifications/logs', labelKey: 'adminNotify.channels.logs' }
  ]
})
