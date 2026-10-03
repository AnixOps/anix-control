import { describe, expect, it } from 'vitest'
import { evaluate } from '../../scripts/check-npm-audit.mjs'

const adv = (id, pkg = 'braces') => [id, { id, package: pkg, severity: 'high', title: 't' }]
const allowlist = { advisories: [{ id: 'GHSA-a', package: 'braces', reason: 'r', expires: '2026-11-02' }] }

describe('npm audit check', () => {
  it('accepts a waived development-only advisory before it expires', () => {
    expect(evaluate(new Map(), new Map([adv('GHSA-a')]), allowlist, '2026-10-03')).toEqual([])
  })

  it('fails once the waiver has expired', () => {
    expect(evaluate(new Map(), new Map([adv('GHSA-a')]), allowlist, '2026-11-03')[0]).toMatch(/expired/)
  })

  it('never waives an advisory in production dependencies', () => {
    const prod = new Map([adv('GHSA-a')])
    expect(evaluate(prod, prod, allowlist, '2026-10-03')[0]).toMatch(/production/)
  })

  it('fails on a new development advisory that is not listed', () => {
    const all = new Map([adv('GHSA-a'), adv('GHSA-b', 'other')])
    expect(evaluate(new Map(), all, allowlist, '2026-10-03')).toHaveLength(1)
  })

  it('asks to remove an entry that is no longer reported', () => {
    expect(evaluate(new Map(), new Map(), allowlist, '2026-10-03')[0]).toMatch(/remove the entry/)
  })
})
