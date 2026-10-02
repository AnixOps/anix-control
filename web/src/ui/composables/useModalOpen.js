// True while a modal overlay (UiDialog, UiSheet, ConfirmDialog: Reka puts
// role="dialog" / "alertdialog" with aria-modal in a portal on <body>) is
// open. Page chrome that floats above the content, such as the DataTable
// bulk bar, uses it to step back behind the overlay.
import { getCurrentScope, onScopeDispose, ref } from 'vue'

const MODAL_SELECTOR = '[role="dialog"][aria-modal="true"], [role="alertdialog"]'

export function isModalOpen(root = globalThis.document) {
  return Boolean(root?.querySelector?.(MODAL_SELECTOR))
}

export function useModalOpen() {
  const open = ref(isModalOpen())
  if (typeof MutationObserver === 'undefined' || !globalThis.document?.body) return open
  const observer = new MutationObserver(() => { open.value = isModalOpen() })
  observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['role', 'aria-modal'] })
  if (getCurrentScope()) onScopeDispose(() => observer.disconnect())
  return open
}
