import { onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

// useRouteIntent reads one-shot instructions from the URL query, such as the
// command palette's "添加节点" (/admin/nodes?create=1) or a user search
// result (/admin/users?email=...), and removes them from the address bar,
// so a reload or a shared link does not repeat them.
//
// It returns the values present when the page is set up (strings, by key).
// The palette can also send an instruction to the page that is already open
// (same route, new query); `onLater` receives those.
// Pages mounted without a router (unit tests) get an empty object.
export function useRouteIntent(keys, onLater) {
  const route = useRoute()
  const router = useRouter()

  function pick(query = {}) {
    const intent = {}
    for (const key of keys) {
      const value = Array.isArray(query[key]) ? query[key][0] : query[key]
      if (typeof value === 'string' && value !== '') intent[key] = value
    }
    return intent
  }

  function strip() {
    const rest = { ...route.query }
    for (const key of keys) delete rest[key]
    void router.replace({ path: route.path, query: rest, hash: route.hash })
  }

  const initial = pick(route?.query)
  if (!route || typeof router?.replace !== 'function') return initial

  if (Object.keys(initial).length > 0) onMounted(strip)

  // Only for this page: an instruction for the page being navigated to is
  // that page's to read.
  const ownPath = route.path
  watch(() => route.query, query => {
    if (route.path !== ownPath) return
    const next = pick(query)
    if (Object.keys(next).length === 0) return
    if (typeof onLater === 'function') onLater(next)
    strip()
  })

  return initial
}
