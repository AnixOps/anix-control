<template>
  <div class="monitor-page">
    <div class="page-header">
      <div>
        <h1>{{ t('adminMonitor.title') }}</h1>
        <p class="text-secondary">{{ t('adminMonitor.subtitle') }}</p>
      </div>
      <div class="connection-badge" :class="connectionClass">
        <span class="dot"></span>
        {{ connectionLabel }}
      </div>
    </div>

    <!-- Overview cards -->
    <div class="stats-grid">
      <div class="stat-card primary">
        <div class="stat-value">{{ overview.total_nodes || 0 }}</div>
        <div class="stat-label">{{ t('adminMonitor.overview.totalNodes') }}</div>
      </div>
      <div class="stat-card success">
        <div class="stat-value">{{ overview.online_nodes || 0 }}</div>
        <div class="stat-label">{{ t('adminMonitor.overview.onlineNodes') }}</div>
      </div>
      <div class="stat-card danger">
        <div class="stat-value">{{ overview.offline_nodes || 0 }}</div>
        <div class="stat-label">{{ t('adminMonitor.overview.offlineNodes') }}</div>
      </div>
      <div class="stat-card info">
        <div class="stat-value">{{ overview.pending_nodes || 0 }}</div>
        <div class="stat-label">{{ t('adminMonitor.overview.pendingNodes') }}</div>
      </div>
    </div>

    <!-- Traffic overview -->
    <div class="traffic-bar">
      <div class="traffic-item">
        <span class="label">{{ t('adminMonitor.overview.totalUpload') }}</span>
        <span class="value">{{ formatBytes(overview.total_upload) }}</span>
      </div>
      <div class="traffic-item">
        <span class="label">{{ t('adminMonitor.overview.totalDownload') }}</span>
        <span class="value">{{ formatBytes(overview.total_download) }}</span>
      </div>
    </div>

    <!-- Node table -->
    <div class="node-section">
      <h3>{{ t('adminMonitor.nodeTable.title') }}</h3>
      <div class="node-table" v-if="nodes.length > 0">
        <div class="node-table-header">
          <span class="col-name">{{ t('adminMonitor.nodeTable.name') }}</span>
          <span class="col-host">{{ t('adminMonitor.nodeTable.host') }}</span>
          <span class="col-status">{{ t('adminMonitor.nodeTable.status') }}</span>
          <span class="col-metric">{{ t('adminMonitor.nodeTable.cpu') }}</span>
          <span class="col-metric">{{ t('adminMonitor.nodeTable.memory') }}</span>
          <span class="col-metric">{{ t('adminMonitor.nodeTable.disk') }}</span>
          <span class="col-metric">{{ t('adminMonitor.nodeTable.users') }}</span>
          <span class="col-metric">{{ t('adminMonitor.nodeTable.uptime') }}</span>
        </div>
        <div
          v-for="node in sortedNodes"
          :key="node.id"
          class="node-row"
          :class="{ 'row-online': node.status === 'online', 'row-offline': node.status === 'offline' }"
        >
          <span class="col-name" :title="node.name">{{ node.name }}</span>
          <span class="col-host" :title="node.host">{{ node.host }}</span>
          <span class="col-status">
            <span class="status-dot" :class="'status-' + node.status"></span>
            {{ statusLabel(node.status) }}
          </span>
          <span class="col-metric">{{ formatPercent(node.cpu_usage) }}</span>
          <span class="col-metric">{{ formatPercent(node.memory_usage) }}</span>
          <span class="col-metric">{{ formatPercent(node.disk_usage) }}</span>
          <span class="col-metric">{{ node.online_users || 0 }}</span>
          <span class="col-metric">{{ formatUptime(node.uptime) }}</span>
        </div>
      </div>
      <div v-else class="empty-state">
        {{ t('adminMonitor.empty') }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

const overview = ref({})
const nodes = ref([])
const wsStatus = ref('disconnected') // 'connecting' | 'connected' | 'disconnected' | 'reconnecting'
const reconnectSeconds = ref(0)
let ws = null
let reconnectTimer = null
let reconnectCountdown = null

const connectionClass = computed(() => `state-${wsStatus.value}`)
const connectionLabel = computed(() => {
  switch (wsStatus.value) {
    case 'connecting': return t('adminMonitor.connecting')
    case 'connected': return t('adminMonitor.connected')
    case 'reconnecting': return t('adminMonitor.reconnecting', { seconds: reconnectSeconds.value })
    default: return t('adminMonitor.disconnected')
  }
})

const sortedNodes = computed(() => {
  return [...nodes.value].sort((a, b) => {
    const order = { online: 0, pending: 1, offline: 2 }
    return (order[a.status] ?? 3) - (order[b.status] ?? 3)
  })
})

function statusLabel(status) {
  const map = {
    online: t('adminMonitor.status.online'),
    offline: t('adminMonitor.status.offline'),
    pending: t('adminMonitor.status.pending')
  }
  return map[status] || status
}

function formatPercent(val) {
  if (val == null || val === 0) return '0%'
  return val.toFixed(1) + '%'
}

function formatBytes(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return bytes.toFixed(2) + ' ' + units[i]
}

function formatUptime(seconds) {
  if (!seconds) return '0s'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
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

function connect() {
  if (ws) {
    ws.onclose = null
    ws.close()
    ws = null
  }
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (reconnectCountdown) {
    clearInterval(reconnectCountdown)
    reconnectCountdown = null
  }

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
    console.error(t('adminMonitor.errors.unsupported'), e)
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
      Object.assign(existing, update)
    } else {
      nodeMap.set(update.id, { ...update })
    }
  }
  nodes.value = Array.from(nodeMap.values())
}

function scheduleReconnect() {
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
  if (ws) {
    ws.onclose = null
    ws.close()
    ws = null
  }
  if (reconnectTimer) clearTimeout(reconnectTimer)
  if (reconnectCountdown) clearInterval(reconnectCountdown)
})
</script>

<style scoped>
.monitor-page {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 24px;
  font-weight: 600;
  margin-bottom: 4px;
}

.connection-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px;
  border-radius: 20px;
  font-size: 13px;
  font-weight: 500;
}

.connection-badge .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}

.state-connecting {
  background: rgba(245, 158, 11, 0.1);
  color: #f59e0b;
}
.state-connecting .dot { background: #f59e0b; }

.state-connected {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
}
.state-connected .dot { background: #10b981; }

.state-disconnected {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
}
.state-disconnected .dot { background: #ef4444; }

.state-reconnecting {
  background: rgba(107, 114, 128, 0.1);
  color: #6b7280;
}
.state-reconnecting .dot { background: #6b7280; animation: pulse 1s infinite; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}

.stat-card {
  background: var(--surface-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  border: 1px solid var(--border-color);
  position: relative;
  overflow: hidden;
}

.stat-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
}

.stat-card.primary::before { background: var(--primary-color); }
.stat-card.success::before { background: var(--success-color); }
.stat-card.danger::before { background: #ef4444; }
.stat-card.info::before { background: #3b82f6; }

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-color);
}

.stat-label {
  font-size: 14px;
  color: var(--text-secondary);
  margin-top: 4px;
}

.traffic-bar {
  display: flex;
  gap: 24px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 16px 24px;
  margin-bottom: 24px;
}

.traffic-item {
  display: flex;
  align-items: center;
  gap: 12px;
}

.traffic-item .label {
  color: var(--text-secondary);
  font-size: 14px;
}

.traffic-item .value {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-color);
}

.node-section h3 {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
}

.node-table {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.node-table-header {
  display: grid;
  grid-template-columns: 1.5fr 1.2fr 0.8fr 0.7fr 0.7fr 0.7fr 0.6fr 0.8fr;
  gap: 8px;
  padding: 12px 16px;
  background: var(--bg-color);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid var(--border-color);
}

.node-row {
  display: grid;
  grid-template-columns: 1.5fr 1.2fr 0.8fr 0.7fr 0.7fr 0.7fr 0.6fr 0.8fr;
  gap: 8px;
  padding: 12px 16px;
  font-size: 13px;
  border-bottom: 1px solid var(--border-color);
  align-items: center;
  transition: background 0.15s;
}

.node-row:last-child {
  border-bottom: none;
}

.node-row:hover {
  background: var(--bg-color);
}

.col-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.col-host {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
  font-family: var(--font-mono, monospace);
  font-size: 12px;
}

.col-status {
  display: flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.status-online { background: var(--success-color); }
.status-offline { background: #ef4444; }
.status-pending { background: #f59e0b; }

.col-metric {
  text-align: right;
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  color: var(--text-secondary);
}

.empty-state {
  text-align: center;
  padding: 48px 16px;
  color: var(--text-secondary);
  font-size: 14px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
  }

  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .traffic-bar {
    flex-direction: column;
    gap: 8px;
  }

  .node-table-header {
    display: none;
  }

  .node-row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 16px;
    padding: 16px;
  }

  .col-name {
    flex: 1 1 100%;
    font-size: 14px;
  }

  .col-host {
    flex: 1 1 100%;
  }

  .col-status, .col-metric {
    flex: 1 1 auto;
    text-align: left;
  }
}
</style>
