<template>
  <li class="ui-row" :class="{ 'is-action': isAction, 'is-stacked': stacked }">
    <component
      :is="actionTag"
      v-bind="actionAttrs"
      class="ui-row__inner"
    >
      <span v-if="$slots.leading" class="ui-row__leading"><slot name="leading" /></span>
      <span v-if="hasMain" class="ui-row__main">
        <component :is="labelFor ? 'label' : 'span'" :for="labelFor || undefined" class="ui-row__label">
          <slot name="label">{{ label }}</slot>
        </component>
        <span v-if="description || $slots.description" class="ui-row__sub">
          <slot name="description">{{ description }}</slot>
        </span>
      </span>
      <span v-if="value !== '' || $slots.value" class="ui-row__value">
        <slot name="value">{{ value }}</slot>
      </span>
      <span v-if="$slots.default" class="ui-row__control">
        <slot />
      </span>
      <UiIcon v-if="isAction" :icon="ChevronRight" class="ui-row__chevron" />
    </component>
  </li>
</template>

<script setup>
// One row of a UiGroupedList: label (and a sub line) on the left; a value,
// a control (switch, select, button) or a chevron on the right.
// - labelFor: the id of the control in the default slot, so the row label
//   names it (<label for>);
// - href or @click: the whole row is a link / button with a chevron
//   (navigation to a detail page or sheet); do not put controls inside it;
// - stacked: the control goes under the label (wide inputs on phones).
import { computed, useAttrs, useSlots } from 'vue'
import { ChevronRight } from '@lucide/vue'
import UiIcon from './UiIcon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  label: { type: String, default: '' },
  description: { type: String, default: '' },
  value: { type: [String, Number], default: '' },
  labelFor: { type: String, default: '' },
  href: { type: String, default: '' },
  stacked: { type: Boolean, default: false }
})

// `click` is deliberately not a declared emit: a parent @click arrives in
// attrs, which is how the row knows to render as a button.
const attrs = useAttrs()
const slots = useSlots()

const isAction = computed(() => Boolean(props.href) || Boolean(attrs.onClick))
// A stacked row may hold only a control (a field with its own label).
const hasMain = computed(() => Boolean(props.label || props.description || slots.label || slots.description))
const actionTag = computed(() => (props.href ? 'a' : isAction.value ? 'button' : 'div'))
const actionAttrs = computed(() => {
  if (props.href) return { ...attrs, href: props.href }
  if (isAction.value) return { ...attrs, type: 'button' }
  return attrs
})
</script>

<style scoped>
.ui-row {
  position: relative;
}

.ui-row + .ui-row::before {
  position: absolute;
  top: 0;
  right: 0;
  left: var(--space-4);
  border-top: 1px solid var(--separator);
  content: '';
}

.ui-row__inner {
  display: flex;
  gap: var(--space-4);
  align-items: center;
  width: 100%;
  min-height: 52px;
  padding: var(--space-3) var(--space-4);
  border: 0;
  background: transparent;
  color: var(--label-1);
  font: inherit;
  text-align: left;
  text-decoration: none;
}

.is-action .ui-row__inner {
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.is-action .ui-row__inner:hover {
  background: var(--fill-1);
}

.is-action .ui-row__inner:active {
  background: var(--fill-2);
}

.is-action .ui-row__inner:focus-visible {
  outline: var(--focus-ring);
  outline-offset: calc(var(--focus-ring-offset) * -1 - 2px);
  border-radius: var(--radius-md);
}

.ui-row__leading {
  display: inline-flex;
  flex: none;
  color: var(--label-2);
}

.ui-row__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.ui-row__label {
  overflow-wrap: anywhere;
}

.ui-row__sub {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.ui-row__value {
  min-width: 0;
  color: var(--label-2);
  font-variant-numeric: tabular-nums;
  text-align: right;
  overflow-wrap: anywhere;
}

.ui-row__control {
  display: flex;
  flex: 0 0 auto;
  justify-content: flex-end;
  min-width: 0;
}

/* Field controls (select, text field) take up to 300 px; switches and
   buttons keep their own width so the label never wraps early. */
.ui-row__control:has(.ui-field) {
  flex: 0 1 300px;
}

.ui-row__chevron {
  color: var(--label-3);
}

.is-stacked .ui-row__inner {
  flex-wrap: wrap;
}

.is-stacked .ui-row__control {
  flex: 1 1 100%;
  justify-content: stretch;
}

@media (max-width: 639px) {
  .ui-row__control:has(.ui-field) {
    flex-basis: auto;
  }

  .is-stacked .ui-row__control:has(.ui-field) {
    flex-basis: 100%;
  }
}
</style>
