import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Invite from '@/views/admin/Invite.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  getInviteConfig: vi.fn(),
  getInviteStats: vi.fn(),
  getWithdrawals: vi.fn(),
  processWithdrawal: vi.fn(),
  updateInviteConfig: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

describe('Admin Invite', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getInviteConfig.mockResolvedValue({ data: {} })
    adminApi.getInviteStats.mockResolvedValue({ data: {} })
    adminApi.getWithdrawals.mockResolvedValue({ data: { list: [] } })
    adminApi.processWithdrawal.mockResolvedValue({ code: 0, data: {} })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads invite config from legacy and panel envelope payloads', async () => {
    adminApi.getInviteConfig
      .mockResolvedValueOnce({
        data: {
          enabled: true,
          commission_rate: 15,
          min_withdraw: 25,
          withdraw_methods: ['bank']
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          enabled: false,
          commission_rate_ratio: 0.2,
          min_withdraw: 30,
          withdraw_methods: ['wechat']
        },
        ts: 1783526400000
      })
      .mockResolvedValueOnce({
        data: {
          data: {
            enabled: true,
            commission_rate: 35,
            min_withdraw: 40,
            withdraw_methods: ['alipay']
          }
        }
      })

    const wrapper = mount(Invite)
    await flushPromises()

    expect(wrapper.vm.config.enabled).toBe(true)
    expect(wrapper.vm.config.commission_rate).toBe(15)
    expect(wrapper.vm.config.min_withdraw).toBe(25)
    expect(wrapper.vm.config.withdraw_methods).toEqual(['bank'])

    await wrapper.vm.fetchConfig()
    await flushPromises()

    expect(wrapper.vm.config.enabled).toBe(false)
    expect(wrapper.vm.config.commission_rate).toBe(20)
    expect(wrapper.vm.config.min_withdraw).toBe(30)
    expect(wrapper.vm.config.withdraw_methods).toEqual(['wechat'])

    await wrapper.vm.fetchConfig()
    await flushPromises()

    expect(wrapper.vm.config.enabled).toBe(true)
    expect(wrapper.vm.config.commission_rate).toBe(35)
    expect(wrapper.vm.config.min_withdraw).toBe(40)
    expect(wrapper.vm.config.withdraw_methods).toEqual(['alipay'])

    wrapper.unmount()
  })

  it('renders withdrawals from legacy, panel envelope, and nested payloads', async () => {
    adminApi.getWithdrawals
      .mockResolvedValueOnce({
        data: {
          list: [{ id: 1, user_id: 7, amount: 12.3, method: 'alipay', account: 'legacy@example.com', status: 'pending' }]
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          list: [{ id: 2, user_id: 8, amount: 23.4, method: 'wechat', account: 'panel@example.com', status: 'approved' }]
        },
        ts: 1783526400000
      })
      .mockResolvedValueOnce({
        data: {
          data: {
            list: [{ id: 3, user_id: 9, amount: 34.5, method: 'bank', account: 'nested@example.com', status: 'rejected' }]
          }
        }
      })

    const wrapper = mount(Invite)
    await flushPromises()

    expect(wrapper.vm.withdrawals[0].id).toBe(1)
    expect(wrapper.vm.withdrawals[0].status).toBe('pending')

    await wrapper.vm.fetchWithdrawals()
    await flushPromises()

    expect(wrapper.vm.withdrawals[0].id).toBe(2)
    expect(wrapper.vm.withdrawals[0].status).toBe('approved')

    await wrapper.vm.fetchWithdrawals()
    await flushPromises()

    expect(wrapper.vm.withdrawals[0].id).toBe(3)
    expect(wrapper.vm.withdrawals[0].status).toBe('rejected')

    wrapper.unmount()
  })

  it('renders invite stats from legacy, panel envelope, and nested payloads', async () => {
    adminApi.getInviteStats
      .mockResolvedValueOnce({
        data: {
          total_invites: 12,
          total_commission: 123.45,
          pending_commission: 67.89,
          withdrawn_commission: 10.11,
          top_inviters: [{ user_id: 7, invite_count: 3, commission: 33.3 }]
        }
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: {
          total_invites: 21,
          total_commission: 234.56,
          pending_commission: 78.9,
          withdrawn_commission: 11.12,
          top_inviters: [{ user_id: 8, invite_count: 4, commission: 44.4 }]
        },
        ts: 1783526400000
      })
      .mockResolvedValueOnce({
        data: {
          data: {
            total_invites: 31,
            total_commission: 345.67,
            pending_commission: 89.01,
            withdrawn_commission: 12.13,
            top_inviters: [{ user_id: 9, invite_count: 5, commission: 55.5 }]
          }
        }
      })

    const wrapper = mount(Invite)
    await flushPromises()

    let metricValues = wrapper.findAll('.stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['12', '¥123.45', '¥67.89', '¥10.11'])
    expect(wrapper.text()).toContain('7')
    expect(wrapper.text()).toContain('3')

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('.stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['21', '¥234.56', '¥78.90', '¥11.12'])
    expect(wrapper.text()).toContain('8')
    expect(wrapper.text()).toContain('4')

    await wrapper.vm.fetchStats()
    await flushPromises()

    metricValues = wrapper.findAll('.stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['31', '¥345.67', '¥89.01', '¥12.13'])
    expect(wrapper.text()).toContain('9')
    expect(wrapper.text()).toContain('5')

    wrapper.unmount()
  })
})
