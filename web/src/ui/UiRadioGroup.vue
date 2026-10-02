<template>
  <div class="ui-radio-group" :class="{ 'is-invalid': Boolean(error) }">
    <span v-if="label" :id="labelId" class="ui-radio-group__label">
      {{ label }}<span v-if="required" class="ui-radio-group__required" aria-hidden="true">*</span>
    </span>
    <RadioGroupRoot
      class="ui-radio-group__items"
      :class="`is-${orientation}`"
      :model-value="modelValue ?? undefined"
      :orientation="orientation"
      :disabled="disabled"
      :required="required"
      :name="name || undefined"
      :aria-labelledby="label ? labelId : undefined"
      :aria-describedby="describedBy"
      v-bind="$attrs"
      @update:model-value="emit('update:modelValue', $event)"
    >
      <div v-for="(option, index) in normalizedOptions" :key="String(option.value)" class="ui-radio" :class="{ 'is-disabled': option.disabled || disabled }">
        <RadioGroupItem
          :id="`${groupId}-${index}`"
          class="ui-radio__control"
          :value="option.value"
          :disabled="option.disabled"
          :aria-describedby="option.description ? `${groupId}-${index}-desc` : undefined"
        >
          <RadioGroupIndicator class="ui-radio__dot" />
        </RadioGroupItem>
        <span class="ui-radio__text">
          <label :for="`${groupId}-${index}`" class="ui-radio__label">{{ option.label }}</label>
          <span v-if="option.description" :id="`${groupId}-${index}-desc`" class="ui-radio__desc">{{ option.description }}</span>
        </span>
      </div>
    </RadioGroupRoot>
    <p v-if="help" :id="`${groupId}-help`" class="ui-radio-group__help">{{ help }}</p>
    <div :id="`${groupId}-error`" class="ui-radio-group__error" aria-live="polite">
      <template v-if="error">{{ error }}</template>
    </div>
  </div>
</template>

<script setup>
// One choice among a few visible options (Reka RadioGroup: role="radiogroup"
// named by the label; Tab enters the group at the checked option, arrow keys
// move and select). options: [{ value, label, description?, disabled? }].
import { computed } from 'vue'
import { RadioGroupIndicator, RadioGroupItem, RadioGroupRoot, useId } from 'reka-ui'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: [String, Number], default: undefined },
  options: { type: Array, default: () => [] },
  label: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  orientation: { type: String, default: 'vertical', validator: value => ['vertical', 'horizontal'].includes(value) },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  name: { type: String, default: '' },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const generated = useId(undefined, 'ui-radio')
const groupId = computed(() => props.id || generated)
const labelId = computed(() => `${groupId.value}-label`)

const describedBy = computed(() => [
  props.error ? `${groupId.value}-error` : '',
  props.help ? `${groupId.value}-help` : ''
].filter(Boolean).join(' ') || undefined)

const normalizedOptions = computed(() => props.options.map(option => (
  typeof option === 'object' && option !== null
    ? { ...option, label: String(option.label ?? option.value) }
    : { value: option, label: String(option) }
)))
</script>

<style scoped>
.ui-radio-group {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.ui-radio-group__label {
  margin-bottom: var(--space-2);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
}

.ui-radio-group__required {
  margin-left: var(--space-0-5);
  color: var(--danger);
}

.ui-radio-group__items {
  display: flex;
  gap: var(--space-3);
}

.ui-radio-group__items.is-vertical {
  flex-direction: column;
}

.ui-radio-group__items.is-horizontal {
  flex-wrap: wrap;
  gap: var(--space-3) var(--space-6);
}

.ui-radio {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
}

.ui-radio__control {
  position: relative;
  display: grid;
  flex: none;
  place-items: center;
  width: 18px;
  height: 18px;
  /* The legacy global button rule (40 / 44 px) must not stretch it. */
  min-height: 0;
  margin-top: 2px;
  padding: 0;
  /* label-3 keeps the boundary at 3:1 or more (WCAG 1.4.11). */
  border: 1px solid var(--label-3);
  border-radius: 50%;
  background: var(--bg-elevated);
  cursor: pointer;
  transition:
    background-color var(--dur-micro) var(--ease-standard),
    border-color var(--dur-micro) var(--ease-standard);
}

.ui-radio__control:hover {
  border-color: var(--label-2);
}

.ui-radio__control[data-state='checked'] {
  border-color: var(--accent-fill);
  background: var(--accent-fill);
}

.ui-radio__control:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

@media (pointer: coarse) {
  .ui-radio__control::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: var(--size-control-lg);
    height: var(--size-control-lg);
    transform: translate(-50%, -50%);
    content: '';
  }
}

.ui-radio__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--on-accent);
}

.ui-radio__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.ui-radio__label {
  color: var(--label-1);
  cursor: pointer;
}

.ui-radio__desc {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-radio.is-disabled {
  opacity: 0.4;
}

.ui-radio.is-disabled .ui-radio__control,
.ui-radio.is-disabled .ui-radio__label {
  cursor: not-allowed;
}

.ui-radio-group__help {
  margin-top: var(--space-2);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-radio-group__error {
  color: var(--danger);
  font-size: var(--type-callout-size);
}

.ui-radio-group__error:not(:empty) {
  margin-top: var(--space-2);
}

@media (forced-colors: active) {
  .ui-radio__control {
    border-color: ButtonText;
  }

  .ui-radio__dot {
    background: ButtonText;
  }
}
</style>
