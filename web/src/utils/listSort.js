// Server-side sort of the admin lists. GET /admin/users, /admin/orders and
// /admin/nodes take `sort` (a column of the list's whitelist) and `order`
// (`asc`, the default, or `desc`); without them they answer in their own
// order. A list page maps the keys of its table columns to those names:
//
//   const SORT = createListSort({ traffic: 'traffic', lastHeartbeat: 'last_check_at' })
//   const state = ref(SORT.fromQuery(listQuery.read('sort', { values: SORT.values }), listQuery.read('order', { values: SORT_ORDERS })))
//   // <UiDataTable manual-sort :sort="state" @update:sort="next => …">
//   getUserList({ page, ...SORT.toParams(state.value) })
//   listQuery.write({ ...SORT.toQuery(state.value) })
//
// A column that is not in the map is not sortable: sorting the rows of one
// page of a long list says nothing about the list, so the page leaves the
// header a plain label. The URL and the request use the API's names.

export const SORT_ORDERS = Object.freeze(['asc', 'desc'])

const NONE = Object.freeze({ key: '', direction: '' })

export function createListSort(columns) {
  const byApi = new Map(Object.entries(columns).map(([key, api]) => [api, key]))
  const toParams = (state) => {
    const api = columns[state?.key]
    if (!api || !SORT_ORDERS.includes(state?.direction)) return {}
    return { sort: api, order: state.direction }
  }
  return {
    // The API's sort names, for the `values` of useListQuery().read().
    values: Object.freeze([...byApi.keys()]),
    // Whether the table column (by key) sorts on the server.
    isSortable: key => Object.prototype.hasOwnProperty.call(columns, key),
    // The table's sort state ({ key, direction }) from the API-named `sort`
    // and `order` values of the URL; anything unknown is no sort. A sort
    // without an order is ascending, as it is for the API.
    fromQuery(sort, order) {
      const key = byApi.get(String(sort || ''))
      if (!key) return { ...NONE }
      return { key, direction: String(order || '').toLowerCase() === 'desc' ? 'desc' : 'asc' }
    },
    // The request's `sort` and `order`: both, or nothing for no sort.
    toParams,
    // The same two values as URL query entries (empty removes the key).
    toQuery(state) {
      const params = toParams(state)
      return { sort: params.sort || '', order: params.order || '' }
    }
  }
}
