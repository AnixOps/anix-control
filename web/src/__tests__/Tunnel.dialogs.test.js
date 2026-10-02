// Tunnel page dialogs (UI U4, U7): editor and diagnosis are Sheets, delete
// is a ConfirmDialog that shows a refused delete inline, row actions sit in
// the row's "…" menu. Visual swap only: the same API calls (flux-panel clone).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Tunnel from '@/views/admin/Tunnel.vue'
import UiHost from '@/ui/UiHost.vue'

const adminApi = vi.hoisted(() => ({
  createForwardTunnel: vi.fn(),
  deleteForwardTunnel: vi.fn(),
  diagnoseForwardTunnel: vi.fn(),
  getAdminForwardTunnelList: vi.fn(),
  getAnsibleMachines: vi.fn(),
  getForwardNodes: vi.fn(),
  getSystemConfig: vi.fn(),
  updateForwardTunnel: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

const Harness = {
  components: { Tunnel, UiHost },
  template: '<div><Tunnel /><UiHost /></div>'
}

async function renderPage() {
  const result = render(Harness, {
    global: { stubs: { 'router-link': { template: '<a><slot /></a>' } } }
  })
  await screen.findByText('hk-tunnel')
  return result
}

async function rowAction(user, name, row = 'hk-tunnel') {
  await user.click(screen.getByRole('button', { name: `Actions for ${row}` }))
  const menu = await screen.findByRole('menu')
  await user.click(within(menu).getByRole('menuitem', { name }))
}

describe('Tunnel dialogs', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    adminApi.getAdminForwardTunnelList.mockResolvedValue({
      code: 0,
      data: [{ id: 7, name: 'hk-tunnel', type: 1, inNodeId: 10, flow: 1, trafficRatio: 1 }]
    })
    adminApi.getAnsibleMachines.mockResolvedValue({ data: { list: [] } })
    adminApi.getForwardNodes.mockResolvedValue({ data: { list: [] } })
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.diagnoseForwardTunnel.mockResolvedValue({ code: 0, data: { results: [] } })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('opens the editor as a sheet that closes with Esc and returns focus', async () => {
    const user = userEvent.setup()
    await renderPage()
    const opener = screen.getAllByRole('button', { name: 'Create tunnel' })[0]
    await user.click(opener)
    const dialog = await screen.findByRole('dialog', { name: 'Create tunnel' })
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))
  })

  it('confirms a delete, keeps it on Cancel and Esc, and shows a refusal inline', async () => {
    const user = userEvent.setup()
    adminApi.deleteForwardTunnel
      .mockResolvedValueOnce({ code: -1, msg: 'tunnel in use' })
      .mockResolvedValueOnce({ code: 0 })
    await renderPage()

    await rowAction(user, 'Delete')
    let dialog = await screen.findByRole('alertdialog', { name: 'Delete tunnel hk-tunnel?' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

    await rowAction(user, 'Delete')
    await screen.findByRole('alertdialog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteForwardTunnel).not.toHaveBeenCalled()

    await rowAction(user, 'Delete')
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete tunnel' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('tunnel in use')

    await user.click(within(dialog).getByRole('button', { name: 'Delete tunnel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteForwardTunnel).toHaveBeenLastCalledWith(7)
  })

  it('shows diagnosis results in a sheet named after the tunnel', async () => {
    const user = userEvent.setup()
    await renderPage()
    await rowAction(user, 'Diagnose')
    const dialog = await screen.findByRole('dialog', { name: 'Tunnel Diagnosis Results' })
    expect(dialog.textContent).toContain('hk-tunnel')
    await waitFor(() => expect(adminApi.diagnoseForwardTunnel).toHaveBeenCalledWith(7))
  })

  it('lists tunnels in a table and shows a load failure with retry', async () => {
    const user = userEvent.setup()
    adminApi.getAdminForwardTunnelList
      .mockResolvedValueOnce({ code: -1, msg: 'tunnel store offline' })
      .mockResolvedValue({ code: 0, data: [{ id: 7, name: 'hk-tunnel', type: 1, inNodeId: 10, flow: 1, trafficRatio: 1 }] })
    render(Harness, { global: { stubs: { 'router-link': { template: '<a><slot /></a>' } } } })
    expect(await screen.findByText('tunnel store offline')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    const table = await screen.findByRole('table', { name: 'Tunnels' })
    expect(within(table).getByText('hk-tunnel')).toBeTruthy()
  })
})

