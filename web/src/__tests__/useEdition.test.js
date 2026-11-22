import { describe, expect, it } from 'vitest'
import {
  applyPublicConfig,
  filterByEdition,
  normalizeEdition,
  routeAllowedByEdition,
  setEdition,
  useEdition,
} from '@/composables/useEdition'

describe('useEdition', () => {
  it('defaults to the community edition', () => {
    const { edition, isCommercial, requireInvite } = useEdition()
    expect(edition.value).toBe('community')
    expect(isCommercial.value).toBe(false)
    expect(requireInvite.value).toBe(false)
    expect(normalizeEdition('Commercial')).toBe('commercial')
    expect(normalizeEdition('enterprise')).toBe('community')
  })

  it('filters commercial items in one place', () => {
    const items = [{ to: '/a' }, { to: '/b', edition: 'commercial' }]
    expect(filterByEdition(items).map(item => item.to)).toEqual(['/a'])
    setEdition('commercial')
    expect(filterByEdition(items).map(item => item.to)).toEqual(['/a', '/b'])
  })

  it('checks routes against the edition', () => {
    const commercialRoute = { matched: [{ meta: {} }, { meta: { edition: 'commercial' } }] }
    expect(routeAllowedByEdition(commercialRoute)).toBe(false)

    applyPublicConfig({ edition: 'commercial', registration: { require_invite: true } })
    expect(routeAllowedByEdition(commercialRoute)).toBe(true)
    expect(useEdition().requireInvite.value).toBe(true)
  })
})
