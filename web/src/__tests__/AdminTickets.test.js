import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Tickets from '@/views/admin/Tickets.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  closeTicket: vi.fn(),
  getTickets: vi.fn(),
  replyTicket: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  default: adminApi,
  ...adminApi
}))

describe('Admin Tickets', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders tickets from legacy and panel envelope payloads', async () => {
    adminApi.getTickets
      .mockResolvedValueOnce({
        data: [
          { id: 1, user_id: 10, subject: 'Legacy open', level: 1, status: 0, created_at: 1783526400 },
          { id: 2, user_id: 11, subject: 'Legacy closed', level: 2, status: 2, created_at: 1783526500 }
        ]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [
          { id: 3, user_id: 12, subject: 'Panel answered', level: 0, status: 1, created_at: 1783526600 }
        ],
        ts: 1783526400000
      })

    const wrapper = mount(Tickets)
    await flushPromises()

    let metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['1', '0', '1'])
    expect(wrapper.text()).toContain('Legacy open')
    expect(wrapper.text()).toContain('Legacy closed')

    await wrapper.vm.load()
    await flushPromises()

    metricValues = wrapper.findAll('.metric-card strong').map(node => node.text())
    expect(metricValues).toEqual(['0', '1', '0'])
    expect(wrapper.text()).toContain('Panel answered')
    expect(wrapper.text()).not.toContain('Legacy open')

    wrapper.unmount()
  })
})
