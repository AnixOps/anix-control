import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as kernelApi from '@/api/kernel'

const mockRequest = vi.hoisted(() => vi.fn())

vi.mock('@/utils/request', () => ({ default: mockRequest }))

// The node diagnostics (/api/v4/kernel/nodes/:id/diagnostics).
describe('node diagnostics API', () => {
  beforeEach(() => {
    mockRequest.mockReset()
    mockRequest.mockResolvedValue({ data: { ok: true } })
  })

  it('reads what a node runs and its newest tasks', async () => {
    expect(await kernelApi.getKernelNodeDiagnostics('proxy-5')).toEqual({ ok: true })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/nodes/proxy-5/diagnostics', method: 'get' })
    await kernelApi.getKernelNodeDiagnostics('forward-12', 25)
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/nodes/forward-12/diagnostics', method: 'get', params: { limit: 25 } })
  })

  it('runs an action and waits as long as it asked for, plus the transport', async () => {
    await kernelApi.runKernelNodeDiagnostic('proxy-5', { action: 'log_tail', params: { service: 'gost', lines: 500 } })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v4',
      url: '/kernel/nodes/proxy-5/diagnostics',
      method: 'post',
      data: { action: 'log_tail', params: { service: 'gost', lines: 500 } },
      timeout: 40_000
    })
    await kernelApi.runKernelNodeDiagnostic('forward-3', { action: 'service_restart', params: { service: 'gost' }, timeoutSeconds: 60 })
    expect(mockRequest).toHaveBeenLastCalledWith({
      baseURL: '/api/v4',
      url: '/kernel/nodes/forward-3/diagnostics',
      method: 'post',
      data: { action: 'service_restart', params: { service: 'gost' }, timeout_seconds: 60 },
      timeout: 70_000
    })
  })

  it('keeps the transport inventory the node list reads', async () => {
    await kernelApi.getKernelAgentTransports({ nodes: ['proxy-5'] })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/transports', method: 'get', params: { node: 'proxy-5' } })
  })
})
