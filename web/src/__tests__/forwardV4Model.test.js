import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, reactive } from 'vue'
import { mount } from '@vue/test-utils'
import {
  diagnosisStages, draftToRoute, entryTraffic, hourlySeries, missingRequired, routeDiff, routeStatus, routeToDraft,
  runBulk, statusContext, underField, fieldId
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
