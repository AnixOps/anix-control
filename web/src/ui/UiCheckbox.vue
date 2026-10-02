<template>
  <div class="ui-checkbox" :class="{ 'is-disabled': disabled }">
    <CheckboxRoot
      :id="checkboxId"
      v-bind="$attrs"
      class="ui-checkbox__box"
      :model-value="modelValue"
      :disabled="disabled"
      :name="name || undefined"
      :value="value"
      :aria-describedby="description ? descId : undefined"
      @update:model-value="emit('update:modelValue', $event)"
    >
      <CheckboxIndicator class="ui-checkbox__indicator">
        <UiIcon :icon="modelValue === 'indeterminate' ? Minus : Check" :size="14" :stroke-width="2.5" />
      </CheckboxIndicator>
    </CheckboxRoot>
    <span v-if="label || $slots.default" class="ui-checkbox__text">
      <label :for="checkboxId" class="ui-checkbox__label"><slot>{{ label }}</slot></label>
      <span v-if="description" :id="descId" class="ui-checkbox__desc">{{ description }}</span>
    </span>
  </div>
</template>

<script setup>
// Checkbox for choices that take effect on submit (Reka Checkbox:
// role="checkbox", aria-checked true/false/mixed, Space toggles).
// modelValue: true, false or 'indeterminate' (table "select all").
import { computed } from 'vue'
import { CheckboxIndicator, CheckboxRoot, useId } from 'reka-ui'
import { Check, Minus } from '@lucide/vue'
import UiIcon from './UiIcon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: [Boolean, String], default: false },
  label: { type: String, default: '' },
  description: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  name: { type: String, default: '' },
  value: { type: String, default: 'on' },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const generated = useId(undefined, 'ui-checkbox')
const checkboxId = computed(() => props.id || generated)
const descId = computed(() => `${checkboxId.value}-desc`)
</script>

<style scoped>
.ui-checkbox {
  display: inline-flex;
  gap: var(--space-3);
  align-items: flex-start;
}

.ui-checkbox__box {
  position: relative;
  display: inline-grid;
  flex: none;
  place-items: center;
  width: 18px;
  height: 18px;
  /* A fixed 18 px box; the label is the touch target. */
  min-height: 0;
  margin-top: 2px;
  padding: 0;
  /* label-3 keeps the boundary at 3:1 or more (WCAG 1.4.11). */
  border: 1px solid var(--label-3);
  border-radius: calc(var(--radius-xs) - 2px);
  background: var(--bg-elevated);
  color: var(--on-accent);
  cursor: pointer;
  transition:
    background-color var(--dur-micro) var(--ease-standard),
    border-color var(--dur-micro) var(--ease-standard);
}

.ui-checkbox__box:hover {
  border-color: var(--label-2);
}

.ui-checkbox__box[data-state='checked'],
.ui-checkbox__box[data-state='indeterminate'] {
  border-color: var(--accent-fill);
  background: var(--accent-fill);
}

.ui-checkbox__box:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

/* Touch: 44 px hit area. */
@media (pointer: coarse) {
  .ui-checkbox__box::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: var(--size-control-lg);
    height: var(--size-control-lg);
    transform: translate(-50%, -50%);
    content: '';
  }
}

.ui-checkbox__indicator {
  display: inline-flex;
}

.ui-checkbox__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.ui-checkbox__label {
  color: var(--label-1);
  cursor: pointer;
}

.ui-checkbox__desc {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.is-disabled {
  opacity: 0.4;
}

.is-disabled .ui-checkbox__box,
.is-disabled .ui-checkbox__label {
  cursor: not-allowed;
}

@media (forced-colors: active) {
  .ui-checkbox__box {
    border-color: ButtonText;
  }
}
</style>
