<template>
  <section class="fwd-screen">
    <UiPageHeader title="转发概览" description="路由、节点和最近 24 小时的流量。">
      <template #actions>
        <span class="fwd-muted">更新于 {{ fmt.relativeTime(MOCK_NOW - 12_000, { now: MOCK_NOW }) }}</span>
        <UiButton variant="primary" :icon="Plus" @click="router.push(`${BASE}/editor`)">新建路由</UiButton>
      </template>
    </UiPageHeader>

    <div class="fwd-grid">
      <UiMetricCard label="路由" :value="ROUTES.length" :icon="RouteIcon" :detail="`${running} 运行 · ${paused} 暂停 · ${enforced} 强制暂停`" />
      <UiMetricCard label="在线节点" :value="`${online} / ${NODES.length}`" :icon="Server" :detail="`${lagging.length} 个同步落后`" />
      <UiMetricCard label="24 小时流量" :value="fmt.bytes(total, { precision: 1 })" :icon="Activity" trend="比前 24 小时多 8%" trend-direction="up" :sparkline="spark" />
      <UiMetricCard label="待处理" :value="attention.length" :icon="AlertTriangle" detail="跳错误、熔断、落后和强制暂停" trend-tone="negative" />
    </div>

    <UiCard title="流量">
      <template #actions>
        <UiSegmentedControl v-model="range" size="sm" aria-label="时间范围" :options="ranges" />
      </template>
      <UiChart :option="chartOption" label="全部路由最近 24 小时每小时流量" :summary="`共 ${fmt.bytes(total, { precision: 1 })}`" :height="240" />
    </UiCard>

    <div class="fwd-two">
      <UiCard title="流量排行" description="最近 24 小时，按入口计量字节。">
        <ol class="fwd-list">
          <li v-for="(row, index) in top" :key="row.id" class="fwd-list__item overview-top">
            <span class="overview-top__rank">{{ index + 1 }}</span>
            <span class="fwd-cell-stack overview-top__main">
              <RouterLink class="overview-link" :to="`${BASE}/route?id=${row.id}`">{{ row.name }}</RouterLink>
              <UiUsageBar :value="row.total" :max="top[0].total" :text="fmt.bytes(row.total, { precision: 1 })" :warn-at="101" :danger-at="101" />
            </span>
          </li>
        </ol>
      </UiCard>

      <UiCard title="需要处理">
        <ul class="fwd-list">
          <li v-for="entry in attention" :key="entry.key" class="fwd-list__item">
            <span class="fwd-cell-stack">
              <span class="overview-att"><UiBadge :tone="entry.tone" :label="entry.kind" /> {{ entry.title }}</span>
              <span class="fwd-muted">{{ entry.detail }}</span>
            </span>
            <UiButton size="sm" @click="router.push(entry.to)">{{ entry.action }}</UiButton>
          </li>
        </ul>
      </UiCard>
    </div>
  </section>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Activity, AlertTriangle, Plus, Route as RouteIcon, Server } from '@lucide/vue'
import { UiBadge, UiButton, UiCard, UiChart, UiMetricCard, UiPageHeader, UiSegmentedControl, UiUsageBar, useFormat } from '@/ui'
import { HOP_ERRORS, MOCK_NOW, NODES, ROUTES, routeById, routeStatus, routeTraffic24h, totalSeries } from '../mockData'

const BASE = '/admin/__mockups/forward'
const router = useRouter()
const fmt = useFormat()
const range = ref('24h')
const ranges = [{ value: '24h', label: '24 小时' }, { value: '7d', label: '7 天' }, { value: '30d', label: '30 天' }]

const statuses = ROUTES.map(item => routeStatus(item).key)
const paused = statuses.filter(key => key === 'paused').length
const enforced = statuses.filter(key => key === 'quota' || key === 'expired').length
const running = ROUTES.length - paused - enforced
const online = NODES.filter(node => node.enabled && node._online).length
const lagging = NODES.filter(node => node.reported && node.reported_generation !== node.desired_generation)

const series = totalSeries()
const total = series.reduce((sum, bucket) => sum + Number(bucket.up_bytes) + Number(bucket.down_bytes), 0)
const spark = series.map(bucket => Number(bucket.down_bytes))
const top = ROUTES.map(({ route }) => {
  const traffic = routeTraffic24h(route.id)
  return { id: route.id, name: route.name, total: traffic.up + traffic.down }
}).filter(row => row.total).sort((a, b) => b.total - a.total).slice(0, 5)

const attention = [
  ...Object.entries(HOP_ERRORS).flatMap(([ref, errors]) => errors.slice(0, 1).map(error => ({
    key: `err-${ref}`,
    kind: '跳错误',
    tone: 'danger',
    title: `${NODES.find(node => node.node_ref === ref).name} · ${routeById(error.route_id).route.name}`,
    detail: error.message,
    action: '查看节点',
    to: `${BASE}/node?ref=${ref}`
  }))),
  { key: 'breaker', kind: '熔断', tone: 'warning', title: 'game-hk-tyo · 第 2 跳', detail: 'tyo-exit-02 连续失败 3 次，18 秒后试探', action: '查看路由', to: `${BASE}/route` },
  ...lagging.map(node => ({ key: `lag-${node.node_ref}`, kind: '落后', tone: 'info', title: node.name, detail: `已应用第 ${node.reported_generation} 代，期望第 ${node.desired_generation} 代（${fmt.relativeTime(Number(node.reported_at_unix_ms), { now: MOCK_NOW })}上报）`, action: '查看节点', to: `${BASE}/node?ref=${node.node_ref}` })),
  { key: 'quota', kind: '强制暂停', tone: 'warning', title: 'reseller-a', detail: '配额 500 GB 已用尽；提高配额后自动恢复', action: '调整配额', to: `${BASE}/editor?id=01JB7Q6Y7Z8A9B0C1D2E3F4G5H` }
]

const hours = series.map(bucket => `${String((new Date(Number(bucket.hour_start_unix_ms)).getUTCHours() + 8) % 24).padStart(2, '0')}:00`)
const gb = value => fmt.bytes(value, { precision: 1 })
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
</script>

<style scoped>
.overview-top {
  align-items: center;
  justify-content: flex-start;
}

.overview-top__rank {
  width: 20px;
  color: var(--label-3);
  font-variant-numeric: tabular-nums;
}

.overview-top__main {
  flex: 1;
}

.overview-link {
  color: var(--accent);
  text-decoration: none;
}

.overview-att {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}
</style>
