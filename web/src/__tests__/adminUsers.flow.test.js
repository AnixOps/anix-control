import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import Users from '@/views/admin/Users.vue'

const mockCreateUser = vi.fn()
const mockGetUserList = vi.fn()
const mockGetUserStats = vi.fn()
const mockUpdateUser = vi.fn()
const mockBanUser = vi.fn()
const mockGetSubscriptionGroups = vi.fn()
const mockGetSubscriptionSettings = vi.fn()
const mockGetTrafficHourly = vi.fn()
const mockResetUserSubscribe = vi.fn()

vi.mock('@/api/admin', () => ({
  assignAdminUserTunnel: vi.fn(),
  banUser: (...args) => mockBanUser(...args),
  createUser: (...args) => mockCreateUser(...args),
  getAdminUserTunnelList: vi.fn(),
  getForwardTunnels: vi.fn(),
  getSpeedLimitList: vi.fn(),
  getSubscriptionGroups: (...args) => mockGetSubscriptionGroups(...args),
  getSubscriptionSettings: (...args) => mockGetSubscriptionSettings(...args),
  getTrafficHourly: (...args) => mockGetTrafficHourly(...args),
  getUserList: (...args) => mockGetUserList(...args),
  getUserStats: (...args) => mockGetUserStats(...args),
  removeAdminUserTunnel: vi.fn(),
  resetUserSubscribe: (...args) => mockResetUserSubscribe(...args),
  resetUserTraffic: vi.fn(),
  resetUserTunnelTraffic: vi.fn(),
  unbanUser: vi.fn(),
  updateAdminUserTunnel: vi.fn(),
  updateUser: (...args) => mockUpdateUser(...args),
}))

describe('Admin Users flow', () => {
  beforeEach(() => {
    mockCreateUser.mockReset()
    mockGetUserList.mockReset()
    mockGetUserStats.mockReset()
    mockUpdateUser.mockReset()
    mockBanUser.mockReset()
    mockGetSubscriptionGroups.mockReset()
    mockGetSubscriptionSettings.mockReset()
    mockGetTrafficHourly.mockReset()
    mockResetUserSubscribe.mockReset()
    vi.spyOn(window, 'alert').mockImplementation(() => {})
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    mockGetUserList.mockResolvedValue({ data: { list: [], total: 0 } })
    mockGetUserStats.mockResolvedValue({ data: {} })
    mockGetSubscriptionGroups.mockResolvedValue({ data: [] })
    mockGetSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    mockGetTrafficHourly.mockResolvedValue({ data: [] })
    mockResetUserSubscribe.mockResolvedValue({})
    mockUpdateUser.mockResolvedValue({})
    mockBanUser.mockResolvedValue({})
    mockCreateUser.mockResolvedValue({})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows and saves user speed and device limits', async () => {
    const user = {
      id: 1,
      email: 'limit-user@example.com',
      transfer_enable: 1073741824,
      u: 0,
      d: 0,
      speed_limit: 20,
      device_limit: 2,
      banned: 0,
      created_at: '2026-01-01T00:00:00Z',
    }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })

    const wrapper = mount(Users)
    await flushPromises()

    expect(wrapper.text()).toContain('20 Mbps / 2 devices')

    wrapper.vm.editUser(user)
    await nextTick()

    await wrapper.find('[data-test="user-speed-limit-input"]').setValue('80')
    await wrapper.find('[data-test="user-device-limit-input"]').setValue('5')
    await wrapper.find('[data-test="user-save-button"]').trigger('click')
    await flushPromises()

    expect(mockUpdateUser).toHaveBeenCalledWith(1, expect.objectContaining({
      speed_limit: 80,
      device_limit: 5,
    }))
  })

  it('renders user stats from legacy and panel envelope payloads', async () => {
    mockGetUserList.mockResolvedValue({ data: { list: [], total: 0 } })
    mockGetUserStats
      .mockResolvedValueOnce({
        data: {
          total_users: 12,
          active_users: 9,
          expired_users: 2,
          banned_users: 1,
        },
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          total_users: 21,
          active_users: 18,
          expired_users: 2,
          banned_users: 1,
        },
        ts: 1783526400000,
      })

    const wrapper = mount(Users)
    await flushPromises()

    let metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['12', '9', '2', '1'])

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['21', '18', '2', '1'])
  })

  it('builds subscribe links from legacy and panel envelope subscription settings', async () => {
    mockGetUserList.mockResolvedValue({ data: { list: [], total: 0 } })
    mockGetSubscriptionSettings
      .mockResolvedValueOnce({
        data: {
          subscribe_path: '/sub',
          subscribe_domains: ['legacy.example.com']
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          subscribe_path: '/x',
          subscribe_domains: ['panel.example.com']
        },
        ts: 1783526400000
      })

    const wrapper = mount(Users)
    await flushPromises()

    expect(wrapper.vm.buildSubscribeUrl({ token: 'tok_legacy' })).toBe('http://legacy.example.com/sub/tok_legacy')

    await wrapper.vm.loadSubscriptionSettings()
    await flushPromises()

    expect(wrapper.vm.buildSubscribeUrl({ token: 'tok_panel' })).toBe('http://panel.example.com/x/tok_panel')
  })

  it('loads users, subscription groups, and traffic rows from legacy, panel, and nested payloads', async () => {
    mockGetUserList.mockResolvedValueOnce({
      data: { list: [{ id: 1, email: 'legacy@example.com', banned: 0 }], total: 1 }
    })
    mockGetSubscriptionGroups.mockResolvedValueOnce({ data: [{ id: 10, name: 'Legacy Group' }] })

    const wrapper = mount(Users)
    await flushPromises()

    expect(wrapper.vm.users[0].email).toBe('legacy@example.com')
    expect(wrapper.vm.total).toBe(1)
    expect(wrapper.vm.subscriptionGroups[0].name).toBe('Legacy Group')

    mockGetUserList.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { list: [{ id: 2, email: 'panel@example.com', banned: 0 }], total: 2 },
      ts: 1783526400000,
    })
    await wrapper.vm.fetchUsers()
    await flushPromises()

    expect(wrapper.vm.users[0].email).toBe('panel@example.com')
    expect(wrapper.vm.total).toBe(2)

    mockGetSubscriptionGroups.mockResolvedValueOnce({ data: { data: [{ id: 11, name: 'Nested Group' }] } })
    await wrapper.vm.loadSubscriptionGroups()
    await flushPromises()
    expect(wrapper.vm.subscriptionGroups[0].name).toBe('Nested Group')

    mockGetTrafficHourly.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ hour_ts: 1783526400, traffic: 4096 }],
      ts: 1783526400000,
    })
    await wrapper.vm.openTrafficModal(wrapper.vm.users[0])
    await flushPromises()

    expect(mockGetTrafficHourly).toHaveBeenCalledWith(720, 2)
    expect(wrapper.vm.hourlyTrafficRows).toEqual([{ hour_ts: 1783526400, traffic: 4096 }])
  })

  it('accepts enveloped reset-subscribe responses', async () => {
    const user = { id: 3, email: 'reset@example.com', token: 'old-token', banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockResetUserSubscribe.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { token: 'new-token' },
      ts: 1783526400000,
    })

    const wrapper = mount(Users)
    await flushPromises()

    await wrapper.vm.resetSubscribe(user)
    await flushPromises()

    expect(mockResetUserSubscribe).toHaveBeenCalledWith(3)
    expect(window.alert).toHaveBeenCalledWith('Subscription link reset')
  })

  it('treats panel code -1 create-user responses as form errors', async () => {
    mockCreateUser.mockResolvedValueOnce({
      code: -1,
      msg: '该邮箱已被注册',
      data: null,
      ts: 1783526400000,
    })

    const wrapper = mount(Users)
    await flushPromises()

    wrapper.vm.newUser.email = 'duplicate@example.com'
    wrapper.vm.newUser.password = 'password123'
    await wrapper.vm.handleCreateUser()
    await flushPromises()

    expect(wrapper.vm.createError).toBe('该邮箱已被注册')
    expect(window.alert).not.toHaveBeenCalledWith('User created successfully')
  })

  it('treats panel code -1 update-user responses as errors', async () => {
    const user = { id: 404, email: 'missing@example.com', banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockUpdateUser.mockResolvedValueOnce({
      code: -1,
      msg: '用户不存在',
      data: null,
      ts: 1783526400000,
    })

    const wrapper = mount(Users)
    await flushPromises()

    wrapper.vm.editUser(user)
    await wrapper.vm.saveUser()
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('用户不存在'))
    expect(mockUpdateUser).toHaveBeenCalledWith(404, expect.any(Object))
  })

  it('treats panel code -1 ban responses as errors', async () => {
    const user = { id: 405, email: 'ban-missing@example.com', banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockBanUser.mockResolvedValueOnce({
      code: -1,
      msg: '用户不存在',
      data: null,
      ts: 1783526400000,
    })

    const wrapper = mount(Users)
    await flushPromises()

    await wrapper.vm.handleBan(user)
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith('用户不存在')
    expect(mockBanUser).toHaveBeenCalledWith(405)
  })

  it('treats panel code -1 reset-subscribe responses as errors', async () => {
    const user = { id: 406, email: 'sub-missing@example.com', token: 'old-token', banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockResetUserSubscribe.mockResolvedValueOnce({
      code: -1,
      msg: '用户不存在',
      data: null,
      ts: 1783526400000,
    })

    const wrapper = mount(Users)
    await flushPromises()

    await wrapper.vm.resetSubscribe(user)
    await flushPromises()

    expect(window.alert).toHaveBeenCalledWith('用户不存在')
    expect(mockResetUserSubscribe).toHaveBeenCalledWith(406)
  })
})
