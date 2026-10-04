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
  createUser: vi.fn(),
  getAdminUser: vi.fn(),
  getSubscriptionGroups: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  getTrafficHourly: vi.fn(),
  getUserList: vi.fn(),
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

  it('bans the selected users from the bulk bar, one request each, with undo', async () => {
    const userEv = userEvent.setup()
    const other = { id: 8, email: 'wu@example.test', banned: 0, u: 0, d: 0, transfer_enable: 0 }
    const banned = { id: 9, email: 'zhao@example.test', banned: 1, u: 0, d: 0, transfer_enable: 0 }
    api.getUserList.mockResolvedValue({ data: { list: [user, other, banned], total: 3 } })
    api.banUser.mockResolvedValue({ code: 0 })
    api.unbanUser.mockResolvedValue({ code: 0 })
    await renderPage()
    await userEv.click(screen.getByRole('checkbox', { name: 'Select all rows on this page' }))
    const bar = await screen.findByRole('region', { name: 'Actions for the selected rows' })
    expect(within(bar).getByText('3 selected')).toBeTruthy()
    await userEv.click(within(bar).getByRole('button', { name: 'Ban' }))
    await waitFor(() => expect(toastMessages('success')).toEqual(['2 users banned']))
    // The already banned user is skipped.
    expect(api.banUser.mock.calls).toEqual([[7], [8]])
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Actions for the selected rows' })).toBeNull())
    await runAction(toasts()[0].id)
    await waitFor(() => expect(api.unbanUser.mock.calls).toEqual([[7], [8]]))
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
