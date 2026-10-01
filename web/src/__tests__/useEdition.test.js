import { describe, expect, it } from 'vitest'
import {
  applyPublicConfig,
  extensionMenuAllowed,
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

  it('checks routes and extension menus against the edition', () => {
    const commercialRoute = { matched: [{ meta: {} }, { meta: { edition: 'commercial' } }] }
    const paymentExtension = { matched: [{ meta: { extension: true, extensionPluginID: 'payment' } }] }
    expect(routeAllowedByEdition(commercialRoute)).toBe(false)
    expect(routeAllowedByEdition(paymentExtension)).toBe(false)
    expect(extensionMenuAllowed({ pluginID: 'payment', id: 'payment.main' })).toBe(false)
    expect(extensionMenuAllowed({ pluginID: 'plan', id: 'plan.main' })).toBe(true)

    applyPublicConfig({ edition: 'commercial', hidden_packages: [], registration: { require_invite: true } })
    expect(routeAllowedByEdition(commercialRoute)).toBe(true)
    expect(routeAllowedByEdition(paymentExtension)).toBe(true)
    expect(extensionMenuAllowed({ pluginID: 'payment', id: 'payment.main' })).toBe(true)
    expect(useEdition().requireInvite.value).toBe(true)
  })
})
