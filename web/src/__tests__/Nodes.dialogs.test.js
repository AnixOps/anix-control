// Node list feedback (UI U4 → U7): the row "…" menu, a typed-name
// ConfirmDialog for deleting a node, the edit sheet with inline errors,
// the confirmation of rejecting a node and toasts for sync.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import Nodes from '@/views/admin/Nodes.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages, toasts } from './helpers/feedback'

const api = vi.hoisted(() => ({
  getNodeStats: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
  getNodeCredentials: vi.fn(),
  syncNodeProtocol: vi.fn()
}))

vi.mock('@/api/admin', () => api)

// The Agent connection of the nodes in view is not what these tests are
// about; the list and the groups are.
const kernel = vi.hoisted(() => ({
  getKernelAdminNodes: vi.fn(),
  getKernelAgentTransports: vi.fn(),
  listKernelNodeGroups: vi.fn(),
  approveKernelNode: vi.fn(),
  rejectKernelNode: vi.fn()
}))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernel }))
vi.mock('@/api/machineTelemetry', async importOriginal => ({
  ...(await importOriginal()),
  getNodesServicesSummary: vi.fn(async () => [])
}))

const Harness = {
  components: { Nodes, UiHost },
  template: '<div><Nodes /><UiHost /></div>'
}

const node = { id: 5, name: 'hk-01', host: 'hk.example.test', status: 1, rate: 1, sort: 0, monthly_reset_day: 1 }

async function renderPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/nodes', component: Harness }, { path: '/admin/nodes/:id', component: { template: '<div />' } }]
  })
  await router.push('/admin/nodes')
  await router.isReady()
  const result = render(Harness, { global: { plugins: [router] } })
  await screen.findAllByText('hk-01')
  return { ...result, router }
}

async function rowAction(user, name) {
  const trigger = screen.getByRole('button', { name: 'Actions for hk-01' })
  await user.click(trigger)
  await user.click(await screen.findByRole('menuitem', { name }))
  return trigger
}

describe('Nodes dialogs and feedback', () => {
  beforeEach(() => {
    for (const fn of [...Object.values(api), ...Object.values(kernel)]) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    kernel.getKernelAdminNodes.mockResolvedValue({ list: [node], total: 1 })
    kernel.getKernelAgentTransports.mockResolvedValue({ nodes: [] })
    kernel.listKernelNodeGroups.mockResolvedValue({ groups: [] })
    api.getNodeStats.mockResolvedValue({ data: { total: 1, online: 1, offline: 0, pending: 0 } })
    api.getNodeCredentials.mockResolvedValue({ data: { api_key: '' } })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('deletes a node only after its name is typed; Esc and Cancel keep it', async () => {
    const user = userEvent.setup()
    api.deleteNode.mockResolvedValue({ data: {} })
    await renderPage()

    const opener = await rowAction(user, 'Delete')
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

    await rowAction(user, 'Delete')
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(api.deleteNode).not.toHaveBeenCalled()

    await rowAction(user, 'Delete')
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

    await rowAction(user, 'Delete')
    const dialog = await screen.findByRole('alertdialog')
    await user.type(within(dialog).getByRole('textbox'), 'hk-01')
    await user.click(within(dialog).getByRole('button', { name: 'Delete node' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('node has forwards')
    expect(toasts()).toHaveLength(0)
  })

  it('validates the edit sheet inline, keeps a failed save in it and reports a save with a toast', async () => {
    const user = userEvent.setup()
    api.updateNode.mockRejectedValueOnce({ response: { data: { error: 'duplicate name' } } }).mockResolvedValueOnce({ data: {} })
    await renderPage()

    const opener = await rowAction(user, 'Edit')
    const sheet = await screen.findByRole('dialog', { name: 'Edit node' })
    expect(sheet.classList.contains('ui-sheet')).toBe(true)
    const name = within(sheet).getByRole('textbox', { name: /Name/ })
    await user.clear(name)
    await user.click(within(sheet).getByRole('button', { name: 'Save changes' }))
    expect(within(sheet).getAllByText('Required')).toHaveLength(1)
    expect(api.updateNode).not.toHaveBeenCalled()

    await user.type(name, 'sg-02')
    await user.click(within(sheet).getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(within(sheet).getByRole('alert').textContent).toContain('duplicate name'))
    expect(screen.getByRole('dialog', { name: 'Edit node' })).toBeTruthy()

    await user.click(within(sheet).getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.updateNode).toHaveBeenLastCalledWith(5, {
      name: 'sg-02', host: 'hk.example.test', tags: '', rate: 1, sort: 0, parent_id: null, monthly_limit: null, monthly_reset_day: 1, status: 1
    })
    expect(toastMessages('success')).toEqual(['Node sg-02 saved'])
    await waitFor(() => expect(document.activeElement).toBe(opener))
  })

  it('has no way to add a node by hand: the header has one entry, Join a node', async () => {
    await renderPage()
    expect(screen.queryByRole('button', { name: 'Add node' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Registration key' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Deploy parent nodes' })).toBeNull()
    expect(screen.getAllByRole('button', { name: 'Join a node' })).toHaveLength(1)
  })

  it('asks before rejecting a node that waits, then rejects it', async () => {
    const user = userEvent.setup()
    kernel.getKernelAdminNodes.mockResolvedValue({ list: [{ ...node, status: 0, approval: 'pending' }], total: 1 })
    kernel.rejectKernelNode.mockResolvedValue({ node: 'proxy-5', approval: 'rejected' })
    await renderPage()

    await rowAction(user, 'Reject')
    let dialog = await screen.findByRole('alertdialog', { name: 'Reject node hk-01?' })
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(kernel.rejectKernelNode).not.toHaveBeenCalled()

    await rowAction(user, 'Reject')
    dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Reject' }))
    await waitFor(() => expect(kernel.rejectKernelNode).toHaveBeenCalledWith('proxy-5'))
    await waitFor(() => expect(toastMessages('success')).toEqual(['hk-01 rejected']))
  })

  it('edits a node from the row menu with the status field', async () => {
    const user = userEvent.setup()
    api.updateNode.mockResolvedValue({ data: {} })
    await renderPage()

    await rowAction(user, 'Edit')
    const sheet = await screen.findByRole('dialog', { name: 'Edit node' })
    expect(within(sheet).getByRole('textbox', { name: /Name/ }).value).toBe('hk-01')
    await user.click(within(sheet).getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(api.updateNode).toHaveBeenCalledWith(5, expect.objectContaining({ name: 'hk-01', host: 'hk.example.test', status: 1 })))
    expect(toastMessages('success')).toEqual(['Node hk-01 saved'])
  })

  it('reports sync results with toasts', async () => {
    const user = userEvent.setup()
    api.syncNodeProtocol.mockResolvedValueOnce({ data: {} }).mockRejectedValueOnce(new Error('agent offline'))
    await renderPage()

    await rowAction(user, 'Sync and reload')
    await waitFor(() => expect(toastMessages('success')).toEqual(['Node “hk-01” accepted the sync']))
    expect(api.syncNodeProtocol).toHaveBeenCalledWith(5)
    await rowAction(user, 'Sync and reload')
    await waitFor(() => expect(toastMessages('error')).toEqual(['Sync failed: agent offline']))
  })

  it('opens the protocols and logs sections of the node page from the row menu', async () => {
    const user = userEvent.setup()
    const { router } = await renderPage()
    await rowAction(user, 'Manage protocols')
    await waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/admin/nodes/5?section=protocols'))
  })
})
