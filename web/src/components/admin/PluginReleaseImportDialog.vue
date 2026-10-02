<template>
  <UiDialog
    :open="open"
    data-testid="plugin-release-import-dialog"
    :title="t('control.releaseImport.title')"
    :dismissible="canClose"
    initial-focus="#plugin-release-manifest"
    @update:open="value => { if (!value) requestClose() }"
  >
    <div class="dialog-body" :aria-busy="releaseReading ? 'true' : undefined">
      <div class="form-group">
        <label for="plugin-release-manifest">{{ t('control.releaseImport.manifest') }}</label>
        <input id="plugin-release-manifest-file" type="file" accept="application/json,.json" :aria-label="t('control.releaseImport.manifest')" :disabled="saving" @change="readTextFile($event, 'manifest')" />
        <textarea id="plugin-release-manifest" v-model="manifest" rows="10" spellcheck="false" :disabled="saving" @input="invalidateTextRead('manifest')"></textarea>
      </div>
      <div class="form-group">
        <label for="plugin-release-signature">{{ t('control.releaseImport.signature') }}</label>
        <input id="plugin-release-signature-file" type="file" accept="text/plain,.sig" :aria-label="t('control.releaseImport.signature')" :disabled="saving" @change="readTextFile($event, 'signature')" />
        <textarea id="plugin-release-signature" v-model="signature" rows="3" spellcheck="false" :disabled="saving" @input="invalidateTextRead('signature')"></textarea>
      </div>
      <div class="form-group">
        <label for="plugin-release-artifact">{{ t('control.releaseImport.artifact') }}</label>
        <input id="plugin-release-artifact" type="file" :disabled="saving" @change="readArtifactFile" />
        <p class="field-help">{{ artifactName || t('control.releaseImport.artifactOptional') }}</p>
      </div>
      <p v-if="inputError || error" class="dialog-error" role="alert">{{ inputError || error }}</p>
    </div>
    <template #footer>
      <UiButton :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton
        variant="primary"
        data-action="save-release"
        :loading="saving"
        :disabled="releaseReading || !manifest.trim() || !signature.trim()"
        @click="save"
      >
        {{ t('control.actions.importRelease') }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Import a signed plugin release (UiDialog: focus trap, Esc, focus return).
// It cannot be dismissed while the import is saving; closing invalidates
// any file read still in flight.
import { computed, reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiDialog } from '@/ui'

const props = defineProps({
  open: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  error: { type: String, default: '' },
})
const emit = defineEmits(['close', 'save'])
const { t } = useAppI18n()
const manifest = ref('')
const signature = ref('')
const artifactBase64 = ref('')
const artifactName = ref('')
const artifactReading = ref(false)
const inputError = ref('')
let artifactReadRevision = 0
const textFields = ['manifest', 'signature']
const textReading = reactive({ manifest: false, signature: false })
const textReadRevision = { manifest: 0, signature: 0 }

const canClose = computed(() => !props.saving)
const releaseReading = computed(() => artifactReading.value || textReading.manifest || textReading.signature)
function requestClose() {
  if (!canClose.value) return
  invalidateReleaseReads()
  emit('close')
}

watch(() => props.open, (isOpen) => {
  if (!isOpen) {
    invalidateReleaseReads()
    return
  }
  resetForm()
}, { immediate: true })

function resetForm() {
  invalidateReleaseReads()
  manifest.value = ''
  signature.value = ''
  artifactBase64.value = ''
  artifactName.value = ''
  inputError.value = ''
}

function invalidateArtifactRead() {
  artifactReadRevision += 1
  artifactReading.value = false
}

function invalidateReleaseReads() {
  invalidateArtifactRead()
  for (const field of textFields) invalidateTextRead(field)
}

function invalidateTextRead(field) {
  if (!textFields.includes(field)) return
  textReadRevision[field] += 1
  textReading[field] = false
}

function setTextValue(field, value) {
  if (field === 'manifest') manifest.value = value
  if (field === 'signature') signature.value = value
}

async function readTextFile(event, field) {
  if (!textFields.includes(field)) return
  const file = event.target.files?.[0]
  const revision = ++textReadRevision[field]
  textReading[field] = false
  if (!file) return
  try {
    inputError.value = ''
    setTextValue(field, '')
    textReading[field] = true
    const value = await file.text()
    if (revision !== textReadRevision[field] || !props.open) return
    setTextValue(field, value)
  } catch {
    if (revision === textReadRevision[field] && props.open) {
      setTextValue(field, '')
      inputError.value = t('control.errors.releaseImport')
    }
  } finally {
    if (revision === textReadRevision[field]) textReading[field] = false
  }
}

async function readArtifactFile(event) {
  const file = event.target.files?.[0]
  const revision = ++artifactReadRevision
  artifactReading.value = false
  artifactBase64.value = ''
  if (!file) {
    artifactName.value = ''
    return
  }
  try {
    inputError.value = ''
    artifactName.value = `${file.name} (${file.size} B)`
    artifactReading.value = true
    const bytes = new Uint8Array(await file.arrayBuffer())
    if (revision !== artifactReadRevision || !props.open) return
    let binary = ''
    const chunkSize = 0x8000
    for (let offset = 0; offset < bytes.length; offset += chunkSize) {
      binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize))
    }
    if (revision === artifactReadRevision && props.open) artifactBase64.value = btoa(binary)
  } catch {
    if (revision === artifactReadRevision && props.open) {
      artifactBase64.value = ''
      inputError.value = t('control.errors.releaseImport')
    }
  } finally {
    if (revision === artifactReadRevision) artifactReading.value = false
  }
}

function save() {
  if (props.saving || releaseReading.value || !manifest.value.trim() || !signature.value.trim()) return
  emit('save', {
    manifest: manifest.value,
    signature: signature.value.trim(),
    artifactBase64: artifactBase64.value,
  })
}
</script>

<style scoped>
.dialog-body { display: grid; gap: var(--space-4); }
.form-group { display: grid; gap: var(--space-2); }
.form-group label { font-size: var(--type-callout-size); font-weight: var(--weight-semibold); }
.form-group textarea { width: 100%; box-sizing: border-box; font-family: var(--font-mono); font-size: var(--type-caption-size); resize: vertical; }
.field-help, .dialog-error { margin: 0; color: var(--label-2); overflow-wrap: anywhere; }
.dialog-error { color: var(--danger); }
</style>
