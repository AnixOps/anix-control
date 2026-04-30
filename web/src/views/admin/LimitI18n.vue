<template>
  <div class="limit-page">
    <div class="toolbar">
      <div class="toolbar-copy">
        <p class="eyebrow">{{ t('runtime.limitPage.heroEyebrow') }}</p>
        <h2>{{ t('runtime.limitPage.title') }}</h2>
      </div>
      <div class="toolbar-actions">
        <button class="btn btn-primary" @click="openCreateModal">{{ t('runtime.limitPage.actions.create') }}</button>
      </div>
    </div>
    <ForwardSuiteNav />
    <p class="text-secondary small runtime-note">{{ t('runtime.limitPage.note') }}</p>
    <div class="runtime-context-bar">
      <span class="tag tag-primary">{{ runtimeModeLabel }}</span>
      <span class="runtime-context-summary">{{ runtimeModeSummary }}</span>
      <div class="runtime-context-links">
        <router-link class="btn btn-secondary btn-sm" to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</router-link>
        <router-link class="btn btn-secondary btn-sm" to="/admin/forward/nodex">{{ t('forwardSuite.nav.nodeXRuntime') }}</router-link>
      </div>
    </div>

    <div v-if="feedback.message" :class="['feedback', `feedback-${feedback.type}`]">
      <span>{{ feedback.message }}</span>
      <button class="feedback-close" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="clearFeedback">×</button>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>{{ t('runtime.limitPage.loading') }}</span>
    </div>

    <template v-else>
      <section v-if="limits.length" class="card-grid">
        <article v-for="item in limits" :key="item.id" class="limit-card">
          <div class="card-head">
            <div class="card-title">
              <h3>{{ formatRuleName(item) }}</h3>
              <p>{{ formatTunnel(item) }}</p>
            </div>
            <div class="card-head-actions">
              <span :class="['tag', item.status === 1 ? 'tag-success' : 'tag-danger']">
                {{ item.status === 1 ? t('runtime.limitPage.status.active') : t('runtime.limitPage.status.error') }}
              </span>
            </div>
          </div>

          <div class="meta-list">
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.limitPage.cards.speed') }}</span>
              <strong>{{ formatSpeed(item.speed) }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.limitPage.cards.updatedAt') }}</span>
              <strong>{{ formatTime(item.updatedTime || item.createdTime) }}</strong>
            </div>
          </div>

          <div class="card-actions">
            <button class="btn btn-secondary btn-sm" @click="openEditModal(item)">{{ t('runtime.limitPage.actions.edit') }}</button>
            <button class="btn btn-secondary btn-sm danger-text" @click="openDeleteModal(item)">{{ t('runtime.limitPage.actions.delete') }}</button>
          </div>
        </article>
      </section>

      <section v-else class="empty-state">
        <h3>{{ t('runtime.limitPage.empty.title') }}</h3>
        <p>{{ t('runtime.limitPage.empty.text') }}</p>
        <button class="btn btn-primary" @click="openCreateModal">{{ t('runtime.limitPage.actions.createNow') }}</button>
      </section>
    </template>

    <div v-if="showFormModal" class="modal-overlay" @click.self="closeFormModal">
      <div class="modal modal-md">
        <div class="modal-header">
          <div>
            <p class="eyebrow">{{ isEditMode ? t('runtime.limitPage.formModal.titleEdit') : t('runtime.limitPage.formModal.titleCreate') }}</p>
            <h3>{{ isEditMode ? t('runtime.limitPage.formModal.titleEdit') : t('runtime.limitPage.formModal.titleCreate') }}</h3>
          </div>
          <button class="modal-close" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="closeFormModal">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('runtime.limitPage.formModal.fields.name') }}</label>
            <input v-model.trim="form.name" type="text" :placeholder="t('runtime.limitPage.formModal.placeholders.name')" />
            <p v-if="formError" class="form-error">{{ formError }}</p>
          </div>
          <div class="form-group">
            <label>{{ t('runtime.limitPage.formModal.fields.speed') }}</label>
            <input v-model.number="form.speed" type="number" min="1" step="1" :placeholder="t('runtime.limitPage.formModal.placeholders.speed')" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.limitPage.formModal.fields.tunnel') }}</label>
            <select v-model.number="form.tunnelId">
              <option :value="0">{{ t('runtime.limitPage.formModal.placeholders.tunnel') }}</option>
              <option v-for="t in tunnels" :key="t.id" :value="t.id">
                {{ t.name || t('runtime.limitPage.values.tunnelFallback', { id: t.id }) }}
              </option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" :disabled="formSubmitting" @click="closeFormModal">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" :disabled="formSubmitting" @click="submitForm">
            {{ formSubmitting ? t('runtime.limitPage.formModal.submitting') : (isEditMode ? t('runtime.limitPage.formModal.submitUpdate') : t('runtime.limitPage.formModal.submitCreate')) }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="showDeleteModal" class="modal-overlay" @click.self="closeDeleteModal">
      <div class="modal">
        <div class="modal-header">
          <div>
            <p class="eyebrow">{{ t('runtime.limitPage.deleteModal.eyebrow') }}</p>
            <h3>{{ t('runtime.limitPage.deleteModal.title') }}</h3>
          </div>
          <button class="modal-close" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="closeDeleteModal">×</button>
        </div>
        <div class="modal-body">
          <p class="modal-copy">{{ t('runtime.limitPage.deleteModal.confirmText', { name: deletingItem?.name || t('runtime.limitPage.values.ruleFallback', { id: deletingItem?.id || '-' }) }) }}</p>
          <p class="hint">{{ t('runtime.limitPage.deleteModal.hint') }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" :disabled="deleteSubmitting" @click="closeDeleteModal">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary danger" :disabled="deleteSubmitting" @click="confirmDelete">
            {{ deleteSubmitting ? t('runtime.limitPage.deleteModal.deleting') : t('runtime.limitPage.deleteModal.confirmDelete') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  createSpeedLimit,
  deleteSpeedLimit,
  getSpeedLimitList,
  getSpeedLimitTunnels,
  getSystemConfig,
  updateSpeedLimit
} from '@/api/admin'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'

const { t, formatDateTime, translateLiteral } = useAppI18n()

const loading = ref(false)
const limits = ref([])
const tunnels = ref([])

const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeNodeXMode = ref(false)
const runtimeBackend = ref('nftables_ansible')
const runtimeModeLabel = computed(() => (
  runtimeNodeXMode.value
    ? t('runtime.limitPage.modeLabelNodeX')
    : t('runtime.limitPage.modeLabelLocal', { backend: humanizeForwardRuntimeBackend(t, runtimeBackend.value) })
))
const runtimeModeSummary = computed(() => (
  runtimeNodeXMode.value
    ? t('runtime.limitPage.modeSummaryNodeX')
    : t('runtime.limitPage.modeSummaryLocal')
))

const showFormModal = ref(false)
const formSubmitting = ref(false)
const formError = ref('')
const form = ref(newForm())

const showDeleteModal = ref(false)
const deleteSubmitting = ref(false)
const deletingItem = ref(null)

const feedback = ref({ type: 'info', message: '' })
let feedbackTimer = null

function newForm() {
  return { id: null, name: '', speed: 100, tunnelId: 0 }
}

const isEditMode = computed(() => Number(form.value.id || 0) > 0)
const selectedTunnel = computed(() => tunnels.value.find(item => Number(item.id) === Number(form.value.tunnelId || 0)) || null)

function parseRuntimeBoolean(value) {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value !== 0
  const normalized = String(value ?? '').trim().toLowerCase()
  if (!normalized) return null
  if (['1', 'true', 'yes', 'on'].includes(normalized)) return true
  if (['0', 'false', 'no', 'off'].includes(normalized)) return false
  return null
}

async function loadRuntimeMode() {
  let explicitMode = null
  try {
    const res = await getSystemConfig(runtimeNodeXModeKey)
    explicitMode = parseRuntimeBoolean(res.data?.value)
  } catch (error) {
    console.error('get forward runtime NodeX mode failed:', error)
  }
  try {
    const res = await getSystemConfig(runtimeBackendKey)
    runtimeBackend.value = String(res.data?.value || 'nftables_ansible').toLowerCase() || 'nftables_ansible'
    runtimeNodeXMode.value = explicitMode === null ? runtimeBackend.value === 'gost' : explicitMode
  } catch (error) {
    console.error('get forward runtime backend failed:', error)
    if (explicitMode !== null) runtimeNodeXMode.value = explicitMode
  }
}

function setFeedback(type, message) {
  feedback.value = { type, message }
  if (feedbackTimer) clearTimeout(feedbackTimer)
  feedbackTimer = setTimeout(() => { feedback.value.message = ''; feedbackTimer = null }, 3600)
}

function clearFeedback() {
  if (feedbackTimer) { clearTimeout(feedbackTimer); feedbackTimer = null }
  feedback.value.message = ''
}

function formatErrorMessage(error, fallbackKey) {
  return translateLiteral(error?.response?.data?.msg || error?.response?.data?.error || error?.message || t(fallbackKey))
}

function unwrapData(res, fallbackKey) {
  if (!res) return null
  if (typeof res.code === 'number') {
    if (res.code !== 0) throw new Error(res.msg || t(fallbackKey))
    return res.data
  }
  return res.data
}

function normalizeTunnels(input) {
  if (!Array.isArray(input)) return []
  return input.map(item => {
    const id = Number(item?.id || 0)
    if (!id) return null
    return { id, name: item?.name || item?.tunnelName || '' }
  }).filter(Boolean)
}

function normalizeLimits(input) {
  if (!Array.isArray(input)) return []
  return input.map(item => {
    const id = Number(item?.id || 0)
    if (!id) return null
    return {
      id,
      name: item?.name || '',
      speed: Number(item?.speed || 0),
      tunnelId: Number(item?.tunnelId ?? item?.tunnel_id ?? 0),
      tunnelName: item?.tunnelName || item?.tunnel_name || '',
      status: Number(item?.status ?? 1),
      createdTime: item?.createdTime ?? item?.created_at ?? item?.createdAt ?? 0,
      updatedTime: item?.updatedTime ?? item?.updated_at ?? item?.updatedAt ?? 0
    }
  }).filter(Boolean)
}

function hydrateLimitTunnelName(items) {
  if (!Array.isArray(items) || items.length === 0) return []
  const tunnelMap = new Map(tunnels.value.map(item => [Number(item.id), item.name || t('runtime.limitPage.values.tunnelFallback', { id: item.id })]))
  return items.map(item => ({ ...item, tunnelName: item.tunnelName || tunnelMap.get(Number(item.tunnelId)) || '' }))
}

async function fetchTunnels() {
  const res = await getSpeedLimitTunnels()
  const payload = unwrapData(res, 'runtime.limitPage.messages.fetchTunnelsFailed')
  const list = Array.isArray(payload) ? payload : (payload?.list || [])
  tunnels.value = normalizeTunnels(list)
}

async function fetchLimits() {
  const res = await getSpeedLimitList()
  const payload = unwrapData(res, 'runtime.limitPage.messages.fetchRulesFailed')
  const list = Array.isArray(payload) ? payload : (payload?.list || [])
  limits.value = hydrateLimitTunnelName(normalizeLimits(list))
}

async function refreshAll() {
  loading.value = true
  try {
    await fetchTunnels()
    await fetchLimits()
  } catch (error) {
    setFeedback('error', formatErrorMessage(error, 'runtime.limitPage.messages.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreateModal() {
  form.value = newForm()
  formError.value = ''
  showFormModal.value = true
}

function openEditModal(item) {
  form.value = {
    id: item.id,
    name: item.name || '',
    speed: Number(item.speed || 0),
    tunnelId: Number(item.tunnelId || 0)
  }
  formError.value = ''
  showFormModal.value = true
}

function closeFormModal() {
  if (formSubmitting.value) return
  showFormModal.value = false
  formError.value = ''
}

function validateForm() {
  if (!form.value.name) return t('runtime.limitPage.messages.nameRequired')
  if (form.value.name.length < 2 || form.value.name.length > 50) return t('runtime.limitPage.messages.nameLength')
  if (!Number.isFinite(Number(form.value.speed)) || Number(form.value.speed) <= 0) return t('runtime.limitPage.messages.speedInvalid')
  if (!Number.isFinite(Number(form.value.tunnelId)) || Number(form.value.tunnelId) <= 0) return t('runtime.limitPage.messages.tunnelRequired')
  if (!selectedTunnel.value?.name) return t('runtime.limitPage.messages.tunnelMissing')
  return ''
}

async function submitForm() {
  const invalid = validateForm()
  if (invalid) { formError.value = invalid; return }

  formSubmitting.value = true
  formError.value = ''
  try {
    const payload = {
      name: form.value.name,
      speed: Number(form.value.speed),
      tunnelId: Number(form.value.tunnelId),
      tunnelName: selectedTunnel.value.name
    }

    if (isEditMode.value) {
      await updateSpeedLimit({ id: Number(form.value.id), ...payload })
    } else {
      await createSpeedLimit(payload)
    }

    await fetchLimits()
    showFormModal.value = false
    setFeedback('success', isEditMode.value ? t('runtime.limitPage.messages.updated') : t('runtime.limitPage.messages.created'))
  } catch (error) {
    formError.value = formatErrorMessage(error, 'runtime.limitPage.messages.submitFailed')
  } finally {
    formSubmitting.value = false
  }
}

function openDeleteModal(item) {
  deletingItem.value = item
  showDeleteModal.value = true
}

function closeDeleteModal() {
  if (deleteSubmitting.value) return
  showDeleteModal.value = false
  deletingItem.value = null
}

async function confirmDelete() {
  if (!deletingItem.value?.id) return
  deleteSubmitting.value = true
  try {
    await deleteSpeedLimit(Number(deletingItem.value.id))
    await fetchLimits()
    closeDeleteModal()
    setFeedback('success', t('runtime.limitPage.messages.deleted'))
  } catch (error) {
    setFeedback('error', formatErrorMessage(error, 'runtime.limitPage.messages.deleteFailed'))
  } finally {
    deleteSubmitting.value = false
  }
}

function formatSpeed(value) {
  const speed = Number(value || 0)
  if (!Number.isFinite(speed) || speed <= 0) return t('runtime.limitPage.values.unlimited')
  return `${speed} Mbps`
}

function formatTunnel(item) {
  if (item.tunnelName) return item.tunnelName
  if (item.tunnelId) return t('runtime.limitPage.values.tunnelFallback', { id: item.tunnelId })
  return '-'
}

function formatRuleName(item) {
  if (item?.name) return item.name
  return t('runtime.limitPage.values.ruleFallback', { id: item?.id || '-' })
}

function formatTime(value) {
  const numeric = Number(value || 0)
  if (!Number.isFinite(numeric) || numeric <= 0) return '-'
  return formatDateTime(numeric, { second: '2-digit' }) || '-'
}

onMounted(async () => {
  await loadRuntimeMode()
  refreshAll()
})
</script>

<style scoped>
.limit-page {
  display: grid;
  gap: 20px;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 22px 24px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.14), transparent 32%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.03), transparent 60%),
    var(--surface-color);
}

.toolbar-copy h2 {
  margin: 4px 0 0;
  font-size: 26px;
  line-height: 1.1;
}

.eyebrow {
  margin: 0;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--text-secondary);
}

.toolbar-actions {
  display: flex;
  gap: 12px;
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  padding: 10px 16px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: var(--transition);
}

.btn:disabled { opacity: 0.6; cursor: not-allowed; }

.btn-primary {
  background: var(--forward-accent, #2563eb);
  color: #fff;
  box-shadow: 0 14px 32px rgba(37, 99, 235, 0.2);
}

.btn-secondary {
  background: var(--surface-color);
  color: var(--text-color);
  border-color: var(--border-color);
}

.btn-sm { padding: 8px 12px; font-size: 12px; }

.danger { background: #dc2626; }
.danger:hover { background: #b91c1c; }
.danger-text { color: #dc2626; }

.feedback {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-radius: var(--radius-md);
  border: 1px solid transparent;
}

.feedback-success {
  background: rgba(16, 185, 129, 0.14);
  color: #047857;
  border-color: rgba(16, 185, 129, 0.24);
}

.feedback-error {
  background: rgba(239, 68, 68, 0.14);
  color: #b91c1c;
  border-color: rgba(239, 68, 68, 0.25);
}

.feedback-close, .modal-close {
  background: transparent;
  border: none;
  color: inherit;
  font-size: 22px;
  cursor: pointer;
}

.loading-state {
  min-height: 300px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  border: 1px dashed rgba(37, 99, 235, 0.2);
  border-radius: var(--radius-lg);
  background: linear-gradient(180deg, rgba(37, 99, 235, 0.04), transparent 55%), var(--surface-color);
  color: var(--text-secondary);
}

.spinner {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  border: 3px solid rgba(37, 99, 235, 0.2);
  border-top-color: var(--forward-accent, #2563eb);
  animation: spin 0.8s linear infinite;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.limit-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px;
  border-radius: 20px;
  border: 1px solid var(--border-color);
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.04), transparent 60%),
    var(--surface-color);
  box-shadow: 0 14px 30px rgba(15, 23, 42, 0.08);
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.limit-card:hover {
  transform: translateY(-3px);
  border-color: rgba(37, 99, 235, 0.24);
  box-shadow: 0 20px 38px rgba(15, 23, 42, 0.12);
}

.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.card-title { min-width: 0; }

.card-title h3 {
  margin: 0;
  font-size: 15px;
}

.card-title p {
  margin: 6px 0 0;
  color: var(--text-secondary);
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.card-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  padding-top: 4px;
}

.meta-list {
  display: grid;
  gap: 12px;
}

.meta-item {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
  border-radius: 16px;
  background: rgba(148, 163, 184, 0.08);
}

.meta-item strong { word-break: break-all; }

.tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 28px;
  padding: 6px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  background: rgba(148, 163, 184, 0.16);
  color: var(--text-secondary);
}

.tag-success {
  background: rgba(16, 185, 129, 0.14);
  color: #047857;
}

.tag-danger {
  background: rgba(239, 68, 68, 0.14);
  color: #b91c1c;
}

.tag-primary {
  background: rgba(37, 99, 235, 0.12);
  color: #1d4ed8;
}

.empty-state {
  padding: 42px 24px;
  text-align: center;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  background: var(--surface-color);
}

.empty-state h3 { margin: 0 0 8px; }
.empty-state p { margin: 0 0 20px; color: var(--text-secondary); }

.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.7);
  backdrop-filter: blur(6px);
}

.modal {
  width: min(560px, 100%);
  max-height: calc(100vh - 48px);
  display: flex;
  flex-direction: column;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: 24px;
  overflow: hidden;
  box-shadow: 0 28px 70px rgba(15, 23, 42, 0.28);
}

.modal-md { width: min(640px, 100%); }

.modal-header, .modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 22px;
  border-bottom: 1px solid var(--border-color);
}

.modal-footer {
  justify-content: flex-end;
  border-top: 1px solid var(--border-color);
  border-bottom: none;
}

.modal-body {
  padding: 22px;
  overflow: auto;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 18px;
}

.form-group label {
  font-size: 13px;
  font-weight: 700;
}

.form-group input, .form-group select {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.04);
  color: var(--text-color);
  font-size: 14px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background 0.2s ease;
}

.form-group input:focus, .form-group select:focus {
  outline: none;
  border-color: rgba(37, 99, 235, 0.4);
  box-shadow: 0 0 0 4px rgba(37, 99, 235, 0.12);
  background: rgba(37, 99, 235, 0.03);
}

.form-error {
  margin: 0;
  font-size: 12px;
  color: #dc2626;
}

.modal-copy { margin: 0; line-height: 1.7; }
.hint { margin: 6px 0 0; color: var(--text-secondary); font-size: 13px; }
.text-secondary { color: var(--text-secondary); }
.small { font-size: 13px; }

.runtime-context-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.runtime-context-summary {
  font-size: 12px;
  color: var(--text-secondary);
}

.runtime-context-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
}

@keyframes spin { to { transform: rotate(360deg); } }

@media (max-width: 768px) {
  .toolbar { flex-direction: column; align-items: stretch; }
  .card-grid { grid-template-columns: 1fr; }
  .modal-overlay { padding: 12px; }
  .form-group { margin-bottom: 14px; }
}
</style>
