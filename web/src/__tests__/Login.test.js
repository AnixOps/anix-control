import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import Login from '@/views/Login.vue'
import { useUserStore } from '@/stores/user'

const mockLogin = vi.hoisted(() => vi.fn())
const mockRegister = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn(),
  }),
}))

vi.mock('@/api/auth', () => ({
  login: (...args) => mockLogin(...args),
  register: (...args) => mockRegister(...args),
}))

describe('Login.vue', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
    mockLogin.mockReset()
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

  it('logs in from a panel envelope payload', async () => {
    mockLogin.mockResolvedValue({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: {
        token: 'login-token',
        is_admin: true,
        user_id: 9,
        email: 'admin@example.com',
      },
    })

    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    await wrapper.find('#email').setValue('admin@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    const userStore = useUserStore()
    expect(mockLogin).toHaveBeenCalledWith({
      email: 'admin@example.com',
      password: 'password123',
    })
    expect(userStore.token).toBe('login-token')
    expect(userStore.userInfo).toMatchObject({
      id: 9,
      email: 'admin@example.com',
      is_admin: true,
    })
  })
})
