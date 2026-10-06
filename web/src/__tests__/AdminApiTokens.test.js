// Security → API tokens (views/admin/security/ApiTokens.vue): the table of
// tokens, its states, the chips for ended and everyone's tokens, the
// revoke confirmation, and the limit of 25. Creating a token is
// apiTokenCreate.test.js.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import ApiTokens from '@/views/admin/security/ApiTokens.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'
import { createFormatter } from '@/ui/composables/useFormat'
import { useUserStore } from '@/stores/user'
import { toastMessages } from './helpers/feedback'

const kernelApi = vi.hoisted(() => ({
  listKernelApiTokens: vi.fn(),
  revokeKernelApiToken: vi.fn(),
  createKernelApiToken: vi.fn()
}))
const userApi = vi.hoisted(() => ({ getMfaStatus: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)
vi.mock('@/api/user', () => userApi)

const NOW = Date.UTC(2026, 9, 5, 12, 0, 0)
const DAY = 86_400_000
const iso = offset => new Date(NOW + offset).toISOString()
// Exact times are shown in the viewer's time zone, whatever the test machine's is.
const clock = offset => createFormatter('en').dateTime(NOW + offset)

function row(id, extra = {}) {
  return {
    id: `tok-${id}`, user_id: 7, name: `token ${id}`, scope: 'read', hint: `h${id}`.padEnd(4, 'x'),
    expires_at: iso(60 * DAY), last_used_at: null, created_at: iso(-3 * DAY), revoked_at: null, ...extra
  }
}

const ACTIVE = row(1, { name: 'nightly export', hint: 'k3Zq', last_used_at: iso(-3 * 3600_000), last_used_ip: '203.0.113.9' })
const ADMIN = row(2, { name: 'deploy bot', scope: 'admin', expires_at: iso(3 * DAY) })
const FOREVER = row(3, { name: 'archive probe', expires_at: null })
const EXPIRED = row(4, { name: 'last year', expires_at: iso(-2 * DAY) })
const REVOKED = row(5, { name: 'leaked one', revoked_at: iso(-DAY), revoke_reason: 'owner_revoked' })
const BANNED = row(6, { name: 'ex-admin', user_id: 9, owner_email: 'gone@example.com', revoked_at: iso(-DAY), revoke_reason: 'owner_not_admin' })

function refusal(status, code, message) {
  return { response: { status, data: { error: { code, message } } }, message: `Request failed with status code ${status}` }
}

const Harness = {
  components: { ApiTokens, UiHost },
  template: '<div><ApiTokens /><UiHost /></div>'
}

describe('Admin API tokens', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(NOW)
    vi.spyOn(console, 'error').mockImplementation(() => {})
    localStorage.clear()
    setActivePinia(createPinia())
    useUserStore().login('jwt', { id: 7, email: 'admin@example.com' })
    await setLocale('en')
    kernelApi.listKernelApiTokens.mockResolvedValue([ACTIVE, ADMIN, FOREVER])
    userApi.getMfaStatus.mockResolvedValue({ code: 0, data: { enabled: false } })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  async function renderPage() {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const result = render(Harness)
    await screen.findByRole('heading', { level: 2, name: 'API tokens' })
    await waitFor(() => expect(kernelApi.listKernelApiTokens).toHaveBeenCalled())
    return { user, ...result }
  }

  const rowOf = name => screen.getByRole('row', { name: new RegExp(name) })

  it('lists the caller\'s active tokens: name and last four characters, scope, status, expiry both ways, last use and address', async () => {
    await renderPage()
    expect(kernelApi.listKernelApiTokens).toHaveBeenCalledWith({ all: false, includeInactive: false })
    await screen.findByRole('row', { name: /nightly export/ })
    const headers = screen.getAllByRole('columnheader').map(header => header.textContent.trim()).filter(Boolean)
    expect(headers).toEqual(['Name', 'Scope', 'Status', 'Expires', 'Last used', 'Actions'])

    const export_ = rowOf('nightly export')
    expect(export_.textContent).toContain('anixadm_…k3Zq')
    // When it was created: the date, the exact time on hover.
    expect(within(export_).getByText(/^Created 20\d\d-\d\d-\d\d$/).getAttribute('title')).toBe(clock(-3 * DAY))
    expect(within(export_).getByText('Read')).toBeTruthy()
    expect(within(export_).getByText('Active')).toBeTruthy()
    // Expiry: relative, with the exact time under it.
    expect(within(export_).getByText('in 2 months')).toBeTruthy()
    expect(within(export_).getByText(clock(60 * DAY))).toBeTruthy()
    // Last use: when and from where.
    expect(within(export_).getByText('3 hours ago')).toBeTruthy()
    expect(within(export_).getByText('203.0.113.9')).toBeTruthy()

    const bot = rowOf('deploy bot')
    expect(within(bot).getByText('Admin')).toBeTruthy()
    expect(within(bot).getByText('Expires soon')).toBeTruthy()
    expect(within(bot).getByText('in 3 days')).toBeTruthy()
    expect(within(bot).getByText('Never used')).toBeTruthy()

    const probe = rowOf('archive probe')
    expect(within(probe).getByText('No expiry')).toBeTruthy()
    // No owner column while only the caller's tokens are listed.
    expect(screen.queryByRole('columnheader', { name: 'Owner' })).toBeNull()
  })

  it('shows the quota and has no token in the page', async () => {
    await renderPage()
    expect((await screen.findByTestId('api-tokens-quota')).textContent).toBe('3 of 25 active tokens in use.')
    expect(document.body.textContent).not.toMatch(/anixadm_[A-Za-z0-9_-]{20}/)
  })

  it('shows ended tokens when asked, with Expired and Revoked and why', async () => {
    kernelApi.listKernelApiTokens.mockResolvedValue([ACTIVE, EXPIRED, REVOKED])
    const { user } = await renderPage()
    await user.click(screen.getByRole('button', { name: 'Revoked and expired', pressed: false }))
    await waitFor(() => expect(kernelApi.listKernelApiTokens).toHaveBeenLastCalledWith({ all: false, includeInactive: true }))
    const expired = await screen.findByRole('row', { name: /last year/ })
    expect(within(expired).getByText('Expired')).toBeTruthy()
    expect(within(expired).getByText('Expired 2 days ago')).toBeTruthy()
    const revoked = rowOf('leaked one')
    expect(within(revoked).getByText('Revoked')).toBeTruthy()
    expect(within(revoked).getByText('Revoked yesterday')).toBeTruthy()
    expect(within(revoked).getByText('Revoked by its owner')).toBeTruthy()
    // They do not count against the limit.
    expect(screen.getByTestId('api-tokens-quota').textContent).toBe('1 of 25 active tokens in use.')
    // An ended token cannot be revoked again: its menu has only the ID.
    await user.click(screen.getByRole('button', { name: 'Actions for leaked one' }))
    const items = within(await screen.findByRole('menu')).getAllByRole('menuitem').map(item => item.textContent.trim())
    expect(items).toEqual(['Copy token ID'])
  })

  it('lists everyone\'s tokens with an owner column, and says whose each is', async () => {
    kernelApi.listKernelApiTokens.mockResolvedValue([ACTIVE, BANNED])
    const { user } = await renderPage()
    await user.click(screen.getByRole('button', { name: 'All administrators', pressed: false }))
    await waitFor(() => expect(kernelApi.listKernelApiTokens).toHaveBeenLastCalledWith({ all: true, includeInactive: false }))
    await screen.findByRole('columnheader', { name: 'Owner' })
    expect(within(rowOf('nightly export')).getByText('You')).toBeTruthy()
    const banned = rowOf('ex-admin')
    expect(within(banned).getByText('gone@example.com')).toBeTruthy()
    expect(within(banned).getByText('Revoked: its owner was banned, demoted or deleted')).toBeTruthy()
    // Only the caller's own active token counts toward the limit.
    expect(screen.getByTestId('api-tokens-quota').textContent).toBe('1 of 25 active tokens in use.')
  })

  it('replaces the all-administrators chip with a sentence when the route says super_admin_required', async () => {
    kernelApi.listKernelApiTokens
      .mockResolvedValueOnce([ACTIVE])
      .mockRejectedValueOnce(refusal(403, 'super_admin_required', 'only a super administrator may list other administrators\' API tokens'))
      .mockResolvedValueOnce([ACTIVE])
    const { user } = await renderPage()
    await user.click(screen.getByRole('button', { name: 'All administrators', pressed: false }))
    expect((await screen.findByTestId('api-tokens-super-only')).textContent).toBe('Only super administrators can list other administrators’ tokens.')
    expect(screen.queryByRole('button', { name: 'All administrators' })).toBeNull()
    expect(screen.getByRole('row', { name: /nightly export/ })).toBeTruthy()
    expect(screen.queryByRole('columnheader', { name: 'Owner' })).toBeNull()
  })

  it('says what will appear and offers to create, when there are no tokens', async () => {
    kernelApi.listKernelApiTokens.mockResolvedValue([])
    const { user } = await renderPage()
    expect(await screen.findByRole('heading', { level: 3, name: 'No API tokens' })).toBeTruthy()
    expect(screen.getByText(/shown once, when you create it/)).toBeTruthy()
    await user.click(screen.getByTestId('api-tokens-empty-create'))
    expect(await screen.findByRole('dialog', { name: 'Create an API token' })).toBeTruthy()
  })

  it('shows the load error with Try again, and loads again on it', async () => {
    kernelApi.listKernelApiTokens.mockRejectedValueOnce(refusal(503, 'database_unavailable', 'the database is unavailable'))
    const { user } = await renderPage()
    expect(await screen.findByText('Couldn’t load the API tokens')).toBeTruthy()
    expect(screen.queryByTestId('api-tokens-quota')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByRole('row', { name: /nightly export/ })).toBeTruthy()
    expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(2)
  })

  it('turns Create off, and says why, at 25 active tokens', async () => {
    kernelApi.listKernelApiTokens.mockResolvedValue(Array.from({ length: 25 }, (_, index) => row(index + 10)))
    await renderPage()
    expect((await screen.findByTestId('api-tokens-quota')).textContent).toBe('You hold the maximum of 25 active tokens. Revoke one to create another.')
    expect(screen.getByTestId('api-token-create-open').disabled).toBe(true)
  })

  it('refreshes the list after a token is created', async () => {
    kernelApi.createKernelApiToken.mockResolvedValue({
      token: 'anixadm_TestTokenNotRealAbCdEfGhIjKlMnOpQrStUvWxYz0',
      api_token: row(8, { name: 'brand new' })
    })
    const { user } = await renderPage()
    await user.click(screen.getByTestId('api-token-create-open'))
    const form = await screen.findByRole('dialog', { name: 'Create an API token' })
    await user.type(within(form).getByRole('textbox', { name: /^Name/ }), 'brand new')
    await user.type(await within(form).findByLabelText(/Current password/), 'pw')
    kernelApi.listKernelApiTokens.mockResolvedValue([ACTIVE, row(8, { name: 'brand new' })])
    await user.click(within(form).getByRole('button', { name: 'Create token' }))
    await screen.findByRole('dialog', { name: 'Your new API token' })
    await waitFor(() => expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(2))
    // The result dialog is still open over the page.
    expect(await screen.findByRole('row', { name: /brand new/, hidden: true })).toBeTruthy()
  })

  describe('revoking', () => {
    async function openRevoke(user, name) {
      await user.click(screen.getByRole('button', { name: `Actions for ${name}` }))
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Revoke token…' }))
      return screen.findByRole('alertdialog', { name: `Revoke “${name}”?` })
    }

    it('asks first, names the token and what happens, and sends nothing until the danger button', async () => {
      const { user } = await renderPage()
      const dialog = await openRevoke(user, 'nightly export')
      expect(dialog.textContent).toMatch(/stops working at once/)
      expect(dialog.textContent).toMatch(/can’t be undone/)
      expect(kernelApi.revokeKernelApiToken).not.toHaveBeenCalled()
      // Cancel has the focus; the button says what it does.
      expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: 'Cancel' }))
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(kernelApi.revokeKernelApiToken).not.toHaveBeenCalled()
    })

    it('revokes by id, says so, and reloads the list', async () => {
      kernelApi.revokeKernelApiToken.mockResolvedValue({ api_token: REVOKED, changed: true })
      const { user } = await renderPage()
      const dialog = await openRevoke(user, 'nightly export')
      kernelApi.listKernelApiTokens.mockResolvedValue([ADMIN, FOREVER])
      await user.click(within(dialog).getByRole('button', { name: 'Revoke token' }))
      await waitFor(() => expect(kernelApi.revokeKernelApiToken).toHaveBeenCalledWith('tok-1'))
      await waitFor(() => expect(screen.queryByRole('row', { name: /nightly export/ })).toBeNull())
      expect(toastMessages('success')).toEqual(['Revoked “nightly export”.'])
      expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(2)
    })

    it('says so when it was already revoked', async () => {
      kernelApi.revokeKernelApiToken.mockResolvedValue({ api_token: REVOKED, changed: false })
      const { user } = await renderPage()
      const dialog = await openRevoke(user, 'nightly export')
      await user.click(within(dialog).getByRole('button', { name: 'Revoke token' }))
      await waitFor(() => expect(toastMessages('success')).toEqual(['“nightly export” was already revoked.']))
    })

    it('shows a failure inside the confirmation and keeps it open', async () => {
      kernelApi.revokeKernelApiToken.mockRejectedValueOnce(refusal(500, 'database_error', 'database operation failed'))
      const { user } = await renderPage()
      const dialog = await openRevoke(user, 'nightly export')
      await user.click(within(dialog).getByRole('button', { name: 'Revoke token' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('Couldn’t revoke the token: database operation failed')
      expect(screen.getByRole('alertdialog')).toBeTruthy()
      expect(toastMessages('success')).toEqual([])
    })

    it('says the token is gone on a 404 and refreshes the list', async () => {
      kernelApi.revokeKernelApiToken.mockRejectedValueOnce(refusal(404, 'not_found', 'the API token does not exist or is not yours'))
      const { user } = await renderPage()
      const dialog = await openRevoke(user, 'nightly export')
      await user.click(within(dialog).getByRole('button', { name: 'Revoke token' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('This token no longer exists, or it isn’t yours. The list has been refreshed.')
      await waitFor(() => expect(kernelApi.listKernelApiTokens).toHaveBeenCalledTimes(2))
    })

    it('names the owner when a super administrator revokes someone else\'s token', async () => {
      kernelApi.listKernelApiTokens.mockResolvedValue([ACTIVE, row(7, { name: 'their probe', user_id: 9 })])
      const { user } = await renderPage()
      await user.click(screen.getByRole('button', { name: 'All administrators', pressed: false }))
      await screen.findByRole('columnheader', { name: 'Owner' })
      const dialog = await openRevoke(user, 'their probe')
      expect(dialog.textContent).toContain('It belongs to administrator #9.')
    })
  })

  it('copies a token\'s ID for the audit log, and the ID is all it copies', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    const { user } = await renderPage()
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    await user.click(screen.getByRole('button', { name: 'Actions for nightly export' }))
    await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Copy token ID' }))
    expect(writeText).toHaveBeenCalledWith('tok-1')
    await waitFor(() => expect(toastMessages('success')).toEqual(['Token ID copied.']))
  })

  it('sorts by name and by expiry in the browser, with no expiry last when ascending', async () => {
    const { user } = await renderPage()
    await screen.findByRole('row', { name: /nightly export/ })
    const names = () => screen.getAllByRole('row').slice(1).map(tr => tr.querySelector('.api-token-name__text')?.textContent)
    await user.click(screen.getByRole('button', { name: /^Name/ }))
    expect(names()).toEqual(['archive probe', 'deploy bot', 'nightly export'])
    await user.click(screen.getByRole('button', { name: /^Expires/ }))
    expect(names()).toEqual(['deploy bot', 'nightly export', 'archive probe'])
    await user.click(screen.getByRole('button', { name: /^Expires/ }))
    expect(names()).toEqual(['archive probe', 'nightly export', 'deploy bot'])
  })
})
