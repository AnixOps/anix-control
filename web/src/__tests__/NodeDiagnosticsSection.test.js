// 诊断 of a node (5.0, D7): what the Agent session runs, service status,
// restart with a confirmation, log tail, the forward checks of a hop, the
// result panel and the recorded tasks.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeDiagnosticsSection from '@/views/admin/nodes/NodeDiagnosticsSection.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({
  getKernelNodeDiagnostics: vi.fn(),
  runKernelNodeDiagnostic: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

const GENERIC = [
  { action: 'log_tail', mutating: false, params: ['service', 'lines'], forward: false },
  { action: 'service_restart', mutating: true, params: ['service'], forward: false },
  { action: 'service_status', mutating: false, params: ['service'], forward: false }
]
const CHECKS = ['forward.connect', 'forward.listen', 'forward.port_conflict', 'forward.udp_probe']
  .map(action => ({ action, mutating: false, params: ['route_id', 'hop_index'], forward: true }))

function listing(overrides = {}) {
  return {
    node: 'proxy-5',
    connected: true,
    agent_version: '4.2.0',
    diagnostics: true,
    forward_checks: false,
    actions: GENERIC,
    services: ['gost'],
    log_tail_max: 1000,
    timeout_seconds: { default: 30, max: 120 },
    can_restart: true,
    history: true,
    tasks: [],
    ...overrides
  }
}

function answer(overrides = {}) {
  return {
    task_id: 'diag-1', node: 'proxy-5', action: 'service_status', params: { service: 'gost' },
    success: true, output: 'active (running) since Mon', error: '', duration_ms: 182, ...overrides
  }
}

function renderSection(props = { nodeRef: 'proxy-5', name: 'hk-01' }) {
  return render({
    components: { NodeDiagnosticsSection, UiHost },
    setup: () => ({ props }),
    template: '<div><NodeDiagnosticsSection v-bind="props" /><UiHost /></div>'
  })
}

describe('NodeDiagnosticsSection', () => {
  beforeEach(async () => {
    for (const fn of Object.values(kernelApi)) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('says the node is offline and offers nothing to run', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({ connected: false, diagnostics: false, actions: [] }))
    renderSection()
    expect(await screen.findByTestId('diagnostics-offline')).toBeTruthy()
    expect(kernelApi.getKernelNodeDiagnostics).toHaveBeenCalledWith('proxy-5')
    expect(screen.queryByTestId('diagnostics-status')).toBeNull()
    expect(screen.queryByTestId('diagnostics-restart')).toBeNull()
  })

  it('says an Agent without agent.diagnostic cannot run diagnostics, with its version', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({ diagnostics: false, actions: [], agent_version: '4.1.9' }))
    renderSection()
    const empty = await screen.findByTestId('diagnostics-unsupported')
    expect(empty.textContent).toContain('Agent 4.1.9 does not run diagnostics')
    expect(screen.queryByTestId('diagnostics-status')).toBeNull()
  })

  it('lists what the Agent supports and shows a service status result', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing())
    kernelApi.runKernelNodeDiagnostic.mockResolvedValue(answer())
    renderSection()
    const agent = await screen.findByTestId('diagnostics-agent')
    expect(agent.textContent).toContain('Agent 4.2.0')
    expect(agent.textContent).toContain('View service status')
    expect(agent.textContent).toContain('Restart service')
    expect(agent.textContent).toContain('View logs')

    await userEvent.click(screen.getByTestId('diagnostics-status'))
    expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenCalledWith('proxy-5', { action: 'service_status', params: { service: 'gost' }, timeoutSeconds: 30 })
    const result = await screen.findByTestId('diagnostics-result')
    expect(within(result).getByTestId('diagnostics-result-status').textContent).toContain('Succeeded')
    expect(result.textContent).toContain('View service status')
    expect(result.textContent).toContain('182 ms')
    expect(screen.getByTestId('diagnostics-output').textContent).toContain('active (running) since Mon')
    expect(within(result).getByRole('button', { name: /copy/i })).toBeTruthy()
    // The listing is read again for the newest tasks.
    await waitFor(() => expect(kernelApi.getKernelNodeDiagnostics).toHaveBeenCalledTimes(2))
  })

  it('shows a failed result with its error and keeps the output', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing())
    kernelApi.runKernelNodeDiagnostic.mockResolvedValue(answer({ success: false, output: 'inactive (dead)', error: 'exit status 3' }))
    renderSection()
    await userEvent.click(await screen.findByTestId('diagnostics-status'))
    expect((await screen.findByTestId('diagnostics-result-status')).textContent).toContain('Failed')
    expect(screen.getByTestId('diagnostics-result-error').textContent).toBe('exit status 3')
    expect(screen.getByTestId('diagnostics-output').textContent).toContain('inactive (dead)')
  })

  it('asks before restarting, and does not restart when declined', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing())
    kernelApi.runKernelNodeDiagnostic.mockResolvedValue(answer({ action: 'service_restart', output: '' }))
    renderSection()
    await userEvent.click(await screen.findByTestId('diagnostics-restart'))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog.textContent).toContain('Restart gost on hk-01?')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(kernelApi.runKernelNodeDiagnostic).not.toHaveBeenCalled()

    await userEvent.click(screen.getByTestId('diagnostics-restart'))
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Restart service' }))
    await waitFor(() => expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenCalledTimes(1))
    expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenCalledWith('proxy-5', { action: 'service_restart', params: { service: 'gost' }, timeoutSeconds: 60 })
    expect((await screen.findByTestId('diagnostics-result')).textContent).toContain('(no output)')
  })

  it('disables the restart for an administrator who is not a super administrator', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({ can_restart: false }))
    renderSection()
    const restart = await screen.findByTestId('diagnostics-restart')
    expect(restart.hasAttribute('disabled')).toBe(true)
    expect(screen.getByTestId('diagnostics-restart-hint').textContent).toContain('super administrator')
    expect(screen.getByTestId('diagnostics-status').hasAttribute('disabled')).toBe(false)
  })

  it('reads the last 100, 500 or 1000 log lines', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing())
    kernelApi.runKernelNodeDiagnostic.mockResolvedValue(answer({ action: 'log_tail', output: 'line 1\nline 2' }))
    renderSection()
    await userEvent.click(await screen.findByTestId('diagnostics-logs'))
    expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenLastCalledWith('proxy-5', { action: 'log_tail', params: { service: 'gost', lines: 100 }, timeoutSeconds: 30 })
    await screen.findByTestId('diagnostics-result')

    await userEvent.click(screen.getByRole('combobox', { name: 'Log lines' }))
    expect(screen.getAllByRole('option').map(option => option.textContent.trim())).toEqual(['Last 100 lines', 'Last 500 lines', 'Last 1000 lines'])
    await userEvent.click(screen.getByRole('option', { name: 'Last 500 lines' }))
    await userEvent.click(screen.getByTestId('diagnostics-logs'))
    expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenLastCalledWith('proxy-5', { action: 'log_tail', params: { service: 'gost', lines: 500 }, timeoutSeconds: 30 })
  })

  it('explains a refusal by its code', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing())
    const refusal = (status, code, message = '') => Object.assign(new Error('failed'), { response: { status, data: { error: { code, message } } } })
    renderSection()
    const status = await screen.findByTestId('diagnostics-status')

    kernelApi.runKernelNodeDiagnostic.mockRejectedValueOnce(refusal(409, 'node_offline', 'the node\'s Agent is not connected'))
    await userEvent.click(status)
    expect((await screen.findByTestId('diagnostics-error')).textContent).toBe('The node’s Agent is offline.')

    kernelApi.runKernelNodeDiagnostic.mockRejectedValueOnce(refusal(504, 'diagnostic_timeout'))
    await userEvent.click(screen.getByTestId('diagnostics-status'))
    await waitFor(() => expect(screen.getByTestId('diagnostics-error').textContent).toBe('The node returned no result within 30 seconds.'))

    kernelApi.runKernelNodeDiagnostic.mockRejectedValueOnce(refusal(409, 'diagnostic_busy'))
    await userEvent.click(screen.getByTestId('diagnostics-status'))
    await waitFor(() => expect(screen.getByTestId('diagnostics-error').textContent).toContain('Another diagnostic of this node is running'))

    kernelApi.runKernelNodeDiagnostic.mockRejectedValueOnce(refusal(502, 'agent_error', 'busy'))
    await userEvent.click(screen.getByTestId('diagnostics-status'))
    await waitFor(() => expect(screen.getByTestId('diagnostics-error').textContent).toBe('The Agent refused this diagnostic: busy'))

    kernelApi.runKernelNodeDiagnostic.mockRejectedValueOnce(refusal(500, 'something_new', 'the server said so'))
    await userEvent.click(screen.getByTestId('diagnostics-status'))
    await waitFor(() => expect(screen.getByTestId('diagnostics-error').textContent).toBe('the server said so'))
  })

  it('shows a load error with a retry', async () => {
    kernelApi.getKernelNodeDiagnostics.mockRejectedValueOnce(Object.assign(new Error('boom'), { response: { status: 502 } }))
    kernelApi.getKernelNodeDiagnostics.mockResolvedValueOnce(listing())
    renderSection()
    expect(await screen.findByText('Could not load diagnostics')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByTestId('diagnostics-status')).toBeTruthy()
  })

  it('lists the recorded tasks and shows one in the result panel', async () => {
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({
      tasks: [
        { task_id: 't-2', action: 'log_tail', status: 'completed', success: true, output: 'old log line', duration_ms: 40, updated_at: '2026-10-12T08:00:00Z', created_at: '2026-10-12T08:00:00Z' },
        { task_id: 't-1', action: 'service_status', status: 'failed', success: false, error: 'unit not found', output: '', duration_ms: 9, updated_at: '2026-10-12T07:00:00Z', created_at: '2026-10-12T07:00:00Z' }
      ]
    }))
    renderSection()
    const history = await screen.findByTestId('diagnostics-history')
    expect(within(history).getAllByRole('listitem')).toHaveLength(2)
    await userEvent.click(screen.getByTestId('diagnostics-show-t-2'))
    expect(screen.getByTestId('diagnostics-output').textContent).toContain('old log line')
    await userEvent.click(screen.getByTestId('diagnostics-show-t-1'))
    expect(screen.getByTestId('diagnostics-result-error').textContent).toBe('unit not found')
  })

  describe('forward node', () => {
    const hops = [
      { route_id: 'r-1', hop_index: 0, label: 'tokyo-entry · Entry (hop 1)' },
      { route_id: 'r-2', hop_index: 2, label: 'osaka-exit · Exit (hop 3)' }
    ]
    const props = { nodeRef: 'forward-3', name: 'relay-3', hops }

    it('runs a forward check on the chosen hop and lists what it probed', async () => {
      kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({ node: 'forward-3', forward_checks: true, actions: [...GENERIC, ...CHECKS], history: false }))
      kernelApi.runKernelNodeDiagnostic.mockResolvedValue(answer({
        node: 'forward-3', action: 'forward.connect', output: '1 upstream reachable',
        result: { check: 'forward.connect', status: 'ok', items: [{ target: '198.51.100.10:443', protocol: 'tcp', status: 'ok', rtt_us: 1500 }] }
      }))
      renderSection(props)
      await userEvent.click(await screen.findByTestId('diagnostics-forward_connect'))
      expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenCalledWith('forward-3', {
        action: 'forward.connect', params: { route_id: 'r-1', hop_index: 0 }, timeoutSeconds: 30
      })
      const items = await screen.findByTestId('diagnostics-items')
      expect(items.textContent).toContain('198.51.100.10:443')
      expect(items.textContent).toContain('OK')
      expect(items.textContent).toContain('1.5 ms')
      expect(screen.getByTestId('diagnostics-no-history')).toBeTruthy()

      await userEvent.click(screen.getByRole('combobox', { name: 'Hop to check' }))
      await userEvent.click(screen.getByRole('option', { name: 'osaka-exit · Exit (hop 3)' }))
      await userEvent.click(screen.getByTestId('diagnostics-forward_listen'))
      expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenLastCalledWith('forward-3', {
        action: 'forward.listen', params: { route_id: 'r-2', hop_index: 2 }, timeoutSeconds: 30
      })
    })

    it('says so when the node hosts no hop, and when its Agent lacks the forward checks', async () => {
      kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({ node: 'forward-3', forward_checks: true, actions: [...GENERIC, ...CHECKS], history: false }))
      const first = renderSection({ nodeRef: 'forward-3', name: 'relay-3', hops: [] })
      expect(await screen.findByTestId('diagnostics-no-hops')).toBeTruthy()
      expect(screen.queryByTestId('diagnostics-forward_listen')).toBeNull()
      first.unmount()

      kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({ node: 'forward-3', forward_checks: false, history: false }))
      renderSection(props)
      expect(await screen.findByTestId('diagnostics-no-checks')).toBeTruthy()
    })

    it('shows no forward checks on a proxy node without hops', async () => {
      kernelApi.getKernelNodeDiagnostics.mockResolvedValue(listing({ forward_checks: true, actions: [...GENERIC, ...CHECKS] }))
      renderSection()
      await screen.findByTestId('diagnostics-status')
      expect(screen.queryByTestId('diagnostics-forward_listen')).toBeNull()
      expect(screen.queryByTestId('diagnostics-no-hops')).toBeNull()
    })
  })
})
