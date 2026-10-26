<template>
  <section class="fwd-page" data-testid="egress-routes">
    <ForwardAreaNav area="egress" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('egressRoutes.page.title')" :description="t('egressRoutes.page.description')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-testid="egress-route-add" @click="openSheet(null)">{{ t('egressRoutes.page.add') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="routes"
      row-key="id"
      :label="t('egressRoutes.page.title')"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :loading="loading && !loaded"
      :error="loadError"
      :error-title="t('egressRoutes.page.loadFailed')"
      :empty-icon="Split"
      :empty-title="t('egressRoutes.page.emptyTitle')"
      :empty-description="t('egressRoutes.page.emptyDescription')"
      :card-fields="4"
      @retry="refresh"
    >
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openSheet(null)">{{ t('egressRoutes.page.add') }}</UiButton>
      </template>

      <template #cell-name="{ row }">
        <span class="fwd-cell-stack">
          <span class="egress-name">{{ row.name }}</span>
          <UiBadge v-if="!row.enabled" tone="neutral" :label="t('egressRoutes.page.disabled')" />
        </span>
      </template>
      <template #cell-path="{ row }">
        <span class="egress-path">
          <span>{{ nodeLabel(row.source_node) }}</span>
          <ArrowRight :size="14" aria-hidden="true" />
          <span>{{ nodeLabel(row.exit_node) }}</span>
        </span>
      </template>
      <template #cell-domains="{ row }">
        <span class="fwd-cell-stack">
          <span>{{ t('egressRoutes.page.domainCount', { n: row.domains.length }, row.domains.length) }}</span>
          <span class="fwd-muted egress-wrap">{{ row.domains.slice(0, 3).join(', ') }}{{ row.domains.length > 3 ? '…' : '' }}</span>
        </span>
      </template>
      <template #cell-relay="{ row }">
        <span class="fwd-mono">{{ row.relay_port }}</span>
      </template>
      <template #cell-state="{ row }">
        <span v-if="row.enabled" class="egress-states">
          <UiBadge :tone="stateTone(row.source_state)" :label="`${t('egressRoutes.page.stateSource')} · ${stateLabel(row.source_state)}`" :title="row.source_state?.error || undefined" />
          <UiBadge :tone="stateTone(row.exit_state)" :label="`${t('egressRoutes.page.stateExit')} · ${stateLabel(row.exit_state)}`" :title="row.exit_state?.error || undefined" />
        </span>
        <span v-else class="fwd-muted">—</span>
      </template>
    </UiDataTable>

    <p v-if="routes.length" class="list-page__note">{{ t('egressRoutes.page.note') }}</p>

    <EgressRouteSheet v-model:open="sheetOpen" :route="editingRoute" :nodes="nodes" @saved="refresh" />
  </section>
</template>

<script setup>
// 分流: the egress routes, each sending some domains a proxy node's users
// open out through another proxy node. Both nodes' Agents get a change with
// their next configuration (within a minute), so the list polls while a
// node is still pending.
import { computed, ref } from 'vue'
import { ArrowRight, Pencil, Plus, Power, Split, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiPageHeader, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteKernelEgressRoute, getKernelAdminNodes, listKernelEgressRoutes, updateKernelEgressRoute } from '@/api/kernel'
import EgressRouteSheet from '@/components/forward/EgressRouteSheet.vue'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import { egressErrorMessage, egressRouteInput } from '@/components/forward/egressModel'
import { usePolling } from '@/components/forward/usePolling'
import { readNodePage } from '@/views/admin/nodes/nodeData'
import '@/components/forward/forward.css'

const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const routes = ref([])
const nodes = ref([])
const loaded = ref(false)
const loadError = ref(null)

const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    const [answer, nodePage] = await Promise.all([
      listKernelEgressRoutes(),
      nodes.value.length ? Promise.resolve(null) : getKernelAdminNodes({ page: 1, page_size: 500 })
    ])
    routes.value = (answer?.routes || []).map(route => ({ ...route, domains: route.domains || [] }))
    if (nodePage) nodes.value = (readNodePage(nodePage).list || []).map(node => ({ id: node.id, name: node.name, host: node.host }))
    loaded.value = true
    loadError.value = null
  } catch (error) {
    loadError.value = { message: egressErrorMessage(error) }
    throw error
  }
}, { interval: 15000 })

function nodeLabel(node) {
  if (!node?.exists) return t('egressRoutes.page.nodeMissing', { id: node?.id })
  return node.name || `#${node.id}`
}

const STATE_TONES = { applied: 'success', pending: 'warning', failed: 'danger' }
const stateTone = state => STATE_TONES[state?.state] || 'neutral'
const stateLabel = state => t(`egressRoutes.page.state.${state?.state || 'pending'}`)

const columns = computed(() => [
  { key: 'name', label: t('egressRoutes.page.columns.name'), primary: true, sortable: true },
  { key: 'path', label: t('egressRoutes.page.columns.path'), secondary: true },
  { key: 'domains', label: t('egressRoutes.page.columns.domains'), breakpoint: 'md' },
  { key: 'relay', label: t('egressRoutes.page.columns.relay'), breakpoint: 'lg' },
  { key: 'state', label: t('egressRoutes.page.columns.state') }
])

const sheetOpen = ref(false)
const editingRoute = ref(null)
function openSheet(route) {
  editingRoute.value = route
  sheetOpen.value = true
}

function rowActions(row) {
  return [
    { key: 'edit', label: t('egressRoutes.page.edit'), icon: Pencil, onSelect: () => openSheet(row) },
    {
      key: 'toggle',
      label: row.enabled ? t('egressRoutes.page.disable') : t('egressRoutes.page.enable'),
      icon: Power,
      onSelect: () => toggle(row)
    },
    { key: 'delete', label: t('egressRoutes.page.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => remove(row) }
  ]
}

async function toggle(row) {
  try {
    await updateKernelEgressRoute(row.id, egressRouteInput(row, { enabled: !row.enabled }))
    toast.success(t('egressRoutes.toast.saved', { name: row.name }))
  } catch (error) {
    toast.error(egressErrorMessage(error))
  }
  await refresh()
}

async function remove(row) {
  await confirm({
    title: t('egressRoutes.delete.title', { name: row.name }),
    message: t('egressRoutes.delete.message'),
    confirmLabel: t('egressRoutes.delete.confirm'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteKernelEgressRoute(row.id)
      } catch (error) {
        throw new Error(egressErrorMessage(error))
      }
      toast.success(t('egressRoutes.toast.deleted', { name: row.name }))
      await refresh()
    }
  })
}
</script>

<style scoped>
.egress-name {
  font-weight: var(--weight-medium);
}

.egress-path {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  align-items: center;
}

.egress-states {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.egress-wrap {
  overflow-wrap: anywhere;
}
</style>
