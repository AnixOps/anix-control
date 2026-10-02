<template>
  <span class="ui-badge" :class="[`ui-badge--${resolvedTone}`, { 'has-dot': dot }]">
    <span v-if="dot" class="ui-badge__dot" aria-hidden="true" />
    <slot>{{ text }}</slot>
  </span>
</template>

<script setup>
// Small status label: a coloured pill with a dot and a word (caption type).
// Either a `status` from the shared map (online, offline, disabled, pending,
// error; the word comes from i18n) or a free `tone` with your own text.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { statusLabelKey, statusTone } from './status'

const props = defineProps({
  status: { type: String, default: '' },
  tone: { type: String, default: '', validator: value => ['', 'success', 'warning', 'danger', 'info', 'neutral'].includes(value) },
  label: { type: String, default: '' },
  dot: { type: Boolean, default: true }
})

const { t } = useI18n()
const resolvedTone = computed(() => props.tone || (props.status ? statusTone(props.status) : 'neutral'))
const text = computed(() => props.label || (statusLabelKey(props.status) ? t(statusLabelKey(props.status)) : props.status))
</script>

<style scoped>
.ui-badge {
  display: inline-flex;
  flex: none;
  gap: var(--space-1);
  align-items: center;
  height: 22px;
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--type-caption-weight);
  line-height: 1;
  white-space: nowrap;
}

.ui-badge__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.ui-badge--success {
  background: var(--success-soft);
  color: var(--success);
}

.ui-badge--warning {
  background: var(--warning-soft);
  color: var(--warning);
}

.ui-badge--danger {
  background: var(--danger-soft);
  color: var(--danger);
}

.ui-badge--info {
  background: var(--accent-soft);
  color: var(--accent);
}

@media (forced-colors: active) {
  .ui-badge {
    border: 1px solid CanvasText;
  }
}
</style>
