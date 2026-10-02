// Forward page dialogs (UI U4, U7): the editor and the diagnosis are
// Sheets, the delete, force-delete and bulk-delete questions are
// ConfirmDialogs, row actions sit in the row's "…" menu. Visual swap only:
// the same API calls in the same order (flux-panel clone).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createPinia, setActivePinia } from 'pinia'
import Forward from '@/views/admin/Forward.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

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

// Row actions live in the row's "…" menu (UI U7).
async function rowAction(user, name, row = 'Web Entry') {
  const trigger = screen.getByRole('button', { name: `Actions for ${row}` })
  await user.click(trigger)
  const menu = await screen.findByRole('menu')
  await user.click(within(menu).getByRole('menuitem', { name }))
  return trigger
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

  it('opens the editor as a sheet that closes with Esc and returns focus', async () => {
    const user = userEvent.setup()
    await renderPage()
    const opener = screen.getAllByRole('button', { name: 'Create' })[0]
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

    await rowAction(user, 'Delete')
    let dialog = await screen.findByRole('alertdialog', { name: 'Delete forward Web Entry?' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

    await rowAction(user, 'Delete')
    await screen.findByRole('alertdialog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteForward).not.toHaveBeenCalled()

    await rowAction(user, 'Delete')
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete forward' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteForward).toHaveBeenCalledWith(11)
    expect(adminApi.forceDeleteForward).not.toHaveBeenCalled()
    // The result is a toast above the page, not the old inline banner.
    await waitFor(() => expect(toastMessages('success')).toEqual(['Deleted successfully']))
  })

  it('asks a second time before a force delete, and shows its failure inline', async () => {
    const user = userEvent.setup()
    adminApi.deleteForward.mockResolvedValue({ code: -1, msg: 'node offline' })
    adminApi.forceDeleteForward
      .mockResolvedValueOnce({ code: -1, msg: 'force failed' })
      .mockResolvedValueOnce({ code: 0 })
    await renderPage()

    await rowAction(user, 'Delete')
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

    await user.click(within(container).getByRole('checkbox', { name: 'Select Web Entry' }))
    const bulkDelete = document.body.querySelector('[data-bulk-bar] [data-test="forward-bulk-delete"]')
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

  it('shows diagnosis results in a sheet named after the forward', async () => {
    const user = userEvent.setup()
    await renderPage()
    await rowAction(user, 'Diagnose')
    const dialog = await screen.findByRole('dialog', { name: 'Forward Diagnosis Results' })
    expect(dialog.textContent).toContain('Web Entry')
    await waitFor(() => expect(adminApi.diagnoseForward).toHaveBeenCalledWith(11))
    await user.click(within(dialog).getAllByRole('button', { name: 'Close' })[0])
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('moves a rule down from its row menu with the flux order payload', async () => {
    const user = userEvent.setup()
    adminApi.getForwardList.mockResolvedValue({ code: 0, data: [forward, { ...forward, id: 12, name: 'API Entry', inPort: 10002, inx: 2 }] })
    adminApi.updateForwardOrder.mockResolvedValue({ code: 0 })
    await renderPage()

    await rowAction(user, 'Move down')
    await waitFor(() => expect(adminApi.updateForwardOrder).toHaveBeenCalledWith({ forwards: [{ id: 12, inx: 0 }, { id: 11, inx: 1 }] }))
    // The first row cannot move up, the last one cannot move down.
    await user.click(screen.getByRole('button', { name: 'Actions for Web Entry' }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).getByRole('menuitem', { name: 'Move down' }).getAttribute('aria-disabled')).toBe('true')
  })

  it('shows the diagnosis as a timeline with a summary', async () => {
    const user = userEvent.setup()
    adminApi.diagnoseForward.mockResolvedValue({
      code: 0,
      data: {
        forwardName: 'Web Entry',
        timestamp: Date.UTC(2026, 9, 2, 6, 30),
        results: [
          { success: true, description: 'Panel to ingress', nodeName: 'hk-01', nodeId: 3, targetIp: '203.0.113.10', targetPort: 10001, averageTime: 18, packetLoss: 0 },
          { success: false, description: 'Ingress to target', nodeName: 'hk-01', nodeId: 3, targetIp: 'example.com', targetPort: 443, message: 'connection refused' }
        ]
      }
    })
    await renderPage()
    await rowAction(user, 'Diagnose')
    const sheet = await screen.findByRole('dialog', { name: 'Forward Diagnosis Results' })
    await waitFor(() => expect(within(sheet).getAllByRole('listitem')).toHaveLength(2))
    expect(within(sheet).getByRole('list', { name: 'Forward Diagnosis Results' })).toBeTruthy()
    expect(sheet.textContent).toContain('1 of 2 checks passed')
    expect(sheet.textContent).toContain('18 ms')
    expect(sheet.textContent).toContain('connection refused')
  })

  it('opens import and export from the page "…" menu', async () => {
    const user = userEvent.setup()
    await renderPage()
    await user.click(screen.getByRole('button', { name: 'More actions' }))
    await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Import' }))
    const dialog = await screen.findByRole('dialog', { name: 'Import Forward Data' })
    expect(within(dialog).getByRole('button', { name: 'Start Import' }).disabled).toBe(true)
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await user.click(screen.getByRole('button', { name: 'More actions' }))
    await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Export' }))
    expect(await screen.findByRole('dialog', { name: 'Export Forward Data' })).toBeTruthy()
  })

  it('shows a load failure as an error state and retries', async () => {
    const user = userEvent.setup()
    adminApi.getForwardList
      .mockResolvedValueOnce({ code: -1, msg: 'upstream unavailable' })
      .mockResolvedValue({ code: 0, data: [forward] })
    render(Harness, { global: { stubs: { 'router-link': { template: '<a><slot /></a>' } } } })
    expect(await screen.findByText('upstream unavailable')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    await waitFor(() => expect(screen.getAllByText('Web Entry').length).toBeGreaterThan(0))
    expect(adminApi.getForwardList).toHaveBeenCalledTimes(2)
  })

  it('switches between the direct and grouped views', async () => {
    const user = userEvent.setup()
    const { container } = await renderPage()
    await user.click(screen.getByRole('button', { name: 'Grouped' }))
    await waitFor(() => expect(container.querySelector('[data-test="forward-grouped-view"]')).not.toBeNull())
    expect(screen.getByRole('heading', { name: 'Unknown User' })).toBeTruthy()
    expect(localStorage.getItem('forward-view-mode')).toBe('grouped')
    await user.click(screen.getByRole('button', { name: 'Direct' }))
    await waitFor(() => expect(container.querySelector('[data-test="forward-direct-view"]')).not.toBeNull())
  })

  // The tooltip itself is checked in a browser; jsdom has no pointer events
  // for Reka's tooltip. The detail is also in the trigger's accessible name.
  it('shows one status badge per row, the worst state, with the detail in a tooltip', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        { ...forward, runtimeBackend: 'nftables_ansible', runtimeStatus: 2 },
        { ...forward, id: 12, name: 'Broken Entry', inx: 2, runtimeBackend: 'nftables_ansible', runtimeStatus: 3, runtimeMessage: 'ansible apply failed: host unreachable' }
      ]
    })
    const { container } = await renderPage()
    const rows = container.querySelectorAll('tbody tr')
    const badges = row => [...row.querySelectorAll('.ui-badge')].map(badge => badge.textContent.trim())
    expect(badges(rows[0])).toEqual(['Applied'])
    expect(badges(rows[1])).toEqual(['Sync Failed'])
    const detail = within(rows[1]).getByRole('button', { name: /Sync Failed/ })
    expect(detail.textContent).toContain('ansible apply failed: host unreachable')
  })
})

