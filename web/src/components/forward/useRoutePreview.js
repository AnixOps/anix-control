import { onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { isCanceled, previewRoute } from '@/api/forwardV4'

// useRoutePreview runs POST /routes/preview for the editor (D4): 1 s after
// the draft stops changing, or at once on refresh(). A newer preview
// cancels the one in flight, and nothing is asked while a required field
// is empty (missing() answers a non-empty list).
export function useRoutePreview(source, { delay = 1000, missing = () => [], request = previewRoute } = {}) {
  const result = shallowRef(null)
  const loading = ref(false)
  const error = shallowRef(null)
  const skipped = ref(true)
  const requestBody = shallowRef(null)
  let timer = null
  let controller = null
  let seq = 0

  function cancel() {
    clearTimeout(timer)
    timer = null
    if (controller) controller.abort()
    controller = null
  }

  async function run() {
    cancel()
    const route = source()
    if (missing().length) {
      skipped.value = true
      loading.value = false
      result.value = null
      requestBody.value = route
      return null
    }
    skipped.value = false
    const mine = ++seq
    const abort = typeof AbortController === 'function' ? new AbortController() : null
    controller = abort
    loading.value = true
    requestBody.value = route
    try {
      const answer = await request(route, { signal: abort?.signal })
      if (mine !== seq) return null
      result.value = {
        states: answer?.states || [],
        allocations: answer?.allocations || [],
        violations: answer?.violations || [],
        warnings: answer?.warnings || []
      }
      error.value = null
      return result.value
    } catch (cause) {
      if (isCanceled(cause) || mine !== seq) return null
      error.value = cause
      return null
    } finally {
      if (mine === seq) {
        loading.value = false
        controller = null
      }
    }
  }

  function schedule() {
    clearTimeout(timer)
    timer = setTimeout(() => { void run() }, delay)
  }

  watch(() => JSON.stringify(source()), schedule)
  onBeforeUnmount(cancel)

  return { result, loading, error, skipped, requestBody, refresh: run, schedule, cancel }
}
