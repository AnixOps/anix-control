<template>
  <DialogRoot :open="open" @update:open="onOpenChange">
    <DialogTrigger v-if="$slots.trigger" as-child>
      <slot name="trigger" />
    </DialogTrigger>
    <DialogPortal>
      <DialogOverlay class="ui-dialog-overlay" :class="`ui-dialog-overlay--${size}`">
        <DialogContent
          ref="contentRef"
          class="ui-dialog"
          :class="`ui-dialog--${size}`"
          v-bind="contentAttrs"
          @open-auto-focus="onOpenAutoFocus"
          @escape-key-down="onEscape"
          @interact-outside="onInteractOutside"
        >
          <header class="ui-dialog__header">
            <DialogTitle class="ui-dialog__title">
              <slot name="title">{{ title }}</slot>
            </DialogTitle>
            <DialogDescription v-if="description || $slots.description" class="ui-dialog__description">
              <slot name="description">{{ description }}</slot>
            </DialogDescription>
          </header>
          <div v-if="$slots.default" class="ui-dialog__body">
            <slot :close="close" />
          </div>
          <footer v-if="$slots.footer" class="ui-dialog__footer">
            <slot name="footer" :close="close" />
          </footer>
          <DialogClose
            v-if="!hideClose"
            class="ui-dialog__close"
            :aria-label="t('ui.actions.close')"
            :title="t('ui.actions.close')"
          >
            <UiIcon :icon="X" :size="20" />
          </DialogClose>
        </DialogContent>
      </DialogOverlay>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup>
// Modal dialog (Reka Dialog): role="dialog", aria-modal, aria-labelledby the
// title and aria-describedby the description; focus moves in, is trapped,
// and returns to the opener; Esc and a click on the scrim close it unless
// `dismissible` is false; the page behind does not scroll.
// Sizes: sm 420, md 560, lg 760 px. Below 834 px md and lg fill the screen.
// Footer: buttons right-aligned, primary action last (rightmost).
// The close button (Lucide x) comes last in tab order, after the footer.
import { computed, ref, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from 'reka-ui'
import { X } from '@lucide/vue'
import UiIcon from './UiIcon.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '' },
  description: { type: String, default: '' },
  size: { type: String, default: 'md', validator: value => ['sm', 'md', 'lg'].includes(value) },
  hideClose: { type: Boolean, default: false },
  // false: Esc and the scrim do not close it (unsaved form, running action).
  dismissible: { type: Boolean, default: true },
  // false: a click on the scrim does not close it (Esc still does).
  closeOnScrim: { type: Boolean, default: true },
  // CSS selector inside the dialog to focus first; default: the first
  // focusable element (a field, else the first footer button).
  initialFocus: { type: String, default: '' }
})

const emit = defineEmits(['update:open', 'close'])
const attrs = useAttrs()
const { t } = useI18n()
const contentRef = ref(null)

// Without a description, drop aria-describedby (Reka points it at an id that
// would not exist).
const contentAttrs = computed(() => {
  // Reka hides the rest of the page from assistive tech; aria-modal says so
  // explicitly (guidelines/accessibility.md).
  const out = { 'aria-modal': 'true', ...attrs }
  if (!props.description && !attrs['aria-describedby']) out['aria-describedby'] = undefined
  return out
})

function onOpenChange(value) {
  emit('update:open', value)
  if (!value) emit('close')
}

function close() {
  onOpenChange(false)
}

function onOpenAutoFocus(event) {
  if (!props.initialFocus) return
  const root = contentRef.value?.$el
  const target = root?.querySelector?.(props.initialFocus)
  if (target) {
    event.preventDefault()
    target.focus()
  }
}

function onEscape(event) {
  if (!props.dismissible) event.preventDefault()
}

function onInteractOutside(event) {
  if (!props.dismissible || !props.closeOnScrim) event.preventDefault()
}

defineExpose({ close })
</script>

<style scoped>
.ui-dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: var(--z-modal);
  display: grid;
  place-items: center;
  padding: var(--space-6) var(--space-4);
  overflow-y: auto;
  background: var(--scrim);
}

.ui-dialog-overlay[data-state='open'] {
  animation: ui-fade-in var(--dur-overlay) var(--ease-standard);
}

.ui-dialog-overlay[data-state='closed'] {
  animation: ui-fade-out var(--dur-micro) var(--ease-exit);
}

.ui-dialog {
  --ui-dialog-width: 560px;

  position: relative;
  display: flex;
  flex-direction: column;
  width: min(var(--ui-dialog-width), 100%);
  max-height: calc(100dvh - var(--space-12));
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-3);
  color: var(--label-1);
  outline: none;
}

.ui-dialog[data-state='open'] {
  animation: ui-pop-in var(--dur-overlay) var(--ease-emphasized);
}

.ui-dialog[data-state='closed'] {
  animation: ui-pop-out var(--dur-micro) var(--ease-exit);
}

.ui-dialog--sm {
  --ui-dialog-width: 420px;
}

.ui-dialog--lg {
  --ui-dialog-width: 760px;
}

.ui-dialog__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-6) var(--space-16) 0 var(--space-6);
}

.ui-dialog__title {
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
  overflow-wrap: anywhere;
}

.ui-dialog__description {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.ui-dialog__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-5);
  min-height: 0;
  padding: var(--space-5) var(--space-6) 0;
  overflow-y: auto;
}

.ui-dialog__footer {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  justify-content: flex-end;
  padding: var(--space-6);
}

.ui-dialog__header:last-child {
  padding-bottom: var(--space-6);
}

.ui-dialog__close {
  position: absolute;
  top: var(--space-4);
  right: var(--space-4);
  display: grid;
  place-items: center;
  width: var(--size-control-md);
  height: var(--size-control-md);
  padding: 0;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--label-2);
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.ui-dialog__close:hover {
  background: var(--fill-1);
  color: var(--label-1);
}

.ui-dialog__close:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

/* Phones and small tablets: md and lg fill the screen; sm stays a card. */
@media (max-width: 833px) {
  .ui-dialog-overlay--md,
  .ui-dialog-overlay--lg {
    padding: 0;
  }

  .ui-dialog--md,
  .ui-dialog--lg {
    width: 100%;
    height: 100dvh;
    max-height: none;
    border-radius: 0;
    padding-top: env(safe-area-inset-top, 0px);
    padding-bottom: env(safe-area-inset-bottom, 0px);
  }

  .ui-dialog__footer {
    padding: var(--space-4);
  }

  .ui-dialog__footer > :deep(*) {
    flex: 1 1 auto;
  }
}

@keyframes ui-fade-in {
  from {
    opacity: 0;
  }
}

@keyframes ui-fade-out {
  to {
    opacity: 0;
  }
}

@keyframes ui-pop-in {
  from {
    opacity: 0;
    transform: scale(0.96);
  }
}

@keyframes ui-pop-out {
  to {
    opacity: 0;
    transform: scale(0.98);
  }
}

/* Reduced motion: fades only, no scale. */
@media (prefers-reduced-motion: reduce) {
  .ui-dialog[data-state='open'] {
    animation-name: ui-fade-in;
  }

  .ui-dialog[data-state='closed'] {
    animation-name: ui-fade-out;
  }
}
</style>
