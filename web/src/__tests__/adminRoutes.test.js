import { describe, it, expect } from 'vitest'
import router from '@/router'

describe('admin routes', () => {
  it('registers all admin menu paths', () => {
    const expectedPaths = [
      '/admin/dashboard',
      '/admin/users',
      '/admin/orders',
      '/admin/tickets',
      '/admin/nodes',
      '/admin/subscriptions',
      '/admin/forward',
      '/admin/forward/tunnel',
      '/admin/forward/limit',
      '/admin/forward/nodes',
      '/admin/forward/tunnels',
      '/admin/forward/limits',
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
    ]

    const routePaths = router.getRoutes().map(route => route.path)

    for (const path of expectedPaths) {
      expect(routePaths).toContain(path)
    }
  })
})
