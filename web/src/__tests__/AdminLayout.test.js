import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import AdminLayout from '@/layouts/AdminLayout.vue'
import { setEdition } from '@/composables/useEdition'
import { usePalette } from '@/components/shell/usePalette'

const mockGetSystemInfo = vi.hoisted(() => vi.fn())
const mockGetUserList = vi.hoisted(() => vi.fn())
const mockAdminExtensionMenus = vi.hoisted(() => ({ value: [] }))
const mockAdminCapabilities = vi.hoisted(() => ({ value: [] }))

vi.mock('@/api/admin', () => ({
  getSystemInfo: (...args) => mockGetSystemInfo(...args),
  getUserList: (...args) => mockGetUserList(...args)
}))

vi.mock('@/extensions/runtime', () => ({
  adminExtensionMenus: mockAdminExtensionMenus,
  adminCapabilities: mockAdminCapabilities
}))

function stubViewport({ narrow = false } = {}) {
  const listeners = new Set()
  const list = {
    matches: narrow,
    addEventListener: vi.fn((event, listener) => listeners.add(listener)),
    removeEventListener: vi.fn((event, listener) => listeners.delete(listener)),
    set(next) {
      list.matches = next
      for (const listener of listeners) listener({ matches: next })
    }
  }
  vi.stubGlobal('matchMedia', vi.fn(() => list))
  return list
}

async function mountLayout(path = '/admin/dashboard', options = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:rest(.*)*', component: { template: '<div class="test-page">page</div>' } }]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(AdminLayout, {
    attachTo: document.body,
    global: { plugins: [router], stubs: { teleport: true } },
    ...options
  })
  await flushPromises()
  return { wrapper, router }
}

function sidebarPaths(wrapper) {
  return wrapper.findAll('#admin-sidebar a[data-nav-item]').map(link => link.attributes('href'))
}

describe('AdminLayout.vue', () => {
  let mounted = null

  beforeEach(() => {
    setActivePinia(createPinia())
    const userStore = useUserStore()
    userStore.getUserInfo = vi.fn()
    userStore.login('admin-token', { id: 1, email: 'admin@example.test', is_admin: true, permission_mode: 'legacy' })
    mockAdminExtensionMenus.value = []
    mockGetSystemInfo.mockReset()
    mockGetSystemInfo.mockResolvedValue({ code: 0, data: { version: '4.1.0', build_code: '202610020001', commit: 'abc123' } })
    mockGetUserList.mockReset()
    mockGetUserList.mockResolvedValue({ code: 0, data: { list: [], total: 0 } })
    stubViewport()
  })

  afterEach(() => {
    mounted?.unmount()
    mounted = null
    usePalette().closePalette()
  })

  it('has the header, navigation and main landmarks and no clock, subtitle, version line or locale switcher', async () => {
    const { wrapper } = await mountLayout()
    mounted = wrapper
    expect(wrapper.find('#admin-sidebar nav').attributes('aria-label')).toBe('Admin navigation')
    expect(wrapper.find('#admin-sidebar').attributes('role')).toBe('complementary')
    expect(wrapper.find('#admin-sidebar').attributes('aria-label')).toBe('Sidebar')
    expect(wrapper.find('header.admin-topbar').exists()).toBe(true)
    const main = wrapper.get('main#app-main-content')
    expect(main.attributes('tabindex')).toBe('-1')
    expect(main.find('.test-page').exists()).toBe(true)
    expect(wrapper.find('.current-time').exists()).toBe(false)
    expect(wrapper.find('.topbar-subtitle').exists()).toBe(false)
    expect(wrapper.find('.version-line').exists()).toBe(false)
    expect(wrapper.find('.locale-switcher').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('4.1.0')
  })

  it('renders the menu config groups and marks one selected item', async () => {
    const { wrapper } = await mountLayout('/admin/forward/tunnel')
    mounted = wrapper
    const groups = wrapper.findAll('[data-nav-group]').map(group => group.attributes('data-nav-group'))
    expect(groups).toEqual(['overview', 'users', 'network', 'extensions', 'system'])
    const current = wrapper.findAll('#admin-sidebar [aria-current="page"]')
    expect(current).toHaveLength(1)
    expect(current[0].attributes('data-nav-item')).toBe('forward')
    expect(current[0].classes()).toContain('is-active')
  })

  it('lists invite codes under 用户 and selects it on its page', async () => {
    for (const edition of ['community', 'commercial']) {
      setEdition(edition)
      const { wrapper } = await mountLayout('/admin/invite-codes')
      const link = wrapper.get('[data-nav-group="users"] a[data-nav-item="invite-codes"]')
      expect(link.attributes('href')).toBe('/admin/invite-codes')
      expect(link.attributes('aria-current')).toBe('page')
      expect(link.text()).toBe('Invite codes')
      expect(wrapper.findAll('.admin-crumbs li').map(item => item.text())).toEqual(['Users', 'Invite codes'])
      wrapper.unmount()
    }
  })

  it('shows group › item › page in the breadcrumb', async () => {
    const { wrapper } = await mountLayout('/admin/forward/tunnel')
    mounted = wrapper
    const crumbs = wrapper.findAll('.admin-crumbs li').map(item => item.text())
    expect(crumbs).toEqual(['Network', 'Forwarding', 'Tunnels'])
    expect(wrapper.get('.admin-crumbs a').attributes('href')).toBe('/admin/forward')
    expect(wrapper.get('.admin-crumbs [aria-current="page"]').text()).toBe('Tunnels')
  })

  it('shows a group named like the page only once in the breadcrumb', async () => {
    const { wrapper } = await mountLayout('/admin/users')
    mounted = wrapper
    const crumbs = wrapper.findAll('.admin-crumbs li').map(item => item.text())
    expect(crumbs).toEqual(['Users'])
    expect(wrapper.get('.admin-crumbs [aria-current="page"]').text()).toBe('Users')
  })

  it('shows group › page › section on a settings section', async () => {
    const { wrapper } = await mountLayout('/admin/system/backup')
    mounted = wrapper
    expect(wrapper.findAll('.admin-crumbs li').map(item => item.text())).toEqual(['System', 'Settings', 'Backups'])
    expect(wrapper.get('.admin-crumbs a').attributes('href')).toBe('/admin/system')
  })

  it('renders the forward suite navigation once, only on forward pages', async () => {
    const { wrapper, router } = await mountLayout('/admin/forward/tunnel')
    mounted = wrapper
    expect(wrapper.findAll('[data-forward-suite-nav]')).toHaveLength(1)
    expect(wrapper.find('#admin-sidebar [data-forward-suite-nav]').exists()).toBe(false)
    // 转发节点 pages (execution plane, UI U7) show their run-mode switch instead.
    for (const path of ['/admin/forward/nodes', '/admin/forward/nodes/5', '/admin/forward/ansible-machines/7', '/admin/forward/local', '/admin/forward/nodex']) {
      await router.push(path)
      await flushPromises()
      expect(wrapper.find('[data-forward-suite-nav]').exists()).toBe(false)
    }
    await router.push('/admin/users')
    await flushPromises()
    expect(wrapper.find('[data-forward-suite-nav]').exists()).toBe(false)
  })

  // F5b: with the forward package's v4 API the sidebar gets the new 转发
  // area, the flux-clone pages stay as 转发（旧版）, the package's own menu
  // entry is left out, and the v4 pages show no forward suite navigation.
  it('adds the v4 forwarding area when the forward package provides it', async () => {
    mockAdminCapabilities.value = ['forward.v4']
    mockAdminExtensionMenus.value = [{ pluginID: 'forward', id: 'forward.main', parent: 'operations', label: 'Forward', icon: 'network', to: '/admin/extensions/forward', permission: 'forward.view', order: 150 }]
    try {
      const { wrapper, router } = await mountLayout('/admin/forward/routes/01J')
      mounted = wrapper
      const paths = sidebarPaths(wrapper)
      expect(paths).toContain('/admin/forward/overview')
      expect(paths).toContain('/admin/forward')
      expect(paths).not.toContain('/admin/extensions/forward')
      expect(wrapper.get('#admin-sidebar a[href="/admin/forward"]').text()).toContain('Forwarding (legacy)')
      expect(wrapper.get('#admin-sidebar a[href="/admin/forward/overview"]').attributes('aria-current')).toBe('page')
      expect(wrapper.find('[data-forward-suite-nav]').exists()).toBe(false)
      await router.push('/admin/forward/inventory/forward-41')
      await flushPromises()
      expect(wrapper.get('#admin-sidebar a[href="/admin/forward/overview"]').attributes('aria-current')).toBe('page')
    } finally {
      mockAdminCapabilities.value = []
      mockAdminExtensionMenus.value = []
    }
  })

  it('keeps the flux-clone forwarding pages as 转发 without the v4 API', async () => {
    const { wrapper } = await mountLayout('/admin/dashboard')
    mounted = wrapper
    const paths = sidebarPaths(wrapper)
    expect(paths).not.toContain('/admin/forward/overview')
    expect(wrapper.get('#admin-sidebar a[href="/admin/forward"]').text()).not.toContain('legacy')
  })

  it('keeps content to the admin width unless the route is wide', async () => {
    const { wrapper } = await mountLayout('/admin/dashboard')
    mounted = wrapper
    expect(wrapper.get('.admin-content').classes()).not.toContain('is-wide')
  })

  it('collapses the sidebar to an icon rail and remembers it', async () => {
    const { wrapper } = await mountLayout()
    mounted = wrapper
    const toggle = wrapper.get('[data-sidebar-toggle]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(toggle.attributes('aria-label')).toBe('Collapse sidebar')
    localStorage.setItem.mockClear()
    await toggle.trigger('click')
    expect(localStorage.setItem).toHaveBeenCalledWith('admin.sidebar.collapsed', 'true')
    expect(wrapper.get('.admin-shell').classes()).toContain('is-rail')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    // Labels stay in the accessibility tree and become tooltips.
    const link = wrapper.get('a[data-nav-item="dashboard"]')
    expect(link.attributes('title')).toBe('Dashboard')
    expect(link.get('.admin-nav__label').classes()).toContain('visually-hidden')
  })

  it('starts as a rail when that was the last choice', async () => {
    localStorage.setItem('admin.sidebar.collapsed', 'true')
    const { wrapper } = await mountLayout()
    mounted = wrapper
    expect(wrapper.get('.admin-shell').classes()).toContain('is-rail')
  })

  it('moves between sidebar links with the arrow keys', async () => {
    const { wrapper } = await mountLayout()
    mounted = wrapper
    const links = wrapper.findAll('#admin-sidebar a[data-nav-item]')
    links[0].element.focus()
    await links[0].trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(links[1].element)
    await links[1].trigger('keydown', { key: 'End' })
    expect(document.activeElement).toBe(links.at(-1).element)
    await links.at(-1).trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(links[0].element)
  })

  describe('below 834 px', () => {
    beforeEach(() => {
      stubViewport({ narrow: true })
    })

    it('makes the closed drawer inert and opens it with focus on its close button', async () => {
      const { wrapper } = await mountLayout()
      mounted = wrapper
      const sidebar = wrapper.get('#admin-sidebar')
      const toggle = wrapper.get('[data-sidebar-toggle]')
      expect(wrapper.get('.admin-shell').classes()).toContain('is-drawer')
      expect(sidebar.attributes('inert')).toBeDefined()
      expect(sidebar.attributes('aria-hidden')).toBe('true')
      expect(toggle.attributes('aria-controls')).toBe('admin-sidebar')
      expect(toggle.attributes('aria-expanded')).toBe('false')

      await toggle.trigger('click')
      await nextTick()
      expect(sidebar.attributes('inert')).toBeUndefined()
      expect(toggle.attributes('aria-expanded')).toBe('true')
      expect(sidebar.attributes('role')).toBe('dialog')
      expect(sidebar.attributes('aria-modal')).toBe('true')
      expect(document.activeElement).toBe(wrapper.get('.admin-sidebar__close').element)
    })

    it('closes on Escape and returns focus to the toggle', async () => {
      const { wrapper } = await mountLayout()
      mounted = wrapper
      const toggle = wrapper.get('[data-sidebar-toggle]')
      await toggle.trigger('click')
      await nextTick()
      await wrapper.get('.admin-sidebar__close').trigger('keydown', { key: 'Escape' })
      await nextTick()
      expect(toggle.attributes('aria-expanded')).toBe('false')
      expect(wrapper.get('#admin-sidebar').attributes('inert')).toBeDefined()
      expect(document.activeElement).toBe(toggle.element)
    })

    it('keeps Tab and Shift+Tab inside the open drawer', async () => {
      const { wrapper } = await mountLayout()
      mounted = wrapper
      await wrapper.get('[data-sidebar-toggle]').trigger('click')
      await nextTick()
      const brand = wrapper.get('.admin-sidebar__brand')
      const account = wrapper.get('#admin-sidebar [data-account-menu-trigger]')
      account.element.focus()
      await account.trigger('keydown', { key: 'Tab' })
      expect(document.activeElement).toBe(brand.element)
      await brand.trigger('keydown', { key: 'Tab', shiftKey: true })
      expect(document.activeElement).toBe(account.element)
    })

    it('closes after a navigation item is chosen', async () => {
      const { wrapper } = await mountLayout()
      mounted = wrapper
      const toggle = wrapper.get('[data-sidebar-toggle]')
      await toggle.trigger('click')
      await wrapper.get('a[data-nav-item="users"]').trigger('click')
      await flushPromises()
      expect(toggle.attributes('aria-expanded')).toBe('false')
    })

    it('leaves drawer mode when the window grows', async () => {
      const list = stubViewport({ narrow: true })
      const { wrapper } = await mountLayout()
      mounted = wrapper
      await wrapper.get('[data-sidebar-toggle]').trigger('click')
      list.set(false)
      await nextTick()
      expect(wrapper.get('.admin-shell').classes()).not.toContain('is-drawer')
      expect(wrapper.get('#admin-sidebar').classes()).not.toContain('is-open')
    })

    it('puts the account avatar in the top bar', async () => {
      const { wrapper } = await mountLayout()
      mounted = wrapper
      expect(wrapper.find('.admin-topbar [data-account-menu-trigger]').exists()).toBe(true)
    })
  })

  it('has no top-bar avatar when the sidebar footer shows the account', async () => {
    const { wrapper } = await mountLayout()
    mounted = wrapper
    expect(wrapper.find('.admin-topbar [data-account-menu-trigger]').exists()).toBe(false)
    expect(wrapper.find('#admin-sidebar [data-account-menu-trigger]').exists()).toBe(true)
  })

  it('shows the commercial group only in the commercial edition', async () => {
    setEdition('commercial')
    let { wrapper } = await mountLayout()
    expect(wrapper.find('[data-nav-group="commerce"]').exists()).toBe(true)
    expect(sidebarPaths(wrapper)).toEqual(expect.arrayContaining(['/admin/orders', '/admin/coupons', '/admin/payment', '/admin/invite']))
    wrapper.unmount()

    setEdition('community')
    ;({ wrapper } = await mountLayout())
    mounted = wrapper
    expect(wrapper.find('[data-nav-group="commerce"]').exists()).toBe(false)
    expect(sidebarPaths(wrapper)).not.toContain('/admin/orders')
    expect(wrapper.get('a[data-nav-item="templates"]').text()).toBe('Subscription templates')
  })

  it('adds plugin menus the admin may open and titles their pages', async () => {
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 1, is_admin: true, permissions: ['example.view'] })
    mockAdminExtensionMenus.value = [
      { pluginID: 'example', id: 'example.main', parent: 'services', label: 'Example Service', icon: 'EX', to: '/admin/extensions/example', permission: 'example.view', order: 100 },
      { pluginID: 'hidden', id: 'hidden.main', parent: 'services', label: 'Hidden', icon: 'EX', to: '/admin/extensions/hidden', permission: 'hidden.view', order: 100 }
    ]
    const { wrapper } = await mountLayout('/admin/extensions/example')
    mounted = wrapper
    const extensions = wrapper.get('[data-nav-group="extensions"]')
    expect(extensions.find('a[data-nav-source="extension"]').attributes('href')).toBe('/admin/extensions/example')
    expect(extensions.find('[data-admin-nav-icon="EX"]').exists()).toBe(true)
    expect(sidebarPaths(wrapper)).not.toContain('/admin/extensions/hidden')
    expect(wrapper.get('.admin-crumbs [aria-current="page"]').text()).toBe('Example Service')
  })

  it('titles pages from the page meta table, the route meta and the menu', async () => {
    const { wrapper, router } = await mountLayout('/admin/notifications/telegram')
    mounted = wrapper
    const title = () => wrapper.get('.admin-crumbs [aria-current="page"]').text()
    expect(title()).toBe('Telegram')
    await router.push('/admin/forward/ansible-machines')
    await flushPromises()
    expect(title()).toBe('Ansible Machines')
    await router.push('/admin/forward/nodex')
    await flushPromises()
    expect(title()).toBe('NodeX Runtime')
    await router.push('/admin/account')
    await flushPromises()
    expect(title()).toBe('Account')
  })

  it('opens the command palette with the search button and with ⌘K / Ctrl+K', async () => {
    const { wrapper } = await mountLayout()
    mounted = wrapper
    const { open } = usePalette()
    await wrapper.get('[data-palette-trigger]').trigger('click')
    expect(open.value).toBe(true)
    open.value = false
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }))
    expect(open.value).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'K', ctrlKey: true }))
    expect(open.value).toBe(false)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, shiftKey: true }))
    expect(open.value).toBe(false)
    expect(wrapper.get('[data-palette-trigger]').attributes('aria-keyshortcuts')).toBe('Meta+K Control+K')
  })
})
