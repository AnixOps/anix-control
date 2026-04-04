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
      '/admin/agent',
      '/admin/plans',
      '/admin/coupons',
      '/admin/invite',
      '/admin/payment',
      '/admin/telegram',
      '/admin/notifications',
      '/admin/knowledge',
      '/admin/mfa',
      '/admin/system',
    ]

    const routePaths = router.getRoutes().map(route => route.path)

    for (const path of expectedPaths) {
      expect(routePaths).toContain(path)
    }
  })
})
