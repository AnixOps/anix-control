import { describe, expect, it, vi } from 'vitest'
import router from '@/router'

vi.mock('@/api/kernel', () => ({
  getKernelExtensions: vi.fn(async () => []),
}))

// Vue Router warns when a named route has an unnamed child with an empty
// path ("Using that name won't render the empty path child"). Every such
// child gets a name of its own.
describe('route names', () => {
  it('names every empty-path child of a named route', () => {
    const offenders = []
    function walk(records, parent) {
      for (const record of records || []) {
        if (parent?.name && record.path === '' && !record.name) offenders.push(String(parent.name))
        walk(record.children, record)
      }
    }
    walk(router.options.routes, null)
    expect(offenders).toEqual([])
  })

  it('keeps the admin parent name extensions add their pages under', () => {
    expect(router.hasRoute('admin')).toBe(true)
    expect(router.resolve({ name: 'admin-index' }).fullPath).toBe('/admin')
  })
})
