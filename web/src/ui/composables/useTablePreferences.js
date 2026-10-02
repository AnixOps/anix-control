// Per-table view preferences (hidden columns, density) remembered in this
// browser under `anix.table.<id>`. Storage can be missing or throw (private
// windows, blocked site data): every read and write is guarded, and the
// table works with the defaults.
import { ref, watch } from 'vue'

export const TABLE_STORAGE_PREFIX = 'anix.table.'
export const DENSITIES = ['comfortable', 'compact']

function readStored(key) {
  if (!key) return null
  try {
    const raw = globalThis.localStorage?.getItem(TABLE_STORAGE_PREFIX + key)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function writeStored(key, value) {
  if (!key) return
  try {
    globalThis.localStorage?.setItem(TABLE_STORAGE_PREFIX + key, JSON.stringify(value))
  } catch {
    // Not remembered; the table keeps working.
  }
}

/**
 * @param {string} key storage id ('' = not remembered)
 * @param {{ hidden?: string[], density?: string }} defaults
 */
export function useTablePreferences(key, defaults = {}) {
  const stored = readStored(key)
  const hidden = ref(Array.isArray(stored?.hidden) ? stored.hidden.filter(item => typeof item === 'string') : [...(defaults.hidden || [])])
  const density = ref(DENSITIES.includes(stored?.density) ? stored.density : (DENSITIES.includes(defaults.density) ? defaults.density : 'comfortable'))

  watch([hidden, density], () => {
    writeStored(key, { hidden: hidden.value, density: density.value })
  }, { deep: true })

  return { hidden, density }
}
