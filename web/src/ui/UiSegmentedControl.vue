<template>
  <ToggleGroupRoot
    type="single"
    class="ui-segmented"
    :class="[`ui-segmented--${size}`, { 'is-block': block }]"
    :model-value="modelValue"
    :disabled="disabled"
    :aria-label="ariaLabel || undefined"
    loop
    @update:model-value="onUpdate"
  >
    <ToggleGroupItem
      v-for="option in normalizedOptions"
      :key="String(option.value)"
      class="ui-segmented__item"
      :value="option.value"
      :disabled="option.disabled"
    >
      <UiIcon v-if="option.icon" :icon="option.icon" />
      <span>{{ option.label }}</span>
    </ToggleGroupItem>
  </ToggleGroupRoot>
</template>

<script setup>
// Pick one of 2–5 mutually exclusive values that filter or change the view in
// place: time ranges (1 h / 24 h / 7 d), list vs. grouped view. Reka
// ToggleGroup: role="group" named by ariaLabel, each segment a button with
// aria-pressed; Tab enters at the selected segment, ←/→ move, Space/Enter
// select. There is always one selection (clicking the active segment keeps it).
// To switch between content panels (rules / tunnels / limits) use UiTabs
// with variant="segmented" instead: that gives tab/tabpanel semantics.
import { computed } from 'vue'
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  modelValue: { type: [String, Number], default: undefined },
  options: { type: Array, default: () => [] },
  ariaLabel: { type: String, required: true },
  size: { type: String, default: 'md', validator: value => ['sm', 'md'].includes(value) },
  disabled: { type: Boolean, default: false },
  block: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue'])

const normalizedOptions = computed(() => props.options.map(option => (
  typeof option === 'object' && option !== null
    ? { ...option, label: String(option.label ?? option.value) }
    : { value: option, label: String(option) }
)))

function onUpdate(value) {
  // Single selection is mandatory: ignore the "unpress" of the active item.
  if (value === undefined || value === null || value === '') return
  emit('update:modelValue', value)
}
</script>

<style scoped>
.ui-segmented {
  --ui-seg-height: 30px;

  display: inline-flex;
  gap: var(--space-0-5);
  max-width: 100%;
  padding: var(--space-0-5);
  overflow-x: auto;
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  scrollbar-width: none;
}

.ui-segmented::-webkit-scrollbar {
  display: none;
}

.ui-segmented.is-block {
  display: flex;
  width: 100%;
}

.ui-segmented.is-block .ui-segmented__item {
  flex: 1;
}

.ui-segmented__item {
  display: inline-flex;
  flex: none;
  gap: var(--space-1);
  align-items: center;
  justify-content: center;
  height: var(--ui-seg-height);
  padding: 0 var(--space-4);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--label-2);
  font-family: inherit;
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  white-space: nowrap;
  cursor: pointer;
  transition:
    background-color var(--dur-toggle) var(--ease-standard),
    color var(--dur-toggle) var(--ease-standard),
    box-shadow var(--dur-toggle) var(--ease-standard);
}

.ui-segmented__item:hover:not([data-disabled]) {
  color: var(--label-1);
}

.ui-segmented__item[data-state='on'] {
  background: var(--bg-elevated);
  color: var(--label-1);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.ui-segmented__item:focus-visible {
  outline: var(--focus-ring);
  outline-offset: 0;
}

.ui-segmented__item[data-disabled] {
  opacity: 0.4;
  cursor: not-allowed;
}

.ui-segmented--sm {
  --ui-seg-height: 26px;
}

.ui-segmented--sm .ui-segmented__item {
  padding: 0 var(--space-3);
}

@media (pointer: coarse) {
  .ui-segmented {
    --ui-seg-height: 40px;
  }
}

@media (forced-colors: active) {
  .ui-segmented__item[data-state='on'] {
    outline: 2px solid transparent;
    text-decoration: underline;
  }
}
</style>
