<template>
  <UiField
    :class="rootClass"
    :style="rootStyle"
    :label="label"
    :help="help"
    :error="error"
    :required="required"
    :disabled="disabled"
    :id="baseId"
    :described-by-extra="unitIds"
  >
    <template v-if="$slots.label" #label>
      <slot name="label" />
    </template>
    <template #default="field">
      <InputBox :size="size" :invalid="Boolean(error)" :disabled="disabled" :readonly="readonly">
        <span v-if="prefix" :id="`${field.id}-prefix`" class="ui-box__affix ui-box__affix--start">{{ prefix }}</span>
        <slot name="start" />
        <input
          :id="field.id"
          ref="inputRef"
          v-bind="inputAttrs"
          :value="modelValue ?? ''"
          :type="type"
          :placeholder="placeholder || undefined"
          :required="required || undefined"
          :disabled="disabled || undefined"
          :readonly="readonly || undefined"
          :aria-invalid="error ? 'true' : undefined"
          :aria-describedby="field.describedBy"
          @input="onInput"
        >
        <span v-if="suffix" :id="`${field.id}-suffix`" class="ui-box__affix ui-box__affix--end">{{ suffix }}</span>
        <span v-if="$slots.end" class="ui-box__end">
          <slot name="end" :id="field.id" />
        </span>
      </InputBox>
    </template>
  </UiField>
</template>

<script setup>
// Single-line text input with a top label, help, error and optional
// prefix/suffix (units such as GB or Mbps; the unit is part of the
// description, so a screen reader hears "限速 … Mbps").
// class and style go to the field wrapper; every other attribute
// (autocomplete, inputmode, maxlength, name, @blur…) goes to the <input>.
import { computed, ref, useAttrs } from 'vue'
import { useId } from 'reka-ui'
import UiField from './UiField.vue'
import InputBox from './internal/InputBox.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  label: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  type: { type: String, default: 'text' },
  placeholder: { type: String, default: '' },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
  size: { type: String, default: 'lg', validator: value => ['sm', 'md', 'lg'].includes(value) },
  prefix: { type: String, default: '' },
  suffix: { type: String, default: '' },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const attrs = useAttrs()
const inputRef = ref(null)
const fallbackId = useId(undefined, 'ui-text')
const baseId = computed(() => props.id || fallbackId)

const rootClass = computed(() => attrs.class)
const rootStyle = computed(() => attrs.style)
const inputAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs
  return rest
})

const unitIds = computed(() => [
  props.prefix ? `${baseId.value}-prefix` : '',
  props.suffix ? `${baseId.value}-suffix` : ''
].filter(Boolean).join(' '))

function onInput(event) {
  emit('update:modelValue', event.target.value)
}

defineExpose({
  focus: () => inputRef.value?.focus(),
  select: () => inputRef.value?.select(),
  input: inputRef
})
</script>
