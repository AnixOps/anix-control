<template>
  <div v-if="open" class="plugin-dialog-backdrop" @click.self="requestClose">
    <section
      class="plugin-dialog"
      data-testid="plugin-release-import-dialog"
      role="dialog"
      aria-modal="true"
      aria-labelledby="plugin-release-title"
      @keydown.esc.prevent="requestClose"
    >
      <header class="dialog-header">
        <h2 id="plugin-release-title">{{ t('control.releaseImport.title') }}</h2>
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
          <label for="plugin-release-manifest">{{ t('control.releaseImport.manifest') }}</label>
          <input type="file" accept="application/json,.json" :disabled="saving" @change="readTextFile($event, 'manifest')" />
          <textarea id="plugin-release-manifest" v-model="manifest" rows="10" spellcheck="false" :disabled="saving"></textarea>
        </div>
        <div class="form-group">
          <label for="plugin-release-signature">{{ t('control.releaseImport.signature') }}</label>
          <input type="file" accept="text/plain,.sig" :disabled="saving" @change="readTextFile($event, 'signature')" />
          <textarea id="plugin-release-signature" v-model="signature" rows="3" spellcheck="false" :disabled="saving"></textarea>
        </div>
        <div class="form-group">
          <label for="plugin-release-artifact">{{ t('control.releaseImport.artifact') }}</label>
          <input id="plugin-release-artifact" type="file" :disabled="saving" @change="readArtifactFile" />
          <p class="field-help">{{ artifactName || t('control.releaseImport.artifactOptional') }}</p>
        </div>
        <p v-if="inputError || error" class="dialog-error" role="alert">{{ inputError || error }}</p>
      </div>

      <footer class="dialog-footer">
        <button class="btn" type="button" :disabled="saving" @click="requestClose">{{ t('common.actions.cancel') }}</button>
        <button
          class="btn btn-primary"
          data-action="save-release"
          type="button"
          :disabled="saving || !manifest.trim() || !signature.trim()"
          @click="save"
        >
          {{ saving ? t('control.actions.importing') : t('control.actions.importRelease') }}
        </button>
      </footer>
    </section>
  </div>
</template>

<script setup>
import { nextTick, ref, watch } from 'vue'
import { X } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'

const props = defineProps({
  open: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  error: { type: String, default: '' },
})
const emit = defineEmits(['close', 'save'])
const { t } = useAppI18n()
const closeButton = ref(null)
const manifest = ref('')
const signature = ref('')
const artifactBase64 = ref('')
const artifactName = ref('')
const inputError = ref('')

watch(() => props.open, async (isOpen) => {
  if (!isOpen) return
  manifest.value = ''
  signature.value = ''
  artifactBase64.value = ''
  artifactName.value = ''
  inputError.value = ''
  await nextTick()
  closeButton.value?.focus()
}, { immediate: true })

function requestClose() {
  if (!props.saving) emit('close')
}

async function readTextFile(event, field) {
  const file = event.target.files?.[0]
  if (!file) return
  try {
    inputError.value = ''
    if (field === 'manifest') manifest.value = await file.text()
    if (field === 'signature') signature.value = await file.text()
  } catch {
    inputError.value = t('control.errors.releaseImport')
  }
}

async function readArtifactFile(event) {
  const file = event.target.files?.[0]
  if (!file) return
  try {
    inputError.value = ''
    artifactName.value = `${file.name} (${file.size} B)`
    const bytes = new Uint8Array(await file.arrayBuffer())
    let binary = ''
    const chunkSize = 0x8000
    for (let offset = 0; offset < bytes.length; offset += chunkSize) {
      binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize))
    }
    artifactBase64.value = btoa(binary)
  } catch {
    artifactBase64.value = ''
    inputError.value = t('control.errors.releaseImport')
  }
}

function save() {
  if (props.saving || !manifest.value.trim() || !signature.value.trim()) return
  emit('save', {
    manifest: manifest.value,
    signature: signature.value.trim(),
    artifactBase64: artifactBase64.value,
  })
}
</script>

<style scoped>
.plugin-dialog-backdrop { position: fixed; inset: 0; z-index: 1200; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(15, 23, 42, .62); }
.plugin-dialog { width: min(100%, 640px); max-height: calc(100dvh - 40px); overflow-y: auto; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface-color); color: var(--text-color); box-shadow: 0 18px 32px rgba(15, 23, 42, .2); }
.dialog-header, .dialog-footer { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid var(--border-color); }
.dialog-header h2 { margin: 0; font-size: 18px; line-height: 1.35; }
.dialog-body { display: grid; gap: 18px; padding: 20px; }
.form-group { display: grid; gap: 8px; }
.form-group label { font-size: 14px; font-weight: 600; }
.form-group textarea { width: 100%; box-sizing: border-box; border: 1px solid var(--border-color); border-radius: 6px; background: var(--bg-color); color: var(--text-color); font: 12px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; padding: 10px; resize: vertical; }
.field-help, .dialog-error { margin: 0; color: var(--text-secondary); overflow-wrap: anywhere; }
.dialog-error { color: var(--error-color); }
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
