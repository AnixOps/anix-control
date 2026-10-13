import { describe, expect, it } from 'vitest'
import { resolveLegacyControlRedirect } from '@/router/controlLegacy'

describe('legacy Control redirects', () => {
  it.each([
    [undefined, '/admin/system/packages'],
    ['plugins', '/admin/system/packages'],
    ['assignments', '/admin/deployments'],
    ['scopes', '/admin/deployments'],
    ['topologies', '/admin/deployments'],
    ['operations', '/admin/deployments'],
    ['unknown', '/admin/system/packages'],
  ])('maps tab %s to %s', (tab, path) => {
    const query = tab === undefined ? { keep: 'yes' } : { tab, keep: 'yes' }

    expect(resolveLegacyControlRedirect({ query, hash: '#activity' })).toEqual({
      path,
      query: { keep: 'yes' },
      hash: '#activity',
    })
  })
})
