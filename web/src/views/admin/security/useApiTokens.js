// State of the API tokens list (GET /api/v4/kernel/api-tokens) and of revoking
// a token (DELETE /api/v4/kernel/api-tokens/:id).
//
// The list shows the caller's active tokens. Two chips widen it:
// `showEnded` adds revoked and expired ones (include_inactive) and `showAll`
// lists every administrator's tokens (all=true), which only a super
// administrator may do. The console does not know who is one (the profile has
// no such field), so the chip is offered to everyone and a 403
// `super_admin_required` turns it off for good, as the rotate-credentials
// button does. The rows carry the owner's id and nothing else about them.
//
// A request that answers after a newer one started is dropped.
import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import { listKernelApiTokens, revokeKernelApiToken } from '@/api/kernel'
import { API_TOKEN_MAX_ACTIVE, classifyTokenRefusal, isActiveToken, readApiTokens } from './apiTokens'

// `me` is the signed-in administrator's id (or a getter of it); `now` the clock.
export function useApiTokens({ me = null, now = () => Date.now() } = {}) {
  const meId = () => Number((typeof me === 'function' ? me() : me) || 0)

  const tokens = shallowRef([])
  // Whether the rows on screen came from a request for everyone's tokens.
  const loadedAll = ref(false)
  const loading = ref(false)
  const error = shallowRef(null)
  const loaded = ref(false)
  const showAll = ref(false)
  const showEnded = ref(false)
  // null until known; false after the route said 403 super_admin_required.
  const superAdmin = ref(null)
  let sequence = 0
  let disposed = false
  // Set while the composable changes a switch itself: that reload is explicit.
  let adjusting = false

  async function load() {
    const mine = ++sequence
    const all = showAll.value && superAdmin.value !== false
    loading.value = true
    try {
      const answer = await listKernelApiTokens({ all, includeInactive: showEnded.value })
      if (mine !== sequence) return false
      tokens.value = readApiTokens(answer)
      loadedAll.value = all
      error.value = null
      loaded.value = true
      return true
    } catch (cause) {
      if (mine !== sequence) return false
      const refusal = classifyTokenRefusal(cause)
      if (all && refusal.kind === 'super_admin_required') {
        // Not a super administrator: back to the caller's own tokens.
        superAdmin.value = false
        adjusting = true
        showAll.value = false
        adjusting = false
        return load()
      }
      console.error('Failed to load the API tokens:', refusal.status || refusal.message)
      tokens.value = []
      error.value = cause
      return false
    } finally {
      if (mine === sequence) loading.value = false
    }
  }

  // Whose a row is: every row of the caller's own list is theirs; in the list of
  // everyone's, compare ids.
  function isMine(token) {
    return !loadedAll.value || (meId() > 0 && token.userId === meId())
  }

  // The caller's active tokens in the rows on screen, for the limit of 25; null
  // until a list has loaded. Rows of other administrators do not count.
  const ownActive = computed(() => {
    if (!loaded.value || error.value) return null
    const at = now()
    return tokens.value.filter(token => isMine(token) && isActiveToken(token, at)).length
  })
  const limitReached = computed(() => ownActive.value !== null && ownActive.value >= API_TOKEN_MAX_ACTIVE)

  // Ends a token. Resolves { changed } (false: it was revoked already) and
  // throws the classified refusal on a failure; a 404 means it is gone or not
  // the caller's, so the list is stale and reloads.
  async function revoke(token) {
    try {
      const answer = await revokeKernelApiToken(token.id)
      return { changed: answer?.changed !== false }
    } catch (cause) {
      const refusal = classifyTokenRefusal(cause)
      if (refusal.kind === 'not_found' && !disposed) load()
      throw Object.assign(new Error(refusal.message), { refusal })
    }
  }

  // Synchronous, so `adjusting` still says who changed the switch.
  watch([showAll, showEnded], () => { if (!adjusting) load() }, { flush: 'sync' })
  onScopeDispose(() => {
    disposed = true
    sequence += 1
  })

  return { tokens, loading, loaded, error, showAll, showEnded, superAdmin, loadedAll, ownActive, limitReached, isMine, load, revoke }
}
