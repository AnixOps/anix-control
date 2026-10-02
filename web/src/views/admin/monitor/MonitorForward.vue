<template>
  <div class="monitor-section" data-monitor-forward>
    <div class="monitor-section__toolbar">
      <UiButton class="monitor-section__end" :icon="RotateCw" :loading="busy" data-monitor-refresh @click="refresh">{{ t('adminMonitor.refresh') }}</UiButton>
    </div>

    <UiCard :title="t('adminMonitor.forward.topology.title')" :description="t('adminMonitor.forward.topology.description')" as="section">
      <UiErrorState v-if="topologyError" compact heading-tag="h3" :title="t('adminMonitor.forward.topology.loadFailed')" :error="topologyError" @retry="loadTopology" />
      <TopologyGraph
        v-else
        :nodes="graphNodes"
        :edges="graphEdges"
        :label="t('adminMonitor.forward.topology.label')"
        :summary="t('adminMonitor.forward.topology.summary', { nodes: graphNodes.length, edges: graphEdges.length })"
        :loading="topologyLoading"
        :height="420"
        :legend="legend"
        :legend-label="t('adminMonitor.forward.topology.legend')"
        :empty-title="t('adminMonitor.forward.topology.empty')"
        :empty-description="t('adminMonitor.forward.topology.emptyDescription')"
        :error-title="t('adminMonitor.forward.topology.loadFailed')"
        data-monitor-topology
      />
    </UiCard>

    <UiSection :title="t('adminMonitor.forward.ingress.title')" :description="t('adminMonitor.forward.ingress.description')">
      <template v-if="forwardTargets.length" #actions>
        <UiCombobox
          class="monitor-section__picker"
          :model-value="selectedForwardId"
          :options="forwardOptions"
          :aria-label="t('adminMonitor.forward.ingress.forward')"
          :placeholder="t('adminMonitor.forward.ingress.selectForward')"
          size="md"
          data-monitor-forward-picker
          @update:model-value="selectForward"
        />
      </template>
      <UiDataTable
        :columns="ingressColumns"
        :rows="ingressRows"
        row-key="tunnelId"
        :label="t('adminMonitor.forward.ingress.label')"
        :loading="targetsLoading || ingressLoading"
        :error="targetsError || ingressError"
        :error-title="t('adminMonitor.forward.ingress.loadFailed')"
        :empty-title="forwardTargets.length ? t('adminMonitor.forward.ingress.noRows') : t('adminMonitor.forward.ingress.noForwards')"
        :empty-description="forwardTargets.length ? '' : t('adminMonitor.forward.ingress.noForwardsDescription')"
        storage-key="admin.monitor.ingress"
        data-monitor-ingress
        @retry="loadTargetsAndIngress"
      >
        <template #cell-online="{ row }">
          <UiBadge :status="row.online ? 'online' : 'offline'" :label="row.online ? t('adminMonitor.forward.online') : t('adminMonitor.forward.offline')" />
        </template>
      </UiDataTable>
    </UiSection>

    <UiSection :title="t('adminMonitor.forward.jobs.title')" :description="t('adminMonitor.forward.jobs.description')">
      <UiDataTable
        :columns="jobColumns"
        :rows="jobs"
        :label="t('adminMonitor.forward.jobs.label')"
        :loading="jobsLoading"
        :error="jobsError"
        :error-title="t('adminMonitor.forward.jobs.loadFailed')"
        :empty-title="t('adminMonitor.forward.jobs.empty')"
        :page-size="10"
        storage-key="admin.monitor.jobs"
        data-monitor-jobs
        @retry="loadJobs"
      >
        <template #cell-status="{ row }">
          <UiBadge :tone="JOB_TONES[jobState(row.status)] || 'neutral'" :label="jobLabel(row.status)" />
        </template>
      </UiDataTable>
    </UiSection>
  </div>
</template>

<script setup>
// 转发 of 流量与监控 (the old 转发可观测性 page's topology, multi-ingress and
// job tabs): the forward topology from GET /admin/forward/observability/
// topology as a G6 graph that follows the theme, the ingress comparison of
// one forward (…/multi-ingress?targetId=, forwards from …/targets), and the
// latest 50 runtime jobs (GET /admin/forward/runtime/jobs?limit=50).
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RotateCw } from '@lucide/vue'
import {
  getForwardObservabilityMultiIngress,
  getForwardObservabilityTargets,
  getForwardObservabilityTopology,
  listForwardRuntimeJobs
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import TopologyGraph from '@/components/admin/TopologyGraph.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiCombobox from '@/ui/UiCombobox.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSection from '@/ui/UiSection.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { JOB_STATES, JOB_TONES, extractPayload, listOf } from './observabilityData'

const { t } = useAppI18n()
const format = useFormat()

const targets = ref([])
const targetsLoading = ref(false)
const targetsError = ref(null)
const selectedForwardId = ref('')
const ingressRows = ref([])
const ingressLoading = ref(false)
const ingressError = ref(null)
const topology = ref({ nodes: [], edges: [] })
const topologyLoading = ref(false)
const topologyError = ref(null)
const jobs = ref([])
const jobsLoading = ref(false)
const jobsError = ref(null)
let active = true
let ingressSeq = 0

const busy = computed(() => targetsLoading.value || ingressLoading.value || topologyLoading.value || jobsLoading.value)
const forwardTargets = computed(() => targets.value.filter(item => item.targetType === 'forward'))
const forwardOptions = computed(() => forwardTargets.value.map(item => ({ value: String(item.targetId), label: item.label || item.host })))

const ms = value => (value === null || value === undefined || value === '' ? '—' : t('adminMonitor.forward.topology.latency', { value: format.number(Number(value), { maximumFractionDigits: 1 }) }))

const KIND_KEYS = { relay: 'relay', exit: 'exit', node: 'proxy' }
const kindLabel = kind => (KIND_KEYS[kind] ? t(`adminMonitor.forward.topology.${KIND_KEYS[kind]}`) : kind)
const toneOf = node => {
  if (!node.online) return 'offline'
  if (node.kind === 'exit') return 'exit'
  if (node.kind === 'node') return 'node'
  return 'relay'
}

const graphNodes = computed(() => topology.value.nodes.map(node => ({
  id: node.id,
  label: node.label,
  detail: `${kindLabel(node.kind)} · ${ms(node.latencyMs)}`,
  tone: toneOf(node)
})))
const graphEdges = computed(() => topology.value.edges.map(edge => ({ id: edge.id, source: edge.source, target: edge.target, label: edge.label })))
const legend = computed(() => [
  { key: 'relay', label: t('adminMonitor.forward.topology.relay') },
  { key: 'exit', label: t('adminMonitor.forward.topology.exit') },
  { key: 'node', label: t('adminMonitor.forward.topology.proxy') },
  { key: 'offline', label: t('adminMonitor.forward.topology.offline') }
])

const ingressColumns = computed(() => [
  { key: 'tunnelName', label: t('adminMonitor.forward.ingress.tunnel'), primary: true, hideable: false, sortable: true },
  { key: 'ingressLabel', label: t('adminMonitor.forward.ingress.ingress'), secondary: true, value: row => row.ingressLabel || '—' },
  { key: 'ingressIp', label: t('adminMonitor.forward.ingress.ingressIp'), breakpoint: 'md', value: row => row.ingressIp || '—' },
  { key: 'avgRtt', label: t('adminMonitor.forward.ingress.avgRtt'), numeric: true, align: 'end', sortable: true, format: ms },
  { key: 'loss', label: t('adminMonitor.forward.ingress.loss'), numeric: true, align: 'end', sortable: true, format: value => format.percent(Number(value || 0) / 100, { precision: 1 }) },
  { key: 'online', label: t('adminMonitor.forward.ingress.status'), sortable: true, sortValue: row => (row.online ? 0 : 1) }
])

const jobColumns = computed(() => [
  { key: 'action', label: t('adminMonitor.forward.jobs.action'), primary: true, hideable: false },
  { key: 'backend', label: t('adminMonitor.forward.jobs.backend'), secondary: true },
  { key: 'status', label: t('adminMonitor.forward.jobs.status'), nowrap: true },
  { key: 'created_at', label: t('adminMonitor.forward.jobs.createdAt'), nowrap: true, numeric: true, format: value => format.dateTime(value) }
])

function jobState(status) {
  return JOB_STATES[status]
}

function jobLabel(status) {
  const state = JOB_STATES[status]
  return state ? t(`adminMonitor.forward.jobs.states.${state}`) : String(status)
}

async function loadTargets() {
  targetsLoading.value = true
  try {
    const payload = extractPayload(await getForwardObservabilityTargets())
    if (!active) return
    targets.value = listOf(payload)
    targetsError.value = null
    if (!forwardTargets.value.some(item => String(item.targetId) === selectedForwardId.value)) {
      selectedForwardId.value = forwardTargets.value[0] ? String(forwardTargets.value[0].targetId) : ''
    }
  } catch (error) {
    if (!active) return
    console.error('load observability targets failed:', error)
    targets.value = []
    targetsError.value = error
  } finally {
    if (active) targetsLoading.value = false
  }
}

async function loadIngress() {
  const seq = ++ingressSeq
  ingressError.value = null
  if (!selectedForwardId.value) {
    ingressRows.value = []
    return
  }
  ingressLoading.value = true
  try {
    const payload = extractPayload(await getForwardObservabilityMultiIngress(selectedForwardId.value))
    if (!active || seq !== ingressSeq) return
    ingressRows.value = listOf(payload)
  } catch (error) {
    if (!active || seq !== ingressSeq) return
    console.error('load multi-ingress failed:', error)
    ingressRows.value = []
    ingressError.value = error
  } finally {
    if (active && seq === ingressSeq) ingressLoading.value = false
  }
}

function selectForward(id) {
  if (!id || String(id) === selectedForwardId.value) return
  selectedForwardId.value = String(id)
  void loadIngress()
}

async function loadTopology() {
  topologyLoading.value = true
  try {
    const payload = extractPayload(await getForwardObservabilityTopology())
    if (!active) return
    topology.value = {
      nodes: Array.isArray(payload?.nodes) ? payload.nodes : [],
      edges: Array.isArray(payload?.edges) ? payload.edges : []
    }
    topologyError.value = null
  } catch (error) {
    if (!active) return
    console.error('load topology failed:', error)
    topology.value = { nodes: [], edges: [] }
    topologyError.value = error
  } finally {
    if (active) topologyLoading.value = false
  }
}

async function loadJobs() {
  jobsLoading.value = true
  try {
    const payload = extractPayload(await listForwardRuntimeJobs({ limit: 50 }))
    if (!active) return
    jobs.value = listOf(payload)
    jobsError.value = null
  } catch (error) {
    if (!active) return
    console.error('load runtime jobs failed:', error)
    jobs.value = []
    jobsError.value = error
  } finally {
    if (active) jobsLoading.value = false
  }
}

async function loadTargetsAndIngress() {
  await loadTargets()
  await loadIngress()
}

function refresh() {
  return Promise.all([loadTargetsAndIngress(), loadTopology(), loadJobs()])
}

onMounted(() => { void refresh() })
onUnmounted(() => { active = false })
</script>
