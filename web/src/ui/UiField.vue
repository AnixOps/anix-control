<template>
  <div class="ui-field" :class="{ 'is-invalid': Boolean(error), 'is-disabled': disabled }">
    <component
      :is="labelTag"
      v-if="label || $slots.label"
      :id="labelId"
      class="ui-field__label"
      :for="labelTag === 'label' ? controlId : undefined"
    >
      <slot name="label">{{ label }}</slot>
      <span v-if="required" class="ui-field__required" aria-hidden="true">*</span>
    </component>
    <slot
      :id="controlId"
      :labelId="labelId"
      :describedBy="describedBy"
      :invalid="Boolean(error)"
    />
    <p v-if="help" :id="helpId" class="ui-field__help">{{ help }}</p>
    <!-- Always present so a new error is announced; empty when valid. -->
    <div :id="errorId" class="ui-field__error" aria-live="polite">
      <template v-if="error">
        <UiIcon :icon="CircleAlert" :size="16" />
        <span>{{ error }}</span>
      </template>
    </div>
  </div>
</template>

<script setup>
// Form field frame: top label (decision D5), help text and error text tied to
// the control with aria-describedby, required marker. The default slot gets
// { id, labelId, describedBy, invalid } to put on the control.
// Text controls use it through UiTextField & co.; use it directly to wrap a
// custom control.
import { computed } from 'vue'
import { useId } from 'reka-ui'
import { CircleAlert } from '@lucide/vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  label: { type: String, default: '' },
  help: { type: String, default: '' },
  error: { type: String, default: '' },
  required: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  id: { type: String, default: '' },
  // 'label' for native controls; 'span' when the control is labelled with
  // aria-labelledby (custom widgets such as a Reka Select trigger or group).
  labelTag: { type: String, default: 'label' },
  // Extra ids to describe the control with (a unit suffix, for example).
  describedByExtra: { type: String, default: '' }
})

const generated = useId(undefined, 'ui-field')
const controlId = computed(() => props.id || generated)
const labelId = computed(() => `${controlId.value}-label`)
const helpId = computed(() => `${controlId.value}-help`)
const errorId = computed(() => `${controlId.value}-error`)

// Error first: when the field is invalid that is what the reader needs.
const describedBy = computed(() => [
  props.error ? errorId.value : '',
  props.describedByExtra,
  props.help ? helpId.value : ''
].filter(Boolean).join(' ') || undefined)
</script>

<style scoped>
.ui-field {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.ui-field__label {
  display: inline-flex;
  gap: var(--space-0-5);
  align-items: baseline;
  margin-bottom: var(--space-2);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  line-height: var(--type-callout-line);
}

.ui-field__required {
  color: var(--danger);
}

.ui-field__help {
  margin-top: var(--space-1);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.ui-field__error {
  display: flex;
  gap: var(--space-1);
  align-items: flex-start;
  color: var(--danger);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.ui-field__error:not(:empty) {
  margin-top: var(--space-1);
}

.ui-field__error :deep(.ui-icon) {
  margin-top: 1px;
}

.is-disabled .ui-field__label {
  color: var(--label-2);
}
</style>
