// Makes the page behind an open Select list inert (UiSelect).
//
// Reka's Select is modal in practice: pointer events outside it are off, Tab
// is swallowed and its content hides everything else from assistive
// technology (`aria-hidden` on every other child of <body>, with no prop to
// turn that off). The controls in the hidden part stayed focusable, which is
// what axe reports as "aria-hidden-focus" (serious): an `aria-hidden`
// element must not contain anything focusable.
//
// `inert` is the honest form of the same thing: the browser itself takes the
// rest of the page out of the tab order and the accessibility tree while the
// list is open, so nothing hidden can be focused. It is applied to the same
// set Reka hides (every sibling along the path from <body> to the list's
// layer; an element with an `aria-live` attribute and its ancestors excepted,
// the same exemption as Reka's hide-others pass) and taken off again, before
// Reka hands focus back to the trigger, the moment the list closes or the
// Select goes away.
//
// It waits until focus is inside the list. Reka moves focus there as soon as
// the list is positioned; inerting the trigger's own subtree earlier would
// blur it with no `relatedTarget` (a field that flags itself on blur could
// not tell "left the field" from "went into the list").
import { onBeforeUnmount, watch } from 'vue'

/**
 * Sets `inert` on everything outside `keep` and the live regions.
 * @param {HTMLElement} keep the element that stays interactive
 * @returns {() => void} puts every attribute it set back
 */
export function inertOthers(keep) {
  const body = keep.ownerDocument.body
  const keepers = [keep, ...body.querySelectorAll('[aria-live]')]
  const path = new Set()
  for (const element of keepers) {
    for (let node = element; node && node !== body.parentNode; node = node.parentNode) path.add(node)
  }
  const stops = new Set(keepers)
  const marked = []
  const walk = parent => {
    for (const child of parent.children) {
      if (path.has(child)) {
        if (!stops.has(child)) walk(child)
        continue
      }
      // Scripts do not matter to assistive technology; Reka's focus guards
      // must stay where they are for its own focus handling.
      if (child.localName === 'script' || child.hasAttribute('data-reka-focus-guard') || child.hasAttribute('inert')) continue
      child.setAttribute('inert', '')
      marked.push(child)
    }
  }
  walk(body)
  return () => {
    for (const element of marked) element.removeAttribute('inert')
  }
}

/**
 * @param {import('vue').Ref<boolean>} open whether the list is open
 * @param {HTMLElement | undefined} layer the element the list is portalled into
 */
export function useInertBehind(open, layer) {
  if (!layer) return
  let release = null

  function apply() {
    layer.removeEventListener('focusin', apply)
    if (!release) release = inertOthers(layer)
  }

  function disarm() {
    layer.removeEventListener('focusin', apply)
    release?.()
    release = null
  }

  // Sync, so the page is back in the tab order before Reka returns focus to
  // the trigger (a timer it starts when the list unmounts).
  watch(open, value => {
    if (!value) return disarm()
    if (layer.contains(layer.ownerDocument.activeElement)) apply()
    else layer.addEventListener('focusin', apply)
  }, { flush: 'sync', immediate: true })
  onBeforeUnmount(disarm)
}
