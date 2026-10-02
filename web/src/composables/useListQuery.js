import { useRouter } from 'vue-router'

// useListQuery keeps a list page's filter state in the URL query (plan §9:
// search, chips and page are shareable and survive a reload or 返回).
//
//   const listQuery = useListQuery()
//   const search = ref(listQuery.read('q'))
//   const status = ref(listQuery.read('status', { values: ['open', 'closed'] }))
//   const page = ref(listQuery.readPage())
//   watch(..., () => listQuery.write({ q: search.value.trim(), status: status.value, page: page.value }))
//
// write() replaces (never pushes) the current entry and only touches the keys
// it is given: an empty value, or page 1, removes the key, so a list with the
// default filters has a clean URL. Other query keys are kept, except those in
// `omit` (one-shot instructions the page has already consumed).
// Pages mounted without a router (unit tests) read empty values and write
// nothing; nor does a write after the page's route has been left.
export function useListQuery({ omit = [] } = {}) {
  const router = useRouter()
  // The router's current route rather than useRoute(), so pages whose tests
  // mock only useRouter work too.
  const routed = Boolean(router?.currentRoute && typeof router.replace === 'function')
  const current = () => (routed ? router.currentRoute.value : null)
  // The page's own path: a late write (a debounced search, a slow load) after
  // leaving the page must not rewrite the next page's URL.
  const ownPath = current()?.path

  function first(key) {
    const value = current()?.query?.[key]
    return String(Array.isArray(value) ? value[0] ?? '' : value ?? '')
  }

  // A string value; with `values`, anything outside the list reads as ''.
  function read(key, { values } = {}) {
    const value = first(key)
    if (Array.isArray(values) && !values.map(String).includes(value)) return ''
    return value
  }

  function readPage(key = 'page') {
    const page = Math.floor(Number(first(key)))
    return Number.isFinite(page) && page > 1 ? page : 1
  }

  function encode(key, value) {
    if (value === undefined || value === null || value === false) return ''
    if (key === 'page') return Number(value) > 1 ? String(Math.floor(Number(value))) : ''
    return String(value)
  }

  function write(state) {
    if (!routed) return
    const route = current()
    if (!route || route.path !== ownPath) return
    const next = { ...route.query }
    for (const key of omit) delete next[key]
    for (const [key, value] of Object.entries(state)) {
      const text = encode(key, value)
      if (text) next[key] = text
      else delete next[key]
    }
    const before = route.query || {}
    const same = Object.keys(next).length === Object.keys(before).length &&
      Object.entries(next).every(([key, value]) => before[key] === value)
    if (same) return
    void router.replace({ path: route.path, query: next, hash: route.hash })
  }

  return { read, readPage, write }
}
