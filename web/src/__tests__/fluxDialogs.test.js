// Limit rules, NodeX forward nodes and Ansible machines (UI U4): the
// hand-rolled overlays became UiDialog and ConfirmDialog, the inline
// feedback banners became toasts. Fields, flows and API calls are unchanged
// (flux-panel clone pages: visual swap only).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import LimitI18n from '@/views/admin/LimitI18n.vue'
import ForwardNodesI18n from '@/views/admin/ForwardNodesI18n.vue'
import AnsibleMachines from '@/views/admin/AnsibleMachines.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages, toasts } from './helpers/feedback'

const api = vi.hoisted(() => ({
  // Limit rules
  createSpeedLimit: vi.fn(),
  deleteSpeedLimit: vi.fn(),
  getSpeedLimitList: vi.fn(),
  getSpeedLimitTunnels: vi.fn(),
  getSystemConfig: vi.fn(),
  updateSpeedLimit: vi.fn(),
  // NodeX forward nodes
  checkForwardNode: vi.fn(),
  createForwardNode: vi.fn(),
  createForwardRule: vi.fn(),
  deleteForwardNode: vi.fn(),
  deleteForwardRule: vi.fn(),
  getForwardNode: vi.fn(),
  getForwardNodes: vi.fn(),
  getForwardRule: vi.fn(),
  getForwardRules: vi.fn(),
  getForwardStats: vi.fn(),
  syncForwardNodeStats: vi.fn(),
  testForwardConnection: vi.fn(),
  toggleForwardNode: vi.fn(),
  toggleForwardRule: vi.fn(),
  updateForwardNode: vi.fn(),
  updateForwardRule: vi.fn(),
  // Ansible machines
  checkAnsibleMachine: vi.fn(),
  createAnsibleMachine: vi.fn(),
  deleteAnsibleMachine: vi.fn(),
  getAnsibleMachine: vi.fn(),
  getAnsibleMachines: vi.fn(),
  syncAnsibleMachineStats: vi.fn(),
  toggleAnsibleMachine: vi.fn(),
  updateAnsibleMachine: vi.fn()
}))

vi.mock('@/api/admin', () => api)

function harness(Page) {
  return {
    components: { Page, UiHost },
    template: '<div><Page /><UiHost /></div>'
  }
}

function renderPage(Page) {
  return render(harness(Page), { global: { stubs: { 'router-link': true } } })
}

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
  api.getSystemConfig.mockResolvedValue({ code: 0, data: { value: '' } })
  api.getSpeedLimitTunnels.mockResolvedValue({ code: 0, data: [{ id: 3, name: 'hk-tunnel' }] })
  api.getSpeedLimitList.mockResolvedValue({ code: 0, data: [{ id: 11, name: 'gold-100', speed: 100, tunnelId: 3, status: 1 }] })
  api.getForwardStats.mockResolvedValue({ code: 0, data: {} })
  api.getForwardNodes.mockResolvedValue({ code: 0, data: { list: [{ id: 5, name: 'hk-relay-01', type: 'relay', host: '203.0.113.5', status: 1 }], total: 1 } })
  api.getForwardRules.mockResolvedValue({ code: 0, data: { list: [{ id: 9, name: 'rule-a', protocol: 'tcp', relay_node_id: 5, listen_port: 9000 }], total: 1 } })
  api.getAnsibleMachines.mockResolvedValue({ data: { data: { list: [{ id: 1, name: 'relay-exec-01', host: '1.2.3.4', port: 22, enabled: true, status: 1 }] } } })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('Limit rules page', () => {
  it('creates a rule in a dialog with inline validation; Esc closes it', async () => {
    const user = userEvent.setup()
    api.createSpeedLimit.mockResolvedValue({ code: 0 })
    renderPage(LimitI18n)
    await screen.findByText('gold-100')

    const opener = screen.getAllByRole('button', { name: 'Create' })[0]
    await user.click(opener)
    let dialog = await screen.findByRole('dialog', { name: 'Create Limit Rule' })
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))

    await user.click(opener)
    dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Create Rule' }))
    expect(within(dialog).getByRole('alert').textContent).toBeTruthy()
    expect(api.createSpeedLimit).not.toHaveBeenCalled()

    await user.type(within(dialog).getByLabelText('Rule Name'), 'silver')
    await user.selectOptions(within(dialog).getByLabelText('Bound Tunnel'), '3')
    await user.click(within(dialog).getByRole('button', { name: 'Create Rule' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.createSpeedLimit).toHaveBeenCalledWith({ name: 'silver', speed: 100, tunnelId: 3, tunnelName: 'hk-tunnel' })
    expect(toastMessages('success')).toHaveLength(1)
  })

  it('deletes a rule after a confirmation; Cancel keeps it and failures stay inline', async () => {
    const user = userEvent.setup()
    api.deleteSpeedLimit.mockRejectedValueOnce({ response: { data: { msg: 'rule in use' } } }).mockResolvedValueOnce({ code: 0 })
    renderPage(LimitI18n)
    await screen.findByText('gold-100')

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    let confirm = await screen.findByRole('alertdialog', { name: 'Delete limit rule gold-100?' })
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteSpeedLimit).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Delete rule' }))
    expect((await within(confirm).findByRole('alert')).textContent).toContain('rule in use')
    expect(toasts()).toHaveLength(0)

    await user.click(within(confirm).getByRole('button', { name: 'Delete rule' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteSpeedLimit).toHaveBeenLastCalledWith(11)
    expect(toastMessages('success')).toHaveLength(1)
  })
})

describe('NodeX forward nodes page', () => {
  it('deletes a forward node only after its name is typed', async () => {
    const user = userEvent.setup()
    api.deleteForwardNode.mockResolvedValue({ code: 0 })
    renderPage(ForwardNodesI18n)
    const name = await screen.findByText('hk-relay-01')
    const card = name.closest('article') || name.parentElement.parentElement

    await user.click(within(card).getByRole('button', { name: 'Delete' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Delete forward node hk-relay-01?' })
    const action = within(confirm).getByRole('button', { name: 'Delete forward node' })
    expect(action.disabled).toBe(true)
    await user.type(within(confirm).getByRole('textbox'), 'hk-relay')
    expect(action.disabled).toBe(true)
    await user.type(within(confirm).getByRole('textbox'), '-01')
    expect(action.disabled).toBe(false)
    await user.click(action)
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteForwardNode).toHaveBeenCalledWith(5, { params: { scope: 'nodex' } })
    expect(toastMessages('success')).toHaveLength(1)
  })

  it('confirms deleting a rule without a typed name; Esc cancels', async () => {
    const user = userEvent.setup()
    api.deleteForwardRule.mockResolvedValue({ code: 0 })
    renderPage(ForwardNodesI18n)
    const row = (await screen.findByText('rule-a')).closest('tr')

    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    await screen.findByRole('alertdialog', { name: 'Delete forward rule rule-a?' })
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteForwardRule).not.toHaveBeenCalled()

    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    const confirm = await screen.findByRole('alertdialog')
    expect(within(confirm).queryByRole('textbox')).toBeNull()
    await user.click(within(confirm).getByRole('button', { name: 'Delete rule' }))
    await waitFor(() => expect(api.deleteForwardRule).toHaveBeenCalledWith(9))
  })

  it('opens the node editor and the connection test as dialogs', async () => {
    const user = userEvent.setup()
    renderPage(ForwardNodesI18n)
    await screen.findByText('hk-relay-01')

    await user.click(screen.getByRole('button', { name: 'Add Node' }))
    const editor = await screen.findByRole('dialog', { name: 'Add Relay/Exit Node' })
    expect(editor.getAttribute('aria-modal')).toBe('true')
    await user.click(within(editor).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    await user.click(screen.getAllByRole('button', { name: 'Test Connection' })[0])
    expect(await screen.findByRole('dialog', { name: 'Test Node Connection' })).toBeTruthy()
  })
})

describe('Ansible machines page', () => {
  it('deletes a machine only after its name is typed', async () => {
    const user = userEvent.setup()
    api.deleteAnsibleMachine.mockResolvedValue({})
    renderPage(AnsibleMachines)
    await screen.findByText('relay-exec-01')

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    const confirm = await screen.findByRole('alertdialog', { name: 'Delete Ansible machine relay-exec-01?' })
    const action = within(confirm).getByRole('button', { name: 'Delete machine' })
    expect(action.disabled).toBe(true)
    await user.type(within(confirm).getByRole('textbox'), 'relay-exec-01')
    await user.click(action)
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteAnsibleMachine).toHaveBeenCalledWith(1)
    expect(toastMessages('success')).toEqual(['relay-exec-01 deleted'])
  })

  it('keeps save errors inside the editor dialog', async () => {
    const user = userEvent.setup()
    api.createAnsibleMachine.mockRejectedValue({ response: { data: { msg: 'host exists' } } })
    renderPage(AnsibleMachines)
    await screen.findByText('relay-exec-01')

    await user.click(screen.getByRole('button', { name: 'Add Machine' }))
    const dialog = await screen.findByRole('dialog', { name: 'Add Ansible Machine' })
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(within(dialog).getByRole('alert').textContent).toContain('required')
    await user.type(within(dialog).getByLabelText('Name'), 'relay-exec-02')
    await user.type(within(dialog).getByLabelText('Host'), '5.6.7.8')
    await user.type(within(dialog).getByLabelText('Reachability Port'), '22')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect((await within(dialog).findByText('host exists')).getAttribute('role')).toBe('alert')
    expect(toasts()).toHaveLength(0)
  })
})
