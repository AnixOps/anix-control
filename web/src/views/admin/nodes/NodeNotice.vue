<template>
  <p class="node-notice" :class="`node-notice--${tone}`">
    <UiIcon :icon="icon" :size="16" />
    <span class="node-notice__body"><slot /></span>
    <span v-if="$slots.actions" class="node-notice__actions"><slot name="actions" /></span>
  </p>
</template>

<script setup>
// A one-line notice above a node list or section (over quota, runtime
// error, nodes waiting for approval). The icon and the words carry the
// meaning, not the colour. `actions` holds a button for the notice's next step.
import { computed } from 'vue'
import { CircleAlert, Info, TriangleAlert } from '@lucide/vue'
import UiIcon from '@/ui/UiIcon.vue'

const props = defineProps({
  tone: { type: String, default: 'warning', validator: value => ['warning', 'danger', 'info'].includes(value) }
})
const icon = computed(() => ({ danger: CircleAlert, info: Info }[props.tone] || TriangleAlert))
</script>

<style scoped>
.node-notice {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.node-notice :deep(svg) {
  flex: none;
  margin-top: 2px;
}

.node-notice--warning {
  background: var(--warning-soft);
}

.node-notice--warning :deep(svg) {
  color: color-mix(in srgb, var(--warning) 70%, var(--label-1));
}

.node-notice__body {
  flex: 1 1 auto;
  min-width: 0;
}

.node-notice__actions {
  flex: none;
  display: inline-flex;
  gap: var(--space-2);
  margin: calc(-1 * var(--space-1)) 0;
}

.node-notice--info {
  background: var(--accent-soft);
}

.node-notice--info :deep(svg) {
  color: var(--accent);
}

.node-notice--danger {
  background: var(--danger-soft);
}

.node-notice--danger :deep(svg) {
  color: var(--danger);
}
</style>
