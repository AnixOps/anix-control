import { onBeforeUnmount, ref } from 'vue'

// The shells switch layout at md (834 px, plan §5.3): below it the admin
// sidebar becomes a drawer and the user navigation a bottom tab bar.
export const NARROW_QUERY = '(max-width: 833.98px)'

function evaluate(query) {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia(query).matches
}

// useMediaQuery: a ref that follows a media query while the component lives.
export function useMediaQuery(query) {
  const matches = ref(evaluate(query))
  let list = null
  const update = event => {
    matches.value = typeof event?.matches === 'boolean' ? event.matches : evaluate(query)
  }
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    list = window.matchMedia(query)
    matches.value = Boolean(list.matches)
    if (typeof list.addEventListener === 'function') list.addEventListener('change', update)
    else if (typeof list.addListener === 'function') list.addListener(update)
  }
  onBeforeUnmount(() => {
    if (!list) return
    if (typeof list.removeEventListener === 'function') list.removeEventListener('change', update)
    else if (typeof list.removeListener === 'function') list.removeListener(update)
  })
  return matches
}
