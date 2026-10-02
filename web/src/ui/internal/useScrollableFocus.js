// A scrolling region with nothing focusable inside cannot be scrolled from
// the keyboard (WCAG 2.1.1; axe scrollable-region-focusable). Dialog and
// sheet bodies call this: while the body overflows and holds no focusable
// element, it gets tabindex="0" so Tab reaches it and the arrows scroll it.
import { onBeforeUnmount, ref } from 'vue'

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable="true"]'

export function useScrollableFocus() {
  const tabindex = ref(undefined)
  let element = null
  let resizeObserver = null
  let mutationObserver = null

  function update() {
    if (!element) return
    const overflows = element.scrollHeight > element.clientHeight + 1 || element.scrollWidth > element.clientWidth + 1
    const hasFocusable = Boolean(element.querySelector(FOCUSABLE))
    tabindex.value = overflows && !hasFocusable ? '0' : undefined
  }

  function disconnect() {
    resizeObserver?.disconnect()
    mutationObserver?.disconnect()
    resizeObserver = null
    mutationObserver = null
  }

  function observeChildren() {
    if (!resizeObserver || !element) return
    resizeObserver.observe(element)
    for (const child of element.children) resizeObserver.observe(child)
  }

  // Function ref for the scrolling element.
  function setElement(el) {
    const next = el?.$el || el || null
    if (next === element) return
    disconnect()
    element = next
    if (!element) {
      tabindex.value = undefined
      return
    }
    if (typeof ResizeObserver === 'function') {
      resizeObserver = new ResizeObserver(update)
      observeChildren()
    }
    if (typeof MutationObserver === 'function') {
      mutationObserver = new MutationObserver(() => {
        resizeObserver?.disconnect()
        observeChildren()
        update()
      })
      mutationObserver.observe(element, { childList: true, subtree: true, attributes: true, attributeFilter: ['disabled', 'tabindex', 'href'] })
    }
    update()
  }

  onBeforeUnmount(disconnect)

  return { tabindex, setElement }
}
