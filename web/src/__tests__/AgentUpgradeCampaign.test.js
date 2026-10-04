import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AgentUpgradeCampaign from '@/components/admin/AgentUpgradeCampaign.vue'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({
  listKernelAgentUpgrades: vi.fn(),
  pauseKernelAgentUpgrade: vi.fn(),
  resumeKernelAgentUpgrade: vi.fn(),
  abortKernelAgentUpgrade: vi.fn()
}))
const confirm = vi.hoisted(() => vi.fn())

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('@/ui/composables/useConfirm', () => ({ useConfirm: () => confirm }))

function campaign(overrides = {}) {
  return {
    id: 'c1',
    target_version: 'v4.2.0',
    status: 'running',
    current_batch: 1,
    batch_ends_at: '2026-10-04T13:00:00Z',
    finished_at: null,
    status_reason: '',
    error_code: '',
    total: 40,
    batches: [
      { index: 0, percent: 5, min_duration_seconds: 1800, nodes: 2, states: { succeeded: 2 }, offered: 2, failed: 0 },
      { index: 1, percent: 25, min_duration_seconds: 1800, nodes: 8, states: { succeeded: 3, upgrading: 4, skipped: 1 }, offered: 7, failed: 0 },
      { index: 2, percent: 100, min_duration_seconds: 1800, nodes: 30, states: { pending: 30 }, offered: 0, failed: 0 }
    ],
    ...overrides
  }
}

const mounted = []

async function mountSection() {
  const wrapper = mount(AgentUpgradeCampaign, { attachTo: document.body })
  mounted.push(wrapper)
  await flushPromises()
  return wrapper
}

describe('Agent upgrade campaign', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
  })

  afterEach(() => {
    while (mounted.length) mounted.pop().unmount()
  })

  it('shows how to start one when there is none', async () => {
    kernelApi.listKernelAgentUpgrades.mockResolvedValue({ campaigns: [] })
    const wrapper = await mountSection()
    expect(kernelApi.listKernelAgentUpgrades).toHaveBeenCalledWith(1)
    expect(wrapper.get('[data-testid="agent-upgrade-empty"]').text()).toContain('anix-control agent upgrade start')
    expect(wrapper.find('[data-testid="agent-upgrade-pause"]').exists()).toBe(false)
  })

  it('shows the batches and pauses and resumes', async () => {
    kernelApi.listKernelAgentUpgrades.mockResolvedValue({ campaigns: [campaign()] })
    kernelApi.pauseKernelAgentUpgrade.mockResolvedValue(campaign({ status: 'paused' }))
    kernelApi.resumeKernelAgentUpgrade.mockResolvedValue(campaign())
    const wrapper = await mountSection()

    const text = wrapper.get('[data-testid="agent-upgrade-campaign"]').text()
    expect(text).toContain('Running')
    expect(text).toContain('Agent v4.2.0')
    expect(text).toContain('batch 2 of 3')
    expect(wrapper.get('[data-testid="agent-upgrade-batch-1"]').attributes('data-current')).toBe('true')
    expect(wrapper.get('[data-testid="agent-upgrade-batch-1"]').text()).toContain('4 / 8 settled')
    expect(wrapper.get('[data-testid="agent-upgrade-batch-1"]').text()).toContain('Skipped 1')
    expect(wrapper.get('[data-testid="agent-upgrade-batch-2"]').text()).toContain('Pending 30')

    await wrapper.get('[data-testid="agent-upgrade-pause"]').trigger('click')
    await flushPromises()
    expect(kernelApi.pauseKernelAgentUpgrade).toHaveBeenCalledWith('c1')
    expect(wrapper.get('[data-testid="agent-upgrade-campaign"]').text()).toContain('Paused')
    await wrapper.get('[data-testid="agent-upgrade-resume"]').trigger('click')
    await flushPromises()
    expect(kernelApi.resumeKernelAgentUpgrade).toHaveBeenCalledWith('c1')
  })

  it('aborts with rollback after confirmation', async () => {
    kernelApi.listKernelAgentUpgrades.mockResolvedValue({ campaigns: [campaign()] })
    kernelApi.abortKernelAgentUpgrade.mockResolvedValue(campaign({ status: 'rolling_back', error_code: 'aborted', status_reason: 'aborted by an administrator with rollback' }))
    confirm.mockImplementation(async options => {
      await options.onConfirm()
      return true
    })
    const wrapper = await mountSection()
    await wrapper.get('[data-testid="agent-upgrade-rollback"]').trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenCalledWith(expect.objectContaining({ tone: 'danger', title: 'Roll back the upgrade to v4.2.0?' }))
    expect(kernelApi.abortKernelAgentUpgrade).toHaveBeenCalledWith('c1', true)
    expect(wrapper.get('[data-testid="agent-upgrade-reason"]').text()).toContain('aborted by an administrator')
    expect(wrapper.find('[data-testid="agent-upgrade-rollback"]').exists()).toBe(false)
  })

  it('shows a rolled back campaign without actions', async () => {
    kernelApi.listKernelAgentUpgrades.mockResolvedValue({ campaigns: [campaign({
      status: 'rolled_back', error_code: 'batch_failed', status_reason: 'batch 2: 1 of 8 offered nodes failed (more than 5%)', finished_at: '2026-10-04T13:10:00Z'
    })] })
    const wrapper = await mountSection()
    expect(wrapper.get('[data-testid="agent-upgrade-campaign"]').text()).toContain('Rolled back')
    expect(wrapper.get('[data-testid="agent-upgrade-reason"]').text()).toContain('batch_failed')
    expect(wrapper.find('[data-testid="agent-upgrade-abort"]').exists()).toBe(false)
  })

  it('shows a load error', async () => {
    kernelApi.listKernelAgentUpgrades.mockRejectedValue({ response: { data: { error: { message: 'forbidden' } } } })
    const wrapper = await mountSection()
    expect(wrapper.get('[role="alert"]').text()).toContain('forbidden')
  })
})
