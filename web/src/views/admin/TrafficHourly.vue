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
          <select v-model.number="hours" @change="fetchData">
            <option :value="24">{{ t('adminTrafficHourly.range.last24h') }}</option>
            <option :value="168">{{ t('adminTrafficHourly.range.last7d') }}</option>
            <option :value="720">{{ t('adminTrafficHourly.range.last30d') }}</option>
          </select>
        </label>
        <button class="btn btn-primary" @click="fetchData" :disabled="loading">
          {{ loading ? t('adminTrafficHourly.actions.refreshing') : t('adminTrafficHourly.actions.refresh') }}
        </button>
      </div>
    </div>

    <section class="summary-grid">
      <article class="summary-card section-panel">
        <span class="summary-label">{{ t('adminTrafficHourly.summary.total') }}</span>
        <span class="summary-value">{{ formatBytes(totalTraffic) }}</span>
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
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { getTrafficHourly } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

const hours = ref(24)
const loading = ref(false)
const points = ref([])

const chartEl = ref(null)
let chart = null
let echartsLib = null

const hasData = computed(() => points.value.length > 0)
const totalTraffic = computed(() => points.value.reduce((sum, p) => sum + (p.traffic || 0), 0))
const peakPoint = computed(() => {
  if (!points.value.length) return null
  return points.value.reduce((max, p) => (p.traffic > (max?.traffic ?? -1) ? p : max), null)
})
const peakTraffic = computed(() => peakPoint.value?.traffic || 0)
const peakLabel = computed(() => (peakPoint.value ? formatHour(peakPoint.value.hour_ts) : '-'))

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

async function fetchData() {
  loading.value = true
  try {
    const res = await getTrafficHourly(hours.value)
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

function handleResize() {
  if (chart) chart.resize()
}

onMounted(() => {
  fetchData()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
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
</style>
