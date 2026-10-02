import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import router from '@/router'
import { useUserStore } from '@/stores/user'
import { getPublicConfig } from '@/api/public'
import { loadEdition, setEdition } from '@/composables/useEdition'

// '@/api/public' is mocked in setup.js.
vi.mock('@/api/kernel', () => ({
  getKernelExtensions: vi.fn(async () => []),
}))

vi.mock('@/api/user', () => ({
  getProfile: vi.fn(async () => ({ data: {} })),
}))

const commercialAdminPaths = ['/admin/orders', '/admin/coupons', '/admin/invite', '/admin/payment']
const commercialUserPaths = ['/user/plans', '/user/orders']

describe('edition route guard', () => {
  beforeEach(async () => {
    localStorage.clear()
    setActivePinia(createPinia())
    if (router.currentRoute.value.path !== '/login') {
      await router.replace('/login')
    }
  })

  it('redirects admins away from commercial pages in the community edition', async () => {
    setEdition('community')
    useUserStore().login('admin-token', { id: 1, is_admin: true, permissions: [] })

    for (const path of commercialAdminPaths) {
      await router.push(path)
      expect(router.currentRoute.value.path).toBe('/admin/dashboard')
    }
    // Subscription templates stay.
    await router.push('/admin/plans')
    expect(router.currentRoute.value.path).toBe('/admin/plans')
    // Invite codes stay: registration control, not affiliate rewards.
    await router.push('/admin/invite-codes')
    expect(router.currentRoute.value.path).toBe('/admin/invite-codes')
  })

  it('redirects users away from plans and orders in the community edition', async () => {
    setEdition('community')
    useUserStore().login('user-token', { id: 2, is_admin: false, permissions: [] })

    for (const path of commercialUserPaths) {
      await router.push(path)
      expect(router.currentRoute.value.path).toBe('/user/dashboard')
    }
  })

  it('serves every commercial page in the commercial edition', async () => {
    setEdition('commercial')
    useUserStore().login('admin-token', { id: 1, is_admin: true, permissions: [] })
    for (const path of commercialAdminPaths) {
      await router.push(path)
      expect(router.currentRoute.value.path).toBe(path)
    }

    useUserStore().logout()
    useUserStore().login('user-token', { id: 2, is_admin: false, permissions: [] })
    for (const path of commercialUserPaths) {
      await router.push(path)
      expect(router.currentRoute.value.path).toBe(path)
    }
  })

  it('reads the edition from the public configuration before a commercial page', async () => {
    vi.mocked(getPublicConfig).mockResolvedValueOnce({ edition: 'commercial', hidden_packages: [], registration: {} })
    useUserStore().login('admin-token', { id: 1, is_admin: true, permissions: [] })

    await router.push('/admin/orders')

    expect(getPublicConfig).toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/admin/orders')
  })

  it('keeps the community edition when the public configuration cannot be read', async () => {
    vi.mocked(getPublicConfig).mockRejectedValueOnce(new Error('offline'))
    await loadEdition()
    useUserStore().login('admin-token', { id: 1, is_admin: true, permissions: [] })

    await router.push('/admin/payment')

    expect(router.currentRoute.value.path).toBe('/admin/dashboard')
  })
})
