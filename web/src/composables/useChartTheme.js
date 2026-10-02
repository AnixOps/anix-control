import { watch } from 'vue'
import { useTheme } from '@/composables/useTheme'

// Chart colours come from the AnixOps Design tokens (src/design/tokens.css):
// the 8-colour --chart-* palette and the neutral label/separator roles.
// ECharts and G6 draw on canvas and cannot read CSS variables, so the values
// are resolved from the document for the active theme and turned into a
// theme object; charts re-theme when useTheme switches.

const CHART_TOKENS = [
  'chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5', 'chart-6', 'chart-7', 'chart-8',
  'label-1', 'label-2', 'label-3', 'separator', 'separator-strong', 'bg-elevated', 'fill-1', 'font-sans'
]

// readChartTokens returns { 'chart-1': '#4F5BE8', ... } for the active theme.
// Missing values (no stylesheet, e.g. unit tests) are left out.
export function readChartTokens(root = typeof document === 'undefined' ? null : document.documentElement) {
  if (!root || typeof getComputedStyle !== 'function') return {}
  const style = getComputedStyle(root)
  const tokens = {}
  for (const name of CHART_TOKENS) {
    const value = style.getPropertyValue(`--${name}`).trim()
    if (value) tokens[name] = value
  }
  return tokens
}

function axis(tokens) {
  return {
    axisLine: { show: true, lineStyle: { color: tokens['separator-strong'] } },
    axisTick: { show: false, lineStyle: { color: tokens['separator-strong'] } },
    axisLabel: { color: tokens['label-2'] },
    splitLine: { lineStyle: { color: tokens.separator } },
    splitArea: { show: false },
    nameTextStyle: { color: tokens['label-2'] }
  }
}

// buildEChartsTheme maps tokens to an ECharts theme object.
export function buildEChartsTheme(tokens) {
  const palette = Array.from({ length: 8 }, (_, index) => tokens[`chart-${index + 1}`]).filter(Boolean)
  const text = { color: tokens['label-2'], fontFamily: tokens['font-sans'] }
  return {
    ...(palette.length === 8 ? { color: palette } : {}),
    backgroundColor: 'transparent',
    textStyle: text,
    title: { textStyle: { color: tokens['label-1'] }, subtextStyle: { color: tokens['label-2'] } },
    legend: { textStyle: { color: tokens['label-2'] }, inactiveColor: tokens['label-3'] },
    tooltip: {
      backgroundColor: tokens['bg-elevated'],
      borderColor: tokens.separator,
      borderWidth: 1,
      textStyle: { color: tokens['label-1'] },
      axisPointer: { lineStyle: { color: tokens['separator-strong'] }, crossStyle: { color: tokens['separator-strong'] } }
    },
    categoryAxis: axis(tokens),
    valueAxis: { ...axis(tokens), axisLine: { show: false } },
    timeAxis: axis(tokens),
    logAxis: { ...axis(tokens), axisLine: { show: false } },
    line: { lineStyle: { width: 2 }, symbolSize: 6 },
    dataZoom: { textStyle: { color: tokens['label-2'] } }
  }
}

// buildGraphColors gives G6 topology colours: node kinds use the chart
// palette, offline nodes the neutral chart-8, labels and edges the label and
// separator roles.
export function buildGraphColors(tokens) {
  return {
    relay: tokens['chart-1'],
    exit: tokens['chart-7'],
    node: tokens['chart-5'],
    offline: tokens['chart-8'],
    label: tokens['label-1'],
    edge: tokens['separator-strong'],
    edgeLabel: tokens['label-2'],
    // The node ring: the elevated surface the graph is drawn on.
    surface: tokens['bg-elevated'],
    // The whole palette, for graphs that colour nodes by an arbitrary kind.
    palette: Array.from({ length: 8 }, (_, index) => tokens[`chart-${index + 1}`]).filter(Boolean)
  }
}

function documentMode(fallback) {
  if (typeof document === 'undefined') return fallback
  return document.documentElement.getAttribute('data-theme') || fallback
}

// watchDocumentTheme runs callback whenever <html data-theme> changes: the
// app's theme toggle and Histoire's dark-mode switch both write it, and the
// tokens a canvas chart reads follow it. Returns a stop function; call it
// on unmount.
export function watchDocumentTheme(callback) {
  if (typeof MutationObserver !== 'function' || typeof document === 'undefined') return () => {}
  const observer = new MutationObserver(() => callback(documentMode('light')))
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  return () => observer.disconnect()
}

// useChartTheme: themeFor(echarts) registers the theme for the active mode
// and returns its name for echarts.init / chart.setTheme; onThemeChange runs
// a callback after the theme switches. Call it during component setup.
export function useChartTheme() {
  const { currentTheme } = useTheme()

  function themeFor(echarts) {
    const name = `anixops-${documentMode(currentTheme.value)}`
    echarts.registerTheme?.(name, buildEChartsTheme(readChartTokens()))
    return name
  }

  function onThemeChange(callback) {
    return watch(currentTheme, () => callback(currentTheme.value))
  }

  return { currentTheme, themeFor, onThemeChange, graphColors: () => buildGraphColors(readChartTokens()) }
}
