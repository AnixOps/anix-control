import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import InviteCodes from '@/views/admin/InviteCodes.vue'
import { setLocale } from '@/i18n'
import { resetEdition, setEdition } from '@/composables/useEdition'

const adminApi = vi.hoisted(() => ({
  getInviteCodes: vi.fn(),
  generateInviteCodes: vi.fn(),
  revokeInviteCode: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

const panel = (data, code = 0, msg = '操作成功') => ({ code, msg, ts: 1, data })

const unused = { id: 3, code: 'a1b2c3d4', user_id: null, status: 0, used_by: null, expired_at: null, created_at: '2026-10-01T10:00:00Z' }
const used = { id: 2, code: 'e5f6a7b8', user_id: 7, status: 1, used_by: 9, expired_at: null, created_at: '2026-09-30T10:00:00Z' }
const expired = { id: 1, code: 'c9d0e1f2', user_id: null, status: 0, used_by: null, expired_at: '2020-01-01T00:00:00Z', created_at: '2019-12-01T10:00:00Z' }

function mountPage() {
  return mount(InviteCodes, {
    global: {
      stubs: {
        'router-link': { props: ['to'], template: '<a :data-to="to" v-bind="$attrs"><slot /></a>' }
      }
    }
  })
}

describe('Admin invite codes', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
    adminApi.getInviteCodes.mockResolvedValue(panel({ list: [unused, used, expired], total: 3, page: 1, page_size: 20 }))
  })

  afterEach(() => {
    resetEdition()
    vi.restoreAllMocks()
  })

  it('lists codes in the community edition without the rewards page', async () => {
    setEdition('community', { requireInvite: true })
    const wrapper = mountPage()
    await flushPromises()

    expect(adminApi.getInviteCodes).toHaveBeenCalledWith({ page: 1, page_size: 20 })
    const rows = wrapper.findAll('[data-testid="invite-code-row"]')
    expect(rows).toHaveLength(3)
    expect(rows[0].text()).toContain('a1b2c3d4')
    expect(rows[0].text()).toContain('Administrator')
    expect(rows[0].text()).toContain('Unused')
    expect(rows[0].text()).toContain('Never')
    expect(rows[1].text()).toContain('User #7')
    expect(rows[1].text()).toContain('Used')
    expect(rows[1].text()).toContain('#9')
    expect(rows[2].text()).toContain('Expired')
    // Only an unused (or expired) code can be revoked.
    expect(rows[0].find('[data-testid="revoke-invite-code"]').exists()).toBe(true)
    expect(rows[1].find('[data-testid="revoke-invite-code"]').exists()).toBe(false)
    // No commission, withdrawal or statistics entry in community.
    expect(wrapper.find('[data-testid="invite-rewards-link"]').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/commission|withdraw/i)
    expect(wrapper.get('[data-testid="registration-hint"]').text()).toContain('requires an invite code')
  })

  it('links the rewards page in the commercial edition', async () => {
    setEdition('commercial')
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-testid="invite-rewards-link"]').attributes('data-to')).toBe('/admin/invite')
    expect(wrapper.get('[data-testid="registration-hint"]').text()).toContain('does not require')
  })

  it('generates codes and shows them', async () => {
    setEdition('community')
    adminApi.generateInviteCodes.mockResolvedValue(panel({ codes: [{ id: 4, code: 'feedbeef' }, { id: 5, code: 'deadbeef' }] }))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('#invite-code-count').setValue(2)
    await wrapper.get('#invite-code-expire').setValue(0)
    await wrapper.get('[data-testid="generate-invite-codes"]').trigger('click')
    await flushPromises()

    expect(adminApi.generateInviteCodes).toHaveBeenCalledWith({ count: 2, expire_days: 0 })
    expect(wrapper.get('[data-testid="generated-codes"]').text()).toContain('feedbeef')
    expect(wrapper.get('[data-testid="generated-codes"]').text()).toContain('deadbeef')
    expect(adminApi.getInviteCodes).toHaveBeenCalledTimes(2)
  })

  it('leaves the expiry to the configuration when empty and refuses a bad count', async () => {
    setEdition('community')
    const alert = vi.spyOn(window, 'alert').mockImplementation(() => {})
    adminApi.generateInviteCodes.mockResolvedValue(panel({ codes: [] }))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="generate-invite-codes"]').trigger('click')
    await flushPromises()
    expect(adminApi.generateInviteCodes).toHaveBeenCalledWith({ count: 1 })

    await wrapper.get('#invite-code-count').setValue(51)
    await wrapper.get('[data-testid="generate-invite-codes"]').trigger('click')
    await flushPromises()
    expect(adminApi.generateInviteCodes).toHaveBeenCalledTimes(1)
    expect(alert).toHaveBeenCalledWith('Generate between 1 and 50 codes at a time.')
  })

  it('revokes an unused code after confirmation and reports a panel error', async () => {
    setEdition('community')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const alert = vi.spyOn(window, 'alert').mockImplementation(() => {})
    adminApi.revokeInviteCode
      .mockResolvedValueOnce(panel({ message: 'invite code revoked' }))
      .mockResolvedValueOnce(panel(null, -1, 'invite code already used'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.findAll('[data-testid="revoke-invite-code"]')[0].trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenCalled()
    expect(adminApi.revokeInviteCode).toHaveBeenCalledWith(3)
    expect(adminApi.getInviteCodes).toHaveBeenCalledTimes(2)

    await wrapper.findAll('[data-testid="revoke-invite-code"]')[0].trigger('click')
    await flushPromises()
    expect(alert).toHaveBeenCalledWith('Failed to revoke the code: invite code already used')
  })

  it('filters by status from the first page', async () => {
    setEdition('community')
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="invite-code-filter"]').setValue('used')
    await flushPromises()
    expect(adminApi.getInviteCodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, status: 'used' })
  })
})
