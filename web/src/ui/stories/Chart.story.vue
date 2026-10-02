<script setup>
import { computed, ref } from 'vue'
import UiChart from '../UiChart.vue'
import UiCard from '../UiCard.vue'
import UiButton from '../UiButton.vue'
import UiSegmentedControl from '../UiSegmentedControl.vue'

const hours = Array.from({ length: 24 }, (_, index) => `${String(index).padStart(2, '0')}:00`)
const traffic = hours.map((_, index) => Math.round((Math.sin(index / 3.5) + 1.4) * 900 + (index % 5) * 120))
const gb = value => `${(value / 1024).toFixed(1)} GB`

const kind = ref('line')
const kinds = [{ value: 'line', label: '折线' }, { value: 'bar', label: '柱状' }]
const option = computed(() => ({
  tooltip: { trigger: 'axis', valueFormatter: gb },
  grid: { left: 8, right: 16, top: 16, bottom: 8, containLabel: true },
  xAxis: { type: 'category', data: hours, boundaryGap: kind.value === 'bar' },
  yAxis: { type: 'value', axisLabel: { formatter: gb } },
  series: [{ name: '流量', type: kind.value, smooth: true, showSymbol: false, areaStyle: kind.value === 'line' ? { opacity: 0.12 } : undefined, data: traffic }]
}))
const latency = {
  tooltip: { trigger: 'axis' },
  legend: { top: 0 },
  grid: { left: 8, right: 16, top: 36, bottom: 8, containLabel: true },
  xAxis: { type: 'category', data: hours, boundaryGap: false },
  yAxis: { type: 'value', axisLabel: { formatter: '{value} ms' } },
  series: ['平均', 'P95', '最大'].map((name, line) => ({
    name,
    type: 'line',
    smooth: true,
    showSymbol: false,
    data: hours.map((_, index) => Math.round(20 + line * 12 + Math.sin(index / 2 + line) * 6))
  }))
}
const table = {
  columns: [{ key: 'hour', label: '时间' }, { key: 'traffic', label: '流量', numeric: true, format: gb }],
  rows: hours.map((hour, index) => ({ hour, traffic: traffic[index] }))
}

const loading = ref(false)
function reload() {
  loading.value = true
  setTimeout(() => { loading.value = false }, 1500)
}
</script>

<template>
  <Story title="Chart" group="display" :layout="{ type: 'single', iframe: true }">
    <Variant title="Series and theme">
      <div class="story">
        <UiSegmentedControl v-model="kind" :options="kinds" aria-label="图表类型" />
        <UiCard title="24 小时流量" description="颜色来自 --chart-* 变量，切换深色模式时图表跟着变。">
          <UiChart :option="option" label="最近 24 小时的每小时流量" summary="共 34.2 GB，峰值 2.6 GB（14:00）" :table="table" :height="260" />
        </UiCard>
        <UiCard title="延迟趋势">
          <UiChart :option="latency" label="hk-01 最近 24 小时的延迟" :height="260" />
        </UiCard>
      </div>
    </Variant>
    <Variant title="States">
      <div class="story">
        <UiButton @click="reload">重新加载（1.5 s）</UiButton>
        <UiCard title="加载中（300 ms 后显示骨架）">
          <UiChart :option="loading ? null : option" label="流量" :loading="loading" :height="200" />
        </UiCard>
        <UiCard title="空">
          <UiChart :option="null" label="流量" empty empty-title="所选区间暂无流量数据" empty-description="节点上报流量后，这里会按小时显示。" :height="200" />
        </UiCard>
        <UiCard title="错误">
          <UiChart :option="null" label="流量" error="网关超时（HTTP 504）" error-title="无法加载流量" :height="200" />
        </UiCard>
      </div>
    </Variant>
  </Story>
</template>

<docs lang="md">
# Chart

An ECharts wrapper. ECharts loads on first use (tree-shaken: line and bar
series, grid, tooltip, legend, canvas renderer) and never in the entry
chunk. Pass a plain `option` without colours: the theme comes from the
`--chart-*` and label/separator tokens and follows light / dark live. It
resizes with its box (ResizeObserver).

- `label` (required) and `summary` name the plot (`role="img"`).
- `table` (`{ columns, rows }`) adds a data table: read by screen readers,
  shown on "以表格查看".
- States: `loading` (chart skeleton after 300 ms), `empty` (+ `emptyTitle`,
  `emptyDescription`), `error` (+ `errorTitle`, `retry` event).
</docs>
