<template>
  <section class="fwd-screen">
    <UiPageHeader title="路由" description="每条路由是一条从入口到目标的跳链；节点按路由收到期望状态。">
      <template #meta>
        <UiBadge v-if="allRows.length" tone="neutral" :label="`${allRows.length} 条`" />
      </template>
      <template #actions>
        <UiMenu label="更多操作" :items="headerMenu" />
        <UiButton variant="primary" :icon="Plus" @click="router.push(`${BASE}/editor`)">新建路由</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      v-model:selected="selected"
      :columns="columns"
      :rows="rows"
      row-key="id"
      label="路由"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :filtered="filtered"
      :empty-icon="Route"
      empty-title="还没有路由"
      empty-description="路由把入口端口经过一个或多个节点转发到目标。先确认节点已加入转发清单，再新建第一条路由。"
      selectable
      activatable
      :card-fields="4"
      :page-size="50"
      @row-activate="row => router.push(`${BASE}/route?id=${row.id}`)"
      @clear-filters="clearFilters"
    >
      <template v-if="allRows.length" #toolbar>
        <div class="routes-toolbar">
          <UiSearchField v-model="search" class="routes-toolbar__search" label="搜索路由" placeholder="名称、端口、目标" />
          <UiFilterChips v-model="status" label="状态" :options="statusOptions" />
          <div class="routes-toolbar__selects">
            <UiSelect v-model="engine" size="md" aria-label="引擎" :options="engineOptions" />
            <UiSelect v-model="nodeRef" size="md" aria-label="节点" :options="nodeOptions" />
            <UiSelect v-model="label" size="md" aria-label="标签" :options="labelOptions" />
          </div>
        </div>
      </template>
      <template #empty-actions>
        <UiButton :icon="Server" @click="router.push(`${BASE}/nodes`)">查看节点</UiButton>
        <UiButton variant="primary" :icon="Plus" @click="router.push(`${BASE}/editor`)">新建路由</UiButton>
      </template>

      <template #cell-name="{ row }">
        <span class="fwd-cell-stack">
          <span class="routes-name">{{ row.name }}</span>
          <span class="routes-labels">
            <span v-for="text in row.labels" :key="text" class="routes-label">{{ text }}</span>
          </span>
        </span>
      </template>
      <template #cell-listen="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono routes-nowrap">{{ row.listenText }}</span>
          <span class="fwd-muted routes-hint" :title="row.entryHint">{{ row.entryHint }}</span>
        </span>
      </template>
      <template #cell-hops="{ row }">
        <HopChain :route="row.route" compact />
      </template>
      <template #cell-targets="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono">{{ row.targetText }}</span>
          <span v-if="row.route.targets.length > 1" class="fwd-muted">另 {{ row.route.targets.length - 1 }} 个</span>
        </span>
      </template>
      <template #cell-strategy="{ row }">
        <span class="fwd-cell-stack">
          <span>{{ row.strategyText }}</span>
          <span v-if="row.directText" class="fwd-muted">{{ row.directText }}</span>
        </span>
      </template>
      <template #cell-status="{ row }">
        <RouteStatus :item="row.item" />
      </template>
      <template #cell-traffic="{ row }">
        <span class="fwd-cell-stack routes-traffic">
          <span>{{ fmt.bytes(row.traffic, { precision: 1, empty: '—' }) }}</span>
          <span v-if="row.traffic" class="fwd-muted routes-nowrap">↑{{ fmt.bytes(row.up, { precision: 1 }) }} ↓{{ fmt.bytes(row.down, { precision: 1 }) }}</span>
        </span>
      </template>

      <template #bulk-actions="{ rows: chosen, clear }">
        <UiButton size="sm" :icon="Pause" @click="clear">暂停 {{ chosen.length }} 条</UiButton>
        <UiButton size="sm" :icon="Play" @click="clear">恢复</UiButton>
        <UiButton size="sm" :icon="Tag" @click="clear">添加标签</UiButton>
        <UiButton size="sm" variant="danger-soft" :icon="Trash2" @click="clear">删除…</UiButton>
      </template>
    </UiDataTable>

    <p v-if="allRows.length" class="list-page__note">状态、引擎和标签筛选只作用于已加载的路由（API 目前只按节点筛选）。流量是最近 24 小时入口节点的计量字节，未乘倍率。</p>
  </section>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Copy, Pause, Pencil, Play, Plus, Route, Server, Tag, Trash2, Download, Upload, Stethoscope } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiFilterChips, UiMenu, UiPageHeader, UiSearchField, UiSelect, useFormat } from '@/ui'
import HopChain from '../parts/HopChain.vue'
import RouteStatus from '../parts/RouteStatus.vue'
import { ROUTES, NODES, LABEL_OPTIONS, STRATEGIES, DIRECT_MODES, routeStatus, routeTraffic24h, nodeName, ENGINES } from '../mockData'

const BASE = '/admin/__mockups/forward'
const router = useRouter()
const currentRoute = useRoute()
const fmt = useFormat()
const empty = currentRoute.query.state === 'empty'

const search = ref('')
const status = ref('')
const engine = ref('all')
const nodeRef = ref('all')
const label = ref('all')
// ?select=1 shows the bulk bar with two rows selected.
const selected = ref(currentRoute.query.select ? ['01JB7Q8U9V0W1X2Y3Z4A5B6C7D', '01JB7Q5N6P7Q8R9S0T1U2V3W4X'] : [])

const PROTOCOL = { L4_PROTOCOL_TCP: 'TCP', L4_PROTOCOL_UDP: 'UDP', L4_PROTOCOL_TCP_UDP: 'TCP+UDP' }

const allRows = computed(() => (empty ? [] : ROUTES).map(item => {
  const { route } = item
  const traffic = routeTraffic24h(route.id)
  const entry = route.hops[0]
  const last = route.targets[0]
  return {
    id: route.id,
    item,
    route,
    name: route.name,
    labels: Object.entries(route.labels || {}).map(([key, value]) => `${key}=${value}`),
    listenText: `:${route.listen.port} ${PROTOCOL[route.listen.protocol]}`,
    entryHint: route.listen.entry_hostname
      ? `${route.listen.entry_hostname} · ${entry.node_refs.length} 个入口`
      : entry.node_refs.map(nodeName).join('、'),
    targetText: `${last.host}:${last.port}`,
    strategyText: STRATEGIES[route.policy?.target || 'BALANCE_STRATEGY_ROUND_ROBIN'].label,
    directText: route.policy?.direct && route.policy.direct !== 'DIRECT_MODE_OFF' ? DIRECT_MODES[route.policy.direct].label : '',
    status: routeStatus(item).key,
    traffic: traffic.up + traffic.down,
    up: traffic.up,
    down: traffic.down
  }
}))

const rows = computed(() => allRows.value.filter(row => {
  if (search.value && !`${row.name} ${row.listenText} ${row.targetText}`.includes(search.value)) return false
  if (status.value === 'enforced' && !['quota', 'expired'].includes(row.status)) return false
  if (status.value && status.value !== 'enforced' && row.status !== status.value) return false
  if (engine.value !== 'all' && !row.route.hops.some(hop => hop.engine === engine.value)) return false
  if (nodeRef.value !== 'all' && !row.route.hops.some(hop => hop.node_refs.includes(nodeRef.value))) return false
  if (label.value !== 'all' && !row.labels.includes(label.value)) return false
  return true
}))

const filtered = computed(() => Boolean(search.value || status.value || engine.value !== 'all' || nodeRef.value !== 'all' || label.value !== 'all'))

function clearFilters() {
  search.value = ''
  status.value = ''
  engine.value = 'all'
  nodeRef.value = 'all'
  label.value = 'all'
}

function count(key) {
  return allRows.value.filter(row => (key === 'enforced' ? ['quota', 'expired'].includes(row.status) : row.status === key)).length
}

const statusOptions = computed(() => [
  { value: 'healthy', label: '正常', count: count('healthy') },
  { value: 'degraded', label: '降级', count: count('degraded') },
  { value: 'error', label: '跳错误', count: count('error') },
  { value: 'paused', label: '已暂停', count: count('paused') },
  { value: 'enforced', label: '已强制暂停', count: count('enforced') }
])
const engineOptions = [{ value: 'all', label: '全部引擎' }, ...['ENGINE_NFTABLES', 'ENGINE_GOST'].map(value => ({ value, label: ENGINES[value].label }))]
const nodeOptions = [{ value: 'all', label: '全部节点' }, ...NODES.filter(node => node.enabled).map(node => ({ value: node.node_ref, label: node.name, description: node.node_ref }))]
const labelOptions = [{ value: 'all', label: '全部标签' }, ...LABEL_OPTIONS.map(value => ({ value, label: value }))]

const columns = [
  { key: 'name', label: '名称', primary: true, sortable: true, minWidth: '160px' },
  { key: 'listen', label: '入口监听', secondary: true, sortValue: row => row.route.listen.port, sortable: true },
  { key: 'hops', label: '跳链', minWidth: '220px' },
  { key: 'targets', label: '目标', breakpoint: 'lg', maxWidth: '200px', card: false },
  { key: 'strategy', label: '目标策略', breakpoint: 'lg', card: false },
  { key: 'status', label: '状态', sortable: true, sortValue: row => row.status },
  { key: 'traffic', label: '24 小时流量', align: 'end', numeric: true, sortable: true, firstDirection: 'desc', sortValue: row => row.traffic }
]

function rowActions(row) {
  const paused = row.route.paused
  const enforced = ['quota', 'expired'].includes(row.status)
  return [
    { key: 'edit', label: '编辑', icon: Pencil, onSelect: () => router.push(`${BASE}/editor?id=${row.id}`) },
    enforced
      ? { key: 'limits', label: row.status === 'quota' ? '提高配额…' : '延长到期…', icon: Pencil, onSelect: () => router.push(`${BASE}/editor?id=${row.id}#limits`) }
      : { key: 'pause', label: paused ? '恢复' : '暂停', icon: paused ? Play : Pause, onSelect: () => {} },
    { key: 'diagnose', label: '诊断', icon: Stethoscope, onSelect: () => {} },
    { key: 'duplicate', label: '复制为新路由', icon: Copy, onSelect: () => router.push(`${BASE}/editor`) },
    { key: 'delete', label: '删除…', icon: Trash2, danger: true, separatorBefore: true, onSelect: () => {} }
  ]
}

const headerMenu = [
  { key: 'import', label: '导入路由 JSON', icon: Upload, onSelect: () => {} },
  { key: 'export', label: '导出所选', icon: Download, onSelect: () => {} }
]
</script>

<style scoped>
.routes-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  width: 100%;
}

.routes-toolbar__search {
  flex: 1 1 220px;
  max-width: 320px;
}

.routes-toolbar__selects {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.routes-toolbar__selects > * {
  min-width: 132px;
}

.routes-name {
  font-weight: var(--weight-medium);
}

.routes-labels {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.routes-label {
  white-space: nowrap;
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.routes-traffic {
  justify-items: end;
}

.routes-nowrap {
  white-space: nowrap;
}

.routes-hint {
  overflow: hidden;
  max-width: 160px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 639.98px) {
  .routes-toolbar__search {
    max-width: none;
  }

  .routes-toolbar__selects > * {
    flex: 1 1 40%;
    min-width: 0;
  }

  .routes-traffic {
    justify-items: start;
  }
}
</style>
