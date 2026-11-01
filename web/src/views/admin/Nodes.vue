<template>
  <div class="list-page nodes-page">
    <ForwardAreaNav area="nodes" />
    <UiPageHeader :title="t('admin.nodes.title')" :description="t('admin.nodes.subtitle')">
      <template #actions>
        <UiButton :icon="Rocket" data-testid="upgrade-nodes" @click="openAgentUpgrade">{{ t('shell.palette.actions.upgradeNodes') }}</UiButton>
        <UiButton variant="primary" :icon="Terminal" data-testid="join-node" @click="openJoin()">{{ t('admin.nodes.join.title') }}</UiButton>
      </template>
    </UiPageHeader>

    <NodeNotice v-if="pendingApprovals > 0" tone="warning" data-testid="approval-banner">
      {{ t('admin.nodes.approval.banner', { count: pendingApprovals }) }}
      <template #actions>
        <UiButton size="sm" data-testid="approval-banner-view" @click="showAwaiting">{{ t('admin.nodes.approval.view') }}</UiButton>
      </template>
    </NodeNotice>

    <UiTabs v-model="tab" variant="underline" :aria-label="t('admin.nodes.tabs.label')" :items="tabItems">
      <template #nodes>
        <div class="nodes-tab">
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
              <UiButton variant="primary" :icon="Terminal" @click="openJoin()">{{ t('admin.nodes.join.title') }}</UiButton>
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
                  v-if="row.approval === 'pending'"
                  tone="warning"
                  :dot="false"
                  :label="t('admin.nodes.approval.pending')"
                  :title="t('admin.nodes.approval.pendingHint')"
                  data-testid="node-approval-pending"
                />
                <UiBadge v-else-if="row.approval === 'rejected'" tone="danger" :dot="false" :label="t('admin.nodes.approval.rejected')" />
                <UiBadge
                  v-if="row.runtime_checked_at"
                  :tone="row.runtime_healthy ? 'success' : 'danger'"
                  :label="row.runtime_healthy ? t('admin.nodes.table.runtimeHealthy') : t('admin.nodes.table.runtimeUnhealthy')"
                  :title="row.runtime_healthy ? undefined : (row.runtime_error || undefined)"
                />
                <UiBadge v-if="isOverQuota(row)" tone="warning" :label="t('admin.nodes.table.quotaExceeded')" />
              </span>
            </template>
            <template #cell-nodeGroup="{ row }">
              <span v-if="row.node_group_name">{{ row.node_group_name }}</span>
              <span v-else class="node-muted">—</span>
            </template>
            <template #cell-connection="{ row }">
              <NodeConnectionBadge :entry="transportEntries.get(nodeRef(row.id))" :status="transportStatus" />
            </template>
            <template #cell-certificate="{ row }">
              <NodeCertificateCell :entry="transportEntries.get(nodeRef(row.id))" :status="transportStatus" />
            </template>
            <template #cell-agentVersion="{ row }">
              <span v-if="agentVersionOf(row)" data-testid="node-agent-version">{{ agentVersionOf(row) }}</span>
              <span v-else class="node-muted">—</span>
              <span
                v-if="kernelVersionOf(row)"
                class="node-kernel-version"
                :title="t('admin.nodes.table.kernelVersion')"
                data-testid="node-kernel-version"
              >anx-node {{ kernelVersionOf(row) }}</span>
            </template>
            <template #cell-serviceStatus="{ row }">
              <NodeServiceStatusCell :entry="serviceEntries.get(Number(row.id))" :status="serviceStatus" />
            </template>
            <template #cell-lastSeen="{ row }">
              <time v-if="lastSeenOf(row)" :title="format.dateTime(lastSeenOf(row))">{{ format.relativeTime(lastSeenOf(row)) }}</time>
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
        </div>
      </template>
      <template #groups>
        <NodeGroupsPanel
          :groups="groupsState.groups.value"
          :loading="groupsState.loading.value"
          :error="groupsState.error.value"
          :busy="groupsState.busy.value"
          @join="group => openJoin(group)"
          @edit="openGroupEdit"
          @create="openGroupCreate"
          @delete="deleteGroup"
          @reload="groupsState.load()"
          @toggle-approval="(group, value) => toggleApproval(group, value)"
        />
      </template>
    </UiTabs>

    <NodeFormSheet v-model:open="formOpen" :node="editingNode" :candidates="nodes" @saved="reload" />
    <NodeJoinSheet v-model:open="joinOpen" :group-id="joinGroupId" @changed="reloadAll" />
    <NodeGroupSheet v-model:open="groupFormOpen" :group="editingGroup" @saved="reloadAll" />
  </div>
</template>

<script setup>
// Nodes (plan §7.1, §8.2; 5.0 node onboarding): one entry point for a new
// machine, 接入节点, and two tabs. 节点 is the proxy nodes in a UiDataTable
// with server search, status chips (and 待确认, the nodes waiting for an
// administrator), sort and pages (all in the URL); a row opens the node page
// (/admin/nodes/:id) and pending rows carry 批准 and 拒绝. The Agent connection,
// certificate and version of the nodes in view come from one batched
// inventory call after the list has loaded (it never holds the list up), and
// so does their systemd services summary (服务状态, the machine-telemetry
// package's; a dash without the package, its host or its permission). 节点组
// lists the node groups behind the join command. There is one kind of node:
// relaying is the network model's (ANX), not a kind of node.
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Check, Cpu, FileText, Layers, Pencil, RefreshCcw, Rocket, Server, SquareArrowOutUpRight, Terminal, Trash2, X } from '@lucide/vue'
import { getNodeStats } from '@/api/admin'
import { approveKernelNode, getKernelAdminNodes, rejectKernelNode } from '@/api/kernel'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiTabs from '@/ui/UiTabs.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { openAgentUpgrade, openKernelUpgrade } from '@/composables/useAgentUpgradeSheet'
import { useAppI18n } from '@/composables/useAppI18n'
import { useRouteIntent } from '@/composables/useRouteIntent'
import { SORT_ORDERS, createListSort } from '@/utils/listSort'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import NodeCertificateCell from './nodes/NodeCertificateCell.vue'
import NodeConnectionBadge from './nodes/NodeConnectionBadge.vue'
import NodeFormSheet from './nodes/NodeFormSheet.vue'
import NodeGroupSheet from './nodes/NodeGroupSheet.vue'
import NodeGroupsPanel from './nodes/NodeGroupsPanel.vue'
import NodeJoinSheet from './nodes/NodeJoinSheet.vue'
import NodeNotice from './nodes/NodeNotice.vue'
import NodeServiceStatusCell from './nodes/NodeServiceStatusCell.vue'
import { nodeRef } from './nodes/agentConnection'
import {
  NODE_STATUS, isOverQuota, monthlyUsed, normalizeNode, readNodePage, readNodePayload,
  rememberListQuery, rememberNodeNames, splitTags, statusName
} from './nodes/nodeData'
import { readKernelError } from './nodes/nodeGroupData'
import { useNodeActions } from './nodes/useNodeActions'
import { useNodeGroups } from './nodes/useNodeGroups'
import { useNodeServiceStatus } from './nodes/useNodeServiceStatus'
import { useNodeTransports } from './nodes/useNodeTransports'

const { t } = useAppI18n()
const format = useFormat()
const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const { syncingNodeIds, syncNode, confirmDelete } = useNodeActions()
const groupsState = useNodeGroups()

const STATUS_VALUES = Object.keys(NODE_STATUS)
// The filter chip of nodes waiting for approval (GET /admin/nodes?approval=pending),
// next to the stored statuses.
const AWAITING = 'awaiting'
// Sorting happens on the server (GET /admin/nodes sort and order): only the
// columns the API sorts by are sortable, keyed here by the table's column key.
// The shown status and the protocol count are derived (the API has no sort
// for them), and sorting the 20 rows of one page would say nothing about the
// list, so those headers stay plain.
const NODE_SORT = createListSort({ id: 'id', name: 'name', address: 'host', load: 'cpu_usage', lastSeen: 'last_check_at' })
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
const statusFilter = ref([...STATUS_VALUES, AWAITING].includes(firstQuery('status')) ? firstQuery('status') : '')
const sortState = ref(NODE_SORT.fromQuery(
  NODE_SORT.values.includes(firstQuery('sort')) ? firstQuery('sort') : '',
  SORT_ORDERS.includes(firstQuery('order')) ? firstQuery('order') : ''
))
const { entries: transportEntries, status: transportStatus, load: loadTransports, reset: resetTransports } = useNodeTransports()
const { entries: serviceEntries, status: serviceStatus, load: loadServiceStatus, reset: resetServiceStatus } = useNodeServiceStatus()

const formOpen = ref(false)
const editingNode = ref(null)
const joinOpen = ref(false)
const joinGroupId = ref(null)
const groupFormOpen = ref(false)
const editingGroup = ref(null)

// ?tab=groups; 节点 is the default and leaves the query clean.
const TABS = ['groups']
function setTab(value) {
  const query = { ...route.query }
  if (TABS.includes(value)) query.tab = value
  else delete query.tab
  return router.replace({ path: route.path, query })
}
const tab = computed({
  get: () => {
    const value = route?.query?.tab
    return value === 'groups' ? 'groups' : 'nodes'
  },
  set: (value) => { void setTab(value) }
})
const tabItems = computed(() => [
  { value: 'nodes', label: t('admin.nodes.tabs.nodes') },
  { value: 'groups', label: t('admin.nodes.tabs.groups'), count: groupsState.groups.value.length }
])

const overQuotaNodes = computed(() => nodes.value.filter(isOverQuota))
// Machines waiting for approval in any group.
const pendingApprovals = computed(() => groupsState.pendingTotal.value)

// The API's `status` filter (the stored status), and 待确认.
const statusChips = computed(() => [...['online', 'offline', 'disabled', 'pending'].map(value => ({
  value,
  label: t(`admin.nodes.statusText.${value}`)
})), { value: AWAITING, label: t('admin.nodes.approval.pending') }])

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

// What the Agent reports on its stream (the inventory), with the node's own
// last check and version while the inventory has not answered or has no row.
const entryOf = node => transportEntries.value.get(nodeRef(node.id))
const lastSeenOf = node => entryOf(node)?.last_seen_at || entryOf(node)?.connection?.last_seen_at || node.last_check_at || null
const agentVersionOf = node => entryOf(node)?.agent_version || node.server_version || ''
// The anx-node release the node's live session reported.
const kernelVersionOf = node => entryOf(node)?.kernel_version || ''

// 版本 and 负载 start hidden, as does 连接 (the Agent's channel): a node that
// joined has a certificate and one channel, so 证书, Agent 版本 and 最后在线
// take its room at 1440 px. The node page has them all, and the table
// settings bring any of them back.
const columns = computed(() => [
  { key: 'id', label: t('admin.nodes.table.id'), numeric: true, hidden: true, width: 72 },
  { key: 'name', label: t('admin.nodes.table.name'), primary: true, hideable: false },
  { key: 'address', label: t('admin.nodes.table.address'), secondary: true, nowrap: true },
  { key: 'nodeGroup', label: t('admin.nodes.table.nodeGroup'), nowrap: true, value: node => node.node_group_name || '', hidden: true, card: false },
  { key: 'protocols', label: t('admin.nodes.table.protocols'), numeric: true, align: 'end', nowrap: true, value: node => node.protocols?.length || 0, card: false },
  { key: 'status', label: t('admin.nodes.table.status') },
  { key: 'connection', label: t('admin.nodes.table.connection'), nowrap: true, breakpoint: 'lg', hidden: true, card: false },
  { key: 'certificate', label: t('admin.nodes.table.certificate'), nowrap: true, breakpoint: 'lg', card: false },
  { key: 'agentVersion', label: t('admin.nodes.table.agentVersion'), nowrap: true, value: agentVersionOf, breakpoint: 'lg', card: false },
  { key: 'serviceStatus', label: t('admin.nodes.table.serviceStatus'), nowrap: true, breakpoint: 'lg', card: false },
  { key: 'load', label: t('admin.nodes.table.load'), nowrap: true, numeric: true, value: loadText, firstDirection: 'desc', breakpoint: 'lg', hidden: true, card: false },
  { key: 'lastSeen', label: t('admin.nodes.table.lastSeen'), nowrap: true, value: lastSeenOf, firstDirection: 'desc' },
  { key: 'parent', label: t('admin.nodes.table.parent'), value: node => parentName(node.parent_id), hidden: true },
  { key: 'traffic', label: t('admin.nodes.table.traffic'), numeric: true, align: 'end', nowrap: true, hidden: true, value: node => format.bytes(node.traffic_today || 0) },
  { key: 'monthlyQuota', label: t('admin.nodes.table.monthlyQuota'), hidden: true, card: false }
].map(column => (NODE_SORT.isSortable(column.key) ? { ...column, sortable: true } : column)))

const nodeActions = node => [
  ...(node.approval === 'pending'
    ? [
        { key: 'approve', label: t('admin.nodes.approval.approve'), icon: Check, onSelect: () => decide(node, 'approve') },
        { key: 'reject', label: t('admin.nodes.approval.reject'), icon: X, danger: true, onSelect: () => decide(node, 'reject') }
      ]
    : []),
  { key: 'open', label: t('admin.nodes.actions.open'), icon: SquareArrowOutUpRight, onSelect: () => openDetail(node), separatorBefore: node.approval === 'pending' },
  { key: 'protocols', label: t('admin.nodes.actions.manageProtocols'), icon: Layers, onSelect: () => openDetail(node, 'protocols') },
  { key: 'logs', label: t('admin.nodes.actions.logs'), icon: FileText, onSelect: () => openDetail(node, 'logs') },
  { key: 'sync', label: t('admin.nodes.actions.syncReload'), icon: RefreshCcw, disabled: syncingNodeIds.has(node.id), onSelect: () => syncNode(node) },
  { key: 'upgradeKernel', label: t('admin.nodes.actions.upgradeKernel'), icon: Cpu, onSelect: () => openKernelUpgrade(nodeRef(node.id)) },
  { key: 'edit', label: t('admin.nodes.actions.edit'), icon: Pencil, onSelect: () => openEdit(node) },
  { key: 'delete', label: t('admin.nodes.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteFromList(node) }
]

function listParams() {
  const params = { page: pagination.page, page_size: pagination.size }
  if (search.value.trim()) params.search = search.value.trim()
  if (statusFilter.value === AWAITING) params.approval = 'pending'
  else if (statusFilter.value) params.status = NODE_STATUS[statusFilter.value]
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
    const payload = readNodePage(await getKernelAdminNodes(listParams()))
    nodes.value = (payload.list || []).map(normalizeNode)
    pagination.total = payload.total || 0
    rememberNodeNames(nodes.value)
    // The Agent connection and the services summary of the nodes in view:
    // one call each, after the list.
    void loadTransports(nodes.value.map(node => node.id))
    void loadServiceStatus(nodes.value.map(node => node.id))
    return true
  } catch (e) {
    console.error('Failed to load nodes:', e)
    loadError.value = e
    nodes.value = []
    resetTransports()
    resetServiceStatus()
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

// A join, an approval or a group edit changed the list and the groups.
async function reloadAll() {
  await Promise.all([reload(), groupsState.load()])
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

// The banner's 查看: the nodes waiting for approval, filtered on the nodes
// tab.
async function showAwaiting() {
  if (tab.value !== 'nodes') await setTab('nodes')
  statusFilter.value = AWAITING
}

function openDetail(node, section) {
  rememberNodeNames(nodes.value)
  router.push({ path: `/admin/nodes/${node.id}`, query: section ? { section } : {} })
}

function openEdit(node) {
  editingNode.value = node
  formOpen.value = true
}

async function deleteFromList(node) {
  if (await confirmDelete(node)) await reloadAll()
}

// 批准 and 拒绝 on a row (POST /kernel/nodes/proxy-<id>/approve|reject).
async function decide(node, action) {
  const name = nodeRef(node.id)
  try {
    if (action === 'reject') {
      const confirmed = await confirm({
        title: t('admin.nodes.approval.rejectTitle', { name: node.name }),
        message: t('admin.nodes.approval.rejectMessage'),
        confirmLabel: t('admin.nodes.approval.reject'),
        tone: 'danger'
      })
      if (!confirmed) return
      await rejectKernelNode(name)
    } else {
      await approveKernelNode(name)
    }
    toast.success(t(action === 'approve' ? 'admin.nodes.approval.approvedToast' : 'admin.nodes.approval.rejectedToast', { name: node.name }))
    await reloadAll()
  } catch (e) {
    toast.error(t('admin.nodes.approval.failed', { message: readKernelError(e).message }))
  }
}

// 接入节点 from the header (the first group), or from a group's row (that
// group's command).
function openJoin(group = null) {
  joinGroupId.value = group?.id ?? null
  joinOpen.value = true
}

function openGroupCreate() {
  editingGroup.value = null
  groupFormOpen.value = true
}

function openGroupEdit(group) {
  editingGroup.value = group
  groupFormOpen.value = true
}

async function deleteGroup(group) {
  if (await groupsState.confirmDelete(group)) await groupsState.load()
}

async function toggleApproval(group, value) {
  await groupsState.toggleApproval(group, value)
}

// The command palette's 接入节点 opens this page with ?join=1.
const routeIntent = useRouteIntent(['join'], intent => {
  if (intent.join === '1') openJoin()
})

onMounted(async () => {
  if (routeIntent.join === '1') openJoin()
  syncQuery({ url: false })
  void groupsState.load()
  if (await loadNodes()) await loadStats()
})

onBeforeUnmount(() => clearTimeout(searchTimer))

defineExpose({ nodes, stats, pagination, loadNodes, loadStats, changeSort, groupsState })
</script>

<style scoped>
.nodes-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

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

.node-muted {
  color: var(--label-2);
}

.node-kernel-version {
  display: block;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}
</style>
