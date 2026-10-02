<template>
  <div class="ui-chart" :class="{ 'is-table-view': tableView }" :aria-busy="loading ? 'true' : undefined" data-chart>
    <div class="ui-chart__plot" :style="{ height: cssHeight }">
      <div
        ref="canvasEl"
        class="ui-chart__canvas"
        :class="{ 'is-hidden': !showPlot }"
        role="img"
        :aria-label="accessibleLabel"
        :aria-hidden="showPlot ? undefined : 'true'"
        data-chart-canvas
      />
      <div v-if="state" class="ui-chart__state">
        <UiErrorState
          v-if="state === 'error'"
          compact
          :title="errorTitle || t('ui.chart.loadFailed')"
          :error="shownError"
          :heading-tag="stateHeadingTag"
          @retry="retry"
        />
        <UiSkeleton v-else-if="state === 'skeleton'" variant="chart" :height="height" :label="t('ui.chart.loading', { label })" />
        <UiEmptyState
          v-else-if="state === 'empty'"
          compact
          :icon="emptyIcon || ChartNoAxesColumn"
          :title="emptyTitle || t('ui.chart.empty')"
          :description="emptyDescription"
          :heading-tag="stateHeadingTag"
        />
      </div>
    </div>

    <template v-if="hasTable">
      <div
        class="ui-chart__table-wrap"
        :class="tableView ? 'is-open' : 'visually-hidden'"
        :role="tableView ? 'region' : undefined"
        :aria-label="tableView ? label : undefined"
        :tabindex="tableView ? 0 : undefined"
        data-chart-table
      >
        <table class="ui-chart__table">
          <caption class="visually-hidden">{{ label }}</caption>
          <thead>
            <tr>
              <th v-for="column in table.columns" :key="column.key" scope="col" :class="{ 'is-numeric': column.numeric }">{{ column.label }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, index) in table.rows" :key="index">
              <td v-for="column in table.columns" :key="column.key" :class="{ 'is-numeric': column.numeric }">{{ tableCell(row, column) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="tableToggle" class="ui-chart__footer">
        <UiButton
          variant="tertiary"
          size="sm"
          :icon="tableView ? ChartLine : Table2"
          :aria-pressed="tableView ? 'true' : 'false'"
          data-chart-table-toggle
          @click="tableView = !tableView"
        >
          {{ tableView ? t('ui.chart.showChart') : t('ui.chart.showTable') }}
        </UiButton>
      </div>
    </template>
  </div>
</template>

<script setup>
// Chart (plan §6): a thin ECharts wrapper the pages draw every chart with.
//
//   <UiChart :option="option" label="24 小时流量" :summary="…" :table="{ columns, rows }"
//            :loading :error :empty @retry />
//
// - ECharts loads on first use from ./internal/echarts.js (tree-shaken:
//   line and bar series, grid, tooltip, legend, canvas), never in the entry
//   chunk; `option` is a plain ECharts option without colours, which come
//   from the AnixOps Design theme (useChartTheme) and follow light / dark
//   live (chart.setTheme), like the tokens do.
// - Resizes with its box (ResizeObserver), not only with the window.
// - States: `error` (UiErrorState with 重试 → `retry`), `loading` (a chart
//   skeleton after 300 ms; with a chart on screen it stays and dims),
//   `empty` (UiEmptyState: emptyTitle / emptyDescription). A failure to
//   load or start ECharts shows the error state too.
// - Accessible: the plot is role="img" named by `label` and `summary`;
//   `table` ({ columns: [{ key, label, numeric, format }], rows }) adds a
//   data table, hidden until "以表格查看" (plan §11), always read by
//   screen readers.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChartLine, ChartNoAxesColumn, Table2 } from '@lucide/vue'
import { useChartTheme, watchDocumentTheme } from '@/composables/useChartTheme'
import UiButton from './UiButton.vue'
import UiEmptyState from './UiEmptyState.vue'
import UiErrorState from './UiErrorState.vue'
import UiSkeleton from './UiSkeleton.vue'
import { useDelayedLoading } from './composables/useDelayedLoading'

const props = defineProps({
  option: { type: Object, default: null },
  // Names the chart (role="img" and the table caption).
  label: { type: String, required: true },
  // One sentence for screen readers: what the chart shows (range, peak…).
  summary: { type: String, default: '' },
  height: { type: [Number, String], default: 280 },
  loading: { type: Boolean, default: false },
  error: { type: [Object, String, null], default: null },
  errorTitle: { type: String, default: '' },
  empty: { type: Boolean, default: false },
  emptyTitle: { type: String, default: '' },
  emptyDescription: { type: String, default: '' },
  emptyIcon: { type: [Object, Function], default: null },
  stateHeadingTag: { type: String, default: 'h3' },
  table: { type: Object, default: null },
  tableToggle: { type: Boolean, default: true }
})

const emit = defineEmits(['retry'])
const { t } = useI18n()
const { themeFor } = useChartTheme()

const canvasEl = ref(null)
const chart = shallowRef(null)
const engineError = ref(null)
const tableView = ref(false)
let engine = null
let resizeObserver = null
let stopThemeWatch = null
let disposed = false

const cssHeight = computed(() => (typeof props.height === 'number' ? `${props.height}px` : props.height))
const shownError = computed(() => props.error || engineError.value)
const showSkeleton = useDelayedLoading(() => props.loading && !chart.value)
// What covers the plot: an error, the skeleton (first load), or empty.
const state = computed(() => {
  if (shownError.value) return 'error'
  if (props.loading && !chart.value) return showSkeleton.value ? 'skeleton' : 'pending'
  if (props.empty) return 'empty'
  return ''
})
const showPlot = computed(() => !state.value && Boolean(props.option))
const accessibleLabel = computed(() => (props.summary ? `${props.label}: ${props.summary}` : props.label))
const hasTable = computed(() => Boolean(props.table?.columns?.length && props.table?.rows?.length) && !state.value)

function tableCell(row, column) {
  const value = row?.[column.key]
  const text = column.format ? column.format(value, row) : value
  return text === null || text === undefined || text === '' ? '—' : text
}

async function loadEngine() {
  if (!engine) engine = await import('./internal/echarts.js')
  return engine
}

async function render() {
  if (disposed || !props.option || props.empty || props.error) return
  try {
    const echarts = await loadEngine()
    if (disposed || !canvasEl.value) return
    if (!chart.value) {
      chart.value = echarts.init(canvasEl.value, themeFor(echarts))
    }
    chart.value.setOption(props.option, { notMerge: true })
    engineError.value = null
  } catch (cause) {
    engineError.value = cause instanceof Error ? cause : new Error(String(cause))
  }
}

function retry() {
  if (engineError.value) {
    engineError.value = null
    void render()
  }
  emit('retry')
}

watch(() => [props.option, props.empty, props.error], () => { void render() })

onMounted(() => {
  void render()
  // Re-theme in place when <html data-theme> changes (light ↔ dark).
  stopThemeWatch = watchDocumentTheme(() => {
    if (chart.value && engine) chart.value.setTheme(themeFor(engine))
  })
  if (typeof ResizeObserver === 'function' && canvasEl.value) {
    resizeObserver = new ResizeObserver(() => chart.value?.resize())
    resizeObserver.observe(canvasEl.value)
  }
})

onBeforeUnmount(() => {
  disposed = true
  stopThemeWatch?.()
  resizeObserver?.disconnect()
  resizeObserver = null
  chart.value?.dispose()
  chart.value = null
})

defineExpose({ chart })
</script>

<style scoped>
.ui-chart {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.ui-chart__plot {
  position: relative;
  min-width: 0;
}

.ui-chart__canvas {
  width: 100%;
  height: 100%;
  transition: opacity var(--dur-toggle) var(--ease-standard);
}

.ui-chart__canvas.is-hidden {
  visibility: hidden;
}

.ui-chart[aria-busy='true'] .ui-chart__canvas {
  opacity: 0.55;
}

.ui-chart__state {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ui-chart__state > * {
  width: 100%;
}

.is-table-view .ui-chart__plot {
  display: none;
}

.ui-chart__table-wrap.is-open {
  max-height: 320px;
  overflow: auto;
  border-radius: var(--radius-sm);
  box-shadow: 0 0 0 0.5px var(--separator);
}

.ui-chart__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--type-callout-size);
}

.ui-chart__table th,
.ui-chart__table td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--separator);
  text-align: left;
}

.ui-chart__table th {
  position: sticky;
  top: 0;
  background: var(--bg-elevated);
  color: var(--label-2);
  font-weight: var(--weight-semibold);
}

.ui-chart__table .is-numeric {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.ui-chart__table tr:last-child td {
  border-bottom: 0;
}

.ui-chart__footer {
  display: flex;
  justify-content: flex-end;
}
</style>
