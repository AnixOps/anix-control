import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import { runAction } from '@/ui/composables/useToast'
import { nextTick } from 'vue'
import Plans from '@/views/admin/Plans.vue'
import { setEdition } from '@/composables/useEdition'
import { inBody, toastMessages, toasts } from './helpers/feedback'

const mockGetPlans = vi.fn()
const mockGetPlanGroups = vi.fn()
const mockGetSubscriptionGroups = vi.fn()
const mockAssignPlanToUser = vi.fn()
const mockCreatePlan = vi.fn()
const mockDeletePlan = vi.fn()
const mockUpdatePlan = vi.fn()
const mockAddGroupToPlan = vi.fn()
const mockRemoveGroupFromPlan = vi.fn()

vi.mock('@/api/admin', () => ({
  addGroupToPlan: (...args) => mockAddGroupToPlan(...args),
  assignPlanToUser: (...args) => mockAssignPlanToUser(...args),
  createPlan: (...args) => mockCreatePlan(...args),
  deletePlan: (...args) => mockDeletePlan(...args),
  getPlanGroups: (...args) => mockGetPlanGroups(...args),
  getPlans: (...args) => mockGetPlans(...args),
  getSubscriptionGroups: (...args) => mockGetSubscriptionGroups(...args),
  removeGroupFromPlan: (...args) => mockRemoveGroupFromPlan(...args),
  updatePlan: (...args) => mockUpdatePlan(...args),
}))

enableAutoUnmount(afterEach)

describe('Admin Plans flow', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    mockGetPlans.mockReset()
    mockGetPlanGroups.mockReset()
    mockGetSubscriptionGroups.mockReset()
    mockAssignPlanToUser.mockReset()
    mockCreatePlan.mockReset()
    mockDeletePlan.mockReset()
    mockUpdatePlan.mockReset()
    mockAddGroupToPlan.mockReset()
    mockRemoveGroupFromPlan.mockReset()

    mockGetPlanGroups.mockResolvedValue({ data: [] })
    mockGetSubscriptionGroups.mockResolvedValue({ data: [] })
    mockAssignPlanToUser.mockResolvedValue({})
    mockCreatePlan.mockResolvedValue({})
    mockDeletePlan.mockResolvedValue({})
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

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    expect(wrapper.text()).toContain('30 Mbps / 3 devices')

    wrapper.vm.edit(plan)
    await nextTick()

    await inBody('[data-test="plan-speed-limit-input"]').setValue('90')
    await inBody('[data-test="plan-device-limit-input"]').setValue('6')
    await inBody('[data-test="plan-save-button"]').trigger('click')
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

    const wrapper = mount(Plans, { attachTo: document.body })
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

  it('treats panel code -1 save responses as errors', async () => {
    const plan = {
      id: 3,
      name: 'Rejected',
      transfer_enable: 100,
      speed_limit: 0,
      device_limit: 0,
      month_price: 1000,
    }
    mockGetPlans.mockResolvedValue({ data: [plan] })
    mockUpdatePlan.mockResolvedValueOnce({
      code: -1,
      msg: '套餐不存在',
      data: null,
      ts: 1783526400000,
    })

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    wrapper.vm.edit(plan)
    await nextTick()
    await inBody('[data-test="plan-save-button"]').trigger('click')
    await flushPromises()

    // The failure stays in the open dialog.
    expect(inBody('[data-test="plan-save-error"]').text()).toContain('套餐不存在')
    expect(wrapper.vm.showPlanModal).toBe(true)
    expect(toasts()).toHaveLength(0)
    expect(mockGetPlans).toHaveBeenCalledTimes(1)
  })

  it('treats panel code -1 assign responses as errors', async () => {
    const plan = {
      id: 4,
      name: 'Assign Rejected',
      transfer_enable: 100,
      speed_limit: 0,
      device_limit: 0,
      month_price: 1000,
    }
    mockGetPlans.mockResolvedValue({ data: [plan] })
    mockAssignPlanToUser.mockResolvedValueOnce({
      code: -1,
      msg: '用户不存在',
      data: null,
      ts: 1783526400000,
    })

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    wrapper.vm.openAssign(plan)
    wrapper.vm.assignForm.user_id = 99999
    await wrapper.vm.assign()
    await flushPromises()

    await flushPromises()
    expect(inBody('[data-test="plan-assign-error"]').text()).toContain('用户不存在')
    expect(wrapper.vm.showAssign).toBe(true)
  })

  it('manages free subscription templates without prices in the community edition', async () => {
    setEdition('community')
    const plan = { id: 5, name: 'Starter', transfer_enable: 50, speed_limit: 0, device_limit: 0, month_price: 990 }
    mockGetPlans.mockResolvedValue({ data: [plan] })

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    expect(wrapper.find('h1').text()).toBe('Subscription templates')
    expect(wrapper.text()).not.toContain('Monthly price')
    expect(wrapper.text()).not.toContain('990')

    wrapper.vm.openCreateModal()
    await nextTick()
    expect(inBody('[data-test="plan-month-price-field"]').exists()).toBe(false)
    expect(document.body.textContent).toContain('New subscription template')
    wrapper.vm.form.name = 'Template'
    wrapper.vm.form.transfer_enable = 100
    await inBody('[data-test="plan-save-button"]').trigger('click')
    await flushPromises()
    expect(mockCreatePlan).toHaveBeenCalledWith(expect.objectContaining({ name: 'Template', transfer_enable: 100, month_price: null }))

    // Editing a template sends a stored price back unchanged.
    wrapper.vm.edit(plan)
    await nextTick()
    await inBody('[data-test="plan-save-button"]').trigger('click')
    await flushPromises()
    expect(mockUpdatePlan).toHaveBeenCalledWith(5, expect.objectContaining({ month_price: 990 }))

    wrapper.vm.openAssign(plan)
    wrapper.vm.assignForm.user_id = 7
    await wrapper.vm.assign()
    await flushPromises()
    expect(mockAssignPlanToUser).toHaveBeenCalled()
  })

  it('shows plan prices in the commercial edition', async () => {
    setEdition('commercial')
    const plan = { id: 6, name: 'Pro', transfer_enable: 50, speed_limit: 0, device_limit: 0, month_price: 1990 }
    mockGetPlans.mockResolvedValue({ data: [plan] })

    const wrapper = mount(Plans, { attachTo: document.body })
    await flushPromises()

    expect(wrapper.find('h1').text()).toBe('Plans')
    expect(wrapper.text()).toContain('Monthly price')
    expect(wrapper.text()).toContain('19.90')
    wrapper.vm.openCreateModal()
    await nextTick()
    expect(inBody('[data-test="plan-month-price-field"]').exists()).toBe(true)
  })

  describe('dialogs and feedback', () => {
    const plan = { id: 8, name: 'Pro', transfer_enable: 50, speed_limit: 0, device_limit: 0, month_price: 1990 }
    const Harness = {
      components: { Plans, UiHost },
      template: '<div><Plans /><UiHost /></div>'
    }

    beforeEach(() => {
      setEdition('commercial')
      mockGetPlans.mockResolvedValue({ code: 0, data: [plan] })
    })

    it('confirms deleting a plan; Cancel and Esc keep it, a failure stays inline', async () => {
      const user = userEvent.setup()
      mockDeletePlan.mockResolvedValueOnce({ code: -1, msg: 'plan in use' }).mockResolvedValueOnce({ code: 0 })
      render(Harness)
      await screen.findByText('Pro')
      const deleteFromMenu = async () => {
        await user.click(screen.getByRole('button', { name: 'Actions for Pro' }))
        await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Delete' }))
      }

      await deleteFromMenu()
      let dialog = await screen.findByRole('alertdialog', { name: 'Delete plan Pro?' })
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      await deleteFromMenu()
      await screen.findByRole('alertdialog')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(mockDeletePlan).not.toHaveBeenCalled()

      await deleteFromMenu()
      dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete plan' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('plan in use')
      await user.click(within(dialog).getByRole('button', { name: 'Delete plan' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(mockDeletePlan).toHaveBeenLastCalledWith(8)
      expect(toastMessages('success')).toEqual(['Plan Pro deleted'])
    })

    it('names a missing plan name and user ID under their fields', async () => {
      const wrapper = mount(Plans, { attachTo: document.body })
      await flushPromises()

      wrapper.vm.openCreateModal()
      await flushPromises()
      await inBody('[data-test="plan-save-button"]').trigger('click')
      await flushPromises()
      expect(inBody('#plan-name-error').text()).toBe('Please enter a plan name')
      expect(inBody('[data-test="plan-name-input"]').attributes('aria-invalid')).toBe('true')
      expect(mockCreatePlan).not.toHaveBeenCalled()
      wrapper.vm.closePlanModal()

      wrapper.vm.openAssign(plan)
      await flushPromises()
      await inBody('[data-test="plan-assign-button"]').trigger('click')
      await flushPromises()
      expect(inBody('#plan-assign-user-error').text()).toBe('Please enter a user ID')
      expect(mockAssignPlanToUser).not.toHaveBeenCalled()

      await inBody('[data-test="plan-assign-user"]').setValue('7')
      await inBody('[data-test="plan-assign-button"]').trigger('click')
      await flushPromises()
      expect(mockAssignPlanToUser).toHaveBeenCalledWith(8, { user_id: 7, expire_at: null })
      expect(toastMessages('success')).toEqual(['Assigned successfully'])
    })

    it('removes a group from a plan at once and offers undo', async () => {
      mockGetPlanGroups.mockResolvedValue({ code: 0, data: [{ id: 3, name: 'HK' }] })
      mockRemoveGroupFromPlan.mockResolvedValue({ code: 0 })
      mockAddGroupToPlan.mockResolvedValue({ code: 0 })
      const wrapper = mount(Plans)
      await flushPromises()

      await wrapper.vm.removeGroup(plan, { id: 3, name: 'HK' })
      expect(mockRemoveGroupFromPlan).toHaveBeenCalledWith(8, 3)
      const [toast] = toasts('success')
      expect(toast.message).toBe('Group HK removed from plan Pro')
      await runAction(toast.id)
      expect(mockAddGroupToPlan).toHaveBeenCalledWith(8, 3)
    })

    it('toggles groups with keyboard-reachable buttons in the groups dialog', async () => {
      const user = userEvent.setup()
      mockGetSubscriptionGroups.mockResolvedValue({ code: 0, data: [{ id: 3, name: 'HK' }] })
      mockAddGroupToPlan.mockResolvedValue({ code: -1, msg: 'group locked' })
      render(Harness)
      await screen.findByText('Pro')
      await user.click(screen.getByRole('button', { name: 'Actions for Pro' }))
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Manage groups' }))
      const dialog = await screen.findByRole('dialog')
      const toggle = within(dialog).getByRole('button', { pressed: false })
      await user.click(toggle)
      expect(mockAddGroupToPlan).toHaveBeenCalledWith(8, 3)
      expect((await within(dialog).findByRole('alert')).textContent).toContain('group locked')
    })
      it('opens the plan details from a row, with its groups and a remove button per group', async () => {
      const user = userEvent.setup()
      mockGetPlanGroups.mockResolvedValue({ code: 0, data: [{ id: 3, name: 'HK' }] })
      mockRemoveGroupFromPlan.mockResolvedValue({ code: 0 })
      render(Harness)
      const row = (await screen.findByText('Pro')).closest('tr')
      expect(within(row).getByText('HK')).toBeTruthy()
      row.focus()
      await user.keyboard('{Enter}')
      const sheet = await screen.findByRole('dialog', { name: 'Pro' })
      expect(within(sheet).getByText('No speed limit / No device limit')).toBeTruthy()
      await user.click(within(sheet).getByRole('button', { name: 'Remove group HK' }))
      expect(mockRemoveGroupFromPlan).toHaveBeenCalledWith(8, 3)
    })

    it('shows a load error with retry and filters by name', async () => {
      const user = userEvent.setup()
      mockGetPlans.mockRejectedValueOnce(new Error('bad gateway'))
      render(Harness)
      const alert = await screen.findByRole('alert')
      expect(alert.textContent).toContain('Failed to load plans')
      await user.click(within(alert).getByRole('button', { name: 'Try again' }))
      await screen.findByText('Pro')
      await user.type(screen.getByRole('searchbox', { name: 'Search plans' }), 'zzz')
      expect(await screen.findByRole('heading', { name: 'No results' })).toBeTruthy()
    })
  })
})
