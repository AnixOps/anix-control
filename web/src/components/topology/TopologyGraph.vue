<template>
  <div ref="frame" class="topo-graph" data-testid="topology-graph" @click="emit('select', '')">
    <div v-if="wired.length" class="topo-graph__stage" :style="{ width: `${layout.width}px`, height: `${layout.height}px` }">
      <svg class="topo-graph__links" :width="layout.width" :height="layout.height" aria-hidden="true">
        <g v-for="edge in edges" :key="edge.key" class="topo-edge" :class="[`is-${edge.tone}`, `is-${edge.kind}`, { 'is-dim': focus && !focus.links.has(edge.key) }]">
          <path :d="edge.d" :stroke-width="edge.width" />
          <path v-if="edge.tone === 'ok'" class="topo-edge__flow" :d="edge.d" :stroke-width="edge.width" />
          <circle :cx="edge.x1" :cy="edge.y1" r="3" />
          <circle :cx="edge.x2" :cy="edge.y2" r="3" />
        </g>
      </svg>

      <p v-for="column in layout.columns" :key="column.x" class="topo-graph__head" :style="{ left: `${column.x}px`, width: `${column.width}px` }">
        {{ t(`topology.roles.${column.role}`) }}
        <span class="topo-graph__count">{{ column.count }}</span>
      </p>

      <button
        v-for="node in wired"
        :key="node.ref"
        type="button"
        class="topo-node is-placed"
        :class="nodeClass(node)"
        :style="{ left: `${layout.at.get(node.ref).x}px`, top: `${layout.at.get(node.ref).y}px` }"
        :aria-pressed="selected === node.ref"
        @click.stop="emit('select', node.ref)"
        @pointerenter="emit('hover', node.ref)"
        @pointerleave="emit('hover', null)"
      >
        <span class="topo-node__dot" aria-hidden="true" />
        <span class="topo-node__text">
          <span class="topo-node__name">{{ node.name }}</span>
          <span class="topo-node__meta">{{ node.meta }}</span>
        </span>
      </button>
    </div>

    <!-- The devices no line uses: users reach them directly. -->
    <div v-if="loose.length" class="topo-graph__loose">
      <p class="topo-graph__head is-inline">
        {{ t('topology.roles.direct') }}
        <span class="topo-graph__count">{{ loose.length }}</span>
      </p>
      <div class="topo-graph__grid">
        <button
          v-for="node in loose"
          :key="node.ref"
          type="button"
          class="topo-node"
          :class="nodeClass(node)"
          :aria-pressed="selected === node.ref"
          @click.stop="emit('select', node.ref)"
        >
          <span class="topo-node__dot" aria-hidden="true" />
          <span class="topo-node__text">
            <span class="topo-node__name">{{ node.name }}</span>
            <span class="topo-node__meta">{{ node.meta }}</span>
          </span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
// 抽象拓扑: the devices of the lines in columns from entry to exit
// (layoutGraph), each link a curve from the device that sends to the one
// that receives; an egress route (分流) between two exits bends around
// their column. The devices no line uses are listed under them.
import { computed, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { CARD, layoutGraph } from './topologyModel'
import { useElementSize } from './useElementSize'

const ROLE_ORDER = ['entry', 'relay', 'exit', 'direct']

const props = defineProps({
  devices: { type: Array, required: true },
  links: { type: Array, required: true },
  focus: { type: Object, default: null },
  selected: { type: String, default: '' }
})
const emit = defineEmits(['select', 'hover'])
const { t, locale } = useAppI18n()

const frame = ref(null)
const size = useElementSize(frame)

const nodes = computed(() => props.devices.map(device => {
  const place = device.place?.[locale.value === 'zh-CN' ? 'zh' : 'en']
  const state = device.users ? t('topology.device.users', { n: device.users }) : t(`topology.status.${device.status}`)
  return { ref: device.ref, name: device.name, status: device.status, wired: device.lines.length > 0, role: ROLE_ORDER.find(role => device.roles.includes(role)), meta: place ? `${place} · ${state}` : state }
}))
const wired = computed(() => nodes.value.filter(node => node.wired))
const loose = computed(() => nodes.value.filter(node => !node.wired))
const layout = computed(() => layoutGraph({ devices: props.devices.filter(device => device.lines.length), links: props.links }, size.width))
const nodeClass = node => [`is-${node.role}`, `is-${node.status}`, { 'is-dim': props.focus && !props.focus.devices.has(node.ref), 'is-selected': props.selected === node.ref }]

const edges = computed(() => props.links.map(link => {
  const a = layout.value.at.get(link.from)
  const b = layout.value.at.get(link.to)
  const y1 = a.y + CARD.height / 2
  const y2 = b.y + CARD.height / 2
  // Forward: right edge to left edge. Backward: left edge to right edge.
  // Inside one column: around its right side.
  const side = Math.sign(b.x - a.x)
  const x1 = side < 0 ? a.x : a.x + CARD.width
  const x2 = side > 0 ? b.x : b.x + CARD.width
  const bend = side ? (x2 - x1) / 2 : 48
  const d = `M${x1} ${y1}C${x1 + bend} ${y1} ${side ? x2 - bend : x2 + bend} ${y2} ${x2} ${y2}`
  return { key: link.key, kind: link.kind, tone: link.tone, d, x1, y1, x2, y2, width: 1.5 + Math.min(link.lines.length, 4) * 0.5 }
}))
</script>

<style scoped>
.topo-graph {
  display: grid;
  gap: var(--space-8);
  align-content: start;
  padding: var(--space-6);
  overflow: auto;
  border-radius: var(--radius-md);
  background:
    radial-gradient(60% 80% at 30% 20%, var(--accent-soft), transparent 70%),
    var(--bg-elevated);
}

.topo-graph__stage {
  position: relative;
  margin: 0 auto;
}

.topo-graph__loose {
  display: grid;
  gap: var(--space-3);
}

.topo-graph__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(176px, 1fr));
  gap: var(--space-3);
}

.topo-graph__links {
  position: absolute;
  inset: 0;
  overflow: visible;
}

.topo-edge path {
  fill: none;
  stroke: var(--accent);
  stroke-linecap: round;
  opacity: 0.7;
}

.topo-edge circle {
  fill: var(--bg-elevated);
  stroke: var(--accent);
  stroke-width: 1.5;
}

.topo-edge.is-egress path,
.topo-edge.is-egress circle {
  stroke: var(--chart-3);
}

.topo-edge.is-egress path {
  stroke-dasharray: 5 4;
}

.topo-edge.is-warn path,
.topo-edge.is-warn circle {
  stroke: var(--warning);
}

.topo-edge.is-bad path,
.topo-edge.is-bad circle {
  stroke: var(--danger);
}

.topo-edge.is-off path,
.topo-edge.is-off circle {
  stroke: var(--label-3);
}

.topo-edge.is-off path {
  stroke-dasharray: 3 4;
}

.topo-edge .topo-edge__flow {
  stroke: var(--bg-elevated);
  stroke-dasharray: 0.1 28;
  opacity: 1;
  animation: topo-edge-flow 1.6s linear infinite;
}

.topo-edge {
  transition: opacity var(--dur-toggle) var(--ease-standard);
}

.topo-edge.is-dim {
  opacity: 0.12;
}

@keyframes topo-edge-flow {
  to { stroke-dashoffset: -28.1; }
}

.topo-graph__head {
  position: absolute;
  top: 0;
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.topo-graph__head.is-inline {
  position: static;
}

.topo-graph__count {
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--fill-2);
  color: var(--label-2);
  font-variant-numeric: tabular-nums;
}

.topo-node {
  --pin: var(--accent);

  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: flex-start;
  min-width: 0;
  height: 56px;
  padding: 0 var(--space-3);
  border: 0;
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator-strong), inset 3px 0 0 var(--pin);
  color: var(--label-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: opacity var(--dur-toggle) var(--ease-standard), box-shadow var(--dur-micro) var(--ease-standard), transform var(--dur-micro) var(--ease-standard);
}

.topo-node.is-placed {
  position: absolute;
  width: 176px;
}

.topo-node.is-entry {
  --pin: var(--chart-2);
}

.topo-node.is-relay {
  --pin: var(--chart-3);
}

.topo-node:hover,
.topo-node.is-selected {
  box-shadow: var(--shadow-2), 0 0 0 1.5px var(--pin), inset 3px 0 0 var(--pin);
  transform: translateY(-1px);
}

.topo-node:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.topo-node.is-dim {
  opacity: 0.3;
}

.topo-node__dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--label-3);
}

.topo-node.is-online .topo-node__dot {
  background: var(--success);
  box-shadow: 0 0 0 3px var(--success-soft);
}

.topo-node.is-offline .topo-node__dot {
  background: var(--danger);
  box-shadow: 0 0 0 3px var(--danger-soft);
}

.topo-node__text {
  display: grid;
  min-width: 0;
}

.topo-node__name {
  overflow: hidden;
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topo-node__meta {
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (prefers-reduced-motion: reduce) {
  .topo-edge .topo-edge__flow {
    display: none;
  }
}
</style>
