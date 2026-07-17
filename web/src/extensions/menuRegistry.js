// The parent names are part of the signed WebUI contract. Keep the list small
// and explicit so a package cannot create arbitrary top-level navigation groups.
export const WEBUI_MENU_PARENTS = Object.freeze([
  'services',
  'operations',
  'system'
])

export const WEBUI_MENU_FALLBACK_PARENT = 'extensions'

export const WEBUI_MENU_PARENT_REGISTRY = Object.freeze([
  ...WEBUI_MENU_PARENTS,
  WEBUI_MENU_FALLBACK_PARENT
])

const parentSet = new Set(WEBUI_MENU_PARENTS)
const parentTokenPattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,119}$/

/**
 * Parent values are display metadata, but they still cross the signed package
 * boundary. Accept only a safe token and normalize unknown values into the
 * explicit extension bucket used by the shell.
 */
export function isWebUIMenuParentToken(value) {
  return value === '' || (typeof value === 'string' && parentTokenPattern.test(value))
}

export function normalizeWebUIMenuParent(value) {
  if (typeof value !== 'string') {
    return WEBUI_MENU_FALLBACK_PARENT
  }
  const normalized = value.trim().toLowerCase()
  return parentSet.has(normalized) ? normalized : WEBUI_MENU_FALLBACK_PARENT
}
