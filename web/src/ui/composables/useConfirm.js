// Promise-based confirmation, the replacement for window.confirm().
//
//   const confirm = useConfirm()
//   if (await confirm({ title: '删除节点 hk-01？', message: '此操作无法撤销。',
//                       confirmLabel: '删除节点', tone: 'danger', requireText: 'hk-01' })) { ... }
//
// With `onConfirm` the dialog stays open, shows the confirm button as busy
// while the promise runs, closes when it resolves and shows the error inline
// when it rejects (the promise from confirm() then resolves true only after
// onConfirm succeeded).
//
// <UiConfirmHost> (mounted once, through <UiHost>) renders the queue, one
// dialog at a time. Only ask for irreversible or wide-reaching actions; prefer
// an undo toast for the rest (frontend-design.md, Components).
import { reactive, readonly } from 'vue'

const state = reactive({ queue: [] })
let nextId = 1

export function requestConfirm(options = {}) {
  return new Promise((resolve) => {
    state.queue.push({ id: nextId++, options: { ...options }, resolve })
  })
}

export function settleConfirm(id, result) {
  const index = state.queue.findIndex(item => item.id === id)
  if (index === -1) return
  const [item] = state.queue.splice(index, 1)
  item.resolve(Boolean(result))
}

export const confirmState = readonly(state)

export function useConfirm() {
  return requestConfirm
}

// Test hook: cancel everything that is still open.
export function resetConfirms() {
  for (const item of [...state.queue]) settleConfirm(item.id, false)
}
