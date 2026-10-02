<template>
  <component
    :is="tag"
    v-bind="elementAttrs"
    class="ui-icon-button"
    :class="[`ui-icon-button--${variant}`, `ui-icon-button--${size}`]"
    :aria-label="label"
    :title="tooltip ? label : undefined"
    @click="onClick"
  >
    <slot>
      <UiIcon :icon="icon" :size="size === 'lg' ? 20 : 16" />
    </slot>
  </component>
</template>

<script setup>
// Square icon-only button. `label` is required: it becomes the aria-label
// and, unless tooltip is false, the native tooltip (guidelines/iconography.md).
// Toggle buttons pass `pressed` (aria-pressed) and keep the same label.
import { computed, useAttrs } from 'vue'
import UiIcon from './UiIcon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  label: { type: String, required: true },
  icon: { type: [Object, Function], default: null },
  variant: { type: String, default: 'ghost', validator: value => ['ghost', 'secondary', 'plain'].includes(value) },
  size: { type: String, default: 'md', validator: value => ['sm', 'md', 'lg'].includes(value) },
  type: { type: String, default: 'button' },
  disabled: { type: Boolean, default: false },
  pressed: { type: Boolean, default: undefined },
  tooltip: { type: Boolean, default: true },
  as: { type: [String, Object, Function], default: '' }
})

const emit = defineEmits(['click'])
const attrs = useAttrs()

if (import.meta.env.DEV && !props.label) {
  console.warn('[UiIconButton] `label` is required: icon-only buttons need an accessible name.')
}

const tag = computed(() => props.as || 'button')

const elementAttrs = computed(() => {
  const out = { ...attrs }
  if (tag.value === 'button') {
    out.type = props.type
    out.disabled = props.disabled || undefined
  } else if (props.disabled) {
    out['aria-disabled'] = 'true'
  }
  if (props.pressed !== undefined) out['aria-pressed'] = String(props.pressed)
  return out
})

function onClick(event) {
  if (props.disabled) {
    event.preventDefault()
    return
  }
  emit('click', event)
}
</script>

<style scoped>
.ui-icon-button {
  --ui-icon-button-size: var(--size-control-md);

  position: relative;
  display: inline-grid;
  flex: none;
  place-items: center;
  width: var(--ui-icon-button-size);
  height: var(--ui-icon-button-size);
  /* The size is fixed; on touch screens the 44 px hit area comes from
     ::after. */
  min-height: 0;
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--label-2);
  cursor: pointer;
  -webkit-tap-highlight-color: transparent;
  transition:
    background-color var(--dur-micro) var(--ease-standard),
    color var(--dur-micro) var(--ease-standard);
}

.ui-icon-button:hover {
  background: var(--fill-1);
  color: var(--label-1);
}

.ui-icon-button:active {
  background: var(--fill-2);
}

.ui-icon-button:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.ui-icon-button[aria-pressed='true'] {
  background: var(--accent-soft);
  color: var(--accent);
}

.ui-icon-button:disabled,
.ui-icon-button[aria-disabled='true'] {
  opacity: 0.4;
  cursor: not-allowed;
}

.ui-icon-button--secondary {
  background: var(--fill-2);
  color: var(--label-1);
}

.ui-icon-button--secondary:hover {
  background: var(--fill-3);
}

/* plain: no hover fill, for use inside inputs and toasts */
.ui-icon-button--plain:hover {
  background: transparent;
}

.ui-icon-button--sm {
  --ui-icon-button-size: var(--size-control-sm);
}

.ui-icon-button--lg {
  --ui-icon-button-size: var(--size-control-lg);
}

/* Touch: a 44 px hit area around the smaller sizes. */
@media (pointer: coarse) {
  .ui-icon-button--sm::after,
  .ui-icon-button--md::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: var(--size-control-lg);
    height: var(--size-control-lg);
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
