import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import InviteCodes from '@/views/admin/InviteCodes.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'
import { resetEdition, setEdition } from '@/composables/useEdition'
import { runAction } from '@/ui/composables/useToast'
import { answerConfirms, inBody, toastMessages, toasts } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  bulkInviteCodes: vi.fn(),
  getInviteCodes: vi.fn(),
  generateInviteCodes: vi.fn(),
  revokeInviteCode: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

const panel = (data, code = 0, msg = '操作成功') => ({ code, msg, ts: 1, data })

const unused = { id: 3, code: 'a1b2c3d4', user_id: null, status: 0, used_by: null, expired_at: null, created_at: '2026-10-01T10:00:00Z' }
const used = { id: 2, code: 'e5f6a7b8', user_id: 7, status: 1, used_by: 9, expired_at: null, created_at: '2026-09-30T10:00:00Z' }
const expired = { id: 1, code: 'c9d0e1f2', user_id: null, status: 0, used_by: null, expired_at: '2020-01-01T00:00:00Z', created_at: '2019-12-01T10:00:00Z' }

const RouterLinkStub = { props: ['to'], template: '<a :data-to="to" v-bind="$attrs"><slot /></a>' }

function mountPage() {
  return mount(InviteCodes, {
    attachTo: document.body,
    global: { stubs: { 'router-link': RouterLinkStub } }
  })
}

function renderPage() {
  return render({ components: { InviteCodes, UiHost }, template: '<div><InviteCodes /><UiHost /></div>' }, {
    global: { stubs: { 'router-link': RouterLinkStub } }
  })
}

async function rowAction(user, code, name) {
  await user.click(screen.getByRole('button', { name: `Actions for ${code}` }))
  await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name }))
}

enableAutoUnmount(afterEach)

describe('Admin invite codes', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
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
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(3)
    expect(rows[0].text()).toContain('a1b2c3d4')
    expect(rows[0].text()).toContain('Administrator')
    expect(rows[0].text()).toContain('Unused')
    expect(rows[0].text()).toContain('Never')
    expect(rows[1].text()).toContain('User #7')
    expect(rows[1].text()).toContain('Used')
    expect(rows[1].text()).toContain('#9')
    expect(rows[2].text()).toContain('Expired')
    // No commission, withdrawal or statistics entry in community.
    expect(wrapper.find('[data-testid="invite-rewards-link"]').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/commission|withdraw/i)
    expect(wrapper.get('[data-testid="registration-hint"]').text()).toContain('requires an invite code')
  })

  it('offers revoke only for codes that were not used', async () => {
    const user = userEvent.setup()
    setEdition('community')
    renderPage()
    await screen.findByText('a1b2c3d4')
    await user.click(screen.getByRole('button', { name: 'Actions for a1b2c3d4' }))
    expect(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Revoke code…' })).toBeTruthy()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())
    await user.click(screen.getByRole('button', { name: 'Actions for e5f6a7b8' }))
    expect(within(await screen.findByRole('menu')).queryByRole('menuitem', { name: 'Revoke code…' })).toBeNull()
  })

  it('links the rewards page in the commercial edition', async () => {
    setEdition('commercial')
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-testid="invite-rewards-link"]').attributes('data-to')).toBe('/admin/invite')
    expect(wrapper.get('[data-testid="registration-hint"]').text()).toContain('does not require')
  })

  it('generates codes in a dialog and shows them', async () => {
    setEdition('community')
    adminApi.generateInviteCodes.mockResolvedValue(panel({ codes: [{ id: 4, code: 'feedbeef' }, { id: 5, code: 'deadbeef' }] }))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="open-generate"]').trigger('click')
    await flushPromises()
    await inBody('#invite-code-count').setValue(2)
    await inBody('#invite-code-expire').setValue(0)
    await inBody('[data-testid="generate-invite-codes"]').trigger('click')
    await flushPromises()

    expect(adminApi.generateInviteCodes).toHaveBeenCalledWith({ count: 2, expire_days: 0 })
    expect(inBody('[data-testid="generated-codes"]').text()).toContain('feedbeef')
    expect(inBody('[data-testid="generated-codes"]').text()).toContain('deadbeef')
    expect(adminApi.getInviteCodes).toHaveBeenCalledTimes(2)
    expect(toastMessages('success')).toContain('2 codes generated')
  })

  it('leaves the expiry to the configuration when empty and refuses a bad count inline', async () => {
    setEdition('community')
    adminApi.generateInviteCodes.mockResolvedValue(panel({ codes: [] }))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-testid="open-generate"]').trigger('click')
    await flushPromises()
    await inBody('[data-testid="generate-invite-codes"]').trigger('click')
    await flushPromises()
    expect(adminApi.generateInviteCodes).toHaveBeenCalledWith({ count: 1 })

    await inBody('#invite-code-count').setValue(51)
    await inBody('[data-testid="generate-invite-codes"]').trigger('click')
    await flushPromises()
    expect(adminApi.generateInviteCodes).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('Generate between 1 and 50 codes at a time.')
  })

  it('revokes an unused code after confirmation and reports a panel error', async () => {
    const user = userEvent.setup()
    setEdition('community')
    const confirms = answerConfirms(true)
    adminApi.revokeInviteCode
      .mockResolvedValueOnce(panel({ message: 'invite code revoked' }))
      .mockResolvedValueOnce(panel(null, -1, 'invite code already used'))
    renderPage()
    await screen.findByText('a1b2c3d4')

    await rowAction(user, 'a1b2c3d4', 'Revoke code…')
    await waitFor(() => expect(adminApi.revokeInviteCode).toHaveBeenCalledWith(3))
    expect(confirms.last()).toMatchObject({ tone: 'danger', title: 'Revoke invite code a1b2c3d4?' })
    await waitFor(() => expect(adminApi.getInviteCodes).toHaveBeenCalledTimes(2))

    await rowAction(user, 'a1b2c3d4', 'Revoke code…')
    await waitFor(() => expect(toastMessages('error')).toContain('Failed to revoke the code: invite code already used'))
  })

  describe('bulk revoke (POST /api/v4/admin/invite-codes/bulk)', () => {
    const answer = results => ({
      action: 'revoke',
      requested: results.length,
      succeeded: results.filter(item => item.ok).length,
      failed: results.filter(item => !item.ok).length,
      results
    })

    async function selectAllAndRevoke(user) {
      await screen.findByText('a1b2c3d4')
      await user.click(screen.getByRole('checkbox', { name: 'Select all rows on this page' }))
      const bar = await screen.findByRole('region', { name: 'Actions for the selected rows' })
      await user.click(within(bar).getByRole('button', { name: 'Revoke' }))
    }

    it('revokes the selected unused codes in one request after one confirmation', async () => {
      const user = userEvent.setup()
      setEdition('community')
      const confirms = answerConfirms(true)
      adminApi.bulkInviteCodes.mockResolvedValue(answer([{ id: 3, ok: true }, { id: 1, ok: true }]))
      renderPage()
      await selectAllAndRevoke(user)
      // The used code is not sent; there is no per-code request.
      await waitFor(() => expect(adminApi.bulkInviteCodes.mock.calls).toEqual([['revoke', [3, 1]]]))
      expect(adminApi.revokeInviteCode).not.toHaveBeenCalled()
      expect(confirms.last()).toMatchObject({ tone: 'danger', title: 'Revoke 2 invite codes?' })
      await waitFor(() => expect(toastMessages('success')).toContain('2 codes revoked'))
    })

    it('says why a code was kept, keeps it selected and retries only what can change', async () => {
      const user = userEvent.setup()
      setEdition('community')
      answerConfirms(true)
      adminApi.bulkInviteCodes
        .mockResolvedValueOnce(answer([{ id: 3, ok: false, error: { code: 'conflict', message: 'invite code already used' } }, { id: 1, ok: false, error: { code: 'failed', message: 'plugin host unavailable' } }]))
        .mockResolvedValueOnce(answer([{ id: 1, ok: true }]))
      renderPage()
      await selectAllAndRevoke(user)
      await waitFor(() => expect(toastMessages('error')).toEqual(['No code was revoked. 1 already used and kept; 1 failed (plugin host unavailable).']))
      const [toast] = toasts('error')
      expect(toast.action.label).toBe('Retry 1')
      // Both codes stay selected so the failed ones can be told apart.
      expect(within(await screen.findByRole('region', { name: 'Actions for the selected rows' })).getByText('2 selected')).toBeTruthy()
      await runAction(toast.id)
      await waitFor(() => expect(adminApi.bulkInviteCodes.mock.calls[1]).toEqual(['revoke', [1]]))
      await waitFor(() => expect(toastMessages('success')).toContain('1 codes revoked'))
    })

    it('shows a refused bulk request inside the confirmation', async () => {
      const user = userEvent.setup()
      setEdition('community')
      const confirms = answerConfirms(true)
      adminApi.bulkInviteCodes.mockRejectedValue({ response: { status: 400, data: { error: { code: 'invalid_request', message: 'ids must be positive' } } } })
      renderPage()
      await selectAllAndRevoke(user)
      await waitFor(() => expect(confirms.errors.map(error => error.message)).toEqual(['ids must be positive']))
      expect(toastMessages('success')).toEqual([])
    })
  })

  it('filters by status from the first page', async () => {
    const user = userEvent.setup()
    setEdition('community')
    renderPage()
    await screen.findByText('a1b2c3d4')
    await user.click(within(screen.getByRole('group', { name: 'Filter by status' })).getByRole('button', { name: 'Used' }))
    await waitFor(() => expect(adminApi.getInviteCodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, status: 'used' }))
  })

  it('shows a load error with retry', async () => {
    const user = userEvent.setup()
    setEdition('community')
    adminApi.getInviteCodes.mockRejectedValueOnce(new Error('bad gateway'))
    renderPage()
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Invite codes didn’t load')
    await user.click(within(alert).getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('a1b2c3d4')).toBeTruthy()
  })
})
