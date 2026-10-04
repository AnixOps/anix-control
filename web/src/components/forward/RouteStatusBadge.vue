<template>
  <span class="route-status">
    <UiBadge :tone="STATUS_TONES[status] || 'neutral'" :label="t(`forwardV4.status.${status}`)" />
    <span v-if="hint && (status === 'quota' || status === 'expired')" class="route-status__hint">{{ t(`forwardV4.status.${status}Hint`) }}</span>
  </span>
</template>

<script setup>
// One status badge, the worst state (routeModel.routeStatus): enforced
// (Control paused it: quota or expired) before paused before health.
import { UiBadge } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { STATUS_TONES } from './routeModel'

defineProps({
  status: { type: String, required: true },
  hint: { type: Boolean, default: false }
})
const { t } = useAppI18n()
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
