import { describe, expect, it } from 'vitest'
import { SORT_ORDERS, createListSort } from '@/utils/listSort'

// The admin lists sort on the server (GET /admin/users|orders|nodes take
// `sort` and `order`): the table's column keys map to the API's names.
describe('createListSort', () => {
  const sort = createListSort({ traffic: 'traffic', lastHeartbeat: 'last_check_at', id: 'id' })

  it('lists the API names for the URL whitelist and the sortable column keys', () => {
    expect(sort.values).toEqual(['traffic', 'last_check_at', 'id'])
    expect(SORT_ORDERS).toEqual(['asc', 'desc'])
    expect(sort.isSortable('lastHeartbeat')).toBe(true)
    expect(sort.isSortable('status')).toBe(false)
    expect(sort.isSortable('toString')).toBe(false)
  })

  it('turns a table sort into request parameters, and no sort into none', () => {
    expect(sort.toParams({ key: 'lastHeartbeat', direction: 'desc' })).toEqual({ sort: 'last_check_at', order: 'desc' })
    expect(sort.toParams({ key: 'traffic', direction: 'asc' })).toEqual({ sort: 'traffic', order: 'asc' })
    expect(sort.toParams({ key: '', direction: '' })).toEqual({})
    expect(sort.toParams({ key: 'traffic', direction: '' })).toEqual({})
    expect(sort.toParams({ key: 'status', direction: 'asc' })).toEqual({})
    expect(sort.toParams({ key: 'traffic', direction: 'sideways' })).toEqual({})
    expect(sort.toParams(undefined)).toEqual({})
  })

  it('writes the same two values to the URL, empty to drop the keys', () => {
    expect(sort.toQuery({ key: 'lastHeartbeat', direction: 'desc' })).toEqual({ sort: 'last_check_at', order: 'desc' })
    expect(sort.toQuery({ key: '', direction: '' })).toEqual({ sort: '', order: '' })
  })

  it('reads the table sort back from the URL values: unknown is no sort, no order is ascending', () => {
    expect(sort.fromQuery('last_check_at', 'desc')).toEqual({ key: 'lastHeartbeat', direction: 'desc' })
    expect(sort.fromQuery('last_check_at', 'DESC')).toEqual({ key: 'lastHeartbeat', direction: 'desc' })
    expect(sort.fromQuery('traffic', '')).toEqual({ key: 'traffic', direction: 'asc' })
    expect(sort.fromQuery('traffic', 'nonsense')).toEqual({ key: 'traffic', direction: 'asc' })
    expect(sort.fromQuery('password', 'desc')).toEqual({ key: '', direction: '' })
    expect(sort.fromQuery('', 'desc')).toEqual({ key: '', direction: '' })
    expect(sort.fromQuery(undefined, undefined)).toEqual({ key: '', direction: '' })
  })

  it('round-trips through the URL', () => {
    for (const state of [{ key: 'traffic', direction: 'desc' }, { key: 'id', direction: 'asc' }]) {
      const query = sort.toQuery(state)
      expect(sort.fromQuery(query.sort, query.order)).toEqual(state)
    }
  })
})
