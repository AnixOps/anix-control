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
    :id="id"
  >
    <InputBox multiline :invalid="Boolean(error)" :disabled="disabled" :readonly="readonly">
      <textarea
        :id="field.id"
        ref="textareaRef"
        v-bind="textareaAttrs"
        :value="modelValue ?? ''"
        :rows="rows"
        :placeholder="placeholder || undefined"
        :required="required || undefined"
        :disabled="disabled || undefined"
        :readonly="readonly || undefined"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="field.describedBy"
        @input="emit('update:modelValue', $event.target.value)"
      />
    </InputBox>
  </UiField>
</template>

<script setup>
// Multi-line text with a top label, help and error. Resizes vertically.
import { computed, ref, useAttrs } from 'vue'
import UiField from './UiField.vue'
import InputBox from './internal/InputBox.vue'

defineOptions({ inheritAttrs: false })

defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  rows: { type: Number, default: 4 },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const attrs = useAttrs()
const textareaRef = ref(null)

const textareaAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs
  return rest
})

defineExpose({ focus: () => textareaRef.value?.focus() })
</script>
