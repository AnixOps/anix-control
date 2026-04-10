<template>
  <div class="tunnel-page">
    <div class="toolbar">
      <div class="toolbar-copy">
        <p class="eyebrow">Flux Compatible</p>
        <h2>{{ t('pageTitles.admin.forwardTunnel') }}</h2>
      </div>
      <div class="toolbar-actions">
        <button class="btn btn-primary" @click="openCreateModal">{{ t('runtime.tunnel.actions.add') }}</button>
      </div>
    </div>
    <ForwardSuiteNav />
    <p class="text-secondary small runtime-note">{{ t('runtime.tunnel.note') }}</p>
    <div class="runtime-context-bar">
      <span class="tag tag-primary">{{ runtimeModeLabel }}</span>
      <span class="runtime-context-summary">{{ runtimeModeSummary }}</span>
      <div class="runtime-context-links">
        <router-link class="btn btn-secondary btn-sm" to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</router-link>
        <router-link class="btn btn-secondary btn-sm" to="/admin/forward/nodex">{{ t('forwardSuite.nav.nodeXRuntime') }}</router-link>
      </div>
    </div>
    <p class="text-secondary small runtime-note runtime-compatibility-note">
      {{ t('runtime.tunnel.modeCompatibilityHint') }}
    </p>

    <div v-if="feedback.message" :class="['feedback', `feedback-${feedback.type}`]">
      <span>{{ feedback.message }}</span>
      <button class="feedback-close" @click="clearFeedback">×</button>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>{{ t('runtime.tunnel.loading') }}</span>
    </div>

    <template v-else>
      <section v-if="tunnels.length" class="card-grid">
        <article v-for="tunnel in tunnels" :key="tunnel.id" class="tunnel-card">
          <div class="card-head">
            <div class="card-title">
              <h3>{{ tunnel.name }}</h3>
              <p>{{ resolveTypeMeta(tunnel.type).text }}</p>
              <p class="card-mode">{{ resolveRuntimeCompatibility(tunnel).text }}</p>
            </div>
            <div class="card-head-actions">
              <span :class="['tag', resolveTypeMeta(tunnel.type).className]">
                {{ resolveTypeMeta(tunnel.type).text }}
              </span>
              <span :class="['tag', resolveRuntimeCompatibility(tunnel).className]">
                {{ resolveRuntimeCompatibility(tunnel).text }}
              </span>
              <span :class="['tag', resolveStatusMeta(tunnel.status).className]">
                {{ resolveStatusMeta(tunnel.status).text }}
              </span>
            </div>
          </div>

          <div class="meta-list">
            <div v-if="runtimeNodeXMode" class="meta-item">
              <span class="meta-label">{{ t('runtime.tunnel.meta.ingressNode') }}</span>
              <strong>{{ resolveNodeName(tunnel.inNodeId) }}</strong>
              <code>{{ tunnel.inIp || '-' }}</code>
            </div>
            <div class="meta-item">
              <span class="meta-label">
                {{ runtimeNodeXMode ? t('runtime.tunnel.meta.egressNode') : t('runtime.tunnel.meta.executionNode') }}
              </span>
              <strong>{{ resolveNodeName(tunnel.outNodeId || tunnel.inNodeId) }}</strong>
              <code>{{ tunnel.outIp || tunnel.inIp || '-' }}</code>
            </div>
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.tunnel.meta.flowAccounting') }}</span>
              <strong>{{ resolveFlowLabel(tunnel.flow) }}</strong>
            </div>
            <div class="meta-item">
              <span class="meta-label">{{ t('runtime.tunnel.meta.trafficRatio') }}</span>
              <strong>{{ formatTrafficRatio(tunnel.trafficRatio) }}</strong>
            </div>
          </div>

          <div class="card-actions">
            <button class="btn btn-secondary btn-sm" @click="openEditModal(tunnel)">{{ t('runtime.tunnel.actions.edit') }}</button>
            <button class="btn btn-secondary btn-sm" @click="openDiagnosisModal(tunnel)">{{ t('runtime.tunnel.actions.diagnose') }}</button>
            <button class="btn btn-secondary btn-sm danger-text" @click="openDeleteModal(tunnel)">{{ t('runtime.tunnel.actions.delete') }}</button>
          </div>
        </article>
      </section>

      <section v-else class="empty-state">
        <h3>{{ t('runtime.tunnel.emptyTitle') }}</h3>
        <p>{{ t('runtime.tunnel.emptyText') }}</p>
      </section>
    </template>

    <div v-if="modalOpen" class="modal-overlay" @click.self="closeEditorModal">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div>
            <p class="eyebrow">{{ t('runtime.tunnel.modal.eyebrow') }}</p>
            <h3>{{ isEdit ? t('runtime.tunnel.modal.titleEdit') : t('runtime.tunnel.modal.titleAdd') }}</h3>
          </div>
          <button class="modal-close" @click="closeEditorModal">×</button>
        </div>

        <div class="modal-body">
          <div class="form-grid">
            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.name') }}</label>
              <input v-model.trim="form.name" type="text" maxlength="50" :placeholder="t('runtime.tunnel.placeholders.name')" />
              <p v-if="errors.name" class="form-error">{{ errors.name }}</p>
            </div>

            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.tunnelType') }}</label>
              <select v-model.number="form.type" :disabled="isEdit || !runtimeNodeXMode">
                <option :value="1">{{ t('runtime.tunnel.options.portForward') }}</option>
                <option :value="2">{{ t('runtime.tunnel.options.tunnelForward') }}</option>
              </select>
              <p v-if="errors.type" class="form-error">{{ errors.type }}</p>
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.flowAccounting') }}</label>
              <select v-model.number="form.flow">
                <option :value="1">{{ t('runtime.tunnel.options.oneWayAccounting') }}</option>
                <option :value="2">{{ t('runtime.tunnel.options.twoWayAccounting') }}</option>
              </select>
            </div>

            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.trafficRatio') }}</label>
              <input v-model.number="form.trafficRatio" type="number" min="0.1" max="100" step="0.1" />
              <p v-if="errors.trafficRatio" class="form-error">{{ errors.trafficRatio }}</p>
            </div>
          </div>

          <div class="form-grid">
            <div v-if="runtimeNodeXMode" class="form-group">
              <label>{{ t('runtime.tunnel.fields.ingressNode') }}</label>
              <select
                data-test="forward-entry-select"
                v-model.number="form.inNodeId"
                :disabled="isEdit"
              >
                <option :value="0">{{ t('runtime.tunnel.validation.ingressRequired') }}</option>
                <option v-for="node in relayNodeOptions" :key="node.id" :value="node.id">
                  {{ node.name }} · {{ t('runtime.tunnel.meta.ingressNode') }} · {{ node.host }}
                </option>
              </select>
              <p class="hint">{{ t('runtime.tunnel.hints.ingressNode') }}</p>
              <p v-if="errors.inNodeId" class="form-error">{{ errors.inNodeId }}</p>
            </div>
            <div v-else class="form-group">
              <label>{{ t('runtime.tunnel.fields.executionNode') }}</label>
              <select data-test="forward-execution-select" v-model.number="form.outNodeId" :disabled="isEdit">
                <option :value="0">{{ t('runtime.tunnel.validation.executionRequired') }}</option>
                <option
                  v-for="node in relayNodeOptions"
                  :key="`exec-${node.id}`"
                  :value="node.id"
                >
                  {{ node.name }} · {{ t('runtime.tunnel.meta.executionNode') }} · {{ node.host }}
                </option>
              </select>
              <p class="hint">{{ t('runtime.tunnel.hints.executionNode') }}</p>
              <p v-if="errors.outNodeId" class="form-error">{{ errors.outNodeId }}</p>
            </div>

            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.tcpListenAddr') }}</label>
              <input v-model.trim="form.tcpListenAddr" type="text" placeholder="[::]" />
              <p v-if="errors.tcpListenAddr" class="form-error">{{ errors.tcpListenAddr }}</p>
            </div>
          </div>

          <div class="form-grid">
            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.udpListenAddr') }}</label>
              <input v-model.trim="form.udpListenAddr" type="text" placeholder="[::]" />
              <p v-if="errors.udpListenAddr" class="form-error">{{ errors.udpListenAddr }}</p>
            </div>

            <div v-if="form.type === 2" class="form-group">
              <label>{{ t('runtime.tunnel.fields.interfaceName') }}</label>
              <input v-model.trim="form.interfaceName" type="text" :placeholder="t('runtime.tunnel.placeholders.interfaceName')" />
            </div>
          </div>

          <div v-if="runtimeNodeXMode && form.type === 2" class="form-grid">
            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.protocol') }}</label>
              <select v-model="form.protocol">
                <option value="tls">tls</option>
                <option value="tcp">tcp</option>
                <option value="udp">udp</option>
                <option value="ws">ws</option>
                <option value="wss">wss</option>
                <option value="grpc">grpc</option>
                <option value="quic">quic</option>
              </select>
              <p v-if="errors.protocol" class="form-error">{{ errors.protocol }}</p>
            </div>

            <div class="form-group">
              <label>{{ t('runtime.tunnel.fields.egressNode') }}</label>
              <select
                data-test="forward-exit-select"
                v-model.number="form.outNodeId"
                :disabled="isEdit"
              >
                <option :value="0">{{ t('runtime.tunnel.validation.egressRequired') }}</option>
                <option
                  v-for="node in exitNodeOptions"
                  :key="`out-${node.id}`"
                  :value="node.id"
                >
                  {{ node.name }} · {{ t('runtime.tunnel.meta.egressNode') }} · {{ node.host }}
                </option>
              </select>
              <p class="hint">{{ t('runtime.tunnel.hints.egressNode') }}</p>
              <p v-if="errors.outNodeId" class="form-error">{{ errors.outNodeId }}</p>
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeEditorModal">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary" :disabled="submitLoading" @click="handleSubmit">
            {{ submitLoading ? t('runtime.tunnel.modal.submitLoading') : (isEdit ? t('runtime.tunnel.modal.submitUpdate') : t('runtime.tunnel.modal.submitCreate')) }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="deleteModalOpen" class="modal-overlay" @click.self="deleteModalOpen = false">
      <div class="modal">
        <div class="modal-header">
          <div>
            <p class="eyebrow">{{ t('runtime.tunnel.modal.deleteEyebrow') }}</p>
            <h3>{{ t('runtime.tunnel.modal.deleteTitle') }}</h3>
          </div>
          <button class="modal-close" @click="deleteModalOpen = false">×</button>
        </div>
        <div class="modal-body">
          <p class="modal-copy">{{ t('runtime.tunnel.modal.deleteConfirmMessage', { name: tunnelToDelete?.name || '-' }) }}</p>
          <p class="hint">{{ t('runtime.tunnel.modal.deleteHint') }}</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="deleteModalOpen = false">{{ t('common.actions.cancel') }}</button>
          <button class="btn btn-primary danger" :disabled="deleteLoading" @click="confirmDelete">
            {{ deleteLoading ? t('runtime.tunnel.modal.deleteLoading') : t('runtime.tunnel.modal.confirmDelete') }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="diagnosisModalOpen" class="modal-overlay" @click.self="diagnosisModalOpen = false">
      <div class="modal modal-xl">
        <div class="modal-header">
          <div>
            <p class="eyebrow">{{ t('runtime.tunnel.diagnosis.eyebrow') }}</p>
            <h3>{{ t('runtime.tunnel.diagnosis.title') }}</h3>
            <p v-if="currentDiagnosisTunnel" class="modal-subtitle">{{ currentDiagnosisTunnel.name }}</p>
          </div>
          <button class="modal-close" @click="diagnosisModalOpen = false">×</button>
        </div>

        <div class="modal-body">
          <div v-if="diagnosisLoading" class="loading-state compact">
            <div class="spinner"></div>
            <span>{{ t('runtime.tunnel.diagnosis.loading') }}</span>
          </div>

          <div v-else-if="diagnosisResult?.results?.length" class="diagnosis-list">
            <article v-for="(result, index) in diagnosisResult.results" :key="`${result.nodeId}-${index}`" class="diagnosis-card">
              <div class="diagnosis-head">
                <div>
                  <h4>{{ result.description }}</h4>
                  <p>{{ result.nodeName }} · Node {{ result.nodeId }}</p>
                </div>
                <span :class="['tag', result.success ? 'tag-success' : 'tag-danger']">
                  {{ result.success ? t('runtime.shared.success') : t('runtime.shared.failed') }}
                </span>
              </div>

              <div class="diagnosis-meta">
                <div>
                  <span class="meta-label">{{ t('runtime.tunnel.diagnosis.targetAddress') }}</span>
                  <code>{{ formatAddress(result.targetIp, result.targetPort) }}</code>
                </div>
                <div v-if="result.averageTime">
                  <span class="meta-label">{{ t('runtime.tunnel.diagnosis.duration') }}</span>
                  <strong>{{ result.averageTime.toFixed(0) }} ms</strong>
                </div>
                <div v-if="result.message">
                  <span class="meta-label">{{ t('runtime.tunnel.diagnosis.message') }}</span>
                  <strong>{{ translateLiteral(result.message) }}</strong>
                </div>
              </div>
            </article>
          </div>

          <div v-else class="empty-state compact">
            <h3>{{ t('runtime.tunnel.diagnosis.emptyTitle') }}</h3>
            <p>{{ t('runtime.tunnel.diagnosis.emptyText') }}</p>
          </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-secondary" @click="diagnosisModalOpen = false">{{ t('common.actions.close') }}</button>
          <button class="btn btn-primary" :disabled="diagnosisLoading" @click="rerunDiagnosis">
            {{ diagnosisLoading ? t('runtime.tunnel.diagnosis.rerunning') : t('runtime.tunnel.diagnosis.rerun') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
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
import ForwardSuiteNav from '@/components/admin/ForwardSuiteNav.vue'
import { humanizeForwardRuntimeBackend } from '@/utils/forwardRuntime'

const { t, translateLiteral } = useAppI18n()

const loading = ref(true)
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

const modalOpen = ref(false)
const deleteModalOpen = ref(false)
const diagnosisModalOpen = ref(false)
const isEdit = ref(false)

const submitLoading = ref(false)
const deleteLoading = ref(false)
const diagnosisLoading = ref(false)

const tunnelToDelete = ref(null)
const currentDiagnosisTunnel = ref(null)
const diagnosisResult = ref(null)

const feedback = reactive({
  type: 'info',
  message: ''
})

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

function setFeedback(type, message) {
  feedback.type = type
  feedback.message = message
}

function clearFeedback() {
  feedback.message = ''
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

  clearFeedback()
  try {
    const inventoryPromise = runtimeNodeXMode.value
      ? getForwardNodes({ page_size: 200, scope: 'nodex' })
      : getAnsibleMachines({ page_size: 200, type: 'relay' })

    const [tunnelRes, nodeRes] = await Promise.all([getAdminForwardTunnelList(), inventoryPromise])

    if (tunnelRes.code === 0) {
      tunnels.value = Array.isArray(tunnelRes.data) ? tunnelRes.data.map(normalizeTunnel) : []
    } else {
      setFeedback('error', translateMessage(tunnelRes.msg, t('runtime.tunnel.messages.loadListFailed')))
    }

    nodes.value = extractNodeList(nodeRes)
  } catch (error) {
    console.error('Failed to load tunnel page data:', error)
    setFeedback('error', t('runtime.tunnel.messages.loadDataFailed'))
  } finally {
    loading.value = false
  }
}

function resolveNodeName(nodeId) {
  const match = nodes.value.find(node => node.id === Number(nodeId))
  return match?.name || (nodeId ? `Node #${nodeId}` : '-')
}

function resolveTypeMeta(type) {
  return Number(type) === 2
    ? { text: t('runtime.tunnel.options.tunnelForward'), className: 'tag-primary' }
    : { text: t('runtime.tunnel.options.portForward'), className: 'tag-neutral' }
}

function resolveStatusMeta(status) {
  return Number(status) === 1
    ? { text: t('runtime.shared.enabled'), className: 'tag-success' }
    : { text: t('runtime.shared.disabled'), className: 'tag-danger' }
}

function resolveFlowLabel(flow) {
  return Number(flow) === 2 ? t('runtime.tunnel.options.twoWayAccounting') : t('runtime.tunnel.options.oneWayAccounting')
}

function resolveRuntimeCompatibility(tunnel) {
  const executionNodeId = Number(tunnel?.outNodeId || tunnel?.inNodeId || 0)

  if (runtimeNodeXMode.value) {
    if (!Number(tunnel?.inNodeId || 0)) {
      return { text: t('runtime.tunnel.compatibility.nodeXNeedsIngress'), className: 'tag-danger' }
    }
    if (Number(tunnel?.type) === 2 && !Number(tunnel?.outNodeId || 0)) {
      return { text: t('runtime.tunnel.compatibility.nodeXNeedsEgress'), className: 'tag-danger' }
    }
    return { text: t('runtime.tunnel.compatibility.nodeXReady'), className: 'tag-success' }
  }

  if (Number(tunnel?.type) !== 1) {
    return { text: t('runtime.tunnel.compatibility.localOnlyPortForward'), className: 'tag-danger' }
  }
  if (!executionNodeId) {
    return { text: t('runtime.tunnel.compatibility.localNeedsExecution'), className: 'tag-danger' }
  }
  return { text: t('runtime.tunnel.compatibility.localReady'), className: 'tag-success' }
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
  deleteModalOpen.value = true
}

async function confirmDelete() {
  if (!tunnelToDelete.value) {
    return
  }

  deleteLoading.value = true
  try {
    const response = await deleteForwardTunnel(tunnelToDelete.value.id)
    if (response.code === 0) {
      deleteModalOpen.value = false
      tunnelToDelete.value = null
      setFeedback('success', t('runtime.tunnel.messages.deleted'))
      await loadData(false)
      return
    }
    setFeedback('error', translateMessage(response.msg, t('runtime.tunnel.messages.deleteFailed')))
  } catch (error) {
    console.error('Failed to delete tunnel:', error)
    setFeedback('error', t('runtime.tunnel.messages.deleteFailed'))
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
.tunnel-page {
  display: grid;
  gap: 20px;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.toolbar-copy h2 {
  margin: 4px 0 0;
  font-size: 28px;
}

.eyebrow {
  margin: 0;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.14em;
  font-size: 12px;
}

.toolbar-actions {
  display: flex;
  gap: 12px;
}

.btn {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 10px 16px;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s ease;
}

.btn:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}

.btn-primary {
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  color: #fff;
  border-color: transparent;
}

.btn-secondary {
  background: var(--surface-color);
  color: var(--text-color);
}

.btn-sm {
  padding: 8px 12px;
  border-radius: 12px;
  font-size: 13px;
}

.danger {
  background: linear-gradient(135deg, #b91c1c, #dc2626);
}

.danger-text {
  color: #dc2626;
}

.feedback {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 18px;
  border-radius: 16px;
  border: 1px solid var(--border-color);
}

.feedback-success {
  background: rgba(22, 163, 74, 0.1);
  border-color: rgba(22, 163, 74, 0.25);
}

.feedback-error {
  background: rgba(220, 38, 38, 0.1);
  border-color: rgba(220, 38, 38, 0.25);
}

.feedback-close,
.modal-close {
  border: none;
  background: transparent;
  color: inherit;
  font-size: 22px;
  cursor: pointer;
}

.loading-state,
.empty-state {
  display: grid;
  place-items: center;
  gap: 12px;
  min-height: 220px;
  padding: 32px;
  border-radius: 20px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  text-align: center;
}

.loading-state.compact,
.empty-state.compact {
  min-height: 160px;
}

.spinner {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 3px solid rgba(37, 99, 235, 0.18);
  border-top-color: #2563eb;
  animation: spin 0.8s linear infinite;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 18px;
}

.tunnel-card,
.diagnosis-card {
  display: grid;
  gap: 16px;
  padding: 20px;
  border-radius: 20px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.05), transparent), var(--surface-color);
  border: 1px solid var(--border-color);
}

.card-head,
.diagnosis-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.card-title h3,
.diagnosis-head h4 {
  margin: 0;
  font-size: 20px;
}

.card-title p,
.diagnosis-head p,
.modal-subtitle,
.hint,
.meta-label {
  margin: 4px 0 0;
  color: var(--text-secondary);
}

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

.card-head-actions,
.card-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.meta-list,
.diagnosis-meta {
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

.meta-item code,
.diagnosis-meta code {
  word-break: break-all;
}

.tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 700;
  background: rgba(148, 163, 184, 0.12);
}

.tag-primary {
  background: rgba(37, 99, 235, 0.16);
  color: #2563eb;
}

.tag-success {
  background: rgba(22, 163, 74, 0.16);
  color: #16a34a;
}

.tag-danger {
  background: rgba(220, 38, 38, 0.16);
  color: #dc2626;
}

.tag-neutral {
  background: rgba(148, 163, 184, 0.14);
  color: var(--text-color);
}

.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.68);
}

.modal {
  width: min(100%, 560px);
  max-height: calc(100vh - 48px);
  overflow: auto;
  border-radius: 24px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
}

.modal-lg {
  width: min(100%, 760px);
}

.modal-xl {
  width: min(100%, 900px);
}

.modal-header,
.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border-color);
}

.modal-footer {
  border-bottom: none;
  border-top: 1px solid var(--border-color);
  justify-content: flex-end;
}

.modal-body {
  display: grid;
  gap: 18px;
  padding: 24px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.form-group {
  display: grid;
  gap: 8px;
}

.form-group input,
.form-group select {
  width: 100%;
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid var(--border-color);
  background: var(--bg-color);
  color: var(--text-color);
}

.form-group input:disabled,
.form-group select:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.form-error {
  margin: 0;
  color: #dc2626;
  font-size: 13px;
}

.modal-copy {
  margin: 0;
  line-height: 1.7;
}

.diagnosis-list {
  display: grid;
  gap: 14px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 768px) {
  .toolbar,
  .modal-header,
  .modal-footer,
  .card-head,
  .diagnosis-head {
    flex-direction: column;
    align-items: stretch;
  }

  .form-grid {
    grid-template-columns: 1fr;
  }

  .modal-overlay {
    padding: 12px;
  }
}
</style>
