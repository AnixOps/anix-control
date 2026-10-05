<template>
  <div class="list-page nodes-page">
    <UiPageHeader :title="t('admin.nodes.title')" :description="t('admin.nodes.subtitle')">
      <template #actions>
        <UiButton :icon="KeyRound" data-testid="open-auth-key" @click="openAuthKey">{{ t('admin.nodes.actions.authKey') }}</UiButton>
        <UiButton :icon="Terminal" data-testid="open-deploy" @click="deployOpen = true">{{ t('admin.nodes.actions.deployParents') }}</UiButton>
        <UiButton variant="primary" :icon="Plus" data-testid="add-node" @click="openCreate">{{ t('admin.nodes.addNode') }}</UiButton>
      </template>
    </UiPageHeader>

    <NodeNotice v-if="overQuotaNodes.length > 0" tone="warning" data-testid="quota-banner">
      {{ t('admin.nodes.messages.quotaExceededBanner', { count: overQuotaNodes.length }) }}
    </NodeNotice>

    <UiDataTable
      :columns="columns"
      :rows="nodes"
      :label="t('admin.nodes.table.label')"
      :row-label="node => node.name"
      storage-key="admin.nodes"
      manual-sort
      :sort="sortState"
      manual-pagination
      :page="pagination.page"
      :page-size="pagination.size"
      :total="pagination.total"
      :loading="loading"
      :error="loadError"
      :error-title="t('admin.nodes.table.loadFailed')"
      :filtered="Boolean(search || statusFilter)"
      :empty-icon="Server"
      :empty-title="t('admin.nodes.table.empty')"
      :empty-description="t('admin.nodes.table.emptyDescription')"
      :row-actions="nodeActions"
      activatable
      @update:sort="changeSort"
      @update:page="changePage"
      @row-activate="openDetail"
      @retry="reload"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiSearchField
          v-model="search"
          class="list-page__search"
          :label="t('admin.nodes.filters.search')"
          :placeholder="t('admin.nodes.filters.searchPlaceholder')"
          data-testid="node-search"
          @update:model-value="scheduleSearch"
          @submit="searchNow"
        />
        <UiFilterChips v-model="statusFilter" :label="t('admin.nodes.filters.label')" :options="statusChips" />
        <p class="list-page__summary" data-testid="node-stats">
          <span>{{ t('admin.nodes.stats.total') }} <strong>{{ stats.total }}</strong></span>
          <span>{{ t('admin.nodes.stats.online') }} <strong>{{ stats.online }}</strong></span>
          <span>{{ t('admin.nodes.stats.offline') }} <strong>{{ stats.offline }}</strong></span>
          <span>{{ t('admin.nodes.stats.pending') }} <strong>{{ stats.pending }}</strong></span>
        </p>
      </template>
      <template #empty-actions>
        <UiButton :icon="KeyRound" @click="openAuthKey">{{ t('admin.nodes.actions.authKey') }}</UiButton>
        <UiButton variant="primary" :icon="Plus" @click="openCreate">{{ t('admin.nodes.addNode') }}</UiButton>
      </template>
      <template #cell-name="{ row, card }">
        <span class="node-name">
          <span class="node-name__text">{{ row.name }}</span>
          <span v-if="!card && splitTags(row.tags).length" class="node-name__tags">
            <UiBadge v-for="tag in splitTags(row.tags)" :key="tag" tone="neutral" :dot="false" :label="tag" />
          </span>
        </span>
      </template>
      <template #cell-address="{ row }">
        <code class="node-address">{{ row.address || '—' }}</code>
      </template>
      <template #cell-status="{ row }">
        <span class="node-badges">
          <UiBadge :status="statusName(row.status)" :label="t(`admin.nodes.statusText.${statusName(row.status)}`)" />
          <UiBadge
            v-if="row.runtime_checked_at"
            :tone="row.runtime_healthy ? 'success' : 'danger'"
            :label="row.runtime_healthy ? t('admin.nodes.table.runtimeHealthy') : t('admin.nodes.table.runtimeUnhealthy')"
            :title="row.runtime_healthy ? undefined : (row.runtime_error || undefined)"
          />
          <UiBadge v-if="isOverQuota(row)" tone="warning" :label="t('admin.nodes.table.quotaExceeded')" />
        </span>
      </template>
      <template #cell-connection="{ row }">
        <NodeConnectionBadge :entry="transportEntries.get(nodeRef(row.id))" :status="transportStatus" />
      </template>
      <template #cell-certificate="{ row }">
        <NodeCertificateCell :entry="transportEntries.get(nodeRef(row.id))" :status="transportStatus" />
      </template>
      <template #cell-lastHeartbeat="{ row }">
        <time v-if="row.last_check_at" :title="format.dateTime(row.last_check_at)">{{ format.relativeTime(row.last_check_at) }}</time>
        <span v-else>{{ t('admin.nodes.table.never') }}</span>
      </template>
      <template #cell-monthlyQuota="{ row }">
        <UiUsageBar
          v-if="row.monthly_limit"
          :value="monthlyUsed(row)"
          :max="Number(row.monthly_limit)"
          :text="`${format.bytes(monthlyUsed(row))} / ${format.bytes(row.monthly_limit)}`"
        />
        <span v-else>—</span>
      </template>
    </UiDataTable>

    <NodeFormSheet v-model:open="formOpen" :node="editingNode" :candidates="nodes" @saved="reload" />

    <UiSheet
      v-model:open="authKeyOpen"
      size="lg"
      :title="t('admin.nodes.authKey.title')"
      data-testid="auth-key-sheet"
    >
      <NodeAuthKeyPanel :deploy="deploy" settings-link @edit-settings="openDeployFromKey" />
    </UiSheet>

    <NodeDeploySheet v-model:open="deployOpen" :nodes="parentNodes" :deploy="deploy" />
  </div>
</template>

<script setup>
// Nodes (plan §7.1, §8.2): the proxy nodes in a UiDataTable with server
// search, status chips, sort and pages (all four in the URL), a row opens the
// node page (/admin/nodes/:id). The Agent connection type and certificate of
// the nodes in view come from one batched inventory call after the list has
// loaded (it never holds the list up). The list keeps add, edit, sync and delete in
// the row menu, and the Agent registration key and the parent-node Ansible
// helper as page actions. Node (proxy service) is not ForwardNode.
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { FileText, KeyRound, Layers, Pencil, Plus, RefreshCcw, Server, SquareArrowOutUpRight, Terminal, Trash2 } from '@lucide/vue'
import { getNodeStats, getNodes } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import { useRouteIntent } from '@/composables/useRouteIntent'
import { SORT_ORDERS, createListSort } from '@/utils/listSort'
import NodeAuthKeyPanel from './nodes/NodeAuthKeyPanel.vue'
import NodeCertificateCell from './nodes/NodeCertificateCell.vue'
import NodeConnectionBadge from './nodes/NodeConnectionBadge.vue'
import NodeDeploySheet from './nodes/NodeDeploySheet.vue'
import NodeFormSheet from './nodes/NodeFormSheet.vue'
import NodeNotice from './nodes/NodeNotice.vue'
import { nodeRef } from './nodes/agentConnection'
import {
  NODE_STATUS, isOverQuota, monthlyUsed, normalizeNode, readNodePage, readNodePayload,
  rememberListQuery, rememberNodeNames, splitTags, statusName
} from './nodes/nodeData'
import { useNodeActions } from './nodes/useNodeActions'
import { useNodeDeploy } from './nodes/useNodeDeploy'
import { useNodeTransports } from './nodes/useNodeTransports'

const { t } = useAppI18n()
const format = useFormat()
const route = useRoute()
const router = useRouter()
const { syncingNodeIds, syncNode, confirmDelete } = useNodeActions()
const deploy = reactive(useNodeDeploy())

const STATUS_VALUES = Object.keys(NODE_STATUS)
// Sorting happens on the server (GET /admin/nodes sort and order): only the
// columns the API sorts by are sortable, keyed here by the table's column key.
// The shown status and the protocol count are derived (the API has no sort
// for them), and sorting the 20 rows of one page would say nothing about the
// list, so those headers stay plain.
const NODE_SORT = createListSort({ id: 'id', name: 'name', address: 'host', load: 'cpu_usage', lastHeartbeat: 'last_check_at' })
const firstQuery = (key) => {
  const value = route?.query?.[key]
  return String(Array.isArray(value) ? value[0] : value ?? '')
}

const loading = ref(false)
const loadError = ref(null)
const nodes = ref([])
const stats = reactive({ total: 0, online: 0, offline: 0, pending: 0 })
const pagination = reactive({ page: Math.max(1, Number(firstQuery('page')) || 1), size: 20, total: 0 })
const search = ref(firstQuery('q'))
const statusFilter = ref(STATUS_VALUES.includes(firstQuery('status')) ? firstQuery('status') : '')
const sortState = ref(NODE_SORT.fromQuery(
  NODE_SORT.values.includes(firstQuery('sort')) ? firstQuery('sort') : '',
  SORT_ORDERS.includes(firstQuery('order')) ? firstQuery('order') : ''
))
const { entries: transportEntries, status: transportStatus, load: loadTransports, reset: resetTransports } = useNodeTransports()

const formOpen = ref(false)
const editingNode = ref(null)
const authKeyOpen = ref(false)
const deployOpen = ref(false)

const overQuotaNodes = computed(() => nodes.value.filter(isOverQuota))
const parentNodes = computed(() => nodes.value.filter(node => !node.parent_id))

// The API's `status` filter (the stored status).
const statusChips = computed(() => ['online', 'offline', 'disabled', 'pending'].map(value => ({
  value,
  label: t(`admin.nodes.statusText.${value}`)
})))

const parentName = (parentId) => {
  if (!parentId) return ''
  return nodes.value.find(n => n.id === parentId)?.name || `#${parentId}`
}

// The Agent's last report; only meaningful while the node is online.
const loadText = (node) => {
  if (!node.last_check_at || Number(node.status) !== NODE_STATUS.online) return '—'
  return t('admin.nodes.table.loadValue', {
    cpu: format.percent(Number(node.cpu_usage || 0) / 100),
    memory: format.percent(Number(node.memory_usage || 0) / 100)
  })
}

// Connection and Certificate take the room of 版本 and 负载 at 1440 px: those
// start hidden (the node page has both, and the table settings bring them back).
const columns = computed(() => [
  { key: 'id', label: t('admin.nodes.table.id'), numeric: true, hidden: true, width: 72 },
  { key: 'name', label: t('admin.nodes.table.name'), primary: true, hideable: false },
  { key: 'address', label: t('admin.nodes.table.address'), secondary: true, nowrap: true },
  { key: 'protocols', label: t('admin.nodes.table.protocols'), numeric: true, align: 'end', nowrap: true, value: node => node.protocols?.length || 0, card: false },
  { key: 'status', label: t('admin.nodes.table.status') },
  { key: 'connection', label: t('admin.nodes.table.connection'), nowrap: true, breakpoint: 'lg' },
  { key: 'agentVersion', label: t('admin.nodes.table.agentVersion'), nowrap: true, value: node => node.server_version || '', breakpoint: 'lg', hidden: true, card: false },
  { key: 'certificate', label: t('admin.nodes.table.certificate'), nowrap: true, breakpoint: 'lg', card: false },
  { key: 'load', label: t('admin.nodes.table.load'), nowrap: true, numeric: true, value: loadText, firstDirection: 'desc', breakpoint: 'lg', hidden: true, card: false },
  { key: 'lastHeartbeat', label: t('admin.nodes.table.lastHeartbeat'), nowrap: true, firstDirection: 'desc' },
  { key: 'parent', label: t('admin.nodes.table.parent'), value: node => parentName(node.parent_id), hidden: true },
  { key: 'traffic', label: t('admin.nodes.table.traffic'), numeric: true, align: 'end', nowrap: true, hidden: true, value: node => format.bytes(node.traffic_today || 0) },
  { key: 'monthlyQuota', label: t('admin.nodes.table.monthlyQuota'), hidden: true, card: false }
].map(column => (NODE_SORT.isSortable(column.key) ? { ...column, sortable: true } : column)))

const nodeActions = node => [
  { key: 'open', label: t('admin.nodes.actions.open'), icon: SquareArrowOutUpRight, onSelect: () => openDetail(node) },
  { key: 'protocols', label: t('admin.nodes.actions.manageProtocols'), icon: Layers, onSelect: () => openDetail(node, 'protocols') },
  { key: 'logs', label: t('admin.nodes.actions.logs'), icon: FileText, onSelect: () => openDetail(node, 'logs') },
  { key: 'sync', label: t('admin.nodes.actions.syncReload'), icon: RefreshCcw, disabled: syncingNodeIds.has(node.id), onSelect: () => syncNode(node) },
  { key: 'edit', label: t('admin.nodes.actions.edit'), icon: Pencil, onSelect: () => openEdit(node) },
  { key: 'delete', label: t('admin.nodes.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteFromList(node) }
]

function listParams() {
  const params = { page: pagination.page, page_size: pagination.size }
  if (search.value.trim()) params.search = search.value.trim()
  if (statusFilter.value) params.status = NODE_STATUS[statusFilter.value]
  return { ...params, ...NODE_SORT.toParams(sortState.value) }
}

// The list state in the URL (q, status, sort, order, page), so a reload, a
// shared link or 返回节点 from a node page shows the same list.
function syncQuery({ url = true } = {}) {
  const query = {}
  if (search.value.trim()) query.q = search.value.trim()
  if (statusFilter.value) query.status = statusFilter.value
  const { sort, order } = NODE_SORT.toQuery(sortState.value)
  if (sort) {
    query.sort = sort
    query.order = order
  }
  if (pagination.page > 1) query.page = String(pagination.page)
  rememberListQuery(query)
  if (!url || !router || !route) return
  const current = { ...route.query }
  delete current.q
  delete current.status
  delete current.sort
  delete current.order
  delete current.page
  router.replace({ path: route.path, query: { ...current, ...query } })
}

const loadNodes = async () => {
  loading.value = true
  loadError.value = null
  try {
    const payload = readNodePage(await getNodes(listParams()))
    nodes.value = (payload.list || []).map(normalizeNode)
    pagination.total = payload.total || 0
    rememberNodeNames(nodes.value)
    // The Agent connection of the nodes in view: one call, after the list.
    void loadTransports(nodes.value.map(node => node.id))
    return true
  } catch (e) {
    console.error('Failed to load nodes:', e)
    loadError.value = e
    nodes.value = []
    resetTransports()
    return false
  } finally {
    loading.value = false
  }
}

const loadStats = async () => {
  try {
    const payload = readNodePayload(await getNodeStats())
    if (payload && typeof payload === 'object') Object.assign(stats, payload)
  } catch (e) {
    console.error('Failed to load stats:', e)
  }
}

async function reload() {
  if (await loadNodes()) await loadStats()
}

// A header click sorts the whole list on the server, from the first page.
function changeSort(next) {
  sortState.value = next
  pagination.page = 1
  syncQuery()
  loadNodes()
}

function changePage(page) {
  pagination.page = page
  syncQuery()
  loadNodes()
}

let searchTimer = null
function scheduleSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(searchNow, 300)
}

function searchNow() {
  clearTimeout(searchTimer)
  pagination.page = 1
  syncQuery()
  loadNodes()
}

watch(statusFilter, () => searchNow())

function clearFilters() {
  search.value = ''
  // Clearing the chip reloads through the watcher.
  if (statusFilter.value) statusFilter.value = ''
  else searchNow()
}

function openDetail(node, section) {
  rememberNodeNames(nodes.value)
  router.push({ path: `/admin/nodes/${node.id}`, query: section ? { section } : {} })
}

function openCreate() {
  editingNode.value = null
  formOpen.value = true
}

function openEdit(node) {
  editingNode.value = node
  formOpen.value = true
}

async function deleteFromList(node) {
  if (await confirmDelete(node)) await reload()
}

async function openAuthKey() {
  deploy.authKeyError = ''
  authKeyOpen.value = true
  await deploy.loadAuthKeysPreview()
}

function openDeployFromKey() {
  authKeyOpen.value = false
  deployOpen.value = true
}

// The command palette's 添加节点 opens this page with ?create=1.
const routeIntent = useRouteIntent(['create'], intent => {
  if (intent.create === '1') openCreate()
})

onMounted(async () => {
  if (routeIntent.create === '1') openCreate()
  syncQuery({ url: false })
  if (await loadNodes()) await loadStats()
})

onBeforeUnmount(() => clearTimeout(searchTimer))

defineExpose({ nodes, stats, pagination, loadNodes, loadStats, deploy })
</script>

<style scoped>
.node-name {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  min-width: 0;
}

.node-name__text {
  font-weight: var(--weight-medium);
}

.node-name__tags,
.node-badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.node-badges {
  min-width: 0;
}

.node-address {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}
</style>
