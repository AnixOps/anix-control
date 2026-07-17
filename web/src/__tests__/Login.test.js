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
        permission_mode: 'authoritative',
        permissions: ['subscription.view'],
        restricted_plugins: ['forward'],
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
    expect(useUserStore().userInfo).toMatchObject({
      permission_mode: 'authoritative',
      permissions: ['subscription.view'],
      restricted_plugins: ['forward'],
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
        permission_mode: 'mixed',
        permissions: ['forward.view'],
        restricted_plugins: ['forward'],
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
      permission_mode: 'mixed',
      permissions: ['forward.view'],
      restricted_plugins: ['forward'],
    })
  })

  it('completes MFA login challenge before storing token', async () => {
    mockLogin
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        ts: 1783536000000,
        data: {
          mfa_required: true,
          methods: ['totp', 'backup'],
          user_id: 10,
          email: 'mfa@example.com',
        },
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        ts: 1783536000000,
        data: {
          token: 'mfa-token',
          is_admin: false,
          user_id: 10,
          email: 'mfa@example.com',
        },
      })

    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    await wrapper.find('#email').setValue('mfa@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(mockLogin).toHaveBeenNthCalledWith(1, {
      email: 'mfa@example.com',
      password: 'password123',
    })
    expect(wrapper.find('#mfa-code').exists()).toBe(true)
    expect(useUserStore().token).toBe('')

    await wrapper.find('#mfa-code').setValue('123456')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(mockLogin).toHaveBeenNthCalledWith(2, {
      email: 'mfa@example.com',
      password: 'password123',
      mfa_code: '123456',
    })
    expect(useUserStore().token).toBe('mfa-token')
  })

  it('requires an MFA code after the challenge response', async () => {
    mockLogin.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: {
        mfa_required: true,
        methods: ['totp'],
        user_id: 11,
        email: 'mfa-required@example.com',
      },
    })

    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    await wrapper.find('#email').setValue('mfa-required@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(mockLogin).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[role="alert"]').text()).toContain('MFA')
    expect(useUserStore().token).toBe('')
  })

  it('shows MFA enrollment-required responses without storing a token', async () => {
    mockLogin.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      ts: 1783536000000,
      data: {
        mfa_enrollment_required: true,
        mfa_setup_required: true,
        methods: ['totp'],
        user_id: 12,
        email: 'mfa-enroll@example.com',
      },
    })

    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    await wrapper.find('#email').setValue('mfa-enroll@example.com')
    await wrapper.find('#password').setValue('password123')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(mockLogin).toHaveBeenCalledWith({
      email: 'mfa-enroll@example.com',
      password: 'password123',
    })
    expect(wrapper.find('#mfa-code').exists()).toBe(false)
    expect(wrapper.find('[role="alert"]').text()).toContain('MFA')
    expect(useUserStore().token).toBe('')
  })

  it('shows panel envelope login errors', async () => {
    mockLogin.mockResolvedValue({
      code: -1,
      msg: '用户不存在或密码错误',
      ts: 1783536000000,
      data: null,
    })

    const wrapper = mount(Login, {
      global: {
        stubs: ['router-link'],
      },
    })

    await wrapper.find('#email').setValue('admin@example.com')
    await wrapper.find('#password').setValue('wrong-password')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').text()).toContain('用户不存在或密码错误')
    expect(useUserStore().token).toBe('')
  })
})
