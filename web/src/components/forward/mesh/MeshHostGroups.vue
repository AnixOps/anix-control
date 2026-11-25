<template>
  <UiSection :title="t('meshAddr.hostGroups.title')" :description="t('meshAddr.hostGroups.description')" heading-tag="h2" data-testid="mesh-host-groups">
    <div class="mesh-toolbar">
      <span />
      <UiButton variant="primary" :icon="Plus" :disabled="!members.length" data-testid="mesh-host-group-add" @click="openGroup(null)">{{ t('meshAddr.hostGroups.add') }}</UiButton>
    </div>
    <UiDataTable
      :columns="groupColumns"
      :rows="groups || []"
      row-key="id"
      :label="t('meshAddr.hostGroups.title')"
      :row-label="row => row.name"
      :row-actions="groupActions"
      :loading="!groups && !loadError"
      :error="groups ? null : loadError"
      :error-title="t('meshAddr.hostGroups.loadFailed')"
      :empty-title="t('meshAddr.hostGroups.empty')"
      data-testid="mesh-host-group-table"
      @retry="load"
    >
      <template #cell-group="{ row }">
        <span class="mesh-cell">
          <span>{{ row.name }}</span>
          <span class="mesh-muted mesh-mono">{{ row.segment }}</span>
          <span v-if="(row.warnings || []).includes('shared_address_space')" class="mesh-badges">
            <UiBadge tone="warning" :label="t('meshAddr.common.sharedAddressSpace')" />
          </span>
        </span>
      </template>
      <template #cell-gateway="{ row }">
        <span class="mesh-cell">
          <span>{{ nodeName(row.gateway_ref) }}</span>
          <span v-if="row.standby_ref" class="mesh-muted">{{ t('meshAddr.hostGroups.standby') }} {{ nodeName(row.standby_ref) }}</span>
        </span>
      </template>
      <template #cell-region="{ row }">{{ regionName(row.region_id) }}</template>
    </UiDataTable>
  </UiSection>

  <UiSection :title="t('meshAddr.deviceNames.title')" :description="t('meshAddr.deviceNames.description')" heading-tag="h2" data-testid="mesh-device-names">
    <div class="mesh-toolbar">
      <span />
      <UiButton :icon="Plus" :disabled="!members.length" data-testid="mesh-device-name-add" @click="openName(null)">{{ t('meshAddr.deviceNames.add') }}</UiButton>
    </div>
    <UiDataTable
      :columns="nameColumns"
      :rows="names || []"
      row-key="id"
      :label="t('meshAddr.deviceNames.title')"
      :row-label="row => row.name"
      :row-actions="nameActions"
      :loading="!names && !loadError"
      :empty-title="t('meshAddr.deviceNames.empty')"
      data-testid="mesh-device-name-table"
    >
      <template #cell-name="{ row }">
        <span class="mesh-cell">
          <span>{{ row.name }}</span>
          <span class="mesh-muted mesh-mono">{{ row.address }}</span>
        </span>
      </template>
      <template #cell-kind="{ row }">{{ row.host_group_id ? groupName(row.host_group_id) : t('meshAddr.deviceNames.localNetwork') }}</template>
      <template #cell-gateway="{ row }">{{ nodeName(row.gateway_ref) }}</template>
    </UiDataTable>
  </UiSection>

  <UiSheet :open="Boolean(groupForm)" size="md" :title="groupForm?.id ? t('meshAddr.hostGroups.editTitle', { name: groupForm.original }) : t('meshAddr.hostGroups.addTitle')" :dismissible="!saving" @update:open="value => { if (!value) groupForm = null }">
    <form v-if="groupForm" class="mesh-form" novalidate data-testid="mesh-host-group-form" @submit.prevent="saveGroup">
      <UiTextField v-model="groupForm.name" size="md" :label="t('meshAddr.common.name')" required :error="groupForm.errors.name" />
      <UiTextField v-model="groupForm.segment" size="md" :label="t('meshAddr.hostGroups.segment')" :help="t('meshAddr.hostGroups.segmentHelp')" :disabled="Boolean(groupForm.id)" placeholder="10.127.0.0/24" />
      <UiSelect v-model="groupForm.gateway" size="md" :label="t('meshAddr.hostGroups.gateway')" :options="nodeOptions" required :error="groupForm.errors.gateway" />
      <UiSelect v-model="groupForm.standby" size="md" :label="t('meshAddr.hostGroups.standby')" :help="t('meshAddr.hostGroups.standbyHelp')" :options="standbyOptions" />
      <p v-if="saveError" class="mesh-error" role="alert">{{ saveError }}</p>
    </form>
    <template #footer>
      <UiButton :disabled="saving" @click="groupForm = null">{{ t('meshAddr.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="mesh-host-group-save" @click="saveGroup">{{ t('meshAddr.common.save') }}</UiButton>
    </template>
  </UiSheet>

  <UiSheet :open="Boolean(nameForm)" size="md" :title="nameForm?.id ? t('meshAddr.deviceNames.editTitle', { name: nameForm.original }) : t('meshAddr.deviceNames.addTitle')" :description="t('meshAddr.deviceNames.addressHelp')" :dismissible="!saving" @update:open="value => { if (!value) nameForm = null }">
    <form v-if="nameForm" class="mesh-form" novalidate data-testid="mesh-device-name-form" @submit.prevent="saveName">
      <UiTextField v-model="nameForm.name" size="md" :label="t('meshAddr.common.name')" required :error="nameForm.errors.name" />
      <UiSelect v-model="nameForm.location" size="md" :label="t('meshAddr.deviceNames.kind')" :options="locationOptions" required />
      <UiSelect v-if="nameForm.location === 'local'" v-model="nameForm.gateway" size="md" :label="t('meshAddr.hostGroups.gateway')" :options="nodeOptions" required :error="nameForm.errors.gateway" />
      <UiTextField v-model="nameForm.address" size="md" :label="t('meshAddr.common.address')" required :error="nameForm.errors.address" />
      <p v-if="saveError" class="mesh-error" role="alert">{{ saveError }}</p>
    </form>
    <template #footer>
      <UiButton :disabled="saving" @click="nameForm = null">{{ t('meshAddr.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="mesh-device-name-save" @click="saveName">{{ t('meshAddr.common.save') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// 组网 → 宿主组: host groups behind gateway nodes (R21) and the names gateway
// nodes publish for the hosts behind them (R25, R26).
import { computed, onMounted, ref } from 'vue'
import { Pencil, Plus, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiSection, UiSelect, UiSheet, UiTextField, useConfirm, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  deleteKernelMeshDeviceName, deleteKernelMeshHostGroup, listKernelMeshDeviceNames, listKernelMeshHostGroups, saveKernelMeshDeviceName, saveKernelMeshHostGroup
} from '@/api/kernel'
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

const groups = ref(null)
const names = ref(null)
const loadError = ref(null)
const groupForm = ref(null)
const nameForm = ref(null)
const saving = ref(false)
const saveError = ref('')

const nodeName = nodeRef => props.members.find(member => member.node_ref === nodeRef)?.node_name || nodeRef
const regionName = id => props.regions.find(region => region.id === id)?.name || '—'
const groupName = id => groups.value?.find(group => group.id === id)?.name || String(id)
const nodeOptions = computed(() => props.members.map(member => ({ value: member.node_ref, label: `${member.node_name || member.node_ref} · ${member.region?.name || ''}` })))
const standbyOptions = computed(() => {
  const gateway = props.members.find(member => member.node_ref === groupForm.value?.gateway)
  const sameRegion = props.members.filter(member => member.node_ref !== gateway?.node_ref && member.region?.id === gateway?.region?.id)
  return [{ value: '', label: t('meshAddr.common.none') }, ...sameRegion.map(member => ({ value: member.node_ref, label: member.node_name || member.node_ref }))]
})
const locationOptions = computed(() => [
  ...(groups.value || []).map(group => ({ value: String(group.id), label: `${t('meshAddr.deviceNames.hostGroup')} · ${group.name} (${group.segment})` })),
  { value: 'local', label: t('meshAddr.deviceNames.localNetwork') }
])

const groupColumns = computed(() => [
  { key: 'group', label: t('meshAddr.common.name'), primary: true },
  { key: 'gateway', label: t('meshAddr.hostGroups.gateway'), secondary: true },
  { key: 'region', label: t('meshAddr.common.region'), breakpoint: 'md' }
])
const nameColumns = computed(() => [
  { key: 'name', label: t('meshAddr.common.name'), primary: true },
  { key: 'kind', label: t('meshAddr.deviceNames.kind'), secondary: true },
  { key: 'gateway', label: t('meshAddr.hostGroups.gateway'), breakpoint: 'md' }
])

async function load() {
  loadError.value = null
  try {
    const [groupList, nameList] = await Promise.all([listKernelMeshHostGroups(), listKernelMeshDeviceNames()])
    groups.value = groupList || []
    names.value = nameList || []
  } catch (e) {
    loadError.value = { message: meshErrorMessage(e) }
  }
}

function openGroup(group) {
  saveError.value = ''
  groupForm.value = {
    id: group?.id || null,
    original: group?.name || '',
    name: group?.name || '',
    segment: group?.segment || '',
    gateway: group?.gateway_ref || props.members[0]?.node_ref || '',
    standby: group?.standby_ref || '',
    errors: {}
  }
}

function openName(name) {
  saveError.value = ''
  nameForm.value = {
    id: name?.id || null,
    original: name?.name || '',
    name: name?.name || '',
    location: name?.host_group_id ? String(name.host_group_id) : (name ? 'local' : (groups.value?.[0] ? String(groups.value[0].id) : 'local')),
    gateway: name?.gateway_ref || props.members[0]?.node_ref || '',
    address: name?.address || '',
    errors: {}
  }
}

async function submit(action, close) {
  saving.value = true
  saveError.value = ''
  try {
    await action()
    toast.success(t('meshAddr.common.saved'))
    close()
    await load()
    emit('changed')
  } catch (e) {
    saveError.value = meshErrorMessage(e)
  } finally {
    saving.value = false
  }
}

function saveGroup() {
  const form = groupForm.value
  if (!form || saving.value) return
  form.errors = {}
  if (!form.name.trim()) form.errors.name = t('meshAddr.common.required')
  if (!form.gateway) form.errors.gateway = t('meshAddr.common.required')
  if (Object.keys(form.errors).length) return
  submit(() => saveKernelMeshHostGroup(form.id, form), () => { groupForm.value = null })
}

function saveName() {
  const form = nameForm.value
  if (!form || saving.value) return
  form.errors = {}
  if (!form.name.trim()) form.errors.name = t('meshAddr.common.required')
  if (!form.address.trim()) form.errors.address = t('meshAddr.common.required')
  if (form.location === 'local' && !form.gateway) form.errors.gateway = t('meshAddr.common.required')
  if (Object.keys(form.errors).length) return
  const local = form.location === 'local'
  submit(() => saveKernelMeshDeviceName(form.id, {
    name: form.name, address: form.address, gateway: local ? form.gateway : '', hostGroupId: local ? 0 : Number(form.location)
  }), () => { nameForm.value = null })
}

function deleteAction(title, message, remove) {
  return {
    key: 'delete',
    label: t('meshAddr.common.delete'),
    icon: Trash2,
    danger: true,
    separatorBefore: true,
    onSelect: () => confirm({
      title,
      message,
      confirmLabel: t('meshAddr.common.deleteConfirm'),
      tone: 'danger',
      onConfirm: async () => {
        try {
          await remove()
        } catch (e) {
          throw new Error(meshErrorMessage(e))
        }
        toast.success(t('meshAddr.common.deleted'))
        await load()
        emit('changed')
      }
    })
  }
}

function groupActions(row) {
  return [
    { key: 'edit', label: t('meshAddr.hostGroups.edit'), icon: Pencil, onSelect: () => openGroup(row) },
    deleteAction(t('meshAddr.hostGroups.deleteTitle', { name: row.name }), t('meshAddr.hostGroups.deleteMessage'), () => deleteKernelMeshHostGroup(row.id))
  ]
}

function nameActions(row) {
  return [
    { key: 'edit', label: t('meshAddr.deviceNames.edit'), icon: Pencil, onSelect: () => openName(row) },
    deleteAction(t('meshAddr.deviceNames.deleteTitle', { name: row.name }), t('meshAddr.deviceNames.deleteMessage'), () => deleteKernelMeshDeviceName(row.id))
  ]
}

onMounted(load)
</script>

<style scoped src="./meshAddr.css"></style>
