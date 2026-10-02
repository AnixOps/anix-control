<template>
  <UiTextField
    ref="fieldRef"
    v-bind="$attrs"
    :model-value="modelValue"
    :type="revealed ? 'text' : 'password'"
    :label="label"
    :help="help"
    :error="error"
    :required="required"
    :disabled="disabled"
    :readonly="readonly"
    :size="size"
    :id="id"
    :autocomplete="autocomplete"
    spellcheck="false"
    autocapitalize="off"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <template #end="{ id: inputId }">
      <UiIconButton
        :label="t('ui.password.show')"
        :icon="revealed ? EyeOff : Eye"
        :pressed="revealed"
        :disabled="disabled"
        :aria-controls="inputId"
        :tooltip="false"
        size="sm"
        variant="plain"
        @click="toggle"
      />
    </template>
  </UiTextField>
</template>

<script setup>
// Password input with a reveal toggle. The toggle is a pressed/unpressed
// button with a constant name ("显示密码", aria-pressed), so a screen reader
// hears the state; focus stays on the toggle.
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Eye, EyeOff } from '@lucide/vue'
import UiTextField from './UiTextField.vue'
import UiIconButton from './UiIconButton.vue'

defineOptions({ inheritAttrs: false })

defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
  size: { type: String, default: 'lg' },
  id: { type: String, default: '' },
  // 'current-password' on sign-in, 'new-password' when setting one.
  autocomplete: { type: String, default: 'current-password' }
})

const emit = defineEmits(['update:modelValue'])
const { t } = useI18n()
const revealed = ref(false)
const fieldRef = ref(null)

function toggle() {
  revealed.value = !revealed.value
}

defineExpose({ focus: () => fieldRef.value?.focus() })
</script>
