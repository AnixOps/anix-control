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
    <SelectRoot
      :model-value="modelValue ?? undefined"
      :disabled="disabled"
      :required="required"
      :name="name || undefined"
      @update:model-value="emit('update:modelValue', $event)"
    >
      <SelectTrigger as-child>
        <InputBox
          :id="field.id"
          as="button"
          type="button"
          class="ui-select__trigger"
          :size="size"
          :invalid="Boolean(error)"
          :disabled="disabled"
          :aria-invalid="error ? 'true' : undefined"
          :aria-describedby="field.describedBy"
          :aria-label="label ? undefined : ariaLabel || undefined"
        >
          <SelectValue class="ui-select__value" :placeholder="placeholder || t('ui.select.placeholder')" />
          <UiIcon :icon="ChevronDown" class="ui-select__chevron" />
        </InputBox>
      </SelectTrigger>
      <SelectPortal>
        <SelectContent class="ui-listbox" position="popper" :side-offset="6" :collision-padding="16">
          <SelectViewport class="ui-listbox__viewport">
            <SelectItem
              v-for="option in normalizedOptions"
              :key="String(option.value)"
              :value="option.value"
              :disabled="option.disabled"
              :text-value="option.label"
              class="ui-listbox__item"
            >
              <span class="ui-listbox__text">
                <SelectItemText>{{ option.label }}</SelectItemText>
                <span v-if="option.description" class="ui-listbox__desc">{{ option.description }}</span>
              </span>
              <SelectItemIndicator class="ui-listbox__check">
                <UiIcon :icon="Check" />
              </SelectItemIndicator>
            </SelectItem>
          </SelectViewport>
        </SelectContent>
      </SelectPortal>
    </SelectRoot>
  </UiField>
</template>

<script setup>
// Single choice from a short list (Reka Select: button with
// role="combobox" and a listbox; Space/Enter/↓ open it, arrows move,
// typing jumps to a match, Esc closes and returns focus).
// options: [{ value, label, description?, disabled? }] or plain strings.
// For long or searchable lists use UiCombobox.
import { computed, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { SelectContent, SelectItem, SelectItemIndicator, SelectItemText, SelectPortal, SelectRoot, SelectTrigger, SelectValue, SelectViewport } from 'reka-ui'
import { Check, ChevronDown } from '@lucide/vue'
import UiField from './UiField.vue'
import UiIcon from './UiIcon.vue'
import InputBox from './internal/InputBox.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: [String, Number, Boolean], default: undefined },
  options: { type: Array, default: () => [] },
  label: { type: String, default: '' },
  // Accessible name when there is no visible label (toolbar filters).
  ariaLabel: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  name: { type: String, default: '' },
  size: { type: String, default: 'lg' },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const attrs = useAttrs()
const { t } = useI18n()

const normalizedOptions = computed(() => props.options.map(option => (
  typeof option === 'object' && option !== null
    ? { ...option, label: String(option.label ?? option.value) }
    : { value: option, label: String(option) }
)))
</script>

<style scoped src="./internal/listbox.css"></style>

<style scoped>
.ui-select__trigger {
  gap: var(--space-2);
  padding: 0 var(--space-3);
  font-family: inherit;
  cursor: pointer;
}

.ui-select__trigger[data-placeholder] .ui-select__value {
  /* Real text, not a native placeholder: it needs 4.5:1 (--label-3 is 3.6:1). */
  color: var(--label-2);
}

.ui-select__value {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ui-select__chevron {
  color: var(--label-2);
  transition: transform var(--dur-toggle) var(--ease-standard);
}

.ui-select__trigger[data-state='open'] .ui-select__chevron {
  transform: rotate(180deg);
}
</style>
