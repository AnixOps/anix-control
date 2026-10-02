// The time range of 流量与监控 (1 h / 24 h / 7 d / 30 d), shared by the
// traffic and latency sections and kept in the URL (?range=7d) so a link
// or a reload keeps it; the default (24 h) leaves the query clean. The
// hourly traffic API takes 1–720 hours; the latency trend takes from / to.
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export const MONITOR_RANGES = Object.freeze({ '1h': 1, '24h': 24, '7d': 168, '30d': 720 })
export const DEFAULT_RANGE = '24h'

export function useMonitorRange() {
  const route = useRoute()
  const router = useRouter()
  const local = ref(DEFAULT_RANGE)

  const range = computed({
    get() {
      const value = route ? route.query?.range : local.value
      return MONITOR_RANGES[value] ? value : DEFAULT_RANGE
    },
    set(value) {
      const next = MONITOR_RANGES[value] ? value : DEFAULT_RANGE
      local.value = next
      if (!route || !router) return
      const query = { ...route.query }
      if (next === DEFAULT_RANGE) delete query.range
      else query.range = next
      Promise.resolve(router.replace({ path: route.path, query, hash: route.hash })).catch(() => {})
    }
  })
  const hours = computed(() => MONITOR_RANGES[range.value])

  return { range, hours }
}
