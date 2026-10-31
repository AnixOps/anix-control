<template>
  <section class="fwd-page" data-testid="reverse-proxies">
    <ForwardAreaNav area="reverse-proxies" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('reverseProxies.page.title')" :description="t('reverseProxies.page.description')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-testid="reverse-proxy-add" @click="openSheet(null)">{{ t('reverseProxies.page.add') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="proxies"
      row-key="id"
      :label="t('reverseProxies.page.title')"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :loading="loading && !loaded"
      :error="loadError"
      :error-title="t('reverseProxies.page.loadFailed')"
      :empty-icon="Globe"
      :empty-title="t('reverseProxies.page.emptyTitle')"
      :empty-description="t('reverseProxies.page.emptyDescription')"
      :card-fields="4"
      @retry="refresh"
    >
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openSheet(null)">{{ t('reverseProxies.page.add') }}</UiButton>
      </template>

      <template #cell-name="{ row }">
        <span class="fwd-cell-stack">
          <span class="rp-name">{{ row.name }}</span>
          <UiBadge v-if="!row.enabled" tone="neutral" :label="t('reverseProxies.page.disabled')" />
        </span>
      </template>
      <template #cell-hosts="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono rp-wrap">{{ hosts(row).slice(0, 3).join(', ') }}{{ hosts(row).length > 3 ? '…' : '' }}</span>
          <span class="fwd-muted">{{ t('reverseProxies.page.routeCount', { n: routeCount(row) }, routeCount(row)) }}</span>
        </span>
      </template>
      <template #cell-path="{ row }">
        <span class="rp-path">
          <span>{{ row.entries.map(nodeLabel).join(', ') }}</span>
          <template v-if="row.via.length">
            <ArrowRight :size="14" aria-hidden="true" />
            <span>{{ row.via.map(nodeLabel).join(' → ') }}</span>
          </template>
        </span>
      </template>
      <template #cell-listen="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono">{{ row.tls_mode === 'none' ? 'http' : 'https' }}:{{ row.listen_port }}<template v-if="row.http_port"> · http:{{ row.http_port }}</template></span>
          <span v-if="row.certificate_error" class="fwd-muted is-danger" :title="row.certificate_error">{{ t('reverseProxies.page.certificateFailed') }}</span>
          <span v-else-if="row.certificate_not_after" class="fwd-muted">{{ t('reverseProxies.page.certificateUntil', { date: formatDate(row.certificate_not_after) }) }}</span>
        </span>
      </template>
      <template #cell-state="{ row }">
        <span v-if="!row.enabled" class="fwd-muted">—</span>
        <span v-else class="rp-states">
          <UiBadge v-for="status in row.status" :key="`${status.node}-${status.id}`" :tone="stateTone(status.state)" :label="`${nodeLabel(status.node)} · ${stateLabel(status.state)}`" :title="status.detail || undefined" />
          <UiBadge v-if="row.pending.length" tone="warning" :label="t('reverseProxies.page.pending')" :title="row.pending.join('\n')" />
          <UiBadge v-if="!row.status.length && !row.pending.length" tone="neutral" :label="t('reverseProxies.page.waiting')" />
        </span>
      </template>
    </UiDataTable>

    <p v-if="proxies.length" class="list-page__note">{{ t('reverseProxies.page.note') }}</p>

    <ReverseProxySheet v-model:open="sheetOpen" :proxy="editingProxy" :nodes="nodes" :dns-providers="dnsProviders" @saved="refresh" />
  </section>
</template>

<script setup>
// 反向代理: HTTP(S) sites on entry nodes, each request sent by host and
// path to a route's upstreams, dialed from the entry or from a node behind
// it over Control's ANX-A link. Control replans the nodes at once; the
// list polls their reports.
import { computed, ref } from 'vue'
import { ArrowRight, Globe, Pencil, Plus, Power, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiPageHeader, useConfirm, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteReverseProxy, getKernelAdminNodes, listReverseProxies, updateReverseProxy } from '@/api/kernel'
import { listDnsProviders } from '@/api/forwardV4'
import ReverseProxySheet from '@/components/forward/ReverseProxySheet.vue'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import { reverseProxyErrorMessage, reverseProxyHosts, reverseProxyInput } from '@/components/forward/reverseProxyModel'
import { usePolling } from '@/components/forward/usePolling'
import { readNodePage } from '@/views/admin/nodes/nodeData'
import '@/components/forward/forward.css'

const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()
const format = useFormat()

const proxies = ref([])
const nodes = ref([])
const dnsProviders = ref([])
let dnsProvidersLoaded = false
const loaded = ref(false)
const loadError = ref(null)

const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    const [answer, nodePage] = await Promise.all([
      listReverseProxies(),
      nodes.value.length ? Promise.resolve(null) : getKernelAdminNodes({ page: 1, page_size: 500 })
    ])
    proxies.value = (answer?.reverse_proxies || []).map(proxy => ({
      ...proxy, entries: proxy.entries || [], via: proxy.via || [], status: proxy.status || [], pending: proxy.pending || []
    }))
    if (!dnsProvidersLoaded) {
      dnsProvidersLoaded = true
      // The providers are the forward package's (forward.v4); without it
      // only manual certificates are offered.
      listDnsProviders().then(list => { dnsProviders.value = list }).catch(() => { dnsProvidersLoaded = false })
    }
    if (nodePage) nodes.value = (readNodePage(nodePage).list || []).map(node => ({ id: node.id, name: node.name, host: node.host }))
    loaded.value = true
    loadError.value = null
  } catch (error) {
    loadError.value = { message: reverseProxyErrorMessage(error) }
    throw error
  }
}, { interval: 15000 })

const hosts = reverseProxyHosts
const routeCount = row => (row.sites || []).reduce((n, site) => n + (site.routes || []).length, 0)

function nodeLabel(ref) {
  const id = Number(String(ref).replace(/^proxy-/, ''))
  const node = nodes.value.find(item => item.id === id)
  return node?.name || ref
}

const formatDate = value => format.date(value)

const STATE_TONES = { APPLIED: 'success', DEGRADED: 'warning', DOWN: 'danger', UNSUPPORTED: 'danger', ERROR: 'danger' }
const stateTone = state => STATE_TONES[state] || 'neutral'
const stateLabel = state => t(`reverseProxies.page.state.${STATE_TONES[state] ? String(state).toLowerCase() : 'unspecified'}`)

const columns = computed(() => [
  { key: 'name', label: t('reverseProxies.page.columns.name'), primary: true, sortable: true },
  { key: 'hosts', label: t('reverseProxies.page.columns.hosts'), secondary: true },
  { key: 'path', label: t('reverseProxies.page.columns.path'), breakpoint: 'md' },
  { key: 'listen', label: t('reverseProxies.page.columns.listen'), breakpoint: 'lg' },
  { key: 'state', label: t('reverseProxies.page.columns.state') }
])

const sheetOpen = ref(false)
const editingProxy = ref(null)
function openSheet(proxy) {
  editingProxy.value = proxy
  sheetOpen.value = true
}

function rowActions(row) {
  return [
    { key: 'edit', label: t('reverseProxies.page.edit'), icon: Pencil, onSelect: () => openSheet(row) },
    {
      key: 'toggle',
      label: row.enabled ? t('reverseProxies.page.disable') : t('reverseProxies.page.enable'),
      icon: Power,
      onSelect: () => toggle(row)
    },
    { key: 'delete', label: t('reverseProxies.page.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => remove(row) }
  ]
}

async function toggle(row) {
  try {
    await updateReverseProxy(row.id, reverseProxyInput(row, { enabled: !row.enabled }))
    toast.success(t('reverseProxies.toast.saved', { name: row.name }))
  } catch (error) {
    toast.error(reverseProxyErrorMessage(error))
  }
  await refresh()
}

async function remove(row) {
  await confirm({
    title: t('reverseProxies.delete.title', { name: row.name }),
    message: t('reverseProxies.delete.message'),
    confirmLabel: t('reverseProxies.delete.confirm'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        await deleteReverseProxy(row.id)
      } catch (error) {
        throw new Error(reverseProxyErrorMessage(error))
      }
      toast.success(t('reverseProxies.toast.deleted', { name: row.name }))
      await refresh()
    }
  })
}
</script>

<style scoped>
.rp-name {
  font-weight: var(--weight-medium);
}

.rp-path,
.rp-states {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  align-items: center;
}

.rp-wrap {
  overflow-wrap: anywhere;
}
</style>
