<template>
  <UiSection :title="t('meshAddr.paths.title')" :description="t('meshAddr.paths.description')" heading-tag="h2" data-testid="mesh-paths">
    <div class="mesh-toolbar">
      <span />
      <UiButton variant="primary" :icon="Plus" :disabled="!sourceOptions.length" data-testid="mesh-path-add" @click="openPath(null)">{{ t('meshAddr.paths.add') }}</UiButton>
    </div>
    <UiDataTable
      :columns="columns"
      :rows="paths || []"
      row-key="id"
      :label="t('meshAddr.paths.title')"
      :row-label="row => row.source_name || row.source"
      :row-actions="actions"
      :loading="!paths && !loadError"
      :error="paths ? null : loadError"
      :error-title="t('meshAddr.paths.loadFailed')"
      :empty-title="t('meshAddr.paths.empty')"
      data-testid="mesh-path-table"
      @retry="load"
    >
      <template #cell-source="{ row }">
        <span class="mesh-cell">
          <span>{{ row.source_name || row.source }}</span>
          <span class="mesh-muted">{{ regionName(row.region_id) }}</span>
        </span>
      </template>
      <template #cell-target="{ row }">
        <span class="mesh-cell">
          <span :class="{ 'mesh-mono': row.target_kind === 'subnet' }">{{ row.target_kind === 'subnet' ? row.target : row.target_name || row.target }}</span>
        </span>
      </template>
      <template #cell-via="{ row }">
        <span class="mesh-cell">
          <span>{{ row.via.map(hop => hop.name || hop.entry).join(' → ') }}</span>
        </span>
      </template>
      <template #cell-state="{ row }">
        <span class="mesh-cell">
          <span class="mesh-badges">
            <UiBadge v-if="row.status" :tone="STATE_TONES[row.status.state] || 'neutral'" :label="stateLabel(row.status.state)" />
            <UiBadge v-else tone="neutral" :label="t('meshAddr.paths.noReport')" />
            <UiBadge v-for="code in row.warnings || []" :key="code" tone="warning" :label="t(`meshAddr.paths.warnings.${code}`)" />
          </span>
          <span v-if="reasonText(row.status)" class="mesh-muted">{{ reasonText(row.status) }}</span>
          <span v-if="unreachableNames(row).length" class="mesh-muted">{{ t('meshAddr.paths.unreachable', { name: unreachableNames(row).join('、') }) }}</span>
        </span>
      </template>
    </UiDataTable>
  </UiSection>

  <UiSheet :open="Boolean(form)" size="md" :title="form?.id ? t('meshAddr.paths.editTitle', { name: form.original }) : t('meshAddr.paths.addTitle')" :dismissible="!saving" @update:open="value => { if (!value) form = null }">
    <form v-if="form" class="mesh-form" novalidate data-testid="mesh-path-form" @submit.prevent="save">
      <UiSelect v-model="form.source" size="md" :label="t('meshAddr.paths.source')" :help="t('meshAddr.paths.sourceHelp')" :options="formSourceOptions" :disabled="Boolean(form.id)" required :error="form.errors.source" />
      <UiSelect v-model="form.target" size="md" :label="t('meshAddr.paths.target')" :help="t('meshAddr.paths.targetHelp')" :options="targetOptions" required :error="form.errors.target" />
      <UiTextField v-if="form.target === SUBNET" v-model="form.subnet" size="md" :label="t('meshAddr.paths.subnet')" :help="t('meshAddr.paths.subnetHelp')" required :error="form.errors.subnet" placeholder="192.168.50.0/24" data-testid="mesh-path-subnet" />
      <UiTextarea v-model="form.via" :rows="4" :label="t('meshAddr.paths.via')" :help="viaHelp" required :error="form.errors.via" placeholder="node:proxy-2" data-testid="mesh-path-via" />
      <p v-if="saveError" class="mesh-error" role="alert">{{ saveError }}</p>
    </form>
    <template #footer>
      <UiButton :disabled="saving" @click="form = null">{{ t('meshAddr.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="mesh-path-save" @click="save">{{ t('meshAddr.common.save') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// 组网 → 自定义路径: for one source member of a region, the path to a peer
// or a subnet through 1 to 6 waypoints (full nodes), in order. Members are
// kept by identity (node:<node>, device:<id>); a path whose member no longer
// resolves is not delivered and the row says so.
import { computed, onMounted, ref, watch } from 'vue'
import { Pencil, Plus, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiSection, UiSelect, UiSheet, UiTextarea, UiTextField, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteKernelMeshPath, listKernelMeshDevices, listKernelMeshPaths, saveKernelMeshPath } from '@/api/kernel'
import { meshErrorMessage } from '../meshModel'

const props = defineProps({
  regions: { type: Array, default: () => [] },
  // The nodes in the mesh: home memberships with node_ref, node_name,
  // address and region.
  members: { type: Array, default: () => [] }
})
const emit = defineEmits(['changed'])
const { t, te } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

// The target option that asks for a subnet.
const SUBNET = '__subnet'
const STATE_TONES = { active: 'success', first_hop_only: 'warning', fallback: 'warning', pending: 'neutral' }

const paths = ref(null)
const devices = ref([])
const loadError = ref(null)
const form = ref(null)
const saving = ref(false)
const saveError = ref('')

const regionName = id => props.regions.find(region => region.id === id)?.name || '—'
const isFull = device => device.role === 'full'

// Every member that may be a source or a waypoint (nodes with an Agent and
// full node devices), with its region.
const relays = computed(() => [
  ...props.members.map(member => ({ value: `node:${member.node_ref}`, label: member.node_name || member.node_ref, region: member.region?.id || 0 })),
  ...devices.value.filter(isFull).map(device => ({ value: `device:${device.id}`, label: device.hostname, region: device.region_id }))
])
// Every member of a region, for the target.
const peers = computed(() => [
  ...relays.value,
  ...devices.value.filter(device => !isFull(device)).map(device => ({ value: `device:${device.id}`, label: device.hostname, region: device.region_id }))
])
const sourceOptions = computed(() => relays.value.map(item => ({ value: item.value, label: `${item.label} · ${regionName(item.region)}` })))
const formSourceOptions = computed(() => (form.value?.id ? sourceOptions.value.filter(option => option.value === form.value.source) : sourceOptions.value))
const formRegion = computed(() => relays.value.find(item => item.value === form.value?.source)?.region || 0)
const peerChoices = computed(() => peers.value.filter(item => item.region === formRegion.value && item.value !== form.value?.source))
const targetOptions = computed(() => [
  ...peerChoices.value.map(item => ({ value: item.value, label: item.label })),
  { value: SUBNET, label: t('meshAddr.paths.targetSubnet') }
])
const viaChoices = computed(() => relays.value.filter(item => item.region === formRegion.value && item.value !== form.value?.source))
const viaHelp = computed(() => (viaChoices.value.length
  ? t('meshAddr.paths.viaHelp', { options: viaChoices.value.map(item => `${item.label} = ${item.value}`).join('；') })
  : t('meshAddr.paths.viaEmpty')))

// A new source has other targets: keep the target only while it is one.
watch(() => form.value?.source, () => {
  if (!form.value || form.value.target === SUBNET) return
  if (!peerChoices.value.some(item => item.value === form.value.target)) form.value.target = peerChoices.value[0]?.value || SUBNET
})

const columns = computed(() => [
  { key: 'source', label: t('meshAddr.paths.source'), primary: true },
  { key: 'target', label: t('meshAddr.paths.target'), secondary: true },
  { key: 'via', label: t('meshAddr.paths.via'), breakpoint: 'md' },
  { key: 'state', label: t('meshAddr.paths.state') }
])

const stateLabel = state => (te(`meshAddr.paths.states.${state}`) ? t(`meshAddr.paths.states.${state}`) : String(state || ''))
const reasonText = status => {
  const reason = typeof status?.reason === 'string' ? status.reason : ''
  if (!reason) return ''
  return te(`meshAddr.paths.reasons.${reason}`) ? t(`meshAddr.paths.reasons.${reason}`) : reason
}
// The waypoints the kernel could not reach, by name when the address is
// one of the path's waypoints.
const unreachableNames = row => {
  const raw = row.status?.unreachable
  const list = Array.isArray(raw) ? raw : (raw === null || raw === undefined || raw === '' ? [] : [raw])
  return list
    .map(item => (typeof item === 'string' ? item : (item?.address || item?.id || '')))
    .filter(Boolean)
    .map(address => row.via.find(hop => hop.address === address)?.name || address)
}

async function load() {
  loadError.value = null
  try {
    const [pathList, deviceList] = await Promise.all([listKernelMeshPaths(), listKernelMeshDevices()])
    paths.value = pathList || []
    devices.value = deviceList || []
  } catch (e) {
    loadError.value = { message: meshErrorMessage(e) }
  }
}

function openPath(path) {
  saveError.value = ''
  const source = path?.source || sourceOptions.value[0]?.value || ''
  const subnet = path?.target_kind === 'subnet'
  form.value = {
    id: path?.id || null,
    original: path?.source_name || path?.source || '',
    source,
    target: path ? (subnet ? SUBNET : path.target) : '',
    subnet: subnet ? path.target : '',
    via: (path?.via || []).map(hop => hop.entry).join('\n'),
    errors: {}
  }
  if (!path) form.value.target = peerChoices.value[0]?.value || SUBNET
}

async function save() {
  const current = form.value
  if (!current || saving.value) return
  current.errors = {}
  if (!current.source) current.errors.source = t('meshAddr.common.required')
  if (current.target === SUBNET && !current.subnet.trim()) current.errors.subnet = t('meshAddr.common.required')
  if (!current.target) current.errors.target = t('meshAddr.common.required')
  const via = current.via.split(/[\s,]+/).filter(Boolean)
  if (!via.length) current.errors.via = t('meshAddr.common.required')
  if (Object.keys(current.errors).length) return
  saving.value = true
  saveError.value = ''
  try {
    await saveKernelMeshPath(current.id, { source: current.source, target: current.target === SUBNET ? current.subnet : current.target, via })
    toast.success(t('meshAddr.common.saved'))
    form.value = null
    await load()
    emit('changed')
  } catch (e) {
    saveError.value = meshErrorMessage(e)
  } finally {
    saving.value = false
  }
}

function actions(row) {
  const name = row.source_name || row.source
  return [
    { key: 'edit', label: t('meshAddr.paths.edit'), icon: Pencil, onSelect: () => openPath(row) },
    {
      key: 'delete',
      label: t('meshAddr.common.delete'),
      icon: Trash2,
      danger: true,
      separatorBefore: true,
      onSelect: () => confirm({
        title: t('meshAddr.paths.deleteTitle', { name }),
        message: t('meshAddr.paths.deleteMessage'),
        confirmLabel: t('meshAddr.common.deleteConfirm'),
        tone: 'danger',
        onConfirm: async () => {
          try {
            await deleteKernelMeshPath(row.id)
          } catch (e) {
            throw new Error(meshErrorMessage(e))
          }
          toast.success(t('meshAddr.common.deleted'))
          await load()
          emit('changed')
        }
      })
    }
  ]
}

onMounted(load)
</script>

<style scoped src="./meshAddr.css"></style>
