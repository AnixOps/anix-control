<template>
  <div class="monitor-section" data-monitor-latency>
    <div class="monitor-section__toolbar">
      <UiSegmentedControl v-model="range" :options="rangeOptions" :aria-label="t('adminMonitor.range.label')" data-monitor-range />
      <UiCombobox
        v-if="nodeTargets.length"
        class="monitor-section__picker"
        :model-value="selectedTargetKey"
        :options="targetOptions"
        :aria-label="t('adminMonitor.latency.target')"
        size="md"
        data-monitor-target
        @update:model-value="selectTarget"
      />
      <UiButton class="monitor-section__end" :icon="RotateCw" :loading="targetsLoading || trendLoading" data-monitor-refresh @click="refresh">{{ t('adminMonitor.refresh') }}</UiButton>
    </div>

    <UiCard :title="t('adminMonitor.latency.chart.title')" as="section">
      <UiChart
        :option="chartOption"
        :label="chartLabel"
        :summary="chartSummary"
        :height="320"
        :loading="targetsLoading || trendLoading"
        :error="trendError"
        :error-title="t('adminMonitor.latency.loadFailed')"
        :empty="!nodeTargets.length || !trendPoints.length"
        :empty-title="nodeTargets.length ? t('adminMonitor.latency.noData') : t('adminMonitor.latency.noTargets')"
        :empty-description="nodeTargets.length ? t('adminMonitor.latency.noDataDescription') : t('adminMonitor.latency.noTargetsDescription')"
        :table="chartTable"
        data-monitor-latency-chart
        @retry="loadTrend"
      />
    </UiCard>

    <UiSection :title="t('adminMonitor.latency.targets.title')" :description="t('adminMonitor.latency.targets.description')">
      <UiDataTable
        :columns="targetColumns"
        :rows="nodeTargets"
        row-key="targetKey"
        :label="t('adminMonitor.latency.targets.label')"
        :row-label="targetName"
        :loading="targetsLoading"
        :error="targetsError"
        :error-title="t('adminMonitor.latency.targets.loadFailed')"
        :empty-title="t('adminMonitor.latency.noTargets')"
        :empty-description="t('adminMonitor.latency.noTargetsDescription')"
        storage-key="admin.monitor.targets"
        activatable
        data-monitor-targets
        @row-activate="row => selectTarget(row.targetKey)"
        @retry="refresh"
      >
        <template #cell-label="{ row }">
          <span :class="{ 'monitor-section__strong': row.targetKey === selectedTargetKey }" :aria-current="row.targetKey === selectedTargetKey ? 'true' : undefined">{{ targetName(row) }}</span>
        </template>
        <template #cell-address="{ row }">
          <code class="monitor-section__code">{{ row.host }}:{{ row.port }}</code>
        </template>
        <template #cell-online="{ row }">
          <UiBadge :status="row.online ? 'online' : 'offline'" :label="row.online ? t('adminMonitor.latency.online') : t('adminMonitor.latency.offline')" />
        </template>
      </UiDataTable>
    </UiSection>
  </div>
</template>

<script setup>
// 节点延迟 of 流量与监控 (the old 转发可观测性 › 延迟趋势): the latency
// prober's node targets (GET /admin/forward/observability/targets, type
// node) with their latest sample, and the trend of one target over the
// range (GET …/trend?targetKey=&from=&to=, epoch ms; the API's default was
// the last hour). Avg, P95 and max in ms.
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RotateCw } from '@lucide/vue'
import { getForwardObservabilityTargets, getForwardObservabilityTrend } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiChart from '@/ui/UiChart.vue'
import UiCombobox from '@/ui/UiCombobox.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { MONITOR_RANGES, useMonitorRange } from './useMonitorRange'
import { extractPayload } from './observabilityData'

const { t } = useAppI18n()
const format = useFormat()
const { range, hours } = useMonitorRange()

const targets = ref([])
const targetsLoading = ref(false)
const targetsError = ref(null)
const selectedTargetKey = ref('')
const trendPoints = ref([])
const trendLoading = ref(false)
const trendError = ref(null)
let active = true
let trendSeq = 0

const rangeOptions = computed(() => Object.keys(MONITOR_RANGES).map(value => ({ value, label: t(`adminMonitor.range.${value}`) })))
const nodeTargets = computed(() => targets.value.filter(item => item.targetType === 'node'))
const targetOptions = computed(() => nodeTargets.value.map(item => ({ value: item.targetKey, label: `${targetName(item)} (${item.host}:${item.port})` })))
const selectedTarget = computed(() => nodeTargets.value.find(item => item.targetKey === selectedTargetKey.value) || null)

const ms = value => (value === null || value === undefined || value === '' ? '—' : t('adminMonitor.latency.chart.unit', { value: format.number(Number(value), { maximumFractionDigits: 1 }) }))

function targetName(item) {
  return item?.label || item?.host || '—'
}

const chartOption = computed(() => {
  if (!trendPoints.value.length) return null
  const times = trendPoints.value.map(point => (hours.value > 24 ? format.dateTime(point.bucketAt).slice(5) : format.dateTime(point.bucketAt).slice(11)))
  const series = ['avg', 'p95', 'max'].map(key => ({
    name: t(`adminMonitor.latency.chart.${key}`),
    type: 'line',
    smooth: true,
    showSymbol: false,
    data: trendPoints.value.map(point => point[key])
  }))
  return {
    tooltip: { trigger: 'axis', valueFormatter: value => ms(value) },
    legend: { top: 0 },
    grid: { left: 8, right: 16, top: 36, bottom: 8, containLabel: true },
    xAxis: { type: 'category', data: times, boundaryGap: false, axisLabel: { hideOverlap: true } },
    yAxis: { type: 'value', axisLabel: { formatter: value => `${value} ms` } },
    series
  }
})
const chartLabel = computed(() => t('adminMonitor.latency.chart.label', { target: targetName(selectedTarget.value), range: t(`adminMonitor.range.${range.value}`) }))
const chartSummary = computed(() => {
  const points = trendPoints.value
  if (!points.length) return t('adminMonitor.latency.chart.summaryEmpty')
  const avg = points.reduce((sum, point) => sum + Number(point.avg || 0), 0) / points.length
  const p95 = Math.max(...points.map(point => Number(point.p95 || 0)))
  return t('adminMonitor.latency.chart.summary', { avg: ms(avg), p95: ms(p95) })
})
const chartTable = computed(() => ({
  columns: [
    { key: 'bucketAt', label: t('adminMonitor.latency.chart.time'), format: value => format.dateTime(value) },
    { key: 'avg', label: t('adminMonitor.latency.chart.avg'), numeric: true, format: ms },
    { key: 'p95', label: t('adminMonitor.latency.chart.p95'), numeric: true, format: ms },
    { key: 'max', label: t('adminMonitor.latency.chart.max'), numeric: true, format: ms }
  ],
  rows: trendPoints.value
}))

const targetColumns = computed(() => [
  { key: 'label', label: t('adminMonitor.latency.targets.name'), primary: true, hideable: false, sortable: true, value: targetName },
  { key: 'address', label: t('adminMonitor.latency.targets.address'), secondary: true, breakpoint: 'md', value: item => `${item.host}:${item.port}` },
  { key: 'latestAvgRtt', label: t('adminMonitor.latency.targets.latest'), numeric: true, align: 'end', sortable: true, format: ms },
  { key: 'latestLoss', label: t('adminMonitor.latency.targets.loss'), numeric: true, align: 'end', sortable: true, format: value => format.percent(Number(value || 0) / 100, { precision: 1 }) },
  { key: 'online', label: t('adminMonitor.latency.targets.status'), sortable: true, sortValue: item => (item.online ? 0 : 1) },
  { key: 'latestBucketAt', label: t('adminMonitor.latency.targets.lastSample'), nowrap: true, breakpoint: 'lg', format: value => (value ? format.relativeTime(value) : '—') }
])

async function loadTargets() {
  targetsLoading.value = true
  try {
    const payload = extractPayload(await getForwardObservabilityTargets())
    if (!active) return
    targets.value = Array.isArray(payload?.list) ? payload.list : []
    targetsError.value = null
    if (!nodeTargets.value.some(item => item.targetKey === selectedTargetKey.value)) {
      selectedTargetKey.value = nodeTargets.value[0]?.targetKey || ''
    }
  } catch (error) {
    if (!active) return
    console.error('load observability targets failed:', error)
    targets.value = []
    targetsError.value = error
  } finally {
    if (active) targetsLoading.value = false
  }
}

async function loadTrend() {
  const seq = ++trendSeq
  trendError.value = null
  if (!selectedTargetKey.value) {
    trendPoints.value = []
    return
  }
  trendLoading.value = true
  const to = Date.now()
  try {
    const payload = extractPayload(await getForwardObservabilityTrend({
      targetKey: selectedTargetKey.value,
      from: to - hours.value * 3600 * 1000,
      to
    }))
    if (!active || seq !== trendSeq) return
    trendPoints.value = Array.isArray(payload?.points) ? payload.points : []
  } catch (error) {
    if (!active || seq !== trendSeq) return
    console.error('load latency trend failed:', error)
    trendPoints.value = []
    trendError.value = error
  } finally {
    if (active && seq === trendSeq) trendLoading.value = false
  }
}

function selectTarget(key) {
  if (!key || key === selectedTargetKey.value) return
  selectedTargetKey.value = key
  void loadTrend()
}

async function refresh() {
  await loadTargets()
  await loadTrend()
}

watch(hours, () => { void loadTrend() })

onMounted(() => { void refresh() })
onUnmounted(() => { active = false })
</script>
