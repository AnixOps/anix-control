<template>
  <section class="fwd-page" data-testid="lines">
    <UiPageHeader :title="t('lines.title')" :description="t('lines.description')">
      <template #actions>
        <UiIconButton :icon="RefreshCw" :label="t('forwardV4.updated.refresh')" :disabled="loading" @click="refresh" />
        <UiButton variant="primary" :icon="Plus" :disabled="!network" data-testid="line-create" @click="openForm(null)">{{ t('lines.form.createTitle') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiFilterChips v-model="filter" :options="filters" :label="t('lines.filter.label')" />

    <UiDataTable
      :columns="columns"
      :rows="shown"
      :label="t('lines.title')"
      :loading="!lines && !loadError"
      :error="lines ? null : loadError"
      :error-title="t('lines.loadFailed')"
      :empty-title="t('lines.emptyTitle')"
      :empty-description="t('lines.emptyDescription')"
      :filtered="Boolean(filter)"
      :settings="false"
      activatable
      @row-activate="selected = $event.id"
      @retry="refresh"
      @clear-filters="filter = ''"
    >
      <template #cell-name="{ row }">
        <span class="lines__name">
          <span class="lines__tone" :class="`is-${row.tone}`" :title="t(`topology.tone.${row.tone}`)" />
          <span class="lines__text">
            <span class="lines__title">{{ row.name }}</span>
            <span class="fwd-muted">{{ summary(row) }}</span>
          </span>
        </span>
      </template>
      <template #cell-path="{ row }"><LinePath :line="row" /></template>
    </UiDataTable>

    <p v-if="lines?.length" class="fwd-muted lines__legend">{{ t('lines.legend') }}</p>

    <UiSection v-if="skipped.length" :title="t('lines.skipped.title', { n: skipped.length })" :description="t('lines.skipped.description')" heading-tag="h2" data-testid="lines-skipped">
      <ul class="lines__skipped">
        <li v-for="item in skipped" :key="item.id"><span class="fwd-mono">{{ item.id }}</span><span class="fwd-muted">{{ item.reason }}</span></li>
      </ul>
    </UiSection>

    <UiSheet :open="Boolean(line)" :title="line?.name" :description="line ? `${t(`lines.kind.${line.kind}`)} · ${t(`topology.tone.${line.tone}`)}` : ''" data-testid="line-sheet" @update:open="selected = ''">
      <template v-if="line?.source.revision" #header-actions>
        <span class="lines__actions">
          <UiButton size="sm" data-testid="line-edit" @click="openForm(line.source)">{{ t('lines.actions.edit') }}</UiButton>
          <UiMenu :label="t('lines.actions.more')" :items="menuItems" data-testid="line-menu" />
        </span>
      </template>
      <template v-if="line">
        <h3 class="lines__sub">{{ t('lines.detail.path') }}</h3>
        <ol class="lines__hops">
          <li v-for="(hop, index) in line.hops" :key="index" :class="`is-${line.hops.length === 1 ? 'exit' : hop.role}`">
            <span class="lines__pin" aria-hidden="true" />
            <span class="lines__hop">
              <span v-for="device in hop.devices" :key="device.ref" class="lines__device">
                <RouterLink v-if="device.id" class="lines__link" :to="`/admin/nodes/${device.id}`">{{ device.name }}</RouterLink>
                <span v-else>{{ device.name }}</span>
                <UiBadge v-if="device.ref === line.served" tone="info" :label="t('lines.detail.served')" />
                <UiBadge v-if="!device.online" status="offline" />
              </span>
              <span class="fwd-muted">{{ hopText(line, hop) }}</span>
            </span>
          </li>
          <li v-for="target in line.kind === 'fixed' ? line.targets : []" :key="target" class="is-target">
            <span class="lines__pin" aria-hidden="true" />
            <span class="lines__hop"><span class="fwd-mono">{{ target }}</span><span class="fwd-muted">{{ t('lines.detail.target') }}</span></span>
          </li>
        </ol>

        <dl class="lines__facts">
          <div><dt>{{ t('lines.detail.protocol') }}</dt><dd>{{ line.protocol || t('lines.detail.noProtocol') }}</dd></div>
          <div><dt>{{ t('lines.detail.port') }}</dt><dd>{{ line.port }}</dd></div>
          <div v-if="line.entryHostname"><dt>{{ t('lines.detail.hostname') }}</dt><dd class="fwd-mono">{{ line.entryHostname }}</dd></div>
          <div><dt>{{ t('lines.detail.subscription') }}</dt><dd>{{ t(line.show ? 'lines.detail.shown' : 'lines.detail.hidden') }}</dd></div>
          <div><dt>{{ t('lines.columns.rate') }}</dt><dd>× {{ line.rate }}</dd></div>
          <div v-if="line.route"><dt>{{ t('lines.columns.traffic') }}</dt><dd>{{ fmt.bytes(line.traffic, { precision: 1 }) }}</dd></div>
        </dl>

        <h3 class="lines__sub">{{ t('lines.detail.rules') }}</h3>
        <p v-if="!line.rules.length" class="fwd-muted">{{ t('lines.detail.noRules') }}</p>
        <ul v-else class="lines__rules">
          <li v-for="rule in line.rules" :key="rule.name">
            <span class="lines__text"><span>{{ rule.name }}</span><span class="fwd-muted">{{ rule.domains.join(', ') }}</span></span>
            <UiBadge :tone="rule.enabled ? 'neutral' : 'warning'" :label="rule.enabled ? t('lines.detail.via', { name: rule.exit?.name || '—' }) : t('topology.tone.off')" />
          </li>
        </ul>

        <div v-if="line.route">
          <UiButton variant="tertiary" :icon-end="ChevronRight" :as="RouterLink" :to="`/admin/forward/routes/${line.route}`">{{ t('lines.detail.openRoute') }}</UiButton>
        </div>
      </template>
    </UiSheet>

    <LineSheet v-model:open="formOpen" :line="formLine" :network="network" @saved="saved" />
  </section>
</template>

<script setup>
// 线路: every way from an entry to an exit, read from what Control stores
// today as lines (docs/design/network-lines). A line is what a subscriber
// connects through: relayed over other devices, direct on one device, or a
// port forward to fixed targets. Read-only: lines are still made of node
// protocols, forward routes and egress routes, which their own pages edit.
// A stored line (one with a revision) is created, edited, copied and
// deleted here (/api/v5/net/lines); a protocol's own line is edited with
// the protocol. Polls every 30 s while visible.
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { ChevronRight, Plus, RefreshCw } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiFilterChips, UiIconButton, UiMenu, UiPageHeader, UiSection, UiSheet, useConfirm, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteNetLine, getKernelAdminNodes, getKernelNetworkLines } from '@/api/kernel'
import { loadForwardSnapshot } from '@/components/forward/forwardData'
import { usePolling } from '@/components/forward/usePolling'
import LinePath from '@/components/lines/LinePath.vue'
import LineSheet from '@/components/lines/LineSheet.vue'
import { buildLines, matches } from '@/components/lines/linesModel'
import { readNodeApiError, readNodePage } from '@/views/admin/nodes/nodeData'
import '@/components/forward/forward.css'

const { t, locale } = useAppI18n()
const fmt = useFormat()
const confirm = useConfirm()
const toast = useToast()

const network = ref(null)
const lines = ref(null)
const skipped = ref([])
const loadError = ref(null)
const { loading, refresh } = usePolling(async () => {
  try {
    // The routes give a relayed line its state and traffic; they need the
    // forward package.
    const [report, nodePage, snapshot] = await Promise.all([
      getKernelNetworkLines(),
      getKernelAdminNodes({ page: 1, page_size: 500 }),
      loadForwardSnapshot().catch(() => null)
    ])
    network.value = report.snapshot
    lines.value = buildLines({ network: report.snapshot, nodes: readNodePage(nodePage).list || [], routes: snapshot?.routes, traffic: snapshot?.traffic })
    skipped.value = report.snapshot.skipped || []
    loadError.value = null
  } catch (error) {
    loadError.value = { message: readNodeApiError(error) }
    throw error
  }
}, { interval: 30_000 })

const filter = ref('')
const filters = computed(() => ['relay', 'direct', 'fixed', 'attention'].map(value => ({
  value,
  label: t(`lines.filter.${value}`),
  count: (lines.value || []).filter(item => matches(item, value)).length
})))
const shown = computed(() => (lines.value || []).filter(item => matches(item, filter.value)))

const columns = computed(() => [
  { key: 'name', label: t('lines.columns.line'), primary: true, hideable: false, sortable: true, minWidth: '220px' },
  { key: 'path', label: t('lines.columns.path'), value: row => row.hops.map(hop => hop.devices[0]?.name).join(' → ') },
  { key: 'rate', label: t('lines.columns.rate'), align: 'end', numeric: true, sortable: true, breakpoint: 'md', format: value => `× ${value}` },
  { key: 'traffic', label: t('lines.columns.traffic'), align: 'end', numeric: true, sortable: true, firstDirection: 'desc', format: (value, row) => (row.route ? fmt.bytes(value, { precision: 1 }) : '—') }
])

const selected = ref('')
const line = computed(() => (lines.value || []).find(item => item.id === selected.value) || null)

// The line sheet: null for a new line, a line without an id for a copy.
const formOpen = ref(false)
const formLine = ref(null)
function openForm(source) {
  formLine.value = source
  formOpen.value = true
}
function saved(id) {
  selected.value = id
  refresh()
}

const menuItems = computed(() => [
  { label: t('lines.actions.copy'), onSelect: () => openForm({ ...line.value.source, id: '', revision: 0, name: t('lines.actions.copyName', { name: line.value.name }) }) },
  { label: t('lines.actions.delete'), danger: true, separatorBefore: true, onSelect: remove }
])

async function remove() {
  const item = line.value
  const ok = await confirm({
    title: t('lines.delete.title', { name: item.name }),
    message: t(item.rules.length ? 'lines.delete.messageRules' : 'lines.delete.message', { n: item.rules.length }),
    confirmLabel: t('lines.delete.confirm'),
    tone: 'danger',
    requireText: item.name,
    onConfirm: () => deleteNetLine(item.id)
  })
  if (!ok) return
  selected.value = ''
  toast.success(t('lines.toast.deleted', { name: item.name }))
  refresh()
}

const summary = row => [row.protocol || t('lines.detail.noProtocol'), row.port, t(`lines.kind.${row.kind}`), row.rules.length ? t('lines.rules', { n: row.rules.length }) : '', row.show ? '' : t('lines.detail.hidden')].filter(Boolean).join(' · ')
const placeName = device => device.place?.[locale.value === 'zh-CN' ? 'zh' : 'en']
function hopText(item, hop) {
  const role = item.hops.length === 1 ? t('lines.detail.entryAndExit') : t(`topology.roles.${hop.role}`)
  return [role, placeName(hop.devices[0]), hop.devices[0]?.kernel && hop.role !== 'exit' ? t('topology.device.kernel') : ''].filter(Boolean).join(' · ')
}
</script>

<style scoped>
.lines__name {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  min-width: 0;
}

.lines__text {
  display: grid;
  min-width: 0;
}

.lines__title {
  font-weight: var(--weight-semibold);
}

.lines__tone {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--label-3);
}

.lines__tone.is-ok {
  background: var(--success);
}

.lines__tone.is-warn {
  background: var(--warning);
}

.lines__tone.is-bad {
  background: var(--danger);
}

.lines__actions {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}

.lines__legend {
  margin: 0;
}

.lines__skipped,
.lines__rules,
.lines__hops {
  display: grid;
  margin: 0;
  padding: 0;
  list-style: none;
}

.lines__skipped li,
.lines__rules li {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  align-items: center;
  justify-content: space-between;
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--separator);
  font-size: var(--type-callout-size);
}

.lines__sub {
  margin: var(--space-4) 0 var(--space-1);
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-weight: var(--weight-semibold);
}

.lines__sub:first-child {
  margin-top: 0;
}

.lines__hops li {
  --pin: var(--accent);

  position: relative;
  display: grid;
  grid-template-columns: var(--space-4) minmax(0, 1fr);
  gap: var(--space-3);
  align-items: center;
  padding: var(--space-2) 0;
}

.lines__hops li.is-entry {
  --pin: var(--chart-2);
}

.lines__hops li.is-relay {
  --pin: var(--chart-3);
}

.lines__hops li.is-target {
  --pin: var(--label-3);
}

.lines__hops li + li::before {
  position: absolute;
  top: calc(-1 * var(--space-4));
  bottom: 50%;
  left: 7px;
  width: 2px;
  background: var(--separator-strong);
  content: "";
}

.lines__pin {
  z-index: 1;
  width: 12px;
  height: 12px;
  margin-left: 2px;
  border-radius: 50%;
  background: var(--pin);
  box-shadow: 0 0 0 3px var(--bg-elevated);
}

.lines__hop {
  display: grid;
  gap: var(--space-0-5);
  min-width: 0;
  font-size: var(--type-callout-size);
}

.lines__device {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.lines__link {
  color: var(--accent);
  font-weight: var(--weight-semibold);
  text-decoration: none;
}

.lines__link:hover {
  text-decoration: underline;
}

.lines__link:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.lines__facts {
  display: grid;
  margin: var(--space-4) 0 0;
}

.lines__facts > div {
  display: flex;
  gap: var(--space-3);
  justify-content: space-between;
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--separator);
  font-size: var(--type-callout-size);
}

.lines__facts dt {
  color: var(--label-2);
}

.lines__facts dd {
  margin: 0;
  overflow-wrap: anywhere;
  text-align: right;
}
</style>
