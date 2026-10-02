// Toast queue shared by the whole app. <UiToastRegion> (mounted once, through
// <UiHost>) renders it; any component calls useToast().
//
// Interaction rules (docs/reference/frontend-design.md, Components):
// - success: disappears after 3 s;
// - error: stays until dismissed (errors in forms go inline first);
// - undo: a success with an "Undo" action that stays 5 s;
// - timers pause while the pointer is over the region or focus is in it,
//   and while the page is hidden;
// - one live region announces each new toast; F8 moves focus to the region.
import { reactive, readonly } from 'vue'

export const TOAST_DURATIONS = Object.freeze({
  success: 3000,
  info: 4000,
  warning: 5000,
  error: 0,
  undo: 5000
})

const MAX_VISIBLE = 3

const state = reactive({
  toasts: [],
  // Text for the single live region; `seq` changes on every announcement so
  // the same message announced twice is still read.
  announcement: { text: '', seq: 0 }
})

const timers = new Map()
let nextId = 1
let paused = false

function clearTimer(id) {
  const timer = timers.get(id)
  if (timer?.handle) clearTimeout(timer.handle)
}

function startTimer(toast) {
  if (!toast.duration) return
  const timer = timers.get(toast.id) || { remaining: toast.duration, handle: null, started: 0 }
  timers.set(toast.id, timer)
  if (paused) return
  timer.started = Date.now()
  timer.handle = setTimeout(() => dismiss(toast.id), timer.remaining)
}

export function dismiss(id) {
  clearTimer(id)
  timers.delete(id)
  const index = state.toasts.findIndex(toast => toast.id === id)
  if (index !== -1) state.toasts.splice(index, 1)
}

export function clear() {
  for (const toast of [...state.toasts]) dismiss(toast.id)
}

export function pauseToasts() {
  if (paused) return
  paused = true
  for (const timer of timers.values()) {
    if (timer.handle) {
      clearTimeout(timer.handle)
      timer.handle = null
      timer.remaining = Math.max(0, timer.remaining - (Date.now() - timer.started))
    }
  }
}

export function resumeToasts() {
  if (!paused) return
  paused = false
  for (const toast of state.toasts) {
    if (timers.has(toast.id)) startTimer(toast)
  }
}

/**
 * Show a toast.
 * @param {object} options
 * @param {string} options.message  One short sentence about the result.
 * @param {'success'|'error'|'info'|'warning'} [options.tone='info']
 * @param {number} [options.duration]  ms; 0 keeps it until dismissed.
 * @param {{ label: string, onAction: Function }} [options.action]
 * @param {Function} [options.undo]  Adds the "Undo" action (5 s).
 * @returns {number} the toast id, for dismiss(id)
 */
export function show(options) {
  const opts = typeof options === 'string' ? { message: options } : { ...options }
  const tone = opts.tone || 'info'
  let action = opts.action || null
  if (typeof opts.undo === 'function') {
    action = { label: null, undo: true, onAction: opts.undo }
  }
  const duration = opts.duration ?? (action?.undo ? TOAST_DURATIONS.undo : TOAST_DURATIONS[tone] ?? TOAST_DURATIONS.info)
  const toast = { id: nextId++, tone, message: String(opts.message ?? ''), action, duration }
  state.toasts.push(toast)
  while (state.toasts.length > MAX_VISIBLE) {
    // Drop the oldest toast that is not an error; errors wait for the user.
    const victim = state.toasts.find(item => item.tone !== 'error') || state.toasts[0]
    dismiss(victim.id)
  }
  state.announcement = { text: toast.message, seq: state.announcement.seq + 1, hasAction: Boolean(action) }
  startTimer(toast)
  return toast.id
}

export async function runAction(id) {
  const toast = state.toasts.find(item => item.id === id)
  if (!toast?.action) return
  dismiss(id)
  await toast.action.onAction?.()
}

export const toastState = readonly(state)

export function useToast() {
  return {
    show,
    success: (message, options = {}) => show({ ...options, message, tone: 'success' }),
    error: (message, options = {}) => show({ ...options, message, tone: 'error' }),
    info: (message, options = {}) => show({ ...options, message, tone: 'info' }),
    warning: (message, options = {}) => show({ ...options, message, tone: 'warning' }),
    dismiss,
    clear
  }
}

// Test hook: forget every toast and timer.
export function resetToasts() {
  clear()
  paused = false
  state.announcement = { text: '', seq: 0 }
}
