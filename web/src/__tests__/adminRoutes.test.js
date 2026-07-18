import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import router from '@/router'
import { useUserStore } from '@/stores/user'

vi.mock('@/api/kernel', () => ({
  getKernelExtensions: vi.fn(async () => []),
}))

vi.mock('@/api/user', () => ({
  getProfile: vi.fn(async () => ({ data: {} })),
}))

async function resetRouter(path = '/login') {
  if (router.currentRoute.value.path !== path) {
    await router.replace(path)
  }
}

describe('admin routes', () => {
  beforeEach(async () => {
    localStorage.clear()
    setActivePinia(createPinia())
    await resetRouter()
  })

  it('registers all admin menu paths', () => {
    const expectedPaths = [
      '/admin/dashboard',
      '/admin/monitor',
      '/admin/traffic-hourly',
      '/admin/users',
      '/admin/orders',
      '/admin/tickets',
      '/admin/nodes',
      '/admin/subscriptions',
      '/admin/forward/setup',
      '/admin/forward',
      '/admin/forward/tunnel',
      '/admin/forward/limit',
      '/admin/forward/ansible-machines',
      '/admin/forward/nodes',
      '/admin/forward/local',
      '/admin/forward/nodex',
      '/admin/forward/agents',
      '/admin/forward/observability',
      '/admin/forward/tunnels',
      '/admin/forward/limits',
      '/admin/forward/ansible',
      '/admin/local',
      '/admin/nodex',
      '/admin/tunnel',
      '/admin/limit',
      '/admin/plans',
      '/admin/coupons',
      '/admin/invite',
      '/admin/payment',
      '/admin/telegram',
      '/admin/notifications',
      '/admin/knowledge',
      '/admin/mfa',
      '/admin/system',
      '/admin/agent',
      '/admin/plugins',
      '/admin/deployments',
      '/admin/control',
      '/admin/access-groups',
    ]

    const routePaths = router.getRoutes().map(route => route.path)

    for (const path of expectedPaths) {
      expect(routePaths).toContain(path)
    }
  })

  it('blocks plugin extension routes when the admin lacks the route permission', async () => {
    const removeRoute = router.addRoute('admin', {
      path: 'extensions/example',
      name: 'test-extension-denied',
      component: { template: '<div />' },
      meta: {
        requiresAuth: true,
        requiresAdmin: true,
        extension: true,
        extensionPermission: 'example.view',
      },
    })
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 1, is_admin: true, permissions: ['other.view'] })

    await router.push('/admin/extensions/example')

    expect(router.currentRoute.value.path).toBe('/admin/plugins')
    removeRoute()
  })

  it('redirects legacy Control routes to their replacement routes', async () => {
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 1, is_admin: true, permissions: [] })

    await router.push('/admin/control?tab=assignments')

    expect(router.currentRoute.value.path).toBe('/admin/deployments')

    await router.push('/admin/control')

    expect(router.currentRoute.value.path).toBe('/admin/plugins')
  })

  it('allows plugin extension routes when the admin has the route permission', async () => {
    const removeRoute = router.addRoute('admin', {
      path: 'extensions/example',
      name: 'test-extension-allowed',
      component: { template: '<div />' },
      meta: {
        requiresAuth: true,
        requiresAdmin: true,
        extension: true,
        extensionPermission: 'example.view',
      },
    })
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 1, is_admin: true, permissions: ['example.view'] })

    await router.push('/admin/extensions/example')

    expect(router.currentRoute.value.path).toBe('/admin/extensions/example')
    removeRoute()
  })
})
