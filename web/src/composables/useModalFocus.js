import { nextTick, onBeforeUnmount, watch } from 'vue'

const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'summary',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

export function useModalFocus({ open, canClose, container, initialFocus, close }) {
  let restoreTarget = null
  let transition = 0

  function isRestorableTarget(element) {
    return Boolean(
      typeof HTMLElement !== 'undefined' &&
      element instanceof HTMLElement &&
      element !== document.body &&
      element !== document.documentElement &&
      element.tabIndex >= 0
    )
  }

  function focusableElements() {
    return [...(container.value?.querySelectorAll(focusableSelector) || [])].filter(element => (
      !element.hasAttribute('hidden') &&
      !element.closest('[hidden]') &&
      element.getAttribute('aria-hidden') !== 'true' &&
      !element.closest('[inert]')
    ))
  }

  function restoreFocus() {
    const target = restoreTarget
    restoreTarget = null
    if (target?.isConnected && typeof target.focus === 'function') target.focus()
  }

  watch(open, async (isOpen) => {
    const currentTransition = ++transition
    if (isOpen) {
      const activeElement = typeof document === 'undefined' ? null : document.activeElement
      restoreTarget = isRestorableTarget(activeElement) ? activeElement : null
      await nextTick()
      if (open.value && currentTransition === transition) initialFocus.value?.focus?.()
      return
    }

    await nextTick()
    if (!open.value && currentTransition === transition) restoreFocus()
  }, { immediate: true })

  function requestClose() {
    if (canClose.value) close()
  }

  function handleKeydown(event) {
    if (!open.value) return

    if (event.key === 'Escape') {
      event.preventDefault()
      requestClose()
      return
    }

    if (event.key !== 'Tab') return
    const elements = focusableElements()
    if (elements.length === 0) {
      event.preventDefault()
      container.value?.focus?.()
      return
    }

    const first = elements[0]
    const last = elements.at(-1)
    const activeElement = typeof document === 'undefined' ? null : document.activeElement
    if (!container.value?.contains(activeElement) || (event.shiftKey && activeElement === first)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && activeElement === last) {
      event.preventDefault()
      first.focus()
    }
  }

  onBeforeUnmount(() => {
    transition += 1
    if (open.value) restoreFocus()
  })

  return { handleKeydown, requestClose }
}
