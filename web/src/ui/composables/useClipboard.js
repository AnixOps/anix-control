// Copy text to the clipboard. navigator.clipboard only exists in secure
// contexts (HTTPS or localhost); self-hosted panels are often reached over
// plain HTTP, so fall back to a hidden textarea and execCommand('copy').
export async function copyText(text) {
  const value = String(text ?? '')
  // navigator.clipboard only exists in secure contexts, so its presence is the test.
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return true
    } catch {
      // Permission denied or not focused: try the fallback.
    }
  }
  return legacyCopy(value)
}

function legacyCopy(value) {
  if (typeof document === 'undefined' || typeof document.execCommand !== 'function') return false
  const active = document.activeElement
  const area = document.createElement('textarea')
  area.value = value
  area.setAttribute('readonly', '')
  area.setAttribute('aria-hidden', 'true')
  area.style.position = 'fixed'
  area.style.top = '0'
  area.style.left = '0'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.select()
  let ok = false
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }
  area.remove()
  if (active && typeof active.focus === 'function') active.focus()
  return ok
}
