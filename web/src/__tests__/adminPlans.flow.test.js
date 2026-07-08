import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import Plans from '@/views/admin/Plans.vue'

const mockGetPlans = vi.fn()
const mockGetPlanGroups = vi.fn()
const mockGetSubscriptionGroups = vi.fn()
const mockUpdatePlan = vi.fn()

vi.mock('@/api/admin', () => ({
  addGroupToPlan: vi.fn(),
  assignPlanToUser: vi.fn(),
  createPlan: vi.fn(),
  deletePlan: vi.fn(),
  getPlanGroups: (...args) => mockGetPlanGroups(...args),
  getPlans: (...args) => mockGetPlans(...args),
  getSubscriptionGroups: (...args) => mockGetSubscriptionGroups(...args),
  removeGroupFromPlan: vi.fn(),
  updatePlan: (...args) => mockUpdatePlan(...args),
}))

describe('Admin Plans flow', () => {
  beforeEach(() => {
    mockGetPlans.mockReset()
    mockGetPlanGroups.mockReset()
    mockGetSubscriptionGroups.mockReset()
    mockUpdatePlan.mockReset()

    mockGetPlanGroups.mockResolvedValue({ data: [] })
    mockGetSubscriptionGroups.mockResolvedValue({ data: [] })
    mockUpdatePlan.mockResolvedValue({})
  })

  it('shows and saves plan speed and device limits', async () => {
    const plan = {
      id: 2,
      name: 'Business',
      transfer_enable: 200,
      speed_limit: 30,
      device_limit: 3,
      month_price: 1200,
    }
    mockGetPlans.mockResolvedValue({ data: [plan] })

    const wrapper = mount(Plans)
    await flushPromises()

    expect(wrapper.text()).toContain('30 Mbps / 3 devices')

    wrapper.vm.edit(plan)
    await nextTick()

    await wrapper.find('[data-test="plan-speed-limit-input"]').setValue('90')
    await wrapper.find('[data-test="plan-device-limit-input"]').setValue('6')
    await wrapper.find('[data-test="plan-save-button"]').trigger('click')
    await flushPromises()

    expect(mockUpdatePlan).toHaveBeenCalledWith(2, expect.objectContaining({
      speed_limit: 90,
      device_limit: 6,
    }))
  })

  it('loads plans and groups from legacy, panel, and nested payloads', async () => {
    mockGetPlans.mockResolvedValueOnce({
      data: [{ id: 1, name: 'Legacy Plan', speed_limit: 10, device_limit: 1 }]
    })
    mockGetPlanGroups.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ id: 9, name: 'Panel Group' }],
      ts: 1783526400000,
    })
    mockGetSubscriptionGroups.mockResolvedValueOnce({
      data: { data: [{ id: 10, name: 'Nested Group' }] }
    })

    const wrapper = mount(Plans)
    await flushPromises()

    expect(wrapper.vm.plans[0].name).toBe('Legacy Plan')
    expect(wrapper.vm.planGroups[1][0].name).toBe('Panel Group')
    expect(wrapper.vm.allGroups[0].name).toBe('Nested Group')

    mockGetPlans.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ id: 2, name: 'Envelope Plan', speed_limit: 20, device_limit: 2 }],
      ts: 1783526400000,
    })
    mockGetPlanGroups.mockResolvedValueOnce({ data: [] })

    await wrapper.vm.load()
    await flushPromises()

    expect(wrapper.vm.plans[0].name).toBe('Envelope Plan')
  })
})
