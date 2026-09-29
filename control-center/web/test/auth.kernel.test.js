import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const kernelLogin = vi.hoisted(() => vi.fn())

vi.mock('@/api', () => ({
  default: { post: vi.fn() },
  kernelAuthApi: { login: kernelLogin }
}))

import { useAuthStore } from '@/stores/auth'

describe('Control session', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    kernelLogin.mockReset()
    localStorage.setItem('token', 'workers-token')
    localStorage.setItem('user', JSON.stringify({ id: 1, role: 'admin' }))
    setActivePinia(createPinia())
  })

  it('stores a Control admin token separately from the Workers token', async () => {
    kernelLogin.mockResolvedValue({ code: 0, data: { token: 'control-token', is_admin: true } })
    const auth = useAuthStore()

    await expect(auth.connectKernel({ email: 'admin@example.com', password: 'secret' })).resolves.toEqual({ connected: true })
    expect(sessionStorage.getItem('kernel_token')).toBe('control-token')
    expect(localStorage.getItem('token')).toBe('workers-token')
    expect(auth.isKernelAuthenticated).toBe(true)

    auth.disconnectKernel()
    expect(auth.isKernelAuthenticated).toBe(false)
    expect(localStorage.getItem('token')).toBe('workers-token')
  })

  it('keeps MFA challenges unauthenticated until a valid code returns a token', async () => {
    kernelLogin.mockResolvedValueOnce({ code: 0, data: { mfa_required: true, methods: ['totp', 'backup'] } })
      .mockResolvedValueOnce({ code: 0, data: { token: 'control-token', is_admin: true } })
    const auth = useAuthStore()

    await expect(auth.connectKernel({ email: 'admin@example.com', password: 'secret' })).resolves.toEqual({
      mfaRequired: true,
      methods: ['totp', 'backup']
    })
    expect(auth.isKernelAuthenticated).toBe(false)
    await expect(auth.connectKernel({ email: 'admin@example.com', password: 'secret', mfaCode: '123456', mfaMethod: 'totp' })).resolves.toEqual({ connected: true })
    expect(kernelLogin).toHaveBeenLastCalledWith({ email: 'admin@example.com', password: 'secret', mfaCode: '123456', mfaMethod: 'totp' })
    expect(auth.isKernelAuthenticated).toBe(true)
  })

  it('does not accept enrollment challenges or non-admin tokens', async () => {
    kernelLogin.mockResolvedValueOnce({ code: 0, data: { mfa_enrollment_required: true, methods: ['totp'] } })
      .mockResolvedValueOnce({ code: 0, data: { token: 'user-token', is_admin: false } })
    const auth = useAuthStore()

    await expect(auth.connectKernel({ email: 'admin@example.com', password: 'secret' })).resolves.toEqual({ enrollmentRequired: true, methods: ['totp'] })
    await expect(auth.connectKernel({ email: 'user@example.com', password: 'secret' })).resolves.toEqual({ connected: false })
    expect(auth.kernelError).toBe('Control administrator access is required')
    expect(sessionStorage.getItem('kernel_token')).toBeNull()
  })

  it('clears an older Control token before a rejected reauthentication', async () => {
    kernelLogin.mockResolvedValue({ code: 0, data: { token: 'user-token', is_admin: false } })
    const auth = useAuthStore()
    auth.kernelToken = 'old-control-token'
    sessionStorage.setItem('kernel_token', 'old-control-token')

    await expect(auth.connectKernel({ email: 'user@example.com', password: 'secret' })).resolves.toEqual({ connected: false })
    expect(auth.isKernelAuthenticated).toBe(false)
    expect(sessionStorage.getItem('kernel_token')).toBeNull()
    expect(localStorage.getItem('token')).toBe('workers-token')
  })

  it('reports the Control panel error envelope without changing Workers auth', async () => {
    kernelLogin.mockResolvedValue({ code: -1, msg: 'invalid mfa code', data: null })
    const auth = useAuthStore()

    await expect(auth.connectKernel({ email: 'admin@example.com', password: 'secret', mfaCode: '000000' })).resolves.toEqual({ connected: false })
    expect(auth.kernelError).toBe('invalid mfa code')
    expect(auth.isAuthenticated).toBe(true)
    expect(localStorage.getItem('token')).toBe('workers-token')
  })
})
