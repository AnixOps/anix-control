<template>
  <section class="fwd-page" data-testid="forward-dns">
    <ForwardAreaNav area="dns" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('forwardDns.page.title')" :description="t('forwardDns.page.description')">
      <template #actions>
        <UiButton
          variant="primary"
          :icon="Plus"
          :disabled="!canWrite || !kinds.length"
          :title="canWrite ? undefined : t('forwardDns.superOnly')"
          data-testid="forward-dns-add"
          @click="openSheet(null)"
        >
          {{ t('forwardDns.page.add') }}
        </UiButton>
      </template>
    </UiPageHeader>

    <p v-if="!canWrite" class="fwd-note" role="status" data-testid="forward-dns-readonly">{{ t('forwardDns.page.readOnly') }}</p>

    <UiDataTable
      :columns="columns"
      :rows="rows"
      row-key="id"
      :label="t('forwardDns.page.title')"
      :row-label="row => row.name"
      :row-actions="rowActions"
      :loading="loading && !loaded"
      :error="loadError"
      :error-title="t('forwardDns.page.loadFailed')"
      :empty-icon="Globe"
      :empty-title="t('forwardDns.page.emptyTitle')"
      :empty-description="t('forwardDns.page.emptyDescription')"
      :card-fields="4"
      @retry="refresh"
    >
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" :disabled="!canWrite || !kinds.length" @click="openSheet(null)">{{ t('forwardDns.page.add') }}</UiButton>
      </template>

      <template #cell-name="{ row }">
        <span class="fwd-cell-stack">
          <span class="dns-name">{{ row.name }}</span>
          <span class="fwd-muted">{{ row.kindName }}</span>
        </span>
      </template>
      <template #cell-config="{ row }">
        <span v-if="row.configText" class="fwd-mono dns-wrap">{{ row.configText }}</span>
        <span v-else class="fwd-muted">{{ t('forwardDns.page.defaultEndpoint') }}</span>
      </template>
      <template #cell-credentials="{ row }">
        <span class="dns-chips">
          <span v-for="name in row.credential_names || []" :key="name" class="fwd-label-chip dns-chip">
            <KeyRound :size="12" aria-hidden="true" />{{ credentialLabel(name) }}
          </span>
          <UiBadge v-if="row.missing.length" tone="warning" :label="t('forwardDns.page.missing', { names: row.missing.map(credentialLabel).join(', ') })" />
        </span>
      </template>
      <template #cell-bindings="{ row }">
        <UiBadge v-if="row.bindingCount" tone="info" :dot="false" :label="t('forwardDns.page.bindingCount', { n: row.bindingCount }, row.bindingCount)" />
        <span v-else class="fwd-muted">{{ t('forwardDns.page.unused') }}</span>
      </template>
      <template #cell-updated="{ row }">
        <span class="fwd-muted">{{ row.updatedAt ? fmt.relativeTime(row.updatedAt) : '—' }}</span>
      </template>
    </UiDataTable>

    <p v-if="rows.length" class="list-page__note">{{ t('forwardDns.page.note') }}</p>

    <DnsProviderSheet v-model:open="sheetOpen" :provider="editingProvider" :kinds="kinds" @saved="refresh" @forbidden="canWrite = false" />
  </section>
</template>

<script setup>
// DNS 服务商 (L2): the DNS provider accounts entry high availability
// writes through. Every administrator may read them; adding, changing and
// deleting one needs a super administrator, which the forwarding lists'
// can_delete tells (D7) and a 403 confirms. The list has no secret: the
// credentials are write-only and only their names show.
import { computed, ref } from 'vue'
import { Globe, KeyRound, Pencil, Plus, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiPageHeader, useConfirm, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteDnsProvider, listDnsKinds, listDnsProviders, listNodes, newIdempotencyKey } from '@/api/forwardV4'
import DnsProviderSheet from '@/components/forward/DnsProviderSheet.vue'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import { dnsErrorMessage, kindName } from '@/components/forward/dnsModel'
import { forwardErrorMessage } from '@/components/forward/messages'
import { usePolling } from '@/components/forward/usePolling'
import { num } from '@/components/forward/routeModel'
import '@/components/forward/forward.css'

const { t, te } = useAppI18n()
const fmt = useFormat()
const toast = useToast()
const confirm = useConfirm()

const providers = ref([])
const kinds = ref([])
const loaded = ref(false)
const loadError = ref(null)
// null until known; false hides the writes (D7 can_delete, or a 403).
const superAdmin = ref(null)
const canWrite = computed({
  get: () => superAdmin.value !== false,
  set: value => { superAdmin.value = value }
})

// Providers change only through this page: no timer, refresh on demand.
const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    const [providerList, kindList, nodeAnswer] = await Promise.all([
      listDnsProviders(),
      kinds.value.length ? Promise.resolve(kinds.value) : listDnsKinds(),
      superAdmin.value === false ? Promise.resolve(null) : listNodes({ kind: 'forward' }).catch(() => null)
    ])
    providers.value = providerList
    kinds.value = kindList
    if (typeof nodeAnswer?.canDelete === 'boolean') superAdmin.value = nodeAnswer.canDelete
    loaded.value = true
    loadError.value = null
  } catch (error) {
    loadError.value = { message: forwardErrorMessage(t, error) }
    throw error
  }
}, { interval: 0 })

function credentialLabel(name) {
  const key = `forwardDns.credential.${name}`
  return te(key) ? t(key) : name
}

const rows = computed(() => providers.value.map(provider => {
  const spec = kinds.value.find(item => item.kind === provider.kind)
  const stored = new Set(provider.credential_names || [])
  return {
    ...provider,
    kindName: kindName(kinds.value, provider.kind),
    configText: Object.entries(provider.config || {}).filter(([, value]) => value).map(([key, value]) => (key === 'endpoint' || key === 'url' ? value : `${key}=${value}`)).join(' · '),
    missing: (spec?.credentials || []).filter(name => !stored.has(name)),
    bindingCount: num(provider.bindings),
    updatedAt: num(provider.updated_at_unix_ms) || num(provider.created_at_unix_ms)
  }
}))

const columns = computed(() => [
  { key: 'name', label: t('forwardDns.page.columns.name'), primary: true, sortable: true },
  { key: 'bindings', label: t('forwardDns.page.columns.bindings'), secondary: true, sortable: true, sortValue: row => row.bindingCount },
  { key: 'config', label: t('forwardDns.page.columns.endpoint'), breakpoint: 'md' },
  { key: 'credentials', label: t('forwardDns.page.columns.credentials') },
  { key: 'updated', label: t('forwardDns.page.columns.updated'), breakpoint: 'lg', sortable: true, sortValue: row => row.updatedAt }
])

const sheetOpen = ref(false)
const editingProvider = ref(null)
function openSheet(provider) {
  editingProvider.value = provider
  sheetOpen.value = true
}

function rowActions(row) {
  const inUse = row.bindingCount > 0
  return [
    {
      key: 'edit',
      label: canWrite.value ? t('forwardDns.page.edit') : t('forwardDns.page.editSuperOnly'),
      icon: Pencil,
      disabled: !canWrite.value,
      onSelect: () => openSheet(providers.value.find(item => item.id === row.id) || row)
    },
    {
      key: 'delete',
      label: !canWrite.value ? t('forwardDns.page.deleteSuperOnly') : (inUse ? t('forwardDns.page.deleteInUse', { n: row.bindingCount }, row.bindingCount) : t('forwardDns.page.delete')),
      icon: Trash2,
      danger: true,
      separatorBefore: true,
      disabled: !canWrite.value || inUse,
      onSelect: () => remove(row)
    }
  ]
}

async function remove(row) {
  await confirm({
    title: t('forwardDns.delete.title', { name: row.name }),
    message: t('forwardDns.delete.message'),
    confirmLabel: t('forwardDns.delete.confirm'),
    tone: 'danger',
    requireText: row.name,
    onConfirm: async () => {
      try {
        await deleteDnsProvider(row.id, { idempotencyKey: newIdempotencyKey() })
      } catch (error) {
        if (error.status === 403 || error.code === 'super_admin_required') canWrite.value = false
        throw new Error(dnsErrorMessage(t, te, error))
      }
      toast.success(t('forwardDns.toast.deleted', { name: row.name }))
      await refresh()
    }
  })
}
</script>

<style scoped>
.dns-name {
  font-weight: var(--weight-medium);
}

.dns-wrap {
  overflow-wrap: anywhere;
}

.dns-chips {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.dns-chip {
  padding-right: var(--space-2);
}
</style>
