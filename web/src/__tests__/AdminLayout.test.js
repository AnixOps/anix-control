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
    expect(wrapper.text()).toContain('Extensions')
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
    expect(wrapper.text()).not.toContain('Extensions')
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
