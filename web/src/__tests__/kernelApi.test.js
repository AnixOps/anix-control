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
      [kernelApi.getKernelTopologies, '/topologies'],
      [() => kernelApi.getKernelTopologyRevisions(3), '/topologies/3/revisions'],
      [() => kernelApi.getKernelTopologyRevision(3, 7), '/topologies/3/revisions/7'],
      [kernelApi.getKernelDeployments, '/deployments'],
      [kernelApi.getKernelOperations, '/operations'],
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

  it('uploads release artifacts through the v3 package repository endpoint', async () => {
    await kernelApi.registerKernelPluginRelease('{"id":"machine-telemetry"}', 'signature')
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-releases',
      method: 'post',
      data: { manifest: '{"id":"machine-telemetry"}', signature: 'signature' }
    })

    await kernelApi.getKernelPluginReleases('machine-telemetry')
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-releases',
      method: 'get',
      params: { plugin_id: 'machine-telemetry' }
    })

    await kernelApi.uploadKernelPluginReleaseArtifact(9, 'YXJ0aWZhY3Q=')
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-releases/9/artifact',
      method: 'post',
      data: { artifact_base64: 'YXJ0aWZhY3Q=' },
      timeout: 120_000
    })
  })

  it('upserts installations and dispatches idempotent lifecycle actions', async () => {
    const installation = { plugin_id: 'machine-telemetry', target: 'control', desired_version: '1.1.0', enabled: true }
    await kernelApi.upsertKernelInstallation(installation)
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations',
      method: 'put',
      data: installation,
      rawResponse: true
    })

    await kernelApi.runKernelInstallationAction(7, 'update', { targetVersion: '1.1.0', idempotencyKey: 'request-1' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/actions',
      method: 'post',
      data: { action: 'update', target_version: '1.1.0', idempotency_key: 'request-1' },
      rawResponse: true
    })

    await kernelApi.runKernelInstallationAction(7, 'rollback', { idempotencyKey: 'request-2' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/actions',
      method: 'post',
      data: { action: 'rollback', idempotency_key: 'request-2' },
      rawResponse: true
    })
  })

  it('manages node service assignments through the kernel routes', async () => {
    const assignment = {
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 4,
      enabled: true,
      rollout_group: 'canary'
    }
    await kernelApi.upsertKernelNodeAssignment(11, assignment)
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/nodes/11/assignments',
      method: 'put',
      data: assignment
    })

    await kernelApi.deleteKernelNodeAssignment(11, 7)
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/nodes/11/assignments/7',
      method: 'delete'
    })
  })

  it('manages service-scope access groups, memberships, grants, quotas, and effective access through the kernel routes', async () => {
    await kernelApi.getKernelAccessGroups('forward')
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3', url: '/access-groups', method: 'get', params: { scope_id: 'forward' }
    })

    const group = { scope_id: 'forward', name: 'canary', description: 'Canary access', enabled: true }
    await kernelApi.createKernelAccessGroup(group)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/access-groups', method: 'post', data: group })
    await kernelApi.updateKernelAccessGroup(7, { name: 'canary', description: '', enabled: false })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/access-groups/7', method: 'put', data: { name: 'canary', description: '', enabled: false } })

    await kernelApi.addKernelAccessGroupUser(7, 11)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/access-groups/7/users', method: 'post', data: { user_id: 11 } })
    await kernelApi.removeKernelAccessGroupUser(7, 11)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/access-groups/7/users/11', method: 'delete' })
    await kernelApi.addKernelAccessGroupPlan(7, 4)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/access-groups/7/plans', method: 'post', data: { plan_id: 4 } })
    await kernelApi.removeKernelAccessGroupPlan(7, 4)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/access-groups/7/plans/4', method: 'delete' })

    const grant = { group_id: 7, resource_type: 'plugin_api', resource_id: 'machine-telemetry', permissions: '["machine-telemetry.api"]' }
    await kernelApi.createKernelResourceGrant(grant)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/resource-grants', method: 'post', data: grant })
    await kernelApi.deleteKernelResourceGrant(12)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/resource-grants/12', method: 'delete' })

    const policy = { group_id: 7, key: 'machine-telemetry.rate', policy: '{"requests_per_minute":60}' }
    await kernelApi.upsertKernelQuotaPolicy(policy)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/quota-policies', method: 'put', data: policy })
    await kernelApi.deleteKernelQuotaPolicy(13)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/quota-policies/13', method: 'delete' })

    await kernelApi.resolveKernelAccess({ userID: 11, planID: 4, scopeID: 'forward' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3', url: '/access-groups/resolve', method: 'get', params: { user_id: 11, plan_id: 4, scope_id: 'forward' }
    })
    await kernelApi.deleteKernelAccessGroup(7)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/access-groups/7', method: 'delete' })
  })

  it('cancels operations through the kernel endpoint', async () => {
    await kernelApi.cancelKernelOperation('operation-1')
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/operations/operation-1/cancel',
      method: 'post',
      rawResponse: true
    })
  })

  it('preserves operation headers for lifecycle responses', async () => {
    mockRequest.mockResolvedValueOnce({
      data: { data: { operation: { id: 'operation-1', state: 'pending' } } },
      headers: {
        'x-anixops-operation-id': 'operation-1',
        'x-anixops-operation-chain': 'dependency-1,operation-1'
      }
    })

    await expect(kernelApi.runKernelInstallationAction(7, 'enable', { idempotencyKey: 'request-3' })).resolves.toEqual({
      operation: { id: 'operation-1', state: 'pending' },
      operation_id: 'operation-1',
      operation_chain: 'dependency-1,operation-1'
    })
  })

  it('manages topology revisions and deployment lifecycle through the v3 kernel', async () => {
    const graph = { message: 'canary', vertices: [], edges: [] }
    await kernelApi.validateKernelTopology(graph)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/topologies/validate', method: 'post', data: graph })

    await kernelApi.diagnoseKernelTopologyDeployment(3, 7, { rolloutGroup: 'canary-a', failurePolicy: 'stop_and_rollback' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3', url: '/topologies/3/revisions/7/diagnose', method: 'post',
      data: { rollout_group: 'canary-a', failure_policy: 'stop_and_rollback' }
    })
    await kernelApi.previewKernelTopologyDeployment(3, 7, { rolloutGroup: 'canary-a', failurePolicy: 'stop_and_rollback' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3', url: '/topologies/3/revisions/7/preview', method: 'post',
      data: { rollout_group: 'canary-a', failure_policy: 'stop_and_rollback' }
    })

    await kernelApi.createKernelTopologyRevision(3, graph)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/topologies/3/revisions', method: 'post', data: graph })

    const deployment = { topology_id: 3, revision_id: 7, rollout_group: 'canary-a', failure_policy: 'stop_and_rollback' }
    await kernelApi.planKernelDeployment(deployment)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/deployments', method: 'post', data: deployment })

    await kernelApi.getKernelDeploymentStatus(9)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/deployments/9', method: 'get' })
    await kernelApi.applyKernelDeployment(9)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/deployments/9/apply', method: 'post' })
    await kernelApi.rollbackKernelDeployment(9)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/deployments/9/rollback', method: 'post' })
  })

  it('creates a topology through the v3 kernel', async () => {
    const topology = { name: 'CN dedicated', service_scope: 'forward', description: 'canary' }
    await kernelApi.createKernelTopology(topology)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v3', url: '/topologies', method: 'post', data: topology })
  })

  it('reads and switches route modes through the v4 kernel', async () => {
    await kernelApi.getKernelRouteModes('legacy-api')
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes', method: 'get', params: { package_id: 'legacy-api' } })
    await kernelApi.getKernelRouteModes()
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes', method: 'get' })

    const input = { package_id: 'legacy-api', routes: ['tickets.list'], mode: 'native', reason: 'canary', confirm: true }
    await kernelApi.setKernelRouteModes(input)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes', method: 'post', data: input })

    await kernelApi.rollbackKernelRouteModes('legacy-api', 'mismatch')
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes/rollback', method: 'post', data: { package_id: 'legacy-api', reason: 'mismatch' } })
    await kernelApi.rollbackKernelRouteModes('legacy-api')
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes/rollback', method: 'post', data: { package_id: 'legacy-api' } })

    await kernelApi.getKernelRouteModeRevisions('legacy-api', 100)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes/revisions', method: 'get', params: { package_id: 'legacy-api', limit: 100 } })


    await kernelApi.getKernelRouteModeMismatches({ packageID: 'legacy-api', routeID: 'tickets.list', limit: 50 })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes/mismatches', method: 'get', params: { package_id: 'legacy-api', route_id: 'tickets.list', limit: 50 } })
    await kernelApi.getKernelRouteModeMismatches()
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/route-modes/mismatches', method: 'get', params: {} })
  })

  it('reads the agent transport inventory through the v4 kernel', async () => {
    await kernelApi.getKernelAgentTransports()
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get' })
    await kernelApi.getKernelAgentTransports(true)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get', params: { legacy_only: true } })
    // The node list and the node page ask for the nodes in view: one
    // comma separated `node` value, repeats dropped.
    await kernelApi.getKernelAgentTransports({ nodes: ['proxy-3', 'proxy-5', 'proxy-3'] })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get', params: { node: 'proxy-3,proxy-5' } })
    await kernelApi.getKernelAgentTransports({ legacyOnly: true, nodes: ['proxy-7'] })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get', params: { legacy_only: true, node: 'proxy-7' } })
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
