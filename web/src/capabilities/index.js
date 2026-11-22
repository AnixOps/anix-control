import { readonly, shallowRef } from 'vue'
import { getAdminCapabilities } from '@/api/kernel'

// The package capabilities the admin app reads (GET /api/v3/capabilities):
// forward.v4, the forward package's v4 API, opens the forwarding pages
// (docs/design/forward-ui, D1). Control answers only the capabilities whose
// package is enabled and whose API the administrator may call.

const REFRESH_INTERVAL_MS = 30_000
const current = shallowRef([])
let loadedAt = 0
let pending = null
let generation = 0

export const adminCapabilities = readonly(current)

export function ensureAdminCapabilities() {
  if (loadedAt && Date.now() - loadedAt < REFRESH_INTERVAL_MS) return Promise.resolve(current.value)
  if (pending) return pending
  const run = ++generation
  pending = getAdminCapabilities()
    .then((capabilities) => {
      if (run === generation) {
        current.value = Array.isArray(capabilities) ? capabilities : []
        loadedAt = Date.now()
      }
      return current.value
    })
    .finally(() => {
      if (run === generation) pending = null
    })
  return pending
}

// Sign-out and a lost admin session forget the answer.
export function resetAdminCapabilities() {
  generation += 1
  pending = null
  loadedAt = 0
  current.value = []
}
