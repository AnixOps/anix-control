// System page dialogs and feedback (UI U4): config and balancer editors are
// UiDialogs with inline errors; deletes and restores ask in a ConfirmDialog.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import System from '@/views/admin/System.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  deleteSystemConfig: vi.fn(),
  getSystemConfig: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  setSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getSystemAuditLogs: vi.fn(),
  getBackupConfig: vi.fn(),
  updateBackupConfig: vi.fn(),
  createBackup: vi.fn(),
  getBackups: vi.fn(),
  getBackupStats: vi.fn(),
  deleteBackup: vi.fn(),
  restoreBackup: vi.fn(),
  getLoadBalancers: vi.fn(),
  createLoadBalancer: vi.fn(),
  updateLoadBalancer: vi.fn(),
  deleteLoadBalancer: vi.fn(),
  runHealthCheck: vi.fn(),
  listForwardRuntimeJobs: vi.fn(),
  getForwardRuntimeStatus: vi.fn(),
  runForwardRuntimeDoctor: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

const Harness = { components: { System, UiHost }, template: '<div><System /><UiHost /></div>' }

async function renderPage() {
  render(Harness, { global: { stubs: { 'router-link': true } } })
  await screen.findByText('site.name')
}

describe('System dialogs and feedback', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [{ key: 'site.name', value: 'AnixOps', description: 'Site name' }] } })
    adminApi.getSystemAuditLogs.mockResolvedValue({ data: { data: { list: [], total: 0, page: 1, page_size: 20 } } })
    adminApi.getBackupConfig.mockResolvedValue({ data: {} })
    adminApi.getBackups.mockResolvedValue({ data: { list: [] } })
    adminApi.getBackupStats.mockResolvedValue({ data: {} })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
    adminApi.listForwardRuntimeJobs.mockResolvedValue({ data: { list: [] } })
    adminApi.getForwardRuntimeStatus.mockResolvedValue({ data: { data: null } })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('edits a config in a dialog: errors inline, Esc closes and returns focus', async () => {
    const user = userEvent.setup()
    adminApi.setSystemConfig.mockResolvedValueOnce({ code: -1, msg: 'read only' }).mockResolvedValueOnce({ code: 0 })
    await renderPage()
    const row = screen.getByText('site.name').closest('tr')
    const opener = within(row).getByRole('button', { name: 'Edit' })
    await user.click(opener)
    const dialog = await screen.findByRole('dialog', { name: 'Edit Config' })
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('read only')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))

    await user.click(opener)
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(toastMessages('success')).toEqual(['Config site.name saved'])
  })

  it('asks before deleting a config; Cancel keeps it', async () => {
    const user = userEvent.setup()
    adminApi.deleteSystemConfig.mockResolvedValue({ code: 0 })
    await renderPage()
    const row = screen.getByText('site.name').closest('tr')
    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    let confirm = await screen.findByRole('alertdialog', { name: 'Delete config site.name?' })
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteSystemConfig).not.toHaveBeenCalled()

    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Delete config' }))
    await waitFor(() => expect(adminApi.deleteSystemConfig).toHaveBeenCalledWith('site.name'))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Config site.name deleted']))
  })

  it('validates balancer weights inside the dialog', async () => {
    const user = userEvent.setup()
    await renderPage()
    await user.click(screen.getByRole('button', { name: 'Load Balancers' }))
    await user.click(screen.getByRole('button', { name: 'Create Balancer' }))
    const dialog = await screen.findByRole('dialog', { name: /Balancer/i })
    await user.type(within(dialog).getByLabelText(/Name/), 'edge')
    await user.type(within(dialog).getByLabelText(/Weights/), 'not json')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(within(dialog).getByRole('alert').textContent).toBe('Weights JSON is invalid')
    expect(adminApi.createLoadBalancer).not.toHaveBeenCalled()
  })
})
