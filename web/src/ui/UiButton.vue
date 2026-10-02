<template>
  <component
    :is="tag"
    v-bind="elementAttrs"
    class="ui-button"
    :class="[`ui-button--${variant}`, `ui-button--${size}`, { 'is-loading': loading, 'is-block': block }]"
    @click="onClick"
  >
    <span class="ui-button__content">
      <slot name="icon">
        <UiIcon v-if="icon" :icon="icon" :size="iconSize" />
      </slot>
      <span v-if="$slots.default" class="ui-button__label"><slot /></span>
      <slot name="icon-end">
        <UiIcon v-if="iconEnd" :icon="iconEnd" :size="iconSize" />
      </slot>
    </span>
    <span v-if="loading" class="ui-button__spinner">
      <UiSpinner :size="iconSize" />
    </span>
  </component>
</template>

<script setup>
// Pill button (guidelines/principles.md: consistent controls).
// - variant: primary (one per view), secondary, tertiary (text link style),
//   danger, danger-soft, ghost;
// - size: sm 28 (tables), md 36 (default), lg 44 (touch, page actions);
// - loading keeps the width, sets aria-busy and swallows clicks, so a form
//   cannot be submitted twice; the button stays focusable;
// - href renders an <a>; `as` renders another component (RouterLink).
import { computed, useAttrs } from 'vue'
import UiIcon from './UiIcon.vue'
import UiSpinner from './UiSpinner.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  variant: {
    type: String,
    default: 'secondary',
    validator: value => ['primary', 'secondary', 'tertiary', 'danger', 'danger-soft', 'ghost'].includes(value)
  },
  size: { type: String, default: 'md', validator: value => ['sm', 'md', 'lg'].includes(value) },
  type: { type: String, default: 'button' },
  icon: { type: [Object, Function], default: null },
  iconEnd: { type: [Object, Function], default: null },
  loading: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  block: { type: Boolean, default: false },
  href: { type: String, default: '' },
  as: { type: [String, Object, Function], default: '' }
})

const emit = defineEmits(['click'])
const attrs = useAttrs()

const tag = computed(() => props.as || (props.href ? 'a' : 'button'))
const isNativeButton = computed(() => tag.value === 'button')
const iconSize = computed(() => (props.size === 'lg' ? 20 : 16))

const elementAttrs = computed(() => {
  const out = { ...attrs }
  if (isNativeButton.value) {
    out.type = props.type
    out.disabled = props.disabled || undefined
  } else {
    if (props.href) out.href = props.disabled ? undefined : props.href
    if (props.disabled) {
      out['aria-disabled'] = 'true'
      out.tabindex = '-1'
    }
  }
  if (props.loading) {
    out['aria-busy'] = 'true'
    out['aria-disabled'] = 'true'
  }
  return out
})

function onClick(event) {
  if (props.loading || props.disabled) {
    event.preventDefault()
    return
  }
  emit('click', event)
}
</script>

<style scoped>
.ui-button {
  --ui-button-height: var(--size-control-md);
  --ui-button-pad: var(--space-4);
  --ui-button-bg: var(--fill-2);
  --ui-button-bg-hover: var(--fill-3);
  --ui-button-fg: var(--label-1);

  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  height: var(--ui-button-height);
  /* The legacy global button rule (style.css) sets 40/44 px on phones;
     the touch hit area comes from ::after instead. */
  min-height: 0;
  padding: 0 var(--ui-button-pad);
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--ui-button-bg);
  color: var(--ui-button-fg);
  font-family: inherit;
  font-size: var(--type-body-size);
  font-weight: var(--weight-medium);
  line-height: 1;
  white-space: nowrap;
  text-decoration: none;
  cursor: pointer;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
  transition:
    background-color var(--dur-micro) var(--ease-standard),
    color var(--dur-micro) var(--ease-standard),
    transform var(--dur-micro) var(--ease-standard);
}

.ui-button:hover {
  background: var(--ui-button-bg-hover);
}

.ui-button:active:not(:disabled, [aria-disabled='true']) {
  transform: scale(0.98);
}

.ui-button:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.ui-button__content {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}

.ui-button__label {
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Loading: the label keeps its space (no width jump) and stays in the
   accessible name; the spinner sits on top. */
.ui-button.is-loading {
  cursor: progress;
}

.ui-button.is-loading .ui-button__content {
  opacity: 0;
}

.ui-button__spinner {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
}

.ui-button:disabled,
.ui-button[aria-disabled='true']:not(.is-loading) {
  opacity: 0.4;
  cursor: not-allowed;
}

.ui-button.is-block {
  display: flex;
  width: 100%;
}

/* Sizes */
.ui-button--sm {
  --ui-button-height: var(--size-control-sm);
  --ui-button-pad: var(--space-3);

  font-size: var(--type-callout-size);
}

.ui-button--sm .ui-button__content {
  gap: var(--space-1);
}

.ui-button--lg {
  --ui-button-height: var(--size-control-lg);
  --ui-button-pad: var(--space-6);
}

/* Variants */
.ui-button--primary {
  --ui-button-bg: var(--accent-fill);
  --ui-button-bg-hover: var(--accent-fill-hover);
  --ui-button-fg: var(--on-accent);
}

.ui-button--secondary {
  --ui-button-bg: var(--fill-2);
  --ui-button-bg-hover: var(--fill-3);
  --ui-button-fg: var(--label-1);
}

.ui-button--tertiary {
  --ui-button-bg: transparent;
  --ui-button-bg-hover: var(--accent-soft);
  --ui-button-fg: var(--accent);
  --ui-button-pad: var(--space-2);
}

.ui-button--tertiary:hover {
  color: var(--accent-hover);
}

.ui-button--danger {
  --ui-button-bg: var(--danger-fill);
  --ui-button-bg-hover: var(--danger-fill-hover);
  --ui-button-fg: var(--on-danger);
}

.ui-button--danger-soft {
  --ui-button-bg: var(--danger-soft);
  --ui-button-bg-hover: var(--danger-soft);
  /* --danger on its own soft tint is 4.2:1 in dark mode (on an elevated
     surface less); mixing in the label colour keeps 4.5:1 or more in both
     themes (darker red in light, lighter in dark). */
  --ui-button-fg: color-mix(in srgb, var(--danger) 65%, var(--label-1));
}

.ui-button--danger-soft:hover {
  box-shadow: inset 0 0 0 1px var(--danger);
}

.ui-button--ghost {
  --ui-button-bg: transparent;
  --ui-button-bg-hover: var(--fill-1);
  --ui-button-fg: var(--label-1);
}

/* Phones: touch targets of at least 44 px without changing the look. */
@media (pointer: coarse) {
  .ui-button--sm::after,
  .ui-button--md::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    min-height: var(--size-control-lg);
    transform: translate(-50%, -50%);
    content: '';
  }
}

/* Windows high-contrast: keep a visible boundary. */
@media (forced-colors: active) {
  .ui-button {
    border: 1px solid ButtonText;
  }
}
</style>
