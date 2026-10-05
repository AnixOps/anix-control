import { ref } from 'vue'
import { getUsersActivity } from '@/api/admin'

// Last online of the users of the page in view. The list renders from the v2
// answer; this is asked for afterwards, for the ids of that page, and never
// holds the list up. `lastOnlineOf(id)` is undefined until the answer is in, null
// for a user no node has reported, else Unix seconds. An answer to an
// earlier page or request is dropped, and a failure only sets `failed`.
export function useUserActivity() {
  const activity = ref({})
  const failed = ref(false)
  let sequence = 0

  async function load(ids) {
    const mine = ++sequence
    activity.value = {}
    failed.value = false
    if (!Array.isArray(ids) || !ids.length) return
    try {
      const rows = await getUsersActivity(ids)
      if (mine !== sequence) return
      const next = {}
      for (const row of rows) next[row.user_id] = Number.isFinite(row.last_online_at) ? row.last_online_at : null
      activity.value = next
    } catch (error) {
      if (mine !== sequence) return
      console.error('Failed to load when the users were last online:', error)
      failed.value = true
    }
  }

  function reset() {
    sequence += 1
    activity.value = {}
    failed.value = false
  }

  function lastOnlineOf(id) {
    return Object.prototype.hasOwnProperty.call(activity.value, id) ? activity.value[id] : undefined
  }

  return { activity, failed, load, reset, lastOnlineOf }
}
