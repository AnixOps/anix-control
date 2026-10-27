<template>
  <UiSection :title="t('nodeEntries.title')" :description="t('nodeEntries.description')" heading-tag="h2" data-testid="node-entries">
    <template #actions>
      <UiButton size="md" variant="ghost" :icon="Cable" data-testid="edit-iwan-lines" @click="linesOpen = true">{{ t('nodeEntries.lines.open') }}</UiButton>
      <UiButton size="md" :icon="Pencil" data-testid="edit-node-entries" :disabled="loading || Boolean(error)" @click="sheetOpen = true">{{ t('nodeEntries.edit') }}</UiButton>
    </template>
    <NodeNotice v-if="uncovered.length" tone="warning" data-testid="node-entries-uncovered">
      {{ t('nodeEntries.uncovered', { protocols: uncoveredList }) }}
    </NodeNotice>
    <UiDataTable
      :columns="columns"
      :rows="entries"
      :label="t('nodeEntries.tableLabel', { name: node.name })"
      :row-label="row => row.label"
      storage-key="admin.node-entries"
      :loading="loading"
      :error="error"
      :error-title="t('nodeEntries.loadFailed')"
      :empty-icon="Waypoints"
      :empty-title="t('nodeEntries.emptyTitle')"
      :empty-description="t('nodeEntries.emptyDescription', { host: node.host || '—' })"
      state-heading-tag="h3"
      @retry="load"
    >
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="sheetOpen = true">{{ t('nodeEntries.add') }}</UiButton>
      </template>
      <template #cell-host="{ row }">
        <span class="node-entry-host">{{ row.host }}</span>
      </template>
      <template #cell-enabled="{ row }">
        <UiBadge :tone="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? t('nodeEntries.enabled') : t('nodeEntries.disabled')" />
      </template>
    </UiDataTable>

    <NodeEntriesSheet v-model:open="sheetOpen" :node="node" :entries="entries" :lines="lines" @saved="onSaved" />
    <IWANLinesSheet v-model:open="linesOpen" :lines="lines" @changed="loadLines" />
  </UiSection>
</template>

<script setup>
// 入口 on the node page's 协议 tab: the addresses subscribers reach the node
// at beyond its host (GET /api/v4/kernel/nodes/proxy-<id>/entries), such as
// each line's native IPv6 address and a provider NAT address with its port
// map. A node with enabled entries is offered once per entry and protocol.
import { computed, onMounted, ref } from 'vue'
import { Cable, Pencil, Plus, Waypoints } from '@lucide/vue'
import { getKernelNodeEntries, listIWANLines } from '@/api/kernel'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSection from '@/ui/UiSection.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import IWANLinesSheet from './IWANLinesSheet.vue'
import NodeEntriesSheet from './NodeEntriesSheet.vue'
import NodeNotice from './NodeNotice.vue'
import { portMapSummary } from './nodeEntriesModel'

const props = defineProps({
  node: { type: Object, required: true }
})
const { t } = useAppI18n()

const entries = ref([])
// Enabled protocols no enabled entry offers: subscriptions leave them out
// although the protocol shows online.
const uncovered = ref([])
const uncoveredList = computed(() => uncovered.value.map(p => `${p.name || p.type} :${p.port}`).join(t('nodeEntries.listSeparator')))
const loading = ref(false)
const error = ref(null)
const sheetOpen = ref(false)
// Panabit iWAN lines, for iwan entries and their sheet.
const lines = ref([])
const linesOpen = ref(false)
const lineNames = computed(() => new Map(lines.value.map(line => [line.id, line.name])))

function portsOf(row) {
  if (row.kind === 'nat') return portMapSummary(row.port_map)
  if (row.kind === 'iwan') return t('nodeEntries.viaLine', { name: lineNames.value.get(row.iwan_line_id) || `#${row.iwan_line_id}` })
  return t('nodeEntries.samePorts')
}

const columns = computed(() => [
  { key: 'label', label: t('nodeEntries.columns.label'), primary: true },
  { key: 'kind', label: t('nodeEntries.columns.kind'), value: row => t(`nodeEntries.kinds.${row.kind}`), secondary: true },
  { key: 'host', label: t('nodeEntries.columns.host') },
  { key: 'family', label: t('nodeEntries.columns.family'), value: row => t(`nodeEntries.families.${row.family}`), breakpoint: 'md' },
  { key: 'ports', label: t('nodeEntries.columns.ports'), value: portsOf, breakpoint: 'lg' },
  { key: 'enabled', label: t('nodeEntries.columns.status') }
])

async function load() {
  loading.value = true
  error.value = null
  try {
    const answer = await getKernelNodeEntries(`proxy-${props.node.id}`)
    entries.value = answer?.entries || []
    uncovered.value = answer?.uncovered_protocols || []
  } catch (e) {
    console.error('Failed to load node entries:', e)
    error.value = e
  } finally {
    loading.value = false
  }
}

async function loadLines() {
  try {
    lines.value = (await listIWANLines())?.lines || []
  } catch (e) {
    // The entries still list without the line names.
    console.error('Failed to load iWAN lines:', e)
  }
}

function onSaved(answer) {
  entries.value = answer?.entries || []
  uncovered.value = answer?.uncovered_protocols || []
}

onMounted(() => {
  load()
  loadLines()
})

defineExpose({ load, entries, uncovered })
</script>

<style scoped>
.node-entry-host {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
