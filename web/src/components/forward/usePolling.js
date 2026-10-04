import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

// usePolling runs load() now and then every `interval` ms while the page is
// visible (D5: nodes and route detail 15 s, overview 30 s). A hidden tab
// stops polling; coming back loads at once when the data is stale.
// `updatedAt` and `secondsAgo` drive the "更新于 N 秒前" line.
export function usePolling(load, { interval = 15_000, immediate = true, now = () => Date.now() } = {}) {
  const loading = ref(false)
  const error = ref(null)
  const updatedAt = ref(0)
  const tick = ref(now())
  let timer = null
  let ticker = null
  let running = null
  let stopped = false

  function hidden() {
    return typeof document !== 'undefined' && document.visibilityState === 'hidden'
  }

  function schedule() {
    clearTimeout(timer)
    timer = null
    if (stopped || hidden() || !interval) return
    timer = setTimeout(() => { void refresh() }, interval)
  }

  async function refresh() {
    if (running) return running
    loading.value = true
    running = (async () => {
      try {
        await load()
        error.value = null
        updatedAt.value = now()
        tick.value = updatedAt.value
      } catch (cause) {
        error.value = cause
      } finally {
        loading.value = false
        running = null
        schedule()
      }
    })()
    return running
  }

  function onVisibility() {
    if (hidden()) {
      clearTimeout(timer)
      timer = null
      return
    }
    if (now() - updatedAt.value >= interval) void refresh()
    else schedule()
  }

  onMounted(() => {
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility)
    ticker = setInterval(() => { tick.value = now() }, 1000)
    if (immediate) void refresh()
    else schedule()
  })

  onBeforeUnmount(() => {
    stopped = true
    clearTimeout(timer)
    clearInterval(ticker)
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility)
  })

  const secondsAgo = computed(() => (updatedAt.value ? Math.max(0, Math.round((tick.value - updatedAt.value) / 1000)) : null))

  return { loading, error, updatedAt, secondsAgo, refresh }
}
