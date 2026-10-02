<template>
  <div class="list-page forward-nodes-page">
    <UiPageHeader :title="t('forwardNodesPage.title')" :description="t('forwardNodesPage.descriptions.nodex')">
      <template #actions>
        <UiIconButton variant="secondary" :icon="RefreshCw" :label="t('runtime.nodeXTopology.actions.refresh')" :disabled="pageBusy" data-test="forward-nodes-refresh" @click="refreshAll" />
        <UiButton :icon="PlugZap" data-test="forward-nodes-test" @click="openConnection()">{{ t('runtime.nodeXTopology.actions.testConnection') }}</UiButton>
        <UiButton variant="primary" :icon="Plus" data-test="forward-nodes-add" @click="openEditor()">{{ t('runtime.nodeXTopology.actions.addNode') }}</UiButton>
      </template>
    </UiPageHeader>

    <ForwardNodesModeNav current="nodex" />

    <UiDataTable
      :columns="columns"
      :rows="nodes"
      :label="t('forwardNodesPage.nodex.tableLabel')"
      :row-label="node => node.name"
      storage-key="admin.forward-nodes"
      :loading="loading"
      :error="error"
      :error-title="t('runtime.nodeXTopology.messages.loadNodesFailed')"
      :filtered="Boolean(typeFilter || statusFilter)"
      :empty-icon="Network"
      :empty-title="t('runtime.nodeXTopology.emptyTitle')"
      :empty-description="t('runtime.nodeXTopology.emptyText')"
      :row-actions="nodeActions"
      :page="page"
      :page-size="pageSize"
      :total="total"
      manual-pagination
      activatable
      @row-activate="openDetail"
      @update:page="changePage"
      @retry="loadNodes"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiFilterChips v-model="typeFilter" :label="t('forwardNodesPage.nodex.typeFilter')" :options="typeChips" />
        <UiFilterChips v-model="statusFilter" :label="t('forwardNodesPage.nodex.statusFilter')" :options="statusChips" />
        <p class="list-page__summary" data-test="forward-nodes-summary">
          <span>{{ t('forwardNodesPage.nodex.summary.relay') }} <strong>{{ stats.relay_nodes }}</strong> · {{ t('forwardNodesPage.nodex.summary.online') }} {{ stats.online_relay }}</span>
          <span>{{ t('forwardNodesPage.nodex.summary.exit') }} <strong>{{ stats.exit_nodes }}</strong> · {{ t('forwardNodesPage.nodex.summary.online') }} {{ stats.online_exit }}</span>
          <span>{{ t('forwardNodesPage.nodex.summary.upload') }} <strong>{{ format.bytes(stats.total_upload) }}</strong></span>
          <span>{{ t('forwardNodesPage.nodex.summary.download') }} <strong>{{ format.bytes(stats.total_download) }}</strong></span>
        </p>
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openEditor()">{{ t('runtime.nodeXTopology.actions.addNode') }}</UiButton>
      </template>
      <template #cell-name="{ row }">
        <span class="fn-stack">
          <span>{{ row.name }}</span>
          <code class="fn-sub">{{ row.host }}:{{ row.port }}</code>
        </span>
      </template>
      <template #cell-type="{ row }">
        <UiBadge :tone="row.type === 'exit' ? 'info' : 'neutral'" :dot="false" :label="typeLabel(row.type)" />
      </template>
      <template #cell-status="{ row }">
        <span class="fn-badges">
          <UiBadge :status="row.status === 1 ? 'online' : 'offline'" :label="row.status === 1 ? t('runtime.nodeXTopology.status.online') : t('runtime.nodeXTopology.status.offline')" />
          <UiBadge v-if="!row.enabled" status="disabled" :label="t('runtime.nodeXTopology.status.disabled')" />
        </span>
      </template>
      <template #cell-api="{ row }">
        <code class="fn-mono">{{ row.host }}:{{ row.apiPort || '-' }}</code>
      </template>
      <template #cell-result="{ row }">
        <span v-if="actions.isPending(row.id)" class="fn-result">{{ pendingLabel(row) }}</span>
        <span v-else-if="actions.results[row.id]" :class="['fn-result', actions.results[row.id].success ? 'is-ok' : 'is-fail']">{{ actions.results[row.id].message }}</span>
        <span v-else>—</span>
      </template>
    </UiDataTable>
    <p class="list-page__note">{{ t('forwardNodesPage.nodex.onlineNote') }}</p>

    <NodeXRulesPanel ref="rulesPanel" @changed="reloadNodesAndStats" />

    <NodeXNodeDialog v-model:open="editorOpen" :node-id="editingId" @saved="reloadNodesAndStats" />
    <NodeXConnectionDialog v-model:open="connectionOpen" :node="connectionNode" />
  </div>
</template>

<script setup>
// 转发节点 › NodeX 节点 (/admin/forward/nodes, UI U7): the stateful NodeX
// relay/exit nodes (?scope=nodex) and the legacy rules. Execution plane,
// separate from the flux-panel forward control plane (AGENTS.md); the
// other run modes keep their own pages, linked by ForwardNodesModeNav.
// A row opens the node's detail page (/admin/forward/nodes/:id).
import { computed, inject, onMounted, reactive, ref, watch } from 'vue'
import { routerKey } from 'vue-router'
import { Network, Pencil, PlugZap, Plus, Power, RefreshCcwDot, RefreshCw, Stethoscope, Trash2 } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { getForwardNodes, getForwardStats } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import ForwardNodesModeNav from './forward-nodes/ForwardNodesModeNav.vue'
import NodeXConnectionDialog from './forward-nodes/NodeXConnectionDialog.vue'
import NodeXNodeDialog from './forward-nodes/NodeXNodeDialog.vue'
import NodeXRulesPanel from './forward-nodes/NodeXRulesPanel.vue'
import { errorMessage, listOf, normalizeNode, unwrapForwardResponse } from './forward-nodes/forwardNodeModel'
import { useNodeXNodeActions } from './forward-nodes/useNodeXNodeActions'

const { t, translateLiteral } = useAppI18n()
const toast = useToast()
const format = useFormat()
const router = inject(routerKey, null)

const stats = reactive({ relay_nodes: 0, exit_nodes: 0, online_relay: 0, online_exit: 0, total_upload: 0, total_download: 0 })
const statsLoading = ref(false)
const nodes = ref([])
const loading = ref(false)
const error = ref(null)
const page = ref(1)
const pageSize = 12
const total = ref(0)
// Server filters: `type` (relay/exit) and `status` (1 reachable, 0 not).
const typeFilter = ref('')
const statusFilter = ref('')
const rulesPanel = ref(null)
const editorOpen = ref(false)
const editingId = ref(null)
const connectionOpen = ref(false)
const connectionNode = ref(null)

const actions = reactive(useNodeXNodeActions({ onChanged: kind => (kind === 'delete' ? Promise.all([loadNodes(), loadStats(), rulesPanel.value?.load()]) : reloadNodesAndStats()) }))

const pageBusy = computed(() => statsLoading.value || loading.value)

const typeChips = computed(() => [
  { value: 'relay', label: t('runtime.nodeXTopology.filters.relay') },
  { value: 'exit', label: t('runtime.nodeXTopology.filters.exit') }
])
const statusChips = computed(() => [
  { value: '1', label: t('runtime.nodeXTopology.filters.online') },
  { value: '0', label: t('runtime.nodeXTopology.filters.offline') }
])

const columns = computed(() => [
  { key: 'name', label: t('forwardNodesPage.nodex.columns.name'), primary: true },
  { key: 'type', label: t('forwardNodesPage.nodex.columns.type') },
  { key: 'status', label: t('forwardNodesPage.nodex.columns.status'), secondary: true },
  { key: 'api', label: t('forwardNodesPage.nodex.columns.managementApi'), hidden: true, nowrap: true },
  { key: 'region', label: t('forwardNodesPage.nodex.columns.regionIsp'), value: node => [node.region, node.isp].filter(Boolean).join(' / '), breakpoint: 'lg' },
  { key: 'latency', label: t('forwardNodesPage.nodex.columns.latency'), numeric: true, align: 'end', nowrap: true, value: node => node.latency ? `${node.latency} ms` : '' },
  { key: 'currentConn', label: t('forwardNodesPage.nodex.columns.connections'), numeric: true, align: 'end' },
  { key: 'traffic', label: t('forwardNodesPage.nodex.columns.traffic'), numeric: true, nowrap: true, breakpoint: 'md', value: node => `${format.bytes(node.totalUpload)} / ${format.bytes(node.totalDownload)}` },
  { key: 'weight', label: t('forwardNodesPage.nodex.columns.weight'), numeric: true, hidden: true, value: node => `${node.weight} / ${node.maxConn || '-'}` },
  { key: 'lastCheck', label: t('forwardNodesPage.nodex.columns.lastCheck'), hidden: true, value: node => node.lastCheck ? format.dateTime(node.lastCheck) : '' },
  { key: 'uptime', label: t('forwardNodesPage.nodex.columns.uptime'), numeric: true, hidden: true, value: node => Number.isFinite(node.uptime) ? `${node.uptime.toFixed(1)}%` : '' },
  { key: 'result', label: t('forwardNodesPage.nodex.columns.result'), card: false, breakpoint: 'lg' }
])

function nodeActions(node) {
  const busy = actions.isPending(node.id)
  return [
    { key: 'edit', label: t('runtime.nodeXTopology.actions.edit'), icon: Pencil, onSelect: () => openEditor(node) },
    { key: 'check', label: t('runtime.nodeXTopology.actions.healthCheck'), icon: Stethoscope, disabled: busy, onSelect: () => actions.check(node) },
    { key: 'sync', label: t('runtime.nodeXTopology.actions.syncStats'), icon: RefreshCcwDot, disabled: busy, onSelect: () => actions.sync(node) },
    { key: 'test', label: t('runtime.nodeXTopology.actions.testConnection'), icon: PlugZap, onSelect: () => openConnection(node) },
    { key: 'toggle', label: node.enabled ? t('runtime.nodeXTopology.actions.disable') : t('runtime.nodeXTopology.actions.enable'), icon: Power, disabled: busy, onSelect: () => actions.toggle(node) },
    { key: 'delete', label: t('runtime.nodeXTopology.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => actions.remove(node) }
  ]
}

function pendingLabel(node) {
  if (actions.isPending(node.id, 'check')) return t('runtime.nodeXTopology.actions.checking')
  if (actions.isPending(node.id, 'sync')) return t('runtime.nodeXTopology.actions.syncing')
  return '…'
}

function typeLabel(value) {
  return value === 'relay' || value === 'exit' ? t(`runtime.nodeXTopology.filters.${value}`) : (value || '-')
}

function unwrap(response) {
  return unwrapForwardResponse(response, t('runtime.nodeXTopology.validation.requestFailed'))
}

async function loadStats() {
  statsLoading.value = true
  try {
    const payload = unwrap(await getForwardStats())
    for (const key of Object.keys(stats)) stats[key] = Number(payload[key] ?? 0)
  } catch (err) {
    for (const key of Object.keys(stats)) stats[key] = 0
    toast.error(errorMessage(err, t('runtime.nodeXTopology.messages.loadStatsFailed'), translateLiteral))
  } finally {
    statsLoading.value = false
  }
}

async function loadNodes() {
  loading.value = true
  error.value = null
  try {
    const params = { page: page.value, page_size: pageSize, scope: 'nodex' }
    if (typeFilter.value) params.type = typeFilter.value
    if (statusFilter.value) params.status = Number(statusFilter.value)
    const payload = unwrap(await getForwardNodes(params))
    const list = listOf(payload)
    nodes.value = list.map(normalizeNode)
    total.value = Number(payload?.total ?? list.length)
  } catch (err) {
    nodes.value = []
    total.value = 0
    error.value = errorMessage(err, t('runtime.nodeXTopology.messages.loadNodesFailed'), translateLiteral)
  } finally {
    loading.value = false
  }
}

function reloadNodesAndStats() {
  return Promise.all([loadNodes(), loadStats()])
}

async function refreshAll() {
  await Promise.all([loadStats(), loadNodes(), rulesPanel.value?.load()])
}

function changePage(next) {
  page.value = next
  loadNodes()
}

function clearFilters() {
  typeFilter.value = ''
  statusFilter.value = ''
}

watch([typeFilter, statusFilter], () => {
  page.value = 1
  loadNodes()
})

function openEditor(node = null) {
  editingId.value = node?.id || null
  editorOpen.value = true
}

function openConnection(node = null) {
  connectionNode.value = node
  connectionOpen.value = true
}

function openDetail(node) {
  if (node?.id && router) router.push(`/admin/forward/nodes/${node.id}`)
}

onMounted(() => {
  loadStats()
  loadNodes()
})
</script>

<style scoped>
.fn-stack {
  display: inline-flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.fn-sub,
.fn-mono {
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.fn-sub {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  white-space: nowrap;
}

.fn-badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.fn-result {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  font-size: var(--type-callout-size);
}

.fn-result.is-ok {
  color: color-mix(in srgb, var(--success) 78%, var(--label-1));
}

.fn-result.is-fail {
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
}
</style>
