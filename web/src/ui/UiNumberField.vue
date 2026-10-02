<template>
  <UiField
    v-slot="field"
    :class="attrs.class"
    :style="attrs.style"
    :label="label"
    :help="help"
    :error="error"
    :required="required"
    :disabled="disabled"
    :id="baseId"
    :described-by-extra="unit ? `${baseId}-unit` : ''"
  >
    <NumberFieldRoot
      :id="field.id"
      class="ui-number"
      :model-value="modelValue ?? undefined"
      :min="min"
      :max="max"
      :step="step"
      :locale="locale"
      :format-options="formatOptions"
      :disabled="disabled"
      :readonly="readonly"
      :disable-wheel-change="true"
      @update:model-value="onUpdate"
    >
      <InputBox :size="size" :invalid="Boolean(error)" :disabled="disabled" :readonly="readonly">
        <NumberFieldDecrement
          v-if="stepper"
          class="ui-number__step"
          :aria-label="t('ui.number.decrement')"
        >
          <UiIcon :icon="Minus" />
        </NumberFieldDecrement>
        <NumberFieldInput
          v-bind="inputAttrs"
          :placeholder="placeholder || undefined"
          :required="required || undefined"
          :aria-invalid="error ? 'true' : undefined"
          :aria-describedby="field.describedBy"
          :aria-roledescription="undefined"
          :class="{ 'is-centered': stepper }"
        />
        <span v-if="unit" :id="`${baseId}-unit`" class="ui-box__affix ui-box__affix--end">{{ unit }}</span>
        <NumberFieldIncrement
          v-if="stepper"
          class="ui-number__step"
          :aria-label="t('ui.number.increment')"
        >
          <UiIcon :icon="Plus" />
        </NumberFieldIncrement>
      </InputBox>
    </NumberFieldRoot>
  </UiField>
</template>

<script setup>
// Numeric input (Reka NumberField: role="spinbutton", ↑/↓ step, PageUp/Down
// step ×10, Home/End jump to min/max) with a unit suffix that is read as part
// of the description. No digit grouping by default, so ports read 30001.
// The model is a number, or null when the field is empty.
import { computed, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { NumberFieldDecrement, NumberFieldIncrement, NumberFieldInput, NumberFieldRoot, useId } from 'reka-ui'
import { Minus, Plus } from '@lucide/vue'
import UiField from './UiField.vue'
import UiIcon from './UiIcon.vue'
import InputBox from './internal/InputBox.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: Number, default: null },
  label: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  unit: { type: String, default: '' },
  min: { type: Number, default: undefined },
  max: { type: Number, default: undefined },
  step: { type: Number, default: 1 },
  formatOptions: { type: Object, default: () => ({ useGrouping: false }) },
  placeholder: { type: String, default: '' },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
  // Show − and + buttons (pointer users); keyboard users step with arrows.
  stepper: { type: Boolean, default: false },
  size: { type: String, default: 'lg' },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const attrs = useAttrs()
const { t, locale } = useI18n()
const fallbackId = useId(undefined, 'ui-number')
const baseId = computed(() => props.id || fallbackId)

const inputAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs
  return rest
})

function onUpdate(value) {
  emit('update:modelValue', value === undefined || Number.isNaN(value) ? null : value)
}
</script>

<style scoped>
.ui-number {
  width: 100%;
}

.ui-number :deep(input.is-centered) {
  text-align: center;
}

.ui-number__step {
  display: grid;
  flex: none;
  place-items: center;
  align-self: stretch;
  width: var(--size-control-md);
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--label-2);
  cursor: pointer;
  transition: color var(--dur-micro) var(--ease-standard);
}

.ui-number__step:hover:not(:disabled) {
  color: var(--label-1);
}

.ui-number__step:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
