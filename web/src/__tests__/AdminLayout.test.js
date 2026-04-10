import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import AdminLayout from '@/layouts/AdminLayout.vue'

const mockPush = vi.fn()
const mockRoute = reactive({ path: '/admin/dashboard' })

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mockPush }),
  useRoute: () => mockRoute,
}))

const adminMenuPaths = [
  '/admin/dashboard',
  '/admin/users',
  '/admin/orders',
  '/admin/tickets',
  '/admin/nodes',
  '/admin/subscriptions',
  '/admin/forward',
  '/admin/forward/tunnel',
  '/admin/forward/limit',
  '/admin/forward/ansible-machines',
  '/admin/forward/nodes',
  '/admin/forward/local',
  '/admin/forward/nodex',
  '/admin/forward/agents',
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
]

describe('AdminLayout.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockRoute.path = '/admin/dashboard'
    mockPush.mockReset()
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

  it('exposes accessible navigation controls and main landmark', () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    expect(wrapper.find('.menu-toggle').attributes('aria-controls')).toBe('admin-sidebar')
    expect(wrapper.find('.menu-toggle').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('.sidebar').attributes('id')).toBe('admin-sidebar')
    expect(wrapper.find('main.main-content').attributes('id')).toBe('app-main-content')
    expect(wrapper.find('main.main-content').attributes('tabindex')).toBe('-1')
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
    expect(new Set(links).size).toBe(adminMenuPaths.length)
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
    expect(wrapper.find('h1').text()).toContain('Telegram')

    mockRoute.path = '/admin/mfa'
    await nextTick()
    expect(wrapper.find('h1').text()).toContain('MFA')

    mockRoute.path = '/admin/forward/agents'
    await nextTick()
    expect(wrapper.find('h1').text()).toContain('NodeX Agents')

    mockRoute.path = '/admin/forward/nodes'
    await nextTick()
    expect(wrapper.find('h1').text()).toContain('NodeX Topology')
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
    expect(wrapper.find('h1').text()).toContain('NodeX')
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
    expect(wrapper.find('h1').text()).toContain('Local Runtime')
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
    expect(wrapper.find('h1').text()).toContain('Ansible Machines')
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

    await wrapper.find('.sidebar-footer .btn-ghost').trigger('click')

    expect(userStore.logout).toHaveBeenCalledTimes(1)
    expect(mockPush).toHaveBeenCalledWith('/login')
  })
})
