// Test helpers for pages that give feedback through the component library
// (useToast, useConfirm) instead of window.alert/confirm/prompt.
//
//   const confirms = answerConfirms(true)   // accept every confirmation
//   await wrapper.vm.handleDelete(row)
//   expect(confirms.last()).toMatchObject({ tone: 'danger', requireText: 'hk-01' })
//   expect(toastMessages('success')).toContain('Node deleted')
//
// answerConfirms() answers without rendering a dialog, so a page can be
// mounted on its own. To test the dialog itself (typed name, Esc, focus),
// mount the page next to <UiHost /> and use the testing-library queries.
// Both queues are module-level: setup.js resets them after every test.
//
// UiDialog and UiSheet render into document.body (a portal), outside the
// wrapper: mount with { attachTo: document.body } (pair it with
// enableAutoUnmount(afterEach)) and query with inBody()/allInBody().
import { watch } from 'vue'
import { DOMWrapper } from '@vue/test-utils'
import { confirmState, settleConfirm } from '@/ui/composables/useConfirm'
import { toastState } from '@/ui/composables/useToast'

const answerers = new Set()

/** Stop every answerConfirms() (setup.js calls it after each test). */
export function stopAnsweringConfirms() {
  for (const stop of [...answerers]) stop()
}

/**
 * Answer every useConfirm() request with `answer` (true, false, or a
 * function of the options returning either). With options.onConfirm the
 * callback runs first, as the real dialog does; when it throws, the error
 * is recorded in `errors` and the request resolves false (the real dialog
 * would stay open with the error inline until the user cancels).
 */
export function answerConfirms(answer = true) {
  const calls = []
  const errors = []
  const handled = new Set()
  let current = answer

  async function handle(item) {
    if (handled.has(item.id)) return
    handled.add(item.id)
    calls.push(item.options)
    const result = typeof current === 'function' ? current(item.options) : current
    if (result && typeof item.options.onConfirm === 'function') {
      try {
        await item.options.onConfirm()
      } catch (error) {
        errors.push(error)
        settleConfirm(item.id, false)
        return
      }
    }
    settleConfirm(item.id, result)
  }

  const unwatch = watch(
    () => confirmState.queue.map(item => item.id),
    () => { for (const item of [...confirmState.queue]) handle(item) },
    { immediate: true, flush: 'sync' }
  )
  const stop = () => {
    unwatch()
    answerers.delete(stop)
  }
  answerers.add(stop)

  return {
    calls,
    errors,
    last: () => calls.at(-1),
    answer(next) { current = next },
    stop
  }
}

/** Toasts currently shown, optionally of one tone. */
export function toasts(tone) {
  return toastState.toasts.filter(toast => !tone || toast.tone === tone)
}

/** Messages of the toasts currently shown, optionally of one tone. */
export function toastMessages(tone) {
  return toasts(tone).map(toast => toast.message)
}

/** A DOMWrapper for the first element in document.body matching selector. */
export function inBody(selector) {
  return new DOMWrapper(document.body.querySelector(selector))
}

/** DOMWrappers for every element in document.body matching selector. */
export function allInBody(selector) {
  return [...document.body.querySelectorAll(selector)].map(element => new DOMWrapper(element))
}

/** The open dialogs and sheets (role dialog or alertdialog). */
export function openDialogs() {
  return allInBody('[role="dialog"], [role="alertdialog"]')
}
