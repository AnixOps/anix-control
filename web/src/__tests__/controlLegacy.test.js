import { describe, expect, it } from 'vitest'
import { resolveLegacyControlRedirect } from '@/router/controlLegacy'

describe('legacy Control redirects', () => {
  it.each([
    [undefined, '/admin/plugins'],
    ['plugins', '/admin/plugins'],
    ['assignments', '/admin/deployments'],
    ['scopes', '/admin/deployments'],
    ['topologies', '/admin/deployments'],
    ['operations', '/admin/deployments'],
    ['unknown', '/admin/plugins'],
  ])('maps tab %s to %s', (tab, path) => {
    const query = tab === undefined ? { keep: 'yes' } : { tab, keep: 'yes' }

    expect(resolveLegacyControlRedirect({ query, hash: '#activity' })).toEqual({
      path,
      query: { keep: 'yes' },
      hash: '#activity',
    })
  })
})
