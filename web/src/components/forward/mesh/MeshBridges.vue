<template>
  <UiSection :title="t('meshAddr.bridges.title')" :description="t('meshAddr.bridges.description')" heading-tag="h2" data-testid="mesh-bridges">
    <div class="mesh-toolbar">
      <span />
      <UiButton variant="primary" :icon="Plus" :disabled="!candidates.length" data-testid="mesh-bridge-add" @click="openBridge(null)">{{ t('meshAddr.bridges.add') }}</UiButton>
    </div>
    <UiDataTable
      :columns="columns"
      :rows="bridges || []"
      row-key="id"
      :label="t('meshAddr.bridges.title')"
      :row-label="row => row.node_name || row.node_ref"
      :row-actions="actions"
      :loading="!bridges && !loadError"
      :error="bridges ? null : loadError"
      :error-title="t('meshAddr.bridges.loadFailed')"
      :empty-title="t('meshAddr.bridges.empty')"
      data-testid="mesh-bridge-table"
      @retry="load"
    >
      <template #cell-node="{ row }">
        <span class="mesh-cell">
          <span>{{ row.node_name || row.node_ref }}</span>
          <span class="mesh-muted">{{ regionName(row.region_id) }} · <span class="mesh-mono">{{ row.interface }}</span></span>
        </span>
      </template>
      <template #cell-routes="{ row }">
        <span class="mesh-cell">
          <span v-for="route in row.routes" :key="route" class="mesh-mono">{{ route }}</span>
          <span class="mesh-badges"><UiBadge tone="neutral" :label="t('meshAddr.bridges.externalNetwork')" /></span>
        </span>
      </template>
      <template #cell-allow="{ row }">
        <span class="mesh-cell">
          <span>{{ t('meshAddr.bridges.allowCount', { n: row.allow.length }, row.allow.length) }}</span>
          <span v-if="row.allow.length" class="mesh-muted">{{ row.allow.map(entryLabel).join('、') }}</span>
        </span>
      </template>
      <template #cell-state="{ row }">
        <span class="mesh-cell">
          <span class="mesh-badges">
            <UiBadge v-if="!(row.warnings || []).includes('bridge_unconfirmed')" tone="success" :label="t('meshAddr.bridges.confirmed')" />
            <UiBadge v-for="code in row.warnings || []" :key="code" tone="warning" :label="t(`meshAddr.bridges.warnings.${code}`)" />
          </span>
          <span v-if="row.held?.length" class="mesh-muted mesh-mono">{{ t('meshAddr.bridges.held', { addresses: row.held.join(', ') }) }}</span>
        </span>
      </template>
    </UiDataTable>
  </UiSection>

  <UiSheet :open="Boolean(form)" size="md" :title="form?.id ? t('meshAddr.bridges.editTitle', { name: form.original }) : t('meshAddr.bridges.addTitle')" :dismissible="!saving" @update:open="value => { if (!value) form = null }">
    <form v-if="form" class="mesh-form" novalidate data-testid="mesh-bridge-form" @submit.prevent="save">
      <UiSelect v-model="form.node" size="md" :label="t('meshAddr.bridges.node')" :help="t('meshAddr.bridges.nodeHelp')" :options="nodeOptions" :disabled="Boolean(form.id)" required :error="form.errors.node" />
      <UiTextField v-model="form.iface" size="md" :label="t('meshAddr.bridges.interface')" :help="t('meshAddr.bridges.interfaceHelp')" required :error="form.errors.iface" placeholder="ext0" />
      <UiTextarea v-model="form.routes" :rows="5" :label="t('meshAddr.bridges.routes')" :help="t('meshAddr.bridges.routesHelp')" required :error="form.errors.routes" placeholder="100.100.1.2&#10;192.168.50.0/24" data-testid="mesh-bridge-routes" />
      <fieldset class="mesh-allow" data-testid="mesh-bridge-allow">
        <legend>{{ t('meshAddr.bridges.allow') }}</legend>
        <p class="mesh-muted">{{ t('meshAddr.bridges.allowHelp') }}</p>
        <p v-if="!allowOptions.length" class="mesh-muted">{{ t('meshAddr.bridges.allowEmpty') }}</p>
        <UiCheckbox
          v-for="option in allowOptions"
          :key="option.value"
          :model-value="form.allow.includes(option.value)"
          :label="option.label"
          :description="option.description"
          @update:model-value="checked => toggle(option.value, checked)"
        />
      </fieldset>
      <p v-if="saveError" class="mesh-error" role="alert">{{ saveError }}</p>
    </form>
    <template #footer>
      <UiButton :disabled="saving" @click="form = null">{{ t('meshAddr.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="mesh-bridge-save" @click="save">{{ t('meshAddr.common.save') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// 组网 → 外部网络桥接 (R36, spec section 15.12): a node that is also on
// another overlay network advertises chosen external addresses in its home
// region to the nodes, devices and host groups it allows. An address that
// leaves an allow list is given out again only after the node confirms.
import { computed, onMounted, ref } from 'vue'
import { Pencil, Plus, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiCheckbox, UiDataTable, UiSection, UiSelect, UiSheet, UiTextarea, UiTextField, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { deleteKernelMeshBridge, listKernelMeshBridges, listKernelMeshDevices, listKernelMeshHostGroups, saveKernelMeshBridge } from '@/api/kernel'
import { meshErrorMessage } from '../meshModel'

const props = defineProps({
  regions: { type: Array, default: () => [] },
  // The nodes in the mesh: home memberships with node_ref, node_name and
  // region.
  members: { type: Array, default: () => [] }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const bridges = ref(null)
const devices = ref([])
const groups = ref([])
const loadError = ref(null)
const form = ref(null)
const saving = ref(false)
const saveError = ref('')

const regionName = id => props.regions.find(region => region.id === id)?.name || '—'
const bridged = computed(() => new Set((bridges.value || []).map(bridge => bridge.node_ref)))
const candidates = computed(() => props.members.filter(member => !bridged.value.has(member.node_ref)))
const nodeOptions = computed(() => {
  const list = form.value?.id ? props.members.filter(member => member.node_ref === form.value.node) : candidates.value
  return list.map(member => ({ value: member.node_ref, label: `${member.node_name || member.node_ref} · ${member.region?.name || ''}` }))
})
const formRegion = computed(() => props.members.find(member => member.node_ref === form.value?.node)?.region?.id || 0)
const allowOptions = computed(() => {
  const region = formRegion.value
  if (!region) return []
  return [
    ...props.members.filter(member => member.region?.id === region && member.node_ref !== form.value?.node)
      .map(member => ({ value: `node:${member.node_ref}`, label: member.node_name || member.node_ref, description: `${t('meshAddr.bridges.kinds.node')} · ${member.address}` })),
    ...devices.value.filter(device => device.region_id === region)
      .map(device => ({ value: `device:${device.id}`, label: device.hostname, description: `${t('meshAddr.bridges.kinds.device')} · ${device.address || '—'}` })),
    ...groups.value.filter(group => group.region_id === region)
      .map(group => ({ value: `host_group:${group.id}`, label: group.name, description: `${t('meshAddr.bridges.kinds.host_group')} · ${group.segment}` }))
  ]
})
const entryLabel = entry => entry.name || entry.entry

const columns = computed(() => [
  { key: 'node', label: t('meshAddr.bridges.node'), primary: true },
  { key: 'routes', label: t('meshAddr.bridges.routes'), secondary: true },
  { key: 'allow', label: t('meshAddr.bridges.allow'), breakpoint: 'md' },
  { key: 'state', label: t('meshAddr.bridges.state') }
])

async function load() {
  loadError.value = null
  try {
    const [bridgeList, deviceList, groupList] = await Promise.all([listKernelMeshBridges(), listKernelMeshDevices(), listKernelMeshHostGroups()])
    bridges.value = bridgeList || []
    devices.value = deviceList || []
    groups.value = groupList || []
  } catch (e) {
    loadError.value = { message: meshErrorMessage(e) }
  }
}

function openBridge(bridge) {
  saveError.value = ''
  form.value = {
    id: bridge?.id || null,
    original: bridge?.node_name || bridge?.node_ref || '',
    node: bridge?.node_ref || candidates.value[0]?.node_ref || '',
    iface: bridge?.interface || '',
    routes: (bridge?.routes || []).join('\n'),
    allow: (bridge?.allow || []).map(entry => entry.entry),
    errors: {}
  }
}

function toggle(value, checked) {
  const allow = form.value.allow.filter(entry => entry !== value)
  if (checked) allow.push(value)
  form.value.allow = allow
}

async function save() {
  const current = form.value
  if (!current || saving.value) return
  current.errors = {}
  if (!current.node) current.errors.node = t('meshAddr.common.required')
  if (!current.iface.trim()) current.errors.iface = t('meshAddr.common.required')
  const routes = current.routes.split(/[\s,]+/).filter(Boolean)
  if (!routes.length) current.errors.routes = t('meshAddr.common.required')
  if (Object.keys(current.errors).length) return
  saving.value = true
  saveError.value = ''
  try {
    await saveKernelMeshBridge(current.id, { node: current.node, iface: current.iface, routes, allow: current.allow })
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
  const name = row.node_name || row.node_ref
  return [
    { key: 'edit', label: t('meshAddr.bridges.edit'), icon: Pencil, onSelect: () => openBridge(row) },
    {
      key: 'delete',
      label: t('meshAddr.common.delete'),
      icon: Trash2,
      danger: true,
      separatorBefore: true,
      onSelect: () => confirm({
        title: t('meshAddr.bridges.deleteTitle', { name }),
        message: t('meshAddr.bridges.deleteMessage'),
        confirmLabel: t('meshAddr.common.deleteConfirm'),
        tone: 'danger',
        onConfirm: async () => {
          try {
            await deleteKernelMeshBridge(row.id)
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
<style scoped>
.mesh-allow {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  border: 0;
  min-width: 0;
}

.mesh-allow legend {
  padding: 0;
  font-weight: var(--weight-semibold);
}

.mesh-allow p {
  margin: 0;
}
</style>
