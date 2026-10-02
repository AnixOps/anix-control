<template>
  <UiDialog
    :open="open"
    size="lg"
    :title="t('runtime.forward.exportModal.title')"
    :description="t('runtime.forward.exportModal.subtitle')"
    data-test="forward-export-dialog"
    @update:open="value => emit('update:open', value)"
  >
    <div class="dialog-section">
      <div v-if="source === 'tunnel'" class="export-pick">
        <UiSelect
          class="export-pick__select"
          :model-value="tunnelId ?? undefined"
          :label="t('runtime.forward.exportModal.tunnelLabel')"
          :placeholder="t('runtime.forward.exportModal.tunnelPlaceholder')"
          :options="tunnelOptions"
          @update:model-value="value => emit('update:tunnelId', value ? Number(value) : null)"
        />
        <UiButton
          variant="secondary"
          size="lg"
          :loading="loading"
          :disabled="!tunnelId"
          data-test="forward-export-generate"
          @click="emit('generate')"
        >
          {{ data ? t('runtime.forward.exportModal.regenerate') : t('runtime.forward.exportModal.generate') }}
        </UiButton>
      </div>
      <p v-else class="export-hint">{{ t('runtime.forward.exportModal.selectionHint', { count: selectionCount }) }}</p>

      <UiTextarea
        v-if="data"
        class="mono-area"
        :model-value="data"
        :label="t('runtime.forward.exportModal.dataLabel')"
        :rows="12"
        readonly
      />
    </div>

    <template #footer="{ close }">
      <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
      <UiButton v-if="data" variant="primary" :icon="Copy" data-test="forward-export-copy" @click="emit('copy')">{{ t('common.actions.copy') }}</UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Export forward rules as relay-panel JSON: the rules of one tunnel, or the
// rules selected in the direct view. The page builds the text.
import { computed } from 'vue'
import { Copy } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextarea from '@/ui/UiTextarea.vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  // 'tunnel' (pick a tunnel, then generate) or 'selection' (bulk export).
  source: { type: String, default: 'tunnel' },
  tunnels: { type: Array, default: () => [] },
  tunnelId: { type: Number, default: null },
  data: { type: String, default: '' },
  selectionCount: { type: Number, default: 0 },
  loading: { type: Boolean, default: false }
})

const emit = defineEmits(['update:open', 'update:tunnelId', 'generate', 'copy'])
const { t } = useAppI18n()

const tunnelOptions = computed(() => props.tunnels.map(tunnel => ({ value: Number(tunnel.id), label: tunnel.name })))
</script>

<style scoped>
.export-pick {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: flex-end;
}

.export-pick__select {
  flex: 1 1 240px;
}

.export-hint {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.mono-area :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}
</style>
