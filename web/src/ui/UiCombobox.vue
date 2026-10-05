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
    <ComboboxRoot
      v-model:open="open"
      :model-value="modelValue ?? undefined"
      :disabled="disabled"
      :required="required"
      :name="name || undefined"
      :reset-search-term-on-blur="true"
      :open-on-click="true"
      class="ui-combobox"
      @update:model-value="onSelect"
    >
      <ComboboxAnchor as-child>
        <InputBox :size="size" :invalid="Boolean(error)" :disabled="disabled">
          <UiIcon :icon="Search" class="ui-combobox__search" />
          <ComboboxInput
            :id="field.id"
            :display-value="displayValue"
            :placeholder="placeholder || t('ui.select.search')"
            :aria-invalid="error ? 'true' : undefined"
            :aria-describedby="field.describedBy"
            :aria-label="label ? undefined : ariaLabel || undefined"
            autocomplete="off"
          />
          <span class="ui-box__end">
            <ComboboxTrigger class="ui-combobox__trigger" tabindex="-1" :aria-label="label || ariaLabel">
              <UiIcon :icon="ChevronDown" />
            </ComboboxTrigger>
          </span>
        </InputBox>
      </ComboboxAnchor>
      <ComboboxPortal :to="layer">
        <ComboboxContent class="ui-listbox" position="popper" :side-offset="6" :collision-padding="16">
          <ComboboxViewport class="ui-listbox__viewport">
            <ComboboxEmpty class="ui-listbox__empty">{{ emptyText || t('ui.select.empty') }}</ComboboxEmpty>
            <ComboboxItem
              v-for="option in normalizedOptions"
              :key="String(option.value)"
              :value="option.value"
              :text-value="option.label"
              :disabled="option.disabled"
              class="ui-listbox__item"
            >
              <span class="ui-listbox__text">
                <span>{{ option.label }}</span>
                <span v-if="option.description" class="ui-listbox__desc">{{ option.description }}</span>
              </span>
              <ComboboxItemIndicator class="ui-listbox__check">
                <UiIcon :icon="Check" />
              </ComboboxItemIndicator>
            </ComboboxItem>
          </ComboboxViewport>
        </ComboboxContent>
      </ComboboxPortal>
    </ComboboxRoot>
  </UiField>
</template>

<script setup>
// Searchable single choice for long lists (users, nodes, templates). Reka
// Combobox: an input with role="combobox" filters the listbox as you type;
// ↓/↑ move, Enter selects, Esc closes, and an unmatched search is reset on
// blur so the field always shows the chosen option. The open list sits in its
// own labelled region (useMenuLayer), not loose on <body>.
// options: [{ value, label, description?, disabled? }] or plain strings.
import { computed, ref, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ComboboxAnchor, ComboboxContent, ComboboxEmpty, ComboboxInput, ComboboxItem, ComboboxItemIndicator,
  ComboboxPortal, ComboboxRoot, ComboboxTrigger, ComboboxViewport
} from 'reka-ui'
import { Check, ChevronDown, Search } from '@lucide/vue'
import UiField from './UiField.vue'
import UiIcon from './UiIcon.vue'
import InputBox from './internal/InputBox.vue'
import './internal/listbox.css'
import { useMenuLayer } from './composables/useMenuLayer'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: [String, Number], default: undefined },
  options: { type: Array, default: () => [] },
  label: { type: String, default: '' },
  ariaLabel: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  emptyText: { type: String, default: '' },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  name: { type: String, default: '' },
  size: { type: String, default: 'lg' },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const attrs = useAttrs()
const { t } = useI18n()
const open = ref(false)
const layer = useMenuLayer(open, () => {
  const name = props.label || props.ariaLabel
  return name ? t('ui.select.optionsOf', { name }) : t('ui.select.options')
})

const normalizedOptions = computed(() => props.options.map(option => (
  typeof option === 'object' && option !== null
    ? { ...option, label: String(option.label ?? option.value) }
    : { value: option, label: String(option) }
)))

function displayValue(value) {
  const match = normalizedOptions.value.find(option => option.value === value)
  return match ? match.label : ''
}

function onSelect(value) {
  emit('update:modelValue', value)
}
</script>

<style scoped>
.ui-combobox {
  width: 100%;
}

.ui-combobox__search {
  margin-left: var(--space-3);
  color: var(--label-3);
}

.ui-combobox :deep(input) {
  padding-left: var(--space-2);
}

.ui-combobox__trigger {
  display: grid;
  place-items: center;
  width: var(--size-control-sm);
  height: var(--size-control-sm);
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--label-2);
  cursor: pointer;
}

.ui-combobox__trigger:hover {
  background: var(--fill-1);
}

.ui-combobox__trigger[data-state='open'] :deep(.ui-icon) {
  transform: rotate(180deg);
}
</style>
