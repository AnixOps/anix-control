<template>
  <div class="topology-graph" data-topology-graph>
    <div class="topology-graph__plot" :style="{ height: cssHeight }">
      <div
        ref="containerEl"
        class="topology-graph__canvas"
        :class="{ 'is-hidden': state }"
        role="img"
        :aria-label="accessibleLabel"
        data-topology-canvas
      />
      <div v-if="state" class="topology-graph__state">
        <UiErrorState v-if="state === 'error'" compact :title="errorTitle" :error="engineError" heading-tag="h3" @retry="retry" />
        <UiSkeleton v-else-if="state === 'skeleton'" variant="chart" :height="height" :label="label" />
        <UiEmptyState v-else-if="state === 'empty'" compact :icon="Waypoints" :title="emptyTitle" :description="emptyDescription" heading-tag="h3" />
      </div>
    </div>
    <ul v-if="legend.length && !state" class="topology-graph__legend" :aria-label="legendLabel || undefined">
      <li v-for="item in legend" :key="item.key">
        <span class="topology-graph__swatch" :style="{ background: colorOf(item.key) }" aria-hidden="true" />
        {{ item.label }}
      </li>
    </ul>
    <!-- The graph for screen readers: each link as text. -->
    <ul v-if="!state" class="visually-hidden" data-topology-text>
      <li v-for="edge in edges" :key="edge.id">{{ nodeLabel(edge.source) }} → {{ nodeLabel(edge.target) }}<template v-if="edge.label">（{{ edge.label }}）</template></li>
      <li v-for="node in isolatedNodes" :key="node.id">{{ node.label || node.id }}</li>
    </ul>
  </div>
</template>

<script setup>
// A read-only G6 topology (UI U8), used by 流量与监控 (forward topology)
// and 部署编排 (a topology revision's graph). G6 loads on first use (the
// `g6` chunk). Colours come from the chart tokens (useChartTheme's
// graphColors): node `tone` names a role (relay, exit, node, offline) or a
// palette slot (`chart-3`); labels and edges use the label and separator
// roles. The graph redraws when <html data-theme> changes, so it follows
// light / dark live, and resizes with its box. role="img" names it with a
// node / link count; a hidden list reads every link.
//   nodes: [{ id, label, tone, detail }]   edges: [{ id, source, target, label }]
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Waypoints } from '@lucide/vue'
import { buildGraphColors, readChartTokens, watchDocumentTheme } from '@/composables/useChartTheme'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'

const props = defineProps({
  nodes: { type: Array, default: () => [] },
  edges: { type: Array, default: () => [] },
  label: { type: String, required: true },
  summary: { type: String, default: '' },
  height: { type: [Number, String], default: 420 },
  loading: { type: Boolean, default: false },
  emptyTitle: { type: String, default: '' },
  emptyDescription: { type: String, default: '' },
  errorTitle: { type: String, default: '' },
  // [{ key: tone, label }]
  legend: { type: Array, default: () => [] },
  legendLabel: { type: String, default: '' },
  layout: { type: String, default: 'force', validator: value => ['force', 'dagre'].includes(value) }
})

const containerEl = ref(null)
const engineError = ref(null)
const colors = ref(buildGraphColors(readChartTokens()))
let graph = null
let renderSeq = 0
let resizeObserver = null
let stopThemeWatch = null
let disposed = false

const cssHeight = computed(() => (typeof props.height === 'number' ? `${props.height}px` : props.height))
const showSkeleton = useDelayedLoading(() => props.loading)
const state = computed(() => {
  if (engineError.value) return 'error'
  if (props.loading) return showSkeleton.value ? 'skeleton' : 'pending'
  if (!props.nodes.length) return 'empty'
  return ''
})
const accessibleLabel = computed(() => props.summary ? `${props.label}: ${props.summary}` : props.label)
const nodeNames = computed(() => new Map(props.nodes.map(node => [String(node.id), node.label || String(node.id)])))
const isolatedNodes = computed(() => {
  const linked = new Set(props.edges.flatMap(edge => [String(edge.source), String(edge.target)]))
  return props.nodes.filter(node => !linked.has(String(node.id)))
})

function nodeLabel(id) {
  return nodeNames.value.get(String(id)) || String(id)
}

function colorOf(tone) {
  const palette = colors.value
  const slot = /^chart-(\d)$/.exec(String(tone || ''))
  if (slot) return palette.palette?.[Number(slot[1]) - 1] || palette.node
  return palette[tone] || palette.node
}

function graphData() {
  const ids = new Set(props.nodes.map(node => String(node.id)))
  return {
    nodes: props.nodes.map(node => ({
      id: String(node.id),
      data: { ...node },
      style: {
        labelText: node.detail ? `${node.label || node.id}\n${node.detail}` : String(node.label || node.id),
        fill: colorOf(node.tone)
      }
    })),
    edges: props.edges
      .filter(edge => ids.has(String(edge.source)) && ids.has(String(edge.target)))
      .map((edge, index) => ({
        id: String(edge.id ?? `edge-${index}`),
        source: String(edge.source),
        target: String(edge.target),
        style: { labelText: edge.label || '' }
      }))
  }
}

function destroyGraph() {
  if (graph) {
    graph.destroy()
    graph = null
  }
}

async function render() {
  const seq = ++renderSeq
  if (disposed || !props.nodes.length || props.loading) {
    destroyGraph()
    return
  }
  try {
    const { Graph } = await import('@antv/g6')
    if (disposed || seq !== renderSeq || !containerEl.value) return
    destroyGraph()
    colors.value = buildGraphColors(readChartTokens())
    const palette = colors.value
    graph = new Graph({
      container: containerEl.value,
      data: graphData(),
      autoFit: 'view',
      layout: props.layout === 'dagre'
        ? { type: 'antv-dagre', rankdir: 'LR', nodesep: 32, ranksep: 72 }
        : { type: 'force', preventOverlap: true, linkDistance: 160 },
      node: {
        style: {
          size: 36,
          stroke: palette.surface,
          lineWidth: 2,
          labelFill: palette.label,
          labelPlacement: 'bottom',
          labelFontSize: 12
        }
      },
      edge: {
        style: {
          endArrow: true,
          stroke: palette.edge,
          labelFill: palette.edgeLabel,
          labelFontSize: 11
        }
      },
      behaviors: ['drag-canvas', 'zoom-canvas', 'drag-element']
    })
    await graph.render()
    // G6 gives its canvases tabindex="1" (for keyboard behaviours this graph
    // does not use), which breaks the page's tab order; the role="img" box
    // and the hidden link list are what assistive tech reads.
    for (const canvas of containerEl.value?.querySelectorAll('canvas[tabindex]') || []) {
      canvas.setAttribute('tabindex', '-1')
    }
    engineError.value = null
  } catch (cause) {
    if (seq === renderSeq) engineError.value = cause instanceof Error ? cause : new Error(String(cause))
  }
}

function retry() {
  engineError.value = null
  void render()
}

function resize() {
  const element = containerEl.value
  if (!graph || !element) return
  const { clientWidth, clientHeight } = element
  if (clientWidth && clientHeight) {
    graph.resize(clientWidth, clientHeight)
    void graph.fitView?.()
  }
}

watch(() => [props.nodes, props.edges, props.loading, props.layout], () => { void render() })

onMounted(() => {
  void render()
  stopThemeWatch = watchDocumentTheme(() => { void render() })
  if (typeof ResizeObserver === 'function' && containerEl.value) {
    resizeObserver = new ResizeObserver(() => resize())
    resizeObserver.observe(containerEl.value)
  }
})

onBeforeUnmount(() => {
  disposed = true
  stopThemeWatch?.()
  resizeObserver?.disconnect()
  destroyGraph()
})
</script>

<style scoped>
.topology-graph {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}

.topology-graph__plot {
  position: relative;
  min-width: 0;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: var(--bg-grouped);
}

.topology-graph__canvas {
  width: 100%;
  height: 100%;
}

.topology-graph__canvas.is-hidden {
  visibility: hidden;
}

.topology-graph__state {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.topology-graph__state > * {
  width: 100%;
}

.topology-graph__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  margin: 0;
  padding: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  list-style: none;
}

.topology-graph__legend li {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
}

.topology-graph__swatch {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}
</style>
