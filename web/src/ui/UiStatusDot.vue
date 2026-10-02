<template>
  <span class="ui-status" :class="`ui-status--${resolvedTone}`">
    <span class="ui-status__dot" aria-hidden="true" />
    <span class="ui-status__text"><slot>{{ text }}</slot></span>
  </span>
</template>

<script setup>
// Status as a dot and a word, for table cells and list rows where a badge is
// too heavy. Same map as UiBadge; the word is always rendered.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { statusLabelKey, statusTone } from './status'

const props = defineProps({
  status: { type: String, default: '' },
  tone: { type: String, default: '' },
  label: { type: String, default: '' }
})

const { t } = useI18n()
const resolvedTone = computed(() => props.tone || (props.status ? statusTone(props.status) : 'neutral'))
const text = computed(() => props.label || (statusLabelKey(props.status) ? t(statusLabelKey(props.status)) : props.status))
</script>

<style scoped>
.ui-status {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
  color: var(--label-1);
  white-space: nowrap;
}

.ui-status__dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--label-3);
}

.ui-status--success .ui-status__dot {
  background: var(--success);
}

.ui-status--warning .ui-status__dot {
  background: var(--warning);
}

.ui-status--danger .ui-status__dot {
  background: var(--danger);
}

.ui-status--info .ui-status__dot {
  background: var(--accent);
}

@media (forced-colors: active) {
  .ui-status__dot {
    background: CanvasText;
  }
}
</style>
