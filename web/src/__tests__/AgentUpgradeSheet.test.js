// 升级节点 (5.0): the start form above the campaign card. The release
// defaults to the Agent version Control pins, the batches to 5/25/100 with
// the 30 minute minimum shown, and starting asks first, with the number of
// nodes it would upgrade.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import AgentUpgradeSheet from '@/components/admin/AgentUpgradeSheet.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'
import { closeAgentUpgrade, openAgentUpgrade } from '@/composables/useAgentUpgradeSheet'
import { toastMessages } from './helpers/feedback'

const kernelApi = vi.hoisted(() => ({
  getKernelAgentUpgradePlan: vi.fn(),
  startKernelAgentUpgrade: vi.fn(),
  listKernelAgentUpgrades: vi.fn(),
  pauseKernelAgentUpgrade: vi.fn(),
  resumeKernelAgentUpgrade: vi.fn(),
  abortKernelAgentUpgrade: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

function plan(overrides = {}) {
  return {
    target_version: 'v4.2.0',
    control_version: 'v4.2.0',
    configured: true,
    unconfigured_reason: '',
    eligible_nodes: 12,
    batches: [
      { percent: 5, min_duration_seconds: 1800 },
      { percent: 25, min_duration_seconds: 1800 },
      { percent: 100, min_duration_seconds: 1800 }
    ],
    min_batch_seconds: 1800,
    first_batch_max_percent: 5,
    failure_threshold_percent: 5,
    reconnect_timeout_seconds: 600,
    active_campaign_id: '',
    can_start: true,
    ...overrides
  }
}

function renderSheet() {
  return render({
    components: { AgentUpgradeSheet, UiHost },
    template: '<div><AgentUpgradeSheet /><UiHost /></div>'
  })
}

async function open() {
  openAgentUpgrade()
  return screen.findByTestId('agent-upgrade-form')
}

describe('AgentUpgradeSheet', () => {
  beforeEach(async () => {
    for (const fn of Object.values(kernelApi)) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    kernelApi.listKernelAgentUpgrades.mockResolvedValue({ campaigns: [] })
    await setLocale('en')
  })

  afterEach(() => {
    closeAgentUpgrade()
    vi.restoreAllMocks()
  })

  it('stays closed until openAgentUpgrade() is called', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan())
    renderSheet()
    expect(screen.queryByTestId('agent-upgrade-form')).toBeNull()
    expect(kernelApi.getKernelAgentUpgradePlan).not.toHaveBeenCalled()
    await open()
    expect(kernelApi.getKernelAgentUpgradePlan).toHaveBeenCalledTimes(1)
  })

  it('fills the pinned release and the default batches, and shows the minimum and the nodes', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan())
    renderSheet()
    await open()
    expect(screen.getByLabelText('Target version').value).toBe('v4.2.0')
    expect(screen.getByLabelText('Canary batches (cumulative %)').value).toBe('5, 25, 100')
    expect(screen.getByTestId('agent-upgrade-minimum').textContent).toBe('30 minutes')
    expect(screen.getByTestId('agent-upgrade-eligible').textContent).toBe('12 nodes')
    expect(screen.getByTestId('agent-upgrade-form').textContent).toContain('more than 5% of its nodes fail')
    expect(screen.getByTestId('agent-upgrade-start').hasAttribute('disabled')).toBe(false)
    // The campaign card is part of the sheet.
    expect(await screen.findByTestId('agent-upgrade-empty')).toBeTruthy()
  })

  it('confirms with the number of nodes, then starts with the batches and the 30 minute minimum', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan())
    kernelApi.startKernelAgentUpgrade.mockResolvedValue({ id: 'c1', status: 'running' })
    renderSheet()
    await open()
    await userEvent.type(screen.getByLabelText('Note (optional)'), 'roll out 4.2.0')
    await userEvent.click(screen.getByTestId('agent-upgrade-start'))

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog.textContent).toContain('Upgrade 12 nodes to v4.2.0?')
    expect(dialog.textContent).toContain('5% → 25% → 100%')
    expect(kernelApi.startKernelAgentUpgrade).not.toHaveBeenCalled()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Start upgrade' }))

    await waitFor(() => expect(kernelApi.startKernelAgentUpgrade).toHaveBeenCalledTimes(1))
    expect(kernelApi.startKernelAgentUpgrade).toHaveBeenCalledWith({
      target_version: 'v4.2.0',
      batches: [
        { percent: 5, min_duration_seconds: 1800 },
        { percent: 25, min_duration_seconds: 1800 },
        { percent: 100, min_duration_seconds: 1800 }
      ],
      reason: 'roll out 4.2.0'
    })
    await waitFor(() => expect(toastMessages('success')).toContain('Upgrade to v4.2.0 started'))
    // The plan and the campaign are read again.
    await waitFor(() => expect(kernelApi.getKernelAgentUpgradePlan).toHaveBeenCalledTimes(2))
  })

  it('does not start when the confirmation is declined', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan({ eligible_nodes: 1 }))
    renderSheet()
    await open()
    await userEvent.click(screen.getByTestId('agent-upgrade-start'))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog.textContent).toContain('Upgrade 1 node to v4.2.0?')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(kernelApi.startKernelAgentUpgrade).not.toHaveBeenCalled()
  })

  it('checks the batches like the server does', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan())
    renderSheet()
    await open()
    const field = screen.getByLabelText('Canary batches (cumulative %)')
    const start = screen.getByTestId('agent-upgrade-start')
    const cases = [
      ['10, 100', 'The first batch is at most 5%.'],
      ['5, 25', 'The last batch must be 100%.'],
      ['25, 5, 100', 'Each percentage must be larger than the one before and at most 100.'],
      ['5, x, 100', 'Write the batches as integers separated by commas, for example 5, 25, 100.'],
      ['1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 100', 'At most 10 batches.']
    ]
    for (const [text, message] of cases) {
      await userEvent.clear(field)
      await userEvent.type(field, text)
      expect(await screen.findByText(message), text).toBeTruthy()
      expect(start.hasAttribute('disabled'), text).toBe(true)
    }
    await userEvent.clear(field)
    await userEvent.type(field, '1, 100')
    await waitFor(() => expect(start.hasAttribute('disabled')).toBe(false))
  })

  it('rejects a target that is not a release tag', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan())
    renderSheet()
    await open()
    const field = screen.getByLabelText('Target version')
    await userEvent.clear(field)
    await userEvent.type(field, 'latest')
    expect(await screen.findByText('Write the target as a release tag, for example v4.2.0.')).toBeTruthy()
    expect(screen.getByTestId('agent-upgrade-start').hasAttribute('disabled')).toBe(true)
  })

  it('explains why it cannot start', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan({ can_start: false }))
    renderSheet()
    await open()
    expect(screen.getByTestId('agent-upgrade-super-only')).toBeTruthy()
    expect(screen.getByTestId('agent-upgrade-start').hasAttribute('disabled')).toBe(true)
    closeAgentUpgrade()
    await waitFor(() => expect(screen.queryByTestId('agent-upgrade-form')).toBeNull())

    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan({ active_campaign_id: 'c-live' }))
    await open()
    await waitFor(() => expect(screen.queryByTestId('agent-upgrade-active')).toBeTruthy())
    expect(screen.getByTestId('agent-upgrade-start').hasAttribute('disabled')).toBe(true)
    closeAgentUpgrade()
    await waitFor(() => expect(screen.queryByTestId('agent-upgrade-form')).toBeNull())

    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan({ eligible_nodes: 0 }))
    await open()
    await waitFor(() => expect(screen.queryByTestId('agent-upgrade-none')).toBeTruthy())
    expect(screen.getByTestId('agent-upgrade-start').hasAttribute('disabled')).toBe(true)
    closeAgentUpgrade()
    await waitFor(() => expect(screen.queryByTestId('agent-upgrade-form')).toBeNull())

    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan({ configured: false, target_version: '', unconfigured_reason: 'nodes must reach Control over https' }))
    await open()
    await waitFor(() => expect(screen.queryByTestId('agent-upgrade-unconfigured')).toBeTruthy())
    expect(screen.getByTestId('agent-upgrade-unconfigured').textContent).toContain('nodes must reach Control over https')
    expect(screen.getByTestId('agent-upgrade-start').hasAttribute('disabled')).toBe(true)
  })

  it('shows the refusal of the server inline', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValue(plan())
    kernelApi.startKernelAgentUpgrade.mockRejectedValue(Object.assign(new Error('conflict'), {
      response: { status: 409, data: { error: { code: 'agent_release_unverified', message: 'SHA256SUMS of the Agent release v4.2.0 is missing' } } }
    }))
    renderSheet()
    await open()
    await userEvent.click(screen.getByTestId('agent-upgrade-start'))
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Start upgrade' }))
    const error = await screen.findByTestId('agent-upgrade-start-error')
    expect(error.textContent).toContain('did not pass verification')
    expect(error.textContent).toContain('SHA256SUMS of the Agent release v4.2.0 is missing')
  })

  it('shows a load error with a retry', async () => {
    kernelApi.getKernelAgentUpgradePlan.mockRejectedValueOnce(Object.assign(new Error('down'), { response: { status: 502 } }))
    kernelApi.getKernelAgentUpgradePlan.mockResolvedValueOnce(plan())
    renderSheet()
    openAgentUpgrade()
    expect(await screen.findByText('Could not load upgrade details')).toBeTruthy()
    await userEvent.click(screen.getAllByRole('button', { name: 'Try again' })[0])
    expect(await screen.findByTestId('agent-upgrade-form')).toBeTruthy()
  })
})
