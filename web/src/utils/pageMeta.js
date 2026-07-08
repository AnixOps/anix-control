const PAGE_TITLE_KEYS = {
  '/login': 'pageTitles.auth.login',
  '/user/dashboard': 'pageTitles.user.dashboard',
  '/user/subscribe': 'pageTitles.user.subscribe',
  '/user/knowledge': 'pageTitles.user.knowledge',
  '/user/tickets': 'pageTitles.user.tickets',
  '/user/plans': 'pageTitles.user.plans',
  '/user/orders': 'pageTitles.user.orders',
  '/admin/dashboard': 'pageTitles.admin.dashboard',
  '/admin/monitor': 'pageTitles.admin.monitor',
  '/admin/traffic-hourly': 'pageTitles.admin.trafficHourly',
  '/admin/users': 'pageTitles.admin.users',
  '/admin/nodes': 'pageTitles.admin.nodes',
  '/admin/subscriptions': 'pageTitles.admin.subscriptions',
  '/admin/orders': 'pageTitles.admin.orders',
  '/admin/plans': 'pageTitles.admin.plans',
  '/admin/tickets': 'pageTitles.admin.tickets',
  '/admin/coupons': 'pageTitles.admin.coupons',
  '/admin/knowledge': 'pageTitles.admin.knowledge',
  '/admin/forward': 'pageTitles.admin.forward',
  '/admin/forward/tunnel': 'pageTitles.admin.forwardTunnel',
  '/admin/forward/limit': 'pageTitles.admin.forwardLimit',
  '/admin/forward/ansible-machines': 'pageTitles.admin.forwardAnsibleMachines',
  '/admin/forward/nodes': 'pageTitles.admin.forwardNodes',
  '/admin/forward/local': 'pageTitles.admin.forwardLocal',
  '/admin/forward/nodex': 'pageTitles.admin.forwardNodeX',
  '/admin/forward/agents': 'pageTitles.admin.forwardAgents',
  '/admin/agent': 'pageTitles.admin.forwardAgents',
  '/admin/payment': 'pageTitles.admin.payment',
  '/admin/telegram': 'pageTitles.admin.telegram',
  '/admin/mfa': 'pageTitles.admin.mfa',
  '/admin/notifications': 'pageTitles.admin.notifications',
  '/admin/invite': 'pageTitles.admin.invite',
  '/admin/system': 'pageTitles.admin.system'
}

const DESCRIPTION_RULES = [
  { prefix: '/admin/forward', key: 'app.meta.forwardDescription' },
  { prefix: '/admin', key: 'app.meta.adminDescription' },
  { prefix: '/user', key: 'app.meta.userDescription' },
  { prefix: '/login', key: 'app.meta.loginDescription' }
]

export function resolveRoutePageTitle(t, path, fallback = '') {
  const key = PAGE_TITLE_KEYS[path]
  return key ? t(key) : fallback
}

export function resolveRouteMetaDescription(t, path, fallback = '') {
  const rule = DESCRIPTION_RULES.find(item => path.startsWith(item.prefix))
  return rule ? t(rule.key) : fallback
}

export function resolveDocumentTitle(t, path, appName) {
  const pageTitle = resolveRoutePageTitle(t, path, '')
  return pageTitle && pageTitle !== appName ? `${pageTitle} | ${appName}` : appName
}
