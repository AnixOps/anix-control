<template>
  <section class="list-page agent-transports" :aria-busy="loading ? 'true' : 'false'">
    <UiPageHeader :title="t('pageTitles.admin.agentTransports')" :description="t('agentTransports.subtitle')">
      <template #actions>
        <UiIconButton
          :icon="RefreshCw"
          variant="secondary"
          data-testid="refresh-agent-transports"
          :label="t('control.actions.refresh')"
          :disabled="loading"
          @click="load()"
        />
        <UiButton :icon="ArrowLeft" data-testid="agent-transports-back" @click="router.push('/admin/agent')">
          {{ t('agentTransports.back') }}
        </UiButton>
      </template>
    </UiPageHeader>

    <div v-if="inventory" class="agent-transports__policy" data-testid="agent-transports-policy" role="status">
      <p class="agent-transports__mode">
        <UiBadge :tone="modeTone" :dot="false" :label="t('agentTransports.mode', { mode: inventory.mode })" />
        <span>{{ modeDescription }}</span>
        <span v-if="inventory.sunset" class="agent-transports__sunset">{{ t('agentTransports.sunset', { date: format.date(inventory.sunset) }) }}</span>
      </p>
      <p v-if="inventory.mode !== 'required'" class="agent-transports__notice" data-testid="agent-transports-notice">
        {{ t('agentTransports.notice') }}
        <a :href="inventory.upgrade_guide" target="_blank" rel="noopener noreferrer">{{ t('agentTransports.guide') }}</a>
      </p>
    </div>

    <AgentUpgradeCampaign />

    <UiErrorState
      v-if="loadError && !inventory && !loading"
      :title="t('agentTransports.loadFailed')"
      :error="loadError"
      @retry="load()"
    />
    <template v-else>
      <UiFilterChips
        v-model="statusFilter"
        data-testid="agent-transports-filter"
        :label="t('agentTransports.filterLabel')"
        :options="statusChips"
      />
      <p v-if="loadError" class="agent-transports__error" role="alert">{{ loadError }}</p>
      <UiDataTable
        :columns="columns"
        :rows="rows"
        row-key="node"
        :row-label="row => row.name || row.node"
        :label="t('agentTransports.tableLabel')"
        storage-key="admin.agentTransports"
        :loading="loading && !inventory"
        :card-fields="4"
        :empty-icon="Cable"
        :empty-title="t('agentTransports.empty.title')"
        :empty-description="t('agentTransports.empty.description')"
        data-testid="agent-transports-table"
      >
        <template #cell-node="{ row }">
          <span class="agent-transports__node">
            <strong>{{ row.name || row.node }}</strong>
            <code>{{ row.node }}</code>
            <UiBadge v-if="!row.enabled" tone="neutral" :dot="false" :label="t('agentTransports.disabled')" />
          </span>
        </template>
        <template #cell-status="{ row }">
          <span class="agent-transports__status" :data-node-status="row.node">
            <UiBadge :tone="statusTone(row.status)" :label="statusLabel(row.status)" />
            <span v-if="row.status === 'legacy'" class="agent-transports__hint" data-legacy-warning>
              <UiIcon :icon="TriangleAlert" :size="14" />
              {{ t('agentTransports.legacyHint') }}
            </span>
          </span>
        </template>
        <template #cell-transport="{ row }">
          <span>{{ transportLabel(row.transport) }}</span>
        </template>
        <template #cell-certificate="{ row }">
          <span v-if="row.certificate" class="agent-transports__certificate">
            <code>{{ row.certificate.serial }}</code>
            <span>{{ t('agentTransports.certificateUntil', { date: format.date(row.certificate.not_after) }) }}</span>
          </span>
          <span v-else class="agent-transports__muted">{{ t('agentTransports.noCertificate') }}</span>
        </template>
        <template #cell-lastSeen="{ row }">
          <time v-if="row.last_seen_at" :datetime="row.last_seen_at" :title="format.dateTime(row.last_seen_at)">
            {{ format.relativeTime(row.last_seen_at) }}
          </time>
          <span v-else>—</span>
        </template>
        <template #cell-seenOn="{ row }">
          <span class="agent-transports__seen-on">
            <UiBadge
              v-for="transport in row.transports"
              :key="transport.transport"
              :tone="transport.legacy ? 'warning' : (transport.transport === 'mtls-stream' ? 'success' : 'neutral')"
              :dot="false"
              :label="transportLabel(transport.transport)"
            />
            <span v-if="!row.transports.length">—</span>
          </span>
        </template>
      </UiDataTable>
    </template>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, Cable, RefreshCw, TriangleAlert } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import AgentUpgradeCampaign from '@/components/admin/AgentUpgradeCampaign.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { getKernelAgentTransports } from '@/api/kernel'

const STATUSES = ['legacy', 'mtls', 'third-party', 'unseen']
const MODES = ['off', 'optional', 'preferred', 'required']
const TRANSPORTS = ['mtls-stream', 'apikey-stream', 'http-legacy', 'websocket', 'clean-agent', 'uniproxy', 'v2board-grpc']

const { t } = useAppI18n()
const router = useRouter()
const format = useFormat()

const inventory = ref(null)
const loading = ref(false)
const loadError = ref('')
const statusFilter = ref('')

const nodes = computed(() => (Array.isArray(inventory.value?.nodes) ? inventory.value.nodes : []).map(node => ({
  ...node,
  transports: Array.isArray(node.transports) ? node.transports : []
})))
const rows = computed(() => statusFilter.value ? nodes.value.filter(node => node.status === statusFilter.value) : nodes.value)
const summary = computed(() => inventory.value?.summary || {})
const statusChips = computed(() => [
  { value: '', label: t('agentTransports.statuses.all'), count: summary.value.total ?? nodes.value.length },
  ...STATUSES.map(status => ({ value: status, label: statusLabel(status), count: countOf(status) }))
])
const modeTone = computed(() => {
  const mode = inventory.value?.mode
  if (mode === 'required') return 'success'
  if (mode === 'preferred') return 'info'
  return 'warning'
})
const modeDescription = computed(() => {
  const mode = inventory.value?.mode
  return MODES.includes(mode) ? t(`agentTransports.modes.${mode}`) : ''
})

const columns = computed(() => [
  { key: 'node', label: t('agentTransports.columns.node'), primary: true, value: row => row.name || row.node, sortable: true },
  { key: 'status', label: t('agentTransports.columns.status'), value: row => STATUSES.indexOf(row.status), sortable: true },
  { key: 'transport', label: t('agentTransports.columns.transport'), secondary: true, value: row => row.transport || '', sortable: true, nowrap: true },
  { key: 'version', label: t('agentTransports.columns.version'), value: row => row.agent_version || '—', sortable: true, nowrap: true },
  { key: 'certificate', label: t('agentTransports.columns.certificate'), value: row => row.certificate?.not_after || '', sortable: true, breakpoint: 'lg' },
  { key: 'lastSeen', label: t('agentTransports.columns.lastSeen'), value: row => row.last_seen_at || '', sortable: true, firstDirection: 'desc', nowrap: true },
  { key: 'seenOn', label: t('agentTransports.columns.seenOn'), breakpoint: 'lg' }
])

function countOf(status) {
  const key = status === 'third-party' ? 'third_party' : status
  return summary.value[key] ?? nodes.value.filter(node => node.status === status).length
}

function statusLabel(status) {
  return STATUSES.includes(status) ? t(`agentTransports.statuses.${status}`) : (status || '—')
}

function statusTone(status) {
  if (status === 'mtls') return 'success'
  if (status === 'legacy') return 'warning'
  if (status === 'third-party') return 'info'
  return 'neutral'
}

function transportLabel(transport) {
  return TRANSPORTS.includes(transport) ? t(`agentTransports.transports.${transport}`) : (transport || '—')
}

function errorMessage(cause) {
  const response = cause?.response?.data
  return response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || response?.message || cause?.message || t('agentTransports.loadFailed')
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    inventory.value = await getKernelAgentTransports()
  } catch (cause) {
    loadError.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}

onMounted(() => load())

defineExpose({ load })
</script>

<style scoped>
.agent-transports__policy {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
}

.agent-transports__mode {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  margin: 0;
}

.agent-transports__sunset,
.agent-transports__muted {
  color: var(--label-2);
}

.agent-transports__notice {
  margin: 0;
  overflow-wrap: anywhere;
}

.agent-transports__error {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
}

.agent-transports__node,
.agent-transports__status,
.agent-transports__certificate,
.agent-transports__seen-on {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  min-width: 0;
}

.agent-transports__node code,
.agent-transports__certificate code {
  overflow-wrap: anywhere;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.agent-transports__hint {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  color: var(--warning);
  font-size: var(--type-caption-size);
}
</style>
