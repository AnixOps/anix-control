<template>
  <p class="node-notice" :class="`node-notice--${tone}`">
    <UiIcon :icon="tone === 'danger' ? CircleAlert : TriangleAlert" :size="16" />
    <span><slot /></span>
  </p>
</template>

<script setup>
// A one-line notice above a node list or section (over quota, runtime
// error). The icon and the words carry the meaning, not the colour.
import { CircleAlert, TriangleAlert } from '@lucide/vue'
import UiIcon from '@/ui/UiIcon.vue'

defineProps({
  tone: { type: String, default: 'warning', validator: value => ['warning', 'danger'].includes(value) }
})
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

.node-notice--danger {
  background: var(--danger-soft);
}

.node-notice--danger :deep(svg) {
  color: var(--danger);
}
</style>
