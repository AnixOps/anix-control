// Users page dialogs and feedback (UI U4): toasts instead of alert(),
// ConfirmDialog instead of confirm(), UiDialog/UiSheet instead of the
// hand-rolled overlays.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Users from '@/views/admin/Users.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages, toasts } from './helpers/feedback'
import { runAction } from '@/ui/composables/useToast'

const api = vi.hoisted(() => ({
  banUser: vi.fn(),
  bulkUsers: vi.fn(),
  createUser: vi.fn(),
  getAdminUser: vi.fn(),
  getSubscriptionGroups: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  getTrafficHourly: vi.fn(),
  getUserList: vi.fn(),
  getUsersActivity: vi.fn(),
  getUserStats: vi.fn(),
  resetUserSubscribe: vi.fn(),
  resetUserTraffic: vi.fn(),
  unbanUser: vi.fn(),
  updateUser: vi.fn()
}))

vi.mock('@/api/admin', () => api)

const Harness = {
  components: { Users, UiHost },
  template: '<div><Users /><UiHost /></div>'
}

const user = { id: 7, email: 'lin@example.test', banned: 0, u: 1024, d: 1024, transfer_enable: 4096 }

async function renderPage() {
  const result = render(Harness)
  await screen.findByText('lin@example.test')
  return result
}

function menuButton(name = 'lin@example.test') {
  return screen.getByRole('button', { name: `Actions for ${name}` })
}

// Row actions live in the row's "…" menu (UI U6).
async function rowAction(userEv, name, row = 'lin@example.test') {
  const trigger = menuButton(row)
  await userEv.click(trigger)
  const menu = await screen.findByRole('menu')
  await userEv.click(within(menu).getByRole('menuitem', { name }))
  return trigger
}

describe('Users dialogs and feedback', () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset()
    api.getUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    api.getUserStats.mockResolvedValue({ data: {} })
    api.getUsersActivity.mockResolvedValue([])
    api.getSubscriptionGroups.mockResolvedValue({ data: [] })
    api.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    api.getAdminUser.mockResolvedValue({ code: 0, data: { ...user, token: 'tok' } })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('bans at once and offers undo in the toast', async () => {
    const userEv = userEvent.setup()
    api.banUser.mockResolvedValue({ code: 0 })
    api.unbanUser.mockResolvedValue({ code: 0 })
    await renderPage()

    await rowAction(userEv, 'Ban')
    await waitFor(() => expect(api.banUser).toHaveBeenCalledWith(7))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    await waitFor(() => expect(toastMessages('success')).toEqual(['lin@example.test banned']))

    const [toast] = toasts('success')
    expect(toast.action?.undo).toBe(true)
    await runAction(toast.id)
    expect(api.unbanUser).toHaveBeenCalledWith(7)
  })

  it('asks before resetting the subscription link; Cancel and Esc keep it', async () => {
    const userEv = userEvent.setup()
    api.resetUserSubscribe.mockResolvedValue({ code: 0 })
    await renderPage()

    const opener = await rowAction(userEv, 'Reset subscription link')
    let dialog = await screen.findByRole('alertdialog', { name: 'Reset the subscription link of lin@example.test?' })
    // Danger: focus starts on Cancel.
    await waitFor(() => expect(document.activeElement.textContent.trim()).toBe('Cancel'))
    await userEv.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

    await rowAction(userEv, 'Reset subscription link')
    dialog = await screen.findByRole('alertdialog')
    await userEv.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.resetUserSubscribe).not.toHaveBeenCalled()

    await rowAction(userEv, 'Reset subscription link')
    dialog = await screen.findByRole('alertdialog')
    await userEv.click(within(dialog).getByRole('button', { name: 'Reset link' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.resetUserSubscribe).toHaveBeenCalledWith(7)
    expect(toastMessages('success')).toEqual(['Subscription link reset'])
  })

  it('shows a failed subscription reset inside the confirmation', async () => {
    const userEv = userEvent.setup()
    api.resetUserSubscribe.mockResolvedValue({ code: -1, msg: 'user not found' })
    await renderPage()

    await rowAction(userEv, 'Reset subscription link')
    const dialog = await screen.findByRole('alertdialog')
    await userEv.click(within(dialog).getByRole('button', { name: 'Reset link' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('user not found')
    expect(toasts()).toHaveLength(0)
  })

  it('edits a user in a dialog that closes with Esc and returns focus', async () => {
    const userEv = userEvent.setup()
    api.updateUser.mockResolvedValue({ code: 0 })
    await renderPage()

    const opener = await rowAction(userEv, 'Edit user')
    const dialog = await screen.findByRole('dialog', { name: 'Edit User' })
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    await userEv.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))

    await rowAction(userEv, 'Edit user')
    await userEv.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.updateUser).toHaveBeenCalledWith(7, expect.objectContaining({ email: 'lin@example.test' }))
    expect(toastMessages('success')).toEqual(['lin@example.test saved'])
  })

  it('resets traffic through a confirmation that shows the usage', async () => {
    const userEv = userEvent.setup()
    api.resetUserTraffic.mockResolvedValueOnce({ code: -1, msg: 'busy' }).mockResolvedValueOnce({ code: 0 })
    await renderPage()

    await rowAction(userEv, 'Reset traffic')
    const dialog = await screen.findByRole('alertdialog', { name: 'Reset the traffic of lin@example.test?' })
    expect(dialog.textContent).toContain('2.00 KB')
    await userEv.click(within(dialog).getByRole('button', { name: 'Reset traffic' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('busy')
    await userEv.click(within(dialog).getByRole('button', { name: 'Reset traffic' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(toastMessages('success')).toEqual(['User traffic reset successfully'])
  })

  it('shows the link in a dialog when it cannot be copied', async () => {
    const userEv = userEvent.setup()
    const clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) }, configurable: true })
    const execCommand = document.execCommand
    document.execCommand = () => false
    try {
      await renderPage()
      await rowAction(userEv, 'Copy subscription link')
      const dialog = await screen.findByRole('dialog', { name: 'Copy subscription link' })
      expect(within(dialog).getByLabelText('Subscription link').value).toMatch(/\/s\/tok$/)
      expect(toasts()).toHaveLength(0)
    } finally {
      document.execCommand = execCommand
      if (clipboard) Object.defineProperty(navigator, 'clipboard', clipboard)
      else delete navigator.clipboard
    }
  })

  it('opens the traffic detail as a side sheet', async () => {
    const userEv = userEvent.setup()
    api.getTrafficHourly.mockResolvedValue({ code: 0, data: [{ hour_ts: 1783526400, traffic: 4096 }] })
    await renderPage()
    await rowAction(userEv, 'Traffic in the last 30 days')
    const sheet = await screen.findByRole('dialog', { name: 'Traffic Detail - lin@example.test' })
    expect(sheet.classList.contains('ui-sheet')).toBe(true)
    expect(await within(sheet).findAllByText('4.00 KB')).not.toHaveLength(0)
  })
  it('opens the detail sheet from a row with click or Enter', async () => {
    const userEv = userEvent.setup()
    await renderPage()
    const row = screen.getByText('lin@example.test').closest('tr')
    await userEv.click(within(row).getAllByRole('cell')[2])
    const sheet = await screen.findByRole('dialog', { name: 'lin@example.test' })
    expect(within(sheet).getByText('No speed limit / No device limit')).toBeTruthy()
    expect(within(sheet).getByRole('button', { name: 'Ban' })).toBeTruthy()
    await userEv.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    row.focus()
    await userEv.keyboard('{Enter}')
    expect(await screen.findByRole('dialog', { name: 'lin@example.test' })).toBeTruthy()
  })

  describe('bulk actions (POST /api/v4/admin/users/bulk)', () => {
    const other = { id: 8, email: 'wu@example.test', banned: 0, u: 0, d: 0, transfer_enable: 0 }
    const third = { id: 10, email: 'sun@example.test', banned: 0, u: 0, d: 0, transfer_enable: 0 }
    const banned = { id: 9, email: 'zhao@example.test', banned: 1, u: 0, d: 0, transfer_enable: 0 }
    const outcome = (action, results) => ({
      action,
      requested: results.length,
      succeeded: results.filter(item => item.ok).length,
      failed: results.filter(item => !item.ok).length,
      results
    })
    const ok = id => ({ id, ok: true })
    const refused = (id, code, message = '') => ({ id, ok: false, error: { code, message } })

    async function selectAll(userEv) {
      await userEv.click(screen.getByRole('checkbox', { name: 'Select all rows on this page' }))
      return screen.findByRole('region', { name: 'Actions for the selected rows' })
    }

    it('bans the selected users in one request and offers undo as one request', async () => {
      const userEv = userEvent.setup()
      api.getUserList.mockResolvedValue({ data: { list: [user, other, banned], total: 3 } })
      api.bulkUsers.mockResolvedValueOnce(outcome('ban', [ok(7), ok(8)])).mockResolvedValueOnce(outcome('unban', [ok(7), ok(8)]))
      await renderPage()
      const bar = await selectAll(userEv)
      expect(within(bar).getByText('3 selected')).toBeTruthy()
      await userEv.click(within(bar).getByRole('button', { name: 'Ban' }))
      await waitFor(() => expect(toastMessages('success')).toEqual(['2 users banned']))
      // The already banned user is not sent, and there is no per-user request.
      expect(api.bulkUsers.mock.calls).toEqual([['ban', [7, 8], {}]])
      expect(api.banUser).not.toHaveBeenCalled()
      await waitFor(() => expect(screen.queryByRole('region', { name: 'Actions for the selected rows' })).toBeNull())
      await runAction(toasts()[0].id)
      await waitFor(() => expect(api.bulkUsers.mock.calls[1]).toEqual(['unban', [7, 8], {}]))
      expect(api.unbanUser).not.toHaveBeenCalled()
    })

    it('says why a user was not banned, keeps those users selected and retries only what can change', async () => {
      const userEv = userEvent.setup()
      api.getUserList.mockResolvedValue({ data: { list: [user, other, third], total: 3 } })
      api.bulkUsers
        .mockResolvedValueOnce(outcome('ban', [ok(7), refused(8, 'not_found', 'user not found'), refused(10, 'package_route_frozen', 'the route is frozen')]))
        .mockResolvedValueOnce(outcome('ban', [ok(10)]))
      await renderPage()
      const bar = await selectAll(userEv)
      await userEv.click(within(bar).getByRole('button', { name: 'Ban' }))

      await waitFor(() => expect(toastMessages('warning')).toEqual(['Banned 1 of 3 users. 1 not found; 1 failed (the route is frozen).']))
      const [toast] = toasts('warning')
      // The warning stays until it is dismissed and offers one retry: the user that can still change.
      expect(toast.duration).toBe(0)
      expect(toast.action.label).toBe('Retry 1')
      // The two users that were not banned stay selected, the banned one is not.
      const after = await screen.findByRole('region', { name: 'Actions for the selected rows' })
      expect(within(after).getByText('2 selected')).toBeTruthy()

      await runAction(toast.id)
      await waitFor(() => expect(api.bulkUsers.mock.calls[1]).toEqual(['ban', [10], {}]))
      await waitFor(() => expect(toastMessages('success')).toEqual(['1 users banned']))
    })

    it('does not offer a retry for your own account or when nothing was banned', async () => {
      const userEv = userEvent.setup()
      api.getUserList.mockResolvedValue({ data: { list: [user, other], total: 2 } })
      api.bulkUsers.mockResolvedValueOnce(outcome('ban', [refused(7, 'forbidden_self', 'no'), refused(8, 'not_found', 'gone')]))
      await renderPage()
      const bar = await selectAll(userEv)
      await userEv.click(within(bar).getByRole('button', { name: 'Ban' }))
      await waitFor(() => expect(toastMessages('error')).toEqual(['No user was banned. 1 not found; your own account can’t be banned.']))
      expect(toasts('error')[0].action).toBeFalsy()
    })

    it('shows a failed bulk request as an error toast', async () => {
      const userEv = userEvent.setup()
      api.bulkUsers.mockRejectedValueOnce({ response: { status: 400, data: { error: { code: 'invalid_request', message: 'ids must be positive' } } } })
      await renderPage()
      const bar = await selectAll(userEv)
      await userEv.click(within(bar).getByRole('button', { name: 'Ban' }))
      await waitFor(() => expect(toastMessages('error')).toEqual(['The bulk request failed: ids must be positive']))
    })

    it('resets the traffic of the selected users after one confirmation, with one idempotency key', async () => {
      const userEv = userEvent.setup()
      api.getUserList.mockResolvedValue({ data: { list: [user, other], total: 2 } })
      api.bulkUsers.mockResolvedValueOnce(outcome('reset_traffic', [ok(7), ok(8)]))
      await renderPage()
      const bar = await selectAll(userEv)
      await userEv.click(within(bar).getByRole('button', { name: 'Reset traffic' }))
      const dialog = await screen.findByRole('alertdialog', { name: 'Reset the traffic of 2 users?' })
      expect(api.bulkUsers).not.toHaveBeenCalled()
      await userEv.click(within(dialog).getByRole('button', { name: 'Reset traffic' }))
      await waitFor(() => expect(toastMessages('success')).toEqual(['Traffic reset for 2 users']))
      expect(api.bulkUsers).toHaveBeenCalledTimes(1)
      expect(api.bulkUsers.mock.calls[0][0]).toBe('reset_traffic')
      expect(api.bulkUsers.mock.calls[0][1]).toEqual([7, 8])
      expect(api.bulkUsers.mock.calls[0][2].idempotencyKey).toMatch(/^ub-/)
      // A reset can't be undone: the toast has no 撤销.
      expect(toasts('success')[0].action).toBeFalsy()
    })

    it('keeps a failed reset request in the dialog and asks again with the same key', async () => {
      const userEv = userEvent.setup()
      api.getUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
      api.bulkUsers
        .mockRejectedValueOnce(Object.assign(new Error('timeout of 30000ms exceeded'), { code: 'ECONNABORTED' }))
        .mockResolvedValueOnce(outcome('reset_traffic', [ok(7)]))
      await renderPage()
      const bar = await selectAll(userEv)
      await userEv.click(within(bar).getByRole('button', { name: 'Reset traffic' }))
      const dialog = await screen.findByRole('alertdialog')
      await userEv.click(within(dialog).getByRole('button', { name: 'Reset traffic' }))
      await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toContain('timeout of 30000ms exceeded'))
      await userEv.click(within(dialog).getByRole('button', { name: 'Reset traffic' }))
      await waitFor(() => expect(toastMessages('success')).toEqual(['Traffic reset for 1 users']))
      const [first, second] = api.bulkUsers.mock.calls
      expect(first[2].idempotencyKey).toBe(second[2].idempotencyKey)
    })

    it('retries the users whose reset failed with the key of that click', async () => {
      const userEv = userEvent.setup()
      api.getUserList.mockResolvedValue({ data: { list: [user, other], total: 2 } })
      api.bulkUsers
        .mockResolvedValueOnce(outcome('reset_traffic', [ok(7), refused(8, 'not_attempted', 'ran out of time')]))
        .mockResolvedValueOnce(outcome('reset_traffic', [ok(8)]))
      await renderPage()
      const bar = await selectAll(userEv)
      await userEv.click(within(bar).getByRole('button', { name: 'Reset traffic' }))
      await userEv.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Reset traffic' }))
      await waitFor(() => expect(toastMessages('warning')).toEqual(['Reset the traffic of 1 of 2 users. 1 not attempted (out of time).']))
      await runAction(toasts('warning')[0].id)
      await waitFor(() => expect(api.bulkUsers).toHaveBeenCalledTimes(2))
      expect(api.bulkUsers.mock.calls[1][1]).toEqual([8])
      expect(api.bulkUsers.mock.calls[1][2].idempotencyKey).toBe(api.bulkUsers.mock.calls[0][2].idempotencyKey)
      await waitFor(() => expect(toastMessages('success')).toEqual(['Traffic reset for 1 users']))
    })
  })

  describe('last online (GET /api/v4/admin/users/activity)', () => {
    const nowSeconds = () => Math.floor(Date.now() / 1000)
    const other = { id: 8, email: 'wu@example.test', banned: 0, u: 0, d: 0, transfer_enable: 0 }

    it('asks for the ids of the page after the list and shows a relative time or Never', async () => {
      api.getUserList.mockResolvedValue({ data: { list: [user, other], total: 2 } })
      api.getUsersActivity.mockResolvedValue([{ user_id: 7, last_online_at: nowSeconds() - 120 }, { user_id: 8, last_online_at: null }])
      await renderPage()
      expect(screen.getByRole('columnheader', { name: 'Last online' })).toBeTruthy()
      await waitFor(() => expect(screen.getAllByTestId('last-online')).toHaveLength(1))
      expect(api.getUsersActivity).toHaveBeenCalledWith([7, 8])
      expect(screen.getByTestId('last-online').textContent).toBe('2 minutes ago')
      expect(screen.getByTestId('last-online').getAttribute('title')).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/)
      expect(screen.getByTestId('last-online-never').textContent).toBe('Never')
    })

    it('does not hold the list up while the activity is on its way', async () => {
      let release
      api.getUsersActivity.mockReturnValue(new Promise((resolve) => { release = resolve }))
      await renderPage()
      expect(screen.getByTestId('last-online-loading')).toBeTruthy()
      release([{ user_id: 7, last_online_at: nowSeconds() - 5 }])
      await waitFor(() => expect(screen.getByTestId('last-online').textContent).toBe('now'))
    })

    it('says so when the activity does not load, and leaves the list alone', async () => {
      vi.spyOn(console, 'error').mockImplementation(() => {})
      api.getUsersActivity.mockRejectedValue(new Error('Network Error'))
      await renderPage()
      await waitFor(() => expect(screen.getByTestId('last-online-unavailable').textContent).toBe('Unavailable'))
      expect(screen.getByText('lin@example.test')).toBeTruthy()
      expect(toastMessages('error')).toEqual([])
    })

    it('shows it in the user detail too', async () => {
      const userEv = userEvent.setup()
      api.getUsersActivity.mockResolvedValue([{ user_id: 7, last_online_at: nowSeconds() - 3 * 3600 }])
      await renderPage()
      await waitFor(() => expect(screen.getByTestId('last-online')).toBeTruthy())
      await userEv.click(screen.getByRole('row', { name: /lin@example.test/ }))
      const sheet = await screen.findByRole('dialog', { name: 'lin@example.test' })
      const row = within(sheet).getByTestId('user-last-online')
      expect(row.textContent).toContain('Last online')
      expect(row.textContent).toContain('3 hours ago')
    })
  })

  describe('sorting on the server', () => {
    it('sorts only the columns the API can sort by, from the first page', async () => {
      const userEv = userEvent.setup()
      await renderPage()
      for (const name of ['Email', 'Used / total', 'Expires at']) {
        expect(within(screen.getByRole('columnheader', { name: new RegExp(`^${name}`) })).getByRole('button')).toBeTruthy()
      }
      // The status is derived and the plan is another table: plain headers.
      for (const name of ['Status', 'Subscription template', 'Last online']) {
        expect(within(screen.getByRole('columnheader', { name })).queryByRole('button')).toBeNull()
      }
      api.getUserList.mockClear()
      await userEv.click(within(screen.getByRole('columnheader', { name: /^Used \/ total/ })).getByRole('button'))
      await waitFor(() => expect(api.getUserList).toHaveBeenCalledTimes(1))
      expect(api.getUserList).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, sort: 'traffic', order: 'desc' }))
      await userEv.click(within(screen.getByRole('columnheader', { name: /^Used \/ total/ })).getByRole('button'))
      await waitFor(() => expect(api.getUserList).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'traffic', order: 'asc' })))
      // The third click is no sort: the request carries neither parameter.
      await userEv.click(within(screen.getByRole('columnheader', { name: /^Used \/ total/ })).getByRole('button'))
      await waitFor(() => expect(api.getUserList.mock.calls.length).toBe(3))
      expect(api.getUserList.mock.calls[2][0]).not.toHaveProperty('sort')
      expect(api.getUserList.mock.calls[2][0]).not.toHaveProperty('order')
    })
  })

  it('filters by status on the server and "out of traffic" on the loaded page', async () => {
    const userEv = userEvent.setup()
    const full = { id: 8, email: 'full@example.test', banned: 0, u: 2048, d: 2048, transfer_enable: 4096 }
    api.getUserList.mockResolvedValue({ data: { list: [user, full], total: 2 } })
    api.getUserStats.mockResolvedValue({ data: { banned_users: 4 } })
    await renderPage()
    const chips = screen.getByRole('group', { name: 'Filter by status' })
    await userEv.click(within(chips).getByRole('button', { name: 'Banned 4' }))
    await waitFor(() => expect(api.getUserList).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'banned', page: 1 })))

    const calls = api.getUserList.mock.calls.length
    await userEv.click(within(chips).getByRole('button', { name: 'Out of traffic' }))
    await waitFor(() => expect(api.getUserList).toHaveBeenLastCalledWith(expect.objectContaining({ status: '' })))
    expect(api.getUserList.mock.calls.length).toBe(calls + 1)
    await waitFor(() => expect(screen.queryByText('lin@example.test')).toBeNull())
    expect(screen.getByText('full@example.test')).toBeTruthy()
    expect(screen.getByText(/narrows the users on this page only/)).toBeTruthy()
  })

  it('searches by email after a pause and on Enter', async () => {
    const userEv = userEvent.setup()
    await renderPage()
    const search = screen.getByRole('searchbox', { name: 'Search by email' })
    await userEv.type(search, 'lin{Enter}')
    await waitFor(() => expect(api.getUserList).toHaveBeenLastCalledWith(expect.objectContaining({ email: 'lin', page: 1 })))
  })

  it('shows a load error with retry', async () => {
    const userEv = userEvent.setup()
    api.getUserList.mockRejectedValueOnce(Object.assign(new Error('Network Error'), { response: { status: 502, data: { msg: 'bad gateway' } } }))
    render(Harness)
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Users didn’t load')
    expect(alert.textContent).toContain('bad gateway')
    await userEv.click(within(alert).getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('lin@example.test')).toBeTruthy()
  })
})
