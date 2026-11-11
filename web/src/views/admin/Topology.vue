<template>
  <section class="fwd-page" data-testid="topology">
    <UiPageHeader :title="t('topology.title')" :description="t('topology.description')">
      <template #actions>
        <UiIconButton :icon="RefreshCw" :label="t('forwardV4.updated.refresh')" :disabled="loading" @click="refresh" />
      </template>
    </UiPageHeader>

    <UiErrorState v-if="loadError && !topology" :title="t('topology.loadFailed')" :error="loadError" @retry="refresh" />

    <template v-else>
      <div class="fwd-grid">
        <template v-if="topology">
          <UiMetricCard :label="t('topology.metrics.devices')" :value="`${online} / ${topology.devices.length}`" :icon="Server" :detail="t('topology.metrics.devicesDetail')" />
          <UiMetricCard :label="t('topology.metrics.lines')" :value="topology.lines.length" :icon="RouteIcon" :detail="t('topology.metrics.linesDetail', { n: broken })" />
          <UiMetricCard :label="t('topology.metrics.places')" :value="placeCount" :icon="MapPin" :detail="t('topology.metrics.placesDetail', { n: unplaced.length })" />
          <UiMetricCard :label="t('topology.metrics.users')" :value="users" :icon="Users" :detail="t('topology.metrics.usersDetail')" />
        </template>
        <template v-else>
          <UiSkeleton v-for="index in 4" :key="index" variant="card" />
        </template>
      </div>

      <UiSkeleton v-if="!topology" variant="card" />
      <UiEmptyState v-else-if="!topology.devices.length" :icon="Server" :title="t('topology.emptyTitle')" :description="t('topology.emptyDescription')" heading-tag="h2">
        <UiButton variant="primary" @click="router.push('/admin/nodes')">{{ t('topology.openNodes') }}</UiButton>
      </UiEmptyState>

      <div v-else class="topo">
        <div class="topo__canvas">
          <div class="topo__bar">
            <UiSegmentedControl :model-value="mode" :options="modes" size="sm" :aria-label="t('topology.view.label')" @update:model-value="setMode" />
            <span v-if="secondsAgo !== null" class="fwd-muted">{{ secondsAgo < 5 ? t('forwardV4.updated.now') : t('forwardV4.updated.ago', { n: secondsAgo }) }}</span>
          </div>
          <TopologyMap v-if="mode === 'map'" :devices="topology.devices" :links="topology.links" :focus="focus" :selected="selectedDevice" :label="t('topology.view.map')" @select="selectDevice" @hover="hovered = $event" />
          <TopologyGraph v-else :devices="topology.devices" :links="topology.links" :focus="focus" :selected="selectedDevice" @select="selectDevice" @hover="hovered = $event" />

          <ul class="topo__legend" :aria-label="t('topology.legend.label')">
            <li><span class="topo__key is-entry" />{{ t('topology.roles.entry') }}</li>
            <li><span class="topo__key is-relay" />{{ t('topology.roles.relay') }}</li>
            <li><span class="topo__key is-exit" />{{ t('topology.roles.exit') }}</li>
            <li><span class="topo__key is-offline" />{{ t('topology.status.offline') }}</li>
            <li><span class="topo__key is-line" />{{ t('topology.legend.line') }}</li>
            <li><span class="topo__key is-line is-egress" />{{ t('topology.legend.egress') }}</li>
          </ul>

          <p v-if="mode === 'map' && unplaced.length" class="topo__unplaced" data-testid="topology-unplaced">
            <span>{{ t('topology.unplaced', { n: unplaced.length }) }}</span>
            <button v-for="device in unplaced" :key="device.ref" type="button" class="topo__chip" @click="selectDevice(device.ref)">{{ device.name }}</button>
          </p>
        </div>

        <UiCard class="topo__panel" :padded="false">
          <!-- A device. -->
          <div v-if="device" class="topo__detail">
            <header class="topo__detail-head">
              <div class="topo__detail-title">
                <h2>{{ device.name }}</h2>
                <UiBadge :status="device.status" />
              </div>
              <UiIconButton :icon="X" size="sm" :label="t('topology.close')" @click="selection = null" />
            </header>
            <dl class="topo__facts">
              <div><dt>{{ t('topology.device.place') }}</dt><dd>{{ placeName(device) || t('topology.device.noPlace') }}</dd></div>
              <div><dt>{{ t('topology.device.role') }}</dt><dd>{{ device.roles.map(role => t(`topology.roles.${role}`)).join(' · ') }}</dd></div>
              <div><dt>{{ t('topology.device.host') }}</dt><dd class="fwd-mono">{{ device.host || '—' }}</dd></div>
              <div><dt>{{ t('topology.device.usersLabel') }}</dt><dd>{{ device.users }}</dd></div>
            </dl>
            <h3 class="topo__sub">{{ t('topology.device.lines', { n: deviceLines.length }) }}</h3>
            <p v-if="!deviceLines.length" class="fwd-muted topo__pad">{{ t('topology.device.noLines') }}</p>
            <ul v-else class="topo__lines">
              <li v-for="line in deviceLines" :key="line.id">
                <button type="button" class="topo__line" @click="selection = { type: 'line', id: line.id }">
                  <span class="topo__tone" :class="`is-${line.tone}`" aria-hidden="true" />
                  <span class="topo__line-text"><span class="topo__line-name">{{ line.name }}</span><span class="fwd-muted">{{ pathText(line) }}</span></span>
                </button>
              </li>
            </ul>
            <div v-if="device.id" class="topo__pad">
              <UiButton variant="tertiary" :icon-end="ChevronRight" :as="RouterLink" :to="`/admin/nodes/${device.id}`">{{ t('topology.device.open') }}</UiButton>
            </div>
          </div>

          <!-- A line. -->
          <div v-else-if="line" class="topo__detail">
            <header class="topo__detail-head">
              <div class="topo__detail-title">
                <h2>{{ line.name }}</h2>
                <UiBadge :tone="TONES[line.tone]" :label="t(`topology.tone.${line.tone}`)" />
              </div>
              <UiIconButton :icon="X" size="sm" :label="t('topology.close')" @click="selection = null" />
            </header>
            <ol class="topo__stages">
              <li v-for="(stage, index) in line.stages" :key="index">
                <span class="topo__stage-role">{{ t(`topology.roles.${stageRole(line, index)}`) }}</span>
                <button v-for="ref in stage" :key="ref" type="button" class="topo__chip" @click="selectDevice(ref)">{{ byRef.get(ref).name }}</button>
              </li>
            </ol>
            <dl class="topo__facts">
              <div><dt>{{ t('topology.line.kind') }}</dt><dd>{{ t(`topology.line.${line.kind}`) }}</dd></div>
              <div v-if="line.kind === 'route'"><dt>{{ t('topology.line.traffic') }}</dt><dd>{{ fmt.bytes(line.traffic, { precision: 1 }) }}</dd></div>
            </dl>
            <div class="topo__pad">
              <UiButton variant="tertiary" :icon-end="ChevronRight" :as="RouterLink" :to="line.kind === 'route' ? `/admin/forward/routes/${line.id}` : '/admin/forward/egress'">{{ t('topology.line.open') }}</UiButton>
            </div>
          </div>

          <!-- Every line. -->
          <div v-else class="topo__detail">
            <header class="topo__detail-head">
              <div class="topo__detail-title"><h2>{{ t('topology.lines.title') }}</h2><span class="fwd-muted">{{ topology.lines.length }}</span></div>
            </header>
            <p v-if="!topology.lines.length" class="fwd-muted topo__pad">{{ t('topology.lines.empty') }}</p>
            <ul v-else class="topo__lines">
              <li v-for="item in sortedLines" :key="item.id">
                <button type="button" class="topo__line" @click="selection = { type: 'line', id: item.id }" @pointerenter="hoveredLine = item.id" @pointerleave="hoveredLine = ''">
                  <span class="topo__tone" :class="`is-${item.tone}`" aria-hidden="true" />
                  <span class="topo__line-text"><span class="topo__line-name">{{ item.name }}</span><span class="fwd-muted">{{ pathText(item) }}</span></span>
                  <span v-if="item.traffic" class="fwd-muted topo__line-traffic">{{ fmt.bytes(item.traffic, { precision: 1 }) }}</span>
                </button>
              </li>
            </ul>
          </div>
        </UiCard>
      </div>
    </template>
  </section>
</template>

<script setup>
// 网络拓扑: every device and the lines between them, as a world map and as
// columns from entry to exit. Built from what Control already serves: the
// node list, the forward routes (when the forward package is there) and the
// egress routes. Polls every 30 s while visible.
import { computed, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ChevronRight, MapPin, RefreshCw, Route as RouteIcon, Server, Users, X } from '@lucide/vue'
import { UiBadge, UiButton, UiCard, UiEmptyState, UiErrorState, UiIconButton, UiMetricCard, UiPageHeader, UiSegmentedControl, UiSkeleton, useFormat } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { getKernelAdminNodes, listKernelEgressRoutes } from '@/api/kernel'
import { loadForwardSnapshot } from '@/components/forward/forwardData'
import { usePolling } from '@/components/forward/usePolling'
import TopologyGraph from '@/components/topology/TopologyGraph.vue'
import TopologyMap from '@/components/topology/TopologyMap.vue'
import { buildTopology, focusOf } from '@/components/topology/topologyModel'
import { readNodeApiError, readNodePage } from '@/views/admin/nodes/nodeData'
import '@/components/forward/forward.css'

const TONES = { ok: 'success', warn: 'warning', bad: 'danger', off: 'neutral' }
const TONE_ORDER = ['bad', 'warn', 'ok', 'off']

const route = useRoute()
const router = useRouter()
const { t, locale } = useAppI18n()
const fmt = useFormat()

const topology = ref(null)
const loadError = ref(null)
const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    // Lines need the forward package; the egress routes their own API.
    const [nodePage, snapshot, egress] = await Promise.all([
      getKernelAdminNodes({ page: 1, page_size: 500 }),
      loadForwardSnapshot().catch(() => null),
      listKernelEgressRoutes().catch(() => null)
    ])
    topology.value = buildTopology({
      nodes: (readNodePage(nodePage).list || []).filter(node => node.approval === 'approved'),
      forwardNodes: snapshot?.nodes,
      routes: snapshot?.routes,
      traffic: snapshot?.traffic,
      egress: egress?.routes
    })
    loadError.value = null
  } catch (error) {
    loadError.value = { message: readNodeApiError(error) }
    throw error
  }
}, { interval: 30_000 })

const mode = computed(() => (route.query.view === 'graph' ? 'graph' : 'map'))
const modes = computed(() => [
  { value: 'map', label: t('topology.view.map') },
  { value: 'graph', label: t('topology.view.graph') }
])
function setMode(value) {
  router.replace({ query: { ...route.query, view: value === 'graph' ? 'graph' : undefined } })
}

// { type: device | line, id }: what the panel shows and the views light up.
const selection = ref(null)
const hovered = ref(null)
const hoveredLine = ref('')
const byRef = computed(() => new Map(topology.value.devices.map(item => [item.ref, item])))
const device = computed(() => (selection.value?.type === 'device' ? byRef.value.get(selection.value.id) : null))
const line = computed(() => (selection.value?.type === 'line' ? topology.value.lines.find(item => item.id === selection.value.id) : null))
const selectedDevice = computed(() => (device.value ? device.value.ref : ''))
const focus = computed(() => focusOf(topology.value, hovered.value ? { type: 'device', id: hovered.value } : hoveredLine.value ? { type: 'line', id: hoveredLine.value } : selection.value))
function selectDevice(ref) {
  selection.value = ref ? { type: 'device', id: ref } : null
}

const online = computed(() => topology.value.devices.filter(item => item.status === 'online').length)
const broken = computed(() => topology.value.lines.filter(item => item.tone === 'bad' || item.tone === 'warn').length)
const unplaced = computed(() => topology.value.devices.filter(item => !item.place))
const placeCount = computed(() => new Set(topology.value.devices.map(item => item.place?.key).filter(Boolean)).size)
const users = computed(() => topology.value.devices.reduce((sum, item) => sum + item.users, 0))
const sortedLines = computed(() => [...topology.value.lines].sort((a, b) => TONE_ORDER.indexOf(a.tone) - TONE_ORDER.indexOf(b.tone) || b.traffic - a.traffic || a.name.localeCompare(b.name)))
const deviceLines = computed(() => topology.value.lines.filter(item => device.value.lines.includes(item.id)))

const placeName = item => item.place?.[locale.value === 'zh-CN' ? 'zh' : 'en']
const stageName = stage => byRef.value.get(stage[0]).name + (stage.length > 1 ? ` +${stage.length - 1}` : '')
const pathText = item => item.stages.map(stageName).join(' → ')
const stageRole = (item, index) => (item.kind === 'egress' ? 'exit' : index === 0 && item.stages.length > 1 ? 'entry' : index === item.stages.length - 1 ? 'exit' : 'relay')
</script>

<style scoped>
.topo {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: var(--space-4);
  align-items: start;
}

.topo__canvas {
  display: grid;
  gap: var(--space-3);
  min-width: 0;
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
  background: var(--bg-elevated);
}

.topo__bar {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4) 0;
}

.topo__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
  margin: 0;
  padding: 0 var(--space-4) var(--space-3);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  list-style: none;
}

.topo__legend li {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
}

.topo__key {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--accent);
}

.topo__key.is-entry {
  background: var(--chart-2);
}

.topo__key.is-relay {
  background: var(--chart-3);
}

.topo__key.is-offline {
  border: 2px solid var(--danger);
  background: transparent;
}

.topo__key.is-line {
  width: 18px;
  height: 3px;
  border-radius: var(--radius-pill);
  background: var(--brand-gradient);
}

.topo__key.is-line.is-egress {
  background: var(--chart-3);
}

.topo__unplaced {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--separator);
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.topo__chip {
  padding: var(--space-0-5) var(--space-2);
  border: 0;
  border-radius: var(--radius-pill);
  background: var(--fill-2);
  color: var(--label-1);
  font: inherit;
  font-size: var(--type-caption-size);
  cursor: pointer;
}

.topo__chip:hover {
  background: var(--fill-3);
}

.topo__chip:focus-visible,
.topo__line:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.topo__panel {
  max-height: calc(clamp(340px, 30vw, 560px) + 96px);
  overflow: auto;
}

.topo__detail-head {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  justify-content: space-between;
  padding: var(--space-4) var(--space-4) var(--space-2);
}

.topo__detail-title {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  min-width: 0;
}

.topo__detail-title h2 {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.topo__facts {
  display: grid;
  margin: 0;
  padding: 0 var(--space-4);
}

.topo__facts > div {
  display: flex;
  gap: var(--space-3);
  justify-content: space-between;
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--separator);
  font-size: var(--type-callout-size);
}

.topo__facts dt {
  color: var(--label-2);
}

.topo__facts dd {
  margin: 0;
  overflow-wrap: anywhere;
  text-align: right;
}

.topo__sub {
  margin: 0;
  padding: var(--space-4) var(--space-4) var(--space-1);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
}

.topo__pad {
  margin: 0;
  padding: var(--space-3) var(--space-4) var(--space-4);
}

.topo__lines {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  margin: 0;
  padding: 0 var(--space-2) var(--space-2);
  list-style: none;
}

.topo__line {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  width: 100%;
  padding: var(--space-2);
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--label-1);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.topo__line:hover {
  background: var(--fill-1);
}

.topo__line-text {
  display: grid;
  flex: 1;
  min-width: 0;
}

.topo__line-name {
  overflow: hidden;
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topo__line-text .fwd-muted {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topo__line-traffic {
  flex: none;
  font-variant-numeric: tabular-nums;
}

.topo__tone {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--label-3);
}

.topo__tone.is-ok {
  background: var(--success);
}

.topo__tone.is-warn {
  background: var(--warning);
}

.topo__tone.is-bad {
  background: var(--danger);
}

.topo__stages {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-1) var(--space-4) var(--space-3);
  list-style: none;
}

.topo__stages li {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.topo__stage-role {
  width: 3em;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

@media (max-width: 1068px) {
  .topo {
    grid-template-columns: minmax(0, 1fr);
  }

  .topo__panel {
    max-height: none;
  }
}
</style>
