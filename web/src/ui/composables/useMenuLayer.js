// The layer an open drop-down list is portalled into: UiMenu, AccountMenu,
// and the lists of UiSelect and UiCombobox.
//
// Reka renders menu content in a portal on <body>, outside every landmark,
// so axe reports its items under "region" (all content belongs in a
// landmark). Moving the portal into the page's own landmark is not an
// option: a fixed-position menu would sit under the top bar's
// backdrop-filter or a dialog's transform and clip. So each menu gets its
// own layer, a labelled `region` on <body>, that exists only while the menu
// (or list) is open:
//
// - no empty landmark is left behind for screen readers to list;
// - it never exists when a modal dialog opens, so Reka's hide-others pass
//   (aria-hidden on every other child of <body>) cannot hide it, and a menu
//   opened inside a dialog stays readable.
//
// The element is made once per menu and attached while `open` is true. It
// stays the Teleport target throughout, so the portal never moves, and it
// removes itself when Reka has taken the menu content out again. "Content"
// is an element with a role (menu, listbox): a closed Select keeps an empty
// placeholder <div> in its portal (it renders its options into a fragment so
// SelectValue can read them), which must not keep the layer alive.
import { onBeforeUnmount, watch } from 'vue'

/**
 * @param {import('vue').Ref<boolean>} open whether the menu is open
 * @param {() => string} label the layer's accessible name
 * @returns {HTMLElement | undefined} the `to` of the menu's portal
 */
export function useMenuLayer(open, label) {
  if (typeof document === 'undefined') return undefined
  const layer = document.createElement('div')
  layer.className = 'ui-menu-layer'
  layer.setAttribute('role', 'region')
  let observer = null

  const hasContent = () => Boolean(layer.querySelector('[role]'))

  // Detaches once the menu is closed and Reka has removed its content.
  function settle() {
    if (!open.value && !hasContent()) detach()
  }

  function detach() {
    observer?.disconnect()
    observer = null
    layer.remove()
  }

  function attach() {
    layer.setAttribute('aria-label', label())
    if (!layer.isConnected) document.body.append(layer)
    if (observer || typeof MutationObserver === 'undefined') return
    // Content mounting is the first change, its removal the last one.
    observer = new MutationObserver(settle)
    observer.observe(layer, { childList: true })
  }

  // Sync: the layer is on <body> before the menu content is rendered. On
  // close it goes at once when nothing was ever rendered into it, else when
  // the observer sees the content leave.
  watch(open, value => { if (value) attach(); else settle() }, { flush: 'sync', immediate: true })
  onBeforeUnmount(detach)
  return layer
}
