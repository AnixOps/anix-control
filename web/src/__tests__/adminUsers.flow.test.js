import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import Users from '@/views/admin/Users.vue'

const mockCreateUser = vi.fn()
const mockGetUserList = vi.fn()
const mockGetUserStats = vi.fn()
const mockUpdateUser = vi.fn()
const mockGetSubscriptionGroups = vi.fn()
const mockGetSubscriptionSettings = vi.fn()

vi.mock('@/api/admin', () => ({
  assignAdminUserTunnel: vi.fn(),
  banUser: vi.fn(),
  createUser: (...args) => mockCreateUser(...args),
  getAdminUserTunnelList: vi.fn(),
  getForwardTunnels: vi.fn(),
  getSpeedLimitList: vi.fn(),
  getSubscriptionGroups: (...args) => mockGetSubscriptionGroups(...args),
  getSubscriptionSettings: (...args) => mockGetSubscriptionSettings(...args),
  getUserList: (...args) => mockGetUserList(...args),
  getUserStats: (...args) => mockGetUserStats(...args),
  removeAdminUserTunnel: vi.fn(),
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
    mockGetSubscriptionGroups.mockReset()
    mockGetSubscriptionSettings.mockReset()
    vi.spyOn(window, 'alert').mockImplementation(() => {})

    mockGetUserStats.mockResolvedValue({ data: {} })
    mockGetSubscriptionGroups.mockResolvedValue({ data: [] })
    mockGetSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    mockUpdateUser.mockResolvedValue({})
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
})
