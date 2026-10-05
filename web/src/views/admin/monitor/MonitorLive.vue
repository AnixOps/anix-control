<template>
  <div class="monitor-section" data-monitor-live>
    <div class="monitor-section__toolbar">
      <UiStatusDot :tone="connectionTone" :label="connectionLabel" data-monitor-connection :data-state="wsStatus" role="status" />
      <UiButton v-if="wsStatus !== 'connected' && wsStatus !== 'connecting'" size="sm" :icon="RotateCw" data-monitor-reconnect @click="connect">
        {{ t('adminMonitor.live.reconnectNow') }}
      </UiButton>
    </div>

    <section class="monitor-section__metrics" :aria-label="t('adminMonitor.live.overview.label')">
      <UiMetricCard v-for="metric in metrics" :key="metric.key" :label="metric.label" :value="metric.value" :detail="metric.detail" :data-overview="metric.key" />
    </section>

    <UiDataTable
      :columns="columns"
      :rows="sortedNodes"
      :label="t('adminMonitor.live.table.label')"
      :row-label="node => node.name || `#${node.id}`"
      storage-key="admin.monitor.live"
      :sticky-header="true"
      :row-actions="nodeActions"
      activatable
      data-monitor-nodes
      @row-activate="openTraffic"
    >
      <template #empty>
        <UiEmptyState
          v-if="!snapshotReceived && wsStatus !== 'connected'"
          :icon="Unplug"
          :title="t('adminMonitor.live.offlineTitle')"
          :description="t('adminMonitor.live.offlineDescription')"
          heading-tag="h2"
        >
          <template #actions>
            <UiButton :icon="RotateCw" @click="connect">{{ t('adminMonitor.live.reconnectNow') }}</UiButton>
          </template>
        </UiEmptyState>
        <UiEmptyState
          v-else-if="!snapshotReceived"
          :icon="Radio"
          :title="t('adminMonitor.live.waiting')"
          :description="t('adminMonitor.live.waitingDescription')"
          heading-tag="h2"
        />
        <UiEmptyState v-else :icon="Server" :title="t('adminMonitor.live.empty')" :description="t('adminMonitor.live.emptyDescription')" heading-tag="h2" />
      </template>
      <template #cell-name="{ row }">
        <span class="monitor-section__strong">{{ row.name || `#${row.id}` }}</span>
      </template>
      <template #cell-host="{ row }">
        <code class="monitor-section__code">{{ row.host || '—' }}</code>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :status="row.status === 'online' ? 'online' : row.status === 'offline' ? 'offline' : 'pending'" :label="statusLabel(row.status)" />
      </template>
      <template v-for="key in USAGE_KEYS" #[`cell-${key}`]="{ row }" :key="key">
        <UiUsageBar :value="Number(row[key] || 0)" :max="100" :text="percentText(row[key])" />
      </template>
    </UiDataTable>

    <MonitorNodeTraffic v-if="trafficNode" v-model:open="trafficOpen" :node="trafficNode" />
  </div>
</template>

<script setup>
// 实时节点 of 流量与监控: the fleet over the admin monitor WebSocket
// (/api/v2/admin/ws/monitor?token=…): a full snapshot (`nodes`), then
// deltas (`node_updates`, `removed`), plus the `overview` counters. It
// reconnects with backoff (3 s, doubling, at most 30 s) and closes on
// leaving the section. Same protocol and behaviour as the old Monitor page;
// only the presentation changed (metric cards, UiDataTable, status words).
// A row opens that node's traffic history in a sheet (MonitorNodeTraffic,
// GET /api/v4/kernel/nodes/:id/traffic), also from its row menu.
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ChartLine, Radio, RotateCw, Server, SquareArrowOutUpRight, Unplug } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiMetricCard from '@/ui/UiMetricCard.vue'
import UiStatusDot from '@/ui/UiStatusDot.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { useFormat } from '@/ui/composables/useFormat'

const USAGE_KEYS = ['cpu_usage', 'memory_usage', 'disk_usage']
const STATUS_ORDER = { online: 0, pending: 1, offline: 2 }

// The sheet draws a chart: its code (and ECharts) load with the first row asked.
const MonitorNodeTraffic = defineAsyncComponent(() => import('./MonitorNodeTraffic.vue'))

const { t } = useAppI18n()
const format = useFormat()
const router = useRouter()

const trafficNode = ref(null)
const trafficOpen = ref(false)
const overview = ref({})
const nodes = ref([])
const snapshotReceived = ref(false)
const wsStatus = ref('disconnected') // 'connecting' | 'connected' | 'disconnected' | 'reconnecting'
const reconnectSeconds = ref(0)
let ws = null
let reconnectTimer = null
let reconnectCountdown = null
let disposed = false

const connectionTone = computed(() => ({ connected: 'success', connecting: 'warning', reconnecting: 'warning' }[wsStatus.value] || 'danger'))
const connectionLabel = computed(() => {
  switch (wsStatus.value) {
    case 'connecting': return t('adminMonitor.live.connection.connecting')
    case 'connected': return t('adminMonitor.live.connection.connected')
    case 'reconnecting': return t('adminMonitor.live.connection.reconnecting', { seconds: reconnectSeconds.value })
    default: return t('adminMonitor.live.connection.disconnected')
  }
})

const metrics = computed(() => {
  const o = overview.value
  return [
    { key: 'total', label: t('adminMonitor.live.overview.totalNodes'), value: format.number(o.total_nodes || 0) },
    { key: 'online', label: t('adminMonitor.live.overview.onlineNodes'), value: format.number(o.online_nodes || 0), detail: t('adminMonitor.live.overview.upload', { value: format.bytes(o.total_upload) }) },
    { key: 'offline', label: t('adminMonitor.live.overview.offlineNodes'), value: format.number(o.offline_nodes || 0), detail: t('adminMonitor.live.overview.download', { value: format.bytes(o.total_download) }) },
    { key: 'pending', label: t('adminMonitor.live.overview.pendingNodes'), value: format.number(o.pending_nodes || 0) }
  ]
})

const columns = computed(() => [
  { key: 'name', label: t('adminMonitor.live.table.name'), primary: true, sortable: true, hideable: false, value: node => node.name || `#${node.id}` },
  { key: 'host', label: t('adminMonitor.live.table.host'), secondary: true, breakpoint: 'md' },
  { key: 'status', label: t('adminMonitor.live.table.status'), sortable: true, nowrap: true, sortValue: node => STATUS_ORDER[node.status] ?? 3 },
  { key: 'cpu_usage', label: t('adminMonitor.live.table.cpu'), sortable: true, numeric: true, firstDirection: 'desc', sortValue: node => Number(node.cpu_usage || 0) },
  { key: 'memory_usage', label: t('adminMonitor.live.table.memory'), sortable: true, numeric: true, firstDirection: 'desc', sortValue: node => Number(node.memory_usage || 0) },
  { key: 'disk_usage', label: t('adminMonitor.live.table.disk'), sortable: true, numeric: true, firstDirection: 'desc', breakpoint: 'lg', sortValue: node => Number(node.disk_usage || 0) },
  { key: 'online_users', label: t('adminMonitor.live.table.users'), sortable: true, numeric: true, align: 'end', firstDirection: 'desc', format: value => format.number(value || 0) },
  { key: 'uptime', label: t('adminMonitor.live.table.uptime'), sortable: true, numeric: true, align: 'end', nowrap: true, breakpoint: 'md', format: value => (value ? format.duration(value) : '—') }
])

// Online first, then pending, then offline (the old page's order).
const sortedNodes = computed(() => [...nodes.value].sort((a, b) => (STATUS_ORDER[a.status] ?? 3) - (STATUS_ORDER[b.status] ?? 3)))

// A node that is gone from the table keeps its sheet until it closes.
function openTraffic(node) {
  if (!node?.id) return
  trafficNode.value = { id: node.id, name: node.name || '', host: node.host || '' }
  trafficOpen.value = true
}

const nodeActions = node => [
  { key: 'traffic', label: t('adminMonitor.live.traffic.view'), icon: ChartLine, onSelect: () => openTraffic(node) },
  { key: 'open', label: t('adminMonitor.live.traffic.openNode'), icon: SquareArrowOutUpRight, onSelect: () => router.push(`/admin/nodes/${node.id}`) }
]

function statusLabel(status) {
  return STATUS_ORDER[status] === undefined ? (status || '—') : t(`adminMonitor.live.status.${status}`)
}

function percentText(value) {
  const n = Number(value)
  return Number.isFinite(n) ? format.percent(n / 100, { precision: 1 }) : '—'
}

function getToken() {
  try {
    return (localStorage.getItem('token') || '').trim()
  } catch {
    return ''
  }
}

function buildWsUrl() {
  const token = getToken()
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${proto}//${host}/api/v2/admin/ws/monitor?token=${encodeURIComponent(token)}`
}

function clearTimers() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (reconnectCountdown) {
    clearInterval(reconnectCountdown)
    reconnectCountdown = null
  }
}

function connect() {
  if (ws) {
    ws.onclose = null
    ws.close()
    ws = null
  }
  clearTimers()

  const token = getToken()
  if (!token) {
    wsStatus.value = 'disconnected'
    return
  }

  wsStatus.value = 'connecting'

  try {
    ws = new WebSocket(buildWsUrl())
  } catch (e) {
    wsStatus.value = 'disconnected'
    console.error(t('adminMonitor.live.unsupported'), e)
    return
  }

  ws.onopen = () => {
    wsStatus.value = 'connected'
    reconnectSeconds.value = 0
  }

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data)
      if (msg.type === 'error') {
        console.error('[Monitor]', msg.error)
        return
      }
      if (msg.data) {
        if (msg.data.overview) {
          overview.value = { ...overview.value, ...msg.data.overview }
        }
        if (Array.isArray(msg.data.nodes)) {
          // Full snapshot
          nodes.value = msg.data.nodes
          snapshotReceived.value = true
        }
        if (Array.isArray(msg.data.node_updates)) {
          // Delta update
          applyDelta(msg.data.node_updates)
        }
      }
    } catch (e) {
      console.error('[Monitor] parse error:', e)
    }
  }

  ws.onclose = () => {
    wsStatus.value = 'disconnected'
    scheduleReconnect()
  }

  ws.onerror = () => {
    wsStatus.value = 'disconnected'
  }
}

function applyDelta(updates) {
  const nodeMap = new Map(nodes.value.map(n => [n.id, n]))
  for (const update of updates) {
    if (update.changed_fields && update.changed_fields.includes('removed')) {
      nodeMap.delete(update.id)
      continue
    }
    const existing = nodeMap.get(update.id)
    if (existing) {
      nodeMap.set(update.id, { ...existing, ...update })
    } else {
      nodeMap.set(update.id, { ...update })
    }
  }
  nodes.value = Array.from(nodeMap.values())
}

function scheduleReconnect() {
  if (disposed) return
  const delay = Math.min(30, reconnectSeconds.value > 0 ? reconnectSeconds.value * 2 : 3)
  reconnectSeconds.value = delay
  wsStatus.value = 'reconnecting'

  reconnectCountdown = setInterval(() => {
    reconnectSeconds.value = Math.max(0, reconnectSeconds.value - 1)
  }, 1000)

  reconnectTimer = setTimeout(() => {
    if (reconnectCountdown) {
      clearInterval(reconnectCountdown)
      reconnectCountdown = null
    }
    connect()
  }, delay * 1000)
}

onMounted(() => connect())
onBeforeUnmount(() => {
  disposed = true
  if (ws) {
    ws.onclose = null
    ws.close()
    ws = null
  }
  clearTimers()
})
</script>
