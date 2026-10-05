// Node list feedback (UI U4 → U7): the row "…" menu, a typed-name
// ConfirmDialog for deleting a node, the add/edit sheet with inline errors,
// and toasts for sync.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import Nodes from '@/views/admin/Nodes.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages, toasts } from './helpers/feedback'

const api = vi.hoisted(() => ({
  getNodes: vi.fn(),
  getNodeStats: vi.fn(),
  createNode: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
  getNodeCredentials: vi.fn(),
  syncNodeProtocol: vi.fn(),
  getAuthKeys: vi.fn(),
  generateAuthKey: vi.fn()
}))

vi.mock('@/api/admin', () => api)

// The Agent connection of the nodes in view is not what these tests are about.
vi.mock('@/api/kernel', async importOriginal => ({
  ...(await importOriginal()),
  getKernelAgentTransports: vi.fn(async () => ({ nodes: [] }))
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
    for (const fn of Object.values(api)) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    api.getNodes.mockResolvedValue({ data: { list: [node], total: 1 } })
    api.getNodeStats.mockResolvedValue({ data: { total: 1, online: 1, offline: 0, pending: 0 } })
    api.getNodeCredentials.mockResolvedValue({ data: { api_key: '' } })
    api.getAuthKeys.mockResolvedValue({ data: [] })
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

  it('validates the node sheet inline, saves and reports with a toast', async () => {
    const user = userEvent.setup()
    api.createNode.mockRejectedValueOnce({ response: { data: { error: 'duplicate name' } } }).mockResolvedValueOnce({ data: {} })
    await renderPage()

    const opener = screen.getByRole('button', { name: 'Add node' })
    await user.click(opener)
    const sheet = await screen.findByRole('dialog', { name: 'Add node' })
    expect(sheet.classList.contains('ui-sheet')).toBe(true)
    await user.click(within(sheet).getByRole('button', { name: 'Add node' }))
    expect(within(sheet).getAllByText('Required')).toHaveLength(2)
    expect(api.createNode).not.toHaveBeenCalled()

    await user.type(within(sheet).getByRole('textbox', { name: /Name/ }), 'sg-02')
    await user.type(within(sheet).getByRole('textbox', { name: /Address/ }), 'sg.example.test')
    await user.click(within(sheet).getByRole('button', { name: 'Add node' }))
    await waitFor(() => expect(within(sheet).getByRole('alert').textContent).toContain('duplicate name'))
    expect(screen.getByRole('dialog', { name: 'Add node' })).toBeTruthy()

    await user.click(within(sheet).getByRole('button', { name: 'Add node' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.createNode).toHaveBeenLastCalledWith({
      name: 'sg-02', host: 'sg.example.test', tags: '', rate: 1, sort: 0, parent_id: null, monthly_limit: null, monthly_reset_day: 1
    })
    expect(toastMessages('success')).toEqual(['Node sg-02 added'])
    await waitFor(() => expect(document.activeElement).toBe(opener))
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

  it('shows a failed key generation inside the registration key sheet and copies with a toast', async () => {
    const user = userEvent.setup()
    api.generateAuthKey.mockRejectedValueOnce(new Error('quota reached')).mockResolvedValueOnce({ code: 0, data: { id: 9, key: 'fresh-key' } })
    const clipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    const secure = Object.getOwnPropertyDescriptor(window, 'isSecureContext')
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
    try {
      await renderPage()
      await user.click(screen.getByRole('button', { name: 'Registration key' }))
      const sheet = await screen.findByRole('dialog', { name: 'Registration key' })
      await user.click(within(sheet).getByRole('button', { name: 'Generate key' }))
      expect((await within(sheet).findByRole('alert')).textContent).toContain('quota reached')

      await user.click(within(sheet).getByRole('button', { name: 'Generate key' }))
      await waitFor(() => expect(within(sheet).getByRole('button', { name: 'Copy key' })).toBeTruthy())
      await user.click(within(sheet).getByRole('button', { name: 'Copy key' }))
      expect(writeText).toHaveBeenCalledWith('fresh-key')
    } finally {
      if (clipboard) Object.defineProperty(navigator, 'clipboard', clipboard)
      else delete navigator.clipboard
      if (secure) Object.defineProperty(window, 'isSecureContext', secure)
      else delete window.isSecureContext
    }
  })
})
