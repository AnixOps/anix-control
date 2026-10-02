<template>
  <Transition name="settings-save-bar">
    <div
      v-if="visible"
      class="settings-save-bar"
      role="region"
      :aria-label="t('settingsForm.bar.label')"
      data-test="settings-save-bar"
    >
      <p class="settings-save-bar__text">
        <span class="settings-save-bar__dot" aria-hidden="true"></span>
        {{ invalid ? t('settingsForm.bar.invalid') : t('settingsForm.bar.unsaved') }}
      </p>
      <div class="settings-save-bar__actions">
        <UiButton :disabled="saving" data-test="settings-discard" @click="emit('discard')">{{ t('settingsForm.bar.discard') }}</UiButton>
        <UiButton
          variant="primary"
          :loading="saving"
          :disabled="invalid"
          data-test="settings-save"
          @click="emit('save')"
        >
          {{ t('settingsForm.bar.save') }}
        </UiButton>
      </div>
    </div>
  </Transition>
</template>

<script setup>
// The settings template's "保存 / 放弃" bar (plan §7.3): it floats up at the
// bottom of the section while the form has unsaved changes and sticks to
// the bottom of the window while the page scrolls. 保存 is disabled while a
// field is invalid; the text then says so.
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'

defineProps({
  visible: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  invalid: { type: Boolean, default: false }
})

const emit = defineEmits(['save', 'discard'])
const { t } = useAppI18n()
</script>

<style scoped>
.settings-save-bar {
  position: sticky;
  bottom: var(--space-4);
  z-index: var(--z-sticky);
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3) var(--space-4);
  align-items: center;
  justify-content: space-between;
  margin-top: var(--space-6);
  padding: var(--space-3) var(--space-3) var(--space-3) var(--space-4);
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-3), 0 0 0 0.5px var(--separator);
}

.settings-save-bar__text {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
}

.settings-save-bar__dot {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-pill);
  background: var(--warning);
}

.settings-save-bar__actions {
  display: flex;
  gap: var(--space-2);
  margin-left: auto;
}

.settings-save-bar-enter-active,
.settings-save-bar-leave-active {
  transition:
    opacity var(--dur-overlay) var(--ease-standard),
    transform var(--dur-overlay) var(--ease-emphasized);
}

.settings-save-bar-enter-from,
.settings-save-bar-leave-to {
  opacity: 0;
  transform: translateY(12px);
}

@media (prefers-reduced-motion: reduce) {
  .settings-save-bar-enter-from,
  .settings-save-bar-leave-to {
    transform: none;
  }
}

@media (max-width: 639.98px) {
  .settings-save-bar {
    bottom: calc(var(--space-3) + env(safe-area-inset-bottom));
  }

  .settings-save-bar__actions {
    flex: 1 1 100%;
  }

  .settings-save-bar__actions > * {
    flex: 1 1 0;
  }
}
</style>
