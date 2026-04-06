import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive, nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import AdminLayout from '@/layouts/AdminLayout.vue'

const mockPush = vi.fn()
const mockRoute = reactive({ path: '/admin/dashboard' })

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush,
  }),
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
  '/admin/forward/tunnels',
  '/admin/forward/limits',
  '/admin/forward/nodes',
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

    const links = wrapper
      .findAll('a.menu-link')
      .map(link => link.attributes('data-to'))

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

    mockRoute.path = '/admin/agent'
    await nextTick()
    expect(wrapper.find('h1').text()).toContain('Agent')

    mockRoute.path = '/admin/forward/nodes'
    await nextTick()
    expect(wrapper.find('h1').text()).toContain('中转节点')
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
