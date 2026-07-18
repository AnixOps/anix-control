<template>
  <div v-if="open" class="plugin-dialog-backdrop" @click.self="requestClose">
    <section
      ref="modal"
      class="plugin-dialog"
      data-testid="plugin-installation-dialog"
      role="dialog"
      aria-modal="true"
      aria-labelledby="plugin-installation-title"
      @keydown="handleKeydown"
    >
      <header class="dialog-header">
        <div>
          <h2 id="plugin-installation-title">{{ isUpgrade ? t('control.update.title') : t('control.install.title') }}</h2>
          <p class="plugin-name">{{ plugin?.name || plugin?.id }}</p>
        </div>
        <button
          ref="closeButton"
          class="icon-button"
          type="button"
          :aria-label="t('common.actions.close')"
          :title="t('common.actions.close')"
          :disabled="saving"
          @click="requestClose"
        >
          <X :size="20" aria-hidden="true" />
          <span class="sr-only">{{ t('common.actions.close') }}</span>
        </button>
      </header>

      <div class="dialog-body">
        <div class="form-group">
          <label for="plugin-install-target">{{ t('control.install.target') }}</label>
          <select id="plugin-install-target" v-model="selectedTarget" :disabled="saving || isUpgrade">
            <option v-for="target in targetOptions" :key="target" :value="target">{{ target }}</option>
          </select>
        </div>

        <div class="form-group">
          <label for="plugin-install-version">{{ t('control.install.version') }}</label>
          <select id="plugin-install-version" v-model="selectedVersion" :disabled="saving">
            <option v-for="release in availableReleases" :key="release.id || release.version" :value="release.version">{{ release.version }}</option>
          </select>
        </div>

        <label v-if="!isUpgrade" class="toggle-field">
          <input v-model="selectedEnabled" type="checkbox" :disabled="saving" />
          <span>{{ t('control.install.enableAfterInstall') }}</span>
        </label>
        <p v-if="error" class="dialog-error" role="alert">{{ error }}</p>
      </div>

      <footer class="dialog-footer">
        <button class="btn" type="button" :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</button>
        <button
          class="btn btn-primary"
          data-action="save-installation"
          type="button"
          :disabled="saving || !selectedTarget || !selectedVersion"
          @click="save"
        >
          {{ saving ? t('control.actions.saving') : isUpgrade ? t('control.actions.upgrade') : t('control.actions.install') }}
        </button>
      </footer>
    </section>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { X } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { releaseTargets } from '@/utils/kernelPluginRelease'
import { useModalFocus } from '@/composables/useModalFocus'

const props = defineProps({
  open: { type: Boolean, default: false },
  plugin: { type: Object, default: null },
  target: { type: String, default: '' },
  targets: { type: Array, default: () => [] },
  releases: { type: Array, default: () => [] },
  installation: { type: Object, default: null },
  enabled: { type: Boolean, default: true },
  saving: { type: Boolean, default: false },
  error: { type: String, default: '' },
})
const emit = defineEmits(['close', 'save'])
const { t } = useAppI18n()
const closeButton = ref(null)
const modal = ref(null)
const selectedTarget = ref('')
const selectedVersion = ref('')
const selectedEnabled = ref(true)

const isUpgrade = computed(() => Boolean(props.installation))
const canClose = computed(() => !props.saving)
const openState = computed(() => props.open)
const targetOptions = computed(() => {
  const explicitTargets = props.targets
    .map(target => typeof target === 'string' ? target : target?.target)
    .filter(Boolean)
  if (explicitTargets.length) return [...new Set(explicitTargets)]
  return [...new Set(props.releases.flatMap(releaseTargets))]
})
const availableReleases = computed(() => props.releases.filter(release => releaseTargets(release).includes(selectedTarget.value)))
const { handleKeydown, requestClose } = useModalFocus({
  open: openState,
  canClose,
  container: modal,
  initialFocus: closeButton,
  close: () => emit('close'),
})

watch([() => props.open, () => props.target, () => props.installation, targetOptions], ([isOpen]) => {
  const requestedTarget = props.target || props.installation?.target || targetOptions.value[0] || ''
  if (isOpen && (requestedTarget || !targetOptions.value.includes(selectedTarget.value))) {
    selectedTarget.value = requestedTarget
    selectedEnabled.value = props.installation?.enabled ?? props.enabled
  }
}, { immediate: true })

watch(() => props.open, isOpen => {
  if (isOpen) resetSelectedVersion()
})

watch([selectedTarget, availableReleases, () => props.installation], () => {
  if (!availableReleases.value.some(release => release.version === selectedVersion.value)) resetSelectedVersion()
}, { immediate: true })

function resetSelectedVersion() {
  const desiredVersion = props.installation?.desired_version
  const releases = availableReleases.value
  selectedVersion.value = releases.find(release => release.version === desiredVersion)?.version || releases[0]?.version || ''
}

function save() {
  if (!selectedTarget.value || !selectedVersion.value || props.saving) return
  emit('save', {
    target: selectedTarget.value,
    version: selectedVersion.value,
    enabled: selectedEnabled.value,
  })
}
</script>

<style scoped>
.plugin-dialog-backdrop { position: fixed; inset: 0; z-index: 1200; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(15, 23, 42, .62); }
.plugin-dialog { width: min(100%, 520px); max-height: calc(100dvh - 40px); overflow-y: auto; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface-color); color: var(--text-color); box-shadow: 0 18px 32px rgba(15, 23, 42, .2); }
.dialog-header, .dialog-footer { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid var(--border-color); }
.dialog-header h2 { margin: 0; font-size: 18px; line-height: 1.35; }
.plugin-name { margin: 4px 0 0; color: var(--text-secondary); }
.dialog-body { display: grid; gap: 18px; padding: 20px; }
.form-group { display: grid; gap: 8px; }
.form-group label, .toggle-field { font-size: 14px; font-weight: 600; }
.form-group select { min-height: 38px; width: 100%; border: 1px solid var(--border-color); border-radius: 6px; background: var(--bg-color); color: var(--text-color); padding: 8px 10px; }
.toggle-field { display: flex; align-items: center; gap: 8px; font-weight: 400; }
.dialog-error { margin: 0; color: var(--error-color); overflow-wrap: anywhere; }
.dialog-footer { align-items: center; justify-content: flex-end; border-top: 1px solid var(--border-color); border-bottom: 0; }
.btn, .icon-button { min-height: 36px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; }
.btn { padding: 8px 12px; }
.icon-button { display: inline-grid; width: 36px; place-items: center; padding: 0; }
.btn-primary { border-color: var(--primary-color); background: var(--primary-color); color: #fff; }
.btn:disabled, .icon-button:disabled { cursor: not-allowed; opacity: .55; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 720px) {
  .plugin-dialog-backdrop { padding: 0; }
  .plugin-dialog { width: 100vw; max-height: none; min-height: 100dvh; border: 0; border-radius: 0; display: flex; flex-direction: column; }
  .dialog-body { flex: 1; align-content: start; }
}
</style>
