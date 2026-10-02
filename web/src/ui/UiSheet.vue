<template>
  <DialogRoot :open="open" @update:open="onOpenChange">
    <DialogTrigger v-if="$slots.trigger" as-child>
      <slot name="trigger" />
    </DialogTrigger>
    <DialogPortal>
      <DialogOverlay class="ui-sheet-overlay">
        <DialogContent
          class="ui-sheet"
          :class="`ui-sheet--${size}`"
          ref="contentRef"
          v-bind="contentAttrs"
          @open-auto-focus="onOpenAutoFocus"
          @escape-key-down="onEscape"
          @interact-outside="onInteractOutside"
        >
          <span class="ui-sheet__grabber" aria-hidden="true" />
          <header class="ui-sheet__header">
            <div class="ui-sheet__heading">
              <DialogTitle class="ui-sheet__title">
                <slot name="title">{{ title }}</slot>
              </DialogTitle>
              <DialogDescription v-if="description || $slots.description" class="ui-sheet__description">
                <slot name="description">{{ description }}</slot>
              </DialogDescription>
            </div>
            <slot name="header-actions" />
            <DialogClose class="ui-sheet__close" :aria-label="t('ui.actions.close')" :title="t('ui.actions.close')">
              <UiIcon :icon="X" :size="20" />
            </DialogClose>
          </header>
          <div :ref="scrollable.setElement" class="ui-sheet__body" :class="{ 'is-grouped': grouped }" :tabindex="scrollable.tabindex.value">
            <slot :close="close" />
          </div>
          <footer v-if="$slots.footer" class="ui-sheet__footer">
            <slot name="footer" :close="close" />
          </footer>
        </DialogContent>
      </DialogOverlay>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup>
// Side panel for quick views and edits that keep the list in context (user
// details, node quick view, rule editor). Same behaviour as UiDialog (Reka
// Dialog: role="dialog", focus trap and return, Esc, scrim click, no page
// scroll), sliding in from the right; below 834 px it becomes a bottom sheet.
// Sizes: sm 360, md 440 (default), lg 600 px. Focus starts on the first
// text field in the body, else on the close button.
import { computed, ref, useAttrs } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from 'reka-ui'
import { X } from '@lucide/vue'
import UiIcon from './UiIcon.vue'
import { useScrollableFocus } from './internal/useScrollableFocus'

defineOptions({ inheritAttrs: false })

const props = defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '' },
  description: { type: String, default: '' },
  size: { type: String, default: 'md', validator: value => ['sm', 'md', 'lg'].includes(value) },
  // Body on --bg-grouped, for UiGroupedList content (settings style).
  grouped: { type: Boolean, default: false },
  dismissible: { type: Boolean, default: true }
})

const emit = defineEmits(['update:open', 'close'])
const attrs = useAttrs()
const { t } = useI18n()

const contentRef = ref(null)
// Read-only content that scrolls stays reachable from the keyboard.
const scrollable = useScrollableFocus()

// Text entry only: never land on a switch or a button, where a stray Space
// would change something.
const TEXT_ENTRY = 'input:not([disabled]):not([readonly]):not([type="hidden"]):not([type="checkbox"]):not([type="radio"]), textarea:not([disabled]):not([readonly])'

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

// Focus the first text field in the body (an edit form starts typing at
// once); otherwise Reka's default, the close button.
function onOpenAutoFocus(event) {
  const root = contentRef.value?.$el
  const target = root?.querySelector?.(`.ui-sheet__body :is(${TEXT_ENTRY})`)
  if (target) {
    event.preventDefault()
    target.focus()
  }
}

function onEscape(event) {
  if (!props.dismissible) event.preventDefault()
}

function onInteractOutside(event) {
  if (!props.dismissible) event.preventDefault()
}

defineExpose({ close })
</script>

<style scoped>
.ui-sheet-overlay {
  position: fixed;
  inset: 0;
  z-index: var(--z-drawer);
  display: flex;
  justify-content: flex-end;
  background: var(--scrim);
}

.ui-sheet-overlay[data-state='open'] {
  animation: ui-sheet-fade-in var(--dur-overlay) var(--ease-standard);
}

.ui-sheet-overlay[data-state='closed'] {
  animation: ui-sheet-fade-out var(--dur-overlay) var(--ease-exit);
}

.ui-sheet {
  --ui-sheet-width: 440px;

  display: flex;
  flex-direction: column;
  width: min(var(--ui-sheet-width), 100%);
  height: 100%;
  background: var(--bg-elevated);
  box-shadow: var(--shadow-3);
  color: var(--label-1);
  outline: none;
}

.ui-sheet[data-state='open'] {
  animation: ui-sheet-in var(--dur-overlay) var(--ease-emphasized);
}

.ui-sheet[data-state='closed'] {
  animation: ui-sheet-out var(--dur-overlay) var(--ease-exit);
}

.ui-sheet--sm {
  --ui-sheet-width: 360px;
}

.ui-sheet--lg {
  --ui-sheet-width: 600px;
}

.ui-sheet__grabber {
  display: none;
}

.ui-sheet__header {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  padding: var(--space-4) var(--space-4) var(--space-4) var(--space-6);
  border-bottom: 1px solid var(--separator);
}

.ui-sheet__heading {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
  padding-top: var(--space-1);
}

.ui-sheet__title {
  overflow: hidden;
  font-size: var(--type-title-3-size);
  font-weight: var(--type-title-3-weight);
  line-height: var(--type-title-3-line);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ui-sheet__description {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-sheet__close {
  display: grid;
  flex: none;
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

.ui-sheet__close:hover {
  background: var(--fill-1);
  color: var(--label-1);
}

.ui-sheet__close:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.ui-sheet__body:focus-visible {
  outline: var(--focus-ring);
  outline-offset: calc(-1 * var(--focus-ring-offset));
}

.ui-sheet__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-6);
  min-height: 0;
  padding: var(--space-6);
  overflow-y: auto;
  overscroll-behavior: contain;
}

.ui-sheet__body.is-grouped {
  background: var(--bg-grouped);
}

.ui-sheet__footer {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  justify-content: flex-end;
  padding: var(--space-4) var(--space-6);
  padding-bottom: calc(var(--space-4) + env(safe-area-inset-bottom, 0px));
  border-top: 1px solid var(--separator);
}

/* Below 834 px: a bottom sheet, full width, up to 92 % of the screen. */
@media (max-width: 833px) {
  .ui-sheet-overlay {
    align-items: flex-end;
  }

  .ui-sheet {
    width: 100%;
    height: auto;
    max-height: 92dvh;
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  }

  .ui-sheet[data-state='open'] {
    animation-name: ui-sheet-up;
  }

  .ui-sheet[data-state='closed'] {
    animation-name: ui-sheet-down;
  }

  .ui-sheet__grabber {
    display: block;
    flex: none;
    width: 36px;
    height: 5px;
    margin: var(--space-2) auto 0;
    border-radius: var(--radius-pill);
    background: var(--fill-3);
  }

  .ui-sheet__header {
    padding: var(--space-2) var(--space-3) var(--space-3) var(--space-4);
  }

  .ui-sheet__body {
    padding: var(--space-4);
  }

  .ui-sheet__footer {
    padding-right: var(--space-4);
    padding-left: var(--space-4);
  }

  .ui-sheet__footer > :deep(*) {
    flex: 1 1 auto;
  }
}

@keyframes ui-sheet-fade-in {
  from {
    opacity: 0;
  }
}

@keyframes ui-sheet-fade-out {
  to {
    opacity: 0;
  }
}

@keyframes ui-sheet-in {
  from {
    transform: translateX(40px);
    opacity: 0;
  }
}

@keyframes ui-sheet-out {
  to {
    transform: translateX(40px);
    opacity: 0;
  }
}

@keyframes ui-sheet-up {
  from {
    transform: translateY(100%);
  }
}

@keyframes ui-sheet-down {
  to {
    transform: translateY(100%);
  }
}

@media (prefers-reduced-motion: reduce) {
  .ui-sheet[data-state='open'] {
    animation-name: ui-sheet-fade-in;
  }

  .ui-sheet[data-state='closed'] {
    animation-name: ui-sheet-fade-out;
  }
}
</style>
