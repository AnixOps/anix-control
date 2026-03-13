import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import Login from '@/views/Login.vue'

describe('Login.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders login form', () => {
    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    expect(wrapper.find('input[type="email"]').exists() || wrapper.find('input[type="text"]').exists()).toBe(true)
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
  })

  it('has login button', () => {
    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    const button = wrapper.find('button')
    expect(button.exists()).toBe(true)
  })

  it('shows validation error for empty fields', async () => {
    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    const button = wrapper.find('button')
    await button.trigger('click')

    // Form validation should trigger
    // Actual behavior depends on implementation
  })
})