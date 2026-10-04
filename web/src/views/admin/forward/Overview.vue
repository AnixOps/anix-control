<template>
  <section class="fwd-page" data-testid="forward-overview">
    <ForwardAreaNav area="overview" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('forwardV4.overview.title')" :description="t('forwardV4.overview.description')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" @click="router.push('/admin/forward/routes/new')">{{ t('forwardV4.routes.new') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiErrorState v-if="loadError && !snapshot" :title="t('forwardV4.overview.loadFailed')" :error="loadError" @retry="refresh" />

    <template v-else>
      <div class="fwd-grid">
        <template v-if="snapshot">
          <UiMetricCard :label="t('forwardV4.overview.routes')" :value="routeCount" :icon="RouteIcon" :detail="t('forwardV4.overview.routesDetail', counts)" />
          <UiMetricCard :label="t('forwardV4.overview.nodes')" :value="`${onlineNodes} / ${nodeTotal}`" :icon="Server" :detail="t('forwardV4.overview.nodesDetail', { n: laggingNodes.length })" />
          <UiMetricCard :label="t('forwardV4.overview.traffic')" :value="fmt.bytes(totalTraffic, { precision: 1 })" :icon="Activity" :sparkline="spark" :detail="t('forwardV4.overview.trafficDetail')" />
          <UiMetricCard :label="t('forwardV4.overview.attention')" :value="attention.length" :icon="AlertTriangle" :detail="t('forwardV4.overview.attentionDetail')" />
        </template>
        <template v-else>
          <UiSkeleton v-for="index in 4" :key="index" variant="card" />
        </template>
      </div>

      <UiCard :title="t('forwardV4.overview.trafficTitle')" :description="t('forwardV4.overview.trafficDescription')">
        <UiChart
          :option="chartOption"
          :label="t('forwardV4.overview.chartLabel')"
          :summary="t('forwardV4.overview.chartSummary', { total: fmt.bytes(totalTraffic, { precision: 1 }) })"
          :table="chartTable"
          :height="240"
          :loading="!snapshot"
          :empty="Boolean(snapshot) && !totalTraffic"
          :empty-title="t('forwardV4.overview.noTraffic')"
        />
        <p v-if="snapshot?.statsTruncated" class="fwd-note is-warning overview-mt" role="status">{{ t('forwardV4.routes.statsTruncated') }}</p>
      </UiCard>

      <div class="fwd-two">
        <UiCard :title="t('forwardV4.overview.top')" :description="t('forwardV4.overview.topDescription')">
          <p v-if="!top.length" class="fwd-muted">{{ t('forwardV4.overview.noTraffic') }}</p>
          <ol v-else class="fwd-list">
            <li v-for="(row, index) in top" :key="row.id" class="fwd-list__item overview-top">
              <span class="overview-top__rank">{{ index + 1 }}</span>
              <span class="fwd-cell-stack overview-top__main">
                <RouterLink class="fwd-link" :to="`/admin/forward/routes/${row.id}`">{{ row.name }}</RouterLink>
                <UiUsageBar :value="row.total" :max="top[0].total" :text="fmt.bytes(row.total, { precision: 1 })" :warn-at="101" :danger-at="101" />
              </span>
            </li>
          </ol>
        </UiCard>

        <UiCard :title="t('forwardV4.overview.attentionTitle')">
          <p v-if="!attention.length" class="fwd-muted">{{ t('forwardV4.overview.allClear') }}</p>
          <ul v-else class="fwd-list">
            <li v-for="entry in attention" :key="entry.key" class="fwd-list__item">
              <span class="fwd-cell-stack">
                <span class="overview-att"><UiBadge :tone="entry.tone" :label="entry.kind" /> {{ entry.title }}</span>
                <span class="fwd-muted overview-att__detail">{{ entry.detail }}</span>
              </span>
              <UiButton size="sm" @click="router.push(entry.to)">{{ entry.action }}</UiButton>
            </li>
          </ul>
        </UiCard>
      </div>

      <p v-if="isCommercial" class="list-page__note" data-testid="forward-commercial-link">
        {{ t('forwardV4.overview.commercialNote') }}
        <RouterLink class="fwd-link" to="/admin/plans">{{ t('forwardV4.overview.commercialLink') }}</RouterLink>
      </p>
    </template>
  </section>
</template>

<script setup>
// 概览 (F5b): route and node counts, 24 h entry traffic, the top routes and
// what needs attention (hop errors, open breakers, lagging nodes and
// enforced routes). Polls every 30 s while visible (D5).
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Activity, AlertTriangle, Plus, Route as RouteIcon, Server } from '@lucide/vue'
import { UiBadge, UiButton, UiCard, UiChart, UiErrorState, UiMetricCard, UiPageHeader, UiSkeleton, UiUsageBar, useFormat } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import { loadForwardSnapshot } from '@/components/forward/forwardData'
import { forwardErrorMessage } from '@/components/forward/messages'
import { usePolling } from '@/components/forward/usePolling'
import { hourlySeries, isEnforced, nodeLag, nodeLagging, nodeOnline, num } from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const router = useRouter()
const { t } = useAppI18n()
const fmt = useFormat()
const { isCommercial } = useEdition()

const snapshot = ref(null)
const loadError = ref('')
const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    snapshot.value = await loadForwardSnapshot()
    loadError.value = ''
  } catch (error) {
    loadError.value = forwardErrorMessage(t, error)
    throw error
  }
}, { interval: 30_000 })

const routes = computed(() => snapshot.value?.routes || [])
const nodes = computed(() => snapshot.value?.nodes || [])
const routeCount = computed(() => routes.value.length)
const counts = computed(() => {
  const paused = routes.value.filter(item => item.status === 'paused').length
  const enforced = routes.value.filter(item => isEnforced(item.enforced)).length
  return { running: routeCount.value - paused - enforced, paused, enforced }
})
const nodeTotal = computed(() => nodes.value.length)
const onlineNodes = computed(() => nodes.value.filter(node => nodeOnline(node)).length)
const laggingNodes = computed(() => nodes.value.filter(node => node.reported && nodeLagging(node)))

const series = computed(() => (snapshot.value
  ? hourlySeries(snapshot.value.series, snapshot.value.window)
  : []))
const totalTraffic = computed(() => series.value.reduce((sum, point) => sum + point.up + point.down, 0))
const spark = computed(() => series.value.map(point => point.down))

const routeName = id => routes.value.find(item => item.route.id === id)?.route.name || id
const nodeName = ref => nodes.value.find(node => node.node_ref === ref)?.name || ref

const top = computed(() => routes.value
  .map(item => {
    const traffic = snapshot.value.traffic.get(item.route.id) || { up: 0, down: 0 }
    return { id: item.route.id, name: item.route.name, total: traffic.up + traffic.down }
  })
  .filter(row => row.total > 0)
  .sort((a, b) => b.total - a.total)
  .slice(0, 5))

const attention = computed(() => {
  if (!snapshot.value) return []
  const out = []
  const seen = new Set()
  for (const error of snapshot.value.hopErrors) {
    const key = `err-${error.node_ref}-${error.route_id}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push({
      key,
      kind: t('forwardV4.status.error'),
      tone: 'danger',
      title: `${nodeName(error.node_ref)} · ${routeName(error.route_id)}`,
      detail: error.message,
      action: t('forwardV4.overview.viewNode'),
      to: `/admin/forward/inventory/${error.node_ref}`
    })
  }
  for (const target of snapshot.value.targets) {
    if (target.state !== 'HEALTH_STATE_CIRCUIT_OPEN') continue
    out.push({
      key: `breaker-${target.route_id}-${target.key}`,
      kind: t('forwardV4.health.HEALTH_STATE_CIRCUIT_OPEN'),
      tone: 'warning',
      title: `${target.route_name || routeName(target.route_id)} · ${target.key}`,
      detail: t('forwardV4.overview.breakerDetail', { healthy: num(target.healthy), reports: num(target.reports) }),
      action: t('forwardV4.overview.viewRoute'),
      to: `/admin/forward/routes/${target.route_id}`
    })
  }
  for (const node of laggingNodes.value) {
    out.push({
      key: `lag-${node.node_ref}`,
      kind: t('forwardV4.overview.lagging'),
      tone: 'info',
      title: node.name || node.node_ref,
      detail: t('forwardV4.overview.lagDetail', {
        applied: num(node.reported_generation),
        desired: num(node.desired_generation),
        n: nodeLag(node),
        when: fmt.relativeTime(num(node.reported_at_unix_ms))
      }),
      action: t('forwardV4.overview.viewNode'),
      to: `/admin/forward/inventory/${node.node_ref}`
    })
  }
  for (const item of routes.value.filter(entry => isEnforced(entry.enforced))) {
    out.push({
      key: `enf-${item.route.id}`,
      kind: t('forwardV4.overview.enforced'),
      tone: 'warning',
      title: item.route.name,
      detail: t(`forwardV4.status.${item.enforced}Hint`),
      action: t(item.enforced === 'quota' ? 'forwardV4.actions.raiseQuota' : 'forwardV4.actions.extendExpiry'),
      to: `/admin/forward/routes/${item.route.id}/edit#limits`
    })
  }
  return out
})

function hourLabel(ms) {
  const date = new Date(ms)
  return `${String(date.getHours()).padStart(2, '0')}:00`
}
const bytes = value => fmt.bytes(value, { precision: 1 })
const chartOption = computed(() => ({
  tooltip: { trigger: 'axis', valueFormatter: bytes },
  legend: { top: 0 },
  grid: { left: 8, right: 16, top: 36, bottom: 8, containLabel: true },
  xAxis: { type: 'category', data: series.value.map(point => hourLabel(point.hour)), boundaryGap: false },
  yAxis: { type: 'value', axisLabel: { formatter: bytes } },
  series: [
    { name: t('forwardV4.traffic.down'), type: 'line', smooth: true, showSymbol: false, areaStyle: { opacity: 0.12 }, data: series.value.map(point => point.down) },
    { name: t('forwardV4.traffic.up'), type: 'line', smooth: true, showSymbol: false, data: series.value.map(point => point.up) }
  ]
}))
const chartTable = computed(() => ({
  columns: [
    { key: 'hour', label: t('forwardV4.traffic.hour') },
    { key: 'down', label: t('forwardV4.traffic.down'), numeric: true, format: bytes },
    { key: 'up', label: t('forwardV4.traffic.up'), numeric: true, format: bytes }
  ],
  rows: series.value.map(point => ({ hour: hourLabel(point.hour), down: point.down, up: point.up }))
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

.overview-att {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.overview-att__detail {
  overflow-wrap: anywhere;
}

.overview-mt {
  margin-top: var(--space-3);
}
</style>
