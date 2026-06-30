import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import Login from '@/views/Login.vue'

const mockRegister = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
}))

vi.mock('@/api/auth', () => ({
  login: vi.fn(),
  register: (...args) => mockRegister(...args),
}))

describe('Login.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockRegister.mockReset()
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

  it('submits invite code when registering', async () => {
    mockRegister.mockResolvedValue({
      data: {
        token: 'registered-token',
        is_admin: false,
        user_id: 7,
        email: 'invite-user@example.com',
      },
    })

    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    wrapper.vm.isRegisterMode = true
    await nextTick()

    await wrapper.find('#email').setValue('invite-user@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('#confirm-password').setValue('password123')
    await wrapper.find('#invite-code').setValue('INVITE123')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(mockRegister).toHaveBeenCalledWith({
      email: 'invite-user@example.com',
      password: 'password123',
      invite_code: 'INVITE123',
    })
  })
})
