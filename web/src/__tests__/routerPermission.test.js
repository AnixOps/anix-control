import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mockGetProfile = vi.hoisted(() => vi.fn())
const mockEnsureAdminCapabilities = vi.hoisted(() => vi.fn())
const mockResetAdminCapabilities = vi.hoisted(() => vi.fn())

vi.mock('@/api/user', () => ({
  getProfile: (...args) => mockGetProfile(...args),
}))

vi.mock('@/capabilities', () => ({
  ensureAdminCapabilities: (...args) => mockEnsureAdminCapabilities(...args),
  resetAdminCapabilities: (...args) => mockResetAdminCapabilities(...args),
}))

vi.mock('@/views/admin/forward/Overview.vue', () => ({ default: { template: '<div>Forward overview</div>' } }))

import router from '@/router'
import { useUserStore } from '@/stores/user'

describe('router capability bootstrap', () => {
  beforeEach(async () => {
    localStorage.clear()
    setActivePinia(createPinia())
    mockGetProfile.mockReset()
    mockEnsureAdminCapabilities.mockReset()
    mockResetAdminCapabilities.mockReset()
    if (router.currentRoute.value.path !== '/login') {
      await router.replace('/login')
    }
  })

  function signInWithMissingProfile(events) {
    mockGetProfile.mockImplementation(async () => {
      events.push('profile')
      return { data: { id: 7, is_admin: true, permission_mode: 'authoritative', permissions: ['forward.api'] } }
    })
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 7, is_admin: true })
    expect(userStore.permissionMode).toBe('missing')
  }

  it('refreshes a missing login profile before reading the capabilities', async () => {
    const events = []
    signInWithMissingProfile(events)
    mockEnsureAdminCapabilities.mockImplementation(async () => {
      events.push(`capabilities:${useUserStore().permissionMode}`)
      return []
    })

    await router.push('/admin/dashboard')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(events).toEqual(['profile', 'capabilities:authoritative'])
    expect(mockGetProfile).toHaveBeenCalledTimes(1)
  })

  it('opens a forwarding page only with forward.v4', async () => {
    signInWithMissingProfile([])
    mockEnsureAdminCapabilities.mockResolvedValue([])
    await router.push('/admin/forward/overview')
    expect(router.currentRoute.value.path).toBe('/admin/system/packages')

    mockEnsureAdminCapabilities.mockResolvedValue(['forward.v4'])
    await router.push('/admin/forward/overview')
    expect(router.currentRoute.value.path).toBe('/admin/forward/overview')
  })
})
