import { openAgentUpgrade } from '@/composables/useAgentUpgradeSheet'
import { ADMIN_PAGE_SECTIONS } from './sections'

// Navigation config: the one place that says what the shells show.
//
// The admin sidebar, the admin command palette, the user top navigation and
// the user tab bar all render what the builders below return. The shells
// never filter or reorder on their own. A built-in item says:
//
//   id         stable key (tests, data-nav-item)
//   to         route path
//   icon       name in navigation/icons.js
//   labelKey   i18n key; labelKeys picks one per edition
//   edition    'commercial' | 'community': shown only in that edition
//   permission kernel permission the admin needs (userStore.hasPermission)
//   match      extra route paths that select this item (exact, or a prefix
//              ending in '/'); `to` and its sub-paths always match
//   optional   true: shown only when the router has the route (for a page
//              whose route lands in a separate pull request)
//   capability shown only when a package provides it (extensions/runtime.js
//              catalogCapabilities, e.g. forward.v4); page sections
//              (sections.js) take it too
//
// Plugin menus (extensions/runtime.js, signed WebUI contract) are merged in
// by their `parent`: services and operations go to 扩展 (Extensions),
// system to 系统 (System), unknown parents to 扩展. Each plugin menu needs
// its permission and must not belong to a package the edition hides.

export const ADMIN_MENU = Object.freeze([
  {
    id: 'overview',
    labelKey: 'shell.admin.groups.overview',
    items: [
      { id: 'dashboard', to: '/admin/dashboard', icon: 'dashboard', labelKey: 'shell.admin.items.dashboard' },
      // 流量与监控 merges 实时监控, 小时流量 and 转发可观测性 (UI U8).
      { id: 'monitor', to: '/admin/monitor', icon: 'monitor', labelKey: 'shell.admin.items.monitor', match: ['/admin/traffic-hourly', '/admin/forward/observability'] }
    ]
  },
  {
    id: 'users',
    labelKey: 'shell.admin.groups.users',
    items: [
      { id: 'users', to: '/admin/users', icon: 'users', labelKey: 'shell.admin.items.users' },
      { id: 'invite-codes', to: '/admin/invite-codes', icon: 'invite-codes', labelKey: 'shell.admin.items.inviteCodes' },
      { id: 'subscriptions', to: '/admin/subscriptions', icon: 'subscriptions', labelKey: 'shell.admin.items.subscriptions' },
      // The community edition calls plans subscription templates; the
      // commercial edition sells them and lists them under 商业.
      { id: 'templates', to: '/admin/plans', icon: 'templates', labelKey: 'shell.admin.items.templates', edition: 'community' },
      { id: 'tickets', to: '/admin/tickets', icon: 'tickets', labelKey: 'shell.admin.items.tickets' },
      { id: 'knowledge', to: '/admin/knowledge', icon: 'knowledge', labelKey: 'shell.admin.items.knowledge' }
    ]
  },
  {
    id: 'network',
    labelKey: 'shell.admin.groups.network',
    // 节点 is the one network area (docs/design/node-unification): every
    // machine, relays included, and the forwarding pages (概览, 路由, DNS,
    // 分流 and a node's 转发设置) as its sections under /admin/forward/,
    // switched by the area's segmented control. There is no separate 转发
    // item. 部署编排 (topologies, node roles, deployments) belongs with the
    // nodes.
    items: [
      { id: 'nodes', to: '/admin/nodes', icon: 'nodes', labelKey: 'shell.admin.items.nodes', match: ['/admin/forward/'] },
      { id: 'deployments', to: '/admin/deployments', icon: 'deployments', labelKey: 'shell.admin.items.deployments' }
    ]
  },
  {
    // Only the packages' own WebUI menus land here (EXTENSION_PARENT_GROUPS):
    // the built-in packages have no page of their own any more, their status
    // is 系统设置 → 内置包 (/admin/system/packages).
    id: 'extensions',
    labelKey: 'shell.admin.groups.extensions',
    items: []
  },
  {
    id: 'system',
    labelKey: 'shell.admin.groups.system',
    items: [
      { id: 'settings', to: '/admin/system', icon: 'settings', labelKey: 'shell.admin.items.settings' },
      // 安全 holds the MFA policy and the access groups; 通知 holds e-mail,
      // Telegram, templates and the log (UI U7). The old paths redirect.
      { id: 'security', to: '/admin/security', icon: 'security', labelKey: 'shell.admin.items.security', match: ['/admin/mfa', '/admin/access-groups'] },
      { id: 'notifications', to: '/admin/notifications', icon: 'notifications', labelKey: 'shell.admin.items.notifications', match: ['/admin/telegram'] }
    ]
  },
  {
    id: 'commerce',
    labelKey: 'shell.admin.groups.commerce',
    items: [
      { id: 'plans', to: '/admin/plans', icon: 'plans', labelKey: 'shell.admin.items.plans', edition: 'commercial' },
      { id: 'orders', to: '/admin/orders', icon: 'orders', labelKey: 'shell.admin.items.orders', edition: 'commercial' },
      { id: 'coupons', to: '/admin/coupons', icon: 'coupons', labelKey: 'shell.admin.items.coupons', edition: 'commercial' },
      { id: 'payment', to: '/admin/payment', icon: 'payment', labelKey: 'shell.admin.items.payment', edition: 'commercial' },
      { id: 'invite', to: '/admin/invite', icon: 'invite', labelKey: 'shell.admin.items.invite', edition: 'commercial' }
    ]
  }
])

// Where plugin menus land, by their (normalized) WebUI parent.
export const EXTENSION_PARENT_GROUPS = Object.freeze({
  services: 'extensions',
  operations: 'extensions',
  system: 'system',
  extensions: 'extensions'
})

// Sections of settings-style pages: navigation/sections.js (kept apart so
// the page titles in the entry chunk do not pull in this file).
export { ADMIN_PAGE_SECTIONS }

// Quick actions in the command palette open an existing create flow (`to`)
// or run a function (`run`). Each is shown only when its page is in the
// visible menu (`page` = menu item id) and, with `section`, that section of
// the page is visible too.
export const ADMIN_QUICK_ACTIONS = Object.freeze([
  { id: 'join-node', page: 'nodes', icon: 'nodes', labelKey: 'shell.palette.actions.addNode', to: { path: '/admin/nodes', query: { join: '1' } } },
  { id: 'add-user', page: 'users', icon: 'users', labelKey: 'shell.palette.actions.addUser', to: { path: '/admin/users', query: { create: '1' } } },
  // 新建转发路由 needs the 路由 section (the forward package).
  { id: 'forward-route', page: 'nodes', section: 'forward-routes', icon: 'forward', labelKey: 'shell.palette.actions.newForwardRoute', to: { path: '/admin/forward/routes/new' } },
  // 升级节点 opens the sheet the admin shell mounts (no route).
  { id: 'upgrade-nodes', page: 'nodes', icon: 'deployments', labelKey: 'shell.palette.actions.upgradeNodes', run: openAgentUpgrade }
])

export const ADMIN_ACCOUNT_PATH = '/admin/account'
export const USER_ACCOUNT_PATH = '/user/account'

// User navigation: top links on wide screens; the items with `tab` form the
// phone tab bar (five at most), the rest move into the account menu there.
export const USER_MENU = Object.freeze([
  { id: 'dashboard', to: '/user/dashboard', icon: 'home', labelKey: 'shell.user.items.dashboard', tab: true },
  { id: 'subscribe', to: '/user/subscribe', icon: 'subscribe', labelKey: 'shell.user.items.subscribe', tab: true },
  { id: 'knowledge', to: '/user/knowledge', icon: 'help', labelKey: 'shell.user.items.knowledge', shortLabelKey: 'shell.user.items.knowledgeShort', tab: true },
  { id: 'tickets', to: '/user/tickets', icon: 'conversation', labelKey: 'shell.user.items.tickets', tab: true },
  { id: 'traffic', to: '/user/traffic', icon: 'traffic', labelKey: 'shell.user.items.traffic' },
  { id: 'plans', to: '/user/plans', icon: 'plans', labelKey: 'shell.user.items.plans', edition: 'commercial' },
  { id: 'orders', to: '/user/orders', icon: 'orders', labelKey: 'shell.user.items.orders', edition: 'commercial' },
  { id: 'account', to: USER_ACCOUNT_PATH, icon: 'account', labelKey: 'shell.user.items.account', tab: true }
])

function itemVisible(item, { editionAllows, hasPermission, routeExists, hasCapability }) {
  if (item.edition && !editionAllows(item.edition)) return false
  if (item.permission && !hasPermission(item.permission)) return false
  if (item.capability && !hasCapability(item.capability)) return false
  if (item.optional && routeExists && !routeExists(item.to)) return false
  return true
}

function compareExtensionMenus(left, right) {
  const leftOrder = Number.isSafeInteger(left.order) ? left.order : 1000
  const rightOrder = Number.isSafeInteger(right.order) ? right.order : 1000
  return leftOrder - rightOrder ||
    String(left.label || '').localeCompare(String(right.label || '')) ||
    String(left.id || '').localeCompare(String(right.id || ''))
}

/**
 * buildAdminMenu merges the built-in items, the edition, the admin's
 * permissions and the plugin menus into the groups the sidebar renders.
 * Pure: everything it depends on is passed in (tests drive it directly).
 *
 * @returns {{ id, label, items: { id, to, label, icon, match, exact, source }[] }[]}
 *   groups in order, without empty ones.
 */
export function buildAdminMenu({
  t,
  menu = ADMIN_MENU,
  editionAllows = () => true,
  hasPermission = () => true,
  routeExists = null,
  hasCapability = () => false,
  extensionMenus = [],
  extensionMenuAllowed = () => true,
  normalizeParent = value => value
}) {
  const context = { editionAllows, hasPermission, routeExists, hasCapability }
  const groups = menu.map(group => ({
    id: group.id,
    label: t(group.labelKey),
    items: group.items
      .filter(item => itemVisible(item, context))
      .map(item => ({
        id: item.id,
        to: item.to,
        label: t(item.labelKey),
        icon: item.icon,
        match: item.match || [],
        exact: Boolean(item.exact),
        sections: (ADMIN_PAGE_SECTIONS[item.id] || []).filter(section => !section.capability || hasCapability(section.capability)),
        source: 'core'
      }))
  }))

  const byId = new Map(groups.map(group => [group.id, group]))
  const extensions = (extensionMenus || [])
    .filter(menu => menu && typeof menu.to === 'string' && hasPermission(menu.permission) && extensionMenuAllowed(menu))
    .slice()
    .sort(compareExtensionMenus)
  for (const menu of extensions) {
    const parent = normalizeParent(menu.parent)
    const group = byId.get(EXTENSION_PARENT_GROUPS[parent] || 'extensions')
    group.items.push({
      id: menu.id,
      to: menu.to,
      label: menu.label,
      icon: menu.icon,
      match: [],
      exact: false,
      source: 'extension',
      parent
    })
  }

  return groups.filter(group => group.items.length > 0)
}

export function buildUserMenu({ t, editionAllows = () => true }) {
  return USER_MENU
    .filter(item => itemVisible(item, { editionAllows, hasPermission: () => true }))
    .map(item => ({
      id: item.id,
      to: item.to,
      label: t(item.labelKey),
      shortLabel: t(item.shortLabelKey || item.labelKey),
      icon: item.icon,
      tab: Boolean(item.tab)
    }))
}

function pathMatches(candidate, path, exact) {
  if (candidate.endsWith('/')) return path.startsWith(candidate)
  return path === candidate || (!exact && path.startsWith(`${candidate}/`))
}

export function menuItemMatches(item, path) {
  if (!path) return false
  if (pathMatches(item.to, path, item.exact)) return true
  return (item.match || []).some(candidate => pathMatches(candidate, path, true))
}

// activeMenuItem picks the one selected item for a path: an exact `to` wins
// over a `match` entry, which wins over a sub-path, so two items are never
// selected at once.
export function activeMenuItem(groups, path) {
  const items = groups.flatMap(group => group.items.map(item => ({ item, group })))
  const exact = items.find(({ item }) => item.to === path)
  if (exact) return exact
  const listed = items.find(({ item }) => (item.match || []).some(candidate => pathMatches(candidate, path, true)))
  if (listed) return listed
  return items.find(({ item }) => menuItemMatches(item, path)) || null
}

/**
 * paletteEntries lists what the command palette can open: every visible
 * page (sidebar items, the sections of settings-style pages, the account
 * page) and the quick actions whose page is visible.
 */
export function paletteEntries({ t, groups }) {
  const pages = []
  const seen = new Set()
  const add = (entry, context) => {
    if (seen.has(entry.to)) return
    seen.add(entry.to)
    pages.push({ ...entry, kind: 'page', context, keywords: entry.to.replace(/^\/admin\//, '').replace(/[/-]/g, ' ') })
  }
  for (const group of groups) {
    for (const item of group.items) {
      add({ id: item.id, to: item.to, label: item.label, icon: item.icon }, group.label)
    }
  }
  // Sections of settings-style pages, under their page.
  for (const group of groups) {
    for (const item of group.items) {
      for (const section of item.sections || ADMIN_PAGE_SECTIONS[item.id] || []) {
        add({ id: section.id, to: section.to, label: t(section.labelKey), icon: item.icon }, `${group.label} · ${item.label}`)
      }
    }
  }
  add({ id: 'account', to: ADMIN_ACCOUNT_PATH, label: t('shell.admin.items.account'), icon: 'account' }, '')

  const visibleIds = new Set(groups.flatMap(group => group.items.map(item => item.id)))
  const visibleSections = new Set(pages.map(page => page.id))
  const actions = ADMIN_QUICK_ACTIONS
    .filter(action => visibleIds.has(action.page) && (!action.section || visibleSections.has(action.section)))
    .map(action => ({
      id: action.id,
      kind: 'action',
      label: t(action.labelKey),
      icon: action.icon,
      to: action.to,
      run: action.run,
      keywords: action.id.replace(/-/g, ' ')
    }))

  return { pages, actions }
}

// matchesQuery: case-insensitive; every word of the query must appear in the
// label, the context or the keywords (route path words, so "tunnel" finds
// 隧道管理 in Chinese too).
export function matchesQuery(entry, query) {
  const words = String(query || '').trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return true
  const haystack = `${entry.label} ${entry.context || ''} ${entry.keywords || ''}`.toLowerCase()
  return words.every(word => haystack.includes(word))
}
