<template>
  <div class="limit-page">
    <div class="page-header">
      <div>
        <h1>{{ t('runtime.limitPage.title') }}</h1>
        <p class="text-secondary">{{ t('runtime.limitPage.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <button class="btn-secondary" :disabled="loading" @click="refreshAll">{{ t('runtime.limitPage.actions.refresh') }}</button>
        <button @click="openCreateModal">{{ t('runtime.limitPage.actions.create') }}</button>
      </div>
    </div>

    <ForwardSuiteNav />

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>{{ t('runtime.limitPage.loading') }}</span>
    </div>

    <div v-else-if="limits.length === 0" class="empty-state">
      <div class="empty-icon">L</div>
      <h3>{{ t('runtime.limitPage.empty.title') }}</h3>
      <p class="text-secondary">{{ t('runtime.limitPage.empty.text') }}</p>
      <button @click="openCreateModal">{{ t('runtime.limitPage.actions.createNow') }}</button>
    </div>

    <div v-else class="card-grid">
      <article v-for="item in limits" :key="item.id" class="limit-card">
        <div class="card-head">
          <div class="title-wrap">
            <h3>{{ formatRuleName(item) }}</h3>
            <span :class="['status-pill', item.status === 1 ? 'status-active' : 'status-disabled']">
              {{ item.status === 1 ? t('runtime.limitPage.status.active') : t('runtime.limitPage.status.error') }}
            </span>
          </div>
          <div class="card-actions">
            <button class="btn-sm btn-ghost" @click="openEditModal(item)">{{ t('runtime.limitPage.actions.edit') }}</button>
            <button class="btn-sm btn-ghost btn-danger" @click="openDeleteModal(item)">{{ t('runtime.limitPage.actions.delete') }}</button>
          </div>
        </div>

        <div class="kv-list">
          <div class="kv-item">
            <span class="k">{{ t('runtime.limitPage.cards.speed') }}</span>
            <span class="v">{{ formatSpeed(item.speed) }}</span>
          </div>
          <div class="kv-item">
            <span class="k">{{ t('runtime.limitPage.cards.tunnel') }}</span>
            <span class="v">{{ formatTunnel(item) }}</span>
          </div>
          <div class="kv-item">
            <span class="k">{{ t('runtime.limitPage.cards.updatedAt') }}</span>
            <span class="v">{{ formatTime(item.updatedTime || item.createdTime) }}</span>
          </div>
        </div>
      </article>
    </div>

    <div v-if="showFormModal" class="modal-overlay" @click.self="closeFormModal">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ isEditMode ? t('runtime.limitPage.formModal.titleEdit') : t('runtime.limitPage.formModal.titleCreate') }}</h3>
          <button class="close-btn" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="closeFormModal">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('runtime.limitPage.formModal.fields.name') }}</label>
            <input v-model.trim="form.name" type="text" :placeholder="t('runtime.limitPage.formModal.placeholders.name')" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.limitPage.formModal.fields.speed') }}</label>
            <input v-model.number="form.speed" type="number" min="1" step="1" :placeholder="t('runtime.limitPage.formModal.placeholders.speed')" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.limitPage.formModal.fields.tunnel') }}</label>
            <select v-model.number="form.tunnelId">
              <option :value="0">{{ t('runtime.limitPage.formModal.placeholders.tunnel') }}</option>
              <option v-for="item in tunnels" :key="item.id" :value="item.id">
                {{ item.name || t('runtime.limitPage.values.tunnelFallback', { id: item.id }) }}
              </option>
            </select>
          </div>
          <p v-if="formError" class="error-msg">{{ formError }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" :disabled="formSubmitting" @click="closeFormModal">{{ t('common.actions.cancel') }}</button>
          <button :disabled="formSubmitting" @click="submitForm">
            {{ formSubmitting ? t('runtime.limitPage.formModal.submitting') : (isEditMode ? t('runtime.limitPage.formModal.submitUpdate') : t('runtime.limitPage.formModal.submitCreate')) }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="showDeleteModal" class="modal-overlay" @click.self="closeDeleteModal">
      <div class="modal modal-sm">
        <div class="modal-header">
          <h3>{{ t('runtime.limitPage.deleteModal.title') }}</h3>
          <button class="close-btn" :aria-label="t('common.actions.close')" :title="t('common.actions.close')" @click="closeDeleteModal">x</button>
        </div>
        <div class="modal-body">
          <p class="delete-copy">
            {{ t('runtime.limitPage.deleteModal.confirmText', { name: deletingItem?.name || t('runtime.limitPage.values.ruleFallback', { id: deletingItem?.id || '-' }) }) }}
          </p>
          <p class="text-secondary">{{ t('runtime.limitPage.deleteModal.hint') }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" :disabled="deleteSubmitting" @click="closeDeleteModal">{{ t('common.actions.cancel') }}</button>
          <button class="btn-danger" :disabled="deleteSubmitting" @click="confirmDelete">
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
  updateSpeedLimit
} from '@/api/admin'
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'

const { t, formatDateTime, translateLiteral } = useAppI18n()

const loading = ref(false)
const limits = ref([])
const tunnels = ref([])

const showFormModal = ref(false)
const formSubmitting = ref(false)
const formError = ref('')
const form = ref(newForm())

const showDeleteModal = ref(false)
const deleteSubmitting = ref(false)
const deletingItem = ref(null)

function newForm() {
  return {
    id: null,
    name: '',
    speed: 100,
    tunnelId: 0
  }
}

const isEditMode = computed(() => Number(form.value.id || 0) > 0)
const selectedTunnel = computed(() => tunnels.value.find(item => Number(item.id) === Number(form.value.tunnelId || 0)) || null)

function formatErrorMessage(error, fallbackKey) {
  return translateLiteral(error?.response?.data?.msg || error?.response?.data?.error || error?.message || t(fallbackKey))
}

function assertCompat(res, fallbackKey) {
  if (res && typeof res.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || t(fallbackKey))
  }
  return res
}

function unwrapData(res, fallbackKey) {
  if (!res) return null
  if (typeof res.code === 'number') {
    if (res.code !== 0) {
      throw new Error(res.msg || t(fallbackKey))
    }
    return res.data
  }
  return res.data
}

function normalizeTunnels(input) {
  if (!Array.isArray(input)) return []
  return input
    .map(item => {
      const id = Number(item?.id || 0)
      if (!id) return null
      return {
        id,
        name: item?.name || item?.tunnelName || ''
      }
    })
    .filter(Boolean)
}

function normalizeLimits(input) {
  if (!Array.isArray(input)) return []
  return input
    .map(item => {
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
    })
    .filter(Boolean)
}

function hydrateLimitTunnelName(items) {
  if (!Array.isArray(items) || items.length === 0) return []
  const tunnelMap = new Map(tunnels.value.map(item => [Number(item.id), item.name || t('runtime.limitPage.values.tunnelFallback', { id: item.id })]))
  return items.map(item => ({
    ...item,
    tunnelName: item.tunnelName || tunnelMap.get(Number(item.tunnelId)) || ''
  }))
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
    window.alert(formatErrorMessage(error, 'runtime.limitPage.messages.loadFailed'))
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
  if (invalid) {
    formError.value = invalid
    return
  }

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
      assertCompat(await updateSpeedLimit({ id: Number(form.value.id), ...payload }), 'runtime.limitPage.messages.updateFailed')
    } else {
      assertCompat(await createSpeedLimit(payload), 'runtime.limitPage.messages.createFailed')
    }

    await fetchLimits()
    showFormModal.value = false
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
    assertCompat(await deleteSpeedLimit(Number(deletingItem.value.id)), 'runtime.limitPage.messages.deleteFailed')
    await fetchLimits()
    closeDeleteModal()
  } catch (error) {
    window.alert(formatErrorMessage(error, 'runtime.limitPage.messages.deleteFailed'))
  } finally {
    deleteSubmitting.value = false
  }
}

function formatSpeed(value) {
  const speed = Number(value || 0)
  if (!Number.isFinite(speed) || speed <= 0) {
    return t('runtime.limitPage.values.unlimited')
  }
  return `${speed} Mbps`
}

function formatTunnel(item) {
  if (item.tunnelName) {
    return item.tunnelName
  }
  if (item.tunnelId) {
    return t('runtime.limitPage.values.tunnelFallback', { id: item.tunnelId })
  }
  return '-'
}

function formatRuleName(item) {
  if (item?.name) {
    return item.name
  }
  return t('runtime.limitPage.values.ruleFallback', { id: item?.id || '-' })
}

function formatTime(value) {
  const numeric = Number(value || 0)
  if (!Number.isFinite(numeric) || numeric <= 0) return '-'
  return formatDateTime(numeric, { second: '2-digit' }) || '-'
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped>
.limit-page {
  max-width: 1400px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.page-header h1 {
  margin: 0 0 6px;
  font-size: 24px;
}

.text-secondary {
  color: var(--text-secondary);
}

.header-actions {
  display: flex;
  gap: 10px;
}

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 240px;
  color: var(--text-secondary);
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(148, 163, 184, 0.35);
  border-top-color: var(--primary-color);
  border-radius: 999px;
  animation: spin 0.8s linear infinite;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.limit-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 16px;
  box-shadow: var(--shadow-sm);
}

.card-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.title-wrap h3 {
  margin: 0;
  font-size: 16px;
}

.status-pill {
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 600;
}

.status-active {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.status-disabled {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.card-actions {
  display: flex;
  gap: 6px;
}

.kv-list {
  display: grid;
  gap: 8px;
}

.kv-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  font-size: 13px;
}

.kv-item .k {
  color: var(--text-secondary);
}

.kv-item .v {
  color: var(--text-color);
  text-align: right;
}

.empty-state {
  border: 1px dashed var(--border-color);
  border-radius: var(--radius-lg);
  padding: 36px 16px;
  text-align: center;
  background: var(--surface-color);
}

.empty-icon {
  font-size: 36px;
  margin-bottom: 8px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  width: 100%;
  max-width: 560px;
  background: var(--surface-color);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
}

.modal-sm {
  max-width: 460px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
}

.modal-body {
  padding: 18px 20px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 20px 18px;
  border-top: 1px solid var(--border-color);
}

.form-group {
  margin-bottom: 14px;
}

.form-group label {
  display: block;
  margin-bottom: 6px;
  font-size: 14px;
  font-weight: 600;
}

.form-group input,
.form-group select {
  width: 100%;
}

.error-msg {
  margin: 0;
  color: var(--error-color);
  font-size: 13px;
}

.close-btn {
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 20px;
  cursor: pointer;
}

.btn-danger {
  background: var(--error-color);
  color: #fff;
}

.delete-copy {
  margin: 0 0 10px;
  line-height: 1.7;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 768px) {
  .card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
