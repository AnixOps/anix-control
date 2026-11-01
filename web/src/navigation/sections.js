// Sections of the pages built on the settings template (UI U7). The
// sidebar shows only the page; the command palette lists every section
// under it, so 访问组 or 审计日志 can still be found by name.
export const ADMIN_PAGE_SECTIONS = Object.freeze({
  // 节点 (one network area, docs/design/node-unification): the forwarding
  // pages are sections of the node area, found by name in the palette while
  // the forward package serves them (`capability`).
  nodes: [
    { id: 'forward-overview', to: '/admin/forward/overview', labelKey: 'pageTitles.admin.forwardV4Overview', capability: 'forward.v4' },
    { id: 'forward-routes', to: '/admin/forward/routes', labelKey: 'pageTitles.admin.forwardV4Routes', capability: 'forward.v4' },
    { id: 'forward-dns', to: '/admin/forward/dns', labelKey: 'pageTitles.admin.forwardV4Dns', capability: 'forward.v4' },
    { id: 'forward-egress', to: '/admin/forward/egress', labelKey: 'pageTitles.admin.forwardEgress' },
    { id: 'forward-reverse-proxies', to: '/admin/forward/reverse-proxies', labelKey: 'pageTitles.admin.reverseProxies' }
  ],
  // 流量与监控 (UI U8): 实时节点 is the page itself (/admin/monitor).
  monitor: [
    { id: 'monitor-live', to: '/admin/monitor/live', labelKey: 'adminMonitor.sections.live' },
    { id: 'monitor-traffic', to: '/admin/monitor/traffic', labelKey: 'adminMonitor.sections.traffic' },
    { id: 'monitor-destinations', to: '/admin/monitor/destinations', labelKey: 'adminMonitor.sections.destinations' },
    { id: 'monitor-categories', to: '/admin/monitor/categories', labelKey: 'adminMonitor.sections.categories' }
  ],
  settings: [
    { id: 'settings-general', to: '/admin/system/general', labelKey: 'adminSettings.sections.general' },
    { id: 'settings-data', to: '/admin/system/data', labelKey: 'adminSettings.sections.data' },
    { id: 'settings-backup', to: '/admin/system/backup', labelKey: 'adminSettings.sections.backup' },
    { id: 'settings-audit', to: '/admin/system/audit', labelKey: 'adminSettings.sections.audit' },
    // 内置包: the status of the built-in packages (it replaces 插件中心).
    { id: 'settings-packages', to: '/admin/system/packages', labelKey: 'adminSettings.sections.packages' },
    { id: 'settings-advanced', to: '/admin/system/advanced', labelKey: 'adminSettings.sections.advanced' },
    { id: 'settings-about', to: '/admin/system/about', labelKey: 'adminSettings.sections.about' }
  ],
  security: [
    { id: 'security-mfa', to: '/admin/security/mfa', labelKey: 'adminSecurity.sections.mfa' },
    { id: 'security-access-groups', to: '/admin/security/access-groups', labelKey: 'adminSecurity.sections.accessGroups' },
    { id: 'security-api-tokens', to: '/admin/security/api-tokens', labelKey: 'adminSecurity.sections.apiTokens' }
  ],
  notifications: [
    { id: 'notifications-email', to: '/admin/notifications/email', labelKey: 'adminNotify.channels.email' },
    { id: 'notifications-telegram', to: '/admin/notifications/telegram', labelKey: 'adminNotify.channels.telegram' },
    { id: 'notifications-templates', to: '/admin/notifications/templates', labelKey: 'adminNotify.channels.templates' },
    { id: 'notifications-logs', to: '/admin/notifications/logs', labelKey: 'adminNotify.channels.logs' }
  ]
})
