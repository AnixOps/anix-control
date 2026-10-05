// State of the node traffic chart: the range, the loaded series and its
// status. One request per range or node change; the answer of a request that
// a newer one (or leaving the page) has replaced is dropped. The series of an
// earlier range stays on screen, dimmed, while the next one loads; a failure
// clears it.
import { onScopeDispose, ref, shallowRef, unref, watch } from 'vue'
import { getKernelNodeTraffic } from '@/api/kernel'
import { adminV4ErrorMessage } from '@/utils/adminV4'
import { DEFAULT_TRAFFIC_RANGE, normalizeTrafficRange, readNodeTraffic, trafficQuery } from './nodeTraffic'

// The error the chart shows: the route's own message ({error: {message}}),
// with the status and request id kept for "copy error details". (The error
// state would print an {error: {...}} object as JSON, so the body is flattened.)
function failure(cause) {
  const message = adminV4ErrorMessage(cause, '')
  const response = cause?.response
  return Object.assign(new Error(message), {
    response: response ? { status: response.status, headers: response.headers, data: { message } } : undefined
  })
}

// `nodeId` is a number, a string or a ref/getter of one; `now` is the clock.
export function useNodeTraffic(nodeId, { initialRange = DEFAULT_TRAFFIC_RANGE, now = Date.now } = {}) {
  const range = ref(normalizeTrafficRange(initialRange))
  const status = ref('idle') // 'idle' | 'loading' | 'ready' | 'error'
  const series = shallowRef(null)
  const error = shallowRef(null)
  let seq = 0

  const idOf = () => (typeof nodeId === 'function' ? nodeId() : unref(nodeId))

  async function load() {
    const id = idOf()
    const mine = ++seq
    if (id === undefined || id === null || id === '') {
      series.value = null
      error.value = null
      status.value = 'idle'
      return
    }
    status.value = 'loading'
    error.value = null
    try {
      const answer = await getKernelNodeTraffic(id, trafficQuery(range.value, now()))
      if (mine !== seq) return
      series.value = readNodeTraffic(answer)
      status.value = 'ready'
    } catch (cause) {
      if (mine !== seq) return
      series.value = null
      error.value = failure(cause)
      status.value = 'error'
    }
  }

  function select(value) {
    range.value = normalizeTrafficRange(value)
  }

  watch([range, idOf], () => { void load() })
  onScopeDispose(() => { seq += 1 })

  return { range, status, series, error, load, select }
}
