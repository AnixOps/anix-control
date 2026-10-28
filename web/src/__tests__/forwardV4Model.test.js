import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, reactive } from 'vue'
import { mount } from '@vue/test-utils'
import {
  diagnosisStages, draftToRoute, entryTraffic, hourlySeries, latencySeries, missingRequired, routeDiff, routeStatus, routeToDraft,
  runBulk, setDraftEngine, statusContext, underField, fieldId
} from '@/components/forward/routeModel'
import { useRoutePreview } from '@/components/forward/useRoutePreview'

const HOUR = 3_600_000

describe('route status', () => {
  const route = { id: 'r1', hops: [{ node_refs: ['forward-1'] }] }
  const context = statusContext({
    nodes: [{ node_ref: 'forward-1', desired_generation: '5', reported: true, reported_generation: '4' }],
    hopErrors: [{ route_id: 'r1', message: 'bind' }],
    targets: [{ route_id: 'r1', state: 'HEALTH_STATE_CIRCUIT_OPEN' }]
  })

  it('puts enforced before paused before health (D8)', () => {
    expect(routeStatus({ route: { ...route, paused: true }, enforced: 'quota' }, context)).toBe('quota')
    expect(routeStatus({ route: { ...route, paused: true }, enforced: 'expired' }, context)).toBe('expired')
    expect(routeStatus({ route: { ...route, paused: true } }, context)).toBe('paused')
    expect(routeStatus({ route }, context)).toBe('error')
    expect(routeStatus({ route }, { ...context, hopErrorRoutes: new Set() })).toBe('degraded')
    expect(routeStatus({ route }, { ...context, hopErrorRoutes: new Set(), degradedRoutes: new Set() })).toBe('syncing')
    expect(routeStatus({ route }, {})).toBe('healthy')
  })
})

describe('the editor draft', () => {
  it('round-trips a stored route and converts the limit units', () => {
    const stored = {
      id: '01J', owner: 'admin', revision: '7', name: 'game',
      listen: { address: '0.0.0.0', port: 30443, protocol: 'L4_PROTOCOL_TCP' },
      hops: [{ role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_NFTABLES', node_refs: ['forward-41'] }, { role: 'HOP_ROLE_EXIT', engine: 'ENGINE_GOST', node_refs: ['forward-61'], ingress: { security: 'LINK_SECURITY_TLS', mux: true } }],
      targets: [{ host: 'a.example', port: 443, weight: 2 }],
      limits: { bandwidth_bps: '500000000', quota_bytes: String(500 * 1024 ** 3) },
      labels: { team: 'game' }
    }
    const draft = routeToDraft(stored)
    expect(draft.limits.bandwidthMbps).toBe(500)
    expect(draft.limits.quotaGB).toBe(500)
    expect(draft.listen.portMode).toBe('explicit')
    const back = draftToRoute(draft)
    expect(back.limits.bandwidth_bps).toBe('500000000')
    expect(back.limits.quota_bytes).toBe(String(500 * 1024 ** 3))
    expect(back.hops[1].ingress).toEqual({ security: 'LINK_SECURITY_TLS', mux: true, server_name: '', path: '' })
    expect(back.labels).toEqual({ team: 'game' })
    expect(back.revision).toBe('7')
  })

  it('lists the required fields a preview waits for (D4)', () => {
    const draft = routeToDraft()
    expect(missingRequired(draft)).toEqual(['name', 'hops[0].node_refs', 'targets[0].host', 'targets[0].port'])
    draft.name = 'x'
    draft.hops[0].node_refs.push('forward-41')
    draft.targets[0].host = 'a.example'
    draft.targets[0].port = 443
    expect(missingRequired(draft)).toEqual([])
  })

  it('puts every hop on the network model or none', () => {
    const draft = routeToDraft({
      name: 'sh-hk',
      listen: { port: 0, protocol: 'L4_PROTOCOL_TCP' },
      hops: [
        { engine: 'ENGINE_GOST', node_refs: ['proxy-1'] },
        { engine: 'ENGINE_GOST', node_refs: ['proxy-2', 'proxy-3'], port: 30000, ingress: { security: 'LINK_SECURITY_TLS', server_name: 'a.example' } }
      ],
      targets: [{ host: 'a.example', port: 443 }],
      policy: { direct: 'DIRECT_MODE_PREFERRED', proxy_protocol: 'PROXY_PROTOCOL_V1' }
    })
    setDraftEngine(draft, 1, 'ENGINE_NETWORK')
    expect(draft.hops.map(hop => hop.engine)).toEqual(['ENGINE_NETWORK', 'ENGINE_NETWORK'])
    expect(draft.hops[1].node_refs).toEqual(['proxy-2'])
    expect(draft.policy.direct).toBe('DIRECT_MODE_OFF')
    expect(missingRequired(draft)).toEqual(['hops[1].ingress.network_transport_id', 'hops[1].network_egress_id', 'listen.port'])
    draft.hops[1].ingress.network_transport_id = 'tr-hk'
    draft.hops[1].network_egress_id = 'eg-hk'
    draft.listen.port = 20022
    expect(missingRequired(draft)).toEqual([])
    const route = draftToRoute(draft)
    expect(route.hops[1]).toEqual({
      role: 'HOP_ROLE_EXIT', engine: 'ENGINE_NETWORK', node_refs: ['proxy-2'], port: 0, dial_address: '',
      ingress: { security: 'LINK_SECURITY_NETWORK', network_transport_id: 'tr-hk' }, network_egress_id: 'eg-hk'
    })
    expect(route.hops[0].network_egress_id).toBeUndefined()
    expect(route.policy.proxy_protocol).toBe('PROXY_PROTOCOL_V1')
    expect(routeToDraft(route).hops[1].network_egress_id).toBe('eg-hk')

    setDraftEngine(draft, 0, 'ENGINE_GOST')
    expect(draft.hops.map(hop => hop.engine)).toEqual(['ENGINE_GOST', 'ENGINE_GOST'])
    expect(draft.hops[1].ingress.security).toBe('LINK_SECURITY_RAW')
    expect(draft.hops[1].network_egress_id).toBe('')
  })

  it('maps violation fields to DOM ids and hop prefixes', () => {
    expect(fieldId('hops[1].ingress.security')).toBe('fwd-f-hops-1-ingress-security')
    const violations = [{ field: 'hops[1].ingress.security', code: 'link_unsupported' }]
    expect(underField(violations, 'hops[1]')).toBe(true)
    expect(underField(violations, 'hops[0]')).toBe(false)
  })
})

describe('traffic', () => {
  it('counts the entry hop only, once per route (D6)', () => {
    const totals = [
      { route_id: 'r1', node_ref: 'a', up_bytes: 10, down_bytes: 100 },
      { route_id: 'r1', hop_index: 1, node_ref: 'b', up_bytes: 10, down_bytes: 100 },
      { route_id: 'r1', node_ref: 'c', up_bytes: 1, down_bytes: 1 }
    ]
    expect(entryTraffic(totals).get('r1')).toEqual({ up: 11, down: 101 })
  })

  it('fills every hour of the window', () => {
    const since = 10 * HOUR
    const series = hourlySeries([
      { route_id: 'r', hour_start_unix_ms: String(11 * HOUR), up_bytes: '5', down_bytes: '7' },
      { route_id: 'r', hop_index: 1, hour_start_unix_ms: String(11 * HOUR), up_bytes: '5', down_bytes: '7' }
    ], { since, until: since + 3 * HOUR })
    expect(series).toEqual([{ hour: 10 * HOUR, up: 0, down: 0 }, { hour: 11 * HOUR, up: 5, down: 7 }, { hour: 12 * HOUR, up: 0, down: 0 }])
  })
})

describe('bulk actions (D9)', () => {
  it('runs at most four at a time and reports the failures', async () => {
    let running = 0
    let peak = 0
    const items = Array.from({ length: 10 }, (_, index) => index)
    const result = await runBulk(items, async item => {
      running++
      peak = Math.max(peak, running)
      await new Promise(resolve => setTimeout(resolve, 5))
      running--
      if (item === 3 || item === 7) throw new Error(`no ${item}`)
    })
    expect(peak).toBe(4)
    expect(result.done).toEqual([0, 1, 2, 4, 5, 6, 8, 9])
    expect(result.failed.map(entry => entry.item)).toEqual([3, 7])
  })
})

describe('conflicts (D10)', () => {
  it('lists what the other writer changed and where the form changed it too', () => {
    const base = { name: 'a', limits: { max_conns: 10 }, revision: '7' }
    const theirs = { name: 'a', limits: { max_conns: 20 }, labels: { team: 'x' }, revision: '8' }
    const mine = { name: 'b', limits: { max_conns: 30 }, revision: '7' }
    expect(routeDiff(base, theirs, mine)).toEqual([
      { field: 'labels.team', before: '', theirs: 'x', mine: '', overlap: false },
      { field: 'limits.max_conns', before: '10', theirs: '20', mine: '30', overlap: true }
    ])
  })
})

describe('diagnosis stages', () => {
  it('groups records, node probes and Control probes', () => {
    const stages = diagnosisStages({
      steps: [
        { kind: 'PROBE_KIND_CONFIG', vantage: 'DIAGNOSE_VANTAGE_CONTROL' },
        { kind: 'PROBE_KIND_TCP_CONNECT', vantage: 'DIAGNOSE_VANTAGE_NODE' },
        { kind: 'PROBE_KIND_TCP_CONNECT', vantage: 'DIAGNOSE_VANTAGE_CONTROL' },
        { kind: 'PROBE_KIND_HEALTH', vantage: 'DIAGNOSE_VANTAGE_CONTROL' }
      ]
    })
    expect(stages.map(stage => [stage.key, stage.steps.length])).toEqual([['control', 2], ['node', 1], ['controlProbe', 1]])
  })
})

describe('the plan preview (D4)', () => {
  function harness(request, missing = () => []) {
    const draft = reactive({ name: 'a' })
    let preview
    const wrapper = mount(defineComponent({
      setup() {
        preview = useRoutePreview(() => ({ ...draft }), { request, missing })
        return () => h('div')
      }
    }))
    return { draft, preview: () => preview, wrapper }
  }

  it('waits 1 s after the last change and cancels the preview in flight', async () => {
    vi.useFakeTimers()
    const signals = []
    const request = vi.fn((route, { signal }) => new Promise((resolve, reject) => {
      signals.push(signal)
      signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { canceled: true })))
      setTimeout(() => resolve({ states: [], violations: [], name: route.name }), 500)
    }))
    const { draft, wrapper } = harness(request)
    draft.name = 'b'
    await nextTick()
    await vi.advanceTimersByTimeAsync(600)
    draft.name = 'c'
    await nextTick()
    await vi.advanceTimersByTimeAsync(900)
    expect(request).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(100)
    expect(request).toHaveBeenCalledTimes(1)
    expect(request.mock.calls[0][0].name).toBe('c')
    // A change while it runs cancels it; the next one starts 1 s later.
    await vi.advanceTimersByTimeAsync(200)
    draft.name = 'd'
    await nextTick()
    expect(signals[0].aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(request).toHaveBeenCalledTimes(2)
    expect(signals[0].aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(600)
    wrapper.unmount()
  })

  it('never asks while a required field is empty', async () => {
    vi.useFakeTimers()
    const request = vi.fn()
    const { draft, preview, wrapper } = harness(request, () => ['name'])
    draft.name = ''
    await nextTick()
    await vi.advanceTimersByTimeAsync(1500)
    expect(request).not.toHaveBeenCalled()
    expect(preview().skipped.value).toBe(true)
    wrapper.unmount()
  })
})

describe('latency series', () => {
  it('weights each hop by its successful checks and keeps failures apart', () => {
    const series = latencySeries([
      { hop_index: 0, node_ref: 'forward-1', bucket_start_unix_ms: '600000', samples: 4, failures: 1, rtt_avg_us: 20_000, rtt_max_us: 30_000 },
      { hop_index: 0, node_ref: 'forward-2', bucket_start_unix_ms: '600000', samples: 1, failures: 0, rtt_avg_us: 40_000, rtt_max_us: 40_000 },
      { hop_index: 1, node_ref: 'forward-3', bucket_start_unix_ms: '600000', samples: 2, failures: 2, rtt_avg_us: 0, rtt_max_us: 0 },
      { hop_index: 0, node_ref: 'forward-1', bucket_start_unix_ms: '300000', samples: 2, failures: 0, rtt_avg_us: 10_000, rtt_max_us: 12_000 }
    ])
    expect(series.hops).toEqual([0, 1])
    expect(series.points.map(point => point.at)).toEqual([300_000, 600_000])
    expect(series.points[0].hops[0]).toEqual({ avgMs: 10, maxMs: 12, failures: 0, samples: 2 })
    expect(series.points[1].hops[0]).toEqual({ avgMs: 25, maxMs: 40, failures: 1, samples: 5 })
    expect(series.points[1].hops[1]).toEqual({ avgMs: null, maxMs: null, failures: 2, samples: 2 })
  })

  it('answers no points without buckets', () => {
    expect(latencySeries([])).toEqual({ hops: [], points: [] })
  })
})
