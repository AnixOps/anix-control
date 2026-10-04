<template>
  <span class="route-status">
    <UiBadge :tone="status.tone" :label="status.label" />
    <span v-if="hint && status.key === 'quota'" class="route-status__hint">提高配额后自动恢复</span>
    <span v-else-if="hint && status.key === 'expired'" class="route-status__hint">延长到期时间后自动恢复</span>
  </span>
</template>

<script setup>
// One status badge, the worst state: enforced (Control paused it: quota or
// expired) before paused (an operator paused it) before health.
import { computed } from 'vue'
import { UiBadge } from '@/ui'
import { routeStatus } from '../mockData'

const props = defineProps({
  item: { type: Object, required: true },
  hint: { type: Boolean, default: false }
})
const status = computed(() => routeStatus(props.item))
</script>

<style scoped>
.route-status {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.route-status__hint {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}
</style>
