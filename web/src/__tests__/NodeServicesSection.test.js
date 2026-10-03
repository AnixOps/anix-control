// 服务 section of the node page: the read-only systemd services table of the
// machine-telemetry package, its states, filters, sorting and the per-node
// settings.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeServicesSection from '@/views/admin/nodes/NodeServicesSection.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'
import { toastMessages } from './helpers/feedback'

const telemetry = vi.hoisted(() => ({
  getNodeServices: vi.fn(),
  saveNodeServicesSettings: vi.fn()
}))

vi.mock('@/api/machineTelemetry', async importOriginal => ({ ...(await importOriginal()), ...telemetry }))

const node = { id: 5, name: 'hk-01' }
const minutesAgo = minutes => new Date(Date.now() - minutes * 60_000).toISOString()

function table(overrides = {}) {
  return {
    node_id: 5,
    enabled: true,
    include: [],
    exclude: ['*-debug.service'],
    reported: true,
    supported: true,
    unsupported_reason: '',
    stale: false,
    observed_at: minutesAgo(2),
    version: '4.1.0',
    window_seconds: 600,
    summary: { total: 3, failed: 1, active: 1, inactive: 1 },
    units: [
      { name: 'nginx.service', active_state: 'active', sub_state: 'running', cpu_avg_percent: 1.5, cpu_peak_percent: 12.25, memory_bytes: 52428800, memory_peak_bytes: 73400320 },
      { name: 'backup.service', active_state: 'failed', sub_state: 'failed', cpu_avg_percent: 0, cpu_peak_percent: 0, memory_bytes: 4096, memory_peak_bytes: 8192 },
      { name: 'cron.service', active_state: 'inactive', sub_state: 'dead', cpu_avg_percent: 0.25, cpu_peak_percent: 1, memory_bytes: 0, memory_peak_bytes: 1048576 }
    ],
    ...overrides
  }
}

function renderSection() {
  return render({
    components: { NodeServicesSection, UiHost },
    setup: () => ({ node }),
    template: '<div><NodeServicesSection :node="node" /><UiHost /></div>'
  })
}

const rowNames = () => screen.getAllByRole('row').slice(1).map(row => within(row).getAllByRole('cell')[0].textContent.trim())

describe('NodeServicesSection', () => {
  beforeEach(() => {
    for (const fn of Object.values(telemetry)) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(async () => {
    vi.restoreAllMocks()
    await setLocale('en')
  })

  it('lists the units with human-readable CPU and memory and the totals line', async () => {
    telemetry.getNodeServices.mockResolvedValue(table())
    renderSection()
    expect(await screen.findByText('nginx.service')).toBeTruthy()
    expect(telemetry.getNodeServices).toHaveBeenCalledWith(5)
    const nginx = screen.getByText('nginx.service').closest('tr')
    expect(within(nginx).getByText('Active')).toBeTruthy()
    expect(within(nginx).getByText('running')).toBeTruthy()
    expect(within(nginx).getByText('1.5%')).toBeTruthy()
    expect(within(nginx).getByText('12.3%')).toBeTruthy()
    expect(within(nginx).getByText('50.0 MB')).toBeTruthy()
    expect(within(nginx).getByText('70.0 MB')).toBeTruthy()
    expect(screen.getByTestId('services-totals').textContent).toContain('Total 3 | Failed 1 | Updated every 10 minutes')
    expect(screen.queryByTestId('services-stale')).toBeNull()
    expect(screen.queryByRole('button', { name: /start|stop|restart/i })).toBeNull()
  })

  it('shows the totals line in Chinese', async () => {
    await setLocale('zh-CN')
    telemetry.getNodeServices.mockResolvedValue(table())
    renderSection()
    await screen.findByText('nginx.service')
    expect(screen.getByTestId('services-totals').textContent).toContain('总计 3 | 失败 1 | 每 10 分钟更新一次')
  })

  it('filters by name and by state, and sorts by any column', async () => {
    const user = userEvent.setup()
    telemetry.getNodeServices.mockResolvedValue(table())
    renderSection()
    await screen.findByText('nginx.service')
    expect(rowNames()).toEqual(['backup.service', 'cron.service', 'nginx.service'])

    await user.click(screen.getByRole('button', { name: /Memory peak/ }))
    expect(rowNames()).toEqual(['nginx.service', 'cron.service', 'backup.service'])
    await user.click(screen.getByRole('button', { name: /^State/ }))
    expect(rowNames()[0]).toBe('backup.service')

    await user.click(within(screen.getByRole('group', { name: 'Filter by state' })).getByRole('button', { name: /Failed/ }))
    expect(rowNames()).toEqual(['backup.service'])
    await user.click(within(screen.getByRole('group', { name: 'Filter by state' })).getByRole('button', { name: /All/ }))
    await user.type(screen.getByRole('searchbox', { name: 'Search services' }), 'CRON')
    expect(rowNames()).toEqual(['cron.service'])
    await user.clear(screen.getByRole('searchbox', { name: 'Search services' }))
    await user.type(screen.getByRole('searchbox', { name: 'Search services' }), 'nothing')
    await user.click(await screen.findByRole('button', { name: 'Clear filters' }))
    await waitFor(() => expect(rowNames()).toHaveLength(3))
  })

  it('warns when the report is stale', async () => {
    telemetry.getNodeServices.mockResolvedValue(table({ stale: true, observed_at: minutesAgo(40) }))
    renderSection()
    expect((await screen.findByTestId('services-stale')).textContent).toContain('40 minutes ago')
  })

  it('explains an unsupported node with its reason, and waits for a first report', async () => {
    telemetry.getNodeServices.mockResolvedValueOnce(table({ supported: false, unsupported_reason: 'cgroup v1', units: [], summary: { total: 0, failed: 0, active: 0, inactive: 0 } }))
    const { unmount } = renderSection()
    const unsupported = await screen.findByTestId('services-unsupported')
    expect(unsupported.textContent).toContain('This node can’t report its services')
    expect(unsupported.textContent).toContain('Reason: cgroup v1')
    unmount()

    telemetry.getNodeServices.mockResolvedValueOnce(table({ reported: false, observed_at: null, units: [] }))
    renderSection()
    expect((await screen.findByTestId('services-waiting')).textContent).toContain('Waiting for the first report')
  })

  it('offers to enable a node that is not enabled, keeping its patterns', async () => {
    const user = userEvent.setup()
    telemetry.getNodeServices.mockResolvedValueOnce(table({ enabled: false, include: ['nginx*'], exclude: [], reported: false, units: [] }))
    telemetry.getNodeServices.mockResolvedValue(table())
    telemetry.saveNodeServicesSettings.mockResolvedValue({})
    renderSection()
    expect((await screen.findByTestId('services-disabled')).textContent).toContain('Not enabled on this node')
    expect(screen.queryByRole('table')).toBeNull()
    await user.click(screen.getByTestId('enable-services'))
    await waitFor(() => expect(telemetry.saveNodeServicesSettings).toHaveBeenCalledWith(5, { enabled: true, include: ['nginx*'], exclude: [] }))
    expect(await screen.findByText('nginx.service')).toBeTruthy()
    expect(toastMessages('success')[0]).toContain('Services settings saved')
  })

  it('edits the switch and the patterns, refusing a malformed pattern', async () => {
    const user = userEvent.setup()
    telemetry.getNodeServices.mockResolvedValue(table())
    telemetry.saveNodeServicesSettings.mockResolvedValue({})
    renderSection()
    await screen.findByText('nginx.service')
    await user.click(screen.getByTestId('services-settings'))
    const dialog = await screen.findByRole('dialog', { name: 'Services settings' })
    expect(within(dialog).getByRole('switch', { name: /Collect services on this node/ }).getAttribute('aria-checked')).toBe('true')
    const include = within(dialog).getByRole('textbox', { name: 'Include patterns' })
    await user.type(include, 'nginx[[')
    expect(within(dialog).getByText('Not a valid pattern: nginx[')).toBeTruthy()
    expect(within(dialog).getByTestId('save-services').hasAttribute('disabled')).toBe(true)

    await user.clear(include)
    await user.type(include, 'nginx*.service{enter}ssh?.service')
    await user.click(within(dialog).getByRole('switch', { name: /Collect services on this node/ }))
    await user.click(within(dialog).getByTestId('save-services'))
    await waitFor(() => expect(telemetry.saveNodeServicesSettings).toHaveBeenCalledWith(5, {
      enabled: false, include: ['nginx*.service', 'ssh?.service'], exclude: ['*-debug.service']
    }))
  })

  it('reloads after a revision conflict and says so', async () => {
    const user = userEvent.setup()
    telemetry.getNodeServices.mockResolvedValue(table())
    telemetry.saveNodeServicesSettings.mockRejectedValue(Object.assign(new Error('conflict'), { response: { status: 409, data: { error: { code: 'configuration_revision_conflict' } } } }))
    renderSection()
    await screen.findByText('nginx.service')
    await user.click(screen.getByTestId('services-settings'))
    const dialog = await screen.findByRole('dialog', { name: 'Services settings' })
    await user.click(within(dialog).getByTestId('save-services'))
    expect(await within(dialog).findByRole('alert')).toBeTruthy()
    expect(within(dialog).getByRole('alert').textContent).toContain('The settings changed elsewhere')
    await waitFor(() => expect(telemetry.getNodeServices).toHaveBeenCalledTimes(2))
  })

  it('shows a load failure with a retry', async () => {
    const user = userEvent.setup()
    telemetry.getNodeServices.mockRejectedValueOnce(Object.assign(new Error('Request failed'), { response: { status: 502 } }))
    telemetry.getNodeServices.mockResolvedValue(table())
    renderSection()
    expect(await screen.findByText('Couldn’t load the services')).toBeTruthy()
    await user.click(document.querySelector('[data-error-retry]'))
    expect(await screen.findByText('nginx.service')).toBeTruthy()
  })
})
