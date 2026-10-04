import { beforeEach, describe, expect, it, vi } from 'vitest'

const request = vi.hoisted(() => vi.fn())
vi.mock('@/utils/request', () => ({ default: request }))

const api = await import('@/api/forwardV4')

beforeEach(() => {
  request.mockReset()
})

describe('forward v4 API client', () => {
  it('sends only contract fields, with 64-bit integers as strings and unset fields left out', () => {
    const body = api.routeBody({
      id: '01J', owner: 'admin', revision: 7, name: ' edge ', paused: false, unknown: 'x',
      listen: { address: '', port: 0, protocol: 'L4_PROTOCOL_TCP', portMode: 'auto' },
      hops: [
        { _key: 'k1', role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_GOST', node_refs: ['forward-41'], port: 0, portMode: 'auto', dial_address: '' },
        { _key: 'k2', role: 'HOP_ROLE_EXIT', engine: 'ENGINE_GOST', node_refs: ['forward-61'], ingress: { security: 'LINK_SECURITY_TLS', mux: true, server_name: '', path: '' }, port: 30020 }
      ],
      targets: [{ _key: 't', host: 'a.example', port: 443, weight: 0, priority: null }],
      policy: { next_hop: 'BALANCE_STRATEGY_FAILOVER', health: { interval_ms: 0, disabled: false }, circuit_breaker: {} },
      limits: { bandwidth_bps: 500000000, quota_bytes: '0', expires_at_unix_ms: '1759579200000', max_conns: 0 },
      labels: { team: 'game', ' ': 'dropped' }
    })
    expect(body).toEqual({
      id: '01J', owner: 'admin', revision: '7', name: 'edge',
      listen: { protocol: 'L4_PROTOCOL_TCP' },
      hops: [
        { role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_GOST', node_refs: ['forward-41'] },
        { role: 'HOP_ROLE_EXIT', engine: 'ENGINE_GOST', node_refs: ['forward-61'], ingress: { security: 'LINK_SECURITY_TLS', mux: true }, port: 30020 }
      ],
      targets: [{ host: 'a.example', port: 443 }],
      policy: { next_hop: 'BALANCE_STRATEGY_FAILOVER' },
      limits: { bandwidth_bps: '500000000', expires_at_unix_ms: '1759579200000' },
      labels: { team: 'game' }
    })
  })

  it('puts an Idempotency-Key on every write and none on reads', async () => {
    request.mockResolvedValue({ data: { route: { id: '01J' } } })
    await api.createRoute({ name: 'a', id: 'x', revision: '3' }, { idempotencyKey: 'fwd-one' })
    expect(request.mock.calls[0][0]).toMatchObject({ method: 'post', url: '/routes', baseURL: '/api/v4/forward', headers: { 'Idempotency-Key': 'fwd-one' }, data: { name: 'a' } })
    await api.pauseRoute('01J')
    expect(request.mock.calls[1][0].headers['Idempotency-Key']).toMatch(/^fwd-/)
    await api.getRoute('01J')
    expect(request.mock.calls[2][0].headers).toEqual({})
    expect(request.mock.calls[2][0].method).toBe('get')
  })

  it('unwraps {data} and reads can_delete from the lists', async () => {
    request.mockResolvedValueOnce({ data: { routes: [{ route: { id: 'r' } }], can_delete: false } })
    expect(await api.listRoutes()).toEqual({ routes: [{ route: { id: 'r' } }], canDelete: false, truncated: false })
    request.mockResolvedValueOnce({ data: { nodes: [] } })
    expect(await api.listNodes()).toEqual({ nodes: [], canDelete: null })
  })

  it('turns refusals into errors with their code and violations', async () => {
    request.mockRejectedValue({ response: { status: 400, data: { error: { code: 'invalid_route', message: 'bad', violations: [{ field: 'name', code: 'required', message: 'name is required' }] } } } })
    const error = await api.createRoute({ name: '' }).catch(cause => cause)
    expect(error).toBeInstanceOf(api.ForwardApiError)
    expect(error).toMatchObject({ status: 400, code: 'invalid_route', message: 'bad' })
    expect(error.violations).toEqual([{ field: 'name', code: 'required', message: 'name is required' }])

    request.mockRejectedValue({ name: 'CanceledError', code: 'ERR_CANCELED' })
    const canceled = await api.previewRoute({ name: 'x' }).catch(cause => cause)
    expect(api.isCanceled(canceled)).toBe(true)
  })

  it('previews {route} without its revision and passes the abort signal', async () => {
    request.mockResolvedValue({ data: { states: [] } })
    const controller = new AbortController()
    await api.previewRoute({ id: '01J', revision: '7', name: 'x' }, { signal: controller.signal })
    expect(request.mock.calls[0][0]).toMatchObject({ url: '/routes/preview', data: { route: { id: '01J', name: 'x' } }, signal: controller.signal })
  })
})
