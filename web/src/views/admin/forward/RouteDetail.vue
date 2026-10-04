<template>
  <section class="fwd-page" data-testid="forward-route-detail">
    <UiErrorState v-if="loadError && !route" :title="t('forwardV4.detail.loadFailed')" :error="loadError" @retry="refresh" />
    <UiSkeleton v-else-if="!route" variant="card" />

    <template v-else>
      <UiPageHeader :title="route.name || route.id">
        <template #back>
          <RouterLink class="fwd-back" to="/admin/forward/routes"><ChevronLeft :size="16" aria-hidden="true" /> {{ t('forwardV4.nav.routes') }}</RouterLink>
        </template>
        <template #meta>
          <RouteStatusBadge :status="status" hint />
          <span class="fwd-mono detail-id">{{ route.id }} · {{ t('forwardV4.detail.revision', { n: route.revision || '1' }) }}</span>
        </template>
        <template #actions>
          <span class="detail-updated fwd-muted">{{ secondsAgo !== null && secondsAgo < 5 ? t('forwardV4.updated.now') : t('forwardV4.updated.ago', { n: secondsAgo ?? 0 }) }}</span>
          <UiMenu :label="t('forwardV4.routes.more')" :items="menu" />
          <UiButton :icon="Stethoscope" data-testid="forward-diagnose" @click="runDiagnose">{{ t('forwardV4.actions.diagnose') }}</UiButton>
          <UiButton v-if="enforced" :icon="Pencil" @click="router.push({ path: `/admin/forward/routes/${route.id}/edit`, hash: '#limits' })">{{ t(enforced === 'quota' ? 'forwardV4.actions.raiseQuota' : 'forwardV4.actions.extendExpiry') }}</UiButton>
          <UiButton v-else :icon="route.paused ? Play : Pause" :loading="toggling" @click="togglePause">{{ route.paused ? t('forwardV4.actions.resume') : t('forwardV4.actions.pause') }}</UiButton>
          <UiButton variant="primary" :icon="Pencil" @click="router.push(`/admin/forward/routes/${route.id}/edit`)">{{ t('forwardV4.actions.edit') }}</UiButton>
        </template>
      </UiPageHeader>

      <HopChain class="detail-chain" :route="route" :node-name="nodeName" />

      <p v-if="degradedText" class="fwd-note is-warning" role="status">{{ degradedText }}</p>

      <div class="fwd-grid">
        <UiMetricCard :label="t('forwardV4.detail.down', { range: rangeLabel })" :value="fmt.bytes(totals.down, { precision: 1 })" :icon="ArrowDownToLine" :sparkline="spark" />
        <UiMetricCard :label="t('forwardV4.detail.up', { range: rangeLabel })" :value="fmt.bytes(totals.up, { precision: 1 })" :icon="ArrowUpFromLine" />
        <UiMetricCard
          :label="t('forwardV4.detail.activeConns')"
          :value="fmt.number(activeConns, { empty: '0' })"
          :icon="Cable"
          :detail="route.limits?.max_conns ? t('forwardV4.detail.connCap', { n: fmt.number(num(route.limits.max_conns)) }) : t('forwardV4.detail.connNoCap')"
        />
        <UiMetricCard :label="t('forwardV4.detail.latency')" :value="entryLatency" :icon="Timer" :detail="t('forwardV4.detail.latencyDetail')" />
      </div>

      <UiCard :title="t('forwardV4.detail.chainTitle')" :description="t('forwardV4.detail.chainDescription')">
        <ol class="chain">
          <li v-for="(hop, index) in route.hops" :key="index" class="chain__hop">
            <div class="chain__head">
              <span class="chain__role">{{ t(`forwardV4.role.${roleOf(index, route.hops.length)}`) }}</span>
              <EngineChip :engine="hop.engine" />
              <span v-if="index > 0" class="fwd-muted">{{ t('forwardV4.detail.ingress', { link: linkLabel(hop, ` · ${t('forwardV4.detail.mux')}`) }) }}</span>
            </div>
            <ul class="chain__nodes">
              <li v-for="ref in hop.node_refs" :key="ref" class="chain__node" :data-node-ref="ref">
                <div class="chain__node-head">
                  <UiStatusDot :tone="nodeTone(ref).tone" :label="nodeName(ref)" />
                  <span class="fwd-mono">:{{ portOf(index, ref) || '—' }}</span>
                </div>
                <span class="fwd-muted">{{ nodeTone(ref).text }}</span>
                <ul class="chain__ups">
                  <li v-for="(up, upIndex) in upstreamsOf(index, ref)" :key="upIndex" class="chain__up">
                    <UiBadge :tone="HEALTH_TONES[up.state] || 'neutral'" :label="t(`forwardV4.health.${up.state}`)" />
                    <span class="fwd-mono">{{ up.label }}</span>
                    <span class="fwd-muted">{{ up.detail }}</span>
                  </li>
                </ul>
              </li>
            </ul>
            <p class="fwd-muted chain__policy">
              {{ index === route.hops.length - 1
                ? t('forwardV4.detail.targetStrategy', { s: t(`forwardV4.strategy.${route.policy?.target || 'BALANCE_STRATEGY_ROUND_ROBIN'}`) })
                : t('forwardV4.detail.nextHopStrategy', { s: t(`forwardV4.strategy.${route.policy?.next_hop || 'BALANCE_STRATEGY_ROUND_ROBIN'}`) }) }}
            </p>
          </li>
          <li class="chain__hop chain__hop--targets">
            <div class="chain__head"><span class="chain__role">{{ t('forwardV4.detail.targets') }}</span></div>
            <ul class="chain__nodes">
              <li v-for="target in route.targets || []" :key="`${target.host}:${target.port}`" class="chain__node">
                <span class="fwd-mono">{{ target.host }}:{{ target.port }}</span>
                <span class="fwd-muted">{{ t('forwardV4.detail.weightPriority', { w: num(target.weight) || 1, p: num(target.priority) }) }}</span>
              </li>
            </ul>
          </li>
        </ol>
        <p class="fwd-muted detail-dot-note">{{ t('forwardV4.detail.dotNote') }}</p>
      </UiCard>

      <UiCard :title="t('forwardV4.detail.trafficTitle')">
        <template #actions>
          <UiSegmentedControl v-model="range" size="sm" :aria-label="t('forwardV4.detail.range')" :options="ranges" />
        </template>
        <UiChart
          :option="chartOption"
          :label="t('forwardV4.detail.chartLabel', { range: rangeLabel })"
          :summary="t('forwardV4.detail.chartSummary', { down: fmt.bytes(totals.down, { precision: 1 }), up: fmt.bytes(totals.up, { precision: 1 }) })"
          :table="chartTable"
          :height="260"
          :loading="statsLoading && !series.length"
          :error="statsError"
          :empty="!statsLoading && !statsError && !(totals.up + totals.down)"
          :empty-title="t('forwardV4.overview.noTraffic')"
          @retry="loadStats"
        />
        <p v-if="statsTruncated" class="fwd-note is-warning detail-mt" role="status">{{ t('forwardV4.routes.statsTruncated') }}</p>
      </UiCard>

      <UiCard :title="t('forwardV4.detail.healthTitle')" :description="t('forwardV4.detail.healthDescription')">
        <UiDataTable :columns="healthColumns" :rows="healthRows" :label="t('forwardV4.detail.healthTitle')" flat :settings="false" :sticky-header="false" row-key="key" :empty-title="t('forwardV4.detail.noHealth')">
          <template #cell-state="{ row }">
            <UiBadge :tone="HEALTH_TONES[row.state] || 'neutral'" :label="t(`forwardV4.health.${row.state}`)" />
          </template>
          <template #cell-upstream="{ row }"><span class="fwd-mono">{{ row.upstream }}</span></template>
        </UiDataTable>
      </UiCard>

      <div class="fwd-two">
        <UiCard :title="t('forwardV4.detail.nodesTitle')" :description="t('forwardV4.detail.nodesDescription')">
          <ul class="fwd-list">
            <li v-for="ref in nodeRefs" :key="ref" class="fwd-list__item">
              <span class="fwd-cell-stack">
                <RouterLink class="fwd-link" :to="`/admin/forward/inventory/${ref}`">{{ nodeName(ref) }}</RouterLink>
                <span class="fwd-muted">{{ ref }}</span>
              </span>
              <span class="fwd-cell-stack detail-end">
                <UiBadge v-if="!nodeOf(ref)?.reported" tone="neutral" :label="t('forwardV4.nodes.neverReported')" />
                <UiBadge v-else :tone="nodeLag(nodeOf(ref)) ? 'info' : 'success'" :label="nodeLag(nodeOf(ref)) ? t('forwardV4.nodes.lag', { n: nodeLag(nodeOf(ref)) }) : t('forwardV4.nodes.converged')" />
                <span v-if="nodeOf(ref)?.reported" class="fwd-muted">{{ t('forwardV4.nodes.reportedAt', { when: fmt.relativeTime(num(nodeOf(ref).reported_at_unix_ms)) }) }}</span>
              </span>
            </li>
          </ul>
        </UiCard>
        <UiCard :title="t('forwardV4.detail.configTitle')">
          <UiGroupedList>
            <UiGroupedListRow :label="t('forwardV4.editor.listen')" :value="listenText" />
            <UiGroupedListRow :label="t('forwardV4.editor.direct')" :value="t(`forwardV4.direct.${route.policy?.direct || 'DIRECT_MODE_OFF'}`)" />
            <UiGroupedListRow :label="t('forwardV4.detail.healthCheck')" :value="healthText" />
            <UiGroupedListRow :label="t('forwardV4.detail.breaker')" :value="breakerText" />
            <UiGroupedListRow :label="t('forwardV4.editor.bandwidth')" :value="num(route.limits?.bandwidth_bps) ? t('forwardV4.detail.perEntry', { v: `${num(route.limits.bandwidth_bps) / 1e6} Mbps` }) : t('forwardV4.editor.unlimited')" />
            <UiGroupedListRow :label="t('forwardV4.editor.maxConns')" :value="num(route.limits?.max_conns) ? t('forwardV4.detail.perEntry', { v: fmt.number(num(route.limits.max_conns)) }) : t('forwardV4.editor.unlimited')" />
            <UiGroupedListRow :label="t('forwardV4.editor.quota')" :value="num(route.limits?.quota_bytes) ? t('forwardV4.detail.global', { v: fmt.bytes(num(route.limits.quota_bytes)) }) : t('forwardV4.editor.unlimited')" />
            <UiGroupedListRow :label="t('forwardV4.editor.expires')" :value="num(route.limits?.expires_at_unix_ms) ? fmt.dateTime(num(route.limits.expires_at_unix_ms)) : t('forwardV4.editor.unlimited')" />
            <UiGroupedListRow :label="t('forwardV4.editor.labels')" :value="labelsText || '—'" />
            <UiGroupedListRow :label="t('forwardV4.detail.updated')" :value="num(route.updated_at_unix_ms) ? fmt.dateTime(num(route.updated_at_unix_ms)) : '—'" />
          </UiGroupedList>
        </UiCard>
      </div>
    </template>

    <UiSheet v-model:open="diagnoseOpen" size="lg" :title="t('forwardV4.diagnose.title', { name: route?.name || id })" :description="t('forwardV4.diagnose.description')">
      <DiagnosisPanel :result="diagnosis" :loading="diagnosing" :error="diagnoseError" :nodes="nodes" :error-message-for="error => forwardErrorMessage(t, error)" />
      <template #footer>
        <UiButton :icon="RefreshCw" :disabled="diagnosing" @click="runDiagnose">{{ t('forwardV4.diagnose.again') }}</UiButton>
      </template>
    </UiSheet>

    <UiDialog v-model:open="jsonOpen" size="lg" :title="t('forwardV4.detail.json')">
      <UiCodeBlock :code="routeJson" :label="t('forwardV4.detail.json')" max-height="60vh" />
    </UiDialog>
  </section>
</template>

<script setup>
// 路由详情 (F5b): the route's chain with each node's state and upstream
// health, its traffic, the breaker table and its configuration. The dot
// next to a node reflects only this route: a hop error or an unhealthy
// upstream of this route, then the node's lag (owner decision, H16).
// Polls every 15 s while visible; traffic at most every 60 s (D5).
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowDownToLine, ArrowUpFromLine, Cable, ChevronLeft, Copy, FileJson, Pause, Pencil, Play, RefreshCw, Stethoscope, Timer, Trash2 } from '@lucide/vue'
import {
  UiBadge, UiButton, UiCard, UiChart, UiCodeBlock, UiDataTable, UiDialog, UiErrorState, UiGroupedList, UiGroupedListRow, UiMenu,
  UiMetricCard, UiPageHeader, UiSegmentedControl, UiSheet, UiSkeleton, UiStatusDot, useConfirm, useFormat, useToast
} from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteRoute, diagnoseRoute, getNode, getRoute, listNodes, newIdempotencyKey, pauseRoute, resumeRoute, routeStats } from '@/api/forwardV4'
import DiagnosisPanel from '@/components/forward/DiagnosisPanel.vue'
import EngineChip from '@/components/forward/EngineChip.vue'
import HopChain from '@/components/forward/HopChain.vue'
import RouteStatusBadge from '@/components/forward/RouteStatusBadge.vue'
import { forwardErrorMessage } from '@/components/forward/messages'
import { usePolling } from '@/components/forward/usePolling'
import { H21_DEFAULTS, hourlySeries, linkLabel, nodeLag, nodeLagging, num, roleOf, routeStatus, statusContext, trafficWindow } from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const props = defineProps({
  id: { type: String, required: true }
})

const MAX_NODE_VIEWS = 12
const HEALTH_TONES = { HEALTH_STATE_HEALTHY: 'success', HEALTH_STATE_UNHEALTHY: 'danger', HEALTH_STATE_CIRCUIT_OPEN: 'warning', HEALTH_STATE_UNSPECIFIED: 'neutral' }

const router = useRouter()
const currentRoute = useRoute()
const { t } = useAppI18n()
const fmt = useFormat()
const toast = useToast()
const confirm = useConfirm()

const route = ref(null)
const enforced = ref('')
const nodes = ref([])
const canDelete = ref(null)
// node_ref → GET /nodes/{ref} answer ({node, state, report}).
const views = ref(new Map())
const loadError = ref('')

const { secondsAgo, refresh } = usePolling(async () => {
  try {
    const [answer, nodeAnswer] = await Promise.all([getRoute(props.id), listNodes()])
    route.value = answer?.route || null
    enforced.value = answer?.enforced || ''
    nodes.value = nodeAnswer.nodes
    canDelete.value = nodeAnswer.canDelete
    const refs = Array.from(new Set((route.value?.hops || []).flatMap(hop => hop.node_refs || []))).slice(0, MAX_NODE_VIEWS)
    const details = await Promise.all(refs.map(ref => getNode(ref).catch(() => null)))
    views.value = new Map(refs.map((ref, index) => [ref, details[index]]))
    loadError.value = ''
    // Traffic refreshes only when it is a minute old (D5).
    void loadStats({ force: false })
  } catch (error) {
    loadError.value = forwardErrorMessage(t, error)
    throw error
  }
}, { interval: 15_000 })

const nodesByRef = computed(() => new Map(nodes.value.map(node => [node.node_ref, node])))
const nodeOf = ref => nodesByRef.value.get(ref)
const nodeName = ref => nodeOf(ref)?.name || ref
const nodeRefs = computed(() => Array.from(new Set((route.value?.hops || []).flatMap(hop => hop.node_refs || []))))

// This route's hop errors and upstream health, per node.
const reportOf = ref => views.value.get(ref)?.report || null
const routeErrors = ref => (reportOf(ref)?.errors || []).filter(error => error.route_id === props.id)
const routeHealthOf = ref => (reportOf(ref)?.health || []).filter(item => item.route_id === props.id)
const allHealth = computed(() => nodeRefs.value.flatMap(ref => routeHealthOf(ref).map(item => ({ ...item, node_ref: ref }))))
const allErrors = computed(() => nodeRefs.value.flatMap(routeErrors))

const status = computed(() => routeStatus({ route: route.value, enforced: enforced.value }, statusContext({
  nodes: nodeRefs.value.map(nodeOf).filter(Boolean),
  hopErrors: allErrors.value,
  health: allHealth.value
})))

// nodeTone: the route-scoped dot and its words.
function nodeTone(ref) {
  const node = nodeOf(ref)
  if (routeErrors(ref).length) return { tone: 'danger', text: t('forwardV4.detail.dotError', { message: routeErrors(ref)[0].message }) }
  const bad = routeHealthOf(ref).filter(item => item.state === 'HEALTH_STATE_UNHEALTHY' || item.state === 'HEALTH_STATE_CIRCUIT_OPEN')
  const generation = t('forwardV4.detail.generation', { applied: node?.reported ? num(node.reported_generation) : '—', desired: num(node?.desired_generation) })
  if (bad.length) return { tone: 'warning', text: `${generation} · ${t('forwardV4.detail.dotDegraded', { n: bad.length })}` }
  if (!node?.reported) return { tone: 'neutral', text: t('forwardV4.nodes.neverReported') }
  if (nodeLagging(node)) return { tone: 'info', text: `${generation} · ${t('forwardV4.detail.dotLagging')}` }
  return { tone: 'success', text: generation }
}

// The port the node listens on for this hop: its desired state.
function stateHop(index, ref) {
  return (views.value.get(ref)?.state?.hops || []).find(hop => hop.route_id === props.id && num(hop.hop_index) === index)
}
function portOf(index, ref) {
  return num(stateHop(index, ref)?.listen?.port) || (index === 0 ? num(route.value?.listen?.port) : 0)
}

// Upstream address → node name, from the desired upstreams.
const upstreamNames = computed(() => {
  const out = new Map()
  for (const view of views.value.values()) {
    for (const hop of view?.state?.hops || []) {
      for (const up of hop.upstreams || []) {
        if (up.node_ref) out.set(`${up.address}:${num(up.port)}`, nodeName(up.node_ref))
      }
    }
  }
  return out
})
function upstreamLabel(address, port) {
  const name = upstreamNames.value.get(`${address}:${num(port)}`)
  return name ? `${name} :${num(port)}` : `${address}:${num(port)}`
}

function describe(up) {
  let detail = num(up.rtt_us) ? `${Math.round(num(up.rtt_us) / 1000)} ms` : ''
  if (up.state === 'HEALTH_STATE_CIRCUIT_OPEN') {
    const seconds = Math.max(0, Math.round((num(up.circuit_open_until_unix_ms) - Date.now()) / 1000))
    detail = t('forwardV4.detail.breakerOpen', { s: seconds })
  } else if (up.state === 'HEALTH_STATE_UNHEALTHY') {
    detail = t('forwardV4.detail.failures', { n: num(up.consecutive_failures) })
  }
  return { state: up.state || 'HEALTH_STATE_UNSPECIFIED', label: upstreamLabel(up.address, up.port), detail }
}

function upstreamsOf(index, ref) {
  const reported = routeHealthOf(ref).filter(item => num(item.hop_index) === index)
  if (reported.length) return reported.map(describe)
  return (stateHop(index, ref)?.upstreams || []).map(up => ({ state: 'HEALTH_STATE_UNSPECIFIED', label: upstreamLabel(up.address, up.port), detail: '' }))
}

const degradedText = computed(() => {
  const open = allHealth.value.filter(item => item.state === 'HEALTH_STATE_CIRCUIT_OPEN')
  const unhealthy = allHealth.value.filter(item => item.state === 'HEALTH_STATE_UNHEALTHY')
  if (!open.length && !unhealthy.length) return ''
  const parts = []
  if (open.length) {
    const first = open[0]
    parts.push(t('forwardV4.detail.degradedOpen', {
      n: open.length,
      hop: num(first.hop_index) + 1,
      name: upstreamLabel(first.address, first.port),
      s: Math.max(0, Math.round((num(first.circuit_open_until_unix_ms) - Date.now()) / 1000))
    }))
  }
  if (unhealthy.length) parts.push(t('forwardV4.detail.degradedUnhealthy', { n: unhealthy.length }))
  return `${parts.join(t('forwardV4.detail.listJoin'))}${t('forwardV4.detail.degradedTail')}`
})

const healthRows = computed(() => allHealth.value.map((up, index) => ({
  key: index,
  hop: t('forwardV4.diagnose.hop', { n: num(up.hop_index) + 1 }),
  node: nodeName(up.node_ref),
  upstream: upstreamLabel(up.address, up.port),
  state: up.state || 'HEALTH_STATE_UNSPECIFIED',
  rotation: up.state === 'HEALTH_STATE_HEALTHY' ? t('forwardV4.detail.yes') : (up.state === 'HEALTH_STATE_CIRCUIT_OPEN' ? t('forwardV4.detail.noOpen') : t('forwardV4.detail.no')),
  failures: num(up.consecutive_failures),
  rtt: num(up.rtt_us) ? `${(num(up.rtt_us) / 1000).toFixed(1)} ms` : '—',
  checked: num(up.checked_at_unix_ms) ? fmt.relativeTime(num(up.checked_at_unix_ms)) : '—'
})))
const healthColumns = computed(() => [
  { key: 'hop', label: t('forwardV4.detail.columns.hop'), primary: true },
  { key: 'node', label: t('forwardV4.detail.columns.node'), secondary: true },
  { key: 'upstream', label: t('forwardV4.detail.columns.upstream') },
  { key: 'state', label: t('forwardV4.detail.columns.state') },
  { key: 'rotation', label: t('forwardV4.detail.columns.rotation') },
  { key: 'failures', label: t('forwardV4.detail.columns.failures'), align: 'end', numeric: true, breakpoint: 'md' },
  { key: 'rtt', label: t('forwardV4.detail.columns.rtt'), align: 'end', numeric: true },
  { key: 'checked', label: t('forwardV4.detail.columns.checked'), breakpoint: 'lg' }
])

// Entry counters: active connections over the entry nodes.
const counters = ref([])
const activeConns = computed(() => counters.value.filter(item => num(item.hop_index) === 0).reduce((sum, item) => sum + num(item.active_conns), 0))
const entryLatency = computed(() => {
  const rtts = allHealth.value.filter(item => num(item.hop_index) === 0 && num(item.rtt_us)).map(item => num(item.rtt_us))
  return rtts.length ? `${Math.round(Math.min(...rtts) / 1000)} ms` : '—'
})

// ---------------------------------------------------------------------------
// Traffic
// ---------------------------------------------------------------------------

const range = ref('24h')
const RANGE_HOURS = { '24h': 24, '7d': 24 * 7, '30d': 24 * 30 }
const ranges = computed(() => Object.keys(RANGE_HOURS).map(value => ({ value, label: t(`forwardV4.range.${value}`) })))
const rangeLabel = computed(() => t(`forwardV4.range.${range.value}`))
const series = ref([])
const statsLoading = ref(false)
const statsError = ref(null)
const statsTruncated = ref(false)
let statsAt = 0
let statsRange = ''

async function loadStats({ force = true } = {}) {
  const now = Date.now()
  if (!force && statsRange === range.value && now - statsAt < 60_000) return
  statsLoading.value = true
  try {
    const window = trafficWindow(now, RANGE_HOURS[range.value])
    const answer = await routeStats(props.id, window)
    counters.value = answer?.counters || []
    series.value = hourlySeries(answer?.series || [], { ...window, routeId: props.id })
    statsTruncated.value = Boolean(answer?.truncated)
    statsError.value = null
    statsAt = now
    statsRange = range.value
  } catch (error) {
    statsError.value = forwardErrorMessage(t, error)
  } finally {
    statsLoading.value = false
  }
}
watch(range, () => loadStats())

const totals = computed(() => series.value.reduce((sum, point) => ({ up: sum.up + point.up, down: sum.down + point.down }), { up: 0, down: 0 }))
const spark = computed(() => series.value.slice(-24).map(point => point.down))
function pointLabel(ms) {
  const date = new Date(ms)
  const hour = `${String(date.getHours()).padStart(2, '0')}:00`
  return range.value === '24h' ? hour : `${date.getMonth() + 1}/${date.getDate()} ${hour}`
}
const bytes = value => fmt.bytes(value, { precision: 1 })
const chartOption = computed(() => ({
  tooltip: { trigger: 'axis', valueFormatter: bytes },
  legend: { top: 0 },
  grid: { left: 8, right: 16, top: 36, bottom: 8, containLabel: true },
  xAxis: { type: 'category', data: series.value.map(point => pointLabel(point.hour)), boundaryGap: false },
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
  rows: series.value.map(point => ({ hour: pointLabel(point.hour), down: point.down, up: point.up }))
}))

// ---------------------------------------------------------------------------
// Configuration text
// ---------------------------------------------------------------------------

const PROTOCOL = { L4_PROTOCOL_TCP: 'TCP', L4_PROTOCOL_UDP: 'UDP', L4_PROTOCOL_TCP_UDP: 'TCP+UDP' }
const listenText = computed(() => {
  const listen = route.value?.listen || {}
  const text = `${listen.address || '0.0.0.0'}:${num(listen.port) || t('forwardV4.routes.autoPort')} ${PROTOCOL[listen.protocol] || 'TCP'}`
  return listen.entry_hostname ? `${text} · ${listen.entry_hostname}` : text
})
const healthText = computed(() => {
  const health = route.value?.policy?.health || {}
  if (health.disabled) return t('forwardV4.detail.healthDisabled')
  const defaulted = !num(health.interval_ms) && !num(health.timeout_ms)
  const text = t('forwardV4.detail.healthValue', { i: (num(health.interval_ms) || H21_DEFAULTS.interval_ms) / 1000, t: (num(health.timeout_ms) || H21_DEFAULTS.timeout_ms) / 1000 })
  return defaulted ? `${text} ${t('forwardV4.detail.default')}` : text
})
const breakerText = computed(() => {
  const breaker = route.value?.policy?.circuit_breaker || {}
  const defaulted = !num(breaker.failure_threshold) && !num(breaker.open_ms)
  const text = t('forwardV4.detail.breakerValue', { n: num(breaker.failure_threshold) || H21_DEFAULTS.failure_threshold, s: (num(breaker.open_ms) || H21_DEFAULTS.open_ms) / 1000 })
  return defaulted ? `${text} ${t('forwardV4.detail.default')}` : text
})
const labelsText = computed(() => Object.entries(route.value?.labels || {}).map(([key, value]) => `${key}=${value}`).join('  '))
const routeJson = computed(() => JSON.stringify(route.value || {}, null, 2))

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

const toggling = ref(false)
async function togglePause() {
  const pause = !route.value.paused
  toggling.value = true
  try {
    await (pause ? pauseRoute : resumeRoute)(props.id, { idempotencyKey: newIdempotencyKey() })
    toast.success(t(pause ? 'forwardV4.toast.paused' : 'forwardV4.toast.resumed', { name: route.value.name }), { undo: () => togglePause() })
  } catch (error) {
    toast.error(forwardErrorMessage(t, error))
  } finally {
    toggling.value = false
  }
  await refresh()
}

const diagnoseOpen = ref(false)
const diagnosing = ref(false)
const diagnosis = ref(null)
const diagnoseError = ref(null)
async function runDiagnose() {
  diagnoseOpen.value = true
  diagnosing.value = true
  diagnoseError.value = null
  try {
    diagnosis.value = await diagnoseRoute(props.id, { idempotencyKey: newIdempotencyKey() })
  } catch (error) {
    diagnoseError.value = error
  } finally {
    diagnosing.value = false
  }
}
onMounted(() => {
  if (currentRoute.query.diagnose) void runDiagnose()
})

const jsonOpen = ref(false)
const menu = computed(() => [
  { key: 'duplicate', label: t('forwardV4.actions.duplicate'), icon: Copy, onSelect: () => router.push({ path: '/admin/forward/routes/new', query: { from: props.id } }) },
  { key: 'json', label: t('forwardV4.detail.json'), icon: FileJson, onSelect: () => { jsonOpen.value = true } },
  {
    key: 'delete',
    label: canDelete.value === false ? t('forwardV4.actions.deleteSuperOnly') : t('forwardV4.actions.deleteRoute'),
    icon: Trash2,
    danger: true,
    disabled: canDelete.value === false,
    separatorBefore: true,
    onSelect: removeRoute
  }
])

async function removeRoute() {
  const name = route.value?.name || props.id
  await confirm({
    title: t('forwardV4.delete.title', { name }),
    message: t('forwardV4.delete.message'),
    confirmLabel: t('forwardV4.delete.confirm'),
    tone: 'danger',
    requireText: name,
    onConfirm: async () => {
      try {
        await deleteRoute(props.id, { idempotencyKey: newIdempotencyKey() })
      } catch (error) {
        throw new Error(forwardErrorMessage(t, error))
      }
      toast.success(t('forwardV4.toast.deleted', { name }))
      router.push('/admin/forward/routes')
    }
  })
}
</script>

<style scoped>
.detail-id {
  color: var(--label-3);
}

.detail-updated {
  align-self: center;
}

.detail-chain {
  margin-top: calc(var(--space-3) * -1);
}

.detail-end {
  justify-items: end;
}

.detail-mt,
.detail-dot-note {
  margin: var(--space-3) 0 0;
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

@media (max-width: 639.98px) {
  .detail-end {
    justify-items: start;
  }

  .fwd-list__item {
    flex-direction: column;
  }
}
</style>
