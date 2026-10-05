<template>
  <div class="node-traffic" data-testid="node-traffic">
    <div class="node-traffic__toolbar">
      <UiSegmentedControl
        :model-value="range"
        :options="rangeOptions"
        :aria-label="t('admin.nodes.traffic.range')"
        data-testid="node-traffic-range"
        @update:model-value="select"
      />
      <UiButton
        class="node-traffic__refresh"
        :icon="RotateCw"
        :loading="status === 'loading'"
        data-testid="node-traffic-refresh"
        @click="load"
      >{{ t('admin.nodes.traffic.refresh') }}</UiButton>
    </div>

    <section class="node-traffic__metrics" :aria-label="t('admin.nodes.traffic.summaryLabel')">
      <UiMetricCard :label="t('admin.nodes.traffic.upload')" :value="shown ? format.bytes(series.total.up, { precision: 1 }) : '—'" :loading="loadingFirst" data-summary="up" />
      <UiMetricCard :label="t('admin.nodes.traffic.download')" :value="shown ? format.bytes(series.total.down, { precision: 1 }) : '—'" :loading="loadingFirst" data-summary="down" />
      <UiMetricCard
        :label="t('admin.nodes.traffic.total')"
        :value="shown ? format.bytes(series.total.up + series.total.down, { precision: 1 }) : '—'"
        :detail="shown ? windowText : ''"
        :loading="loadingFirst"
        data-summary="total"
      />
    </section>

    <UiCard :title="t('admin.nodes.traffic.chart.title')" heading-tag="h3" as="section">
      <UiChart
        :option="option"
        :label="chartLabel"
        :summary="chartSummary"
        :height="chartHeight"
        :loading="status === 'loading' || status === 'idle'"
        :error="error"
        :error-title="t('admin.nodes.traffic.loadFailed')"
        :empty="status === 'ready' && !hasData"
        :empty-title="t('admin.nodes.traffic.empty')"
        :empty-description="emptyDescription"
        :table="table"
        :state-heading-tag="'h4'"
        data-testid="node-traffic-chart"
        @retry="load"
      />
      <p v-if="shown" class="node-traffic__note" data-testid="node-traffic-note">{{ series.granularity === 'day' ? t('admin.nodes.traffic.noteDay') : t('admin.nodes.traffic.noteHour') }}</p>
    </UiCard>
  </div>
</template>

<script setup>
// A proxy node's traffic over time (GET /api/v4/kernel/nodes/:id/traffic):
// upload and download as two lines in the unit of the peak, the totals of the
// range as metric cards, a range switch (24 h, 7 d and 30 d by the hour; 90 d
// and 1 y by the day), the chart's data as a table for screen readers, and
// loading, empty and error states of their own. The state lives in
// useNodeTraffic; this draws it. Used by the node page's 流量 section and the
// 实时节点 traffic sheet.
import { computed, onMounted } from 'vue'
import { RotateCw } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiChart from '@/ui/UiChart.vue'
import UiMetricCard from '@/ui/UiMetricCard.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import { TRAFFIC_RANGE_KEYS, bucketLabelTime, hasTraffic, peakBucket, trafficSeriesOption } from './nodeTraffic'
import { useNodeTraffic } from './useNodeTraffic'

const props = defineProps({
  nodeId: { type: [Number, String], required: true },
  // Names the chart for screen readers; the page title already says it.
  nodeName: { type: String, default: '' },
  initialRange: { type: String, default: '24h' },
  chartHeight: { type: Number, default: 320 }
})

const { t } = useAppI18n()
const format = useFormat()
const { range, status, series, error, load, select } = useNodeTraffic(() => props.nodeId, { initialRange: props.initialRange })

const rangeOptions = computed(() => TRAFFIC_RANGE_KEYS.map(value => ({ value, label: t(`admin.nodes.traffic.ranges.${value}`) })))
const rangeName = computed(() => t(`admin.nodes.traffic.rangeNames.${range.value}`))
const shown = computed(() => status.value === 'ready' && Boolean(series.value))
const loadingFirst = computed(() => !series.value && (status.value === 'loading' || status.value === 'idle'))
const hasData = computed(() => hasTraffic(series.value))

// An hour is named by date and time, a day by its date.
function labelOf(time, granularity) {
  return granularity === 'day' ? format.date(time) : format.dateTime(time)
}

const windowText = computed(() => {
  const data = series.value
  if (!data?.points.length) return ''
  // From the first bucket to the last one (each is named by where it starts).
  const first = data.points[0]
  const last = data.points[data.points.length - 1]
  const from = labelOf(bucketLabelTime(first.start, data.granularity), data.granularity)
  const to = labelOf(bucketLabelTime(last.start, data.granularity), data.granularity)
  return t('admin.nodes.traffic.window', { from, to })
})

const option = computed(() => (shown.value && hasData.value
  ? trafficSeriesOption(series.value, {
    range: range.value,
    label: labelOf,
    bytes: value => format.bytes(value, { precision: 1 }),
    upName: t('admin.nodes.traffic.upload'),
    downName: t('admin.nodes.traffic.download')
  })
  : null))

const chartLabel = computed(() => t('admin.nodes.traffic.chart.label', { node: props.nodeName || `#${props.nodeId}`, range: rangeName.value }))
const chartSummary = computed(() => {
  const data = series.value
  if (!shown.value) return ''
  const peak = peakBucket(data)
  if (!peak) return t('admin.nodes.traffic.chart.summaryEmpty')
  return t('admin.nodes.traffic.chart.summary', {
    up: format.bytes(data.total.up, { precision: 1 }),
    down: format.bytes(data.total.down, { precision: 1 }),
    bucket: t(data.granularity === 'day' ? 'admin.nodes.traffic.chart.bucketDay' : 'admin.nodes.traffic.chart.bucketHour'),
    peak: format.bytes(peak.up + peak.down, { precision: 1 }),
    time: labelOf(bucketLabelTime(peak.start, data.granularity), data.granularity)
  })
})
const emptyDescription = computed(() => t(series.value?.granularity === 'day' ? 'admin.nodes.traffic.emptyDay' : 'admin.nodes.traffic.emptyHour'))

const table = computed(() => {
  const data = series.value
  if (!shown.value || !hasData.value) return null
  return {
    columns: [
      { key: 'time', label: t('admin.nodes.traffic.chart.time') },
      { key: 'up', label: t('admin.nodes.traffic.upload'), numeric: true, format: value => format.bytes(value) },
      { key: 'down', label: t('admin.nodes.traffic.download'), numeric: true, format: value => format.bytes(value) },
      { key: 'sum', label: t('admin.nodes.traffic.total'), numeric: true, format: value => format.bytes(value) }
    ],
    rows: data.points.map(point => ({
      time: labelOf(bucketLabelTime(point.start, data.granularity), data.granularity),
      up: point.up,
      down: point.down,
      sum: point.up + point.down
    }))
  }
})

onMounted(load)

defineExpose({ load, range, series, status })
</script>

<style scoped>
.node-traffic {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.node-traffic__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
}

.node-traffic__metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 200px), 1fr));
  gap: var(--space-4);
}

.node-traffic__note {
  margin: var(--space-3) 0 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  line-height: var(--type-caption-line);
}
</style>
