<template>
  <div class="ui-switch" :class="{ 'is-disabled': disabled, 'has-label': hasLabel }">
    <span v-if="hasLabel" class="ui-switch__text">
      <label :id="labelId" :for="switchId" class="ui-switch__label"><slot>{{ label }}</slot></label>
      <span v-if="description" :id="descId" class="ui-switch__desc">{{ description }}</span>
    </span>
    <SwitchRoot
      :id="switchId"
      v-bind="$attrs"
      class="ui-switch__control"
      :model-value="modelValue"
      :disabled="disabled"
      :name="name || undefined"
      :aria-describedby="description ? descId : undefined"
      @update:model-value="emit('update:modelValue', $event)"
    >
      <SwitchThumb class="ui-switch__thumb" />
    </SwitchRoot>
  </div>
</template>

<script setup>
// On/off setting that applies immediately (Reka Switch: role="switch",
// aria-checked, Space/Enter toggle). Label on the left, switch on the right,
// as in a settings row. Without `label`, pass aria-label or aria-labelledby.
// Checked track is --success, as in the reviewed prototype.
import { computed, useSlots } from 'vue'
import { SwitchRoot, SwitchThumb, useId } from 'reka-ui'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  label: { type: String, default: '' },
  description: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  name: { type: String, default: '' },
  id: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue'])
const slots = useSlots()
const generated = useId(undefined, 'ui-switch')
const switchId = computed(() => props.id || generated)
const labelId = computed(() => `${switchId.value}-label`)
const descId = computed(() => `${switchId.value}-desc`)
const hasLabel = computed(() => Boolean(props.label || slots.default))
</script>

<style scoped>
.ui-switch {
  display: inline-flex;
  gap: var(--space-4);
  align-items: center;
}

.ui-switch.has-label {
  display: flex;
  justify-content: space-between;
  width: 100%;
}

.ui-switch__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.ui-switch__label {
  color: var(--label-1);
  cursor: pointer;
}

.ui-switch__desc {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-switch__control {
  position: relative;
  display: inline-flex;
  flex: none;
  align-items: center;
  width: 51px;
  height: 31px;
  padding: 2px;
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--fill-3);
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
  transition: background-color var(--dur-toggle) var(--ease-standard);
}

.ui-switch__control[data-state='checked'] {
  background: var(--success);
}

.ui-switch__control:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.ui-switch__thumb {
  display: block;
  width: 27px;
  height: 27px;
  border-radius: 50%;
  /* White in both themes, like the system switch; --on-accent is the
     token for white on a filled control. */
  background: var(--on-accent);
  box-shadow: 0 2px 6px var(--shadow-color), 0 0 0 0.5px var(--separator);
  transition: transform var(--dur-toggle) var(--ease-emphasized);
}

.ui-switch__thumb[data-state='checked'] {
  transform: translateX(20px);
}

.is-disabled .ui-switch__control,
.is-disabled .ui-switch__label {
  opacity: 0.4;
  cursor: not-allowed;
}

/* Windows high-contrast: system colours for the track and thumb. */
@media (forced-colors: active) {
  .ui-switch__control {
    border: 1px solid ButtonText;
  }

  .ui-switch__thumb {
    background: ButtonText;
  }
}
</style>
