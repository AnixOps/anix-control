import { afterEach, describe, expect, it } from 'vitest'
import { maskAddresses, useAddressPrivacy } from '@/composables/useAddressPrivacy'

describe('maskAddresses', () => {
  it('masks the middle of IPv4 and IPv6 addresses and keeps the rest', () => {
    expect(maskAddresses('203.0.113.7')).toBe('203.*.*.7')
    expect(maskAddresses('203.0.113.7:443')).toBe('203.*.*.7:443')
    expect(maskAddresses('proxy-12 · 198.51.100.20')).toBe('proxy-12 · 198.*.*.20')
    expect(maskAddresses('203.0.113.1, 203.0.113.2')).toBe('203.*.*.1, 203.*.*.2')
    expect(maskAddresses('[2001:db8::1]:443')).toBe('[2001:****]:443')
    expect(maskAddresses('2001:db8:0:0:0:0:0:1')).toBe('2001:****')
    expect(maskAddresses('::1')).toBe('::****')
  })

  it('leaves names, versions, times and ports alone', () => {
    for (const text of ['hk.example.com:8443', 'v5.0.2', '1.2.3.4.5', '300.1.1.1', '12:30:45', '2026-11-09 10:26:32', 'deadbeef:cafe', '']) {
      expect(maskAddresses(text)).toBe(text)
    }
    expect(maskAddresses(null)).toBe(null)
    expect(maskAddresses(undefined)).toBe(undefined)
  })
})

describe('useAddressPrivacy', () => {
  afterEach(() => {
    useAddressPrivacy().setRevealed(false)
  })

  it('masks by default and shows the real address once revealed, for this tab only', () => {
    const { display, revealed, toggle } = useAddressPrivacy()
    expect(revealed.value).toBe(false)
    expect(display('203.0.113.7')).toBe('203.*.*.7')
    toggle()
    expect(display('203.0.113.7')).toBe('203.0.113.7')
    expect(sessionStorage.getItem('anixops-reveal-addresses')).toBe('1')
    toggle()
    expect(display('203.0.113.7')).toBe('203.*.*.7')
    expect(sessionStorage.getItem('anixops-reveal-addresses')).toBeNull()
  })
})
