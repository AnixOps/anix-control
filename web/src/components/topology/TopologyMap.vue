<template>
  <div ref="frame" class="topo-map" data-testid="topology-map">
    <svg
      class="topo-map__svg"
      :class="{ 'is-dragging': drag }"
      :viewBox="`${view.x} ${view.y} ${view.w} ${viewHeight}`"
      role="group"
      :aria-label="label"
      @pointerdown="startDrag"
      @pointermove="moveDrag"
      @pointerup="endDrag"
      @pointercancel="drag = null"
      @wheel="onWheel"
    >
      <defs>
        <linearGradient v-for="arc in arcs" :id="`${uid}-${arc.index}`" :key="arc.key" gradientUnits="userSpaceOnUse" :x1="arc.a.x" :y1="arc.a.y" :x2="arc.b.x" :y2="arc.b.y">
          <stop offset="0" stop-color="var(--brand-gradient-start)" />
          <stop offset="1" stop-color="var(--brand-gradient-end)" />
        </linearGradient>
      </defs>
      <path class="topo-map__land" :d="LAND" :stroke-width="Math.max(1.5 * unit, 1.3)" />

      <g v-for="arc in arcs" :key="arc.key" class="topo-arc" :class="[`is-${arc.tone}`, `is-${arc.kind}`, { 'is-dim': focus && !focus.links.has(arc.key) }]">
        <path :d="arc.d" :style="arc.tone === 'ok' && arc.kind === 'route' ? { stroke: `url(#${uid}-${arc.index})` } : null" :stroke-width="arc.width * unit" />
        <path v-if="arc.tone === 'ok'" class="topo-arc__flow" :d="arc.d" :stroke-width="arc.width * unit" :stroke-dasharray="`${0.1 * unit} ${28 * unit}`" :style="{ '--flow': `${-28.1 * unit}px` }" />
      </g>

      <g
        v-for="pin in pins"
        :key="pin.ref"
        class="topo-pin"
        :class="[`is-${pin.role}`, `is-${pin.status}`, { 'is-dim': focus && !focus.devices.has(pin.ref), 'is-selected': selected === pin.ref }]"
        role="button"
        tabindex="0"
        :aria-label="pin.name"
        :aria-pressed="selected === pin.ref"
        @pointerdown.stop
        @click.stop="emit('select', pin.ref)"
        @keydown.enter.prevent="emit('select', pin.ref)"
        @pointerenter="emit('hover', pin.ref)"
        @pointerleave="emit('hover', null)"
      >
        <circle class="topo-pin__halo" :cx="pin.x" :cy="pin.y" :r="11 * unit" />
        <circle class="topo-pin__core" :cx="pin.x" :cy="pin.y" :r="4.5 * unit" :stroke-width="1.5 * unit" />
        <text
          v-if="named.has(pin.ref)"
          class="topo-pin__name"
          :x="pin.x + (pin.left ? -9 : 9) * unit"
          :y="pin.y + 4 * unit"
          :text-anchor="pin.left ? 'end' : 'start'"
          :font-size="11.5 * unit"
          :stroke-width="3 * unit"
        >{{ pin.name }}</text>
      </g>
    </svg>

    <div class="topo-map__tools">
      <UiIconButton :icon="Plus" size="sm" :label="t('topology.map.zoomIn')" @click="zoom(0.6)" />
      <UiIconButton :icon="Minus" size="sm" :label="t('topology.map.zoomOut')" @click="zoom(1 / 0.6)" />
      <UiIconButton :icon="Locate" size="sm" :label="t('topology.map.fit')" @click="fit" />
      <UiIconButton :icon="Globe" size="sm" :label="t('topology.map.world')" @click="world" />
    </div>
  </div>
</template>

<script setup>
// 全球地图: the devices on a dotted world map, each link an arc from the
// device that sends to the one that receives. Devices of one place stand in
// a ring around it. Drag to move, ⌘/Ctrl + wheel (or a pinch) to zoom; the
// map opens fitted to the devices.
import { computed, reactive, ref, useId, watch } from 'vue'
import { Globe, Locate, Minus, Plus } from '@lucide/vue'
import { UiIconButton } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { MAP_HEIGHT, MAP_WIDTH, project } from './geo'
import { useElementSize } from './useElementSize'
import { worldDotsPath } from './worldDots'

const LAND = worldDotsPath()
const MIN_WIDTH = 60
const ROLE_ORDER = ['entry', 'relay', 'exit', 'direct']

const props = defineProps({
  devices: { type: Array, required: true },
  links: { type: Array, required: true },
  // { devices: Set, links: Set } to light up, or null.
  focus: { type: Object, default: null },
  selected: { type: String, default: '' },
  label: { type: String, required: true }
})
const emit = defineEmits(['select', 'hover'])
const { t } = useAppI18n()
const uid = useId()

const frame = ref(null)
const measured = useElementSize(frame)
// Before the first measure: the size the map opens at on a desktop.
const size = computed(() => (measured.width ? measured : { width: 900, height: 430 }))
const view = reactive({ x: 0, y: 0, w: MAP_WIDTH })
const viewHeight = computed(() => view.w * size.value.height / size.value.width)
// Map units a screen pixel covers: marks keep their size at every zoom.
const unit = computed(() => view.w / size.value.width)

const places = computed(() => {
  const out = new Map()
  for (const device of props.devices) {
    if (!device.place) continue
    const group = out.get(device.place.key) || { ...project(device.place.lat, device.place.lon), devices: [] }
    group.devices.push(device)
    out.set(device.place.key, group)
  }
  return [...out.values()]
})

const pins = computed(() => places.value.flatMap(place => place.devices.map((device, index) => {
  const count = place.devices.length
  const angle = (index / count) * 2 * Math.PI - Math.PI / 2
  const radius = count > 1 ? (8 + 2.5 * count) * unit.value : 0
  return {
    ref: device.ref,
    name: device.name,
    status: device.status,
    role: ROLE_ORDER.find(role => device.roles.includes(role)),
    x: place.x + Math.cos(angle) * radius,
    y: place.y + Math.sin(angle) * radius,
    left: count > 1 && Math.cos(angle) < -0.1
  }
})))

const arcs = computed(() => {
  const at = new Map(pins.value.map(pin => [pin.ref, pin]))
  return props.links.filter(link => at.has(link.from) && at.has(link.to)).map((link, index) => {
    const a = at.get(link.from)
    const b = at.get(link.to)
    const length = Math.hypot(b.x - a.x, b.y - a.y) || 1
    // Bows upward, a fifth of its length.
    const lift = Math.min(length * 0.22, 70) * (b.x >= a.x ? 1 : -1)
    const cx = (a.x + b.x) / 2 + (b.y - a.y) / length * lift
    const cy = (a.y + b.y) / 2 - (b.x - a.x) / length * lift
    return { key: link.key, index, kind: link.kind, tone: link.tone, a, b, d: `M${a.x} ${a.y}Q${cx} ${cy} ${b.x} ${b.y}`, width: 1.2 + Math.min(link.lines.length, 4) * 0.35 }
  })
})

// The names that fit: the selected and the lit devices first, then the
// rest, each shown unless it would cover a device or a name already placed.
const named = computed(() => {
  const lit = pin => (props.selected === pin.ref || props.focus?.devices.has(pin.ref) ? 0 : 1)
  const dot = 6 * unit.value
  const taken = pins.value.map(pin => ({ ref: pin.ref, x1: pin.x - dot, x2: pin.x + dot, y1: pin.y - dot, y2: pin.y + dot }))
  const out = new Set()
  for (const pin of [...pins.value].sort((a, b) => lit(a) - lit(b))) {
    const width = (pin.name.length * 6.6 + 12) * unit.value
    const box = { x1: pin.left ? pin.x - width : pin.x, x2: pin.left ? pin.x : pin.x + width, y1: pin.y - 8 * unit.value, y2: pin.y + 8 * unit.value }
    if (taken.some(other => other.ref !== pin.ref && box.x1 < other.x2 && other.x1 < box.x2 && box.y1 < other.y2 && other.y1 < box.y2)) continue
    taken.push(box)
    out.add(pin.ref)
  }
  return out
})

function show(x, y, w) {
  view.w = Math.min(MAP_WIDTH, Math.max(MIN_WIDTH, w))
  const h = view.w * size.value.height / size.value.width
  view.x = Math.min(MAP_WIDTH - view.w, Math.max(0, x - view.w / 2))
  view.y = h >= MAP_HEIGHT ? (MAP_HEIGHT - h) / 2 : Math.min(MAP_HEIGHT - h, Math.max(0, y - h / 2))
}

function world() {
  show(MAP_WIDTH / 2, MAP_HEIGHT / 2, MAP_WIDTH)
}

// fit shows every placed device with room around it.
function fit() {
  if (!places.value.length) return world()
  const xs = places.value.map(place => place.x)
  const ys = places.value.map(place => place.y)
  const [left, right, top, bottom] = [Math.min(...xs), Math.max(...xs), Math.min(...ys), Math.max(...ys)]
  const w = Math.max((right - left) * 1.35 + 60, (bottom - top) * 1.5 * size.value.width / size.value.height + 60, 240)
  show((left + right) / 2, (top + bottom) / 2, w)
}

function zoom(factor, cx = view.x + view.w / 2, cy = view.y + viewHeight.value / 2) {
  const w = Math.min(MAP_WIDTH, Math.max(MIN_WIDTH, view.w * factor))
  const k = w / view.w
  // The point under the cursor stays where it is.
  show(cx + (view.x + view.w / 2 - cx) * k, cy + (view.y + viewHeight.value / 2 - cy) * k, w)
}

function onWheel(event) {
  if (!event.ctrlKey && !event.metaKey) return
  event.preventDefault()
  const box = frame.value.getBoundingClientRect()
  zoom(event.deltaY > 0 ? 1.2 : 1 / 1.2, view.x + (event.clientX - box.left) * unit.value, view.y + (event.clientY - box.top) * unit.value)
}

const drag = ref(null)
function startDrag(event) {
  drag.value = { x: event.clientX, y: event.clientY, moved: false }
  event.currentTarget.setPointerCapture(event.pointerId)
}
function moveDrag(event) {
  if (!drag.value) return
  const dx = event.clientX - drag.value.x
  const dy = event.clientY - drag.value.y
  if (!drag.value.moved && Math.hypot(dx, dy) < 4) return
  show(view.x + view.w / 2 - dx * unit.value, view.y + viewHeight.value / 2 - dy * unit.value, view.w)
  drag.value = { x: event.clientX, y: event.clientY, moved: true }
}
function endDrag() {
  // A click on the map itself clears the selection.
  if (drag.value && !drag.value.moved) emit('select', '')
  drag.value = null
}

// The first devices to arrive and the first measure set the view; later
// polls leave it alone, a resize keeps its centre.
watch(() => places.value.length > 0, placed => { if (placed) fit() })
watch(() => [measured.width, measured.height], (now, before) => {
  if (!before[0]) fit()
  else show(view.x + view.w / 2, view.y + viewHeight.value / 2, view.w)
})
</script>

<style scoped>
.topo-map {
  position: relative;
  height: clamp(340px, 30vw, 560px);
  overflow: hidden;
  border-radius: var(--radius-md);
  background:
    radial-gradient(60% 80% at 30% 20%, var(--accent-soft), transparent 70%),
    var(--bg-elevated);
}

.topo-map__svg {
  display: block;
  width: 100%;
  height: 100%;
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.topo-map__svg.is-dragging {
  cursor: grabbing;
}

.topo-map__land {
  fill: none;
  stroke: var(--label-3);
  stroke-linecap: round;
  opacity: 0.38;
}

.topo-arc path {
  fill: none;
  stroke: var(--label-3);
  stroke-linecap: round;
  transition: opacity var(--dur-toggle) var(--ease-standard);
}

.topo-arc.is-egress path {
  stroke: var(--chart-3);
}

.topo-arc.is-warn path {
  stroke: var(--warning);
}

.topo-arc.is-bad path {
  stroke: var(--danger);
}

.topo-arc.is-off path {
  stroke: var(--label-3);
  stroke-dasharray: 3 3;
}

.topo-arc .topo-arc__flow {
  stroke: var(--bg-elevated);
  animation: topo-flow 1.6s linear infinite;
}

.topo-arc.is-dim {
  opacity: 0.12;
}

@keyframes topo-flow {
  to { stroke-dashoffset: var(--flow); }
}

.topo-pin {
  --pin: var(--accent);

  cursor: pointer;
  outline: none;
  transition: opacity var(--dur-toggle) var(--ease-standard);
}

.topo-pin.is-entry {
  --pin: var(--chart-2);
}

.topo-pin.is-relay {
  --pin: var(--chart-3);
}

.topo-pin__halo {
  fill: var(--pin);
  opacity: 0.16;
  transform-box: fill-box;
  transform-origin: center;
}

.topo-pin.is-online .topo-pin__halo {
  animation: topo-pulse 2.4s var(--ease-standard) infinite;
}

.topo-pin__core {
  fill: var(--pin);
  stroke: var(--bg-elevated);
}

.topo-pin.is-offline {
  --pin: var(--danger);
}

.topo-pin.is-pending,
.topo-pin.is-disabled {
  --pin: var(--label-3);
}

.topo-pin.is-offline .topo-pin__core,
.topo-pin.is-pending .topo-pin__core,
.topo-pin.is-disabled .topo-pin__core {
  fill: var(--bg-elevated);
  stroke: var(--pin);
}

.topo-pin__name {
  fill: var(--label-1);
  stroke: var(--bg-elevated);
  stroke-linejoin: round;
  paint-order: stroke;
  font-weight: var(--weight-medium);
  pointer-events: none;
}

.topo-pin.is-selected .topo-pin__halo,
.topo-pin:focus-visible .topo-pin__halo,
.topo-pin:hover .topo-pin__halo {
  opacity: 0.34;
  animation: none;
}

.topo-pin.is-dim {
  opacity: 0.25;
}

@keyframes topo-pulse {
  0%, 100% { transform: scale(0.7); }
  50% { transform: scale(1.15); }
}

.topo-map__tools {
  position: absolute;
  top: var(--space-3);
  right: var(--space-3);
  display: grid;
  gap: var(--space-1);
  padding: var(--space-1);
  border-radius: var(--radius-sm);
  background: var(--material);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
  backdrop-filter: blur(12px);
}

@media (prefers-reduced-motion: reduce) {
  .topo-arc .topo-arc__flow {
    display: none;
  }

  .topo-pin.is-online .topo-pin__halo {
    animation: none;
  }
}
</style>
