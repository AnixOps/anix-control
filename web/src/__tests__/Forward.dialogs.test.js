// Forward page dialogs (UI U4): the hand-rolled overlays are UiDialogs and
// the delete, force-delete and bulk-delete questions are ConfirmDialogs.
// Visual swap only: the same API calls in the same order (flux-panel clone).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import Forward from '@/views/admin/Forward.vue'
import UiHost from '@/ui/UiHost.vue'

const adminApi = vi.hoisted(() => ({
  createForward: vi.fn(),
  getForwardList: vi.fn(),
  updateForward: vi.fn(),
  deleteForward: vi.fn(),
  forceDeleteForward: vi.fn(),
  pauseForwardService: vi.fn(),
  resumeForwardService: vi.fn(),
  diagnoseForward: vi.fn(),
  updateForwardOrder: vi.fn(),
  getForwardTunnels: vi.fn(),
  getSystemConfig: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

const Harness = {
  components: { Forward, UiHost },
  template: '<div><Forward /><UiHost /></div>'
}

const forward = {
  id: 11,
  userId: 1,
  name: 'Web Entry',
  tunnelId: 1,
  tunnelName: 'Port Tunnel',
  inIp: '203.0.113.10',
  inPort: 10001,
  remoteAddr: 'example.com:443',
  status: 1,
  inx: 1
}

async function renderPage() {
  const result = render(Harness, {
    global: { stubs: { 'router-link': { template: '<a><slot /></a>' } } }
  })
  await waitFor(() => expect(screen.getAllByText('Web Entry').length).toBeGreaterThan(0))
  return result
}

function firstButton(name) {
  return screen.getAllByRole('button', { name })[0]
}

describe('Forward dialogs', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.resetAllMocks()
    localStorage.clear()
    adminApi.getForwardList.mockResolvedValue({ code: 0, data: [forward] })
    adminApi.getForwardTunnels.mockResolvedValue({ code: 0, data: [{ id: 1, name: 'Port Tunnel', type: 1, inNodePortSta: 1000, inNodePortEnd: 2000 }] })
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.diagnoseForward.mockResolvedValue({ code: 0, data: { results: [] } })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('opens the editor as a dialog that closes with Esc and returns focus', async () => {
    const user = userEvent.setup()
    await renderPage()
    const opener = screen.getByRole('button', { name: 'Create' })
    await user.click(opener)
    const dialog = await screen.findByRole('dialog', { name: 'Create Forward' })
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(within(dialog).getByRole('button', { name: 'Create Forward' })).toBeTruthy()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))
  })

  it('deletes after the danger confirmation; Cancel and Esc keep the forward', async () => {
    const user = userEvent.setup()
    adminApi.deleteForward.mockResolvedValue({ code: 0 })
    await renderPage()

    await user.click(firstButton('Delete'))
    let dialog = await screen.findByRole('alertdialog', { name: 'Delete forward Web Entry?' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

    await user.click(firstButton('Delete'))
    await screen.findByRole('alertdialog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteForward).not.toHaveBeenCalled()

    await user.click(firstButton('Delete'))
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete forward' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteForward).toHaveBeenCalledWith(11)
    expect(adminApi.forceDeleteForward).not.toHaveBeenCalled()
  })

  it('asks a second time before a force delete, and shows its failure inline', async () => {
    const user = userEvent.setup()
    adminApi.deleteForward.mockResolvedValue({ code: -1, msg: 'node offline' })
    adminApi.forceDeleteForward
      .mockResolvedValueOnce({ code: -1, msg: 'force failed' })
      .mockResolvedValueOnce({ code: 0 })
    await renderPage()

    await user.click(firstButton('Delete'))
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete forward Web Entry?' })

    // Declining the force delete leaves the first dialog open, nothing forced.
    await user.click(within(dialog).getByRole('button', { name: 'Delete forward' }))
    let force = await screen.findByRole('alertdialog', { name: 'Force delete Web Entry?' })
    expect(force.textContent).toContain('node offline')
    await user.click(within(force).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog', { name: 'Force delete Web Entry?' })).toBeNull())
    expect(adminApi.forceDeleteForward).not.toHaveBeenCalled()
    expect(screen.getByRole('alertdialog', { name: 'Delete forward Web Entry?' })).toBeTruthy()

    // A failed force delete shows the reason in the delete dialog.
    await user.click(within(dialog).getByRole('button', { name: 'Delete forward' }))
    force = await screen.findByRole('alertdialog', { name: 'Force delete Web Entry?' })
    await user.click(within(force).getByRole('button', { name: 'Force delete' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('force failed')

    await user.click(within(dialog).getByRole('button', { name: 'Delete forward' }))
    force = await screen.findByRole('alertdialog', { name: 'Force delete Web Entry?' })
    await user.click(within(force).getByRole('button', { name: 'Force delete' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.forceDeleteForward).toHaveBeenCalledTimes(2)
    expect(adminApi.forceDeleteForward).toHaveBeenLastCalledWith(11)
  })

  it('confirms a bulk delete of the selected forwards', async () => {
    const user = userEvent.setup()
    adminApi.deleteForward.mockResolvedValue({ code: 0 })
    const { container } = await renderPage()

    const selector = container.querySelector('[data-test="forward-row-select"]')
    await user.click(selector)
    const bulkDelete = container.querySelector('[data-test="forward-bulk-delete"]')
    await user.click(bulkDelete)
    let dialog = await screen.findByRole('alertdialog', { name: 'Delete 1 selected forwards?' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteForward).not.toHaveBeenCalled()

    await user.click(bulkDelete)
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete forwards' }))
    await waitFor(() => expect(adminApi.deleteForward).toHaveBeenCalledWith(11))
  })

  it('shows diagnosis results in a dialog named after the forward', async () => {
    const user = userEvent.setup()
    await renderPage()
    await user.click(firstButton('Diagnose'))
    const dialog = await screen.findByRole('dialog', { name: 'Forward Diagnosis Results' })
    expect(dialog.textContent).toContain('Web Entry')
    await waitFor(() => expect(adminApi.diagnoseForward).toHaveBeenCalledWith(11))
    await user.click(within(dialog).getAllByRole('button', { name: 'Close' })[0])
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })
})
