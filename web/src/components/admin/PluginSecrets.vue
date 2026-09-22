<template>
  <section class="secret-panel" aria-labelledby="secret-panel-title">
    <div class="secret-toolbar">
      <div>
        <h2 id="secret-panel-title">{{ t('control.secrets.title') }}</h2>
        <p>{{ t('control.secrets.subtitle') }}</p>
      </div>
      <div class="secret-actions">
        <button class="btn btn-sm" type="button" :disabled="loading" @click="loadSecrets">{{ t('control.actions.refresh') }}</button>
        <button id="new-plugin-secret" class="btn btn-primary btn-sm" type="button" @click="openCreate">{{ t('control.secrets.new') }}</button>
      </div>
    </div>

    <p v-if="error" class="error-message" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice-message" role="status">{{ notice }}</p>

    <div class="table-container">
      <table class="data-table">
        <thead><tr><th>{{ t('control.secrets.name') }}</th><th>{{ t('control.secrets.activeVersion') }}</th><th>{{ t('control.secrets.updated') }}</th><th>{{ t('control.table.actions') }}</th></tr></thead>
        <tbody>
          <tr v-if="loading"><td colspan="4">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="secret in secrets" v-else :key="secret.id">
            <td><span class="primary-cell">{{ secret.name }}</span><code class="secondary-cell">{{ secret.id }}</code></td>
            <td><code>v{{ secret.active_version }}</code></td>
            <td>{{ formatDate(secret.updated_at) }}</td>
            <td><div class="row-actions">
              <button class="btn btn-sm" type="button" @click="selectSecret(secret.id)">{{ t('control.secrets.inspect') }}</button>
              <button class="btn btn-sm" type="button" @click="openVersion(secret.id)">{{ t('control.secrets.addVersion') }}</button>
              <button class="btn btn-sm btn-danger" type="button" @click="removeSecret(secret)">{{ t('common.actions.delete') }}</button>
            </div></td>
          </tr>
          <tr v-if="!loading && secrets.length === 0"><td colspan="4" class="empty-row">{{ t('control.empty.secrets') }}</td></tr>
        </tbody>
      </table>
    </div>

    <section v-if="selected" class="secret-detail" aria-live="polite">
      <div class="secret-detail-header">
        <div><h3>{{ selected.name }}</h3><code>{{ selected.id }}</code></div>
        <button class="btn btn-sm" type="button" @click="selected = null">{{ t('common.actions.close') }}</button>
      </div>
      <p v-if="selected.description" class="secret-description">{{ selected.description }}</p>
      <div class="secret-detail-grid">
        <div>
          <h4>{{ t('control.secrets.versions') }}</h4>
          <div v-for="version in selected.versions || []" :key="version.version" class="version-block">
            <div class="version-heading">
              <strong>v{{ version.version }}</strong>
              <span>{{ version.key_id }}</span>
              <button v-if="version.version !== selected.active_version" class="btn btn-sm btn-danger" type="button" @click="removeVersion(version.version)">{{ t('common.actions.delete') }}</button>
            </div>
            <div v-for="file in version.files || []" :key="file.name" class="secret-file">
              <div><code>{{ file.name }}</code><span>{{ formatBytes(file.size) }} · {{ shortHash(file.sha256) }}</span></div>
              <button class="btn btn-sm" type="button" @click="copyReference(selected.id, version.version, file.name)">{{ t('control.secrets.copyReference') }}</button>
            </div>
          </div>
        </div>
        <div>
          <h4>{{ t('control.secrets.audit') }}</h4>
          <div class="audit-list">
            <div v-for="entry in audit" :key="entry.id" class="audit-entry">
              <span><strong>{{ entry.action }}</strong> · {{ entry.outcome }}</span>
              <span>{{ formatDate(entry.created_at) }}</span>
              <code v-if="entry.operation_id">{{ entry.operation_id }}</code>
            </div>
            <p v-if="audit.length === 0" class="empty-copy">{{ t('control.secrets.noAudit') }}</p>
          </div>
        </div>
      </div>
    </section>

    <div v-if="editor.open" class="modal-overlay" @click.self="closeEditor">
      <section class="modal" role="dialog" aria-modal="true" aria-labelledby="secret-editor-title">
        <div class="modal-header">
          <h3 id="secret-editor-title">{{ editor.mode === 'create' ? t('control.secrets.createTitle') : t('control.secrets.versionTitle', { id: editor.secretID }) }}</h3>
          <button class="btn btn-ghost close-btn" type="button" :aria-label="t('common.actions.close')" @click="closeEditor">×</button>
        </div>
        <div class="modal-body">
          <div v-if="editor.mode === 'create'" class="form-group"><label for="secret-id">{{ t('control.secrets.id') }}</label><input id="secret-id" v-model.trim="editor.id" autocomplete="off" /></div>
          <div v-if="editor.mode === 'create'" class="form-group"><label for="secret-name">{{ t('control.secrets.name') }}</label><input id="secret-name" v-model.trim="editor.name" autocomplete="off" /></div>
          <div v-if="editor.mode === 'create'" class="form-group"><label for="secret-description">{{ t('control.table.description') }}</label><textarea id="secret-description" v-model.trim="editor.description" rows="3"></textarea></div>
          <div class="form-group">
            <label for="secret-files">{{ t('control.secrets.files') }}</label>
            <input :key="editor.fileInputKey" id="secret-files" type="file" multiple @change="selectFiles" />
            <p class="field-help">{{ t('control.secrets.fileLimits') }}</p>
          </div>
          <ul v-if="editor.files.length" class="selected-files"><li v-for="file in editor.files" :key="file.name"><code>{{ file.name }}</code><span>{{ formatBytes(file.size) }}</span></li></ul>
        </div>
        <div class="modal-footer">
          <button class="btn" type="button" :disabled="editor.saving" @click="closeEditor">{{ t('common.actions.cancel') }}</button>
          <button id="save-plugin-secret" class="btn btn-primary" type="button" :disabled="!editorValid || editor.saving" @click="saveSecret">{{ editor.saving ? t('control.actions.saving') : t('common.actions.save') }}</button>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  createKernelPluginSecret,
  createKernelPluginSecretVersion,
  deleteKernelPluginSecret,
  deleteKernelPluginSecretVersion,
  getKernelPluginSecret,
  getKernelPluginSecretAudit,
  getKernelPluginSecrets
} from '@/api/kernel'

const MAX_FILES = 16
const MAX_FILE_BYTES = 1 << 20
const MAX_TOTAL_BYTES = 4 << 20
const { t, formatDateTime } = useAppI18n()
const loading = ref(false)
const secrets = ref([])
const selected = ref(null)
const audit = ref([])
const error = ref('')
const notice = ref('')
const editor = reactive({ open: false, mode: 'create', secretID: '', id: '', name: '', description: '', files: [], saving: false, fileInputKey: 0 })

const editorValid = computed(() => editor.files.length > 0 && (editor.mode === 'version' || Boolean(editor.id && editor.name)))

function errorMessage(cause, fallback) {
  const response = cause?.response?.data
  return response?.error?.message || response?.message || cause?.message || t(fallback)
}

function formatDate(value) {
  return value ? formatDateTime(value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }) : '-'
}

function formatBytes(value) {
  const bytes = Number(value || 0)
  return bytes < 1024 ? `${bytes} B` : `${(bytes / 1024).toFixed(bytes < 10240 ? 1 : 0)} KiB`
}

function shortHash(value) {
  return value ? `${value.slice(0, 12)}…` : '-'
}

async function loadSecrets() {
  loading.value = true
  error.value = ''
  try {
    const rows = await getKernelPluginSecrets()
    secrets.value = Array.isArray(rows) ? rows : []
    if (selected.value && !secrets.value.some(item => item.id === selected.value.id)) selected.value = null
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.secretLoad')
  } finally {
    loading.value = false
  }
}

async function selectSecret(secretID) {
  error.value = ''
  try {
    const [detail, entries] = await Promise.all([getKernelPluginSecret(secretID), getKernelPluginSecretAudit(secretID)])
    selected.value = detail
    audit.value = Array.isArray(entries) ? entries : []
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.secretLoad')
  }
}

function resetEditor(mode, secretID = '') {
  Object.assign(editor, { open: true, mode, secretID, id: '', name: '', description: '', files: [], saving: false, fileInputKey: editor.fileInputKey + 1 })
}

function openCreate() { resetEditor('create') }
function openVersion(secretID) { resetEditor('version', secretID) }
function closeEditor() { if (!editor.saving) editor.open = false }

function selectFiles(event) {
  const files = Array.from(event.target?.files || [])
  const total = files.reduce((sum, file) => sum + file.size, 0)
  if (files.length === 0 || files.length > MAX_FILES || files.some(file => file.size === 0 || file.size > MAX_FILE_BYTES) || total > MAX_TOTAL_BYTES) {
    editor.files = []
    error.value = t('control.errors.secretFiles')
    editor.fileInputKey++
    return
  }
  editor.files = files
  error.value = ''
}

function bytesToBase64(buffer) {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (let offset = 0; offset < bytes.length; offset += 0x8000) binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000))
  return btoa(binary)
}

async function encodedFiles() {
  return Promise.all(editor.files.map(async file => ({ name: file.name, content_base64: bytesToBase64(await file.arrayBuffer()) })))
}

async function saveSecret() {
  if (!editorValid.value) return
  editor.saving = true
  error.value = ''
  notice.value = ''
  try {
    const files = await encodedFiles()
    const detail = editor.mode === 'create'
      ? await createKernelPluginSecret({ id: editor.id, name: editor.name, description: editor.description, files })
      : await createKernelPluginSecretVersion(editor.secretID, files)
    editor.open = false
    editor.files = []
    await loadSecrets()
    await selectSecret(detail.id || editor.secretID)
    notice.value = t(editor.mode === 'create' ? 'control.messages.secretCreated' : 'control.messages.secretVersionCreated')
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.secretSave')
  } finally {
    editor.saving = false
  }
}

async function removeVersion(version) {
  if (!selected.value || !confirm(t('control.secrets.deleteVersionConfirm', { version }))) return
  error.value = ''
  try {
    await deleteKernelPluginSecretVersion(selected.value.id, version)
    await loadSecrets()
    await selectSecret(selected.value.id)
    notice.value = t('control.messages.secretVersionDeleted')
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.secretDelete')
  }
}

async function removeSecret(secret) {
  if (!confirm(t('control.secrets.deleteConfirm', { id: secret.id }))) return
  error.value = ''
  try {
    await deleteKernelPluginSecret(secret.id)
    if (selected.value?.id === secret.id) selected.value = null
    await loadSecrets()
    notice.value = t('control.messages.secretDeleted')
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.secretDelete')
  }
}

async function copyReference(secretID, version, name) {
  const reference = `secret://${secretID}@${version}/${name}`
  try {
    await navigator.clipboard.writeText(reference)
    notice.value = t('control.messages.secretReferenceCopied')
  } catch {
    error.value = t('control.errors.secretCopy')
  }
}

onMounted(loadSecrets)
</script>

<style scoped>
.secret-panel { display: grid; gap: 16px; }
.secret-toolbar, .secret-detail-header, .version-heading, .secret-file { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.secret-toolbar h2, .secret-detail h3, .secret-detail h4 { margin: 0; }
.secret-toolbar p, .secret-description { margin: 4px 0 0; color: var(--text-secondary); }
.secret-actions, .row-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.secret-detail { border-top: 1px solid var(--border-color); padding-top: 18px; }
.secret-detail-grid { display: grid; grid-template-columns: minmax(0, 3fr) minmax(260px, 2fr); gap: 24px; margin-top: 16px; }
.version-block { border-bottom: 1px solid var(--border-color); padding: 12px 0; }
.version-heading span, .secret-file span, .audit-entry span:last-of-type { color: var(--text-secondary); font-size: 12px; }
.secret-file { padding: 8px 0 0 16px; }
.secret-file div, .audit-entry { display: grid; gap: 3px; min-width: 0; }
.audit-list { display: grid; gap: 10px; }
.audit-entry { border-bottom: 1px solid var(--border-color); padding: 0 0 10px; }
.audit-entry code { overflow-wrap: anywhere; }
.selected-files { margin: 0; padding: 0; list-style: none; display: grid; gap: 8px; }
.selected-files li { display: flex; justify-content: space-between; gap: 12px; }
.empty-copy { color: var(--text-secondary); }
@media (max-width: 760px) {
  .secret-toolbar, .secret-detail-header, .secret-file { align-items: flex-start; flex-direction: column; }
  .secret-detail-grid { grid-template-columns: 1fr; }
  .secret-actions { width: 100%; }
  .secret-actions .btn { flex: 1; }
}
</style>
