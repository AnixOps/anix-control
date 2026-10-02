<template>
  <div class="list-page ansible-machines-page">
    <UiPageHeader :title="t('forwardNodesPage.title')" :description="t('runtime.ansibleMachines.heroText')">
      <template #actions>
        <UiIconButton variant="secondary" :icon="RefreshCw" :label="t('common.actions.refresh')" :disabled="loading" data-test="ansible-refresh" @click="refreshAll" />
        <UiButton variant="primary" :icon="Plus" data-test="ansible-add" @click="openEditor()">{{ t('runtime.ansibleMachines.addMachine') }}</UiButton>
      </template>
    </UiPageHeader>

    <ForwardNodesModeNav current="ansible" />

    <p class="inventory-hint">
      <UiIcon :icon="KeyRound" :size="16" />
      <span>{{ t('runtime.ansibleMachines.inventoryHint') }}</span>
    </p>

    <UiDataTable
      :columns="columns"
      :rows="machines"
      :label="t('runtime.ansibleMachines.sectionTitle')"
      :row-label="machine => machine.name"
      storage-key="admin.ansible-machines"
      :loading="loading"
      :error="pageError"
      :error-title="t('runtime.ansibleMachines.errors.loadFailed')"
      :filtered="statusFilter !== 'all'"
      :empty-icon="Server"
      :empty-title="t('runtime.ansibleMachines.empty')"
      :empty-description="t('runtime.ansibleMachines.sectionCopy')"
      :row-actions="machineActions"
      activatable
      @row-activate="openDetail"
      @retry="refreshAll"
      @clear-filters="statusFilter = 'all'"
    >
      <template #toolbar>
        <UiFilterChips v-model="statusChip" :label="t('runtime.ansibleMachines.filterLabel')" :options="statusChips" />
        <p class="list-page__summary">
          <span>{{ t('runtime.ansibleMachines.stats.machines') }} <strong>{{ machines.length }}</strong></span>
          <span>{{ t('runtime.ansibleMachines.stats.online') }} <strong>{{ onlineCount }}</strong></span>
          <span>{{ t('runtime.ansibleMachines.stats.enabled') }} <strong>{{ enabledCount }}</strong></span>
        </p>
      </template>
      <template #empty-actions>
        <UiButton variant="primary" :icon="Plus" @click="openEditor()">{{ t('runtime.ansibleMachines.addMachine') }}</UiButton>
      </template>
      <template #cell-name="{ row }">
        <span class="machine-name">
          <span>{{ row.name }}</span>
          <code class="machine-host">{{ row.host }}:{{ row.port }}</code>
        </span>
      </template>
      <template #cell-status="{ row }">
        <span class="status-stack">
          <UiBadge :status="row.status === 1 ? 'online' : 'offline'" :label="row.status === 1 ? t('runtime.shared.online') : t('runtime.shared.offline')" />
          <UiBadge v-if="!row.enabled" status="disabled" :label="t('runtime.shared.disabled')" />
        </span>
      </template>
      <template #cell-result="{ row }">
        <span v-if="isPending(row.id)" class="result-text">{{ pendingLabel(row) }}</span>
        <span v-else-if="results[row.id]" :class="['result-text', results[row.id].success ? 'is-ok' : 'is-fail']">{{ results[row.id].message }}</span>
        <span v-else>—</span>
      </template>
    </UiDataTable>

    <AnsibleMachineDialog v-model:open="editorOpen" :machine-id="editingId" @saved="onSaved" />
  </div>
</template>

<script setup>
// 转发节点 › Ansible 机器 (/admin/forward/ansible-machines): stateless
// execution targets of the local Ansible runtime (execution plane, separate
// from the flux-panel forward control plane). UI U6 list template; U7 adds
// the run-mode switch, the detail page (/admin/forward/ansible-machines/:id)
// and moves the editor and actions into forward-nodes/ for both pages.
import { computed, inject, onMounted, ref } from 'vue'
import { routerKey } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'
import { getAnsibleMachines } from '@/api/admin'
import { KeyRound, Pencil, Plus, Power, RefreshCw, RefreshCcwDot, Server, Stethoscope, Trash2 } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import AnsibleMachineDialog from './forward-nodes/AnsibleMachineDialog.vue'
import ForwardNodesModeNav from './forward-nodes/ForwardNodesModeNav.vue'
import { listOf, normalizeMachine, unwrapPayload } from './forward-nodes/forwardNodeModel'
import { useAnsibleMachineActions } from './forward-nodes/useAnsibleMachineActions'

const { t } = useAppI18n()
const router = inject(routerKey, null)

const loading = ref(false)
const statusFilter = ref('all')
const machines = ref([])
const editorOpen = ref(false)
const editingId = ref(null)
const toast = useToast()
const pageError = ref('')
const format = useFormat()

const {
  results,
  isPending,
  pendingLabel,
  checkMachine,
  syncMachine,
  toggleMachine,
  deleteMachine,
  resolveRuntimeError
} = useAnsibleMachineActions({ onChanged: () => refreshAll() })

const onlineCount = computed(() => machines.value.filter(item => item.status === 1).length)
const enabledCount = computed(() => machines.value.filter(item => item.enabled).length)

// The status filter is the list's server-side `status` (1 reachable, 0 not).
const statusChips = computed(() => [
  { value: '1', label: t('runtime.ansibleMachines.filters.online') },
  { value: '0', label: t('runtime.ansibleMachines.filters.offline') }
])
const statusChip = computed({
  get: () => (statusFilter.value === 'all' ? '' : statusFilter.value),
  set: (value) => {
    statusFilter.value = value || 'all'
    refreshAll()
  }
})
const columns = computed(() => [
  { key: 'name', label: t('runtime.ansibleMachines.fields.name'), primary: true, sortable: true },
  { key: 'status', label: t('runtime.ansibleMachines.table.reachability'), secondary: true, sortable: true, sortValue: machine => machine.status },
  { key: 'region', label: t('runtime.ansibleMachines.meta.regionIsp'), value: machine => [machine.region, machine.isp].filter(Boolean).join(' / ') },
  { key: 'currentConn', label: t('runtime.ansibleMachines.meta.currentConn'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc' },
  { key: 'traffic', label: t('runtime.ansibleMachines.meta.traffic'), numeric: true, nowrap: true, value: machine => `${formatBytes(machine.totalUpload)} / ${formatBytes(machine.totalDownload)}`, sortValue: machine => machine.totalUpload + machine.totalDownload, sortable: true, firstDirection: 'desc', breakpoint: 'md' },
  { key: 'weight', label: t('runtime.ansibleMachines.fields.weight'), numeric: true, hidden: true },
  { key: 'result', label: t('runtime.ansibleMachines.table.lastResult'), card: false, breakpoint: 'lg' }
])
const machineActions = machine => {
  const busy = isPending(machine.id)
  return [
    { key: 'edit', label: t('runtime.ansibleMachines.actions.edit'), icon: Pencil, onSelect: () => openEditor(machine) },
    { key: 'check', label: t('runtime.ansibleMachines.actions.check'), icon: Stethoscope, disabled: busy, onSelect: () => checkMachine(machine) },
    { key: 'sync', label: t('runtime.ansibleMachines.actions.sync'), icon: RefreshCcwDot, disabled: busy, onSelect: () => syncMachine(machine) },
    { key: 'toggle', label: machine.enabled ? t('runtime.ansibleMachines.actions.disable') : t('runtime.ansibleMachines.actions.enable'), icon: Power, disabled: busy, onSelect: () => toggleMachine(machine) },
    { key: 'delete', label: t('runtime.ansibleMachines.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteMachine(machine) }
  ]
}

function formatBytes(value) {
  return format.bytes(Number(value || 0))
}

async function refreshAll() {
  loading.value = true
  pageError.value = ''
  try {
    const params = { page: 1, page_size: 200, type: 'relay' }
    if (statusFilter.value !== 'all') params.status = Number(statusFilter.value)
    machines.value = listOf(unwrapPayload(await getAnsibleMachines(params))).map(normalizeMachine)
  } catch (error) {
    pageError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.loadFailed')
    machines.value = []
  } finally {
    loading.value = false
  }
}

function openEditor(machine = null) {
  pageError.value = ''
  editingId.value = machine?.id || null
  editorOpen.value = true
}

async function onSaved(name) {
  await refreshAll()
  toast.success(t('runtime.ansibleMachines.messages.saved', { name }))
}

function openDetail(machine) {
  if (machine?.id && router) router.push(`/admin/forward/ansible-machines/${machine.id}`)
}

onMounted(async () => {
  await refreshAll()
})
</script>

<style scoped>
.inventory-hint {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.inventory-hint :deep(svg) {
  flex: none;
  margin-top: 2px;
}

.machine-name {
  display: inline-flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.machine-host {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.status-stack {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.result-text {
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  font-size: var(--type-callout-size);
}

.result-text.is-ok {
  color: color-mix(in srgb, var(--success) 78%, var(--label-1));
}

.result-text.is-fail {
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
}
</style>
