import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UserLayout from '@/layouts/UserLayout.vue'

describe('UserLayout.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders slot content', () => {
    const wrapper = mount(UserLayout, {
      slots: {
        default: '<div class="test-content">Test Content</div>',
      },
      global: {
        stubs: ['router-link', 'router-view'],
      },
    })

    expect(wrapper.html()).toContain('Test Content')
  })
})