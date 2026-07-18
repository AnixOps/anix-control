import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import AdminLayout from '@/layouts/AdminLayout.vue'

const mockPush = vi.fn()
const mockRoute = reactive({ path: '/admin/dashboard' })
const mockGetSystemInfo = vi.hoisted(() => vi.fn())
const mockAdminExtensionMenus = vi.hoisted(() => ({ value: [] }))

function stubTabletViewport(matches = true) {
  const listeners = new Map()
  const mediaQuery = {
    matches,
    addEventListener: vi.fn((event, listener) => listeners.set(event, listener)),
    removeEventListener: vi.fn((event) => listeners.delete(event)),
    setMatches(nextMatches) {
      mediaQuery.matches = nextMatches
      listeners.get('change')?.({ matches: nextMatches })
    }
  }
  vi.stubGlobal('matchMedia', vi.fn(() => mediaQuery))
  return mediaQuery
}

vi.mock('vue-router', () => ({
  routeLocationKey: Symbol('route location'),
  useRouter: () => ({ push: mockPush }),
  useRoute: () => mockRoute,
}))

vi.mock('@/api/admin', () => ({
  getSystemInfo: (...args) => mockGetSystemInfo(...args),
}))

vi.mock('@/extensions/runtime', () => ({
  adminExtensionMenus: mockAdminExtensionMenus,
}))

const adminMenuPaths = [
  '/admin/dashboard',
  '/admin/monitor',
  '/admin/traffic-hourly',
  '/admin/users',
  '/admin/orders',
  '/admin/tickets',
  '/admin/nodes',
  '/admin/subscriptions',
  '/admin/forward/setup',
  '/admin/forward',
  '/admin/forward/tunnel',
  '/admin/forward/limit',
  '/admin/forward/ansible-machines',
  '/admin/forward/nodes',
  '/admin/forward/local',
  '/admin/forward/nodex',
  '/admin/forward/agents',
  '/admin/forward/observability',
  '/admin/agent',
  '/admin/plugins',
  '/admin/deployments',
  '/admin/plans',
  '/admin/coupons',
  '/admin/invite',
  '/admin/payment',
  '/admin/telegram',
  '/admin/notifications',
  '/admin/knowledge',
  '/admin/mfa',
  '/admin/system',
  '/admin/control',
  '/admin/access-groups',
]

describe('AdminLayout.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useUserStore().getUserInfo = vi.fn()
    mockRoute.path = '/admin/dashboard'
    mockAdminExtensionMenus.value = []
    mockPush.mockReset()
    mockGetSystemInfo.mockReset()
    mockGetSystemInfo.mockResolvedValue({ data: { version: '2.0.0' } })
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('renders router view container', () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    expect(wrapper.find('router-view-stub').exists()).toBe(true)
  })

  it('shows system version from legacy and panel envelope payloads', async () => {
    mockGetSystemInfo.mockResolvedValueOnce({
      data: {
        version: '2.1.0',
        build_code: '202607090001',
        build_time: '2026-07-09T00:00:00Z',
        commit: 'abc123'
      }
    })

    const legacyWrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })
    await flushPromises()

    expect(legacyWrapper.find('.version-line').text()).toBe('AnixOps v2.1.0 #202607090001')
    expect(legacyWrapper.find('.version-line').attributes('title')).toContain('Build code: 202607090001')
    legacyWrapper.unmount()

    mockGetSystemInfo.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        version: '2.2.0',
        build_code: '202607090002',
        build_time: '2026-07-09T00:01:00Z',
        commit: 'def456'
      },
      ts: 1783526400000
    })

    const envelopeWrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })
    await flushPromises()

    expect(envelopeWrapper.find('.version-line').text()).toBe('AnixOps v2.2.0 #202607090002')
    expect(envelopeWrapper.find('.version-line').attributes('title')).toContain('Commit: def456')
    envelopeWrapper.unmount()
  })

  it('exposes accessible navigation controls and main landmark', () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    expect(wrapper.find('.menu-button').attributes('aria-controls')).toBe('admin-sidebar')
    expect(wrapper.find('.menu-button').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('.sidebar').attributes('id')).toBe('admin-sidebar')
    expect(wrapper.find('main.content').attributes('id')).toBe('app-main-content')
    expect(wrapper.find('main.content').attributes('tabindex')).toBe('-1')
  })

  it('groups the compact navigation and persists the desktop collapse preference', async () => {
    stubTabletViewport(false)
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            emits: ['click'],
            template: '<a class="menu-link" :data-to="to" @click="$emit(\'click\', $event)"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    expect(wrapper.find('[data-nav-group="overview"]').text()).toContain('Dashboard')
    expect(wrapper.find('[data-nav-group="control-center"] a[data-to="/admin/plugins"]').exists()).toBe(true)
    expect(wrapper.find('[data-nav-group="control-center"] a[data-to="/admin/deployments"]').exists()).toBe(true)

    localStorage.setItem.mockClear()
    await wrapper.get('[aria-label="Collapse navigation"]').trigger('click')

    expect(localStorage.setItem).toHaveBeenCalledWith('admin.sidebar.collapsed', 'true')
    expect(wrapper.find('.admin-layout').classes()).toContain('navigation-collapsed')
  })

  it('closes the mobile drawer after a navigation item is activated', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            emits: ['click'],
            template: '<a class="menu-link" :data-to="to" @click="$emit(\'click\', $event)"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    await wrapper.get('.menu-button').trigger('click')
    expect(wrapper.get('.menu-button').attributes('aria-expanded')).toBe('true')

    await wrapper.get('a[data-to="/admin/dashboard"]').trigger('click')
    await nextTick()

    expect(wrapper.get('.menu-button').attributes('aria-expanded')).toBe('false')
  })

  it('makes a closed mobile drawer inert and restores focus to its trigger', async () => {
    const mediaQuery = stubTabletViewport()
    const wrapper = mount(AdminLayout, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            emits: ['click'],
            template: '<a class="menu-link" :data-to="to" @click="$emit(\'click\', $event)"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    const sidebar = wrapper.get('#admin-sidebar')
    const menuButton = wrapper.get('.menu-button')

    expect(mediaQuery.addEventListener).toHaveBeenCalledWith('change', expect.any(Function))
    expect(sidebar.attributes('inert')).toBeDefined()
    expect(sidebar.attributes('aria-hidden')).toBe('true')

    await menuButton.trigger('click')
    await nextTick()

    expect(sidebar.attributes('inert')).toBeUndefined()
    expect(sidebar.attributes('aria-hidden')).toBeUndefined()
    expect(document.activeElement).toBe(wrapper.get('.close-button').element)

    await wrapper.get('a[data-to="/admin/dashboard"]').trigger('click')
    await nextTick()

    expect(sidebar.attributes('inert')).toBeDefined()
    expect(sidebar.attributes('aria-hidden')).toBe('true')
    expect(document.activeElement).toBe(menuButton.element)
    wrapper.unmount()
  })

  it('resets an open drawer when the viewport leaves the tablet breakpoint', async () => {
    const mediaQuery = stubTabletViewport()
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            emits: ['click'],
            template: '<a class="menu-link" :data-to="to" @click="$emit(\'click\', $event)"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    await wrapper.get('.menu-button').trigger('click')
    expect(wrapper.find('.sidebar-overlay').classes()).toContain('active')
    expect(wrapper.get('.menu-button').attributes('aria-expanded')).toBe('true')

    mediaQuery.setMatches(false)
    await nextTick()

    expect(wrapper.find('.sidebar-overlay').classes()).not.toContain('active')
    expect(wrapper.get('.menu-button').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('#admin-sidebar').classes()).not.toContain('open')
  })

  it('wraps Tab from the final mobile drawer control to its close button', async () => {
    stubTabletViewport()
    const wrapper = mount(AdminLayout, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            emits: ['click'],
            template: '<a class="menu-link" href="#" :data-to="to" @click="$emit(\'click\', $event)"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    await wrapper.get('.menu-button').trigger('click')
    await nextTick()

    const closeButton = wrapper.get('.close-button')
    const logoutButton = wrapper.get('.logout-button')
    logoutButton.element.focus()
    await logoutButton.trigger('keydown', { key: 'Tab' })

    expect(document.activeElement).toBe(closeButton.element)
    wrapper.unmount()
  })

  it('wraps Shift+Tab from the first mobile drawer control to its final control', async () => {
    stubTabletViewport()
    const wrapper = mount(AdminLayout, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            emits: ['click'],
            template: '<a class="menu-link" href="#" :data-to="to" @click="$emit(\'click\', $event)"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    await wrapper.get('.menu-button').trigger('click')
    await nextTick()

    const closeButton = wrapper.get('.close-button')
    const logoutButton = wrapper.get('.logout-button')
    closeButton.element.focus()
    await closeButton.trigger('keydown', { key: 'Tab', shiftKey: true })

    expect(document.activeElement).toBe(logoutButton.element)
    wrapper.unmount()
  })

  it('closes the mobile drawer on Escape and cleans up focus containment', async () => {
    stubTabletViewport()
    const wrapper = mount(AdminLayout, {
      attachTo: document.body,
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            emits: ['click'],
            template: '<a class="menu-link" href="#" :data-to="to" @click="$emit(\'click\', $event)"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    const menuButton = wrapper.get('.menu-button')
    await menuButton.trigger('click')
    await nextTick()

    await wrapper.get('.close-button').trigger('keydown', { key: 'Escape' })
    await nextTick()

    expect(menuButton.attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('#admin-sidebar').attributes('inert')).toBeDefined()
    expect(document.activeElement).toBe(menuButton.element)
    wrapper.unmount()
  })

  it('contains all admin menu routes in sidebar', () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            template: '<a class="menu-link" :data-to="to"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    const links = wrapper.findAll('a.menu-link').map(link => link.attributes('data-to'))

    expect(links).toEqual(expect.arrayContaining(adminMenuPaths))
  })

  it('projects enabled extension menus and titles without changing core navigation', () => {
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 1, is_admin: true, permissions: ['example.view'] })
    mockAdminExtensionMenus.value = [{
      pluginID: 'example',
      id: 'example.main',
      parent: 'services',
      label: 'Example Service',
      icon: 'EX',
      to: '/admin/extensions/example',
      permission: 'example.view',
      order: 100,
    }]
    mockRoute.path = '/admin/extensions/example'

    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            template: '<a class="menu-link" :data-to="to"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    const links = wrapper.findAll('a.menu-link').map(link => link.attributes('data-to'))
    expect(links).toContain('/admin/extensions/example')
    expect(links).toContain('/admin/dashboard')
    expect(wrapper.find('.topbar-title').text()).toContain('Example Service')
    const controlCenter = wrapper.get('[data-nav-group="control-center"]')
    expect(controlCenter.text()).toContain('Extensions / Services')
    expect(controlCenter.find('[data-admin-nav-icon="box"]').exists()).toBe(true)
  })

  it('groups extension menus by registered parent, sorts each group, and isolates unknown parents', () => {
    const userStore = useUserStore()
    userStore.login('admin-token', {
      id: 1,
      is_admin: true,
      permissions: ['service-a.view', 'service-b.view', 'operation.view', 'system.view', 'legacy.view']
    })
    mockAdminExtensionMenus.value = [
      { pluginID: 'service-a', id: 'service-a.main', parent: 'services', label: 'Service A', icon: 'A', to: '/admin/extensions/service-a', permission: 'service-a.view', order: 20 },
      { pluginID: 'service-b', id: 'service-b.main', parent: 'services', label: 'Service B', icon: 'B', to: '/admin/extensions/service-b', permission: 'service-b.view', order: 10 },
      { pluginID: 'operation', id: 'operation.main', parent: 'operations', label: 'Operation', icon: 'OP', to: '/admin/extensions/operation', permission: 'operation.view', order: 30 },
      { pluginID: 'system', id: 'system.main', parent: 'system', label: 'System Extension', icon: 'SY', to: '/admin/extensions/system', permission: 'system.view', order: 40 },
      { pluginID: 'legacy', id: 'legacy.main', parent: 'legacy-parent', label: 'Legacy Extension', icon: 'LE', to: '/admin/extensions/legacy', permission: 'legacy.view', order: 50 },
    ]

    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            template: '<a class="menu-link" :data-to="to"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    const controlCenter = wrapper.get('[data-nav-group="control-center"]')
    const extensionSections = controlCenter.findAll('section[data-extension-parent]')
    expect(extensionSections.map(section => section.attributes('data-extension-parent'))).toEqual([
      'services', 'operations', 'system', 'extensions'
    ])
    expect(extensionSections[0].findAll('a.menu-link').map(link => link.attributes('data-to'))).toEqual([
      '/admin/extensions/service-b', '/admin/extensions/service-a'
    ])
    expect(extensionSections[3].find('a.menu-link').attributes('data-to')).toBe('/admin/extensions/legacy')
  })

  it('hides extension menus when the admin lacks the plugin permission', () => {
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 1, is_admin: true, permissions: ['other.view'] })
    mockAdminExtensionMenus.value = [{
      pluginID: 'example',
      id: 'example.main',
      parent: 'services',
      label: 'Example Service',
      icon: 'EX',
      to: '/admin/extensions/example',
      permission: 'example.view',
      order: 100,
    }]
    mockRoute.path = '/admin/extensions/example'

    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': {
            props: ['to'],
            template: '<a class="menu-link" :data-to="to"><slot /></a>',
          },
          'router-view': true,
        },
      },
    })

    const links = wrapper.findAll('a.menu-link').map(link => link.attributes('data-to'))
    expect(links).not.toContain('/admin/extensions/example')
    expect(wrapper.find('[data-nav-group="control-center"] [data-extension-parent]').exists()).toBe(false)
    expect(wrapper.find('.topbar-title').text()).not.toContain('Example Service')
  })

  it('updates page title on route changes for key admin pages', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    mockRoute.path = '/admin/telegram'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('Telegram')

    mockRoute.path = '/admin/monitor'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('Monitor')

    mockRoute.path = '/admin/traffic-hourly'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('Hourly Traffic')

    mockRoute.path = '/admin/mfa'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('MFA')

    mockRoute.path = '/admin/forward/agents'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('NodeX Agents')

    mockRoute.path = '/admin/forward/nodes'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('NodeX Topology')

    mockRoute.path = '/admin/control'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('Control Kernel')
  })

  it('shows NodeX title for the dedicated runtime route', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    mockRoute.path = '/admin/forward/nodex'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('NodeX')
  })

  it('shows Local Runtime title for the dedicated local route', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    mockRoute.path = '/admin/forward/local'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('Local Runtime')
  })

  it('shows Ansible Machines title for the dedicated route', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    mockRoute.path = '/admin/forward/ansible-machines'
    await nextTick()
    expect(wrapper.find('.topbar-title').text()).toContain('Ansible Machines')
  })

  it('logs out and redirects to login', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    const userStore = useUserStore()
    userStore.logout = vi.fn()

    await wrapper.find('.sidebar-actions .btn').trigger('click')

    expect(userStore.logout).toHaveBeenCalledTimes(1)
    expect(mockPush).toHaveBeenCalledWith('/login')
  })
})
