import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Users from '@/views/admin/Users.vue'
import { setEdition } from '@/composables/useEdition'
import { allInBody, answerConfirms, inBody, toastMessages, toasts } from './helpers/feedback'

const mockCreateUser = vi.fn()
const mockGetUserList = vi.fn()
const mockGetUserStats = vi.fn()
const mockUpdateUser = vi.fn()
const mockBanUser = vi.fn()
const mockGetSubscriptionGroups = vi.fn()
const mockGetSubscriptionSettings = vi.fn()
const mockGetTrafficHourly = vi.fn()
const mockResetUserSubscribe = vi.fn()
const mockGetAdminUser = vi.fn()

vi.mock('@/api/admin', () => ({
  assignAdminUserTunnel: vi.fn(),
  banUser: (...args) => mockBanUser(...args),
  createUser: (...args) => mockCreateUser(...args),
  getAdminUser: (...args) => mockGetAdminUser(...args),
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

enableAutoUnmount(afterEach)

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
    mockGetAdminUser.mockReset()

    mockGetUserList.mockResolvedValue({ data: { list: [], total: 0 } })
    mockGetUserStats.mockResolvedValue({ data: {} })
    mockGetSubscriptionGroups.mockResolvedValue({ data: [] })
    mockGetSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    mockGetTrafficHourly.mockResolvedValue({ data: [] })
    mockResetUserSubscribe.mockResolvedValue({})
    mockUpdateUser.mockResolvedValue({})
    mockBanUser.mockResolvedValue({})
    mockCreateUser.mockResolvedValue({})
    mockGetAdminUser.mockResolvedValue({ code: 0, msg: '操作成功', data: {}, ts: 1783526400000 })
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

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    // Limits are in the details (and an optional column).
    wrapper.vm.openDetail(wrapper.vm.users[0])
    await flushPromises()
    expect(inBody('[data-test="user-detail-sheet"]').text()).toContain('20 Mbps / 2 devices')

    await wrapper.vm.editUser(user)
    await nextTick()

    await inBody('[data-test="user-speed-limit-input"]').setValue('80')
    await inBody('[data-test="user-device-limit-input"]').setValue('5')
    await inBody('[data-test="user-save-button"]').trigger('click')
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

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    // The counts sit on the status filter chips.
    const chipCounts = () => wrapper.findAll('.ui-filter-chips__count').map(node => node.text())
    expect(chipCounts()).toEqual(['9', '2', '1'])

    await wrapper.vm.fetchStats()
    await flushPromises()

    expect(chipCounts()).toEqual(['18', '2', '1'])
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

    const wrapper = mount(Users, { attachTo: document.body })
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

    const wrapper = mount(Users, { attachTo: document.body })
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

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    const confirms = answerConfirms(true)
    await wrapper.vm.resetSubscribe(user)
    await flushPromises()

    expect(confirms.last()).toMatchObject({ tone: 'danger', title: 'Reset the subscription link of reset@example.com?', confirmLabel: 'Reset link' })
    expect(mockResetUserSubscribe).toHaveBeenCalledWith(3)
    expect(toastMessages('success')).toContain('Subscription link reset')
  })

  it('treats panel code -1 create-user responses as form errors', async () => {
    mockCreateUser.mockResolvedValueOnce({
      code: -1,
      msg: '该邮箱已被注册',
      data: null,
      ts: 1783526400000,
    })

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    wrapper.vm.newUser.email = 'duplicate@example.com'
    wrapper.vm.newUser.password = 'password123'
    await wrapper.vm.handleCreateUser()
    await flushPromises()

    expect(wrapper.vm.createError).toBe('该邮箱已被注册')
    expect(toastMessages()).not.toContain('User created successfully')
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

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    await wrapper.vm.editUser(user)
    await wrapper.vm.saveUser()
    await flushPromises()

    // A failed save stays in the dialog as an inline error.
    expect(wrapper.vm.showEditModal).toBe(true)
    expect(inBody('[data-test="user-save-error"]').text()).toContain('用户不存在')
    expect(toasts()).toHaveLength(0)
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

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    await wrapper.vm.handleBan(user)
    await flushPromises()

    expect(toastMessages('error')).toEqual(['用户不存在'])
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

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    const confirms = answerConfirms(true)
    await wrapper.vm.resetSubscribe(user)
    await flushPromises()

    // The dialog keeps the failure inline and stays open; nothing succeeded.
    expect(confirms.errors.map(error => error.message)).toEqual(['用户不存在'])
    expect(toastMessages('success')).toEqual([])
    expect(mockResetUserSubscribe).toHaveBeenCalledWith(406)
  })

  it('shows each user\'s plan by the name the slim list carries', async () => {
    mockGetUserList.mockResolvedValue({
      data: {
        list: [
          { id: 1, email: 'pro@example.com', plan_id: 7, plan: { id: 7, name: 'Pro' }, banned: 0 },
          { id: 2, email: 'gone@example.com', plan_id: 99, banned: 0 },
          { id: 3, email: 'none@example.com', plan_id: null, banned: 0 },
        ],
        total: 3,
      },
    })

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()

    const headers = wrapper.findAll('thead th').map(th => th.text())
    const column = headers.indexOf('Subscription template')
    expect(column).toBeGreaterThan(0)
    const planCells = wrapper.findAll('tbody tr').map(row => row.findAll('td')[column]?.text())
    expect(planCells).toEqual(['Pro', '—', '—'])
  })

  it('copies the subscription link with the token of the user detail', async () => {
    const user = { id: 8, email: 'copy@example.com', banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockGetAdminUser.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { id: 8, token: 'detail-token' }, ts: 1783526400000 })
    const writeText = vi.fn().mockResolvedValue(undefined)
    const clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    const secure = Object.getOwnPropertyDescriptor(window, 'isSecureContext')
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })

    try {
      const wrapper = mount(Users, { attachTo: document.body })
      await flushPromises()
      await wrapper.vm.copySubscribe(user)
      await flushPromises()

      expect(mockGetAdminUser).toHaveBeenCalledWith(8)
      expect(writeText).toHaveBeenCalledWith(`${window.location.protocol}//${window.location.host}/s/detail-token`)
      expect(toastMessages('success')).toEqual(['Subscription link copied to clipboard'])
    } finally {
      if (clipboard) Object.defineProperty(navigator, 'clipboard', clipboard)
      else delete navigator.clipboard
      if (secure) Object.defineProperty(window, 'isSecureContext', secure)
      else delete window.isSecureContext
    }
  })

  it('reports a user without a token and a detail that fails to load', async () => {
    const user = { id: 9, email: 'none@example.com', banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockGetAdminUser
      .mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { id: 9, token: '' }, ts: 1783526400000 })
      .mockResolvedValueOnce({ code: -1, msg: '用户不存在', data: null, ts: 1783526400000 })

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.copySubscribe(user)
    await wrapper.vm.copySubscribe(user)
    await flushPromises()

    expect(toastMessages('error')).toEqual(['This user has no subscription token', '用户不存在'])
  })

  it('fills the edit form from the user detail, remark included', async () => {
    const user = { id: 10, email: 'edit@example.com', balance: 5, banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockGetAdminUser.mockResolvedValueOnce({
      code: 0, msg: '操作成功', data: { id: 10, email: 'edit@example.com', balance: 7, remark_content: 'vip' }, ts: 1783526400000,
    })

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.editUser(user)
    await flushPromises()

    expect(mockGetAdminUser).toHaveBeenCalledWith(10)
    expect(wrapper.vm.editingUser.remark_content).toBe('vip')
    expect(wrapper.vm.editingUser.balance).toBe(7)

    mockGetAdminUser.mockRejectedValueOnce(new Error('offline'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await wrapper.vm.editUser(user)
    expect(wrapper.vm.editingUser.balance).toBe(5)
    expect(wrapper.vm.editingUser.remark_content).toBeUndefined()
    expect(wrapper.vm.showEditModal).toBe(true)
  })

  it('hides the balance in the community edition and keeps the stored value', async () => {
    setEdition('community')
    const user = { id: 11, email: 'community@example.com', balance: 9, banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockGetAdminUser.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { ...user }, ts: 1783526400000 })

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.editUser(user)
    await flushPromises()

    expect(inBody('[data-test="user-balance-field"]').exists()).toBe(false)
    expect(wrapper.findAll('thead th').map(th => th.text())).toContain('Subscription template')
    await wrapper.vm.saveUser()
    expect(mockUpdateUser).toHaveBeenCalledWith(11, expect.objectContaining({ balance: 9 }))
  })

  it('shows the balance in the commercial edition', async () => {
    setEdition('commercial')
    const user = { id: 12, email: 'commercial@example.com', balance: 3, banned: 0 }
    mockGetUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    mockGetAdminUser.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { ...user }, ts: 1783526400000 })

    const wrapper = mount(Users, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.editUser(user)
    await flushPromises()

    expect(inBody('[data-test="user-balance-field"]').exists()).toBe(true)
    expect(wrapper.findAll('thead th').map(th => th.text())).toContain('Plan')
  })
})
