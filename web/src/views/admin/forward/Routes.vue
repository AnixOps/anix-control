<template>
  <section class="fwd-page" data-testid="forward-routes">
    <ForwardAreaNav area="routes" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('forwardV4.routes.title')" :description="t('forwardV4.routes.description')">
      <template #meta>
        <UiBadge v-if="allRows.length" tone="neutral" :dot="false" :label="t('forwardV4.routes.count', { n: allRows.length })" />
      </template>
      <template #actions>
        <UiMenu :label="t('forwardV4.routes.more')" :items="headerMenu" />
        <UiButton variant="primary" :icon="Plus" data-testid="forward-new-route" @click="router.push('/admin/forward/routes/new')">{{ t('forwardV4.routes.new') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      v-model:selected="selected"
      :columns="columns"
      :rows="rows"
      row-key="id"
      :label="t('forwardV4.routes.title')"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :loading="loading && !loaded"
      :error="loadError"
      :error-title="t('forwardV4.routes.loadFailed')"
      :filtered="filtered"
      :empty-icon="RouteIcon"
      :empty-title="t('forwardV4.routes.emptyTitle')"
      :empty-description="t('forwardV4.routes.emptyDescription')"
      selectable
      activatable
      :card-fields="4"
      :page-size="50"
      @row-activate="row => router.push(`/admin/forward/routes/${row.id}`)"
      @clear-filters="clearFilters"
      @retry="refresh"
    >
      <template v-if="allRows.length" #toolbar>
        <div class="routes-toolbar">
          <UiSearchField v-model="search" class="routes-toolbar__search" :label="t('forwardV4.routes.search')" :placeholder="t('forwardV4.routes.searchPlaceholder')" />
          <UiFilterChips v-model="status" :label="t('forwardV4.routes.statusFilter')" :options="statusOptions" />
          <div class="routes-toolbar__selects">
            <UiSelect v-model="engine" size="md" :aria-label="t('forwardV4.routes.engineFilter')" :options="engineOptions" />
            <UiSelect v-model="nodeRef" size="md" :aria-label="t('forwardV4.routes.nodeFilter')" :options="nodeOptions" />
            <UiSelect v-model="label" size="md" :aria-label="t('forwardV4.routes.labelFilter')" :options="labelOptions" />
          </div>
        </div>
      </template>
      <template #empty-actions>
        <UiButton :icon="Server" @click="router.push('/admin/forward/inventory')">{{ t('forwardV4.routes.viewNodes') }}</UiButton>
        <UiButton variant="primary" :icon="Plus" @click="router.push('/admin/forward/routes/new')">{{ t('forwardV4.routes.new') }}</UiButton>
      </template>

      <template #cell-name="{ row }">
        <span class="fwd-cell-stack">
          <span class="routes-name">{{ row.name }}</span>
          <span v-if="row.labels.length" class="routes-labels">
            <span v-for="text in row.labels" :key="text" class="routes-label">{{ text }}</span>
          </span>
        </span>
      </template>
      <template #cell-listen="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono routes-nowrap">{{ row.listenText }}</span>
          <span class="fwd-muted routes-hint" :title="row.entryHint">{{ row.entryHint }}</span>
        </span>
      </template>
      <template #cell-hops="{ row }">
        <HopChain :route="row.route" :node-name="nodeName" compact />
      </template>
      <template #cell-targets="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono">{{ row.targetText }}</span>
          <span v-if="row.targetCount > 1" class="fwd-muted">{{ t('forwardV4.routes.moreTargets', { n: row.targetCount - 1 }) }}</span>
        </span>
      </template>
      <template #cell-strategy="{ row }">
        <span class="fwd-cell-stack">
          <span>{{ t(`forwardV4.strategy.${row.strategy}`) }}</span>
          <span v-if="row.direct !== 'DIRECT_MODE_OFF'" class="fwd-muted">{{ t(`forwardV4.direct.${row.direct}`) }}</span>
        </span>
      </template>
      <template #cell-status="{ row }">
        <RouteStatusBadge :status="row.status" />
      </template>
      <template #cell-traffic="{ row }">
        <span class="fwd-cell-stack routes-traffic">
          <span>{{ fmt.bytes(row.traffic, { precision: 1, empty: '—' }) }}</span>
          <span v-if="row.traffic" class="fwd-muted routes-nowrap">↑{{ fmt.bytes(row.up, { precision: 1 }) }} ↓{{ fmt.bytes(row.down, { precision: 1 }) }}</span>
        </span>
      </template>

      <template #bulk-actions="{ rows: chosen, clear }">
        <UiButton size="sm" :icon="Pause" :disabled="bulkBusy" data-testid="forward-bulk-pause" @click="bulkPause(chosen, true, clear)">{{ t('forwardV4.bulk.pause', { n: chosen.length }) }}</UiButton>
        <UiButton size="sm" :icon="Play" :disabled="bulkBusy" data-testid="forward-bulk-resume" @click="bulkPause(chosen, false, clear)">{{ t('forwardV4.bulk.resume') }}</UiButton>
        <UiButton size="sm" variant="danger-soft" :icon="Trash2" :disabled="bulkBusy || canDelete === false" data-testid="forward-bulk-delete" @click="bulkDelete(chosen, clear)">{{ t('forwardV4.bulk.delete') }}</UiButton>
      </template>
    </UiDataTable>

    <div v-if="allRows.length" class="routes-notes">
      <p class="list-page__note">{{ t('forwardV4.routes.note') }}</p>
      <p v-if="truncatedNote" class="fwd-note is-warning" role="status">{{ truncatedNote }}</p>
    </div>

    <UiDialog v-model:open="importOpen" :title="t('forwardV4.import.title')" :description="t('forwardV4.import.description')" size="lg">
      <UiTextarea v-model="importText" :label="t('forwardV4.import.label')" :rows="10" :error="importError" />
      <template #footer="{ close }">
        <UiButton @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="importing" @click="runImport">{{ t('forwardV4.import.run') }}</UiButton>
      </template>
    </UiDialog>
  </section>
</template>

<script setup>
// 路由 (F5b): every route with its hop chain, worst status and 24 h entry
// traffic. Status, engine, node and label filter the loaded routes in the
// browser (D3); the traffic is one GET /stats joined by route (D6). Bulk
// actions run the per-route calls, four at a time, each with its own
// Idempotency-Key (D9).
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Copy, Download, Pause, Pencil, Play, Plus, Route as RouteIcon, Server, Stethoscope, Trash2, Upload } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiDialog, UiFilterChips, UiMenu, UiPageHeader, UiSearchField, UiSelect, UiTextarea, useConfirm, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createRoute, deleteRoute, newIdempotencyKey, pauseRoute, resumeRoute } from '@/api/forwardV4'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import HopChain from '@/components/forward/HopChain.vue'
import RouteStatusBadge from '@/components/forward/RouteStatusBadge.vue'
import { loadForwardSnapshot } from '@/components/forward/forwardData'
import { forwardErrorMessage } from '@/components/forward/messages'
import { usePolling } from '@/components/forward/usePolling'
import { ENGINES, isEnforced, runBulk } from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const router = useRouter()
const { t } = useAppI18n()
const fmt = useFormat()
const toast = useToast()
const confirm = useConfirm()

const snapshot = ref(null)
const loaded = ref(false)
const loadError = ref(null)
const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    snapshot.value = await loadForwardSnapshot()
    loadError.value = null
    loaded.value = true
  } catch (error) {
    loadError.value = { message: forwardErrorMessage(t, error) }
    throw error
  }
}, { interval: 30_000 })

const search = ref('')
const status = ref('')
const engine = ref('all')
const nodeRef = ref('all')
const label = ref('all')
const selected = ref([])
const bulkBusy = ref(false)

const PROTOCOL = { L4_PROTOCOL_TCP: 'TCP', L4_PROTOCOL_UDP: 'UDP', L4_PROTOCOL_TCP_UDP: 'TCP+UDP' }
const nodesByRef = computed(() => new Map((snapshot.value?.nodes || []).map(node => [node.node_ref, node])))
function nodeName(ref) {
  return nodesByRef.value.get(ref)?.name || ref
}
const canDelete = computed(() => snapshot.value?.canDeleteRoutes ?? null)

const allRows = computed(() => (snapshot.value?.routes || []).map(item => {
  const route = item.route || {}
  const traffic = snapshot.value.traffic.get(route.id) || { up: 0, down: 0 }
  const entry = route.hops?.[0] || { node_refs: [] }
  const first = route.targets?.[0]
  return {
    id: route.id,
    item,
    route,
    name: route.name || route.id,
    labels: Object.entries(route.labels || {}).map(([key, value]) => `${key}=${value}`),
    listenText: `:${route.listen?.port || t('forwardV4.routes.autoPort')} ${PROTOCOL[route.listen?.protocol] || 'TCP'}`,
    entryHint: route.listen?.entry_hostname
      ? t('forwardV4.routes.entryHa', { host: route.listen.entry_hostname, n: entry.node_refs.length })
      : (entry.node_refs || []).map(nodeName).join(', '),
    targetText: first ? `${first.host}:${first.port}` : '—',
    targetCount: (route.targets || []).length,
    strategy: route.policy?.target || 'BALANCE_STRATEGY_ROUND_ROBIN',
    direct: route.policy?.direct || 'DIRECT_MODE_OFF',
    status: item.status,
    enforced: item.enforced || '',
    paused: Boolean(route.paused),
    traffic: traffic.up + traffic.down,
    up: traffic.up,
    down: traffic.down
  }
}))

function statusMatches(row, key) {
  return key === 'enforced' ? isEnforced(row.enforced) : row.status === key
}

const rows = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return allRows.value.filter(row => {
    if (needle && !`${row.name} ${row.listenText} ${row.route.targets?.map(target => `${target.host}:${target.port}`).join(' ')}`.toLowerCase().includes(needle)) return false
    if (status.value && !statusMatches(row, status.value)) return false
    if (engine.value !== 'all' && !row.route.hops?.some(hop => hop.engine === engine.value)) return false
    if (nodeRef.value !== 'all' && !row.route.hops?.some(hop => (hop.node_refs || []).includes(nodeRef.value))) return false
    if (label.value !== 'all' && !row.labels.includes(label.value)) return false
    return true
  })
})

const filtered = computed(() => Boolean(search.value || status.value || engine.value !== 'all' || nodeRef.value !== 'all' || label.value !== 'all'))

function clearFilters() {
  search.value = ''
  status.value = ''
  engine.value = 'all'
  nodeRef.value = 'all'
  label.value = 'all'
}

const statusOptions = computed(() => ['healthy', 'degraded', 'error', 'paused', 'enforced'].map(key => ({
  value: key,
  label: t(`forwardV4.status.filter.${key}`),
  count: allRows.value.filter(row => statusMatches(row, key)).length
})))
const engineOptions = computed(() => {
  const used = new Set(allRows.value.flatMap(row => (row.route.hops || []).map(hop => hop.engine)))
  const engines = ['ENGINE_NFTABLES', 'ENGINE_GOST', ...(used.has('ENGINE_ANIXOPS') ? ['ENGINE_ANIXOPS'] : [])]
  return [{ value: 'all', label: t('forwardV4.routes.allEngines') }, ...engines.map(value => ({ value, label: ENGINES[value].label }))]
})
const nodeOptions = computed(() => [
  { value: 'all', label: t('forwardV4.routes.allNodes') },
  ...(snapshot.value?.nodes || []).map(node => ({ value: node.node_ref, label: node.name || node.node_ref, description: node.node_ref }))
])
const labelOptions = computed(() => [
  { value: 'all', label: t('forwardV4.routes.allLabels') },
  ...Array.from(new Set(allRows.value.flatMap(row => row.labels))).sort().map(value => ({ value, label: value }))
])

const columns = computed(() => [
  { key: 'name', label: t('forwardV4.routes.columns.name'), primary: true, sortable: true, minWidth: '160px' },
  { key: 'listen', label: t('forwardV4.routes.columns.listen'), secondary: true, sortable: true, sortValue: row => Number(row.route.listen?.port || 0) },
  { key: 'hops', label: t('forwardV4.routes.columns.hops'), minWidth: '200px' },
  { key: 'targets', label: t('forwardV4.routes.columns.targets'), breakpoint: 'lg', maxWidth: '200px', card: false },
  { key: 'strategy', label: t('forwardV4.routes.columns.strategy'), breakpoint: 'lg', card: false },
  { key: 'status', label: t('forwardV4.routes.columns.status'), sortable: true, sortValue: row => row.status },
  { key: 'traffic', label: t('forwardV4.routes.columns.traffic'), align: 'end', numeric: true, sortable: true, firstDirection: 'desc', sortValue: row => row.traffic }
])

const truncatedNote = computed(() => {
  if (snapshot.value?.statsTruncated) return t('forwardV4.routes.statsTruncated')
  if (snapshot.value?.routesTruncated) return t('forwardV4.routes.routesTruncated')
  return ''
})

// ---------------------------------------------------------------------------
// Row actions
// ---------------------------------------------------------------------------

function rowActions(row) {
  const enforcedAction = row.enforced === 'quota' ? 'raiseQuota' : 'extendExpiry'
  return [
    { key: 'edit', label: t('forwardV4.actions.edit'), icon: Pencil, onSelect: () => router.push(`/admin/forward/routes/${row.id}/edit`) },
    isEnforced(row.enforced)
      ? { key: 'limits', label: t(`forwardV4.actions.${enforcedAction}`), icon: Pencil, onSelect: () => router.push({ path: `/admin/forward/routes/${row.id}/edit`, hash: '#limits' }) }
      : { key: 'pause', label: row.paused ? t('forwardV4.actions.resume') : t('forwardV4.actions.pause'), icon: row.paused ? Play : Pause, onSelect: () => togglePause(row) },
    { key: 'diagnose', label: t('forwardV4.actions.diagnose'), icon: Stethoscope, onSelect: () => router.push({ path: `/admin/forward/routes/${row.id}`, query: { diagnose: '1' } }) },
    { key: 'duplicate', label: t('forwardV4.actions.duplicate'), icon: Copy, onSelect: () => router.push({ path: '/admin/forward/routes/new', query: { from: row.id } }) },
    {
      key: 'delete',
      label: canDelete.value === false ? t('forwardV4.actions.deleteSuperOnly') : t('forwardV4.actions.delete'),
      icon: Trash2,
      danger: true,
      disabled: canDelete.value === false,
      separatorBefore: true,
      onSelect: () => removeRoute(row)
    }
  ]
}

async function togglePause(row) {
  const pause = !row.paused
  try {
    await (pause ? pauseRoute : resumeRoute)(row.id, { idempotencyKey: newIdempotencyKey() })
    toast.success(t(pause ? 'forwardV4.toast.paused' : 'forwardV4.toast.resumed', { name: row.name }), {
      undo: () => togglePause({ ...row, paused: pause })
    })
  } catch (error) {
    toast.error(forwardErrorMessage(t, error))
  }
  await refresh()
}

async function removeRoute(row) {
  await confirm({
    title: t('forwardV4.delete.title', { name: row.name }),
    message: t('forwardV4.delete.message'),
    confirmLabel: t('forwardV4.delete.confirm'),
    tone: 'danger',
    requireText: row.name,
    onConfirm: async () => {
      try {
        await deleteRoute(row.id, { idempotencyKey: newIdempotencyKey() })
      } catch (error) {
        throw new Error(forwardErrorMessage(t, error))
      }
      toast.success(t('forwardV4.toast.deleted', { name: row.name }))
      await refresh()
    }
  })
}

// ---------------------------------------------------------------------------
// Bulk (D9)
// ---------------------------------------------------------------------------

async function bulkPause(chosen, pause, clear) {
  // Enforced routes are Control's to resume; pausing them changes nothing.
  const targets = chosen.filter(row => !isEnforced(row.enforced) && row.paused !== pause)
  if (!targets.length) {
    toast.info(t('forwardV4.bulk.nothing'))
    return
  }
  bulkBusy.value = true
  const call = pause ? pauseRoute : resumeRoute
  const { done, failed } = await runBulk(targets, row => call(row.id, { idempotencyKey: newIdempotencyKey() }))
  bulkBusy.value = false
  clear()
  await refresh()
  const message = t(pause ? 'forwardV4.bulk.pausedSummary' : 'forwardV4.bulk.resumedSummary', { n: done.length, failed: failed.length })
  if (failed.length) {
    toast.show({
      tone: done.length ? 'warning' : 'error',
      message: `${message} ${forwardErrorMessage(t, failed[0].error)}`,
      action: { label: t('forwardV4.bulk.retry', { n: failed.length }), onAction: () => bulkPause(failed.map(entry => entry.item), pause, () => {}) }
    })
  } else {
    toast.success(message, { undo: () => bulkPause(done.map(row => ({ ...row, paused: pause })), !pause, () => {}) })
  }
}

async function bulkDelete(chosen, clear) {
  const phrase = chosen.length === 1 ? chosen[0].name : t('forwardV4.bulk.deletePhrase', { n: chosen.length })
  let result = null
  await confirm({
    title: t('forwardV4.bulk.deleteTitle', { n: chosen.length }),
    message: t('forwardV4.bulk.deleteMessage', { names: chosen.slice(0, 5).map(row => row.name).join(', ') + (chosen.length > 5 ? '…' : '') }),
    confirmLabel: t('forwardV4.delete.confirm'),
    tone: 'danger',
    requireText: phrase,
    onConfirm: async () => {
      bulkBusy.value = true
      result = await runBulk(chosen, row => deleteRoute(row.id, { idempotencyKey: newIdempotencyKey() }))
      bulkBusy.value = false
      if (!result.done.length && result.failed.length) throw new Error(forwardErrorMessage(t, result.failed[0].error))
    }
  })
  if (!result) return
  clear()
  await refresh()
  const message = t('forwardV4.bulk.deletedSummary', { n: result.done.length, failed: result.failed.length })
  if (result.failed.length) {
    toast.show({
      tone: 'warning',
      message: `${message} ${forwardErrorMessage(t, result.failed[0].error)}`,
      action: { label: t('forwardV4.bulk.retry', { n: result.failed.length }), onAction: () => bulkDelete(result.failed.map(entry => entry.item), () => {}) }
    })
  } else {
    toast.success(message)
  }
}

// ---------------------------------------------------------------------------
// Import and export (route JSON, protojson)
// ---------------------------------------------------------------------------

const importOpen = ref(false)
const importText = ref('')
const importError = ref('')
const importing = ref(false)

function exportRoutes() {
  const chosen = selected.value.length ? allRows.value.filter(row => selected.value.includes(row.id)) : allRows.value
  const body = JSON.stringify(chosen.map(row => row.route), null, 2)
  const blob = new Blob([body], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'forward-routes.json'
  link.click()
  URL.revokeObjectURL(url)
}

async function runImport() {
  importError.value = ''
  let parsed
  try {
    parsed = JSON.parse(importText.value)
  } catch {
    importError.value = t('forwardV4.import.invalid')
    return
  }
  const list = (Array.isArray(parsed) ? parsed : [parsed]).map(item => item?.route || item).filter(item => item && typeof item === 'object')
  if (!list.length) {
    importError.value = t('forwardV4.import.invalid')
    return
  }
  importing.value = true
  const { done, failed } = await runBulk(list, route => createRoute(route, { idempotencyKey: newIdempotencyKey() }))
  importing.value = false
  if (failed.length) {
    importError.value = t('forwardV4.import.partial', { n: done.length, failed: failed.length }) + ' ' + forwardErrorMessage(t, failed[0].error)
  } else {
    importOpen.value = false
    importText.value = ''
    toast.success(t('forwardV4.import.done', { n: done.length }))
  }
  await refresh()
}

const headerMenu = computed(() => [
  { key: 'import', label: t('forwardV4.routes.import'), icon: Upload, onSelect: () => { importOpen.value = true } },
  { key: 'export', label: selected.value.length ? t('forwardV4.routes.exportSelected', { n: selected.value.length }) : t('forwardV4.routes.exportAll'), icon: Download, disabled: !allRows.value.length, onSelect: exportRoutes }
])
</script>

<style scoped>
.routes-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  width: 100%;
}

.routes-toolbar__search {
  flex: 1 1 220px;
  max-width: 320px;
}

.routes-toolbar__selects {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.routes-toolbar__selects > * {
  min-width: 132px;
}

.routes-name {
  font-weight: var(--weight-medium);
}

.routes-labels {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.routes-label {
  white-space: nowrap;
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.routes-traffic {
  justify-items: end;
}

.routes-nowrap {
  white-space: nowrap;
}

.routes-hint {
  overflow: hidden;
  max-width: 160px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.routes-notes {
  display: grid;
  gap: var(--space-2);
}

@media (max-width: 639.98px) {
  .routes-toolbar__search {
    max-width: none;
  }

  .routes-toolbar__selects > * {
    flex: 1 1 40%;
    min-width: 0;
  }

  .routes-traffic {
    justify-items: start;
  }
}
</style>
