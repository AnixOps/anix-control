import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UserLayout from '@/layouts/UserLayout.vue'

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
}))

describe('UserLayout.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
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
})
