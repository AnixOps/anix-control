import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const disconnectKernel = vi.hoisted(() => vi.fn())

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ disconnectKernel })
}))

import { kernelApiClient, kernelAuthClient, kernelAuthApi, kernelBaseURL, kernelPluginsApi } from '@/api'

describe('kernel plugin API client', () => {
  const originalAdapter = kernelApiClient.defaults.adapter
  const originalAuthAdapter = kernelAuthClient.defaults.adapter
  let requestConfig

  beforeEach(() => {
    requestConfig = null
    disconnectKernel.mockReset()
    localStorage.clear()
    sessionStorage.clear()
  })

  afterEach(() => {
    kernelApiClient.defaults.adapter = originalAdapter
    kernelAuthClient.defaults.adapter = originalAuthAdapter
    vi.unstubAllGlobals()
  })

  it('normalizes configured API paths to the kernel v3 path', () => {
    expect(kernelBaseURL()).toBe('/api/v3')
    expect(kernelBaseURL('https://control.example/api/v1')).toBe('https://control.example/api/v3')
    expect(kernelBaseURL('https://control.example/api/v3/')).toBe('https://control.example/api/v3')
    expect(kernelBaseURL('https://control.example/')).toBe('https://control.example/api/v3')
  })

  it('unwraps kernel data and attaches the bearer token', async () => {
    localStorage.setItem('token', 'workers-token')
    sessionStorage.setItem('kernel_token', 'kernel-token')
    kernelApiClient.defaults.adapter = async (config) => {
      requestConfig = config
      return {
        data: { data: [{ id: 'machine-telemetry' }] },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
        request: {}
      }
    }

    await expect(kernelPluginsApi.list()).resolves.toEqual([{ id: 'machine-telemetry' }])
    expect(requestConfig.baseURL).toBe(kernelApiClient.defaults.baseURL)
    expect(requestConfig.url).toBe('/plugins')
    expect(requestConfig.headers.Authorization).toBe('Bearer kernel-token')
  })

  it('posts Control credentials to the v2 login route without the Workers token', async () => {
    localStorage.setItem('token', 'workers-token')
    kernelAuthClient.defaults.adapter = async (config) => {
      requestConfig = config
      return { data: { code: 0, data: { mfa_required: true, methods: ['totp'] } }, status: 200, statusText: 'OK', headers: {}, config, request: {} }
    }

    await expect(kernelAuthApi.login({ email: 'admin@example.com', password: 'secret' })).resolves.toMatchObject({
      code: 0,
      data: { mfa_required: true }
    })
    expect(requestConfig.baseURL).toBe('/api/v2')
    expect(requestConfig.url).toBe('/login')
    expect(requestConfig.headers.Authorization).toBeUndefined()
    expect(requestConfig.data).toBe(JSON.stringify({ email: 'admin@example.com', password: 'secret' }))
  })

  it('maps operation response headers and sends lifecycle payloads', async () => {
    kernelApiClient.defaults.adapter = async (config) => {
      requestConfig = config
      return {
        data: { data: { operation: { id: 'op-7', state: 'pending' } } },
        status: 202,
        statusText: 'Accepted',
        headers: {
          'x-anixops-operation-id': 'op-7',
          'x-anixops-operation-chain': 'op-dependency,op-7'
        },
        config,
        request: {}
      }
    }

    const result = await kernelPluginsApi.action(7, 'enable', {
      idempotencyKey: 'control-center:enable:7:test',
      targetVersion: '1.2.0'
    })

    expect(result).toMatchObject({
      operation_id: 'op-7',
      operation_chain: 'op-dependency,op-7'
    })
    expect(requestConfig.method).toBe('post')
    expect(requestConfig.url).toBe('/plugin-installations/7/actions')
    expect(requestConfig.data).toBe(JSON.stringify({
      action: 'enable',
      idempotency_key: 'control-center:enable:7:test',
      target_version: '1.2.0'
    }))
  })

  it('disconnects only the Control session when the kernel returns 401', async () => {
    kernelApiClient.defaults.adapter = async (config) => Promise.reject({
      response: { status: 401 },
      config
    })

    await expect(kernelPluginsApi.list()).rejects.toMatchObject({ response: { status: 401 } })
    expect(disconnectKernel).toHaveBeenCalledWith('Control session expired. Connect again.')
  })

  it('ignores a late 401 from an older Control session', async () => {
    sessionStorage.setItem('kernel_token', 'new-control-token')
    kernelApiClient.defaults.adapter = async (config) => Promise.reject({
      response: { status: 401 },
      config: { ...config, headers: { Authorization: 'Bearer old-control-token' } }
    })

    await expect(kernelPluginsApi.list()).rejects.toMatchObject({ response: { status: 401 } })
    expect(disconnectKernel).not.toHaveBeenCalled()
    expect(sessionStorage.getItem('kernel_token')).toBe('new-control-token')
  })
})
