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
      '/admin/invite-codes',
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
      '/admin/security',
      '/admin/security/mfa',
      '/admin/security/access-groups',
      '/admin/system/general',
      '/admin/system/audit',
      '/admin/notifications/telegram',
      '/admin/subscriptions/1',
    ]

    // Each resolves to a real route (a page, a page with a section
    // parameter, or a redirect), never the catch-all.
    for (const path of expectedPaths) {
      const matched = router.resolve(path).matched
      expect(matched.length > 0 && !matched.at(-1).path.includes(':pathMatch'), path).toBe(true)
    }
  })

  it('keeps the old settings-style page URLs working (UI U7)', () => {
    const redirects = {
      '/admin/telegram': '/admin/notifications/telegram',
      '/admin/mfa': '/admin/security/mfa',
      '/admin/access-groups': '/admin/security/access-groups'
    }
    for (const [from, to] of Object.entries(redirects)) {
      expect(router.resolve(from).matched.at(-1).redirect, from).toBe(to)
    }
    for (const path of ['/admin/system', '/admin/system/backup', '/admin/security', '/admin/notifications', '/admin/notifications/email', '/admin/subscriptions/3', '/admin/subscriptions/3/templates']) {
      expect(router.resolve(path).matched.at(-1).path, path).not.toContain(':pathMatch')
    }
  })

  it('gives forward nodes and Ansible machines numeric detail routes (UI U7)', () => {
    const node = router.resolve('/admin/forward/nodes/5')
    expect(node.matched.at(-1).path).toBe('/admin/forward/nodes/:id(\\d+)')
    expect(node.matched.at(-1).props.default(node)).toEqual({ id: 5 })
    expect(node.meta.titleKey).toBe('forwardNodesPage.detail.sections')
    const machine = router.resolve('/admin/forward/ansible-machines/7')
    expect(machine.matched.at(-1).props.default(machine)).toEqual({ id: 7 })
    expect(router.resolve('/admin/forward/nodes/abc').matched.at(-1).path).not.toBe('/admin/forward/nodes/:id(\\d+)')
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
