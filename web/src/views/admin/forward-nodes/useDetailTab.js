// The open section of a detail page, kept in the URL (?tab=config) so a
// reload or a shared link opens the same section. Without a router (unit
// tests) it is a plain ref.
import { inject, ref, watch } from 'vue'
import { routeLocationKey, routerKey } from 'vue-router'

export function useDetailTab(values, fallback = values[0]) {
  const router = inject(routerKey, null)
  const route = inject(routeLocationKey, null)
  const initial = String(route?.query?.tab || '')
  const tab = ref(values.includes(initial) ? initial : fallback)

  if (router && route) {
    watch(tab, (value) => {
      const query = { ...route.query }
      if (value === fallback) delete query.tab
      else query.tab = value
      void router.replace({ path: route.path, query, hash: route.hash })
    })
  }
  return tab
}
