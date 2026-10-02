// Scroll restoration (plan §8.3): back/forward returns to where the list
// was, a new page starts at the top, and a query-only change (filters,
// pagination written to the URL) keeps the position.
//
// Pages load their data after they mount, so a saved position may not be
// reachable yet: wait (up to `timeout`) until the document is tall enough,
// then jump without the smooth-scroll animation base.css sets on <html>.

export const RESTORE_TIMEOUT = 1500

function reachable(top) {
  if (typeof document === 'undefined' || typeof window === 'undefined') return true
  const root = document.documentElement
  return root.scrollHeight - window.innerHeight >= top
}

export function waitUntilReachable(top, { timeout = RESTORE_TIMEOUT, now = () => Date.now(), frame } = {}) {
  const nextFrame = frame || (callback => (typeof requestAnimationFrame === 'function'
    ? requestAnimationFrame(callback)
    : setTimeout(callback, 16)))
  const started = now()
  return new Promise(resolve => {
    const check = () => {
      if (reachable(top) || now() - started >= timeout) {
        resolve()
        return
      }
      nextFrame(check)
    }
    check()
  })
}

export async function scrollBehavior(to, from, savedPosition) {
  if (savedPosition) {
    await waitUntilReachable(savedPosition.top || 0)
    return { left: savedPosition.left || 0, top: savedPosition.top || 0, behavior: 'instant' }
  }
  if (to.hash) {
    return { el: to.hash, top: 72 }
  }
  if (from && to.path === from.path) {
    return false
  }
  return { left: 0, top: 0, behavior: 'instant' }
}
