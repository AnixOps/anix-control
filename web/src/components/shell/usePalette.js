import { ref } from 'vue'

// One command palette per app: the admin shell mounts it, the top-bar search
// button and the ⌘K / Ctrl+K shortcut open it.
const open = ref(false)

export function isApplePlatform() {
  if (typeof navigator === 'undefined') return false
  const platform = navigator.userAgentData?.platform || navigator.platform || ''
  return /mac|iphone|ipad|ipod/i.test(platform)
}

export function paletteShortcutLabel() {
  return isApplePlatform() ? '⌘K' : 'Ctrl K'
}

// ⌘K on Apple platforms, Ctrl+K elsewhere (both accepted everywhere, so a
// remote desktop or an external keyboard still works). Not with Alt or Shift.
export function isPaletteShortcut(event) {
  return Boolean(event &&
    (event.metaKey || event.ctrlKey) &&
    !event.altKey &&
    !event.shiftKey &&
    String(event.key || '').toLowerCase() === 'k')
}

export function usePalette() {
  return {
    open,
    openPalette: () => { open.value = true },
    closePalette: () => { open.value = false },
    togglePalette: () => { open.value = !open.value }
  }
}
