<template>
  <div class="list-page tunnel-page">
    <UiPageHeader :title="t('pageTitles.admin.forwardTunnel')" :description="t('runtime.tunnel.note')">
      <template #meta>
        <UiBadge tone="neutral" :dot="false" :label="t('miscPages.shared.compatibilityEyebrow')" />
        <UiBadge tone="info" :dot="false" :label="runtimeModeLabel" data-test="tunnel-runtime-mode" />
      </template>
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-test="tunnel-add" @click="openCreateModal">{{ t('runtime.tunnel.actions.add') }}</UiButton>
      </template>
    </UiPageHeader>

    <nav class="runtime-context" :aria-label="t('runtime.tunnel.runtimeLinks')">
      <span class="runtime-context__summary">{{ runtimeModeSummary }}</span>
      <span class="runtime-context__links">
        <router-link to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</router-link>
        <router-link to="/admin/forward/nodex">{{ t('forwardSuite.nav.nodeXRuntime') }}</router-link>
      </span>
    </nav>
    <p class="list-page__note runtime-compatibility-note">{{ t('runtime.tunnel.modeCompatibilityHint') }}</p>

    <UiDataTable
      :columns="columns"
      :rows="tunnels"
      :label="t('runtime.tunnel.table.label')"
      :row-label="tunnel => tunnel.name"
      storage-key="admin.forward.tunnels"
      :loading="loading"
      :error="tunnels.length ? null : pageError"
      :error-title="t('runtime.tunnel.messages.loadListFailed')"
      :empty-icon="Waypoints"
      :empty-title="t('runtime.tunnel.emptyTitle')"
      :empty-description="t('runtime.tunnel.emptyText')"
      :row-actions="tunnelActions"
      activatable
      data-test="tunnel-table"
      @row-activate="openEditModal"
      @retry="loadData(true)"
    >
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openCreateModal">{{ t('runtime.tunnel.actions.add') }}</UiButton>
      </template>
      <template #cell-name="{ row }">
        <span class="tunnel-name">{{ row.name }}</span>
      </template>
      <template #cell-type="{ row }">
        <UiBadge :tone="Number(row.type) === 2 ? 'info' : 'neutral'" :dot="false" :label="resolveTypeMeta(row.type).text" />
      </template>
      <template #cell-compatibility="{ row }">
        <UiBadge :tone="resolveRuntimeCompatibility(row).tone" :label="resolveRuntimeCompatibility(row).text" />
      </template>
      <template #cell-ingress="{ row }">
        <span class="node-cell">
          <span>{{ resolveNodeName(row.inNodeId) }}</span>
          <code>{{ row.inIp || '—' }}</code>
        </span>
      </template>
      <template #cell-execution="{ row }">
        <span class="node-cell">
          <span>{{ resolveNodeName(row.outNodeId || row.inNodeId) }}</span>
          <code>{{ row.outIp || row.inIp || '—' }}</code>
        </span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :tone="resolveStatusMeta(row.status).tone" :label="resolveStatusMeta(row.status).text" />
      </template>
    </UiDataTable>

    <UiSheet
      :open="modalOpen"
      size="md"
      :title="isEdit ? t('runtime.tunnel.modal.titleEdit') : t('runtime.tunnel.modal.titleAdd')"
      :description="isEdit ? form.name : runtimeModeLabel"
      :dismissible="!submitLoading"
      data-test="tunnel-editor-sheet"
      @update:open="value => { if (!value) closeEditorModal() }"
    >
      <form id="tunnel-editor-form" class="editor-form" novalidate @submit.prevent="handleSubmit">
        <UiTextField
          v-model.trim="form.name"
          :label="t('runtime.tunnel.fields.name')"
          :placeholder="t('runtime.tunnel.placeholders.name')"
          :error="errors.name"
          maxlength="50"
          required
          size="md"
        />
        <div class="form-grid">
          <UiSelect
            :model-value="Number(form.type)"
            :label="t('runtime.tunnel.fields.tunnelType')"
            :options="typeOptions"
            :disabled="isEdit || !runtimeNodeXMode"
            :error="errors.type"
            size="md"
            @update:model-value="value => { form.type = Number(value) }"
          />
          <UiSelect
            :model-value="Number(form.flow)"
            :label="t('runtime.tunnel.fields.flowAccounting')"
            :options="flowOptions"
            size="md"
            @update:model-value="value => { form.flow = Number(value) }"
          />
        </div>
        <UiNumberField
          v-model="form.trafficRatio"
          :label="t('runtime.tunnel.fields.trafficRatio')"
          :min="0.1"
          :max="100"
          :step="0.1"
          :format-options="{ useGrouping: false, maximumFractionDigits: 2 }"
          unit="x"
          :error="errors.trafficRatio"
          size="md"
        />

        <div v-if="runtimeNodeXMode" data-test="forward-entry-select">
          <UiSelect
            :model-value="form.inNodeId || undefined"
            :label="t('runtime.tunnel.fields.ingressNode')"
            :placeholder="t('runtime.tunnel.validation.ingressRequired')"
            :options="relayNodeSelectOptions('ingress')"
            :disabled="isEdit"
            :help="t('runtime.tunnel.hints.ingressNode')"
            :error="errors.inNodeId"
            required
            size="md"
            @update:model-value="value => { form.inNodeId = Number(value) || 0 }"
          />
        </div>
        <div v-else data-test="forward-execution-select">
          <UiSelect
            :model-value="form.outNodeId || undefined"
            :label="t('runtime.tunnel.fields.executionNode')"
            :placeholder="t('runtime.tunnel.validation.executionRequired')"
            :options="relayNodeSelectOptions('execution')"
            :disabled="isEdit"
            :help="t('runtime.tunnel.hints.executionNode')"
            :error="errors.outNodeId"
            required
            size="md"
            @update:model-value="value => { form.outNodeId = Number(value) || 0 }"
          />
        </div>

        <div v-if="runtimeNodeXMode && form.type === 2" class="form-grid">
          <UiSelect
            v-model="form.protocol"
            :label="t('runtime.tunnel.fields.protocol')"
            :options="protocolOptions"
            :error="errors.protocol"
            size="md"
          />
          <div data-test="forward-exit-select">
            <UiSelect
              :model-value="form.outNodeId || undefined"
              :label="t('runtime.tunnel.fields.egressNode')"
              :placeholder="t('runtime.tunnel.validation.egressRequired')"
              :options="exitNodeSelectOptions"
              :disabled="isEdit"
              :error="errors.outNodeId"
              required
              size="md"
              @update:model-value="value => { form.outNodeId = Number(value) || 0 }"
            />
          </div>
          <p class="editor-form__hint form-grid__full">{{ t('runtime.tunnel.hints.egressNode') }}</p>
        </div>

        <div class="form-grid">
          <UiTextField
            v-model.trim="form.tcpListenAddr"
            :label="t('runtime.tunnel.fields.tcpListenAddr')"
            placeholder="[::]"
            :error="errors.tcpListenAddr"
            required
            size="md"
          />
          <UiTextField
            v-model.trim="form.udpListenAddr"
            :label="t('runtime.tunnel.fields.udpListenAddr')"
            placeholder="[::]"
            :error="errors.udpListenAddr"
            required
            size="md"
          />
        </div>
        <UiTextField
          v-if="form.type === 2"
          v-model.trim="form.interfaceName"
          :label="t('runtime.tunnel.fields.interfaceName')"
          :placeholder="t('runtime.tunnel.placeholders.interfaceName')"
          size="md"
        />
      </form>
      <template #footer>
        <UiButton :disabled="submitLoading" @click="closeEditorModal">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton type="submit" form="tunnel-editor-form" variant="primary" :loading="submitLoading" data-test="tunnel-editor-submit">
          {{ isEdit ? t('runtime.tunnel.modal.submitUpdate') : t('runtime.tunnel.modal.submitCreate') }}
        </UiButton>
      </template>
    </UiSheet>

    <UiConfirmDialog
      :open="deleteModalOpen"
      tone="danger"
      :title="t('runtime.tunnel.modal.deleteConfirmMessage', { name: tunnelToDelete?.name || '-' })"
      :message="t('runtime.tunnel.modal.deleteHint')"
      :confirm-label="t('runtime.tunnel.modal.confirmDelete')"
      :loading="deleteLoading"
      :error="deleteError"
      @confirm="confirmDelete"
      @cancel="closeDeleteModal"
    />

    <UiSheet
      v-model:open="diagnosisModalOpen"
      size="md"
      :title="t('runtime.tunnel.diagnosis.title')"
      :description="currentDiagnosisTunnel?.name || ''"
      data-test="tunnel-diagnosis-sheet"
    >
      <div v-if="diagnosisLoading" class="diagnosis-loading" role="status">
        <UiSpinner />
        <span>{{ t('runtime.tunnel.diagnosis.loading') }}</span>
      </div>
      <DiagnosisTimeline
        v-else-if="diagnosisSteps.length"
        :steps="diagnosisSteps"
        :label="t('runtime.tunnel.diagnosis.title')"
        :summary="diagnosisSummary"
        :checked-at="diagnosisCheckedAt"
      />
      <UiEmptyState
        v-else
        compact
        :icon="Stethoscope"
        :title="t('runtime.tunnel.diagnosis.emptyTitle')"
        :description="t('runtime.tunnel.diagnosis.emptyText')"
      />
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('common.actions.close') }}</UiButton>
        <UiButton variant="primary" :icon="RotateCw" :loading="diagnosisLoading" @click="rerunDiagnosis">
          {{ t('runtime.tunnel.diagnosis.rerun') }}
        </UiButton>
      </template>
    </UiSheet>
  </div>
</template>

<script setup>
// 隧道 (/admin/forward/tunnel): flux-panel tunnel.tsx with the local
// dual-runtime fields. UI U7 changed visuals and interaction components
// only: the tunnels are a UiDataTable, the editor and the diagnosis are
// Sheets (the diagnosis as a timeline). Same endpoints and fields.
import { computed, onMounted, reactive, ref } from 'vue'
import { Pencil, Plus, RotateCw, Stethoscope, Trash2, Waypoints } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  createForwardTunnel,
  deleteForwardTunnel,
  diagnoseForwardTunnel,
  getAdminForwardTunnelList,
  getAnsibleMachines,
  getForwardNodes,
  getSystemConfig,
  updateForwardTunnel
} from '@/api/admin'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiConfirmDialog from '@/ui/UiConfirmDialog.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSpinner from '@/ui/UiSpinner.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import DiagnosisTimeline from '@/components/admin/forward/DiagnosisTimeline.vue'

const { t, translateLiteral } = useAppI18n()

const format = useFormat()
const loading = ref(true)
const pageError = ref(null)
const tunnels = ref([])
const nodes = ref([])
const runtimeNodeXModeKey = 'forward.runtime.nodex_mode'
const runtimeBackendKey = 'forward.runtime_backend'
const runtimeNodeXMode = ref(false)
const runtimeBackend = ref('nftables_ansible')

const relayNodeOptions = computed(() =>
  nodes.value.filter(
    node => node.id > 0 && String(node.type ?? '').toLowerCase() === 'relay'
  )
)

const exitNodeOptions = computed(() =>
  nodes.value.filter(
    node => node.id > 0 && String(node.type ?? '').toLowerCase() === 'exit'
  )
)

const runtimeModeLabel = computed(() =>
  runtimeNodeXMode.value
    ? t('runtime.tunnel.modeLabelNodeX')
    : t('runtime.tunnel.modeLabelLocal', { backend: humanizeForwardRuntimeBackend(t, runtimeBackend.value) })
)
const runtimeModeSummary = computed(() =>
  runtimeNodeXMode.value
    ? t('runtime.tunnel.modeSummaryNodeX')
    : t('runtime.tunnel.modeSummaryLocal')
)

// Interaction components (UI U7) ----------------------------------------
const columns = computed(() => [
  { key: 'name', label: t('runtime.tunnel.fields.name'), primary: true, hideable: false },
  { key: 'type', label: t('runtime.tunnel.fields.tunnelType'), secondary: true, value: tunnel => resolveTypeMeta(tunnel.type).text },
  { key: 'compatibility', label: t('runtime.tunnel.table.compatibility'), value: tunnel => resolveRuntimeCompatibility(tunnel).text },
  ...(runtimeNodeXMode.value
    ? [{ key: 'ingress', label: t('runtime.tunnel.meta.ingressNode'), value: tunnel => resolveNodeName(tunnel.inNodeId), breakpoint: 'md' }]
    : []),
  {
    key: 'execution',
    label: runtimeNodeXMode.value ? t('runtime.tunnel.meta.egressNode') : t('runtime.tunnel.meta.executionNode'),
    value: tunnel => resolveNodeName(tunnel.outNodeId || tunnel.inNodeId)
  },
  { key: 'flow', label: t('runtime.tunnel.meta.flowAccounting'), value: tunnel => resolveFlowLabel(tunnel.flow), breakpoint: 'lg', card: false },
  { key: 'trafficRatio', label: t('runtime.tunnel.meta.trafficRatio'), numeric: true, value: tunnel => formatTrafficRatio(tunnel.trafficRatio), breakpoint: 'lg', card: false },
  { key: 'status', label: t('runtime.tunnel.table.status'), value: tunnel => resolveStatusMeta(tunnel.status).text }
])
const tunnelActions = tunnel => [
  { key: 'edit', label: t('runtime.tunnel.actions.edit'), icon: Pencil, onSelect: () => openEditModal(tunnel) },
  { key: 'diagnose', label: t('runtime.tunnel.actions.diagnose'), icon: Stethoscope, onSelect: () => openDiagnosisModal(tunnel) },
  { key: 'delete', label: t('runtime.tunnel.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => openDeleteModal(tunnel) }
]
const typeOptions = computed(() => [
  { value: 1, label: t('runtime.tunnel.options.portForward') },
  { value: 2, label: t('runtime.tunnel.options.tunnelForward') }
])
const flowOptions = computed(() => [
  { value: 1, label: t('runtime.tunnel.options.oneWayAccounting') },
  { value: 2, label: t('runtime.tunnel.options.twoWayAccounting') }
])
const protocolOptions = ['tls', 'tcp', 'udp', 'ws', 'wss', 'grpc', 'quic']
function relayNodeSelectOptions(role) {
  const roleLabel = role === 'ingress' ? t('runtime.tunnel.meta.ingressNode') : t('runtime.tunnel.meta.executionNode')
  return relayNodeOptions.value.map(node => ({ value: node.id, label: `${node.name} · ${roleLabel} · ${node.host}` }))
}
const exitNodeSelectOptions = computed(() => exitNodeOptions.value.map(node => ({
  value: node.id,
  label: `${node.name} · ${t('runtime.tunnel.meta.egressNode')} · ${node.host}`
})))
const diagnosisSteps = computed(() => (diagnosisResult.value?.results || []).map((result, index) => {
  const fields = [{ label: t('runtime.tunnel.diagnosis.targetAddress'), value: formatAddress(result.targetIp, result.targetPort), mono: true }]
  if (result.averageTime) {
    fields.push({ label: t('runtime.tunnel.diagnosis.duration'), value: `${result.averageTime.toFixed(0)} ms`, numeric: true })
  }
  return {
    key: `${result.nodeId}-${index}`,
    title: result.description,
    meta: `${result.nodeName} · Node ${result.nodeId}`,
    success: Boolean(result.success),
    statusLabel: result.success ? t('runtime.shared.success') : t('runtime.shared.failed'),
    fields,
    message: result.message ? translateLiteral(result.message) : ''
  }
}))
const diagnosisSummary = computed(() => {
  const steps = diagnosisSteps.value
  if (!steps.length) return ''
  return t('runtime.tunnel.diagnosis.summary', { passed: steps.filter(step => step.success).length, total: steps.length })
})
const diagnosisCheckedAt = computed(() => (diagnosisResult.value?.timestamp ? format.dateTime(diagnosisResult.value.timestamp) : ''))

const modalOpen = ref(false)
const deleteModalOpen = ref(false)
const diagnosisModalOpen = ref(false)
const isEdit = ref(false)

const submitLoading = ref(false)
const deleteLoading = ref(false)
const diagnosisLoading = ref(false)

const tunnelToDelete = ref(null)
const deleteError = ref('')
const currentDiagnosisTunnel = ref(null)
const diagnosisResult = ref(null)


const form = reactive(createDefaultForm())
const errors = reactive({
  name: '',
  type: '',
  inNodeId: '',
  outNodeId: '',
  trafficRatio: '',
  protocol: '',
  tcpListenAddr: '',
  udpListenAddr: ''
})

const enforceFormMode = () => {
  if (!runtimeNodeXMode.value) {
    form.type = 1
    form.inNodeId = 0
    form.outNodeId = 0
    form.protocol = ''
  }
}

const parseBooleanConfig = (value) => {
  if (value === undefined || value === null) {
    return null
  }
  if (typeof value === 'boolean') {
    return value
  }
  const normalized = String(value).trim().toLowerCase()
  if (!normalized) {
    return null
  }
  return ['1', 'true', 'yes', 'on', 'enabled'].includes(normalized)
}

async function loadRuntimeMode() {
  let explicitMode = null
  try {
    const res = await getSystemConfig(runtimeNodeXModeKey)
    explicitMode = parseBooleanConfig(res.data?.value)
  } catch (err) {
    console.error('Failed to fetch runtime NodeX mode:', err)
  }

  try {
    const res = await getSystemConfig(runtimeBackendKey)
    runtimeBackend.value = String(res.data?.value || 'nftables_ansible').toLowerCase() || 'nftables_ansible'
    runtimeNodeXMode.value = explicitMode === null ? runtimeBackend.value === 'gost' : explicitMode
    enforceFormMode()
  } catch (err) {
    console.error('Failed to fetch runtime backend:', err)
  }
}

onMounted(async () => {
  await loadRuntimeMode()
  await loadData(true)
})

function createDefaultForm() {
  return {
    id: null,
    name: '',
    type: 1,
    inNodeId: 0,
    outNodeId: 0,
    flow: 1,
    trafficRatio: 1,
    protocol: 'tls',
    tcpListenAddr: '[::]',
    udpListenAddr: '[::]',
    interfaceName: ''
  }
}

function resetForm() {
  Object.assign(form, createDefaultForm())
  clearErrors()
}

function clearErrors() {
  errors.name = ''
  errors.type = ''
  errors.inNodeId = ''
  errors.outNodeId = ''
  errors.trafficRatio = ''
  errors.protocol = ''
  errors.tcpListenAddr = ''
  errors.udpListenAddr = ''
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

function translateMessage(value, fallback = '') {
  const text = String(value ?? '').trim()
  if (!text) return fallback
  return translateLiteral(text)
}

function normalizeTunnel(raw) {
  return {
    ...raw,
    id: Number(raw.id ?? 0),
    inNodeId: Number(raw.inNodeId ?? raw.in_node_id ?? 0),
    outNodeId: raw.outNodeId == null && raw.out_node_id == null ? null : Number(raw.outNodeId ?? raw.out_node_id),
    type: Number(raw.type ?? 1),
    flow: Number(raw.flow ?? 1),
    trafficRatio: Number(raw.trafficRatio ?? raw.traffic_ratio ?? 1),
    protocol: raw.protocol ?? '',
    tcpListenAddr: raw.tcpListenAddr ?? raw.tcp_listen_addr ?? '[::]',
    udpListenAddr: raw.udpListenAddr ?? raw.udp_listen_addr ?? '[::]',
    interfaceName: raw.interfaceName ?? raw.interface_name ?? '',
    inIp: raw.inIp ?? raw.in_ip ?? '',
    outIp: raw.outIp ?? raw.out_ip ?? '',
    status: Number(raw.status ?? 1)
  }
}

function normalizeNode(raw) {
  return {
    ...raw,
    id: Number(raw.id ?? 0),
    host: raw.host ?? '',
    status: Number(raw.status ?? 0)
  }
}

function extractNodeList(response) {
  const list = response?.list ?? response?.data?.list ?? response?.data ?? []
  return Array.isArray(list) ? list.map(normalizeNode) : []
}

async function loadData(showLoading = true) {
  if (showLoading) {
    loading.value = true
  }

  try {
    const inventoryPromise = runtimeNodeXMode.value
      ? getForwardNodes({ page_size: 200, scope: 'nodex' })
      : getAnsibleMachines({ page_size: 200, type: 'relay' })

    const [tunnelRes, nodeRes] = await Promise.all([getAdminForwardTunnelList(), inventoryPromise])

    if (tunnelRes.code === 0) {
      tunnels.value = Array.isArray(tunnelRes.data) ? tunnelRes.data.map(normalizeTunnel) : []
      pageError.value = null
    } else {
      reportLoadError(translateMessage(tunnelRes.msg, t('runtime.tunnel.messages.loadListFailed')))
    }

    nodes.value = extractNodeList(nodeRes)
  } catch (error) {
    console.error('Failed to load tunnel page data:', error)
    reportLoadError(error?.response ? error : t('runtime.tunnel.messages.loadDataFailed'))
  } finally {
    loading.value = false
  }
}

// Nothing listed yet: the table's error state (重试); otherwise a toast.
function reportLoadError(error) {
  if (tunnels.value.length) {
    setFeedback('error', typeof error === 'string' ? error : t('runtime.tunnel.messages.loadDataFailed'))
    return
  }
  pageError.value = error
}

function resolveNodeName(nodeId) {
  const match = nodes.value.find(node => node.id === Number(nodeId))
  return match?.name || (nodeId ? t('miscPages.shared.nodeNumber', { id: nodeId }) : '-')
}

function resolveTypeMeta(type) {
  return Number(type) === 2
    ? { text: t('runtime.tunnel.options.tunnelForward'), tone: 'info' }
    : { text: t('runtime.tunnel.options.portForward'), tone: 'neutral' }
}

function resolveStatusMeta(status) {
  return Number(status) === 1
    ? { text: t('runtime.shared.enabled'), tone: 'success' }
    : { text: t('runtime.shared.disabled'), tone: 'danger' }
}

function resolveFlowLabel(flow) {
  return Number(flow) === 2 ? t('runtime.tunnel.options.twoWayAccounting') : t('runtime.tunnel.options.oneWayAccounting')
}

function resolveRuntimeCompatibility(tunnel) {
  const executionNodeId = Number(tunnel?.outNodeId || tunnel?.inNodeId || 0)

  if (runtimeNodeXMode.value) {
    if (!Number(tunnel?.inNodeId || 0)) {
      return { text: t('runtime.tunnel.compatibility.nodeXNeedsIngress'), tone: 'danger' }
    }
    if (Number(tunnel?.type) === 2 && !Number(tunnel?.outNodeId || 0)) {
      return { text: t('runtime.tunnel.compatibility.nodeXNeedsEgress'), tone: 'danger' }
    }
    return { text: t('runtime.tunnel.compatibility.nodeXReady'), tone: 'success' }
  }

  if (Number(tunnel?.type) !== 1) {
    return { text: t('runtime.tunnel.compatibility.localOnlyPortForward'), tone: 'danger' }
  }
  if (!executionNodeId) {
    return { text: t('runtime.tunnel.compatibility.localNeedsExecution'), tone: 'danger' }
  }
  return { text: t('runtime.tunnel.compatibility.localReady'), tone: 'success' }
}

function formatTrafficRatio(value) {
  const ratio = Number(value ?? 1)
  return `${ratio.toFixed(ratio % 1 === 0 ? 1 : 2)}x`
}

function formatAddress(host, port) {
  if (!host) {
    return '-'
  }
  return port ? `${host}:${port}` : host
}

function openCreateModal() {
  isEdit.value = false
  resetForm()
  if (!runtimeNodeXMode.value) {
    form.type = 1
    form.inNodeId = 0
    form.outNodeId = 0
  }
  modalOpen.value = true
}

function openEditModal(tunnel) {
  isEdit.value = true
  Object.assign(form, {
    id: tunnel.id,
    name: tunnel.name,
    type: tunnel.type,
    inNodeId: tunnel.inNodeId,
    outNodeId: tunnel.outNodeId || tunnel.inNodeId || 0,
    flow: tunnel.flow,
    trafficRatio: tunnel.trafficRatio,
    protocol: tunnel.protocol || 'tls',
    tcpListenAddr: tunnel.tcpListenAddr || '[::]',
    udpListenAddr: tunnel.udpListenAddr || '[::]',
    interfaceName: tunnel.interfaceName || ''
  })
  clearErrors()
  modalOpen.value = true
}

function closeEditorModal() {
  if (submitLoading.value) return
  modalOpen.value = false
}

function validateForm() {
  clearErrors()

  if (!form.name.trim()) {
    errors.name = t('runtime.tunnel.validation.nameRequired')
  } else if (form.name.trim().length < 2 || form.name.trim().length > 50) {
    errors.name = t('runtime.tunnel.validation.nameLength')
  }

  if (![1, 2].includes(Number(form.type))) {
    errors.type = t('runtime.tunnel.validation.typeInvalid')
  }

  if (runtimeNodeXMode.value) {
    if (!form.inNodeId) {
      errors.inNodeId = t('runtime.tunnel.validation.ingressRequired')
    } else if (!relayNodeOptions.value.some(node => node.id === Number(form.inNodeId))) {
      errors.inNodeId = t('runtime.tunnel.validation.ingressMustRelay')
    }
  }

  const trafficRatio = Number(form.trafficRatio)
  if (!Number.isFinite(trafficRatio) || trafficRatio <= 0 || trafficRatio > 100) {
    errors.trafficRatio = t('runtime.tunnel.validation.trafficRatio')
  }

  if (!String(form.tcpListenAddr || '').trim()) {
    errors.tcpListenAddr = t('runtime.tunnel.validation.tcpListenRequired')
  }

  if (!String(form.udpListenAddr || '').trim()) {
    errors.udpListenAddr = t('runtime.tunnel.validation.udpListenRequired')
  }

  if (runtimeNodeXMode.value && Number(form.type) === 2) {
    if (!form.outNodeId) {
      errors.outNodeId = t('runtime.tunnel.validation.egressRequired')
    } else if (Number(form.outNodeId) === Number(form.inNodeId)) {
      errors.outNodeId = t('runtime.tunnel.validation.ingressEgressDifferent')
    } else if (!exitNodeOptions.value.some(node => node.id === Number(form.outNodeId))) {
      errors.outNodeId = t('runtime.tunnel.validation.egressMustExit')
    }

    if (!String(form.protocol || '').trim()) {
      errors.protocol = t('runtime.tunnel.validation.protocolRequired')
    }
  }

  if (!runtimeNodeXMode.value) {
    if (!form.outNodeId) {
      errors.outNodeId = t('runtime.tunnel.validation.executionRequired')
    } else if (!relayNodeOptions.value.some(node => node.id === Number(form.outNodeId))) {
      errors.outNodeId = t('runtime.tunnel.validation.executionMustRelay')
    }
  }

  return Object.values(errors).every(value => !value)
}

async function handleSubmit() {
  if (!validateForm()) {
    return
  }

  submitLoading.value = true
  try {
    const payload = {
      name: form.name.trim(),
      flow: Number(form.flow),
      trafficRatio: Number(form.trafficRatio),
      protocol: String(form.protocol || 'tls').trim(),
      tcpListenAddr: String(form.tcpListenAddr || '[::]').trim(),
      udpListenAddr: String(form.udpListenAddr || '[::]').trim(),
      interfaceName: String(form.interfaceName || '').trim()
    }

    const typeValue = runtimeNodeXMode.value ? Number(form.type) : 1
    const normalizedOutNode = runtimeNodeXMode.value
      ? Number(form.type) === 2
        ? Number(form.outNodeId)
        : null
      : Number(form.outNodeId) || null
    const normalizedInNode = runtimeNodeXMode.value ? Number(form.inNodeId) : 0
    const protocolValue = runtimeNodeXMode.value && Number(form.type) === 2 ? payload.protocol : ''

    const requestPayload = {
      ...payload,
      type: typeValue,
      inNodeId: normalizedInNode,
      outNodeId: normalizedOutNode,
      protocol: protocolValue
    }

    const response = isEdit.value
      ? await updateForwardTunnel({
          id: form.id,
          ...requestPayload
        })
      : await createForwardTunnel(requestPayload)

    if (response.code === 0) {
      modalOpen.value = false
      setFeedback('success', isEdit.value ? t('runtime.tunnel.messages.updated') : t('runtime.tunnel.messages.created'))
      await loadData(false)
      return
    }

    setFeedback('error', translateMessage(response.msg, t('runtime.tunnel.messages.actionFailed')))
  } catch (error) {
    console.error('Failed to submit tunnel:', error)
    setFeedback('error', t('runtime.tunnel.messages.actionFailed'))
  } finally {
    submitLoading.value = false
  }
}

function openDeleteModal(tunnel) {
  tunnelToDelete.value = tunnel
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
  if (!tunnelToDelete.value) {
    return
  }

  deleteLoading.value = true
  deleteError.value = ''
  try {
    const response = await deleteForwardTunnel(tunnelToDelete.value.id)
    if (response.code === 0) {
      deleteModalOpen.value = false
      tunnelToDelete.value = null
      setFeedback('success', t('runtime.tunnel.messages.deleted'))
      await loadData(false)
      return
    }
    // The reason shows in the open confirmation (e.g. still referenced).
    deleteError.value = translateMessage(response.msg, t('runtime.tunnel.messages.deleteFailed'))
  } catch (error) {
    console.error('Failed to delete tunnel:', error)
    deleteError.value = t('runtime.tunnel.messages.deleteFailed')
  } finally {
    deleteLoading.value = false
  }
}

async function runDiagnosis(tunnel) {
  diagnosisLoading.value = true
  diagnosisResult.value = null
  try {
    const response = await diagnoseForwardTunnel(tunnel.id)
    if (response.code === 0) {
      diagnosisResult.value = response.data
      return
    }
    diagnosisResult.value = {
      tunnelName: tunnel.name,
      tunnelType: resolveTypeMeta(tunnel.type).text,
      timestamp: Date.now(),
      results: [
        {
          success: false,
          description: t('runtime.tunnel.messages.diagnosis'),
          nodeName: '-',
          nodeId: '-',
          targetIp: '-',
          message: translateMessage(response.msg, t('runtime.tunnel.messages.diagnosisFailed'))
        }
      ]
    }
  } catch (error) {
    console.error('Failed to diagnose tunnel:', error)
    diagnosisResult.value = {
      tunnelName: tunnel.name,
      tunnelType: resolveTypeMeta(tunnel.type).text,
      timestamp: Date.now(),
      results: [
        {
          success: false,
          description: t('runtime.tunnel.messages.diagnosis'),
          nodeName: '-',
          nodeId: '-',
          targetIp: '-',
          message: t('runtime.tunnel.messages.diagnosisRequestFailed')
        }
      ]
    }
  } finally {
    diagnosisLoading.value = false
  }
}

async function openDiagnosisModal(tunnel) {
  currentDiagnosisTunnel.value = tunnel
  diagnosisModalOpen.value = true
  await runDiagnosis(tunnel)
}

async function rerunDiagnosis() {
  if (!currentDiagnosisTunnel.value) {
    return
  }
  await runDiagnosis(currentDiagnosisTunnel.value)
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

.runtime-compatibility-note {
  margin-top: calc(-1 * var(--space-4));
}

.tunnel-name {
  font-weight: var(--weight-medium);
  overflow-wrap: anywhere;
}

.node-cell {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.node-cell code {
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

.editor-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.editor-form__hint {
  margin: calc(-1 * var(--space-2)) 0 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.diagnosis-loading {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: center;
  min-height: 160px;
  color: var(--label-2);
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .runtime-context__links a {
    position: relative;
  }

  .runtime-context__links a::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
