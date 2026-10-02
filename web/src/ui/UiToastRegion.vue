<template>
  <div class="ui-toast-root">
    <section
      ref="regionRef"
      class="ui-toast-region"
      :aria-label="toasts.length ? t('ui.toast.region') : undefined"
      tabindex="-1"
      @mouseenter="hovered = true"
      @mouseleave="hovered = false"
      @focusin="focused = true"
      @focusout="onFocusOut"
      @keydown.esc="onEscape"
    >
      <TransitionGroup tag="ol" name="ui-toast" class="ui-toast-list">
        <li
          v-for="toast in toasts"
          :key="toast.id"
          class="ui-toast"
          :class="`ui-toast--${toast.tone}`"
          :data-toast-id="toast.id"
        >
          <UiIcon :icon="ICONS[toast.tone] || Info" :size="20" />
          <p class="ui-toast__message">{{ toast.message }}</p>
          <button
            v-if="toast.action"
            type="button"
            class="ui-toast__action"
            @click="runAction(toast.id)"
          >
            {{ toast.action.label || t('ui.actions.undo') }}
          </button>
          <button
            v-if="!toast.duration"
            type="button"
            class="ui-toast__close"
            :aria-label="t('ui.actions.dismiss')"
            :title="t('ui.actions.dismiss')"
            @click="dismiss(toast.id)"
          >
            <UiIcon :icon="X" />
          </button>
        </li>
      </TransitionGroup>
    </section>
    <!-- The single live region: every toast is announced here once. -->
    <div class="visually-hidden" role="status" aria-live="polite" aria-atomic="true">{{ liveText }}</div>
  </div>
</template>

<script setup>
// Renders the useToast() queue at the bottom centre, above everything but
// tooltips. One polite live region announces each toast (with a hint when
// it has an action); F8 moves focus to the toasts and Esc returns it.
// Timers pause while the pointer is over the toasts, focus is in them, or
// the page is hidden. Mount once, through <UiHost>.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from '@lucide/vue'
import UiIcon from './UiIcon.vue'
import { dismiss, pauseToasts, resumeToasts, runAction, toastState } from './composables/useToast'

const ICONS = { success: CircleCheck, error: CircleAlert, warning: TriangleAlert, info: Info }

const { t } = useI18n()
const regionRef = ref(null)
const hovered = ref(false)
const focused = ref(false)
const hidden = ref(false)
const liveText = ref('')
let returnFocus = null
let liveTimer = null

const toasts = computed(() => toastState.toasts)

watch([hovered, focused, hidden], ([isHovered, isFocused, isHidden]) => {
  if (isHovered || isFocused || isHidden) pauseToasts()
  else resumeToasts()
})

watch(() => toastState.announcement.seq, () => {
  const { text, hasAction } = toastState.announcement
  // Clear first so a repeated message is announced again.
  liveText.value = ''
  if (liveTimer) clearTimeout(liveTimer)
  liveTimer = setTimeout(() => {
    liveText.value = hasAction ? `${text} ${t('ui.toast.hotkeyHint')}` : text
  }, 100)
})

// When the last toast goes away while focus is inside, give focus back.
watch(() => toasts.value.length, async (length) => {
  if (length === 0 && focused.value) {
    await nextTick()
    restoreFocus()
  }
})

function onFocusOut(event) {
  if (!regionRef.value?.contains(event.relatedTarget)) focused.value = false
}

function restoreFocus() {
  const target = returnFocus
  returnFocus = null
  focused.value = false
  if (target && document.contains(target)) target.focus()
}

function onEscape(event) {
  const item = event.target.closest?.('[data-toast-id]')
  if (item) dismiss(Number(item.dataset.toastId))
  restoreFocus()
}

function onKeydown(event) {
  if (event.key !== 'F8' || !toasts.value.length) return
  event.preventDefault()
  if (!regionRef.value?.contains(document.activeElement)) returnFocus = document.activeElement
  const first = regionRef.value?.querySelector('button')
  ;(first || regionRef.value)?.focus()
}

function onVisibility() {
  hidden.value = document.visibilityState === 'hidden'
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  document.addEventListener('visibilitychange', onVisibility)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  document.removeEventListener('visibilitychange', onVisibility)
  if (liveTimer) clearTimeout(liveTimer)
})
</script>

<style scoped>
.ui-toast-region {
  position: fixed;
  bottom: calc(var(--space-6) + env(safe-area-inset-bottom, 0px));
  left: 50%;
  z-index: var(--z-toast);
  width: max-content;
  max-width: calc(100vw - var(--space-8));
  transform: translateX(-50%);
  outline: none;
  pointer-events: none;
}

.ui-toast-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}

/* Inverted surface, as in the reviewed prototype: --label-1 behind
   --bg-elevated text is the highest-contrast pair in both themes. Icons
   use the text colour; the icon shape (check, alert) carries the tone. */
.ui-toast {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  max-width: 560px;
  min-height: var(--size-control-lg);
  padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--label-1);
  box-shadow: var(--shadow-3);
  color: var(--bg-elevated);
  font-size: var(--type-body-size);
  pointer-events: auto;
}

.ui-toast__message {
  flex: 1;
  min-width: 0;
  padding: var(--space-1) var(--space-2) var(--space-1) 0;
  line-height: var(--type-callout-line);
}

.ui-toast__action,
.ui-toast__close {
  flex: none;
  height: var(--size-control-sm);
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: inherit;
  font-family: inherit;
  cursor: pointer;
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.ui-toast__action {
  padding: 0 var(--space-3);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.ui-toast__close {
  display: grid;
  place-items: center;
  width: var(--size-control-sm);
  padding: 0;
}

.ui-toast__action:hover,
.ui-toast__close:hover {
  background: color-mix(in srgb, var(--bg-elevated) 16%, transparent);
}

/* The ring must show on the inverted surface. */
.ui-toast__action:focus-visible,
.ui-toast__close:focus-visible {
  outline: 2px solid var(--bg-elevated);
  outline-offset: 0;
}

.ui-toast-enter-active {
  transition:
    opacity var(--dur-overlay) var(--ease-emphasized),
    transform var(--dur-overlay) var(--ease-emphasized);
}

.ui-toast-leave-active {
  transition: opacity var(--dur-micro) var(--ease-exit);
}

.ui-toast-enter-from {
  opacity: 0;
  transform: translateY(12px) scale(0.98);
}

.ui-toast-leave-to {
  opacity: 0;
}

.ui-toast-move {
  transition: transform var(--dur-toggle) var(--ease-standard);
}

@media (max-width: 833px) {
  .ui-toast-region {
    bottom: calc(var(--space-20) + env(safe-area-inset-bottom, 0px));
    width: calc(100vw - var(--space-8));
  }

  .ui-toast {
    width: 100%;
  }
}

@media (forced-colors: active) {
  .ui-toast {
    border: 1px solid CanvasText;
  }
}
</style>
