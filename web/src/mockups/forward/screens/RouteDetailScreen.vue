<template>
  <section class="fwd-screen">
    <UiPageHeader :title="route.name">
      <template #back>
        <RouterLink class="detail-back" :to="`${BASE}/routes`"><ChevronLeft :size="16" /> 路由</RouterLink>
      </template>
      <template #meta>
        <RouteStatus :item="item" hint />
        <span class="fwd-mono detail-id">{{ route.id }} · 修订 {{ route.revision }}</span>
      </template>
      <template #description>
        <HopChain :route="route" />
      </template>
      <template #actions>
        <UiMenu label="更多操作" :items="menu" />
        <UiButton :icon="Stethoscope">诊断</UiButton>
        <UiButton :icon="route.paused ? Play : Pause">{{ route.paused ? '恢复' : '暂停' }}</UiButton>
        <UiButton variant="primary" :icon="Pencil" @click="router.push(`${BASE}/editor?id=${route.id}`)">编辑</UiButton>
      </template>
    </UiPageHeader>

    <p v-if="status.key === 'degraded'" class="fwd-note is-warning" role="status">
      第 2 跳有 1 个上游熔断（tyo-exit-02，18 秒后半开试探），第 3 跳有 1 个目标不健康；流量已切到其余上游。
    </p>

    <div class="fwd-grid">
      <UiMetricCard label="24 小时下行" :value="fmt.bytes(traffic.down, { precision: 1 })" :icon="ArrowDownToLine" :sparkline="downSpark" />
      <UiMetricCard label="24 小时上行" :value="fmt.bytes(traffic.up, { precision: 1 })" :icon="ArrowUpFromLine" />
      <UiMetricCard label="活跃连接" value="1,284" :icon="Cable" detail="上限 4,000 / 每个入口节点" />
      <UiMetricCard label="入口延迟" value="31 ms" :icon="Timer" detail="入口 → 中转 TCP 连接耗时" />
    </div>

    <UiCard title="链路与健康" description="每个节点只根据自己的检查切换上游；Control 离线时故障转移照常工作。">
      <ol class="chain">
        <li v-for="(hop, index) in route.hops" :key="index" class="chain__hop">
          <div class="chain__head">
            <span class="chain__role">{{ ROLE[hop.role] }}</span>
            <EngineChip :engine="hop.engine" />
            <span v-if="index > 0" class="fwd-muted">接入 {{ linkText(hop) }}</span>
          </div>
          <ul class="chain__nodes">
            <li v-for="ref in hop.node_refs" :key="ref" class="chain__node">
              <div class="chain__node-head">
                <UiStatusDot :tone="nodeTone(ref)" :label="nodeByRef(ref).name" />
                <span class="fwd-mono">:{{ portOf(index, ref) }}</span>
              </div>
              <span class="fwd-muted">代 {{ nodeByRef(ref).reported_generation || '—' }} / {{ nodeByRef(ref).desired_generation }}<template v-if="lagging(ref)"> · 同步中</template></span>
              <ul class="chain__ups">
                <li v-for="(up, upIndex) in upstreamsOf(index, ref)" :key="upIndex" class="chain__up">
                  <UiBadge :tone="HEALTH_TONE[up.state]" :label="HEALTH_TEXT[up.state]" />
                  <span class="fwd-mono">{{ up.label }}</span>
                  <span class="fwd-muted">{{ up.detail }}</span>
                </li>
              </ul>
            </li>
          </ul>
          <p class="fwd-muted chain__policy">
            {{ index === route.hops.length - 1 ? `目标策略：${STRATEGIES[route.policy.target].label}` : `下一跳策略：${STRATEGIES[route.policy.next_hop].label}` }}
          </p>
        </li>
        <li class="chain__hop chain__hop--targets">
          <div class="chain__head"><span class="chain__role">目标</span></div>
          <ul class="chain__nodes">
            <li v-for="target in route.targets" :key="target.host" class="chain__node">
              <span class="fwd-mono">{{ target.host }}:{{ target.port }}</span>
              <span class="fwd-muted">权重 {{ target.weight || 1 }} · 优先级 {{ target.priority || 0 }}</span>
            </li>
          </ul>
        </li>
      </ol>
    </UiCard>

    <UiCard title="流量">
      <template #actions>
        <UiSegmentedControl v-model="range" size="sm" aria-label="时间范围" :options="ranges" />
      </template>
      <UiChart :option="chartOption" label="最近 24 小时每小时流量" :summary="`下行 ${fmt.bytes(traffic.down, { precision: 1 })}，上行 ${fmt.bytes(traffic.up, { precision: 1 })}`" :table="chartTable" :height="260" />
    </UiCard>

    <UiCard title="上游健康" description="来自每个节点最新的上报（GET /routes/{id}/health）。熔断：连续失败 3 次打开，30 秒后放一次试探（H21 默认值）。">
      <UiDataTable :columns="healthColumns" :rows="healthRows" label="上游健康" flat :settings="false" :sticky-header="false" row-key="key">
        <template #cell-state="{ row }">
          <UiBadge :tone="HEALTH_TONE[row.state]" :label="HEALTH_TEXT[row.state]" />
        </template>
        <template #cell-upstream="{ row }"><span class="fwd-mono">{{ row.upstream }}</span></template>
      </UiDataTable>
    </UiCard>

    <div class="fwd-two">
      <UiCard title="节点状态" description="期望代与节点已应用的代；一致即已收敛。">
        <ul class="fwd-list">
          <li v-for="ref in nodeRefs" :key="ref" class="fwd-list__item">
            <span class="fwd-cell-stack">
              <RouterLink class="detail-link" :to="`${BASE}/node?ref=${ref}`">{{ nodeByRef(ref).name }}</RouterLink>
              <span class="fwd-muted">{{ ref }}</span>
            </span>
            <span class="fwd-cell-stack detail-end">
              <UiBadge :tone="lagging(ref) ? 'info' : 'success'" :label="lagging(ref) ? `落后 ${nodeByRef(ref).desired_generation - nodeByRef(ref).reported_generation} 代` : '已收敛'" />
              <span class="fwd-muted">上报于 {{ fmt.relativeTime(Number(nodeByRef(ref).reported_at_unix_ms), { now: MOCK_NOW }) }}</span>
            </span>
          </li>
        </ul>
      </UiCard>
      <UiCard title="配置">
        <UiGroupedList>
          <UiGroupedListRow label="入口监听" :value="`${route.listen.address}:${route.listen.port} ${route.listen.protocol.replace('L4_PROTOCOL_', '')}`" />
          <UiGroupedListRow label="直连模式" :value="DIRECT_MODES[route.policy.direct || 'DIRECT_MODE_OFF'].label" />
          <UiGroupedListRow label="健康检查" :value="`每 ${defaults.interval_ms / 1000} 秒 · 超时 ${defaults.timeout_ms / 1000} 秒（默认）`" />
          <UiGroupedListRow label="熔断" :value="`${defaults.failure_threshold} 次失败 · ${defaults.open_ms / 1000} 秒（默认）`" />
          <UiGroupedListRow label="带宽" :value="route.limits?.bandwidth_bps ? `${Number(route.limits.bandwidth_bps) / 1e6} Mbps / 每个入口节点` : '不限'" />
          <UiGroupedListRow label="最大连接" :value="route.limits?.max_conns ? `${route.limits.max_conns} / 每个入口节点` : '不限'" />
          <UiGroupedListRow label="配额" :value="route.limits?.quota_bytes ? fmt.bytes(route.limits.quota_bytes) : '不限'" />
          <UiGroupedListRow label="标签" :value="Object.entries(route.labels).map(([k, v]) => `${k}=${v}`).join('  ')" />
          <UiGroupedListRow label="更新于" :value="fmt.dateTime(Number(route.updated_at_unix_ms))" />
        </UiGroupedList>
      </UiCard>
    </div>
  </section>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowDownToLine, ArrowUpFromLine, Cable, ChevronLeft, Copy, Pause, Pencil, Play, Stethoscope, Timer, Trash2, FileJson } from '@lucide/vue'
import { UiBadge, UiButton, UiCard, UiChart, UiDataTable, UiGroupedList, UiGroupedListRow, UiMenu, UiMetricCard, UiPageHeader, UiSegmentedControl, UiStatusDot, useFormat } from '@/ui'
import EngineChip from '../parts/EngineChip.vue'
import HopChain from '../parts/HopChain.vue'
import RouteStatus from '../parts/RouteStatus.vue'
import { DIRECT_MODES, HEALTH, MOCK_NOW, ROUTES, SECURITIES, STRATEGIES, nodeByRef, routeById, routeSeries, routeStatus, routeTraffic24h } from '../mockData'
import { healthDefaults } from '../mockPlanner'

const BASE = '/admin/__mockups/forward'
const router = useRouter()
const fmt = useFormat()
const query = useRoute().query
const item = routeById(query.id) || ROUTES[0]
const route = item.route
const status = computed(() => routeStatus(item))
const traffic = routeTraffic24h(route.id)
const series = routeSeries(route.id)
const defaults = healthDefaults(route.policy)
const range = ref('24h')
const ranges = [{ value: '24h', label: '24 小时' }, { value: '7d', label: '7 天' }, { value: '30d', label: '30 天' }]

const ROLE = { HOP_ROLE_ENTRY: '入口', HOP_ROLE_RELAY: '中转', HOP_ROLE_EXIT: '出口' }
const HEALTH_TONE = { HEALTH_STATE_HEALTHY: 'success', HEALTH_STATE_UNHEALTHY: 'danger', HEALTH_STATE_CIRCUIT_OPEN: 'warning' }
const HEALTH_TEXT = { HEALTH_STATE_HEALTHY: '健康', HEALTH_STATE_UNHEALTHY: '不健康', HEALTH_STATE_CIRCUIT_OPEN: '熔断' }

const nodeRefs = Array.from(new Set(route.hops.flatMap(hop => hop.node_refs)))
const health = HEALTH[route.id] || []

function linkText(hop) {
  const security = SECURITIES[hop.ingress?.security || 'LINK_SECURITY_RAW']
  return hop.ingress?.mux ? `${security} · 多路复用` : security
}
function lagging(ref) {
  const node = nodeByRef(ref)
  return node.reported_generation !== node.desired_generation
}
function nodeTone(ref) {
  const node = nodeByRef(ref)
  if (node.hop_errors) return 'danger'
  if (lagging(ref)) return 'info'
  return 'success'
}
// The allocated port per (hop, node); the real page reads the node state.
function portOf(index, ref) {
  return index === 0 ? route.listen.port : 30020 + route.hops[index].node_refs.indexOf(ref)
}
function upstreamsOf(index, ref) {
  const rows = health.filter(up => up.hop_index === index && up._node === ref)
  if (rows.length) return rows.map(describe)
  const next = route.hops[index + 1]
  const list = next ? next.node_refs.map(nextRef => ({ address: nodeByRef(nextRef).host, port: 30020 })) : route.targets.map(target => ({ address: target.host, port: target.port }))
  return list.map(up => describe({ ...up, state: 'HEALTH_STATE_HEALTHY', rtt_us: 24_000, consecutive_failures: 0 }))
}
function describe(up) {
  const name = nodeNameByHost(up.address)
  let detail = up.rtt_us ? `${Math.round(up.rtt_us / 1000)} ms` : ''
  if (up.state === 'HEALTH_STATE_CIRCUIT_OPEN') detail = `${Math.round((Number(up.circuit_open_until_unix_ms) - MOCK_NOW) / 1000)} 秒后试探 · 不在轮转`
  if (up.state === 'HEALTH_STATE_UNHEALTHY') detail = `连续失败 ${up.consecutive_failures} 次`
  return { state: up.state, label: name ? `${name} :${up.port}` : `${up.address}:${up.port}`, detail }
}
function nodeNameByHost(host) {
  return nodeRefs.map(nodeByRef).find(node => node.host === host)?.name
}

const healthRows = computed(() => health.map((up, index) => ({
  key: index,
  hop: `第 ${up.hop_index + 1} 跳`,
  node: nodeByRef(up._node).name,
  upstream: `${nodeNameByHost(up.address) || up.address}:${up.port}`,
  state: up.state,
  failures: up.consecutive_failures,
  rtt: up.rtt_us ? `${(up.rtt_us / 1000).toFixed(1)} ms` : '—',
  rotation: up.state === 'HEALTH_STATE_CIRCUIT_OPEN' ? '否（熔断中）' : (up.state === 'HEALTH_STATE_UNHEALTHY' ? '否' : '是'),
  checked: fmt.relativeTime(Number(up.checked_at_unix_ms), { now: MOCK_NOW })
})))
const healthColumns = [
  { key: 'hop', label: '跳', primary: true },
  { key: 'node', label: '节点', secondary: true },
  { key: 'upstream', label: '上游' },
  { key: 'state', label: '状态' },
  { key: 'rotation', label: '在轮转' },
  { key: 'failures', label: '连续失败', align: 'end', numeric: true, breakpoint: 'md' },
  { key: 'rtt', label: '延迟', align: 'end', numeric: true },
  { key: 'checked', label: '检查于', breakpoint: 'lg' }
]

const hours = series.map(bucket => {
  const d = new Date(Number(bucket.hour_start_unix_ms))
  return `${String((d.getUTCHours() + 8) % 24).padStart(2, '0')}:00`
})
const gb = value => fmt.bytes(value, { precision: 1 })
const downSpark = series.map(bucket => Number(bucket.down_bytes))
const chartOption = computed(() => ({
  tooltip: { trigger: 'axis', valueFormatter: gb },
  legend: { top: 0 },
  grid: { left: 8, right: 16, top: 36, bottom: 8, containLabel: true },
  xAxis: { type: 'category', data: hours, boundaryGap: false },
  yAxis: { type: 'value', axisLabel: { formatter: gb } },
  series: [
    { name: '下行', type: 'line', smooth: true, showSymbol: false, areaStyle: { opacity: 0.12 }, data: series.map(bucket => Number(bucket.down_bytes)) },
    { name: '上行', type: 'line', smooth: true, showSymbol: false, data: series.map(bucket => Number(bucket.up_bytes)) }
  ]
}))
const chartTable = {
  columns: [{ key: 'hour', label: '时间' }, { key: 'down', label: '下行', numeric: true, format: gb }, { key: 'up', label: '上行', numeric: true, format: gb }],
  rows: series.map((bucket, index) => ({ hour: hours[index], down: Number(bucket.down_bytes), up: Number(bucket.up_bytes) }))
}

const menu = [
  { key: 'duplicate', label: '复制为新路由', icon: Copy, onSelect: () => {} },
  { key: 'json', label: '查看 JSON', icon: FileJson, onSelect: () => {} },
  { key: 'delete', label: '删除路由…', icon: Trash2, danger: true, separatorBefore: true, onSelect: () => {} }
]
</script>

<style scoped>
.detail-back,
.detail-link {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  color: var(--accent);
  text-decoration: none;
}

.detail-id {
  color: var(--label-3);
}

.detail-end {
  justify-items: end;
}

.chain {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}

.chain__hop {
  position: relative;
  display: grid;
  align-content: start;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.chain__hop--targets {
  border-style: dashed;
}

.chain__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.chain__role {
  font-weight: var(--weight-semibold);
}

.chain__nodes,
.chain__ups {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.chain__node {
  display: grid;
  gap: var(--space-1);
  padding: var(--space-2);
  border-radius: var(--radius-xs);
  background: var(--bg-elevated);
}

.chain__node-head {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
}

.chain__up {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
}

.chain__policy {
  margin: 0;
}
</style>
