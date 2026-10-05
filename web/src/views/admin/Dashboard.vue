<template>
  <div class="dashboard-page" :aria-busy="loading ? 'true' : undefined">
    <UiPageHeader :title="t('adminDashboard.title')" :description="t('adminDashboard.subtitle')">
      <template #actions>
        <span v-if="stats.cached_at" class="dashboard-page__updated">
          {{ t('adminDashboard.updatedAt', { time: format.relativeTime(stats.cached_at) }) }}
        </span>
        <UiButton :icon="RotateCw" :loading="loading" data-dashboard-refresh @click="refresh">{{ t('adminDashboard.refresh') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiErrorState v-if="statsError && !statsLoaded" :title="t('adminDashboard.loadFailed')" :error="statsError" @retry="loadStats()" />

    <template v-else>
      <section class="dashboard-page__metrics" :aria-label="t('adminDashboard.metrics.label')">
        <RouterLink v-for="metric in metrics" :key="metric.key" :to="metric.to" class="dashboard-page__metric-link" :data-metric="metric.key">
          <UiMetricCard
            :label="metric.label"
            :icon="metric.icon"
            :value="metric.value"
            :detail="metric.detail"
            :trend="metric.trend"
            :trend-direction="metric.trendDirection"
            :trend-tone="metric.trendTone"
            :sparkline="metric.sparkline"
            :loading="metric.loading"
          />
        </RouterLink>
      </section>

      <UiCard :title="t('adminDashboard.traffic.title')" :description="t('adminDashboard.traffic.description')" as="section">
        <template #actions>
          <RouterLink class="dashboard-page__link" to="/admin/monitor/traffic">{{ t('adminDashboard.traffic.open') }}</RouterLink>
        </template>
        <UiChart
          :option="trafficChartOption"
          :label="t('adminDashboard.traffic.label')"
          :summary="trafficSummary"
          :height="260"
          :loading="trafficLoading"
          :error="trafficError"
          :error-title="t('adminDashboard.traffic.loadFailed')"
          :empty="!trafficHasData"
          :empty-title="t('adminDashboard.traffic.empty')"
          :empty-description="t('adminDashboard.traffic.emptyDescription')"
          :table="trafficTable"
          data-dashboard-traffic
          @retry="loadTraffic"
        />
      </UiCard>

      <div class="dashboard-page__columns">
        <DashboardAlerts
          :offline-nodes="offlineNodes"
          :nodes-error="nodesError"
          :nodes-loading="nodesLoading"
          :open-tickets="openTickets"
          :tickets-error="ticketsError"
          :pending-orders="isCommercial ? Number(stats.pending_orders || 0) : 0"
          :latest-report-at="latestReportAt"
          :kernel-alerts="kernelAlerts"
          :alerts-summary="alertsSummary"
          :alerts-error="alertsError"
          :alerts-loading="alertsLoading"
          :view="alertsView"
          @retry-nodes="loadNodes"
          @retry-tickets="loadTickets"
          @retry-alerts="loadAlerts"
          @update:view="setAlertsView"
        />
        <DashboardActivity
          :entries="activity"
          :loading="activityLoading"
          :error="activityError"
          @retry="loadActivity"
        />
      </div>

      <UiSection v-if="isCommercial" :title="t('adminDashboard.commerce.title')">
        <div class="dashboard-page__metrics">
          <RouterLink v-for="metric in commerceMetrics" :key="metric.key" :to="metric.to" class="dashboard-page__metric-link" :data-metric="metric.key">
            <UiMetricCard :label="metric.label" :icon="metric.icon" :value="metric.value" :detail="metric.detail" :loading="metric.loading" />
          </RouterLink>
        </div>
      </UiSection>
    </template>
  </div>
</template>

<script setup>
// 仪表盘 (plan §7.4 dashboard template, §8.2): four metric cards (users,
// online / total nodes, today's traffic, open tickets) → the 24-hour traffic
// chart → two columns: what needs attention (offline nodes, open tickets,
// stalled traffic reports; pending orders in the commercial edition) and
// the recent audit log. Every number comes from an existing endpoint:
// GET /admin/dashboard (cached 60 s; 刷新 asks refresh=true),
// /admin/traffic/hourly (24 h), /admin/ticket, /admin/nodes (offline ones)
// /admin/system/audit-logs and GET /api/v4/kernel/alerts (certificates and
// stalled phases, merged into 需要处理). Each block loads, fails and retries on its
// own. Revenue and orders stay commercial-only.
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Activity, CircleDollarSign, LifeBuoy, ReceiptText, RotateCw, Server, Users } from '@lucide/vue'
import { getDashboard, getKernelAlerts, getNodes, getSystemAuditLogs, getTickets, getTrafficHourly } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useEdition } from '@/composables/useEdition'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiChart from '@/ui/UiChart.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiMetricCard from '@/ui/UiMetricCard.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSection from '@/ui/UiSection.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { readNodeList, statusName } from './nodes/nodeData'
import { errorText, hasTraffic, peakPoint, readTrafficMeta, readTrafficSeries, trafficOption, trafficTotal } from './monitor/trafficData'
import DashboardActivity from './dashboard/DashboardActivity.vue'
import DashboardAlerts from './dashboard/DashboardAlerts.vue'

const ACTIVITY_SIZE = 6
const OPEN_TICKET = 0
const ANSWERED_TICKET = 1

const { t } = useAppI18n()
const format = useFormat()
const { isCommercial } = useEdition()

const stats = ref({})
const statsLoaded = ref(false)
const statsLoading = ref(false)
const statsError = ref(null)

const points = ref([])
const trafficMeta = ref({})
const trafficLoading = ref(false)
const trafficError = ref(null)

const tickets = ref(null)
const ticketsLoading = ref(false)
const ticketsError = ref(null)

const nodes = ref([])
const nodesLoading = ref(false)
const nodesError = ref(null)

// The kernel's alerts: the active ones, or the resolved history. They load
// apart from the rest and never keep the page busy; a server without the route
// (404) just has none.
const RESOLVED_SIZE = 30
const kernelAlerts = ref([])
const alertsSummary = ref({ active: 0, critical: 0, warning: 0 })
const alertsLoading = ref(false)
const alertsError = ref(null)
const alertsView = ref('active')
let alertsRequest = 0

const activity = ref([])
const activityLoading = ref(false)
const activityError = ref(null)

const loading = computed(() => statsLoading.value || trafficLoading.value || ticketsLoading.value || nodesLoading.value || activityLoading.value)

function readPayload(res) {
  if (!res || typeof res !== 'object') return {}
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  return payload && typeof payload === 'object' ? payload : {}
}

function readList(res) {
  if (res && typeof res.code === 'number' && res.code !== 0) throw new Error(res.msg || res.message || 'request failed')
  const payload = readPayload(res)
  if (Array.isArray(payload)) return payload
  return Array.isArray(payload.list) ? payload.list : []
}

const count = value => format.number(Number(value || 0))

const openTickets = computed(() => (tickets.value || []).filter(ticket => Number(ticket.status) === OPEN_TICKET))
const answeredTickets = computed(() => (tickets.value || []).filter(ticket => Number(ticket.status) === ANSWERED_TICKET))
const offlineNodes = computed(() => nodes.value.filter(node => statusName(node.status) === 'offline'))
const latestReportAt = computed(() => Number(trafficMeta.value?.latest_log_at || 0))

const metrics = computed(() => {
  const s = stats.value
  const waitingStats = statsLoading.value && !statsLoaded.value
  const newUsers = Number(s.today_new_users || 0)
  return [
    {
      key: 'users',
      to: '/admin/users',
      icon: Users,
      label: t('adminDashboard.metrics.users'),
      value: count(s.total_users),
      trend: newUsers > 0 ? t('adminDashboard.metrics.usersToday', { count: count(newUsers) }) : '',
      trendDirection: 'up',
      trendTone: 'positive',
      detail: t('adminDashboard.metrics.usersDetail', { active: count(s.active_users), expired: count(s.expired_users), banned: count(s.banned_users) }),
      loading: waitingStats
    },
    {
      key: 'nodes',
      to: '/admin/monitor',
      icon: Server,
      label: t('adminDashboard.metrics.nodes'),
      value: `${count(s.active_nodes)} / ${count(s.total_nodes)}`,
      detail: t('adminDashboard.metrics.nodesDetail', { count: count(s.online_users) }),
      loading: waitingStats
    },
    {
      key: 'traffic',
      to: '/admin/monitor/traffic',
      icon: Activity,
      label: t('adminDashboard.metrics.traffic'),
      value: format.bytes(s.today_traffic, { precision: 1 }),
      detail: t('adminDashboard.metrics.trafficDetail', { total: format.bytes(s.total_traffic_used) }),
      sparkline: points.value.map(point => point.traffic),
      loading: waitingStats
    },
    {
      key: 'tickets',
      to: '/admin/tickets',
      icon: LifeBuoy,
      label: t('adminDashboard.metrics.tickets'),
      value: tickets.value ? count(openTickets.value.length) : '—',
      detail: tickets.value
        ? t('adminDashboard.metrics.ticketsDetail', { count: count(answeredTickets.value.length) })
        : (ticketsError.value ? t('adminDashboard.metrics.ticketsUnknown') : ''),
      loading: ticketsLoading.value && !tickets.value
    }
  ]
})

const commerceMetrics = computed(() => {
  const s = stats.value
  const waitingStats = statsLoading.value && !statsLoaded.value
  return [
    {
      key: 'revenue',
      to: '/admin/orders',
      icon: CircleDollarSign,
      label: t('adminDashboard.metrics.revenue'),
      value: format.money(s.monthly_income || 0),
      detail: t('adminDashboard.metrics.revenueDetail', { today: format.money(s.today_income || 0), total: format.money(s.total_revenue || 0) }),
      loading: waitingStats
    },
    {
      key: 'orders',
      to: '/admin/orders',
      icon: ReceiptText,
      label: t('adminDashboard.metrics.orders'),
      value: count(s.pending_orders),
      detail: t('adminDashboard.metrics.ordersDetail', { total: count(s.total_orders), paid: count(s.paid_orders) }),
      loading: waitingStats
    }
  ]
})

const hourLabel = ts => {
  const date = new Date(ts * 1000)
  return `${String(date.getHours()).padStart(2, '0')}:00`
}

const trafficHasData = computed(() => hasTraffic(points.value))
const trafficChartOption = computed(() => (trafficHasData.value
  ? trafficOption(points.value, { label: hourLabel, bytes: format.bytes, seriesName: t('adminDashboard.traffic.series') })
  : null))
const trafficSummary = computed(() => {
  const peak = peakPoint(points.value)
  if (!peak) return t('adminDashboard.traffic.summaryEmpty')
  return t('adminDashboard.traffic.summary', {
    total: format.bytes(trafficTotal(points.value)),
    peak: format.bytes(peak.traffic),
    hour: format.dateTime(peak.hour_ts)
  })
})
const trafficTable = computed(() => ({
  columns: [
    { key: 'hour_ts', label: t('adminDashboard.traffic.hour'), format: value => format.dateTime(value) },
    { key: 'traffic', label: t('adminDashboard.traffic.value'), numeric: true, format: value => format.bytes(value) }
  ],
  rows: points.value
}))

async function loadStats(refreshCache = false) {
  statsLoading.value = true
  try {
    stats.value = readPayload(await getDashboard(refreshCache))
    statsLoaded.value = true
    statsError.value = null
  } catch (error) {
    statsError.value = error
  } finally {
    statsLoading.value = false
  }
}

async function loadTraffic() {
  trafficLoading.value = true
  try {
    const res = await getTrafficHourly(24, 0)
    points.value = readTrafficSeries(res)
    trafficMeta.value = readTrafficMeta(res)
    trafficError.value = null
  } catch (error) {
    trafficError.value = errorText(error, t('adminDashboard.traffic.loadFailed'))
  } finally {
    trafficLoading.value = false
  }
}

async function loadTickets() {
  ticketsLoading.value = true
  try {
    tickets.value = readList(await getTickets())
    ticketsError.value = null
  } catch (error) {
    ticketsError.value = error
  } finally {
    ticketsLoading.value = false
  }
}

async function loadNodes() {
  nodesLoading.value = true
  try {
    nodes.value = readNodeList(await getNodes({ page: 1, page_size: 200 }))
    nodesError.value = null
  } catch (error) {
    nodesError.value = error
  } finally {
    nodesLoading.value = false
  }
}

async function loadActivity() {
  activityLoading.value = true
  try {
    const res = await getSystemAuditLogs({ page: 1, page_size: ACTIVITY_SIZE })
    const payload = res?.data?.data || res?.data || {}
    activity.value = Array.isArray(payload.list) ? payload.list.slice(0, ACTIVITY_SIZE) : []
    activityError.value = null
  } catch (error) {
    activityError.value = error
  } finally {
    activityLoading.value = false
  }
}

async function loadAlerts() {
  const request = ++alertsRequest
  const status = alertsView.value
  alertsLoading.value = true
  try {
    const page = await getKernelAlerts(status === 'resolved' ? { status, limit: RESOLVED_SIZE } : { status })
    if (request !== alertsRequest) return
    kernelAlerts.value = page.alerts
    alertsSummary.value = page.summary
    alertsError.value = null
  } catch (error) {
    if (request !== alertsRequest) return
    kernelAlerts.value = []
    alertsError.value = error?.response?.status === 404 ? null : error
  } finally {
    if (request === alertsRequest) alertsLoading.value = false
  }
}

function setAlertsView(view) {
  if (view === alertsView.value || !['active', 'resolved'].includes(view)) return
  alertsView.value = view
  kernelAlerts.value = []
  return loadAlerts()
}

function loadAll(refreshCache = false) {
  return Promise.all([loadStats(refreshCache), loadTraffic(), loadTickets(), loadNodes(), loadActivity(), loadAlerts()])
}

function refresh() {
  return loadAll(true)
}

onMounted(() => loadAll(false))
</script>

<style scoped>
.dashboard-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.dashboard-page__updated {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.dashboard-page__metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
  gap: var(--space-4);
}

.dashboard-page__metric-link {
  display: block;
  min-width: 0;
  border-radius: var(--radius-md);
  color: inherit;
  text-decoration: none;
  transition: transform var(--dur-micro) var(--ease-standard);
}

.dashboard-page__metric-link:hover {
  transform: translateY(-1px);
}

.dashboard-page__metric-link:focus-visible {
  outline: var(--focus-ring);
  outline-offset: 2px;
}

.dashboard-page__link {
  color: var(--accent);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  text-decoration: none;
}

.dashboard-page__link:hover {
  text-decoration: underline;
}

.dashboard-page__columns {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
  gap: var(--space-4);
  align-items: start;
}

@media (prefers-reduced-motion: reduce) {
  .dashboard-page__metric-link:hover {
    transform: none;
  }
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .dashboard-page__link {
    position: relative;
  }

  .dashboard-page__link::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
