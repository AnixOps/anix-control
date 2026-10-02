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
  assignAdminUserTunnel: vi.fn(),
  banUser: vi.fn(),
  createUser: vi.fn(),
  getAdminUser: vi.fn(),
  getAdminUserTunnelList: vi.fn(),
  getForwardTunnels: vi.fn(),
  getSpeedLimitList: vi.fn(),
  getSubscriptionGroups: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  getTrafficHourly: vi.fn(),
  getUserList: vi.fn(),
  getUserStats: vi.fn(),
  removeAdminUserTunnel: vi.fn(),
  resetUserSubscribe: vi.fn(),
  resetUserTraffic: vi.fn(),
  resetUserTunnelTraffic: vi.fn(),
  unbanUser: vi.fn(),
  updateAdminUserTunnel: vi.fn(),
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

function rowButton(name) {
  const row = screen.getByText('lin@example.test').closest('tr')
  return within(row).getByRole('button', { name })
}

describe('Users dialogs and feedback', () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset()
    api.getUserList.mockResolvedValue({ data: { list: [user], total: 1 } })
    api.getUserStats.mockResolvedValue({ data: {} })
    api.getSubscriptionGroups.mockResolvedValue({ data: [] })
    api.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    api.getAdminUser.mockResolvedValue({ code: 0, data: { ...user, token: 'tok' } })
    api.getForwardTunnels.mockResolvedValue({ code: 0, data: [{ id: 3, name: 'hk-relay' }] })
    api.getSpeedLimitList.mockResolvedValue({ code: 0, data: [] })
    api.getAdminUserTunnelList.mockResolvedValue({ code: 0, data: [{ id: 41, tunnelId: 3, tunnelName: 'hk-relay', status: 1, flow: 10 }] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('bans at once and offers undo in the toast', async () => {
    const userEv = userEvent.setup()
    api.banUser.mockResolvedValue({ code: 0 })
    api.unbanUser.mockResolvedValue({ code: 0 })
    await renderPage()

    await userEv.click(rowButton('Ban'))
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

    const opener = rowButton('Reset Sub')
    await userEv.click(opener)
    let dialog = await screen.findByRole('alertdialog', { name: 'Reset the subscription link of lin@example.test?' })
    // Danger: focus starts on Cancel.
    await waitFor(() => expect(document.activeElement.textContent.trim()).toBe('Cancel'))
    await userEv.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

    await userEv.click(opener)
    dialog = await screen.findByRole('alertdialog')
    await userEv.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.resetUserSubscribe).not.toHaveBeenCalled()

    await userEv.click(opener)
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

    await userEv.click(rowButton('Reset Sub'))
    const dialog = await screen.findByRole('alertdialog')
    await userEv.click(within(dialog).getByRole('button', { name: 'Reset link' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('user not found')
    expect(toasts()).toHaveLength(0)
  })

  it('edits a user in a dialog that closes with Esc and returns focus', async () => {
    const userEv = userEvent.setup()
    api.updateUser.mockResolvedValue({ code: 0 })
    await renderPage()

    const opener = rowButton('Edit')
    await userEv.click(opener)
    const dialog = await screen.findByRole('dialog', { name: 'Edit User' })
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    await userEv.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))

    await userEv.click(opener)
    await userEv.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.updateUser).toHaveBeenCalledWith(7, expect.objectContaining({ email: 'lin@example.test' }))
    expect(toastMessages('success')).toEqual(['lin@example.test saved'])
  })

  it('validates the tunnel grant form inline and confirms deleting a grant', async () => {
    const userEv = userEvent.setup()
    api.removeAdminUserTunnel.mockResolvedValue({ code: 0 })
    await renderPage()

    await userEv.click(rowButton('Tunnel'))
    const dialog = await screen.findByRole('dialog', { name: /lin@example.test/ })
    await within(dialog).findByText('hk-relay')
    await userEv.click(within(dialog).getByRole('button', { name: 'Add grant' }))
    expect(within(dialog).getByRole('alert').textContent).toBe('Please select a tunnel')

    await userEv.click(within(dialog).getByRole('button', { name: 'Delete' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Delete tunnel grant #41?' })
    await userEv.click(within(confirm).getByRole('button', { name: 'Delete grant' }))
    await waitFor(() => expect(api.removeAdminUserTunnel).toHaveBeenCalledWith({ id: 41 }))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Tunnel grant #41 deleted']))
    // The grants dialog is still open behind the confirmation.
    expect(screen.getByRole('dialog', { name: /lin@example.test/ })).toBeTruthy()
  })

  it('resets traffic through a confirmation that shows the usage', async () => {
    const userEv = userEvent.setup()
    api.resetUserTraffic.mockResolvedValueOnce({ code: -1, msg: 'busy' }).mockResolvedValueOnce({ code: 0 })
    await renderPage()

    await userEv.click(rowButton('Reset'))
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
      await userEv.click(rowButton('Copy Sub'))
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
    await userEv.click(rowButton('Traffic'))
    const sheet = await screen.findByRole('dialog', { name: 'Traffic Detail - lin@example.test' })
    expect(sheet.classList.contains('ui-sheet')).toBe(true)
    expect(await within(sheet).findAllByText('4.00 KB')).not.toHaveLength(0)
  })
})
