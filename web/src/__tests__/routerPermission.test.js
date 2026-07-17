import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const mockGetProfile = vi.hoisted(() => vi.fn())
const mockEnsureAdminExtensions = vi.hoisted(() => vi.fn())
const mockResetAdminExtensions = vi.hoisted(() => vi.fn())

vi.mock('@/api/user', () => ({
  getProfile: (...args) => mockGetProfile(...args),
}))

vi.mock('@/extensions/runtime', () => ({
  ensureAdminExtensions: (...args) => mockEnsureAdminExtensions(...args),
  resetAdminExtensions: (...args) => mockResetAdminExtensions(...args),
}))

import router from '@/router'
import { useUserStore } from '@/stores/user'

describe('router extension permission bootstrap', () => {
  beforeEach(async () => {
    localStorage.clear()
    setActivePinia(createPinia())
    mockGetProfile.mockReset()
    mockEnsureAdminExtensions.mockReset()
    mockResetAdminExtensions.mockReset()
    if (router.currentRoute.value.path !== '/login') {
      await router.replace('/login')
    }
  })

  it('refreshes a missing login profile before discovering and loading authorized extensions', async () => {
    const events = []
    let removeExtension
    mockGetProfile.mockImplementation(async () => {
      events.push('profile')
      return {
        data: {
          id: 7,
          is_admin: true,
          permission_mode: 'authoritative',
          permissions: ['example.view'],
        },
      }
    })
    mockEnsureAdminExtensions.mockImplementation(async targetRouter => {
      events.push(`ensure:${useUserStore().permissionMode}:${useUserStore().hasPermission('example.view')}`)
      if (!targetRouter.hasRoute('test-extension-loaded')) {
        removeExtension = targetRouter.addRoute('admin', {
          path: 'extensions/example',
          name: 'test-extension-loaded',
          component: { template: '<div>Example extension</div>' },
          meta: {
            requiresAuth: true,
            requiresAdmin: true,
            extension: true,
            extensionPermission: 'example.view',
          },
        })
      }
    })

    const userStore = useUserStore()
    userStore.login('admin-token', { id: 7, is_admin: true })
    expect(userStore.permissionMode).toBe('missing')

    await router.push('/admin/dashboard')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(events).toEqual(['profile', 'ensure:authoritative:true'])
    expect(mockGetProfile).toHaveBeenCalledTimes(1)
    expect(mockEnsureAdminExtensions).toHaveBeenCalledTimes(1)
    expect(router.hasRoute('test-extension-loaded')).toBe(true)

    await router.push('/admin/extensions/example')
    expect(router.currentRoute.value.path).toBe('/admin/extensions/example')
    expect(mockGetProfile).toHaveBeenCalledTimes(1)
    expect(mockEnsureAdminExtensions).toHaveBeenCalledTimes(1)

    removeExtension?.()
  })
})
