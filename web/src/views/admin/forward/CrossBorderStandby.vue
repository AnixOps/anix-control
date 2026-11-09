<template>
  <section class="fwd-page" data-testid="cross-border-standby">
    <ForwardAreaNav area="standby" :seconds-ago="secondsAgo" :loading="loading" refreshable @refresh="refresh" />

    <UiPageHeader :title="t('standby.page.title')" :description="t('standby.page.description')">
      <template #actions>
        <UiButton :icon="Wrench" data-testid="standby-prepare" @click="openSheet">{{ t('standby.page.prepare') }}</UiButton>
        <UiButton v-if="links.length" :icon="ShieldAlert" data-testid="standby-all-reality" @click="switchTo(null, 'reality')">{{ t('standby.page.allReality') }}</UiButton>
        <UiButton v-if="links.length" :icon="ShieldAlert" data-testid="standby-all-xhttp" @click="switchTo(null, 'xhttp')">{{ t('standby.page.allXhttp') }}</UiButton>
        <UiButton v-if="links.length" variant="primary" :icon="Undo2" data-testid="standby-all-off" @click="switchTo(null, 'off')">{{ t('standby.page.allOff') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiDataTable
      :columns="columns"
      :rows="links"
      row-key="transport_id"
      :label="t('standby.page.title')"
      :row-label="row => row.transport_id"
      :row-actions="rowActions"
      :loading="loading && !loaded"
      :error="loadError"
      :error-title="t('standby.page.loadFailed')"
      :empty-icon="ShieldAlert"
      :empty-title="t('standby.page.emptyTitle')"
      :empty-description="t('standby.page.emptyDescription')"
      :card-fields="4"
      @retry="refresh"
    >
      <template #cell-transport="{ row }">
        <span class="fwd-cell-stack">
          <span class="fwd-mono">{{ row.transport_id }}</span>
          <span class="fwd-muted">{{ row.from }} → {{ row.to }}</span>
          <span v-if="row.skipped?.length" class="fwd-muted" :title="row.skipped.join('\n')">{{ t('standby.page.skipped') }}</span>
        </span>
      </template>
      <template #cell-pairs="{ row }">
        <span class="fwd-cell-stack">
          <span v-for="pair in row.pairs || []" :key="`${pair.source_node_id}-${pair.exit_node_id}`" class="fwd-mono">
            #{{ pair.source_node_id }} → #{{ pair.exit_node_id }}
          </span>
        </span>
      </template>
      <template #cell-active="{ row }">
        <UiBadge :tone="tierTone(row.active_tier)" :label="t(`standby.page.tier.${row.active_tier || 'off'}`)" />
      </template>
      <template #cell-ready="{ row }">
        <span class="standby-badges">
          <UiBadge v-for="tier in row.ready || []" :key="tier" tone="neutral" :label="t(`standby.page.tier.${tier}`)" />
          <span v-if="!(row.ready || []).length" class="fwd-muted">{{ t('standby.page.none') }}</span>
        </span>
      </template>
      <template #cell-channels="{ row }">
        <span class="fwd-cell-stack">
          <span v-for="channel in row.channels || []" :key="channel.id" class="fwd-mono standby-wrap">
            {{ channel.tier }} · #{{ channel.exit_node_id }}:{{ channel.relay_port }}
            {{ channel.tier === 'reality' ? channel.reality_server_name : `${channel.xhttp_domain}${channel.xhttp_path}` }}
          </span>
        </span>
      </template>
    </UiDataTable>

    <p v-if="links.length" class="list-page__note">{{ t('standby.page.note') }}</p>
    <p v-if="stale.length" class="list-page__note">{{ t('standby.page.stale', { n: stale.length }) }}</p>

    <UiSheet
      :open="sheetOpen"
      size="md"
      :title="t('standby.sheet.title')"
      :description="t('standby.sheet.description')"
      :dismissible="!saving"
      @update:open="value => (sheetOpen = value)"
    >
      <form class="standby-sheet" novalidate data-testid="standby-form" @submit.prevent="prepare">
        <UiTextField v-model="form.serverName" size="md" :label="t('standby.sheet.serverName')" placeholder="www.microsoft.com" :help="t('standby.sheet.serverNameHelp')" />
        <UiTextField v-model="form.dest" size="md" :label="t('standby.sheet.dest')" :help="t('standby.sheet.destHelp')" />
        <UiTextarea v-model="form.domains" :rows="5" :label="t('standby.sheet.domains')" :help="t('standby.sheet.domainsHelp')" />
        <p v-if="saveError" class="fwd-note is-danger" role="alert">{{ saveError }}</p>
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" data-testid="standby-save" @click="prepare">{{ t('standby.sheet.save') }}</UiButton>
      </template>
    </UiSheet>
  </section>
</template>

<script setup>
// 过境应急: each cross-border transport's public-protocol standby channels
// (internal/l2standby), switched on and back in one click. Both nodes'
// Agents get a switch with their next configuration (within a minute).
import { computed, ref } from 'vue'
import { ShieldAlert, ShieldCheck, Undo2, Wrench } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiPageHeader, UiSheet, UiTextarea, UiTextField, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { getKernelStandby, prepareKernelStandby, switchKernelStandby } from '@/api/kernel'
import ForwardAreaNav from '@/components/forward/ForwardAreaNav.vue'
import { egressErrorMessage } from '@/components/forward/egressModel'
import { standbyForm, standbyPrepareRequest, tierTone } from '@/components/forward/standbyModel'
import { usePolling } from '@/components/forward/usePolling'
import '@/components/forward/forward.css'

const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const overview = ref(null)
const loaded = ref(false)
const loadError = ref(null)
const links = computed(() => overview.value?.links || [])
const stale = computed(() => overview.value?.stale || [])

const { loading, secondsAgo, refresh } = usePolling(async () => {
  try {
    overview.value = await getKernelStandby()
    loaded.value = true
    loadError.value = null
  } catch (error) {
    loadError.value = { message: egressErrorMessage(error) }
    throw error
  }
}, { interval: 15000 })

const columns = computed(() => [
  { key: 'transport', label: t('standby.page.columns.transport'), primary: true },
  { key: 'active', label: t('standby.page.columns.active'), secondary: true },
  { key: 'pairs', label: t('standby.page.columns.pairs'), breakpoint: 'md' },
  { key: 'ready', label: t('standby.page.columns.ready') },
  { key: 'channels', label: t('standby.page.columns.channels'), breakpoint: 'lg' }
])

function rowActions(row) {
  return [
    { key: 'reality', label: t('standby.page.toReality'), icon: ShieldAlert, onSelect: () => switchTo(row, 'reality') },
    { key: 'xhttp', label: t('standby.page.toXhttp'), icon: ShieldAlert, onSelect: () => switchTo(row, 'xhttp') },
    { key: 'off', label: t('standby.page.toOff'), icon: ShieldCheck, separatorBefore: true, onSelect: () => switchTo(row, 'off') }
  ]
}

async function switchTo(row, tier) {
  const tierLabel = t(`standby.page.tier.${tier}`)
  await confirm({
    title: t('standby.confirm.title', { what: row ? row.transport_id : t('standby.confirm.all'), tier: tierLabel }),
    message: t('standby.confirm.message'),
    confirmLabel: t('standby.confirm.ok'),
    tone: tier === 'off' ? 'default' : 'danger',
    onConfirm: async () => {
      let answer
      try {
        answer = await switchKernelStandby(row ? { transports: [row.transport_id], tier } : { all: true, tier })
      } catch (error) {
        throw new Error(egressErrorMessage(error))
      }
      toast.success(t('standby.toast.switched', { n: answer?.transports?.length || 0, tier: tierLabel }))
      if (answer?.overview) overview.value = answer.overview
      await refresh()
    }
  })
}

const sheetOpen = ref(false)
const saving = ref(false)
const saveError = ref('')
const form = ref(standbyForm(null))

function openSheet() {
  form.value = standbyForm(overview.value)
  saveError.value = ''
  sheetOpen.value = true
}

async function prepare() {
  saving.value = true
  saveError.value = ''
  try {
    const answer = await prepareKernelStandby(standbyPrepareRequest(form.value))
    toast.success(t('standby.toast.prepared', { created: answer?.created || 0, updated: answer?.updated || 0, removed: answer?.removed || 0 }))
    if (answer?.overview) overview.value = answer.overview
    sheetOpen.value = false
  } catch (error) {
    saveError.value = egressErrorMessage(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.standby-badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.standby-wrap {
  overflow-wrap: anywhere;
}

.standby-sheet {
  display: grid;
  gap: var(--space-4);
}
</style>
