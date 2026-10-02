import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AgentTransports from '@/views/admin/AgentTransports.vue'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({ getKernelAgentTransports: vi.fn() }))
const router = vi.hoisted(() => ({ push: vi.fn() }))

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('vue-router', () => ({ useRouter: () => router }))

function inventory(mode = 'preferred') {
  return {
    mode,
    sunset: '2027-03-31T00:00:00Z',
    generated_at: '2026-10-02T12:00:00Z',
    upgrade_guide: 'https://example.test/UPGRADE.md#agent-transports-preparing-for-v42',
    summary: { total: 3, mtls: 1, legacy: 1, third_party: 1, unseen: 0 },
    nodes: [
      {
        node: 'proxy-1', kind: 'proxy', id: 1, name: 'tokyo', enabled: true, status: 'mtls', transport: 'mtls-stream',
        agent_version: '2.0.0', last_seen_at: '2026-10-02T11:59:00Z',
        certificate: { serial: '0a1b', not_after: '2026-10-09T00:00:00Z', spiffe_id: 'spiffe://anixops/prod/agent/proxy-1' },
        transports: [{ transport: 'mtls-stream', legacy: false, third_party: false, last_seen_at: '2026-10-02T11:59:00Z' }]
      },
      {
        node: 'proxy-2', kind: 'proxy', id: 2, name: 'osaka', enabled: false, status: 'legacy', transport: 'websocket',
        agent_version: '1.1.0', last_seen_at: '2026-10-02T11:58:00Z', certificate: null,
        transports: [
          { transport: 'websocket', legacy: true, third_party: false, last_seen_at: '2026-10-02T11:58:00Z' },
          { transport: 'uniproxy', legacy: false, third_party: true, last_seen_at: '2026-10-02T11:50:00Z' }
        ]
      },
      {
        node: 'proxy-3', kind: 'proxy', id: 3, name: '', enabled: true, status: 'third-party', transport: 'uniproxy',
        last_seen_at: null, certificate: null, transports: null
      }
    ]
  }
}

const mounted = []

async function mountPage() {
  const wrapper = mount(AgentTransports, { attachTo: document.body })
  mounted.push(wrapper)
  await flushPromises()
  return wrapper
}

describe('Agent transports', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
  })

  afterEach(() => {
    while (mounted.length) mounted.pop().unmount()
  })

  it('lists each node with its transport and flags legacy ones', async () => {
    kernelApi.getKernelAgentTransports.mockResolvedValue(inventory())
    const wrapper = await mountPage()

    const policy = wrapper.get('[data-testid="agent-transports-policy"]').text()
    expect(policy).toContain('agent_control.mtls: preferred')
    expect(policy).toContain('deprecation signals')
    expect(policy).toContain('Legacy sunset')
    expect(wrapper.get('[data-testid="agent-transports-notice"]').text()).toContain('v4.2')
    expect(wrapper.get('[data-testid="agent-transports-notice"] a').attributes('href')).toContain('agent-transports-preparing-for-v42')

    const table = wrapper.get('[data-testid="agent-transports-table"]').text()
    expect(table).toContain('tokyo')
    expect(table).toContain('mTLS stream')
    expect(table).toContain('0a1b')
    expect(table).toContain('Not enrolled')
    expect(table).toContain('Disabled')
    expect(table).toContain('proxy-3')
    expect(wrapper.find('[data-node-status="proxy-2"] [data-legacy-warning]').exists()).toBe(true)
    expect(wrapper.find('[data-node-status="proxy-1"] [data-legacy-warning]').exists()).toBe(false)
  })

  it('filters by status and goes back to the agents page', async () => {
    kernelApi.getKernelAgentTransports.mockResolvedValue(inventory())
    const wrapper = await mountPage()
    const legacyChip = wrapper.get('[data-testid="agent-transports-filter"] [data-filter="legacy"]')
    expect(legacyChip.text()).toContain('1')
    await legacyChip.trigger('click')
    const table = wrapper.get('[data-testid="agent-transports-table"]').text()
    expect(table).toContain('osaka')
    expect(table).not.toContain('tokyo')

    await wrapper.get('[data-testid="agent-transports-back"]').trigger('click')
    expect(router.push).toHaveBeenCalledWith('/admin/agent')
  })

  it('hides the v4.2 notice once required and describes each mode', async () => {
    kernelApi.getKernelAgentTransports.mockResolvedValue({ ...inventory('required'), sunset: null, summary: undefined })
    const wrapper = await mountPage()
    expect(wrapper.find('[data-testid="agent-transports-notice"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="agent-transports-policy"]').text()).toContain('Only enrolled agents')
    expect(wrapper.get('[data-testid="agent-transports-filter"] [data-filter="mtls"]').text()).toContain('1')

    for (const mode of ['off', 'optional', 'unknown']) {
      kernelApi.getKernelAgentTransports.mockResolvedValue({ ...inventory(mode), nodes: undefined })
      await wrapper.vm.load()
      await flushPromises()
      expect(wrapper.get('[data-testid="agent-transports-policy"]').text()).toContain(`agent_control.mtls: ${mode}`)
    }
  })

  it('shows a load error with retry, and keeps the table on a failed refresh', async () => {
    kernelApi.getKernelAgentTransports.mockRejectedValueOnce({ response: { data: { error: { code: 'database_error', message: 'database operation failed' } } } })
    const wrapper = await mountPage()
    expect(wrapper.text()).toContain('Unable to load agent transports')

    kernelApi.getKernelAgentTransports.mockResolvedValueOnce(inventory())
    await wrapper.vm.load()
    await flushPromises()
    expect(wrapper.get('[data-testid="agent-transports-table"]').text()).toContain('tokyo')

    kernelApi.getKernelAgentTransports.mockRejectedValueOnce(new Error('offline'))
    await wrapper.get('[data-testid="refresh-agent-transports"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('offline')
    expect(wrapper.get('[data-testid="agent-transports-table"]').text()).toContain('tokyo')
  })
})
