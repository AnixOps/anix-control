// Reactive CSS media query (`matchMedia`), for components that render a
// different structure at a breakpoint (UiDataTable's phone cards), where
// CSS alone would keep both structures in the accessibility tree.
//
//   const isPhone = useMediaQuery(PHONE_QUERY)
//
// Without matchMedia (tests, server) it is always false.
import { getCurrentScope, onScopeDispose, ref } from 'vue'

// Breakpoint sm (640 px, frontend-design.md): phones are narrower.
export const PHONE_QUERY = '(max-width: 639.98px)'

export function useMediaQuery(query) {
  const matches = ref(false)
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return matches

  let list = null
  try {
    list = window.matchMedia(query)
  } catch {
    return matches
  }
  matches.value = Boolean(list?.matches)
  const onChange = (event) => { matches.value = Boolean(event.matches) }
  list?.addEventListener?.('change', onChange)
  if (getCurrentScope()) onScopeDispose(() => list?.removeEventListener?.('change', onChange))
  return matches
}
