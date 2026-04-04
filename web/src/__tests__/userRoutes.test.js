import { describe, it, expect } from 'vitest'
import router from '@/router'

describe('user routes', () => {
  it('registers all user menu paths', () => {
    const expectedPaths = [
      '/user/dashboard',
      '/user/subscribe',
      '/user/knowledge',
      '/user/tickets',
      '/user/plans',
      '/user/orders',
    ]

    const routePaths = router.getRoutes().map(route => route.path)

    for (const path of expectedPaths) {
      expect(routePaths).toContain(path)
    }
  })
})
