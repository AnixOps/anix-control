import { isCommercialEdition } from '@/composables/useEdition'
import { ADMIN_PAGE_SECTIONS } from '@/navigation/sections'

const PAGE_TITLE_KEYS = {
  '/login': 'pageTitles.auth.login',
  '/user/dashboard': 'pageTitles.user.dashboard',
  '/user/subscribe': 'pageTitles.user.subscribe',
  '/user/knowledge': 'pageTitles.user.knowledge',
  '/user/tickets': 'pageTitles.user.tickets',
  '/user/plans': 'pageTitles.user.plans',
  '/user/orders': 'pageTitles.user.orders',
  '/user/account': 'shell.accountPage.title',
  '/admin/dashboard': 'pageTitles.admin.dashboard',
  '/admin/monitor': 'pageTitles.admin.monitor',
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
  '/admin/agent/transports': 'pageTitles.admin.agentTransports',
  '/admin/control': 'pageTitles.admin.control',
  '/admin/plugins': 'pageTitles.admin.plugins',
  '/admin/plugins/route-modes': 'pageTitles.admin.routeModes',
  '/admin/deployments': 'pageTitles.admin.deployments',
  '/admin/access-groups': 'pageTitles.admin.accessGroups',
  '/admin/payment': 'pageTitles.admin.payment',
  '/admin/notifications': 'pageTitles.admin.notifications',
  '/admin/invite': 'pageTitles.admin.invite',
  '/admin/invite-codes': 'pageTitles.admin.inviteCodes',
  '/admin/system': 'pageTitles.admin.system',
  '/admin/security': 'pageTitles.admin.security',
  '/admin/account': 'shell.accountPage.title'
}

const DESCRIPTION_RULES = [
  { prefix: '/admin/forward', key: 'app.meta.forwardDescription' },
  { prefix: '/admin', key: 'app.meta.adminDescription' },
  { prefix: '/user', key: 'app.meta.userDescription' },
  { prefix: '/login', key: 'app.meta.loginDescription' }
]

// The community edition names plans subscription templates.
const COMMUNITY_PAGE_TITLE_KEYS = {
  '/admin/plans': 'pageTitles.admin.subscriptionTemplates'
}

// Sections of settings-style pages (/admin/system/backup → 备份).
const SECTION_TITLE_KEYS = Object.fromEntries(
  Object.values(ADMIN_PAGE_SECTIONS).flat().map(section => [section.to, section.labelKey])
)

export function resolveRoutePageTitle(t, path, fallback = '') {
  const key = (!isCommercialEdition() && COMMUNITY_PAGE_TITLE_KEYS[path]) || PAGE_TITLE_KEYS[path] || SECTION_TITLE_KEYS[path]
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
