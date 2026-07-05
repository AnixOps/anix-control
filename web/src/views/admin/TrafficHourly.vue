<template>
  <div class="page-shell">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminTrafficHourly.title') }}</h1>
        <p>{{ t('adminTrafficHourly.subtitle') }}</p>
      </div>
      <div class="toolbar-actions">
        <label class="range-field">
          <span>{{ t('adminTrafficHourly.range.label') }}</span>
          <select v-model.number="hours" @change="onRangeChange">
            <option :value="24">{{ t('adminTrafficHourly.range.last24h') }}</option>
            <option :value="168">{{ t('adminTrafficHourly.range.last7d') }}</option>
            <option :value="720">{{ t('adminTrafficHourly.range.last30d') }}</option>
          </select>
        </label>
        <label class="range-field">
          <span>{{ t('adminTrafficHourly.user.label') }}</span>
          <select v-model.number="selectedUserId" @change="fetchChart">
            <option :value="0">{{ t('adminTrafficHourly.user.all') }}</option>
            <option v-for="u in ranking" :key="u.user_id" :value="u.user_id">
              {{ u.email || ('#' + u.user_id) }}
            </option>
          </select>
        </label>
        <button class="btn btn-primary" @click="refreshAll" :disabled="loading">
          {{ loading ? t('adminTrafficHourly.actions.refreshing') : t('adminTrafficHourly.actions.refresh') }}
        </button>
      </div>
    </div>

    <section class="summary-grid">
      <article class="summary-card section-panel">
        <span class="summary-label">{{ t('adminTrafficHourly.summary.total') }}</span>
        <span class="summary-value">{{ formatBytes(totalTraffic) }}</span>
        <span class="summary-detail">{{ selectedLabel }}</span>
      </article>
      <article class="summary-card section-panel">
        <span class="summary-label">{{ t('adminTrafficHourly.summary.peak') }}</span>
        <span class="summary-value">{{ formatBytes(peakTraffic) }}</span>
        <span class="summary-detail">{{ peakLabel }}</span>
      </article>
    </section>

    <section class="section-panel chart-panel">
      <div v-show="hasData" ref="chartEl" class="chart-canvas"></div>
      <div v-if="!hasData && !loading" class="empty-state">{{ t('adminTrafficHourly.empty') }}</div>
    </section>

    <section class="section-panel ranking-panel">
      <div class="ranking-header">
        <h2>{{ t('adminTrafficHourly.ranking.title') }}</h2>
        <span class="ranking-hint">{{ t('adminTrafficHourly.ranking.hint') }}</span>
      </div>
      <div v-if="ranking.length" class="ranking-table">
        <div class="ranking-row ranking-row-head">
          <span class="col-rank">#</span>
          <span class="col-user">{{ t('adminTrafficHourly.ranking.user') }}</span>
          <span class="col-traffic">{{ t('adminTrafficHourly.ranking.traffic') }}</span>
        </div>
        <div
          v-for="(u, idx) in ranking"
          :key="u.user_id"
          class="ranking-row"
          :class="{ active: u.user_id === selectedUserId }"
          @click="selectUser(u.user_id)"
        >
          <span class="col-rank">{{ idx + 1 }}</span>
          <span class="col-user" :title="u.email">{{ u.email || ('#' + u.user_id) }}</span>
          <span class="col-traffic">{{ formatBytes(u.traffic) }}</span>
        </div>
      </div>
      <div v-else class="empty-state">{{ t('adminTrafficHourly.ranking.empty') }}</div>
    </section>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { getTrafficHourly, getUserTrafficRanking } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

const hours = ref(24)
const selectedUserId = ref(0)
const loading = ref(false)
const points = ref([])
const ranking = ref([])

const chartEl = ref(null)
let chart = null
let echartsLib = null
let isActive = true

const hasData = computed(() => points.value.length > 0)
const totalTraffic = computed(() => points.value.reduce((sum, p) => sum + (p.traffic || 0), 0))
const peakPoint = computed(() => {
  if (!points.value.length) return null
  return points.value.reduce((max, p) => (p.traffic > (max?.traffic ?? -1) ? p : max), null)
})
const peakTraffic = computed(() => peakPoint.value?.traffic || 0)
const peakLabel = computed(() => (peakPoint.value ? formatHour(peakPoint.value.hour_ts) : '-'))
const selectedLabel = computed(() => {
  if (!selectedUserId.value) return t('adminTrafficHourly.user.all')
  const u = ranking.value.find(x => x.user_id === selectedUserId.value)
  return u ? (u.email || ('#' + u.user_id)) : ('#' + selectedUserId.value)
})

function formatHour(ts) {
  // 后端返回整点 Unix 秒
  return formatDateTime(ts * 1000, { year: undefined, second: undefined })
}

function formatBytes(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(2)} ${units[index]}`
}

async function ensureECharts() {
  if (!echartsLib) {
    echartsLib = await import('echarts')
  }
  return echartsLib
}

async function renderChart() {
  if (!chartEl.value || !points.value.length) return
  const echarts = await ensureECharts()
  if (!isActive || !chartEl.value || !chartEl.value.isConnected) return
  if (!chart) {
    chart = echarts.init(chartEl.value)
  }
  const labels = points.value.map(p => formatHour(p.hour_ts))
  const values = points.value.map(p => p.traffic || 0)
  chart.setOption({
    tooltip: {
      trigger: 'axis',
      valueFormatter: val => formatBytes(val)
    },
    grid: { left: 64, right: 16, top: 24, bottom: 48 },
    xAxis: {
      type: 'category',
      data: labels,
      boundaryGap: false,
      axisLabel: { rotate: hours.value > 24 ? 45 : 0 }
    },
    yAxis: {
      type: 'value',
      name: t('adminTrafficHourly.chart.yAxis'),
      axisLabel: { formatter: val => formatBytes(val) }
    },
    series: [
      {
        name: t('adminTrafficHourly.chart.seriesName'),
        type: 'line',
        smooth: true,
        areaStyle: {},
        showSymbol: false,
        data: values
      }
    ]
  })
  chart.resize()
}

async function fetchChart() {
  loading.value = true
  try {
    const res = await getTrafficHourly(hours.value, selectedUserId.value)
    points.value = Array.isArray(res.data) ? res.data : []
    await nextTick()
    renderChart()
  } catch (err) {
    console.error(t('adminTrafficHourly.messages.fetchFailed'), err)
    points.value = []
  } finally {
    loading.value = false
  }
}

async function fetchRanking() {
  try {
    const res = await getUserTrafficRanking(hours.value, 20)
    ranking.value = Array.isArray(res.data) ? res.data : []
    // 若当前选中的用户已不在区间内, 回退到全局
    if (selectedUserId.value && !ranking.value.some(u => u.user_id === selectedUserId.value)) {
      selectedUserId.value = 0
    }
  } catch (err) {
    console.error(t('adminTrafficHourly.messages.fetchFailed'), err)
    ranking.value = []
  }
}

function selectUser(userId) {
  selectedUserId.value = userId
  fetchChart()
}

// 时间范围变化: 排行榜和图表都要重新拉
async function onRangeChange() {
  await fetchRanking()
  await fetchChart()
}

async function refreshAll() {
  await fetchRanking()
  await fetchChart()
}

function handleResize() {
  if (chart) chart.resize()
}

onMounted(async () => {
  await fetchRanking()
  await fetchChart()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  isActive = false
  window.removeEventListener('resize', handleResize)
  if (chart) {
    chart.dispose()
    chart = null
  }
})
</script>

<style scoped>
.page-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
  flex-wrap: wrap;
}

.toolbar-actions {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  flex-wrap: wrap;
}

.range-field {
  display: inline-flex;
  flex-direction: column;
  gap: 4px;
  font-size: 13px;
  color: var(--text-secondary);
}

.range-field select {
  min-width: 160px;
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid var(--border-color);
  background: var(--surface-color);
  color: var(--text-color);
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 20px;
  margin: 20px 0;
}

.summary-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 20px;
}

.summary-label {
  color: var(--text-secondary);
  font-size: 13px;
}

.summary-value {
  font-size: 28px;
  font-weight: 700;
  line-height: 1.1;
}

.summary-detail {
  color: var(--text-secondary);
  font-size: 12px;
}

.chart-panel {
  padding: 20px;
}

.chart-canvas {
  width: 100%;
  height: 380px;
}

.empty-state {
  padding: 60px 16px;
  text-align: center;
  color: var(--text-secondary);
}

.ranking-panel {
  padding: 20px;
  margin-top: 20px;
}

.ranking-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.ranking-header h2 {
  font-size: 18px;
  line-height: 1.2;
}

.ranking-hint {
  color: var(--text-secondary);
  font-size: 12px;
}

.ranking-table {
  display: flex;
  flex-direction: column;
}

.ranking-row {
  display: grid;
  grid-template-columns: 48px 1fr 160px;
  gap: 12px;
  align-items: center;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border-color);
  cursor: pointer;
  border-radius: 6px;
}

.ranking-row:hover {
  background: var(--surface-hover);
}

.ranking-row.active {
  background: var(--primary-soft);
}

.ranking-row-head {
  cursor: default;
  color: var(--text-secondary);
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.ranking-row-head:hover {
  background: transparent;
}

.col-rank {
  text-align: center;
  color: var(--text-secondary);
}

.col-user {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.col-traffic {
  text-align: right;
  font-weight: 600;
}
</style>
