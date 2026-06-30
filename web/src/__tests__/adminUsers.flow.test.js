import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import Users from '@/views/admin/Users.vue'

const mockCreateUser = vi.fn()
const mockGetUserList = vi.fn()
const mockGetUserStats = vi.fn()
const mockUpdateUser = vi.fn()
const mockGetSubscriptionGroups = vi.fn()

vi.mock('@/api/admin', () => ({
  assignAdminUserTunnel: vi.fn(),
  banUser: vi.fn(),
  createUser: (...args) => mockCreateUser(...args),
  getAdminUserTunnelList: vi.fn(),
  getForwardTunnels: vi.fn(),
  getSpeedLimitList: vi.fn(),
  getSubscriptionGroups: (...args) => mockGetSubscriptionGroups(...args),
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
    vi.spyOn(window, 'alert').mockImplementation(() => {})

    mockGetUserStats.mockResolvedValue({ data: {} })
    mockGetSubscriptionGroups.mockResolvedValue({ data: [] })
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
})
