import { describe, expect, it } from 'vitest'
import {
  ADMIN_MENU,
  ADMIN_PAGE_SECTIONS,
  FORWARD_SUITE_LINKS,
  activeMenuItem,
  buildAdminMenu,
  buildUserMenu,
  isForwardSuitePath,
  matchesQuery,
  paletteEntries
} from '@/navigation/menu'
import { normalizeWebUIMenuParent } from '@/extensions/menuRegistry'
import router from '@/router'

const t = key => key
const community = edition => edition === 'community'
const commercial = edition => edition === 'commercial'

function ids(groups) {
  return groups.flatMap(group => group.items.map(item => item.id))
}

function groupOf(groups, itemId) {
  return groups.find(group => group.items.some(item => item.id === itemId))?.id
}

const extensionMenus = [
  { pluginID: 'service-a', id: 'service-a.main', parent: 'services', label: 'Service A', icon: 'SV', to: '/admin/extensions/service-a', permission: 'service-a.view', order: 20 },
  { pluginID: 'service-b', id: 'service-b.main', parent: 'services', label: 'Service B', icon: 'EX', to: '/admin/extensions/service-b', permission: 'service-b.view', order: 10 },
  { pluginID: 'ops', id: 'ops.main', parent: 'operations', label: 'Ops', icon: 'GA', to: '/admin/extensions/ops', permission: 'ops.view', order: 30 },
  { pluginID: 'sys', id: 'sys.main', parent: 'system', label: 'Sys', icon: 'ST', to: '/admin/extensions/sys', permission: 'sys.view', order: 40 },
  { pluginID: 'legacy', id: 'legacy.main', parent: 'legacy-parent', label: 'Legacy', icon: 'zz', to: '/admin/extensions/legacy', permission: 'legacy.view', order: 50 },
  { pluginID: 'payment', id: 'payment.main', parent: 'services', label: 'Payment', icon: 'PL', to: '/admin/extensions/payment', permission: 'payment.view', order: 5 }
]

describe('navigation/menu.js: buildAdminMenu', () => {
  it('orders the groups as plan §4.2 and keeps Node and ForwardNode apart', () => {
    const groups = buildAdminMenu({ t, editionAllows: commercial })
    expect(groups.map(group => group.id)).toEqual(['overview', 'users', 'network', 'extensions', 'system', 'commerce'])
    const network = groups.find(group => group.id === 'network').items
    expect(network.map(item => [item.id, item.to])).toEqual([
      ['nodes', '/admin/nodes'],
      ['forward', '/admin/forward'],
      ['forward-nodes', '/admin/forward/nodes'],
      ['agents', '/admin/agent']
    ])
    expect(network.find(item => item.id === 'forward-nodes').label).toBe('shell.admin.items.forwardNodes')
  })

  it('hides 商业 and shows subscription templates in the community edition', () => {
    const groups = buildAdminMenu({ t, editionAllows: community })
    expect(groups.map(group => group.id)).not.toContain('commerce')
    expect(ids(groups)).not.toEqual(expect.arrayContaining(['orders']))
    for (const id of ['plans', 'orders', 'coupons', 'payment', 'invite']) {
      expect(ids(groups)).not.toContain(id)
    }
    expect(groupOf(groups, 'templates')).toBe('users')
    expect(groups.flatMap(group => group.items).find(item => item.id === 'templates').to).toBe('/admin/plans')
  })

  it('moves plans into 商业 in the commercial edition', () => {
    const groups = buildAdminMenu({ t, editionAllows: commercial })
    expect(ids(groups)).not.toContain('templates')
    expect(groupOf(groups, 'plans')).toBe('commerce')
    expect(groups.find(group => group.id === 'commerce').items.map(item => item.to)).toEqual([
      '/admin/plans', '/admin/orders', '/admin/coupons', '/admin/payment', '/admin/invite'
    ])
  })

  it('lists one sidebar entry per destination, every one a registered route', () => {
    const groups = buildAdminMenu({ t, editionAllows: commercial, routeExists: () => false })
    const paths = groups.flatMap(group => group.items.map(item => item.to))
    expect(new Set(paths).size).toBe(paths.length)
    // Resolved by a real route, not the catch-all (pages with sections use
    // a parameter: /admin/system/:section?).
    for (const path of paths) {
      const matched = router.resolve(path).matched
      expect(matched.length > 0 && !matched.at(-1).path.includes(':pathMatch'), path).toBe(true)
    }
    // The legacy redirect and the duplicated forward-suite links are not in the sidebar.
    expect(paths).not.toContain('/admin/control')
    expect(paths).not.toContain('/admin/forward/tunnel')
    expect(paths).not.toContain('/admin/forward/agents')
  })

  it('lists invite codes under 用户 in both editions', () => {
    for (const editionAllows of [community, commercial]) {
      const groups = buildAdminMenu({ t, editionAllows })
      expect(groupOf(groups, 'invite-codes')).toBe('users')
      expect(groups.flatMap(group => group.items).find(item => item.id === 'invite-codes').to).toBe('/admin/invite-codes')
    }
  })

  it('shows optional items only when their route exists', () => {
    const menu = [{ id: 'a', labelKey: 'a', items: [{ id: 'later', to: '/admin/later', labelKey: 'later', optional: true }] }]
    expect(buildAdminMenu({ t, menu, routeExists: () => false })).toEqual([])
    expect(ids(buildAdminMenu({ t, menu, routeExists: path => path === '/admin/later' }))).toEqual(['later'])
  })

  it('merges plugin menus by parent, sorted, after the built-in items', () => {
    const groups = buildAdminMenu({
      t,
      extensionMenus,
      normalizeParent: normalizeWebUIMenuParent
    })
    const extensions = groups.find(group => group.id === 'extensions').items
    expect(extensions.map(item => item.id)).toEqual([
      'plugins', 'deployments', 'payment.main', 'service-b.main', 'service-a.main', 'ops.main', 'legacy.main'
    ])
    expect(extensions.find(item => item.id === 'legacy.main').parent).toBe('extensions')
    const system = groups.find(group => group.id === 'system').items
    expect(system.at(-1)).toMatchObject({ id: 'sys.main', source: 'extension', to: '/admin/extensions/sys' })
  })

  it('drops plugin menus without the permission or from a package the edition hides', () => {
    const granted = new Set(['service-a.view', 'payment.view'])
    const groups = buildAdminMenu({
      t,
      extensionMenus,
      hasPermission: permission => !permission || granted.has(permission),
      extensionMenuAllowed: menu => menu.pluginID !== 'payment',
      normalizeParent: normalizeWebUIMenuParent
    })
    const all = ids(groups)
    expect(all).toContain('service-a.main')
    expect(all).not.toContain('service-b.main')
    expect(all).not.toContain('payment.main')
    expect(all).not.toContain('sys.main')
  })

  it('drops built-in items that need a permission the admin lacks, and empty groups', () => {
    const menu = [
      { id: 'a', labelKey: 'a', items: [{ id: 'open', to: '/admin/open', labelKey: 'open' }, { id: 'locked', to: '/admin/locked', labelKey: 'locked', permission: 'x.view' }] },
      { id: 'b', labelKey: 'b', items: [{ id: 'only-locked', to: '/admin/b', labelKey: 'b', permission: 'y.view' }] }
    ]
    const groups = buildAdminMenu({ t, menu, hasPermission: permission => !permission || permission === 'x.view' })
    expect(groups.map(group => group.id)).toEqual(['a'])
    expect(ids(groups)).toEqual(['open', 'locked'])
    // Built-in items carry no permission of their own today (admin routes are
    // guarded as a whole); plugin menus always do.
    expect(ADMIN_MENU.flatMap(group => group.items).every(item => !item.permission)).toBe(true)
  })
})

describe('navigation/menu.js: active item', () => {
  const groups = buildAdminMenu({ t, editionAllows: commercial })

  it.each([
    ['/admin/dashboard', 'dashboard'],
    ['/admin/forward', 'forward'],
    ['/admin/forward/setup', 'forward'],
    ['/admin/forward/tunnel', 'forward'],
    ['/admin/forward/limit', 'forward'],
    ['/admin/forward/observability', 'forward'],
    ['/admin/forward/nodes', 'forward-nodes'],
    ['/admin/forward/local', 'forward-nodes'],
    ['/admin/forward/nodex', 'forward-nodes'],
    ['/admin/forward/ansible-machines', 'forward-nodes'],
    ['/admin/forward/agents', 'agents'],
    ['/admin/agent', 'agents'],
    ['/admin/nodes', 'nodes'],
    ['/admin/plans', 'plans']
  ])('selects exactly one item for %s', (path, expected) => {
    expect(activeMenuItem(groups, path)?.item.id).toBe(expected)
  })

  it('selects nothing for pages outside the menu', () => {
    expect(activeMenuItem(groups, '/admin/account')).toBeNull()
    expect(activeMenuItem(groups, '/admin/nowhere')).toBeNull()
  })

  it('knows the forward suite paths', () => {
    expect(isForwardSuitePath('/admin/forward')).toBe(true)
    expect(isForwardSuitePath('/admin/forward/local')).toBe(true)
    expect(isForwardSuitePath('/admin/forwarding')).toBe(false)
    expect(isForwardSuitePath('/admin/nodes')).toBe(false)
  })
})

describe('navigation/menu.js: palette entries', () => {
  it('offers every visible page, the forward suite pages and the account, without duplicates', () => {
    const groups = buildAdminMenu({ t, editionAllows: community })
    const { pages, actions } = paletteEntries({ t, groups })
    const paths = pages.map(page => page.to)
    expect(new Set(paths).size).toBe(paths.length)
    for (const link of [...FORWARD_SUITE_LINKS.core, ...FORWARD_SUITE_LINKS.advanced]) {
      expect(paths).toContain(link.to)
    }
    expect(paths).toContain('/admin/account')
    expect(paths).not.toContain('/admin/orders')
    // Forward suite pages carry the sidebar entry that owns them.
    expect(pages.find(page => page.to === '/admin/forward/local').context).toBe('shell.admin.groups.network · shell.admin.items.forwardNodes')
    expect(pages.find(page => page.to === '/admin/forward/tunnel').context).toBe('shell.admin.groups.network · shell.admin.items.forward')
    expect(actions.map(action => action.id)).toEqual(['add-node', 'add-user', 'forward-wizard'])
    expect(actions.find(action => action.id === 'add-node').to).toEqual({ path: '/admin/nodes', query: { create: '1' } })
  })

  it('offers a quick action only when its page is visible', () => {
    const groups = buildAdminMenu({ t }).map(group => ({ ...group, items: group.items.filter(item => item.id !== 'nodes') }))
    const { actions } = paletteEntries({ t, groups })
    expect(actions.map(action => action.id)).not.toContain('add-node')
  })

  it('lists the sections of settings-style pages under their page (UI U7)', () => {
    const groups = buildAdminMenu({ t, editionAllows: community })
    const { pages } = paletteEntries({ t, groups })
    const ids = pages.map(page => page.id)
    expect(ids).not.toContain('mfa')
    expect(ids).not.toContain('telegram')
    for (const section of [...ADMIN_PAGE_SECTIONS.settings, ...ADMIN_PAGE_SECTIONS.security, ...ADMIN_PAGE_SECTIONS.notifications]) {
      expect(ids).toContain(section.id)
    }
    const accessGroups = pages.find(page => page.id === 'security-access-groups')
    expect(accessGroups).toMatchObject({ to: '/admin/security/access-groups', label: 'adminSecurity.sections.accessGroups', context: 'shell.admin.groups.system · shell.admin.items.security' })
    expect(matchesQuery(accessGroups, 'access groups')).toBe(true)
    // The old paths still select their new menu item.
    expect(activeMenuItem(groups, '/admin/mfa').item.id).toBe('security')
    expect(activeMenuItem(groups, '/admin/telegram').item.id).toBe('notifications')
    expect(activeMenuItem(groups, '/admin/system/backup').item.id).toBe('settings')
  })

  it('matches every query word against label, context and route words', () => {
    const entry = { label: '隧道管理', context: '网络 · 转发', keywords: 'forward tunnel' }
    expect(matchesQuery(entry, '')).toBe(true)
    expect(matchesQuery(entry, '隧道')).toBe(true)
    expect(matchesQuery(entry, 'TUNNEL')).toBe(true)
    expect(matchesQuery(entry, 'forward 隧道')).toBe(true)
    expect(matchesQuery(entry, 'nodes')).toBe(false)
  })
})

describe('navigation/menu.js: buildUserMenu', () => {
  it('has five tab bar items and hides commercial pages in the community edition', () => {
    const items = buildUserMenu({ t, editionAllows: community })
    expect(items.map(item => item.id)).toEqual(['dashboard', 'subscribe', 'knowledge', 'tickets', 'account'])
    expect(items.filter(item => item.tab)).toHaveLength(5)
    expect(items.find(item => item.id === 'knowledge').shortLabel).toBe('shell.user.items.knowledgeShort')
  })

  it('adds plans and orders (not tabs) in the commercial edition', () => {
    const items = buildUserMenu({ t, editionAllows: commercial })
    expect(items.map(item => item.id)).toEqual(['dashboard', 'subscribe', 'knowledge', 'tickets', 'plans', 'orders', 'account'])
    expect(items.filter(item => !item.tab).map(item => item.to)).toEqual(['/user/plans', '/user/orders'])
  })
})
