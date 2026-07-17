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
      [() => kernelApi.getKernelPluginReleaseArtifact(9), '/plugin-releases/9/artifact'],
      [() => kernelApi.getKernelInstallationConfig(7), '/plugin-installations/7/config'],
      [kernelApi.getKernelScopes, '/service-scopes'],
      [kernelApi.getKernelTopologies, '/topologies'],
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
      data: { config: { port: 443 }, expected_revision: 2 }
    })

    await kernelApi.updateKernelInstallationConfig(7, { port: 8443 })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/config',
      method: 'put',
      data: { config: { port: 8443 } }
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
      data: installation
    })

    await kernelApi.runKernelInstallationAction(7, 'update', { targetVersion: '1.1.0', idempotencyKey: 'request-1' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/actions',
      method: 'post',
      data: { action: 'update', target_version: '1.1.0', idempotency_key: 'request-1' }
    })

    await kernelApi.runKernelInstallationAction(7, 'rollback', { idempotencyKey: 'request-2' })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/plugin-installations/7/actions',
      method: 'post',
      data: { action: 'rollback', idempotency_key: 'request-2' }
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

  it('cancels operations through the kernel endpoint', async () => {
    await kernelApi.cancelKernelOperation('operation-1')
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v3',
      url: '/operations/operation-1/cancel',
      method: 'post'
    })
  })
})
