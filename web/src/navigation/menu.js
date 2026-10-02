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
    // Node (proxy service, /admin/nodes) and ForwardNode (forward execution,
    // /admin/forward/nodes) are different resources: separate items.
    items: [
      { id: 'nodes', to: '/admin/nodes', icon: 'nodes', labelKey: 'shell.admin.items.nodes' },
      {
        id: 'forward',
        to: '/admin/forward',
        icon: 'forward',
        labelKey: 'shell.admin.items.forward',
        exact: true,
        match: ['/admin/forward/setup', '/admin/forward/tunnel', '/admin/forward/limit']
      },
      {
        id: 'forward-nodes',
        to: '/admin/forward/nodes',
        icon: 'forward-nodes',
        labelKey: 'shell.admin.items.forwardNodes',
        match: ['/admin/forward/ansible-machines', '/admin/forward/ansible-machines/', '/admin/forward/local', '/admin/forward/nodex']
      },
      { id: 'agents', to: '/admin/agent', icon: 'agents', labelKey: 'shell.admin.items.agents', match: ['/admin/forward/agents'] }
    ]
  },
  {
    id: 'extensions',
    labelKey: 'shell.admin.groups.extensions',
    items: [
      { id: 'plugins', to: '/admin/plugins', icon: 'plugins', labelKey: 'shell.admin.items.plugins' },
      { id: 'deployments', to: '/admin/deployments', icon: 'deployments', labelKey: 'shell.admin.items.deployments' }
    ]
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

// The forward suite (flux-panel clone): its own sub-navigation, rendered once
// at the top of every /admin/forward* page except the 转发节点 pages
// (ForwardSuiteNav, showsForwardSuiteNav). The sidebar
// only links into it (转发, 转发节点, NodeX Agents).
export const FORWARD_SUITE_LINKS = Object.freeze({
  core: [
    { id: 'forward-setup', to: '/admin/forward/setup', icon: 'setup', labelKey: 'forwardSuite.nav.setupWizard', hintKey: 'forwardSuite.hints.setupWizard' },
    { id: 'forward-rules', to: '/admin/forward', icon: 'forward', labelKey: 'forwardSuite.nav.forwards' },
    { id: 'forward-tunnel', to: '/admin/forward/tunnel', icon: 'tunnel', labelKey: 'forwardSuite.nav.tunnels' },
    { id: 'forward-limit', to: '/admin/forward/limit', icon: 'limits', labelKey: 'forwardSuite.nav.limits' },
    { id: 'forward-topology', to: '/admin/forward/nodes', icon: 'forward-nodes', labelKey: 'forwardSuite.nav.nodeXTopology', hintKey: 'forwardSuite.hints.nodeXTopology' }
  ],
  advanced: [
    { id: 'forward-ansible', to: '/admin/forward/ansible-machines', icon: 'ansible', labelKey: 'forwardSuite.nav.ansibleMachines', hintKey: 'forwardSuite.hints.ansibleMachines' },
    { id: 'forward-local', to: '/admin/forward/local', icon: 'local', labelKey: 'forwardSuite.nav.localRuntime', hintKey: 'forwardSuite.hints.localRuntime' },
    { id: 'forward-nodex', to: '/admin/forward/nodex', icon: 'nodex', labelKey: 'forwardSuite.nav.nodeXRuntime', hintKey: 'forwardSuite.hints.nodeXRuntime' },
    { id: 'forward-agents', to: '/admin/forward/agents', icon: 'agents', labelKey: 'forwardSuite.nav.nodeXAgents', hintKey: 'forwardSuite.hints.nodeXAgents' }
  ]
})

// Sections of settings-style pages: navigation/sections.js (kept apart so
// the page titles in the entry chunk do not pull in this file).
export { ADMIN_PAGE_SECTIONS }

export function isForwardSuitePath(path) {
  return path === '/admin/forward' || String(path || '').startsWith('/admin/forward/')
}

// 转发节点 (UI U7): the execution-plane pages (NodeX nodes, Ansible machines,
// the local and NodeX runtimes, and their detail pages) live under
// /admin/forward/ but show their own run-mode switch instead of the forward
// suite navigation, which belongs to the Flux control plane.
const FORWARD_NODE_PATHS = ['/admin/forward/nodes', '/admin/forward/ansible-machines', '/admin/forward/local', '/admin/forward/nodex']

export function isForwardNodesPath(path) {
  const value = String(path || '')
  return FORWARD_NODE_PATHS.some(base => value === base || value.startsWith(`${base}/`))
}

export function showsForwardSuiteNav(path) {
  return isForwardSuitePath(path) && !isForwardNodesPath(path)
}

// Quick actions in the command palette open an existing create flow. Each is
// shown only when its page is in the visible menu (`page` = menu item id).
export const ADMIN_QUICK_ACTIONS = Object.freeze([
  { id: 'add-node', page: 'nodes', icon: 'nodes', labelKey: 'shell.palette.actions.addNode', to: { path: '/admin/nodes', query: { create: '1' } } },
  { id: 'add-user', page: 'users', icon: 'users', labelKey: 'shell.palette.actions.addUser', to: { path: '/admin/users', query: { create: '1' } } },
  { id: 'forward-wizard', page: 'forward', icon: 'setup', labelKey: 'shell.palette.actions.forwardWizard', to: { path: '/admin/forward/setup' } }
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
  { id: 'plans', to: '/user/plans', icon: 'plans', labelKey: 'shell.user.items.plans', edition: 'commercial' },
  { id: 'orders', to: '/user/orders', icon: 'orders', labelKey: 'shell.user.items.orders', edition: 'commercial' },
  { id: 'account', to: USER_ACCOUNT_PATH, icon: 'account', labelKey: 'shell.user.items.account', tab: true }
])

function itemVisible(item, { editionAllows, hasPermission, routeExists }) {
  if (item.edition && !editionAllows(item.edition)) return false
  if (item.permission && !hasPermission(item.permission)) return false
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
  extensionMenus = [],
  extensionMenuAllowed = () => true,
  normalizeParent = value => value
}) {
  const context = { editionAllows, hasPermission, routeExists }
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

function forwardSuiteEntries(t) {
  return [...FORWARD_SUITE_LINKS.core, ...FORWARD_SUITE_LINKS.advanced].map(link => ({
    id: link.id,
    to: link.to,
    label: t(link.labelKey),
    icon: link.icon
  }))
}

/**
 * paletteEntries lists what the command palette can open: every visible
 * page (sidebar items, the forward suite pages when 转发 is visible, the
 * account page) and the quick actions whose page is visible.
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
  // Forward suite pages, placed where the sidebar puts them (网络 · 转发,
  // 网络 · 转发节点, ...); only when the sidebar shows that entry.
  for (const entry of forwardSuiteEntries(t)) {
    const owner = activeMenuItem(groups, entry.to)
    if (owner) add(entry, `${owner.group.label} · ${owner.item.label}`)
  }
  // Sections of settings-style pages, under their page.
  for (const group of groups) {
    for (const item of group.items) {
      for (const section of ADMIN_PAGE_SECTIONS[item.id] || []) {
        add({ id: section.id, to: section.to, label: t(section.labelKey), icon: item.icon }, `${group.label} · ${item.label}`)
      }
    }
  }
  add({ id: 'account', to: ADMIN_ACCOUNT_PATH, label: t('shell.admin.items.account'), icon: 'account' }, '')

  const visibleIds = new Set(groups.flatMap(group => group.items.map(item => item.id)))
  const actions = ADMIN_QUICK_ACTIONS
    .filter(action => visibleIds.has(action.page))
    .map(action => ({
      id: action.id,
      kind: 'action',
      label: t(action.labelKey),
      icon: action.icon,
      to: action.to,
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
