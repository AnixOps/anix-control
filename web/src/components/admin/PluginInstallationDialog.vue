<template>
  <UiDialog
    :open="open"
    size="sm"
    data-testid="plugin-installation-dialog"
    :title="isUpgrade ? t('control.update.title') : t('control.install.title')"
    :description="plugin?.name || plugin?.id || ''"
    :dismissible="canClose"
    @update:open="value => { if (!value) requestClose() }"
  >
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
    <template #footer>
      <UiButton :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton
        variant="primary"
        data-action="save-installation"
        :loading="saving"
        :disabled="!selectedTarget || !selectedVersion"
        @click="save"
      >
        {{ isUpgrade ? t('control.actions.upgrade') : t('control.actions.install') }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Install or upgrade one plugin target (UiDialog: focus trap, Esc, focus
// return). It cannot be dismissed while the request is saving.
import { computed, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { releaseTargets } from '@/utils/kernelPluginRelease'
import { UiButton, UiDialog } from '@/ui'

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
const selectedTarget = ref('')
const selectedVersion = ref('')
const selectedEnabled = ref(true)

const isUpgrade = computed(() => Boolean(props.installation))
const canClose = computed(() => !props.saving)
const targetOptions = computed(() => {
  const explicitTargets = props.targets
    .map(target => typeof target === 'string' ? target : target?.target)
    .filter(Boolean)
  if (explicitTargets.length) return [...new Set(explicitTargets)]
  return [...new Set(props.releases.flatMap(releaseTargets))]
})
const availableReleases = computed(() => props.releases.filter(release => releaseTargets(release).includes(selectedTarget.value)))
function requestClose() {
  if (canClose.value) emit('close')
}

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
.dialog-body { display: grid; gap: var(--space-4); }
.form-group { display: grid; gap: var(--space-2); }
.form-group label, .toggle-field { font-size: var(--type-callout-size); font-weight: var(--weight-semibold); }
.form-group select { width: 100%; }
.toggle-field { display: flex; align-items: center; gap: var(--space-2); font-weight: var(--weight-regular); }
.dialog-error { margin: 0; color: var(--danger); overflow-wrap: anywhere; }
</style>
