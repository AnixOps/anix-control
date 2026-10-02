<template>
  <div class="monitor-section" data-monitor-traffic>
    <div class="monitor-section__toolbar">
      <UiSegmentedControl v-model="range" :options="rangeOptions" :aria-label="t('adminMonitor.range.label')" data-monitor-range />
      <UiCombobox
        class="monitor-section__picker"
        :model-value="selectedUserId"
        :options="userOptions"
        :aria-label="t('adminMonitor.traffic.user')"
        size="md"
        data-monitor-user
        @update:model-value="value => selectUser(value)"
      />
      <UiButton class="monitor-section__end" :icon="RotateCw" :loading="loading" data-monitor-refresh @click="refreshAll">{{ t('adminMonitor.refresh') }}</UiButton>
    </div>

    <section class="monitor-section__metrics" :aria-label="t('adminMonitor.traffic.summary.label')">
      <UiMetricCard :label="t('adminMonitor.traffic.summary.total')" :value="format.bytes(totalTraffic)" :detail="selectedLabel" :loading="loading && !points.length" data-summary="total" />
      <UiMetricCard :label="t('adminMonitor.traffic.summary.peak')" :value="peak ? format.bytes(peak.traffic) : '—'" :detail="peak ? format.dateTime(peak.hour_ts) : ''" :loading="loading && !points.length" data-summary="peak" />
      <UiMetricCard
        :label="t('adminMonitor.traffic.summary.latestReport')"
        :value="latestLogAt ? format.relativeTime(latestLogAt) : '—'"
        :detail="latestLogAt ? format.dateTime(latestLogAt) : t('adminMonitor.traffic.summary.noReport')"
        :loading="loading && !points.length"
        data-summary="latest"
      />
    </section>

    <UiCard :title="t('adminMonitor.traffic.chart.title')" as="section">
      <template v-if="selectedUserId" #actions>
        <UiButton variant="tertiary" size="sm" :icon="X" data-monitor-clear-user @click="selectUser(0)">{{ t('adminMonitor.traffic.clearUser') }}</UiButton>
      </template>
      <UiChart
        :option="chartOption"
        :label="chartLabel"
        :summary="chartSummary"
        :height="320"
        :loading="loading"
        :error="errorMessage"
        :error-title="t('adminMonitor.traffic.loadFailed')"
        :empty="!hasData"
        :empty-title="t('adminMonitor.traffic.empty')"
        :empty-description="emptyDetail"
        :table="chartTable"
        data-monitor-traffic-chart
        @retry="fetchChart()"
      />
    </UiCard>

    <UiSection :title="t('adminMonitor.traffic.ranking.title')" :description="t('adminMonitor.traffic.ranking.description')">
      <UiDataTable
        :columns="rankingColumns"
        :rows="ranking"
        row-key="user_id"
        :label="t('adminMonitor.traffic.ranking.label')"
        :row-label="row => row.email || `#${row.user_id}`"
        :loading="rankingLoading"
        :error="rankingErrorMessage"
        :error-title="t('adminMonitor.traffic.ranking.loadFailed')"
        :empty-title="t('adminMonitor.traffic.ranking.empty')"
        :page-size="20"
        storage-key="admin.monitor.ranking"
        activatable
        data-monitor-ranking
        @row-activate="row => selectUser(row.user_id)"
        @retry="fetchRanking"
      >
        <template #cell-email="{ row }">
          <span :class="{ 'monitor-section__strong': row.user_id === selectedUserId }" :aria-current="row.user_id === selectedUserId ? 'true' : undefined">{{ row.email || `#${row.user_id}` }}</span>
        </template>
      </UiDataTable>
    </UiSection>
  </div>
</template>

<script setup>
// 用户流量 of 流量与监控 (the old 小时流量 page): hourly traffic over the
// range (GET /admin/traffic/hourly?hours=&user_id=), for all users or one,
// and the user ranking of the same range (GET /admin/traffic/user-ranking,
// limit 1000, zero users included) that picks the user. A range change
// reloads both; a user who leaves the ranking falls back to all users. Late
// answers of an earlier request are ignored.
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { RotateCw, X } from '@lucide/vue'
import { getTrafficHourly, getUserTrafficRanking } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiChart from '@/ui/UiChart.vue'
import UiCombobox from '@/ui/UiCombobox.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiMetricCard from '@/ui/UiMetricCard.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { errorText, hasTraffic, peakPoint, readRankingRows, readTrafficMeta, readTrafficSeries, trafficOption, trafficTotal } from './trafficData'
import { MONITOR_RANGES, useMonitorRange } from './useMonitorRange'

const { t } = useAppI18n()
const format = useFormat()
const { range, hours } = useMonitorRange()

const selectedUserId = ref(0)
const loading = ref(false)
const points = ref([])
const ranking = ref([])
const rankingLoading = ref(false)
const trafficMeta = ref({})
const errorMessage = ref('')
const rankingErrorMessage = ref('')

let isActive = true
let chartRequestSeq = 0
let rankingRequestSeq = 0

const rangeOptions = computed(() => Object.keys(MONITOR_RANGES).map(value => ({ value, label: t(`adminMonitor.range.${value}`) })))
const userOptions = computed(() => [
  { value: 0, label: t('adminMonitor.traffic.allUsers') },
  ...ranking.value.map(row => ({ value: row.user_id, label: row.email || `#${row.user_id}` }))
])
const hasData = computed(() => hasTraffic(points.value))
const totalTraffic = computed(() => trafficTotal(points.value))
const peak = computed(() => peakPoint(points.value))
const latestLogAt = computed(() => Number(trafficMeta.value?.latest_log_at || 0))
const emptyDetail = computed(() => (latestLogAt.value
  ? t('adminMonitor.traffic.emptyWithLatest', { time: format.dateTime(latestLogAt.value) })
  : t('adminMonitor.traffic.emptyNever')))
const selectedLabel = computed(() => {
  if (!selectedUserId.value) return t('adminMonitor.traffic.allUsers')
  const row = ranking.value.find(item => item.user_id === selectedUserId.value)
  return row ? (row.email || `#${row.user_id}`) : `#${selectedUserId.value}`
})

function hourLabel(ts) {
  return hours.value > 24 ? format.dateTime(ts).slice(5) : format.dateTime(ts).slice(11)
}

const chartOption = computed(() => (hasData.value
  ? trafficOption(points.value, { label: hourLabel, bytes: format.bytes, seriesName: t('adminMonitor.traffic.chart.series') })
  : null))
const chartLabel = computed(() => t('adminMonitor.traffic.chart.label', { user: selectedLabel.value, range: t(`adminMonitor.range.${range.value}`) }))
const chartSummary = computed(() => (peak.value
  ? t('adminMonitor.traffic.chart.summary', { total: format.bytes(totalTraffic.value), peak: format.bytes(peak.value.traffic), hour: format.dateTime(peak.value.hour_ts) })
  : t('adminMonitor.traffic.chart.summaryEmpty')))
const chartTable = computed(() => ({
  columns: [
    { key: 'hour_ts', label: t('adminMonitor.traffic.chart.hour'), format: value => format.dateTime(value) },
    { key: 'traffic', label: t('adminMonitor.traffic.chart.value'), numeric: true, format: value => format.bytes(value) }
  ],
  rows: points.value
}))

const rankingColumns = computed(() => [
  { key: 'rank', label: t('adminMonitor.traffic.ranking.rank'), numeric: true, width: 72, card: false },
  { key: 'email', label: t('adminMonitor.traffic.ranking.user'), primary: true, hideable: false, truncate: true, maxWidth: 360, value: row => row.email || `#${row.user_id}` },
  { key: 'traffic', label: t('adminMonitor.traffic.ranking.traffic'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => format.bytes(value) }
])

async function fetchChart(userId = selectedUserId.value, rangeHours = hours.value) {
  const requestSeq = ++chartRequestSeq
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await getTrafficHourly(rangeHours, userId)
    if (!isActive || requestSeq !== chartRequestSeq) return
    points.value = readTrafficSeries(res)
    trafficMeta.value = readTrafficMeta(res)
    await nextTick()
  } catch (err) {
    if (!isActive || requestSeq !== chartRequestSeq) return
    console.error(t('adminMonitor.traffic.loadFailed'), err)
    errorMessage.value = errorText(err, t('adminMonitor.traffic.loadFailed'))
    points.value = []
    trafficMeta.value = {}
  } finally {
    if (isActive && requestSeq === chartRequestSeq) {
      loading.value = false
    }
  }
}

async function fetchRanking() {
  const requestSeq = ++rankingRequestSeq
  rankingErrorMessage.value = ''
  rankingLoading.value = true
  try {
    const res = await getUserTrafficRanking(hours.value, 1000, true)
    if (!isActive || requestSeq !== rankingRequestSeq) return
    ranking.value = readRankingRows(res).map((row, index) => ({ ...row, rank: index + 1 }))
    // The selected user is no longer in the range: back to all users.
    if (selectedUserId.value && !ranking.value.some(row => row.user_id === selectedUserId.value)) {
      selectedUserId.value = 0
    }
  } catch (err) {
    if (!isActive || requestSeq !== rankingRequestSeq) return
    console.error(t('adminMonitor.traffic.ranking.loadFailed'), err)
    rankingErrorMessage.value = errorText(err, t('adminMonitor.traffic.ranking.loadFailed'))
  } finally {
    if (isActive && requestSeq === rankingRequestSeq) rankingLoading.value = false
  }
}

function selectUser(userId) {
  selectedUserId.value = Number(userId) || 0
  fetchChart(selectedUserId.value, hours.value)
}

// A new range: the ranking and the chart both reload.
async function refreshAll() {
  await fetchRanking()
  await fetchChart(selectedUserId.value, hours.value)
}

watch(hours, () => { void refreshAll() })

onMounted(() => { void refreshAll() })

onUnmounted(() => {
  isActive = false
})
</script>
