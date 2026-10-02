<template>
  <div class="list-page limit-page">
    <UiPageHeader :title="t('runtime.limitPage.title')" :description="t('runtime.limitPage.note')">
      <template #meta>
        <UiBadge tone="neutral" :dot="false" :label="t('runtime.limitPage.heroEyebrow')" />
        <UiBadge tone="info" :dot="false" :label="runtimeModeLabel" data-test="limit-runtime-mode" />
      </template>
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-test="limit-add" @click="openCreateModal">{{ t('runtime.limitPage.actions.create') }}</UiButton>
      </template>
    </UiPageHeader>

    <nav class="runtime-context" :aria-label="t('runtime.limitPage.runtimeLinks')">
      <span class="runtime-context__summary">{{ runtimeModeSummary }}</span>
      <span class="runtime-context__links">
        <router-link to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</router-link>
        <router-link to="/admin/forward/nodex">{{ t('forwardSuite.nav.nodeXRuntime') }}</router-link>
      </span>
    </nav>

    <UiDataTable
      :columns="columns"
      :rows="limits"
      :label="t('runtime.limitPage.table.label')"
      :row-label="formatRuleName"
      storage-key="admin.forward.limits"
      :loading="loading"
      :error="limits.length ? null : pageError"
      :error-title="t('runtime.limitPage.messages.loadFailed')"
      :empty-icon="Gauge"
      :empty-title="t('runtime.limitPage.empty.title')"
      :empty-description="t('runtime.limitPage.empty.text')"
      :row-actions="limitActions"
      activatable
      data-test="limit-table"
      @row-activate="openEditModal"
      @retry="refreshAll"
    >
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openCreateModal">{{ t('runtime.limitPage.actions.createNow') }}</UiButton>
      </template>
      <template #cell-name="{ row }">
        <span class="limit-name">{{ formatRuleName(row) }}</span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge
          :tone="row.status === 1 ? 'success' : 'danger'"
          :label="row.status === 1 ? t('runtime.limitPage.status.active') : t('runtime.limitPage.status.error')"
        />
      </template>
    </UiDataTable>

    <UiSheet
      :open="showFormModal"
      size="sm"
      :title="isEditMode ? t('runtime.limitPage.formModal.titleEdit') : t('runtime.limitPage.formModal.titleCreate')"
      :description="isEditMode ? formatRuleName(form) : t('runtime.limitPage.note')"
      :dismissible="!formSubmitting"
      data-test="limit-form-sheet"
      @update:open="value => { if (!value) closeFormModal() }"
    >
      <form id="limit-form" class="editor-form" novalidate @submit.prevent="submitForm">
        <UiTextField
          id="limit-form-name"
          v-model.trim="form.name"
          :label="t('runtime.limitPage.formModal.fields.name')"
          :placeholder="t('runtime.limitPage.formModal.placeholders.name')"
          maxlength="50"
          required
          size="md"
        />
        <UiNumberField
          v-model="form.speed"
          :label="t('runtime.limitPage.formModal.fields.speed')"
          :placeholder="t('runtime.limitPage.formModal.placeholders.speed')"
          :min="1"
          :step="1"
          unit="Mbps"
          required
          size="md"
        />
        <UiSelect
          :model-value="form.tunnelId || undefined"
          :label="t('runtime.limitPage.formModal.fields.tunnel')"
          :placeholder="t('runtime.limitPage.formModal.placeholders.tunnel')"
          :options="tunnelOptions"
          required
          size="md"
          @update:model-value="value => { form.tunnelId = Number(value) || 0 }"
        />
        <p v-if="formError" id="limit-form-error" class="form-error" role="alert">{{ formError }}</p>
      </form>
      <template #footer>
        <UiButton :disabled="formSubmitting" @click="closeFormModal">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton type="submit" form="limit-form" variant="primary" :loading="formSubmitting" data-test="limit-form-submit">
          {{ isEditMode ? t('runtime.limitPage.formModal.submitUpdate') : t('runtime.limitPage.formModal.submitCreate') }}
        </UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
// 限速 (/admin/forward/limit): flux-panel limit.tsx. UI U7 changed visuals
// and interaction components only: the rules are a UiDataTable and the
// editor is a Sheet. Same /speed-limit/* endpoints and fields.
import { computed, onMounted, ref } from 'vue'
import { Gauge, Pencil, Plus, Trash2 } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  createSpeedLimit,
  deleteSpeedLimit,
  getSpeedLimitList,
  getSpeedLimitTunnels,
  getSystemConfig,
  updateSpeedLimit
} from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'

const { t, formatDateTime, translateLiteral } = useAppI18n()

const loading = ref(true)
const pageError = ref(null)
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
    : t('runtime.limitPage.modeSummaryLocal', { backend: humanizeForwardRuntimeBackend(t, runtimeBackend.value) })
))

const showFormModal = ref(false)
const formSubmitting = ref(false)
const formError = ref('')
const form = ref(newForm())

const toast = useToast()
const confirm = useConfirm()

function newForm() {
  return { id: null, name: '', speed: 100, tunnelId: 0 }
}

// Interaction components (UI U7) ----------------------------------------
const columns = computed(() => [
  { key: 'name', label: t('runtime.limitPage.formModal.fields.name'), primary: true, hideable: false, value: formatRuleName },
  { key: 'tunnel', label: t('runtime.limitPage.formModal.fields.tunnel'), secondary: true, value: formatTunnel },
  { key: 'speed', label: t('runtime.limitPage.cards.speed'), numeric: true, value: item => formatSpeed(item.speed) },
  { key: 'status', label: t('runtime.limitPage.table.status'), value: item => (item.status === 1 ? t('runtime.limitPage.status.active') : t('runtime.limitPage.status.error')) },
  { key: 'updatedAt', label: t('runtime.limitPage.cards.updatedAt'), numeric: true, value: item => formatTime(item.updatedTime || item.createdTime), breakpoint: 'md' }
])
const limitActions = item => [
  { key: 'edit', label: t('runtime.limitPage.actions.edit'), icon: Pencil, onSelect: () => openEditModal(item) },
  { key: 'delete', label: t('runtime.limitPage.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => openDeleteModal(item) }
]
const tunnelOptions = computed(() => tunnels.value.map(item => ({
  value: item.id,
  label: item.name || t('runtime.limitPage.values.tunnelFallback', { id: item.id })
})))

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

// Results go to the shared toasts: success disappears, errors stay.
function setFeedback(type, message) {
  if (type === 'error') toast.error(message)
  else toast.success(message)
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
    pageError.value = null
  } catch (error) {
    // Nothing listed yet: the table's error state (重试); otherwise a toast.
    if (limits.value.length) setFeedback('error', formatErrorMessage(error, 'runtime.limitPage.messages.loadFailed'))
    else pageError.value = error?.response ? error : formatErrorMessage(error, 'runtime.limitPage.messages.loadFailed')
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

async function openDeleteModal(item) {
  if (!item?.id) return
  const confirmed = await confirm({
    title: t('runtime.limitPage.deleteModal.title', { name: formatRuleName(item) }),
    message: t('runtime.limitPage.deleteModal.hint'),
    confirmLabel: t('runtime.limitPage.deleteModal.confirmDelete'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteSpeedLimit(Number(item.id))
      } catch (error) {
        throw new Error(formatErrorMessage(error, 'runtime.limitPage.messages.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  setFeedback('success', t('runtime.limitPage.messages.deleted'))
  try {
    await fetchLimits()
  } catch (error) {
    setFeedback('error', formatErrorMessage(error, 'runtime.limitPage.messages.loadFailed'))
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
.runtime-context {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  align-items: baseline;
  margin-top: calc(-1 * var(--space-3));
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.runtime-context__links {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-4);
}

.runtime-context__links a {
  color: var(--accent);
  font-weight: var(--weight-medium);
  text-decoration: none;
}

.runtime-context__links a:hover {
  text-decoration: underline;
}

.runtime-context__links a:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.limit-name {
  font-weight: var(--weight-medium);
  overflow-wrap: anywhere;
}

.editor-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.editor-form .form-error {
  margin: 0;
}
</style>
