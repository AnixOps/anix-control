<template>
  <figure class="node-code">
    <figcaption class="node-code__head">
      <span :id="titleId" class="node-code__title">{{ title }}</span>
      <UiButton size="sm" :icon="Copy" :disabled="disabled" :aria-describedby="titleId" @click="emit('copy', code)">{{ copyLabel || t('common.actions.copy') }}</UiButton>
    </figcaption>
    <pre class="node-code__body" tabindex="0" :aria-labelledby="titleId"><code>{{ code }}</code></pre>
  </figure>
</template>

<script setup>
// A generated file or command (config.json, inventory.ini, group vars,
// playbook commands): a title, a copy button and the text, read-only and
// scrollable with the keyboard.
import { Copy } from '@lucide/vue'
import { useId } from 'reka-ui'
import UiButton from '@/ui/UiButton.vue'
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  title: { type: String, required: true },
  code: { type: String, default: '' },
  copyLabel: { type: String, default: '' },
  disabled: { type: Boolean, default: false }
})
const emit = defineEmits(['copy'])
const { t } = useAppI18n()
const titleId = useId(undefined, 'node-code')
</script>

<style scoped>
.node-code {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
  margin: 0;
}

.node-code__head {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
}

.node-code__title {
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
}

.node-code__body {
  max-height: 320px;
  margin: 0;
  padding: var(--space-3) var(--space-4);
  overflow: auto;
  border: 1px solid var(--separator);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
  color: var(--label-1);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  line-height: 1.6;
  white-space: pre;
}

.node-code__body:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}
</style>
