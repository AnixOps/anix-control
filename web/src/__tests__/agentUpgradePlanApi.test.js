import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as kernelApi from '@/api/kernel'

const mockRequest = vi.hoisted(() => vi.fn())

vi.mock('@/utils/request', () => ({ default: mockRequest }))

describe('Agent upgrade plan API', () => {
  beforeEach(() => {
    mockRequest.mockReset()
    mockRequest.mockResolvedValue({ data: { target_version: 'v4.2.0', eligible_nodes: 3 } })
  })

  it('reads what an upgrade would do now', async () => {
    expect(await kernelApi.getKernelAgentUpgradePlan()).toEqual({ target_version: 'v4.2.0', eligible_nodes: 3 })
    expect(mockRequest).toHaveBeenLastCalledWith({ baseURL: '/api/v4', url: '/kernel/agents/upgrade-plan', method: 'get' })
  })
})
