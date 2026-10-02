<template>
  <UiDialog
    :open="open"
    size="lg"
    :title="t('runtime.forward.importModal.title')"
    :description="t('runtime.forward.importModal.subtitle')"
    :dismissible="!loading"
    data-test="forward-import-dialog"
    @update:open="value => emit('update:open', value)"
  >
    <div class="dialog-section">
      <p class="import-example">{{ t('runtime.forward.importModal.subtitleSecondary') }}</p>
      <UiSelect
        :model-value="tunnelId ?? undefined"
        :label="t('runtime.forward.importModal.tunnelLabel')"
        :placeholder="t('runtime.forward.importModal.tunnelPlaceholder')"
        :options="tunnelOptions"
        required
        @update:model-value="value => emit('update:tunnelId', value ? Number(value) : null)"
      />
      <UiTextarea
        class="mono-area"
        :model-value="data"
        :label="t('runtime.forward.importModal.dataLabel')"
        :placeholder="t('runtime.forward.importModal.placeholder')"
        :rows="8"
        required
        @update:model-value="value => emit('update:data', value)"
      />
    </div>

    <section v-if="results.length" class="dialog-section import-results" aria-labelledby="forward-import-results">
      <div class="import-results__head">
        <h3 id="forward-import-results" class="dialog-section__title">{{ t('runtime.forward.importModal.resultTitle') }}</h3>
        <span class="import-results__summary">{{ t('runtime.forward.importModal.resultSummary', { success: successCount, total: results.length }) }}</span>
      </div>
      <ul class="import-results__list" tabindex="0" aria-labelledby="forward-import-results" data-test="forward-import-results">
        <li
          v-for="(result, index) in results"
          :key="`${result.line}-${index}`"
          class="import-result"
        >
          <UiBadge
            :tone="result.success ? 'success' : 'danger'"
            :label="result.success ? t('runtime.forward.importModal.statusSuccess') : t('runtime.forward.importModal.statusFailed')"
          />
          <code class="import-result__line">{{ result.line }}</code>
          <span class="import-result__message">{{ result.message }}</span>
        </li>
      </ul>
    </section>

    <template #footer="{ close }">
      <UiButton :disabled="loading" @click="close">{{ t('common.actions.close') }}</UiButton>
      <UiButton
        variant="primary"
        :loading="loading"
        :disabled="!data.trim() || !tunnelId"
        data-test="forward-import-submit"
        @click="emit('import')"
      >
        {{ t('runtime.forward.importModal.startImport') }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Import forward rules (relay-panel JSON or the legacy `addr|name|port`
// lines) into one tunnel. The page parses the text and creates each rule
// with forward/create; this dialog only shows the form and the results.
import { computed } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextarea from '@/ui/UiTextarea.vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  tunnels: { type: Array, default: () => [] },
  tunnelId: { type: Number, default: null },
  data: { type: String, default: '' },
  results: { type: Array, default: () => [] },
  successCount: { type: Number, default: 0 },
  loading: { type: Boolean, default: false }
})

const emit = defineEmits(['update:open', 'update:tunnelId', 'update:data', 'import'])
const { t } = useAppI18n()

const tunnelOptions = computed(() => props.tunnels.map(tunnel => ({ value: Number(tunnel.id), label: tunnel.name })))
</script>

<style scoped>
.import-example {
  margin: 0;
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

.mono-area :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.import-results {
  margin-top: var(--space-6);
}

.import-results__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: baseline;
  justify-content: space-between;
}

.import-results__summary {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-variant-numeric: tabular-nums;
}

.import-results__list {
  display: flex;
  flex-direction: column;
  max-height: 280px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  border-radius: var(--radius-md);
  background: var(--fill-1);
  list-style: none;
}

.import-result {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: var(--space-1) var(--space-3);
  align-items: center;
  padding: var(--space-3);
}

.import-result + .import-result {
  border-top: 0.5px solid var(--separator);
}

.import-result__line {
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

.import-result__message {
  grid-column: 2;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
