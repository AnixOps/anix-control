<template>
  <div class="ui-filter-chips" role="group" :aria-label="label">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="ui-filter-chips__chip"
      :class="{ 'is-active': isActive(option.value) }"
      :aria-pressed="String(isActive(option.value))"
      :disabled="option.disabled || undefined"
      :data-filter="option.value || 'all'"
      @click="toggle(option.value)"
    >
      <UiIcon v-if="option.icon" :icon="option.icon" :size="14" />
      <span>{{ option.label }}</span>
      <span v-if="option.count !== undefined && option.count !== null" class="ui-filter-chips__count tabular-nums">{{ option.count }}</span>
    </button>
  </div>
</template>

<script setup>
// Filter chips of a list toolbar (已封禁 / 已到期 / 流量用尽). A named group
// of toggle buttons (aria-pressed). Single choice by default: modelValue is
// the value ('' for an "all" chip), and pressing the active chip again
// clears it. `multiple` makes modelValue an array.
import UiIcon from './UiIcon.vue'

const props = defineProps({
  modelValue: { type: [String, Array], default: '' },
  // [{ value, label, count?, icon?, disabled? }]
  options: { type: Array, required: true },
  label: { type: String, required: true },
  multiple: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue'])

function isActive(value) {
  if (props.multiple) return Array.isArray(props.modelValue) && props.modelValue.includes(value)
  return props.modelValue === value
}

function toggle(value) {
  if (props.multiple) {
    const current = Array.isArray(props.modelValue) ? props.modelValue : []
    emit('update:modelValue', current.includes(value) ? current.filter(item => item !== value) : [...current, value])
    return
  }
  emit('update:modelValue', props.modelValue === value ? '' : value)
}
</script>

<style scoped>
.ui-filter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.ui-filter-chips__chip {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  height: var(--size-control-sm);
  min-height: 0;
  padding: 0 var(--space-3);
  border: 1px solid var(--separator-strong);
  border-radius: var(--radius-pill);
  background: var(--bg-elevated);
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-callout-size);
  white-space: nowrap;
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard), border-color var(--dur-micro) var(--ease-standard);
}

.ui-filter-chips__chip:hover {
  background: var(--fill-1);
}

.ui-filter-chips__chip:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.ui-filter-chips__chip.is-active {
  border-color: transparent;
  background: var(--accent-fill);
  color: var(--on-accent);
}

.ui-filter-chips__chip:disabled {
  color: var(--label-3);
  cursor: not-allowed;
}

.ui-filter-chips__count {
  color: inherit;
  font-weight: var(--weight-semibold);
}

@media (pointer: coarse) {
  .ui-filter-chips__chip {
    height: 44px;
  }
}

@media (forced-colors: active) {
  .ui-filter-chips__chip.is-active {
    border-color: Highlight;
    text-decoration: underline;
  }
}
</style>
