import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as kernelApi from '@/api/kernel'

const mockRequest = vi.hoisted(() => vi.fn())

vi.mock('@/utils/request', () => ({ default: mockRequest }))

describe('kernel API', () => {
  beforeEach(() => {
    mockRequest.mockReset()
    mockRequest.mockResolvedValue({ data: [] })
  })

  it('sends every request through the v3 API root', async () => {
    const calls = [
      [kernelApi.getKernelPlugins, '/plugins'],
      [kernelApi.getKernelPluginReleases, '/plugin-releases'],
      [kernelApi.getKernelInstallations, '/plugin-installations'],
      [() => kernelApi.getKernelNodeAssignments(11), '/nodes/11/assignments'],
      [() => kernelApi.getKernelInstallationConfig(7), '/plugin-installations/7/config'],
      [kernelApi.getKernelScopes, '/service-scopes'],
      [kernelApi.getKernelAccessGroups, '/access-groups'],
      [() => kernelApi.getKernelAccessGroupDetail(7), '/access-groups/7'],
      [kernelApi.getKernelExtensions, '/extensions']
    ]

    for (const [call, url] of calls) {
      await call()
      expect(mockRequest).toHaveBeenLastCalledWith({
        baseURL: '/api/v3',
        url,
        method: 'get'
      })
    }
  })

  it('unwraps the v3 data envelope and tolerates direct payloads', async () => {
    const rows = [{ id: 'machine-telemetry' }]
    mockRequest.mockResolvedValueOnce({ data: rows })
    await expect(kernelApi.getKernelPlugins()).resolves.toEqual(rows)

    mockRequest.mockResolvedValueOnce(rows)
    await expect(kernelApi.getKernelPlugins()).resolves.toEqual(rows)
  })

  it('writes an installation config with an optional optimistic revision', async () => {
    await kernelApi.updateKernelInstallationConfig(7, { port: 443 }, 2)
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/config',
      method: 'put',
      data: { config: { port: 443 }, expected_revision: 2 },
      rawResponse: true
    })

    await kernelApi.updateKernelInstallationConfig(7, { port: 8443 })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/config',
      method: 'put',
      data: { config: { port: 8443 } },
      rawResponse: true
    })
  })

  it('reads releases and restarts a built-in package host', async () => {
    await kernelApi.getKernelPluginReleases('machine-telemetry')
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-releases',
      method: 'get',
      params: { plugin_id: 'machine-telemetry' }
    })

    mockRequest.mockResolvedValueOnce({
      data: { data: { operation: { id: 'operation-1', state: 'pending' } } },
      headers: {
        'x-anixops-operation-id': 'operation-1',
        'x-anixops-operation-chain': 'operation-1'
      }
    })
    await expect(kernelApi.restartKernelInstallation(7, 'request-1')).resolves.toEqual({
      operation: { id: 'operation-1', state: 'pending' },
      operation_id: 'operation-1',
      operation_chain: 'operation-1'
    })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/actions',
      method: 'post',
      data: { action: 'restart', idempotency_key: 'request-1' },
      rawResponse: true
    })
  })

  it('has no client for the removed import, install and route-mode routes', () => {
    for (const name of [
      'registerKernelPluginRelease', 'uploadKernelPluginReleaseArtifact', 'upsertKernelInstallation', 'runKernelInstallationAction',
      'getKernelRouteModes', 'setKernelRouteModes', 'rollbackKernelRouteModes', 'getKernelRouteModeRevisions', 'getKernelRouteModeMismatches'
    ]) {
      expect(kernelApi[name], name).toBeUndefined()
    }
  })

  it('reads the agent transport inventory through the v4 kernel', async () => {
    await kernelApi.getKernelAgentTransports()
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get' })
    // The node list and the node page ask for the nodes in view: one
    // comma separated `node` value, repeats dropped.
    await kernelApi.getKernelAgentTransports({ nodes: ['proxy-3', 'proxy-5', 'proxy-3'] })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get', params: { node: 'proxy-3,proxy-5' } })
    await kernelApi.getKernelAgentTransports({ nodes: ['proxy-7'] })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get', params: { node: 'proxy-7' } })
    await kernelApi.getKernelAgentTransports({ nodes: [] })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get' })
    await kernelApi.listKernelAgentUpgrades(1)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/upgrades', method: 'get', params: { limit: 1 } })
    await kernelApi.getKernelAgentUpgrade('c/1')
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/upgrades/c%2F1', method: 'get' })
    await kernelApi.startKernelAgentUpgrade({ target_version: 'v4.2.0' })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/upgrades', method: 'post', data: { target_version: 'v4.2.0' } })
    await kernelApi.pauseKernelAgentUpgrade('c1')
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/upgrades/c1/pause', method: 'post' })
    await kernelApi.resumeKernelAgentUpgrade('c1')
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/upgrades/c1/resume', method: 'post' })
    await kernelApi.abortKernelAgentUpgrade('c1', true)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/upgrades/c1/abort', method: 'post', data: { rollback: true } })
  })
  it('reads a proxy node\'s traffic series through the v4 kernel, leaving out what is not given', async () => {
    mockRequest.mockResolvedValue({ data: { node_id: 7, granularity: 'hour', points: [], total: { up_bytes: 0, down_bytes: 0 } } })
    // The answer is the route's {data}, unwrapped like its siblings.
    await expect(kernelApi.getKernelNodeTraffic(7)).resolves.toMatchObject({ node_id: 7, granularity: 'hour' })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/nodes/7/traffic', method: 'get' })
    await kernelApi.getKernelNodeTraffic(7, { granularity: 'day' })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/nodes/7/traffic', method: 'get', params: { granularity: 'day' } })
    await kernelApi.getKernelNodeTraffic('12', { granularity: 'hour', since: 1_790_000_000_000, until: 1_790_086_400_000 })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v4', url: '/kernel/nodes/12/traffic', method: 'get', params: { granularity: 'hour', since: 1_790_000_000_000, until: 1_790_086_400_000 }
    })
    // An id with a slash cannot change the path.
    await kernelApi.getKernelNodeTraffic('1/../2')
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/nodes/1%2F..%2F2/traffic', method: 'get' })
  })

  it('rotates a node\'s Agent credentials with only the fields that were asked for', async () => {
    mockRequest.mockResolvedValue({ data: { node: 'proxy-12', credential: 'anixagt_x' } })
    await expect(kernelApi.rotateKernelAgentCredentials({ node: 'proxy-12' })).resolves.toEqual({ node: 'proxy-12', credential: 'anixagt_x' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v4', url: '/kernel/agents/rotate-credentials', method: 'post', data: { node: 'proxy-12' }, timeout: 30_000
    })
    await kernelApi.rotateKernelAgentCredentials({ node: 'proxy-12', rotateApiKey: true, ttlSeconds: 86400, reason: '  disk of the host was stolen  ' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v4', url: '/kernel/agents/rotate-credentials', method: 'post',
      data: { node: 'proxy-12', rotate_api_key: true, ttl_seconds: 86400, reason: 'disk of the host was stolen' }, timeout: 30_000
    })
    // A blank reason and a false flag are not sent.
    await kernelApi.rotateKernelAgentCredentials({ node: 'forward-3', rotateApiKey: false, reason: '   ' })
    expect(mockRequest.mock.lastCall[0].data).toEqual({ node: 'forward-3' })
  })

  it('lists the caller\'s API tokens, and adds only the filters that were asked for', async () => {
    const rows = [{ id: 'tok-1', name: 'nightly export', hint: 'k3Zq' }]
    mockRequest.mockResolvedValue({ data: rows })
    await expect(kernelApi.listKernelApiTokens()).resolves.toEqual(rows)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/api-tokens', method: 'get' })
    await kernelApi.listKernelApiTokens({ includeInactive: true })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/api-tokens', method: 'get', params: { include_inactive: true } })
    await kernelApi.listKernelApiTokens({ all: true, includeInactive: true })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v4', url: '/kernel/api-tokens', method: 'get', params: { include_inactive: true, all: true }
    })
    // One administrator's tokens; everyone's wins over it.
    await kernelApi.listKernelApiTokens({ userId: 9 })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/api-tokens', method: 'get', params: { user_id: 9 } })
    await kernelApi.listKernelApiTokens({ userId: 9, all: true })
    expect(mockRequest.mock.lastCall[0].params).toEqual({ all: true })
  })

  it('creates an API token with only the fields that are set, marks the request sensitive, and answers the token and its record', async () => {
    mockRequest.mockResolvedValue({ data: { token: 'anixadm_x', api_token: { id: 'tok-1' } } })
    await expect(kernelApi.createKernelApiToken({ name: '  nightly export ', scope: 'read', expiresInDays: 90, password: 'pw' }))
      .resolves.toEqual({ token: 'anixadm_x', api_token: { id: 'tok-1' } })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v4', url: '/kernel/api-tokens', method: 'post',
      data: { name: 'nightly export', scope: 'read', expires_in_days: 90, password: 'pw' }, timeout: 30_000, sensitive: true
    })
    // No expiry, no password: a code and its method instead.
    await kernelApi.createKernelApiToken({ name: 'probe', scope: 'admin', expiresInDays: 0, code: '123456', method: 'totp' })
    expect(mockRequest.mock.lastCall[0].data).toEqual({ name: 'probe', scope: 'admin', code: '123456', method: 'totp' })
    await kernelApi.createKernelApiToken({ name: 'probe', scope: 'read', code: 'AB12-CD34', method: 'backup' })
    expect(mockRequest.mock.lastCall[0].data).toEqual({ name: 'probe', scope: 'read', code: 'AB12-CD34', method: 'backup' })
    // A code without a method is a TOTP code.
    await kernelApi.createKernelApiToken({ name: 'probe', scope: 'read', code: '123456' })
    expect(mockRequest.mock.lastCall[0].data.method).toBe('totp')
  })

  it('revokes an API token by id, which cannot change the path', async () => {
    mockRequest.mockResolvedValue({ data: { api_token: { id: 'tok-1' }, changed: true } })
    await expect(kernelApi.revokeKernelApiToken('6b0e-1')).resolves.toEqual({ api_token: { id: 'tok-1' }, changed: true })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/api-tokens/6b0e-1', method: 'delete' })
    await kernelApi.revokeKernelApiToken('1/../2')
    expect(mockRequest.mock.lastCall[0].url).toBe('/kernel/api-tokens/1%2F..%2F2')
  })
})
