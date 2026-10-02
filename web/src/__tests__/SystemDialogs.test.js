// System settings dialogs and feedback (UI U4, U7): the configuration key
// and load balancer editors are UiDialogs with inline errors, opened from
// the table's row menu; deletes ask in a ConfirmDialog; the subscription
// domains are validated as you type and saved from the save bar.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import SettingsBalancer from '@/views/admin/system/SettingsBalancer.vue'
import SettingsGeneral from '@/views/admin/system/SettingsGeneral.vue'
import UiHost from '@/ui/UiHost.vue'
import { toastMessages } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  deleteSystemConfig: vi.fn(),
  getSystemConfig: vi.fn(),
  getSubscriptionSettings: vi.fn(),
  setSystemConfig: vi.fn(),
  getSystemConfigs: vi.fn(),
  getLoadBalancers: vi.fn(),
  createLoadBalancer: vi.fn(),
  updateLoadBalancer: vi.fn(),
  deleteLoadBalancer: vi.fn(),
  runHealthCheck: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

async function renderSection(Section, ready) {
  render({ components: { Section, UiHost }, template: '<div><Section /><UiHost /></div>' }, { global: { stubs: { 'router-link': true } } })
  await screen.findByText(ready)
}

async function rowAction(user, name, item) {
  const opener = screen.getByRole('button', { name: `Actions for ${name}` })
  await user.click(opener)
  await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: item }))
  return opener
}

describe('System settings dialogs and feedback', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getSystemConfig.mockResolvedValue({ data: { value: '' } })
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: [] } })
    adminApi.getSystemConfigs.mockResolvedValue({ data: { list: [{ key: 'site.name', value: 'AnixOps', description: 'Site name' }] } })
    adminApi.getLoadBalancers.mockResolvedValue({ data: { list: [] } })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('edits a config in a dialog: errors inline, Esc closes and returns focus', async () => {
    const user = userEvent.setup()
    adminApi.setSystemConfig.mockResolvedValueOnce({ code: -1, msg: 'read only' }).mockResolvedValueOnce({ code: 0 })
    await renderSection(SettingsGeneral, 'site.name')
    const opener = await rowAction(user, 'site.name', 'Edit')
    const dialog = await screen.findByRole('dialog', { name: 'Edit configuration key' })
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect((await within(dialog).findByRole('alert')).textContent).toContain('read only')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))

    await rowAction(user, 'site.name', 'Edit')
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(toastMessages('success')).toEqual(['Saved site.name'])
  })

  it('asks for a key before adding a config', async () => {
    const user = userEvent.setup()
    await renderSection(SettingsGeneral, 'site.name')
    await user.click(screen.getByRole('button', { name: 'Add key' }))
    const dialog = await screen.findByRole('dialog', { name: 'Add configuration key' })
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(within(dialog).getByText('Enter a key')).toBeTruthy()
    expect(adminApi.setSystemConfig).not.toHaveBeenCalled()
  })

  it('asks before deleting a config; Cancel keeps it', async () => {
    const user = userEvent.setup()
    adminApi.deleteSystemConfig.mockResolvedValue({ code: 0 })
    await renderSection(SettingsGeneral, 'site.name')
    await rowAction(user, 'site.name', 'Delete')
    let confirm = await screen.findByRole('alertdialog', { name: 'Delete site.name?' })
    await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(adminApi.deleteSystemConfig).not.toHaveBeenCalled()

    await rowAction(user, 'site.name', 'Delete')
    confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: 'Delete key' }))
    await waitFor(() => expect(adminApi.deleteSystemConfig).toHaveBeenCalledWith('site.name'))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Deleted site.name']))
  })

  it('validates subscription domains as you type and saves them from the save bar', async () => {
    const user = userEvent.setup()
    adminApi.setSystemConfig.mockResolvedValue({ code: 0 })
    await renderSection(SettingsGeneral, 'site.name')
    expect(screen.queryByRole('region', { name: 'Unsaved changes' })).toBeNull()

    const field = await screen.findByLabelText('Alternate domains')
    await user.type(field, 'bad domain')
    const bar = await screen.findByRole('region', { name: 'Unsaved changes' })
    expect(screen.getByText('Not valid domains: bad domain')).toBeTruthy()
    expect(within(bar).getByRole('button', { name: 'Save' }).disabled).toBe(true)

    await user.clear(field)
    await user.type(field, 'https://sub.example.com/path')
    const save = within(await screen.findByRole('region', { name: 'Unsaved changes' })).getByRole('button', { name: 'Save' })
    expect(save.disabled).toBe(false)
    expect(screen.getByText(/\/\/sub\.example\.com\/s$/)).toBeTruthy()
    await user.click(save)
    await waitFor(() => expect(adminApi.setSystemConfig).toHaveBeenCalledWith('app.subscribe_domains', {
      value: '["sub.example.com"]',
      type: 'json',
      group: 'app',
      description: 'Alternate subscription domains'
    }))
    await waitFor(() => expect(toastMessages('success')).toEqual(['Subscription domains saved']))
  })

  it('discards subscription domain edits from the save bar', async () => {
    const user = userEvent.setup()
    adminApi.getSubscriptionSettings.mockResolvedValue({ data: { subscribe_path: '/s', subscribe_domains: ['a.example.com'] } })
    await renderSection(SettingsGeneral, 'site.name')
    const field = await screen.findByLabelText('Alternate domains')
    await user.type(field, '\nb.example.com')
    const bar = await screen.findByRole('region', { name: 'Unsaved changes' })
    await user.click(within(bar).getByRole('button', { name: 'Discard' }))
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Unsaved changes' })).toBeNull())
    expect(field.value).toBe('a.example.com')
    expect(adminApi.setSystemConfig).not.toHaveBeenCalled()
  })

  it('validates balancer name and weights inside the dialog', async () => {
    const user = userEvent.setup()
    await renderSection(SettingsBalancer, 'No load balancers yet')
    await user.click(screen.getAllByRole('button', { name: 'New load balancer' })[0])
    const dialog = await screen.findByRole('dialog', { name: 'New load balancer' })
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(within(dialog).getByText('Enter a name')).toBeTruthy()

    await user.type(within(dialog).getByLabelText(/Name/), 'edge')
    await user.type(within(dialog).getByLabelText(/Node weights/), 'not json')
    expect(within(dialog).getByText('Not valid JSON')).toBeTruthy()
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect(within(dialog).getByRole('alert').textContent).toBe('Not valid JSON')
    expect(adminApi.createLoadBalancer).not.toHaveBeenCalled()
  })
})
