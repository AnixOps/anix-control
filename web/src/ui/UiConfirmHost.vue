<template>
  <UiConfirmDialog
    v-if="current"
    :key="current.id"
    :open="open"
    :title="current.options.title"
    :message="current.options.message"
    :confirm-label="current.options.confirmLabel"
    :cancel-label="current.options.cancelLabel"
    :tone="current.options.tone"
    :require-text="current.options.requireText"
    :loading="busy"
    :error="error"
    @confirm="onConfirm"
    @cancel="settle(false)"
  />
</template>

<script setup>
// Renders the useConfirm() queue one dialog at a time. Mount once, through
// <UiHost>. With options.onConfirm the dialog stays open while it runs and
// shows its error inline if it throws.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmState, settleConfirm } from './composables/useConfirm'
import UiConfirmDialog from './UiConfirmDialog.vue'

const { t } = useI18n()
const current = computed(() => confirmState.queue[0] || null)
const open = ref(false)
const busy = ref(false)
const error = ref('')

watch(current, (item) => {
  busy.value = false
  error.value = ''
  open.value = Boolean(item)
}, { immediate: true })

function settle(result) {
  const item = current.value
  if (!item) return
  open.value = false
  settleConfirm(item.id, result)
}

async function onConfirm() {
  const item = current.value
  if (!item) return
  if (typeof item.options.onConfirm !== 'function') {
    settle(true)
    return
  }
  busy.value = true
  error.value = ''
  try {
    await item.options.onConfirm()
    busy.value = false
    settle(true)
  } catch (err) {
    busy.value = false
    const reason = err?.message || String(err || '')
    error.value = t('ui.confirm.failed', { reason })
  }
}
</script>
