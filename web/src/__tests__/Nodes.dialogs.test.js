// Nodes page dialogs and feedback (UI U4): toasts instead of alert(), a
// typed-name ConfirmDialog for deleting a node, UiDialog/UiSheet instead of
// the hand-rolled overlays, and a template picker dialog instead of prompt().
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Nodes from '@/views/admin/Nodes.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages, toasts } from './helpers/feedback'

const api = vi.hoisted(() => ({
  getNodes: vi.fn(),
  getNodeStats: vi.fn(),
  getNodeLogs: vi.fn(),
  createNode: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
  getNodeCredentials: vi.fn(),
  syncNodeProtocol: vi.fn(),
  getNodeProtocols: vi.fn(),
  createNodeProtocol: vi.fn(),
  updateNodeProtocol: vi.fn(),
  deleteNodeProtocol: vi.fn(),
  getProtocolTemplates: vi.fn(),
  getAuthKeys: vi.fn(),
  generateAuthKey: vi.fn(),
  generateWireGuardKeypair: vi.fn()
}))

vi.mock('@/api/admin', () => api)

const Harness = {
  components: { Nodes, UiHost },
  template: '<div><Nodes /><UiHost /></div>'
}

const node = { id: 5, name: 'hk-01', host: 'hk.example.test', status: 1 }

async function renderPage() {
  const result = render(Harness)
  await screen.findByText('hk-01')
  return result
}

function rowButton(name) {
  const row = screen.getByText('hk-01').closest('tr')
  return within(row).getByRole('button', { name })
}

describe('Nodes dialogs and feedback', () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    api.getNodes.mockResolvedValue({ data: { list: [node], total: 1 } })
    api.getNodeStats.mockResolvedValue({ data: { total: 1, online: 1, offline: 0, pending: 0 } })
    api.getNodeLogs.mockResolvedValue({ data: { list: [], total: 0 } })
    api.getNodeCredentials.mockResolvedValue({ data: { api_key: '' } })
    api.getNodeProtocols.mockResolvedValue({ data: [{ id: 61, type: 'vless', port: 443, enable: 1 }] })
    api.getProtocolTemplates.mockResolvedValue({
      data: [
        { name: 'VLESS Reality', type: 'vless', default_port: 8443, tls: 2, transport: 'tcp' },
        { name: 'Trojan', type: 'trojan', default_port: 9443, tls: 1, transport: 'tcp' }
      ]
    })
    api.getAuthKeys.mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('deletes a node only after its name is typed; Esc and Cancel keep it', async () => {
    const user = userEvent.setup()
    api.deleteNode.mockResolvedValue({ data: {} })
    await renderPage()

    const opener = rowButton('Delete')
    await user.click(opener)
    let dialog = await screen.findByRole('alertdialog', { name: 'Delete node hk-01?' })
    const typed = within(dialog).getByRole('textbox')
    await waitFor(() => expect(document.activeElement).toBe(typed))
    const action = within(dialog).getByRole('button', { name: 'Delete node' })
    expect(action.disabled).toBe(true)
    await user.type(typed, 'hk-0')
    expect(action.disabled).toBe(true)
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))

    await user.click(opener)
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteNode).not.toHaveBeenCalled()

    await user.click(opener)
    dialog = await screen.findByRole('alertdialog')
    await user.type(within(dialog).getByRole('textbox'), 'hk-01')
    await user.click(within(dialog).getByRole('button', { name: 'Delete node' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteNode).toHaveBeenCalledWith(5)
    expect(toastMessages('success')).toEqual(['Node hk-01 deleted'])
  })

  it('keeps a failed node delete inside the confirmation', async () => {
    const user = userEvent.setup()
    api.deleteNode.mockRejectedValue({ response: { data: { error: 'node has forwards' } } })
    await renderPage()

    await user.click(rowButton('Delete'))
    const dialog = await screen.findByRole('alertdialog')
    await user.type(within(dialog).getByRole('textbox'), 'hk-01')
    await user.click(within(dialog).getByRole('button', { name: 'Delete node' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('node has forwards')
    expect(toasts()).toHaveLength(0)
  })

  it('validates the node form inline, saves and reports with a toast', async () => {
    const user = userEvent.setup()
    api.createNode.mockRejectedValueOnce({ response: { data: { error: 'duplicate name' } } }).mockResolvedValueOnce({ data: {} })
    await renderPage()

    const opener = screen.getByRole('button', { name: 'Add Node' })
    await user.click(opener)
    const dialog = await screen.findByRole('dialog', { name: 'Add Node' })
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(within(dialog).getByRole('alert').textContent).toBe('Please fill in the required fields')
    expect(api.createNode).not.toHaveBeenCalled()

    await user.type(within(dialog).getByPlaceholderText('Enter node name'), 'sg-02')
    await user.type(within(dialog).getByPlaceholderText('IP or domain'), 'sg.example.test')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(within(dialog).getByRole('alert').textContent).toContain('duplicate name'))
    expect(screen.getByRole('dialog', { name: 'Add Node' })).toBeTruthy()

    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.createNode).toHaveBeenLastCalledWith(expect.objectContaining({ name: 'sg-02', host: 'sg.example.test' }))
    expect(toastMessages('success')).toEqual(['Node sg-02 added'])
    await waitFor(() => expect(document.activeElement).toBe(opener))
  })

  it('reports sync results with toasts', async () => {
    const user = userEvent.setup()
    api.syncNodeProtocol.mockResolvedValueOnce({ data: {} }).mockRejectedValueOnce(new Error('agent offline'))
    await renderPage()

    await user.click(rowButton('Sync / Reload'))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Node "hk-01" accepted the sync operation']))
    await user.click(rowButton('Sync / Reload'))
    await waitFor(() => expect(toastMessages('error')).toEqual(['Sync failed: agent offline']))
  })

  it('manages protocols in a sheet and confirms deleting one', async () => {
    const user = userEvent.setup()
    api.deleteNodeProtocol.mockResolvedValue({ data: {} })
    await renderPage()

    await user.click(rowButton('Manage protocols'))
    const sheet = await screen.findByRole('dialog', { name: 'Protocol Management - hk-01' })
    expect(sheet.classList.contains('ui-sheet')).toBe(true)
    await within(sheet).findByText('VLESS')

    await user.click(within(sheet).getByRole('button', { name: 'Delete' }))
    let confirm = await screen.findByRole('alertdialog', { name: 'Delete protocol VLESS :443?' })
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteNodeProtocol).not.toHaveBeenCalled()

    await user.click(within(sheet).getByRole('button', { name: 'Delete' }))
    confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Delete protocol' }))
    await waitFor(() => expect(api.deleteNodeProtocol).toHaveBeenCalledWith(5, 61))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Protocol VLESS :443 deleted']))
    // The sheet stays open behind the confirmation.
    expect(screen.getByRole('dialog', { name: 'Protocol Management - hk-01' })).toBeTruthy()
  })

  it('shows invalid protocol JSON inline and loads a template through a picker dialog', async () => {
    const user = userEvent.setup()
    await renderPage()

    await user.click(rowButton('Manage protocols'))
    const sheet = await screen.findByRole('dialog', { name: 'Protocol Management - hk-01' })
    await user.click(within(sheet).getByRole('button', { name: /Add Protocol/ }))
    const form = await waitFor(() => screen.getAllByRole('dialog').find(el => !el.classList.contains('ui-sheet')))
    const editor = form.querySelector('.json-textarea')

    await user.clear(editor)
    await user.type(editor, 'not json')
    await user.click(within(form).getByRole('button', { name: 'Save' }))
    expect(within(form).getByRole('alert').textContent).toMatch(/^Invalid JSON: /)
    expect(api.createNodeProtocol).not.toHaveBeenCalled()

    await user.click(within(form).getByRole('button', { name: 'Load from Template' }))
    const picker = await screen.findByRole('dialog', { name: 'Load from template' })
    await user.click(within(picker).getByRole('button', { name: 'Load template' }))
    expect(within(picker).getByText('Choose a template to load.')).toBeTruthy()
    expect(within(picker).getByRole('combobox').getAttribute('aria-invalid')).toBe('true')
    expect(screen.getByRole('dialog', { name: 'Load from template' })).toBeTruthy()

    await user.click(within(picker).getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: 'Trojan' }))
    await user.click(within(picker).getByRole('button', { name: 'Load template' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Load from template' })).toBeNull())
    expect(JSON.parse(editor.value)).toMatchObject({ type: 'trojan', port: 9443, tls: 1 })
  })

  it('shows a failed key generation inside the auth key dialog and copies with a toast', async () => {
    const user = userEvent.setup()
    api.generateAuthKey.mockRejectedValueOnce(new Error('quota reached')).mockResolvedValueOnce({ code: 0, data: { id: 9, key: 'fresh-key' } })
    const clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    const secure = Object.getOwnPropertyDescriptor(window, 'isSecureContext')
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
    try {
      await renderPage()
      await user.click(screen.getByRole('button', { name: 'Auth Key' }))
      const dialog = await screen.findByRole('dialog', { name: 'Node Auth Key' })
      await user.click(within(dialog).getByRole('button', { name: 'Generate Key' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('quota reached')

      await user.click(within(dialog).getByRole('button', { name: 'Generate Key' }))
      await within(dialog).findByText('fresh-key')
      await user.click(within(dialog).getByRole('button', { name: 'Copy Key' }))
      expect(writeText).toHaveBeenCalledWith('fresh-key')
      await waitFor(() => expect(toastMessages('success')).toEqual(['Copied to clipboard']))
    } finally {
      if (clipboard) Object.defineProperty(navigator, 'clipboard', clipboard)
      else delete navigator.clipboard
      if (secure) Object.defineProperty(window, 'isSecureContext', secure)
      else delete window.isSecureContext
    }
  })

  it('opens the logs as a side sheet', async () => {
    const user = userEvent.setup()
    api.getNodeLogs.mockResolvedValue({ data: { list: [{ id: 1, level: 'error', source: 'xray', message: 'listen failed', created_at: 1790000000 }], total: 1 } })
    await renderPage()
    await user.click(rowButton('Logs'))
    const sheet = await screen.findByRole('dialog', { name: /hk-01/ })
    expect(sheet.classList.contains('ui-sheet')).toBe(true)
    expect(await within(sheet).findByText('listen failed')).toBeTruthy()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })
})
