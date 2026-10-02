// Unsaved changes on a settings page (plan §9 "离开未保存页面提示").
//
//   const { confirmLeave } = useUnsavedChanges(dirty, { discard })
//
// While `dirty` is true, leaving the route or changing its path (another
// settings section) asks first; 放弃更改 runs `discard` and lets the
// navigation go on, 继续编辑 stays. A query-only change never asks. Closing or
// reloading the tab gets the browser's own prompt. Works in any component
// rendered inside a <router-view>; outside one (unit tests) only the tab
// prompt and confirmLeave() are active.
import { computed, getCurrentInstance, inject, onBeforeUnmount, ref, toValue } from 'vue'
import { matchedRouteKey, onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'
import { useConfirm } from '@/ui/composables/useConfirm'

export function useUnsavedChanges(dirty, { discard } = {}) {
  const { t } = useAppI18n()
  const confirm = useConfirm()

  async function confirmLeave() {
    if (!toValue(dirty)) return true
    const leave = await confirm({
      title: t('settingsForm.leave.title'),
      message: t('settingsForm.leave.message'),
      confirmLabel: t('settingsForm.leave.discard'),
      cancelLabel: t('settingsForm.leave.stay'),
      tone: 'danger'
    })
    if (leave) discard?.()
    return leave
  }

  if (getCurrentInstance() && inject(matchedRouteKey, null)) {
    onBeforeRouteLeave(() => confirmLeave())
    onBeforeRouteUpdate((to, from) => (to.path === from.path ? true : confirmLeave()))
  }

  function onBeforeUnload(event) {
    if (!toValue(dirty)) return
    event.preventDefault()
    // Older browsers need returnValue set to show the prompt.
    event.returnValue = ''
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('beforeunload', onBeforeUnload)
    if (getCurrentInstance()) {
      onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))
    }
  }

  return { confirmLeave }
}

// useDraft: an editable copy of a settings form and whether it differs from
// what was loaded or last saved.
//
//   const { draft, dirty, commit, discard } = useDraft(() => ({ enabled: false }))
//   commit(loaded)   // after a load or a save: the new baseline
//   discard()        // back to the baseline
export function useDraft(factory) {
  const draft = ref(factory())
  const baseline = ref(JSON.stringify(draft.value))
  const dirty = computed(() => JSON.stringify(draft.value) !== baseline.value)

  function commit(value = draft.value) {
    const snapshot = JSON.stringify(value)
    draft.value = JSON.parse(snapshot)
    baseline.value = snapshot
  }

  function discard() {
    draft.value = JSON.parse(baseline.value)
  }

  return { draft, dirty, commit, discard }
}
