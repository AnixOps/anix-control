<template>
  <div class="observability-page">
    <section class="hero-card">
      <div class="hero-copy">
        <p class="eyebrow">{{ t('observability.subtitle') }}</p>
        <h2>{{ t('observability.title') }}</h2>
      </div>
      <div class="hero-actions">
        <button class="btn btn-secondary" :disabled="loading" @click="refreshActive">
          {{ loading ? '…' : t('observability.refresh') }}
        </button>
      </div>
    </section>

    <nav class="tab-bar" role="tablist">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        :class="['tab', activeTab === tab.key ? 'tab-active' : '']"
        role="tab"
        :aria-selected="activeTab === tab.key"
        @click="selectTab(tab.key)"
      >
        {{ tab.label }}
      </button>
    </nav>

    <!-- Latency Trend -->
    <section v-show="activeTab === 'trend'" class="panel-card">
      <div class="toolbar">
        <label class="field">
          <span>{{ t('observability.trend.targetLabel') }}</span>
          <select v-model="selectedTargetKey" @change="loadTrend">
            <option value="" disabled>{{ t('observability.trend.selectTarget') }}</option>
            <option v-for="item in targets" :key="item.targetKey" :value="item.targetKey">
              {{ targetOptionLabel(item) }}
            </option>
          </select>
        </label>
      </div>
      <div v-if="!targets.length" class="state-card">{{ t('observability.trend.noTargets') }}</div>
      <div v-else-if="selectedTargetKey && !trendPoints.length" class="state-card">{{ t('observability.trend.noData') }}</div>
      <div v-show="selectedTargetKey && trendPoints.length" ref="trendChartEl" class="chart-canvas"></div>
    </section>

    <!-- Topology -->
    <section v-show="activeTab === 'topology'" class="panel-card">
      <div v-if="!topology.nodes.length" class="state-card">{{ t('observability.topology.empty') }}</div>
      <template v-else>
        <div class="legend">
          <span class="legend-item"><i class="dot" style="background:#3491fa"></i>{{ t('observability.topology.relay') }}</span>
          <span class="legend-item"><i class="dot" style="background:#00b42a"></i>{{ t('observability.topology.exit') }}</span>
          <span class="legend-item"><i class="dot" style="background:#ff7d00"></i>{{ t('observability.topology.proxy') }}</span>
          <span class="legend-item"><i class="dot" style="background:#86909c"></i>{{ t('observability.trend.offline') }}</span>
        </div>
        <div ref="topologyEl" class="chart-canvas topology-canvas"></div>
      </template>
    </section>

    <!-- Multi-Ingress -->
    <section v-show="activeTab === 'multiIngress'" class="panel-card">
      <div class="toolbar">
        <label class="field">
          <span>{{ t('observability.multiIngress.forwardLabel') }}</span>
          <select v-model="selectedForwardId" @change="loadMultiIngress">
            <option value="" disabled>{{ t('observability.multiIngress.selectForward') }}</option>
            <option v-for="item in forwardTargets" :key="item.targetId" :value="item.targetId">
              {{ item.label || item.host }}
            </option>
          </select>
        </label>
      </div>
      <div v-if="!selectedForwardId" class="state-card">{{ t('observability.multiIngress.empty') }}</div>
      <div v-else-if="!multiIngressRows.length" class="state-card">{{ t('observability.multiIngress.noRows') }}</div>
      <table v-else class="data-table">
        <thead>
          <tr>
            <th>{{ t('observability.multiIngress.tunnel') }}</th>
            <th>{{ t('observability.multiIngress.ingress') }}</th>
            <th>{{ t('observability.multiIngress.ingressIp') }}</th>
            <th>{{ t('observability.multiIngress.avgRtt') }}</th>
            <th>{{ t('observability.multiIngress.loss') }}</th>
            <th>{{ t('observability.multiIngress.status') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in multiIngressRows" :key="row.tunnelId">
            <td>{{ row.tunnelName }}</td>
            <td>{{ row.ingressLabel || '-' }}</td>
            <td>{{ row.ingressIp || '-' }}</td>
            <td>{{ formatRtt(row.avgRtt) }}</td>
            <td>{{ row.loss.toFixed(1) }}</td>
            <td>
              <span :class="['badge', row.online ? 'badge-online' : 'badge-offline']">
                {{ row.online ? t('observability.trend.online') : t('observability.trend.offline') }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <!-- Job Timeline -->
    <section v-show="activeTab === 'jobs'" class="panel-card">
      <div v-if="!jobs.length" class="state-card">{{ t('observability.jobs.empty') }}</div>
      <table v-else class="data-table">
        <thead>
          <tr>
            <th>{{ t('observability.jobs.action') }}</th>
            <th>{{ t('observability.jobs.backend') }}</th>
            <th>{{ t('observability.jobs.status') }}</th>
            <th>{{ t('observability.jobs.createdAt') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="job in jobs" :key="job.id">
            <td>{{ job.action }}</td>
            <td>{{ job.backend }}</td>
            <td>
              <span :class="['badge', jobStatusClass(job.status)]">{{ jobStatusLabel(job.status) }}</span>
            </td>
            <td>{{ formatDateTime(job.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup>
import { nextTick, onMounted, onUnmounted, ref, computed } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  getForwardObservabilityTargets,
  getForwardObservabilityTrend,
  getForwardObservabilityTopology,
  getForwardObservabilityMultiIngress,
  listForwardRuntimeJobs
} from '@/api/admin'

const { t, formatDateTime } = useAppI18n()

const tabs = computed(() => ([
  { key: 'trend', label: t('observability.tabs.trend') },
  { key: 'topology', label: t('observability.tabs.topology') },
  { key: 'multiIngress', label: t('observability.tabs.multiIngress') },
  { key: 'jobs', label: t('observability.tabs.jobs') }
]))

const activeTab = ref('trend')
const loading = ref(false)

const targets = ref([])
const selectedTargetKey = ref('')
const trendPoints = ref([])
const trendChartEl = ref(null)
let trendChart = null
let echartsLib = null

const topology = ref({ nodes: [], edges: [] })
const topologyEl = ref(null)
let topologyGraph = null

const selectedForwardId = ref('')
const multiIngressRows = ref([])
const jobs = ref([])

const forwardTargets = computed(() => targets.value.filter(item => item.targetType === 'forward'))

// Response interceptor returns the {code,msg,data} envelope; unwrap defensively.
function extractPayload(res) {
  return res?.data?.data ?? res?.data ?? res
}

function targetOptionLabel(item) {
  const label = item.label || item.host
  return `${label} (${item.host}:${item.port})`
}

function formatRtt(value) {
  if (!value && value !== 0) return '-'
  return Number(value).toFixed(1)
}

// Topology node fill: offline is grey; online color encodes the node kind
// (relay/exit = forward infra, node = V2bX proxy node).
function nodeFill(n) {
  if (!n.online) return '#86909c'
  if (n.kind === 'relay') return '#3491fa'
  if (n.kind === 'exit') return '#00b42a'
  if (n.kind === 'node') return '#ff7d00'
  return '#3491fa'
}

// Human-readable node kind for topology labels.
function kindLabel(kind) {
  if (kind === 'relay') return t('observability.topology.relay')
  if (kind === 'exit') return t('observability.topology.exit')
  if (kind === 'node') return t('observability.topology.proxy')
  return kind
}

function jobStatusLabel(status) {
  return {
    0: t('observability.jobs.pending'),
    1: t('observability.jobs.running'),
    2: t('observability.jobs.success'),
    3: t('observability.jobs.failed')
  }[status] ?? String(status)
}

function jobStatusClass(status) {
  return {
    0: 'badge-pending',
    1: 'badge-running',
    2: 'badge-online',
    3: 'badge-offline'
  }[status] ?? 'badge-pending'
}

async function loadTargets() {
  try {
    const payload = extractPayload(await getForwardObservabilityTargets())
    targets.value = Array.isArray(payload?.list) ? payload.list : []
    if (!selectedTargetKey.value && targets.value.length) {
      selectedTargetKey.value = targets.value[0].targetKey
    }
  } catch (error) {
    console.error('load observability targets failed:', error)
    targets.value = []
  }
}

async function loadTrend() {
  if (!selectedTargetKey.value) return
  try {
    const payload = extractPayload(await getForwardObservabilityTrend({ targetKey: selectedTargetKey.value }))
    trendPoints.value = Array.isArray(payload?.points) ? payload.points : []
    await nextTick()
    renderTrendChart()
  } catch (error) {
    console.error('load latency trend failed:', error)
    trendPoints.value = []
  }
}

async function ensureECharts() {
  if (!echartsLib) {
    echartsLib = await import('echarts')
  }
  return echartsLib
}

async function renderTrendChart() {
  if (!trendChartEl.value || !trendPoints.value.length) return
  const echarts = await ensureECharts()
  if (!trendChart) {
    trendChart = echarts.init(trendChartEl.value)
  }
  const times = trendPoints.value.map(p => formatDateTime(p.bucketAt, { month: undefined, year: undefined }))
  trendChart.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: [t('observability.trend.avg'), t('observability.trend.p95'), t('observability.trend.max')] },
    grid: { left: 48, right: 16, top: 40, bottom: 32 },
    xAxis: { type: 'category', data: times, boundaryGap: false },
    yAxis: { type: 'value', name: t('observability.trend.latencyAxis') },
    series: [
      { name: t('observability.trend.avg'), type: 'line', smooth: true, data: trendPoints.value.map(p => p.avg) },
      { name: t('observability.trend.p95'), type: 'line', smooth: true, data: trendPoints.value.map(p => p.p95) },
      { name: t('observability.trend.max'), type: 'line', smooth: true, data: trendPoints.value.map(p => p.max) }
    ]
  })
  trendChart.resize()
}

async function loadTopology() {
  try {
    const payload = extractPayload(await getForwardObservabilityTopology())
    topology.value = {
      nodes: Array.isArray(payload?.nodes) ? payload.nodes : [],
      edges: Array.isArray(payload?.edges) ? payload.edges : []
    }
    await nextTick()
    await renderTopology()
  } catch (error) {
    console.error('load topology failed:', error)
    topology.value = { nodes: [], edges: [] }
  }
}

async function renderTopology() {
  if (!topologyEl.value || !topology.value.nodes.length) return
  const { Graph } = await import('@antv/g6')
  if (topologyGraph) {
    topologyGraph.destroy()
    topologyGraph = null
  }
  const data = {
    nodes: topology.value.nodes.map(n => ({
      id: n.id,
      data: { ...n },
      style: {
        labelText: `${n.label} [${kindLabel(n.kind)}]\n${n.latencyMs}ms`,
        fill: nodeFill(n)
      }
    })),
    edges: topology.value.edges.map(e => ({
      id: e.id,
      source: e.source,
      target: e.target,
      style: { labelText: e.label }
    }))
  }
  topologyGraph = new Graph({
    container: topologyEl.value,
    data,
    autoFit: 'view',
    layout: { type: 'force', preventOverlap: true, linkDistance: 160 },
    node: { style: { size: 44, labelFill: '#1d2129', labelPlacement: 'bottom' } },
    edge: { style: { endArrow: true, stroke: '#c9cdd4', labelFill: '#86909c' } },
    behaviors: ['drag-canvas', 'zoom-canvas', 'drag-element']
  })
  topologyGraph.render()
}

async function loadMultiIngress() {
  if (!selectedForwardId.value) return
  try {
    const payload = extractPayload(await getForwardObservabilityMultiIngress(selectedForwardId.value))
    multiIngressRows.value = Array.isArray(payload?.list) ? payload.list : []
  } catch (error) {
    console.error('load multi-ingress failed:', error)
    multiIngressRows.value = []
  }
}

async function loadJobs() {
  try {
    const res = await listForwardRuntimeJobs({ limit: 50 })
    const payload = extractPayload(res)
    jobs.value = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
  } catch (error) {
    console.error('load runtime jobs failed:', error)
    jobs.value = []
  }
}

async function refreshActive() {
  loading.value = true
  try {
    if (activeTab.value === 'trend') {
      await loadTargets()
      await loadTrend()
    } else if (activeTab.value === 'topology') {
      await loadTopology()
    } else if (activeTab.value === 'multiIngress') {
      if (!targets.value.length) await loadTargets()
      await loadMultiIngress()
    } else if (activeTab.value === 'jobs') {
      await loadJobs()
    }
  } finally {
    loading.value = false
  }
}

async function selectTab(key) {
  activeTab.value = key
  await refreshActive()
}

function handleVisibility() {
  if (!document.hidden) {
    refreshActive()
  }
}

function handleResize() {
  if (trendChart) trendChart.resize()
}

onMounted(async () => {
  await loadTargets()
  await loadTrend()
  document.addEventListener('visibilitychange', handleVisibility)
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', handleVisibility)
  window.removeEventListener('resize', handleResize)
  if (trendChart) {
    trendChart.dispose()
    trendChart = null
  }
  if (topologyGraph) {
    topologyGraph.destroy()
    topologyGraph = null
  }
})
</script>

<style scoped>
.observability-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.hero-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-radius: 12px;
  background: linear-gradient(135deg, #1d2129, #2b3140);
  color: #fff;
}

.eyebrow {
  font-size: 12px;
  opacity: 0.7;
  margin: 0 0 4px;
}

.hero-card h2 {
  margin: 0;
  font-size: 20px;
}

.tab-bar {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--color-border, #e5e6eb);
}

.tab {
  padding: 10px 18px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  color: var(--color-text-2, #4e5969);
  border-bottom: 2px solid transparent;
}

.tab-active {
  color: #3491fa;
  border-bottom-color: #3491fa;
  font-weight: 600;
}

.panel-card {
  background: var(--color-bg-2, #fff);
  border: 1px solid var(--color-border, #e5e6eb);
  border-radius: 12px;
  padding: 16px;
}

.toolbar {
  margin-bottom: 12px;
}

.field {
  display: inline-flex;
  flex-direction: column;
  gap: 4px;
  font-size: 13px;
  color: var(--color-text-2, #4e5969);
}

.field select {
  min-width: 280px;
  padding: 6px 10px;
  border-radius: 6px;
  border: 1px solid var(--color-border, #e5e6eb);
  background: var(--color-bg-1, #fff);
  color: var(--color-text-1, #1d2129);
}

.chart-canvas {
  width: 100%;
  height: 360px;
}

.topology-canvas {
  height: 480px;
}

.legend {
  display: flex;
  gap: 16px;
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--color-text-2, #4e5969);
}

.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.legend .dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}

.state-card {
  padding: 40px 16px;
  text-align: center;
  color: var(--color-text-3, #86909c);
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 14px;
}

.data-table th,
.data-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid var(--color-border, #e5e6eb);
}

.data-table th {
  color: var(--color-text-3, #86909c);
  font-weight: 600;
}

.badge {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 10px;
  font-size: 12px;
}

.badge-online {
  background: #e8ffea;
  color: #00b42a;
}

.badge-offline {
  background: #ffece8;
  color: #f53f3f;
}

.badge-pending {
  background: #f2f3f5;
  color: #86909c;
}

.badge-running {
  background: #e8f3ff;
  color: #3491fa;
}

.btn {
  padding: 8px 16px;
  border-radius: 6px;
  border: none;
  cursor: pointer;
  font-size: 14px;
}

.btn-secondary {
  background: rgba(255, 255, 255, 0.16);
  color: #fff;
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
