// Loading states appear only after 300 ms (frontend-design.md, Components):
// a fast response shows no flicker, a slow one shows a skeleton.
//
//   const showSkeleton = useDelayedLoading(loading)          // Ref<boolean>
//   <UiSkeleton v-if="showSkeleton" variant="table-row" />
import { getCurrentScope, onScopeDispose, ref, toValue, watch } from 'vue'

export const LOADING_DELAY = 300

export function useDelayedLoading(source, delay = LOADING_DELAY) {
  const visible = ref(false)
  let handle = null

  function stop() {
    if (handle) clearTimeout(handle)
    handle = null
  }

  watch(() => Boolean(toValue(source)), (loading) => {
    stop()
    if (!loading) {
      visible.value = false
      return
    }
    if (delay <= 0) {
      visible.value = true
      return
    }
    handle = setTimeout(() => {
      handle = null
      visible.value = true
    }, delay)
  }, { immediate: true })

  if (getCurrentScope()) onScopeDispose(stop)
  return visible
}
