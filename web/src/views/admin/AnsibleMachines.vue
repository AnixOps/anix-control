<template>
  <div class="list-page ansible-machines-page">
    <UiPageHeader :title="t('runtime.ansibleMachines.title')" :description="t('runtime.ansibleMachines.heroText')">
      <template #meta>
        <UiBadge tone="neutral" :dot="false" :label="t('runtime.ansibleMachines.heroEyebrow')" />
      </template>
      <template #actions>
        <UiIconButton variant="secondary" :icon="RefreshCw" :label="t('common.actions.refresh')" :disabled="loading" data-test="ansible-refresh" @click="refreshAll" />
        <UiButton variant="primary" :icon="Plus" data-test="ansible-add" @click="openEditor()">{{ t('runtime.ansibleMachines.addMachine') }}</UiButton>
      </template>
    </UiPageHeader>

    <nav class="runtime-links" :aria-label="t('runtime.ansibleMachines.relatedPages')">
      <RouterLink to="/admin/forward/local">{{ t('forwardSuite.nav.localRuntime') }}</RouterLink>
      <RouterLink to="/admin/forward/nodes">{{ t('forwardSuite.nav.nodeXTopology') }}</RouterLink>
      <RouterLink to="/admin/forward/agents">{{ t('forwardSuite.nav.nodeXAgents') }}</RouterLink>
    </nav>

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
        <span v-if="pendingAction.startsWith(`${row.id}:`)" class="result-text">{{ pendingLabel(row) }}</span>
        <span v-else-if="results[row.id]" :class="['result-text', results[row.id].success ? 'is-ok' : 'is-fail']">{{ results[row.id].message }}</span>
        <span v-else>—</span>
      </template>
    </UiDataTable>

    <UiDialog
      :open="editorOpen"
      :title="editorMode ? t('runtime.ansibleMachines.modal.titleEdit') : t('runtime.ansibleMachines.modal.titleAdd')"
      :description="t('runtime.ansibleMachines.inventoryHint')"
      :dismissible="!saving"
      @update:open="value => { if (!value) closeEditor() }"
    >
      <div class="form-grid">
        <UiTextField v-model.trim="form.name" required :label="t('runtime.ansibleMachines.fields.name')" :placeholder="t('runtime.ansibleMachines.placeholders.name')" />
        <UiTextField v-model.trim="form.host" required :label="t('runtime.ansibleMachines.fields.host')" :placeholder="t('runtime.ansibleMachines.placeholders.host')" />
        <UiTextField v-model.trim="form.port" required type="number" min="1" max="65535" :label="t('runtime.ansibleMachines.fields.reachabilityPort')" :help="t('runtime.ansibleMachines.fields.reachabilityHelp')" />
        <UiTextField v-model.trim="form.weight" type="number" min="1" :label="t('runtime.ansibleMachines.fields.weight')" />
        <UiTextField v-model.trim="form.region" :label="t('runtime.ansibleMachines.fields.region')" :placeholder="t('runtime.ansibleMachines.placeholders.region')" />
        <UiTextField v-model.trim="form.isp" :label="t('runtime.ansibleMachines.fields.isp')" :placeholder="t('runtime.ansibleMachines.placeholders.isp')" />
      </div>
      <p v-if="formError" class="form-error" role="alert">{{ formError }}</p>
      <template #footer>
        <UiButton :disabled="saving" @click="closeEditor">{{ t('runtime.ansibleMachines.modal.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="ansible-machine-save" :loading="saving" @click="submitForm">{{ t('runtime.ansibleMachines.modal.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  checkAnsibleMachine,
  createAnsibleMachine,
  deleteAnsibleMachine,
  getAnsibleMachine,
  getAnsibleMachines,
  syncAnsibleMachineStats,
  toggleAnsibleMachine,
  updateAnsibleMachine
} from '@/api/admin'
import { KeyRound, Pencil, Plus, Power, RefreshCw, RefreshCcwDot, Server, Stethoscope, Trash2 } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t, translateLiteral } = useAppI18n()

const loading = ref(false)
const saving = ref(false)
const statusFilter = ref('all')
const machines = ref([])
const results = reactive({})
const pendingAction = ref('')
const editorOpen = ref(false)
const editorMode = ref(false)
const toast = useToast()
const confirm = useConfirm()
const formError = ref('')
const pageError = ref('')
const form = reactive(createForm())

const onlineCount = computed(() => machines.value.filter(item => item.status === 1).length)
const enabledCount = computed(() => machines.value.filter(item => item.enabled).length)
const format = useFormat()

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
  const busy = pendingAction.value.startsWith(`${machine.id}:`)
  return [
    { key: 'edit', label: t('runtime.ansibleMachines.actions.edit'), icon: Pencil, onSelect: () => openEditor(machine) },
    { key: 'check', label: t('runtime.ansibleMachines.actions.check'), icon: Stethoscope, disabled: busy, onSelect: () => checkMachine(machine) },
    { key: 'sync', label: t('runtime.ansibleMachines.actions.sync'), icon: RefreshCcwDot, disabled: busy, onSelect: () => syncMachine(machine) },
    { key: 'toggle', label: machine.enabled ? t('runtime.ansibleMachines.actions.disable') : t('runtime.ansibleMachines.actions.enable'), icon: Power, disabled: busy, onSelect: () => toggleMachine(machine) },
    { key: 'delete', label: t('runtime.ansibleMachines.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => openDelete(machine) }
  ]
}
function pendingLabel(machine) {
  const action = pendingAction.value.split(':')[1]
  if (action === 'check') return t('runtime.ansibleMachines.actions.checking')
  if (action === 'sync') return t('runtime.ansibleMachines.actions.syncing')
  return t('runtime.ansibleMachines.actions.updating', { name: machine.name })
}

function translateRuntimeText(value, fallback = '-') {
  const text = String(value ?? '').trim()
  if (!text) {
    return fallback
  }
  return translateLiteral(text)
}

function resolveRuntimeError(error, fallbackKey) {
  return translateRuntimeText(error?.response?.data?.msg || error?.message, t(fallbackKey))
}

function createForm() {
  return { id: null, name: '', host: '', port: '', region: '', isp: '', weight: '1' }
}

function resetForm() {
  Object.assign(form, createForm())
  formError.value = ''
}

function unwrapResponse(response) {
  return response?.data?.data ?? response?.data ?? response
}

function normalizeMachine(node) {
  return {
    id: Number(node?.id || 0),
    name: node?.name || '-',
    host: node?.host || '-',
    port: Number(node?.port || 0),
    region: node?.region || '',
    isp: node?.isp || '',
    weight: Number(node?.weight || 1),
    enabled: Boolean(node?.enabled ?? true),
    status: Number(node?.status ?? 0),
    currentConn: Number(node?.current_conn || node?.currentConn || 0),
    totalUpload: Number(node?.total_upload || node?.totalUpload || 0),
    totalDownload: Number(node?.total_download || node?.totalDownload || 0)
  }
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
    const payload = unwrapResponse(await getAnsibleMachines(params))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    machines.value = list.map(normalizeMachine)
  } catch (error) {
    pageError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.loadFailed')
    machines.value = []
  } finally {
    loading.value = false
  }
}

async function openEditor(machine = null) {
  resetForm()
  pageError.value = ''
  editorMode.value = Boolean(machine?.id)
  editorOpen.value = true
  if (!machine?.id) return
  try {
    const detail = normalizeMachine(unwrapResponse(await getAnsibleMachine(machine.id)))
    Object.assign(form, {
      id: detail.id,
      name: detail.name,
      host: detail.host,
      port: detail.port ? String(detail.port) : '',
      region: detail.region,
      isp: detail.isp,
      weight: String(detail.weight || 1)
    })
  } catch (error) {
    formError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.detailFailed')
  }
}

function closeEditor(force = false) {
  if (saving.value && !force) return
  editorOpen.value = false
  editorMode.value = false
  resetForm()
}

async function submitForm() {
  formError.value = ''
  if (!form.name || !form.host || !Number(form.port)) {
    formError.value = t('runtime.ansibleMachines.errors.required')
    return
  }
  const payload = {
    name: form.name.trim(),
    type: 'relay',
    host: form.host.trim(),
    port: Number(form.port),
    weight: Number(form.weight || 1)
  }
  if (form.region.trim()) payload.region = form.region.trim()
  if (form.isp.trim()) payload.isp = form.isp.trim()

  saving.value = true
  try {
    if (editorMode.value && form.id) {
      await updateAnsibleMachine(form.id, payload)
    } else {
      await createAnsibleMachine(payload)
    }
    const saved = form.name.trim()
    await refreshAll()
    closeEditor(true)
    toast.success(t('runtime.ansibleMachines.messages.saved', { name: saved }))
  } catch (error) {
    formError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.saveFailed')
  } finally {
    saving.value = false
  }
}

// An execution machine is deleted only after its name is typed.
async function openDelete(machine) {
  if (!machine?.id) return
  const name = machine.name && machine.name !== '-' ? machine.name : `#${machine.id}`
  const confirmed = await confirm({
    title: t('runtime.ansibleMachines.modal.deleteTitle', { name }),
    message: t('runtime.ansibleMachines.modal.deleteConfirm'),
    confirmLabel: t('runtime.ansibleMachines.modal.deleteAction'),
    tone: 'danger',
    requireText: name,
    onConfirm: async () => {
      try {
        await deleteAnsibleMachine(machine.id)
      } catch (error) {
        throw new Error(resolveRuntimeError(error, 'runtime.ansibleMachines.errors.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('runtime.ansibleMachines.messages.deleted', { name }))
  await refreshAll()
}

async function checkMachine(machine) {
  pendingAction.value = `${machine.id}:check`
  try {
    const payload = unwrapResponse(await checkAnsibleMachine(machine.id))
    const success = Number(payload?.status ?? 0) === 1 && !payload?.error
    results[machine.id] = {
      success,
      message: success
        ? (payload?.latency
          ? t('runtime.ansibleMachines.results.latency', { value: payload.latency })
          : t('runtime.ansibleMachines.results.reachable'))
        : translateRuntimeText(payload?.error, t('runtime.ansibleMachines.results.unavailable'))
    }
    await refreshAll()
  } catch (error) {
    results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.checkFailed') }
  } finally {
    pendingAction.value = ''
  }
}

async function syncMachine(machine) {
  pendingAction.value = `${machine.id}:sync`
  try {
    const payload = unwrapResponse(await syncAnsibleMachineStats(machine.id))
    results[machine.id] = { success: true, message: translateRuntimeText(payload?.message, t('runtime.ansibleMachines.results.synced')) }
    await refreshAll()
  } catch (error) {
    results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.syncFailed') }
  } finally {
    pendingAction.value = ''
  }
}

async function toggleMachine(machine) {
  pendingAction.value = `${machine.id}:toggle`
  try {
    await toggleAnsibleMachine(machine.id, !machine.enabled)
    await refreshAll()
  } catch (error) {
    results[machine.id] = { success: false, message: resolveRuntimeError(error, 'runtime.ansibleMachines.errors.toggleFailed') }
  } finally {
    pendingAction.value = ''
  }
}

onMounted(async () => {
  await refreshAll()
})
</script>

<style scoped>
.runtime-links {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-5);
  margin-top: calc(-1 * var(--space-3));
  font-size: var(--type-callout-size);
}

.runtime-links a {
  color: var(--accent);
  text-decoration: none;
}

.runtime-links a:hover {
  text-decoration: underline;
}

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
