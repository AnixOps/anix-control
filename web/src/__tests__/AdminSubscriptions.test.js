import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import Subscriptions from '@/views/admin/Subscriptions.vue'
import { setLocale } from '@/i18n'

const adminApiMock = vi.hoisted(() => ({
  getAvailableProtocols: vi.fn(),
  getSubscriptionGroups: vi.fn(),
  getSubscriptionProtocols: vi.fn(),
  getSubscriptionTemplates: vi.fn()
}))

const getSubscriptionStatsMock = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  default: adminApiMock,
  getSubscriptionStats: (...args) => getSubscriptionStatsMock(...args)
}))

describe('Admin Subscriptions', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    setActivePinia(createPinia())
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApiMock.getSubscriptionGroups.mockResolvedValue({ data: [] })
    adminApiMock.getSubscriptionTemplates.mockResolvedValue({ data: [] })
    adminApiMock.getSubscriptionProtocols.mockResolvedValue({ data: [] })
    adminApiMock.getAvailableProtocols.mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders subscription stats from legacy and panel envelope payloads', async () => {
    getSubscriptionStatsMock
      .mockResolvedValueOnce({
        data: [{
          enabled_users: 10,
          group_id: 1,
          group_name: 'Default',
          online_nodes: 1,
          plan_count: 4,
          protocol_count: 2,
          template_count: 3,
          total_traffic: 2147483648,
          user_count: 12
        }]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [{
          enabled_users: 18,
          group_id: 2,
          group_name: 'VIP',
          online_nodes: 2,
          plan_count: 6,
          protocol_count: 4,
          template_count: 5,
          total_traffic: 3221225472,
          user_count: 21
        }],
        ts: 1783526400000
      })

    const wrapper = mount(Subscriptions)
    await flushPromises()

    let metricValues = wrapper.findAll('.overview-stats .stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['0', '12', '3', '2.00 GB'])
    expect(wrapper.text()).toContain('Default')

    await wrapper.vm.loadStats()
    await flushPromises()

    metricValues = wrapper.findAll('.overview-stats .stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['0', '21', '5', '3.00 GB'])
    expect(wrapper.text()).toContain('VIP')

    wrapper.unmount()
  })
})
