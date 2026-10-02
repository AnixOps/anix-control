<template>
  <div class="list-page forward-page">
    <UiPageHeader :title="t('runtime.forward.title')" :description="t('runtime.forward.modeCompatibilityHint')">
      <template #meta>
        <UiBadge tone="info" :dot="false" :label="runtimeModeLabel" data-test="forward-runtime-mode" />
      </template>
      <template #actions>
        <UiMenu :label="t('runtime.forward.actions.more')" :items="pageMenuItems" size="md" variant="secondary" data-test="forward-page-menu" />
        <UiButton variant="primary" :icon="Plus" data-test="forward-add" @click="openCreateModal">{{ t('runtime.forward.actions.add') }}</UiButton>
      </template>
    </UiPageHeader>

    <nav class="runtime-context" :aria-label="t('runtime.forward.runtimeLinks')">
      <span class="runtime-context__summary">{{ runtimeModeSummary }}</span>
      <span class="runtime-context__links">
        <router-link to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</router-link>
        <router-link to="/admin/forward/nodex">{{ t('forwardSuite.nav.nodeXRuntime') }}</router-link>
      </span>
    </nav>

    <ForwardRulesTable
      v-if="viewMode === 'direct'"
      v-model:selected="selectedForwardIds"
      :rows="sortedDirectForwards"
      :label="t('runtime.forward.table.label')"
      storage-key="admin.forward.rules"
      :loading="loading"
      :error="forwards.length ? null : pageError"
      :error-title="t('runtime.forward.messages.loadForwardsFailed')"
      :filtered="hasDirectFilters"
      :empty-title="t('runtime.forward.emptyDirectTitle')"
      :empty-description="t('runtime.forward.emptyDirectText')"
      selectable
      reorderable
      data-test="forward-direct-view"
      @edit="openEditModal"
      @diagnose="openDiagnosisModal"
      @delete="openDeleteModal"
      @toggle="handleToggleService"
      @address="showAddressModal"
      @reorder="reorderDirectForwards"
      @drag-change="id => { draggingId = id }"
      @retry="reload"
      @clear-filters="clearDirectFilters"
    >
      <template #toolbar>
        <UiSearchField
          v-model="directFilters.keyword"
          class="list-page__search"
          :label="t('runtime.forward.filters.search')"
          :placeholder="t('runtime.forward.filters.searchPlaceholder')"
          data-test="forward-filter-keyword"
        />
        <span class="tunnel-filter" data-test="forward-filter-tunnel">
          <UiSelect
            v-model="tunnelFilter"
            size="md"
            :aria-label="t('runtime.forward.filters.tunnel')"
            :options="tunnelFilterOptions"
          />
        </span>
        <UiFilterChips v-model="statusFilter" :label="t('runtime.forward.filters.status')" :options="statusChips" data-test="forward-filter-status" />
      </template>
      <template #toolbar-end>
        <UiSegmentedControl v-model="viewModeModel" :options="viewOptions" :aria-label="t('runtime.forward.view.label')" size="sm" data-test="forward-view-mode" />
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openCreateModal">{{ t('runtime.forward.actions.add') }}</UiButton>
      </template>
      <template #bulk-actions>
        <UiButton size="sm" :icon="Play" :disabled="bulkLoading" data-test="forward-bulk-resume" @click="runBulkServiceAction('resume')">{{ t('runtime.forward.bulk.resume') }}</UiButton>
        <UiButton size="sm" :icon="Pause" :disabled="bulkLoading" data-test="forward-bulk-pause" @click="runBulkServiceAction('pause')">{{ t('runtime.forward.bulk.pause') }}</UiButton>
        <UiButton size="sm" :icon="Download" :disabled="bulkLoading" data-test="forward-bulk-export" @click="bulkExportSelected">{{ t('runtime.forward.bulk.export') }}</UiButton>
        <UiButton size="sm" variant="danger-soft" :icon="Trash2" :disabled="bulkLoading" data-test="forward-bulk-delete" @click="bulkDeleteSelected">{{ t('runtime.forward.bulk.delete') }}</UiButton>
      </template>
    </ForwardRulesTable>

    <section v-else class="grouped-section" :aria-label="t('runtime.forward.view.groupedLabel')">
      <div class="grouped-toolbar">
        <UiSegmentedControl v-model="viewModeModel" :options="viewOptions" :aria-label="t('runtime.forward.view.label')" size="sm" data-test="forward-view-mode" />
      </div>
      <UiErrorState
        v-if="pageError && !forwards.length"
        :title="t('runtime.forward.messages.loadForwardsFailed')"
        :error="pageError"
        @retry="reload"
      />
      <UiSkeleton v-else-if="showGroupedSkeleton" variant="card" :rows="2" :label="t('runtime.forward.loading')" />
      <div v-else-if="loading && !forwards.length" class="grouped-pending" />
      <UiEmptyState
        v-else-if="!groupedForwards.length"
        :icon="ArrowLeftRight"
        heading-tag="h2"
        :title="t('runtime.forward.emptyGroupedTitle')"
        :description="t('runtime.forward.emptyGroupedText')"
      >
        <template #actions>
          <UiButton variant="primary" :icon="Plus" @click="openCreateModal">{{ t('runtime.forward.actions.add') }}</UiButton>
        </template>
      </UiEmptyState>
      <ForwardGroupedView
        v-else
        :groups="groupedForwards"
        @edit="openEditModal"
        @diagnose="openDiagnosisModal"
        @delete="openDeleteModal"
        @toggle="handleToggleService"
        @address="showAddressModal"
      />
    </section>

    <UiSheet
      :open="modalOpen"
      size="md"
      :title="isEdit ? t('runtime.forward.editor.titleEdit') : t('runtime.forward.editor.titleAdd')"
      :description="isEdit ? form.name : t('runtime.forward.editor.description')"
      :dismissible="!submitLoading"
      data-test="forward-editor-dialog"
      @update:open="value => { if (!value) closeEditorModal() }"
    >
      <form id="forward-editor-form" class="editor-form" novalidate @submit.prevent="handleSubmit">
        <UiTextField
          v-model.trim="form.name"
          :label="t('runtime.forward.editor.fields.name')"
          :placeholder="t('runtime.forward.editor.placeholders.name')"
          :error="errors.name"
          maxlength="50"
          required
          size="md"
        />
        <span class="editor-form__select" data-test="forward-tunnel-select">
          <UiSelect
            :model-value="form.tunnelId ?? undefined"
            :label="t('runtime.forward.editor.fields.tunnel')"
            :placeholder="t('runtime.forward.editor.placeholders.tunnel')"
            :options="tunnelOptions"
            :help="selectedTunnelModeHint"
            :error="errors.tunnelId"
            required
            size="md"
            @update:model-value="handleTunnelChange"
          />
        </span>
        <div class="form-grid">
          <UiTextField
            v-model="portInput"
            type="number"
            min="1"
            max="65535"
            inputmode="numeric"
            :label="t('runtime.forward.editor.fields.ingressPort')"
            :placeholder="t('runtime.forward.editor.placeholders.ingressPort')"
            :help="selectedTunnelPortHint"
            :error="errors.inPort"
            size="md"
          />
          <UiTextField
            v-model.trim="form.interfaceName"
            :label="t('runtime.forward.editor.fields.interfaceName')"
            :placeholder="t('runtime.forward.editor.placeholders.interfaceName')"
            size="md"
          />
        </div>
        <UiTextarea
          v-model="form.remoteAddr"
          class="editor-form__targets"
          :rows="6"
          :label="t('runtime.forward.editor.fields.remoteAddress')"
          :placeholder="t('runtime.forward.editor.placeholders.remoteAddress')"
          :help="t('runtime.forward.editor.remoteHint')"
          :error="errors.remoteAddr"
          required
        />
        <UiSelect
          v-if="addressLineCount > 1"
          v-model="form.strategy"
          :label="t('runtime.forward.editor.fields.strategy')"
          :options="strategyOptions"
          size="md"
        />
      </form>
      <template #footer>
        <UiButton :disabled="submitLoading" @click="closeEditorModal">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton type="submit" form="forward-editor-form" variant="primary" :loading="submitLoading" data-test="forward-editor-submit">
          {{ isEdit ? t('runtime.forward.editor.submitUpdate') : t('runtime.forward.editor.submitCreate') }}
        </UiButton>
      </template>
    </UiSheet>

    <UiConfirmDialog
      :open="deleteModalOpen"
      tone="danger"
      :title="t('runtime.forward.deleteModal.confirmText', { name: forwardToDelete?.name || '-' })"
      :message="t('runtime.forward.deleteModal.hint')"
      :confirm-label="t('runtime.forward.deleteModal.confirmDelete')"
      :loading="deleteLoading"
      :error="deleteError"
      @confirm="confirmDelete"
      @cancel="closeDeleteModal"
    />

    <ForwardAddressDialog
      v-model:open="addressModalOpen"
      :title="addressModalTitle"
      :addresses="addressList"
      @copy="copyAddress"
      @copy-all="copyAllAddresses"
    />

    <ForwardExportDialog
      v-model:open="exportModalOpen"
      v-model:tunnel-id="selectedTunnelForExport"
      :source="exportDataSource"
      :tunnels="tunnels"
      :data="exportData"
      :selection-count="exportSelectionCount"
      :loading="exportLoading"
      @generate="executeExport"
      @copy="copyExportData"
    />

    <ForwardImportDialog
      v-model:open="importModalOpen"
      v-model:tunnel-id="selectedTunnelForImport"
      v-model:data="importData"
      :tunnels="tunnels"
      :results="importResults"
      :success-count="importSuccessCount"
      :loading="importLoading"
      @import="executeImport"
    />

    <UiSheet
      v-model:open="diagnosisModalOpen"
      size="md"
      :title="t('runtime.forward.diagnosis.title')"
      :description="currentDiagnosisForward?.name || ''"
      data-test="forward-diagnosis-sheet"
    >
      <div v-if="diagnosisLoading" class="diagnosis-loading" role="status">
        <UiSpinner />
        <span>{{ t('runtime.forward.diagnosis.loading') }}</span>
      </div>
      <DiagnosisTimeline
        v-else-if="diagnosisSteps.length"
        :steps="diagnosisSteps"
        :label="t('runtime.forward.diagnosis.title')"
        :summary="diagnosisSummary"
        :checked-at="diagnosisCheckedAt"
      />
      <UiEmptyState
        v-else
        compact
        :icon="Stethoscope"
        :title="t('runtime.forward.diagnosis.emptyTitle')"
        :description="t('runtime.forward.diagnosis.emptyText')"
      />
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
        <UiButton v-if="currentDiagnosisForward" variant="primary" :icon="RotateCw" :loading="diagnosisLoading" @click="openDiagnosisModal(currentDiagnosisForward)">
          {{ t('runtime.forward.diagnosis.rerun') }}
        </UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
// 流量转发 (/admin/forward): the flux-panel forward.tsx clone. UI U7 changed
// the visuals and the interaction components only: the rules are a
// UiDataTable (direct view, with selection, bulk bar and drag to reorder)
// or per-user / per-tunnel groups, the editor and the diagnosis are Sheets,
// import and export sit in the "…" menu. The endpoints, the fields and the
// flows are those of docs/guide/flux-forward-contract.md.
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ArrowLeftRight, Download, Pause, Play, Plus, RotateCw, Stethoscope, Trash2, Upload } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUserStore } from '@/stores/user'
import {
  createForward,
  getForwardList,
  updateForward,
  deleteForward,
  forceDeleteForward,
  pauseForwardService,
  resumeForwardService,
  diagnoseForward,
  updateForwardOrder,
  getForwardTunnels,
  getSystemConfig
} from '@/api/admin'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiConfirmDialog from '@/ui/UiConfirmDialog.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiMenu from '@/ui/UiMenu.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSpinner from '@/ui/UiSpinner.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import DiagnosisTimeline from '@/components/admin/forward/DiagnosisTimeline.vue'
import ForwardAddressDialog from './forward/ForwardAddressDialog.vue'
import ForwardExportDialog from './forward/ForwardExportDialog.vue'
import ForwardGroupedView from './forward/ForwardGroupedView.vue'
import ForwardImportDialog from './forward/ForwardImportDialog.vue'
import ForwardRulesTable from './forward/ForwardRulesTable.vue'
import {
  arrayMove,
  buildExportData,
  findInvalidTargetLine,
  formatInAddress,
  getDirectFilterStatus,
  hasValidInx,
  isForwardRuntimeBusy,
  isForwardToggleDisabled,
  isNodeXOnlyTunnel,
  normalizeForward,
  normalizeInboundAddress,
  normalizeTunnel,
  parseImportEntries,
  qualityKey,
  splitLines
} from './forward/forwardModel'

const { t, translateLiteral } = useAppI18n()
const confirm = useConfirm()
const userStore = useUserStore()
const format = useFormat()

const loading = ref(true)
const refreshing = ref(false)
const pageError = ref(null)
const viewMode = ref(getSavedViewMode())
const forwardOrder = ref(getSavedOrder())
const forwards = ref([])
const tunnels = ref([])
const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeNodeXMode = ref(false)
const runtimeBackend = ref('nftables_ansible')
const runtimeModeLabel = computed(() => (
  runtimeNodeXMode.value
    ? t('runtime.forward.modeLabelNodeX')
    : t('runtime.forward.modeLabelLocal', { backend: humanizeForwardRuntimeBackend(t, runtimeBackend.value) })
))
const runtimeModeSummary = computed(() => (
  runtimeNodeXMode.value
    ? t('runtime.forward.modeSummaryNodeX')
    : t('runtime.forward.modeSummaryLocal')
))

const modalOpen = ref(false)
const deleteModalOpen = ref(false)
const addressModalOpen = ref(false)
const diagnosisModalOpen = ref(false)
const exportModalOpen = ref(false)
const importModalOpen = ref(false)
const isEdit = ref(false)

const submitLoading = ref(false)
const deleteLoading = ref(false)
const diagnosisLoading = ref(false)
const exportLoading = ref(false)
const importLoading = ref(false)
const bulkLoading = ref(false)

const forwardToDelete = ref(null)
const deleteError = ref('')
const currentDiagnosisForward = ref(null)
const diagnosisResult = ref(null)
const addressModalTitle = ref('')
const addressList = ref([])
const exportData = ref('')
const exportDataSource = ref('tunnel')
const exportSelectionCount = ref(0)
const selectedTunnelForExport = ref(null)
const importData = ref('')
const selectedTunnelForImport = ref(null)
const importResults = ref([])
const selectedTunnel = ref(null)
const selectedForwardIds = ref([])
const portInput = ref('')
const draggingId = ref(null)

const form = reactive({
  id: null,
  userId: null,
  name: '',
  tunnelId: null,
  inPort: null,
  remoteAddr: '',
  interfaceName: '',
  strategy: 'fifo'
})

const errors = reactive({
  name: '',
  tunnelId: '',
  remoteAddr: '',
  inPort: ''
})

const directFilters = reactive({
  keyword: '',
  tunnelId: '',
  status: 'all'
})

const AUTO_REFRESH_INTERVAL_MS = 10000
let refreshTimer = null
let dataLoadPromise = null

const currentUserId = computed(() => resolveCurrentUserId())
const addressLineCount = computed(() => splitLines(form.remoteAddr).length)
const directForwards = computed(() => getSortedForwards('direct'))
const sortedDirectForwards = computed(() => filterDirectForwards(directForwards.value))
const directForwardIds = computed(() => sortedDirectForwards.value.map(item => item.id))
const selectedDirectForwards = computed(() => {
  const selectedSet = new Set(selectedForwardIds.value)
  return sortedDirectForwards.value.filter(forward => selectedSet.has(forward.id))
})
const directFilterTunnels = computed(() => {
  const entries = new Map()
  directForwards.value.forEach(forward => {
    const id = Number(forward.tunnelId)
    if (!Number.isFinite(id)) {
      return
    }
    entries.set(id, {
      id,
      name: forward.tunnelName || formatTunnelReference(id)
    })
  })
  return Array.from(entries.values()).sort((a, b) => a.name.localeCompare(b.name, undefined, {
    numeric: true,
    sensitivity: 'base'
  }))
})
const hasDirectFilters = computed(() => Boolean(
  directFilters.keyword.trim() ||
    directFilters.tunnelId ||
    directFilters.status !== 'all'
))
const groupedForwards = computed(() => buildGroupedForwards())
const importSuccessCount = computed(() => importResults.value.filter(item => item.success).length)
const selectableTunnels = computed(() => {
  const selectedTunnelID = Number(selectedTunnel.value?.id || form.tunnelId || 0)
  return tunnels.value.filter(tunnel => {
    if (Number(tunnel.id) === selectedTunnelID) {
      return true
    }
    return runtimeNodeXMode.value || !isNodeXOnlyTunnel(tunnel)
  })
})
const selectedTunnelModeHint = computed(() => {
  if (!selectedTunnel.value) {
    return runtimeNodeXMode.value
      ? t('runtime.forward.modeHintNodeX')
      : t('runtime.forward.modeHintLocal')
  }

  const tunnelName = selectedTunnel.value.name || formatTunnelReference(selectedTunnel.value.id || '-')
  if (!runtimeNodeXMode.value && isNodeXOnlyTunnel(selectedTunnel.value)) {
    return t('runtime.forward.tunnelHintLocalIncompatible', { name: tunnelName })
  }
  return runtimeNodeXMode.value
    ? t('runtime.forward.tunnelHintNodeX', { name: tunnelName })
    : t('runtime.forward.tunnelHintLocal', { name: tunnelName })
})
const selectedTunnelPortHint = computed(() => {
  if (selectedTunnel.value?.inNodePortSta && selectedTunnel.value?.inNodePortEnd) {
    return t('runtime.forward.portRange', {
      start: selectedTunnel.value.inNodePortSta,
      end: selectedTunnel.value.inNodePortEnd
    })
  }
  return runtimeNodeXMode.value
    ? t('runtime.forward.portHintNodeX')
    : t('runtime.forward.portHintLocal')
})

// Interaction components (UI U7) ----------------------------------------
const pageMenuItems = computed(() => [
  { key: 'import', label: t('runtime.forward.actions.import'), icon: Upload, onSelect: openImportModal },
  { key: 'export', label: t('runtime.forward.actions.export'), icon: Download, onSelect: openExportModal }
])
const viewOptions = computed(() => [
  { value: 'direct', label: t('runtime.forward.view.directLabel') },
  { value: 'grouped', label: t('runtime.forward.view.groupedLabel') }
])
const viewModeModel = computed({
  get: () => viewMode.value,
  set: value => {
    if (value && value !== viewMode.value) toggleViewMode()
  }
})
const tunnelFilterOptions = computed(() => [
  { value: 'all', label: t('runtime.forward.filters.allTunnels') },
  ...directFilterTunnels.value.map(tunnel => ({ value: String(tunnel.id), label: tunnel.name }))
])
const tunnelFilter = computed({
  get: () => directFilters.tunnelId || 'all',
  set: value => { directFilters.tunnelId = value && value !== 'all' ? String(value) : '' }
})
const statusChips = computed(() => [
  { value: 'running', label: t('runtime.forward.filters.running') },
  { value: 'paused', label: t('runtime.forward.filters.paused') },
  { value: 'error', label: t('runtime.forward.filters.error') }
])
const statusFilter = computed({
  get: () => (directFilters.status === 'all' ? '' : directFilters.status),
  set: value => { directFilters.status = value || 'all' }
})
const tunnelOptions = computed(() => selectableTunnels.value.map(tunnel => ({
  value: Number(tunnel.id),
  label: `${tunnel.name} · ${tunnelTypeLabel(tunnel.type)}`
})))
const strategyOptions = computed(() => ['fifo', 'round', 'rand', 'hash'].map(value => ({
  value,
  label: t(`runtime.forward.strategy.${value}`)
})))
const showGroupedSkeleton = useDelayedLoading(() => loading.value && !forwards.value.length)

const diagnosisSteps = computed(() => (diagnosisResult.value?.results || []).map((result, index) => {
  const target = result.targetIp ? `${result.targetIp}${result.targetPort ? `:${result.targetPort}` : ''}` : '—'
  const fields = [{ label: t('runtime.forward.diagnosis.targetAddress'), value: target, mono: true }]
  if (result.success) {
    fields.push(
      { label: t('runtime.forward.diagnosis.averageLatency'), value: `${result.averageTime?.toFixed(0) || '0'} ms`, numeric: true },
      { label: t('runtime.forward.diagnosis.packetLoss'), value: `${result.packetLoss?.toFixed(1) || '0.0'}%`, numeric: true },
      { label: t('runtime.forward.diagnosis.quality'), value: t(`runtime.forward.quality.${qualityKey(result.averageTime, result.packetLoss)}`) }
    )
  }
  return {
    key: `${result.targetIp}-${index}`,
    title: result.description,
    meta: formatDiagnosisNodeMeta(result),
    success: Boolean(result.success),
    statusLabel: result.success ? t('runtime.forward.diagnosis.connectionSuccess') : t('runtime.forward.diagnosis.connectionFailed'),
    fields,
    message: result.success ? '' : (translateLiteral(result.message) || t('runtime.forward.diagnosis.failedFallback'))
  }
}))
const diagnosisSummary = computed(() => {
  const steps = diagnosisSteps.value
  if (!steps.length) return ''
  return t('runtime.forward.diagnosis.summary', { passed: steps.filter(step => step.success).length, total: steps.length })
})
const diagnosisCheckedAt = computed(() => (diagnosisResult.value?.timestamp ? format.dateTime(diagnosisResult.value.timestamp) : ''))

watch(portInput, value => {
  if (value === '' || value === null) {
    form.inPort = null
    return
  }
  const parsed = Number(value)
  form.inPort = Number.isFinite(parsed) ? parsed : null
})

watch(
  () => form.tunnelId,
  value => {
    selectedTunnel.value = tunnels.value.find(item => Number(item.id) === Number(value)) || null
    if (errors.tunnelId) errors.tunnelId = ''
  }
)

watch(
  () => form.name,
  () => {
    if (errors.name) errors.name = ''
  }
)

watch(
  () => form.remoteAddr,
  () => {
    if (errors.remoteAddr) errors.remoteAddr = ''
  }
)

watch(
  () => form.inPort,
  () => {
    if (errors.inPort) errors.inPort = ''
  }
)

watch(
  directForwardIds,
  () => {
    pruneSelectedForwardIds()
  }
)

function parseRuntimeBoolean(value) {
  if (typeof value === 'boolean') {
    return value
  }
  if (typeof value === 'number') {
    return value !== 0
  }

  const normalized = String(value ?? '').trim().toLowerCase()
  if (!normalized) {
    return null
  }
  if (['1', 'true', 'yes', 'on'].includes(normalized)) {
    return true
  }
  if (['0', 'false', 'no', 'off'].includes(normalized)) {
    return false
  }
  return null
}

function tunnelTypeLabel(type) {
  return Number(type) === 2
    ? t('runtime.tunnel.options.tunnelForward')
    : t('runtime.tunnel.options.portForward')
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
    if (explicitMode !== null) {
      runtimeNodeXMode.value = explicitMode
    }
  }
}

onMounted(async () => {
  if (!currentUserId.value && userStore.isLoggedIn) {
    await userStore.getUserInfo()
  }

  await loadRuntimeMode()
  await loadData(true)
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
  clearFeedback()
})

function getSavedViewMode() {
  try {
    const saved = localStorage.getItem('forward-view-mode')
    return saved === 'grouped' || saved === 'direct' ? saved : 'direct'
  } catch {
    return 'direct'
  }
}

function getSavedOrder() {
  try {
    const saved = localStorage.getItem('forward-order')
    if (!saved) return []
    const parsed = JSON.parse(saved)
    return Array.isArray(parsed) ? parsed.map(item => Number(item)).filter(item => Number.isFinite(item)) : []
  } catch {
    return []
  }
}

function saveOrder(order) {
  try {
    localStorage.setItem('forward-order', JSON.stringify(order))
  } catch (error) {
    console.warn('Failed to save order to localStorage:', error)
  }
}

function resolveCurrentUserId() {
  const storeId = Number(
    userStore.userInfo?.id ??
      userStore.userInfo?.user_id ??
      userStore.userInfo?.ID ??
      0
  )
  if (Number.isFinite(storeId) && storeId > 0) {
    return storeId
  }

  const token = userStore.token || localStorage.getItem('token') || ''
  if (!token || !token.includes('.')) {
    return null
  }

  try {
    const payload = token.split('.')[1]
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/')
    const decoded = atob(normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '='))
    const parsed = JSON.parse(decoded)
    const id = Number(parsed.user_id ?? parsed.id ?? parsed.uid ?? parsed.sub ?? 0)
    return Number.isFinite(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

// Results go to the shared toasts (UI U4); the same message replaces its
// previous toast instead of stacking (auto-refresh can repeat a failure).
const toast = useToast()
const feedbackToasts = new Map()
function setFeedback(type, message) {
  const text = String(message ?? '')
  if (!text) return
  if (feedbackToasts.has(text)) toast.dismiss(feedbackToasts.get(text))
  const tone = ['success', 'error', 'warning', 'info'].includes(type) ? type : 'info'
  feedbackToasts.set(text, toast[tone](text))
}

function clearFeedback() {
  for (const id of feedbackToasts.values()) toast.dismiss(id)
  feedbackToasts.clear()
}

function clearBulkSelection() {
  selectedForwardIds.value = []
}

function pruneSelectedForwardIds() {
  const validIds = new Set(directForwardIds.value)
  selectedForwardIds.value = selectedForwardIds.value.filter(id => validIds.has(id))
}

function shouldSuspendAutoRefresh() {
  return Boolean(
    loading.value ||
      refreshing.value ||
      dataLoadPromise ||
      bulkLoading.value ||
      submitLoading.value ||
      deleteLoading.value ||
      diagnosisLoading.value ||
      exportLoading.value ||
      importLoading.value ||
      draggingId.value !== null ||
      modalOpen.value ||
      deleteModalOpen.value ||
      addressModalOpen.value ||
      diagnosisModalOpen.value ||
      exportModalOpen.value ||
      importModalOpen.value
  )
}

function startAutoRefresh() {
  stopAutoRefresh()
  refreshTimer = window.setInterval(() => {
    refreshDataSilently()
  }, AUTO_REFRESH_INTERVAL_MS)
}

function stopAutoRefresh() {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
}

async function refreshDataSilently() {
  if (shouldSuspendAutoRefresh()) {
    return
  }
  await loadData(false, { silent: true })
}

function translateMessage(message, fallbackKey = null, params = {}) {
  const translated = translateLiteral(message)
  if (translated && translated !== message) {
    return translated
  }
  if (message) {
    return message
  }
  return fallbackKey ? t(fallbackKey, params) : ''
}

function getForwardSearchText(forward) {
  return [
    forward?.id,
    forward?.name,
    forward?.userName,
    forward?.email,
    forward?.tunnelName,
    forward?.tunnelId ? formatTunnelReference(forward.tunnelId) : '',
    forward?.inIp,
    forward?.inPort,
    formatInAddress(forward?.inIp, forward?.inPort),
    forward?.remoteAddr,
    forward?.strategy
  ]
    .filter(value => value !== undefined && value !== null)
    .join(' ')
    .toLowerCase()
}

function filterDirectForwards(list) {
  const keyword = directFilters.keyword.trim().toLowerCase()
  const tunnelId = directFilters.tunnelId ? Number(directFilters.tunnelId) : null
  const status = directFilters.status

  return list.filter(forward => {
    if (keyword && !getForwardSearchText(forward).includes(keyword)) {
      return false
    }
    if (tunnelId !== null && Number(forward.tunnelId) !== tunnelId) {
      return false
    }
    if (status !== 'all' && getDirectFilterStatus(forward) !== status) {
      return false
    }
    return true
  })
}

function clearDirectFilters() {
  directFilters.keyword = ''
  directFilters.tunnelId = ''
  directFilters.status = 'all'
}

function mergeReferencedTunnels(list, forwardList) {
  const merged = Array.isArray(list) ? [...list] : []
  const tunnelMap = new Map(merged.map(item => [Number(item.id), item]))

  for (const forward of Array.isArray(forwardList) ? forwardList : []) {
    const tunnelId = Number(forward.tunnelId)
    if (!Number.isFinite(tunnelId) || tunnelId <= 0 || tunnelMap.has(tunnelId)) {
      continue
    }

    const fallbackTunnel = normalizeTunnel({
      id: tunnelId,
      name: forward.tunnelName || formatTunnelReference(tunnelId),
      inIp: forward.inIp || '',
      status: 0
    })
    merged.push(fallbackTunnel)
    tunnelMap.set(tunnelId, fallbackTunnel)
  }

  return merged.sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')))
}

function filterCurrentUserForwards(list) {
  if (!Array.isArray(list)) return []
  if (userStore.isAdmin) return list
  if (currentUserId.value == null) return list
  return list.filter(forward => Number(forward.userId) === Number(currentUserId.value))
}

function initializeOrder(list) {
  const userForwards = filterCurrentUserForwards(list)
  if (!userForwards.length) {
    forwardOrder.value = []
    saveOrder([])
    return
  }

  const hasDbOrdering = userForwards.some(hasValidInx)
  if (hasDbOrdering) {
    const dbOrder = [...userForwards]
      .sort((a, b) => (a.inx ?? 0) - (b.inx ?? 0))
      .map(item => item.id)
    forwardOrder.value = dbOrder
    saveOrder(dbOrder)
    return
  }

  const savedOrder = getSavedOrder()
  if (savedOrder.length) {
    const validOrder = savedOrder.filter(id => userForwards.some(item => item.id === id))
    userForwards.forEach(forward => {
      if (!validOrder.includes(forward.id)) {
        validOrder.push(forward.id)
      }
    })
    forwardOrder.value = validOrder
    saveOrder(validOrder)
    return
  }

  const order = userForwards.map(item => item.id)
  forwardOrder.value = order
  saveOrder(order)
}

async function loadData(showLoading = true, options = {}) {
  if (dataLoadPromise) {
    if (!options.force) {
      return dataLoadPromise
    }
    await dataLoadPromise
  }

  if (showLoading) {
    loading.value = true
  } else {
    refreshing.value = true
  }

  dataLoadPromise = (async () => {
    try {
      const [forwardsRes, tunnelsRes] = await Promise.all([getForwardList(), getForwardTunnels()])
      let items = forwards.value
      let availableTunnels = tunnels.value

      if (forwardsRes.code === 0) {
        items = Array.isArray(forwardsRes.data) ? forwardsRes.data.map(normalizeForward) : []
        forwards.value = items
        pageError.value = null
        if (viewMode.value === 'direct') {
          initializeOrder(items)
        }
        pruneSelectedForwardIds()
      } else if (!options.silent) {
        reportLoadError(translateMessage(forwardsRes.msg, 'runtime.forward.messages.loadForwardsFailed'))
      }

      if (tunnelsRes.code === 0) {
        availableTunnels = Array.isArray(tunnelsRes.data) ? tunnelsRes.data.map(normalizeTunnel) : []
      } else if (!options.silent) {
        setFeedback('warning', translateMessage(tunnelsRes.msg, 'runtime.forward.messages.loadTunnelsFailed'))
      }
      tunnels.value = mergeReferencedTunnels(availableTunnels, items)
    } catch (error) {
      console.error('Failed to load forward page data:', error)
      if (!options.silent) {
        reportLoadError(error?.response ? error : t('runtime.forward.messages.loadDataFailed'))
      }
    } finally {
      if (showLoading) {
        loading.value = false
      }
      refreshing.value = false
      dataLoadPromise = null
    }
  })()

  return dataLoadPromise
}

// A failed load with nothing on screen is the error state (重试); with
// rules already listed it is a toast and the list stays.
function reportLoadError(error) {
  if (forwards.value.length) {
    setFeedback('error', typeof error === 'string' ? error : t('runtime.forward.messages.loadDataFailed'))
    return
  }
  pageError.value = error
}

function reload() {
  return loadData(true, { force: true })
}

function getSortedForwards(mode = viewMode.value) {
  if (!Array.isArray(forwards.value) || !forwards.value.length) {
    return []
  }

  let filtered = forwards.value
  if (mode === 'direct') {
    filtered = filterCurrentUserForwards(forwards.value)
  }

  if (!filtered.length) {
    return []
  }

  const sorted = [...filtered].sort((a, b) => (a.inx ?? 0) - (b.inx ?? 0))

  if (forwardOrder.value.length && sorted.every(item => !hasValidInx(item))) {
    const forwardMap = new Map(filtered.map(item => [item.id, item]))
    const localSorted = []

    forwardOrder.value.forEach(id => {
      const match = forwardMap.get(id)
      if (match) {
        localSorted.push(match)
      }
    })

    filtered.forEach(item => {
      if (!forwardOrder.value.includes(item.id)) {
        localSorted.push(item)
      }
    })

    return localSorted
  }

  return sorted
}

function buildGroupedForwards() {
  const userMap = new Map()
  const sorted = getSortedForwards('grouped')

  sorted.forEach(forward => {
    const userKey = forward.userId ? String(forward.userId) : 'unknown'
    const userName = forward.userName || t('runtime.forward.messages.unknownUser')

    if (!userMap.has(userKey)) {
      userMap.set(userKey, {
        userKey,
        userName,
        total: 0,
        tunnelGroups: []
      })
    }

    const userGroup = userMap.get(userKey)
    userGroup.total += 1

    let tunnelGroup = userGroup.tunnelGroups.find(item => item.tunnelId === forward.tunnelId)
    if (!tunnelGroup) {
      tunnelGroup = {
        tunnelId: forward.tunnelId,
        tunnelName: forward.tunnelName || formatTunnelReference(forward.tunnelId),
        running: 0,
        forwards: []
      }
      userGroup.tunnelGroups.push(tunnelGroup)
    }

    tunnelGroup.forwards.push(forward)
    if (forward.serviceRunning) {
      tunnelGroup.running += 1
    }
  })

  return Array.from(userMap.values())
    .sort((a, b) => a.userName.localeCompare(b.userName))
    .map(group => ({
      ...group,
      tunnelGroups: [...group.tunnelGroups].sort((a, b) => a.tunnelName.localeCompare(b.tunnelName))
    }))
}

function toggleViewMode() {
  viewMode.value = viewMode.value === 'grouped' ? 'direct' : 'grouped'
  try {
    localStorage.setItem('forward-view-mode', viewMode.value)
  } catch (error) {
    console.warn('Failed to save view mode to localStorage:', error)
  }

  if (viewMode.value === 'direct') {
    initializeOrder(forwards.value)
    pruneSelectedForwardIds()
  } else {
    clearBulkSelection()
  }
}

function handleTunnelChange(value) {
  form.tunnelId = value ? Number(value) : null
}

function validateForm() {
  errors.name = ''
  errors.tunnelId = ''
  errors.remoteAddr = ''
  errors.inPort = ''

  if (!form.name.trim()) {
    errors.name = t('runtime.forward.messages.nameRequired')
  } else if (form.name.length < 2 || form.name.length > 50) {
    errors.name = t('runtime.forward.messages.nameLength')
  }

  if (!form.tunnelId) {
    errors.tunnelId = t('runtime.forward.messages.tunnelRequired')
  } else if (selectedTunnel.value && !runtimeNodeXMode.value && isNodeXOnlyTunnel(selectedTunnel.value)) {
    errors.tunnelId = t('runtime.forward.messages.localRuntimeTunnelForwardUnsupported')
  }

  if (!form.remoteAddr.trim()) {
    errors.remoteAddr = t('runtime.forward.messages.remoteAddrRequired')
  } else {
    const invalidLine = findInvalidTargetLine(form.remoteAddr)
    if (invalidLine >= 0) {
      errors.remoteAddr = t('runtime.forward.messages.remoteAddrLineInvalid', { line: invalidLine + 1 })
    }
  }

  if (form.inPort !== null && (form.inPort < 1 || form.inPort > 65535)) {
    errors.inPort = t('runtime.forward.messages.portRange')
  }

  if (
    selectedTunnel.value &&
    selectedTunnel.value.inNodePortSta &&
    selectedTunnel.value.inNodePortEnd &&
    form.inPort
  ) {
    if (form.inPort < selectedTunnel.value.inNodePortSta || form.inPort > selectedTunnel.value.inNodePortEnd) {
      errors.inPort = t('runtime.forward.messages.portRangeTunnel', {
        start: selectedTunnel.value.inNodePortSta,
        end: selectedTunnel.value.inNodePortEnd
      })
    }
  }

  return !errors.name && !errors.tunnelId && !errors.remoteAddr && !errors.inPort
}

function resetFormState() {
  Object.assign(form, {
    id: null,
    userId: null,
    name: '',
    tunnelId: null,
    inPort: null,
    remoteAddr: '',
    interfaceName: '',
    strategy: 'fifo'
  })
  portInput.value = ''
  selectedTunnel.value = null
  errors.name = ''
  errors.tunnelId = ''
  errors.remoteAddr = ''
  errors.inPort = ''
}

function openCreateModal() {
  isEdit.value = false
  resetFormState()
  modalOpen.value = true
}

function openEditModal(forward) {
  isEdit.value = true
  Object.assign(form, {
    id: forward.id,
    userId: forward.userId,
    name: forward.name,
    tunnelId: forward.tunnelId,
    inPort: forward.inPort,
    remoteAddr: String(forward.remoteAddr || '').split(',').join('\n'),
    interfaceName: forward.interfaceName || '',
    strategy: forward.strategy || 'fifo'
  })
  portInput.value = forward.inPort ? String(forward.inPort) : ''
  selectedTunnel.value = tunnels.value.find(item => Number(item.id) === Number(forward.tunnelId)) || null
  errors.name = ''
  errors.tunnelId = ''
  errors.remoteAddr = ''
  errors.inPort = ''
  modalOpen.value = true
}

function closeEditorModal() {
  if (submitLoading.value) return
  modalOpen.value = false
}

async function handleSubmit() {
  if (!validateForm()) {
    return
  }

  submitLoading.value = true
  try {
    const processedRemoteAddr = splitLines(form.remoteAddr).join(',')
    const addressCount = processedRemoteAddr.split(',').map(item => item.trim()).filter(Boolean).length
    const payload = {
      name: form.name,
      tunnelId: form.tunnelId,
      inPort: form.inPort,
      remoteAddr: processedRemoteAddr,
      interfaceName: form.interfaceName,
      strategy: addressCount > 1 ? form.strategy : 'fifo'
    }

    const response = isEdit.value
      ? await updateForward({
          id: form.id,
          userId: form.userId,
          ...payload
        })
      : await createForward(payload)

    if (response.code === 0) {
      modalOpen.value = false
      setFeedback('success', isEdit.value ? t('runtime.forward.messages.updated') : t('runtime.forward.messages.created'))
      await loadData(true, { force: true })
    } else {
      setFeedback('error', translateMessage(response.msg, 'runtime.forward.messages.actionFailed'))
    }
  } catch (error) {
    console.error('Failed to submit forward:', error)
    setFeedback('error', t('runtime.forward.messages.actionFailed'))
  } finally {
    submitLoading.value = false
  }
}

async function handleToggleService(forward) {
  if (isForwardRuntimeBusy(forward)) {
    setFeedback('warning', t('runtime.forward.messages.runtimeBusy'))
    return
  }
  if (Number(forward.status) !== 1 && Number(forward.status) !== 0) {
    setFeedback('error', t('runtime.forward.messages.invalidStatus'))
    return
  }

  const targetState = !forward.serviceRunning
  forwards.value = forwards.value.map(item =>
    item.id === forward.id ? { ...item, serviceRunning: targetState } : item
  )

  try {
    const response = targetState ? await resumeForwardService(forward.id) : await pauseForwardService(forward.id)

    if (response.code === 0) {
      await loadData(false, { force: true })
      setFeedback('success', targetState ? t('runtime.forward.messages.serviceChanged') : t('runtime.forward.messages.servicePaused'))
      return
    }

    forwards.value = forwards.value.map(item =>
      item.id === forward.id ? { ...item, serviceRunning: !targetState } : item
    )
    setFeedback('error', translateMessage(response.msg, 'runtime.forward.messages.actionFailed'))
  } catch (error) {
    console.error('Failed to toggle forward service:', error)
    forwards.value = forwards.value.map(item =>
      item.id === forward.id ? { ...item, serviceRunning: !targetState } : item
    )
    setFeedback('error', t('runtime.forward.messages.networkActionFailed'))
  }
}

function setBulkResultFeedback(success, failed) {
  const type = failed > 0 ? (success > 0 ? 'warning' : 'error') : 'success'
  setFeedback(type, t('runtime.forward.messages.bulkActionComplete', { success, failed }))
}

function shouldRunBulkServiceAction(forward, action) {
  if (isForwardToggleDisabled(forward)) {
    return false
  }
  return action === 'resume' ? !forward.serviceRunning : forward.serviceRunning
}

async function runBulkServiceAction(action) {
  const items = selectedDirectForwards.value
  if (!items.length) {
    return
  }

  bulkLoading.value = true
  let success = 0
  let failed = 0

  try {
    for (const forward of items) {
      if (!shouldRunBulkServiceAction(forward, action)) {
        continue
      }

      try {
        const response = action === 'resume'
          ? await resumeForwardService(forward.id)
          : await pauseForwardService(forward.id)
        if (response.code === 0) {
          success += 1
        } else {
          failed += 1
        }
      } catch (error) {
        console.error('Failed to run batch service action:', error)
        failed += 1
      }
    }

    await loadData(false, { force: true })
    setBulkResultFeedback(success, failed)
    if (failed === 0) {
      clearBulkSelection()
    }
  } finally {
    bulkLoading.value = false
  }
}

async function bulkDeleteSelected() {
  const items = selectedDirectForwards.value
  if (!items.length) {
    return
  }

  const confirmed = await confirm({
    title: t('runtime.forward.messages.bulkDeleteTitle', { count: items.length }),
    message: t('runtime.forward.messages.bulkDeleteMessage'),
    confirmLabel: t('runtime.forward.messages.bulkDeleteAction'),
    tone: 'danger'
  })
  if (!confirmed) {
    return
  }

  bulkLoading.value = true
  let success = 0
  let failed = 0

  try {
    for (const forward of items) {
      try {
        const response = await deleteForward(forward.id)
        if (response.code === 0) {
          success += 1
        } else {
          failed += 1
        }
      } catch (error) {
        console.error('Failed to batch delete forward:', error)
        failed += 1
      }
    }

    await loadData(false, { force: true })
    setBulkResultFeedback(success, failed)
    if (failed === 0) {
      clearBulkSelection()
    }
  } finally {
    bulkLoading.value = false
  }
}

function openDeleteModal(forward) {
  forwardToDelete.value = forward
  deleteError.value = ''
  deleteModalOpen.value = true
}

function closeDeleteModal() {
  if (deleteLoading.value) {
    return
  }
  deleteModalOpen.value = false
  deleteError.value = ''
}

async function confirmDelete() {
  if (!forwardToDelete.value) {
    return
  }

  const forward = forwardToDelete.value
  deleteLoading.value = true
  deleteError.value = ''
  try {
    const response = await deleteForward(forward.id)
    if (response.code === 0) {
      deleteModalOpen.value = false
      setFeedback('success', t('runtime.forward.messages.deleted'))
      await loadData(true, { force: true })
      return
    }

    // Same flow as before: a failed regular delete asks a second time
    // whether to force delete.
    const shouldForceDelete = await confirm({
      title: t('runtime.forward.deleteModal.forceDeleteTitle', { name: forward.name || '-' }),
      message: buildForceDeleteConfirmMessage(translateMessage(response.msg, 'runtime.forward.messages.deleteFailed')),
      confirmLabel: t('runtime.forward.deleteModal.forceDeleteAction'),
      tone: 'danger'
    })

    if (!shouldForceDelete) {
      return
    }

    const forceResponse = await forceDeleteForward(forward.id)
    if (forceResponse.code === 0) {
      deleteModalOpen.value = false
      setFeedback('success', t('runtime.forward.messages.forceDeleted'))
      await loadData(true, { force: true })
    } else {
      deleteError.value = translateMessage(forceResponse.msg, 'runtime.forward.messages.forceDeleteFailed')
    }
  } catch (error) {
    console.error('Failed to delete forward:', error)
    deleteError.value = t('runtime.forward.messages.deleteFailed')
  } finally {
    deleteLoading.value = false
  }
}

function buildForceDeleteConfirmMessage(message) {
  return t('runtime.forward.deleteModal.forceDeleteMessage', { message })
}

function buildDiagnosisFallback(forward, title, message) {
  return {
    forwardName: forward.name,
    timestamp: Date.now(),
    results: [
      {
        success: false,
        description: title,
        nodeName: '-',
        nodeId: '-',
        targetIp: String(forward.remoteAddr || '').split(',')[0] || '-',
        message
      }
    ]
  }
}

async function openDiagnosisModal(forward) {
  currentDiagnosisForward.value = forward
  diagnosisModalOpen.value = true
  diagnosisLoading.value = true
  diagnosisResult.value = null

  try {
    const response = await diagnoseForward(forward.id)
    if (response.code === 0) {
      diagnosisResult.value = response.data
    } else {
      const diagnosisMessage = translateMessage(response.msg, 'runtime.forward.messages.diagnosisFailed')
      setFeedback('error', diagnosisMessage)
      diagnosisResult.value = buildDiagnosisFallback(
        forward,
        t('runtime.forward.messages.diagnosisFailed'),
        translateMessage(response.msg, 'runtime.forward.messages.diagnosisProcessingFailed')
      )
    }
  } catch (error) {
    console.error('Failed to diagnose forward:', error)
    setFeedback('error', t('runtime.forward.messages.diagnosisNetworkFailed'))
    diagnosisResult.value = buildDiagnosisFallback(
      forward,
      t('runtime.forward.messages.diagnosisNetworkFailed'),
      t('runtime.forward.messages.unableConnectServer')
    )
  } finally {
    diagnosisLoading.value = false
  }
}

function formatTunnelReference(id) {
  return t('runtime.forward.references.tunnel', { id })
}

function formatNodeReference(id) {
  return t('runtime.forward.references.node', { id })
}

function formatDiagnosisNodeMeta(result) {
  const nodeRef = formatNodeReference(result?.nodeId ?? '-')
  const nodeName = String(result?.nodeName ?? '').trim()

  if (!nodeName || nodeName === '-') {
    return nodeRef
  }

  return t('runtime.forward.diagnosis.nodeMeta', {
    name: nodeName,
    node: nodeRef
  })
}

async function copyToClipboard(text, label = '') {
  try {
    await navigator.clipboard.writeText(text)
    setFeedback('success', t('runtime.forward.messages.contentCopied', { label }))
  } catch (error) {
    console.error('Copy failed:', error)
    setFeedback('error', t('runtime.forward.messages.copyFailedHttp'))
  }
}

function showAddressModal({ value, port = null, title }) {
  if (!value) {
    return
  }

  let addresses = []
  if (port !== null) {
    const ips = String(value)
      .split(',')
      .map(item => item.trim())
      .filter(Boolean)

    if (ips.length <= 1) {
      copyToClipboard(formatInAddress(value, port), title)
      return
    }

    addresses = ips.map(ip => normalizeInboundAddress(ip, port))
  } else {
    addresses = String(value)
      .split(',')
      .map(item => item.trim())
      .filter(Boolean)

    if (addresses.length <= 1) {
      copyToClipboard(addresses[0], title)
      return
    }
  }

  addressList.value = addresses.map((address, index) => ({
    id: index,
    address,
    copying: false
  }))
  addressModalTitle.value = t('runtime.forward.addressModal.titleWithCount', { title, count: addresses.length })
  addressModalOpen.value = true
}

async function copyAddress(item) {
  addressList.value = addressList.value.map(entry =>
    entry.id === item.id ? { ...entry, copying: true } : entry
  )
  try {
    await copyToClipboard(item.address, t('runtime.forward.card.targetLabel'))
  } finally {
    addressList.value = addressList.value.map(entry =>
      entry.id === item.id ? { ...entry, copying: false } : entry
    )
  }
}

async function copyAllAddresses() {
  if (!addressList.value.length) {
    return
  }
  await copyToClipboard(addressList.value.map(item => item.address).join('\n'), t('runtime.forward.actions.copyAll'))
}

function openExportModal() {
  selectedTunnelForExport.value = null
  exportData.value = ''
  exportDataSource.value = 'tunnel'
  exportSelectionCount.value = 0
  exportModalOpen.value = true
}

function getExportSource() {
  if (!selectedTunnelForExport.value) {
    return []
  }

  if (viewMode.value === 'grouped') {
    return groupedForwards.value.flatMap(userGroup =>
      userGroup.tunnelGroups
        .filter(tunnelGroup => Number(tunnelGroup.tunnelId) === Number(selectedTunnelForExport.value))
        .flatMap(tunnelGroup => tunnelGroup.forwards)
    )
  }

  return getSortedForwards('direct').filter(forward => Number(forward.tunnelId) === Number(selectedTunnelForExport.value))
}

function bulkExportSelected() {
  const items = selectedDirectForwards.value
  if (!items.length) {
    setFeedback('error', t('runtime.forward.messages.noExportData'))
    return
  }

  selectedTunnelForExport.value = null
  exportDataSource.value = 'selection'
  exportSelectionCount.value = items.length
  exportData.value = buildExportData(items)
  exportModalOpen.value = true
}

async function executeExport() {
  if (!selectedTunnelForExport.value) {
    setFeedback('error', t('runtime.forward.messages.selectExportTunnel'))
    return
  }

  exportLoading.value = true
  try {
    const items = getExportSource()
    if (!items.length) {
      setFeedback('error', t('runtime.forward.messages.noExportData'))
      return
    }

    exportDataSource.value = 'tunnel'
    exportSelectionCount.value = 0
    exportData.value = buildExportData(items)
  } catch (error) {
    console.error('Failed to export forwards:', error)
    setFeedback('error', t('runtime.forward.messages.exportFailed'))
  } finally {
    exportLoading.value = false
  }
}

async function copyExportData() {
  await copyToClipboard(exportData.value, t('runtime.forward.exportModal.title'))
}

function openImportModal() {
  importData.value = ''
  importResults.value = []
  selectedTunnelForImport.value = null
  importModalOpen.value = true
}

function appendImportResult(result) {
  importResults.value = [result, ...importResults.value]
}

async function executeImport() {
  if (!importData.value.trim()) {
    setFeedback('error', t('runtime.forward.messages.enterImportData'))
    return
  }

  if (!selectedTunnelForImport.value) {
    setFeedback('error', t('runtime.forward.messages.selectImportTunnel'))
    return
  }

  importLoading.value = true
  importResults.value = []

  try {
    const entries = parseImportEntries(importData.value)

    for (const entry of entries) {
      const line = entry.source

      if (entry.legacyParts !== undefined && entry.legacyParts < 2) {
        appendImportResult({
          line,
          success: false,
          message: t('runtime.forward.messages.importFormatError')
        })
        continue
      }

      const remoteAddr = entry.remoteAddr
      const name = entry.name
      const inPortRaw = entry.inPortRaw

      if (!remoteAddr || !name) {
        appendImportResult({
          line,
          success: false,
          message: t('runtime.forward.messages.importRequiredFields')
        })
        continue
      }

      const addressPattern = /^[^:]+:\d+$/
      const isValidRemoteAddr = remoteAddr
        .split(',')
        .map(item => item.trim())
        .every(item => addressPattern.test(item))

      if (!isValidRemoteAddr) {
        appendImportResult({
          line,
          success: false,
          message: t('runtime.forward.messages.importAddressInvalid')
        })
        continue
      }

      let portNumber = null
      if (inPortRaw) {
        const parsedPort = Number(inPortRaw)
        if (!Number.isFinite(parsedPort) || parsedPort < 1 || parsedPort > 65535) {
          appendImportResult({
            line,
            success: false,
            message: t('runtime.forward.messages.importPortInvalid')
          })
          continue
        }
        portNumber = parsedPort
      }

      try {
        const response = await createForward({
          name,
          tunnelId: selectedTunnelForImport.value,
          inPort: portNumber,
          remoteAddr,
          strategy: 'fifo'
        })

        if (response.code === 0) {
          appendImportResult({
            line,
            success: true,
            message: t('runtime.forward.messages.importCreateSuccess'),
            forwardName: name
          })
        } else {
          appendImportResult({
            line,
            success: false,
            message: translateMessage(response.msg, 'runtime.forward.messages.importCreateFailed')
          })
        }
      } catch (error) {
        console.error('Failed to create imported forward:', error)
        appendImportResult({
          line,
          success: false,
          message: t('runtime.forward.messages.importNetworkCreateFailed')
        })
      }
    }

    setFeedback('success', t('runtime.forward.messages.importCompleted'))
    await loadData(false, { force: true })
  } catch (error) {
    console.error('Failed to import forwards:', error)
    setFeedback('error', t('runtime.forward.messages.importFailed'))
  } finally {
    importLoading.value = false
  }
}

// Drag to reorder and 上移 / 下移 (direct view): the same order payload as
// flux-panel, { forwards: [{ id, inx }] }, for the rules the user sees.
async function reorderDirectForwards(activeId, overId) {
  const orderedIds = sortedDirectForwards.value.map(item => item.id)
  const oldIndex = orderedIds.indexOf(activeId)
  const newIndex = orderedIds.indexOf(overId)

  if (oldIndex === -1 || newIndex === -1 || oldIndex === newIndex) {
    return
  }

  const newOrder = arrayMove(orderedIds, oldIndex, newIndex)
  forwardOrder.value = newOrder
  saveOrder(newOrder)

  const inxMap = new Map(newOrder.map((id, index) => [id, index]))
  forwards.value = forwards.value.map(item =>
    inxMap.has(item.id) ? { ...item, inx: inxMap.get(item.id) } : item
  )

  try {
    const response = await updateForwardOrder({
      forwards: newOrder.map((id, index) => ({
        id,
        inx: index
      }))
    })

    if (response.code !== 0) {
      setFeedback('error', t('runtime.forward.messages.orderSaveFailed', {
        message: translateMessage(response.msg, 'runtime.forward.messages.unknownError')
      }))
    }
  } catch (error) {
    console.error('Failed to save forward order:', error)
    setFeedback('error', t('runtime.forward.messages.orderSaveRetry'))
  }
}
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

.tunnel-filter {
  display: inline-flex;
  min-width: 160px;
}

.tunnel-filter > :deep(*) {
  flex: 1;
}

.grouped-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.grouped-toolbar {
  display: flex;
  justify-content: flex-end;
}

.grouped-pending {
  min-height: 200px;
}

.editor-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.editor-form__select {
  display: block;
}

.editor-form__targets :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.diagnosis-loading {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: center;
  min-height: 160px;
  color: var(--label-2);
}

@media (max-width: 639.98px) {
  .tunnel-filter {
    flex: 1 1 100%;
  }
}
</style>
