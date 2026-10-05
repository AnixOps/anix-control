import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { flushPromises } from '@vue/test-utils'
import { setLocale } from '@/i18n'
import {
  CERTIFICATE_STATES, CONNECTION_TYPES, certificateOf, connectionTone, connectionTypeOf, indexTransports, nodeRef
} from '@/views/admin/nodes/agentConnection'
import { useNodeTransports } from '@/views/admin/nodes/useNodeTransports'
import NodeConnectionBadge from '@/views/admin/nodes/NodeConnectionBadge.vue'
import NodeCertificateCell from '@/views/admin/nodes/NodeCertificateCell.vue'

const kernelApi = vi.hoisted(() => ({ getKernelAgentTransports: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)

// The transport inventory row of a node (GET /api/v4/kernel/agents/transports):
// connection type, certificate state and the newest certificate in any state.
const NOW = Date.parse('2026-10-05T08:00:00Z')
const record = (extra = {}) => ({
  serial: 'ab12', issued_at: '2026-10-01T00:00:00Z', not_after: '2026-10-08T00:00:00Z', renew_after: '2026-10-05T16:00:00Z', revoked_at: null, ...extra
})

describe('agent connection model', () => {
  it('names a proxy node the way the inventory does and indexes the answer by that name', () => {
    expect(nodeRef(7)).toBe('proxy-7')
    const index = indexTransports({ nodes: [{ node: 'proxy-5', name: 'hk' }, { node: 'forward-9' }, null, {}] })
    expect([...index.keys()]).toEqual(['proxy-5', 'forward-9'])
    expect(index.get('proxy-5').name).toBe('hk')
    expect(indexTransports(null).size).toBe(0)
    expect(indexTransports({ nodes: 'nope' }).size).toBe(0)
  })

  it('tones the five connection types, and reads an unknown or missing one as offline', () => {
    expect(CONNECTION_TYPES).toEqual(['mtls_stream', 'apikey_stream', 'legacy', 'third_party', 'offline'])
    expect(CONNECTION_TYPES.map(connectionTone)).toEqual(['success', 'info', 'warning', 'neutral', 'neutral'])
    expect(connectionTypeOf({ connection: { type: 'legacy' } })).toBe('legacy')
    expect(connectionTypeOf({ connection: { type: 'carrier-pigeon' } })).toBe('offline')
    expect(connectionTypeOf({})).toBe('offline')
    expect(connectionTypeOf(null)).toBe('offline')
  })

  it('reads a valid certificate as valid, and as overdue once past renew_after (the Agent did not renew in time)', () => {
    const valid = certificateOf({ certificate_state: 'valid', last_certificate: record({ renew_after: '2026-10-06T00:00:00Z' }) }, NOW)
    expect(valid).toMatchObject({ state: 'valid', overdue: false, tone: 'success', notAfter: '2026-10-08T00:00:00Z' })
    const overdue = certificateOf({ certificate_state: 'valid', last_certificate: record({ renew_after: '2026-10-05T07:59:59Z' }) }, NOW)
    expect(overdue).toMatchObject({ state: 'valid', overdue: true, tone: 'warning' })
  })

  it('keeps when and why a revoked or expired certificate ended, and never calls it overdue', () => {
    const revoked = certificateOf({
      certificate_state: 'revoked',
      last_certificate: record({ renew_after: '2000-01-01T00:00:00Z', revoked_at: '2026-10-03T00:00:00Z', revoke_reason: 'node disabled' })
    }, NOW)
    expect(revoked).toMatchObject({ state: 'revoked', overdue: false, tone: 'danger', revokedAt: '2026-10-03T00:00:00Z', revokeReason: 'node disabled' })
    expect(certificateOf({ certificate_state: 'expired', last_certificate: record() }, NOW)).toMatchObject({ state: 'expired', tone: 'danger' })
  })

  it('reads no record, or an unknown state, as none', () => {
    expect(CERTIFICATE_STATES).toEqual(['valid', 'revoked', 'expired', 'none'])
    expect(certificateOf({ certificate_state: 'none', last_certificate: null }, NOW)).toMatchObject({ state: 'none', tone: 'neutral', notAfter: null, overdue: false })
    expect(certificateOf({ certificate_state: 'mystery' }, NOW).state).toBe('none')
    expect(certificateOf(null, NOW).state).toBe('none')
  })

  it('falls back to the valid certificate when the row has no last_certificate', () => {
    expect(certificateOf({ certificate_state: 'valid', certificate: record() }, NOW).serial).toBe('ab12')
  })
})

describe('useNodeTransports', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  it('asks for the inventory names of the ids in one call and indexes the answer', async () => {
    kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [{ node: 'proxy-5' }] })
    const transports = useNodeTransports()
    expect(transports.status.value).toBe('idle')
    await transports.load([5, 6])
    expect(kernelApi.getKernelAgentTransports).toHaveBeenCalledWith({ nodes: ['proxy-5', 'proxy-6'] })
    expect(transports.status.value).toBe('ready')
    expect(transports.entries.value.has('proxy-5')).toBe(true)
  })

  it('makes no request for no ids and clears what it had', async () => {
    kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [{ node: 'proxy-5' }] })
    const transports = useNodeTransports()
    await transports.load([5])
    await transports.load([])
    expect(kernelApi.getKernelAgentTransports).toHaveBeenCalledTimes(1)
    expect(transports.entries.value.size).toBe(0)
    expect(transports.status.value).toBe('idle')
  })

  it('keeps nothing and says error when the call fails', async () => {
    kernelApi.getKernelAgentTransports.mockRejectedValue(new Error('403'))
    const transports = useNodeTransports()
    await transports.load([5])
    expect(transports.status.value).toBe('error')
    expect(transports.entries.value.size).toBe(0)
  })

  it('drops the answer of a request that a newer one (or a reset) has replaced', async () => {
    let releaseFirst
    kernelApi.getKernelAgentTransports
      .mockReturnValueOnce(new Promise((resolve) => { releaseFirst = resolve }))
      .mockResolvedValueOnce({ nodes: [{ node: 'proxy-6', second: true }] })
    const transports = useNodeTransports()
    const first = transports.load([5])
    await transports.load([6])
    releaseFirst({ nodes: [{ node: 'proxy-5' }] })
    await first
    expect([...transports.entries.value.keys()]).toEqual(['proxy-6'])

    let releaseThird
    kernelApi.getKernelAgentTransports.mockReturnValueOnce(new Promise((resolve) => { releaseThird = resolve }))
    const third = transports.load([7])
    transports.reset()
    releaseThird({ nodes: [{ node: 'proxy-7' }] })
    await third
    expect(transports.entries.value.size).toBe(0)
    expect(transports.status.value).toBe('idle')
  })
})

describe('connection chip and certificate cell', () => {
  beforeEach(async () => {
    await setLocale('en')
    vi.useRealTimers()
  })

  it('shows the chip with its words and a hint, a dash while loading and "Unavailable" when it failed', async () => {
    const chip = mount(NodeConnectionBadge, { props: { entry: { connection: { type: 'third_party' } }, status: 'ready' } })
    expect(chip.text()).toBe('Third-party')
    expect(chip.get('[data-testid="node-connection"]').attributes('title')).toContain('UniProxy or v2board gRPC')
    const loading = mount(NodeConnectionBadge, { props: { status: 'loading' } })
    expect(loading.get('[aria-hidden="true"]').text()).toBe('—')
    expect(loading.get('.visually-hidden').text()).toBe('Loading the Agent connection…')
    expect(mount(NodeConnectionBadge, { props: { status: 'error' } }).text()).toBe('Unavailable')
    // The inventory has no row for the node.
    const none = mount(NodeConnectionBadge, { props: { entry: null, status: 'ready' } })
    expect(none.text()).toBe('—')
    expect(none.find('.visually-hidden').exists()).toBe(false)
  })

  it('shows the certificate state as a word with the day that matters', async () => {
    const cell = entry => mount(NodeCertificateCell, { props: { entry, status: 'ready' } })
    expect(cell({ certificate_state: 'valid', last_certificate: record({ renew_after: '2999-01-01T12:00:00Z', not_after: '2999-02-01T12:00:00Z' }) }).text()).toBe('Validuntil 2999-02-01')
    expect(cell({ certificate_state: 'valid', last_certificate: record({ renew_after: '2000-01-01T12:00:00Z', not_after: '2999-02-01T12:00:00Z' }) }).text()).toBe('Renewal overdueuntil 2999-02-01')
    expect(cell({ certificate_state: 'expired', last_certificate: record({ not_after: '2026-09-01T12:00:00Z' }) }).text()).toBe('Expiredexpired 2026-09-01')
    expect(cell({ certificate_state: 'revoked', last_certificate: record({ revoked_at: '2026-10-03T12:00:00Z' }) }).text()).toBe('Revokedrevoked 2026-10-03')
    expect(cell({ certificate_state: 'none', last_certificate: null }).text()).toBe('No certificate')
    await flushPromises()
  })
})
