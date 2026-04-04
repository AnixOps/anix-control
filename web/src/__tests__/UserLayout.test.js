import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useUserStore } from '@/stores/user'
import UserLayout from '@/layouts/UserLayout.vue'

const mockPush = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockPush,
  }),
}))

describe('UserLayout.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockPush.mockReset()
    localStorage.clear()
  })

  it('renders router view container', () => {
    const wrapper = mount(UserLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    expect(wrapper.find('router-view-stub').exists()).toBe(true)
  })

  it('contains all user menu routes', () => {
    const expectedPaths = [
      '/user/dashboard',
      '/user/subscribe',
      '/user/knowledge',
      '/user/tickets',
      '/user/plans',
      '/user/orders',
    ]

    const wrapper = mount(UserLayout, {
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

    expect(links).toEqual(expect.arrayContaining(expectedPaths))
  })

  it('fetches user info on mount when already logged in', () => {
    const userStore = useUserStore()
    userStore.token = 'token-123'
    userStore.getUserInfo = vi.fn()

    mount(UserLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    expect(userStore.getUserInfo).toHaveBeenCalledTimes(1)
  })

  it('logs out and redirects to login', async () => {
    const wrapper = mount(UserLayout, {
      global: {
        stubs: {
          'router-link': true,
          'router-view': true,
        },
      },
    })

    const userStore = useUserStore()
    userStore.logout = vi.fn()

    await wrapper.find('.user-actions .btn-ghost').trigger('click')

    expect(userStore.logout).toHaveBeenCalledTimes(1)
    expect(mockPush).toHaveBeenCalledWith('/login')
  })
})
