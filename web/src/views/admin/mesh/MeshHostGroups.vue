<template>
  <div class="mesh-stack">
    <UiSection :title="t('mesh.hostGroups.title')" :description="t('mesh.hostGroups.description')" heading-tag="h2" data-testid="mesh-host-groups">
      <template #actions>
        <UiButton variant="primary" size="sm" :icon="Plus" :disabled="!gateways.length" data-testid="mesh-host-group-create" @click="openGroup(null)">{{ t('mesh.hostGroups.create') }}</UiButton>
      </template>
      <p v-if="loaded && !gateways.length" class="mesh-hint">{{ t('mesh.hostGroups.noGateway') }}</p>
      <UiDataTable
        :columns="groupColumns"
        :rows="groups || []"
        :label="t('mesh.hostGroups.title')"
        :loading="!loaded && !loadError"
        :error="loaded ? null : loadError"
        :error-title="t('mesh.hostGroups.loadFailed')"
        :empty-title="t('mesh.hostGroups.empty')"
        :settings="false"
        :row-actions="groupActions"
        @retry="load"
      >
        <template #cell-segment="{ row }"><span class="mesh-mono">{{ row.segment }}</span></template>
        <template #cell-gateways="{ row }">
          <span class="mesh-cell">
            <span>{{ row.gateway_hostname }}</span>
            <span v-if="row.standby_hostname" class="mesh-muted">{{ t('mesh.hostGroups.standbyOf', { name: row.standby_hostname }) }}</span>
          </span>
        </template>
      </UiDataTable>
    </UiSection>

    <UiSection :title="t('mesh.deviceNames.title')" :description="t('mesh.deviceNames.description')" heading-tag="h2" data-testid="mesh-device-names">
      <template #actions>
        <UiButton size="sm" :icon="Plus" :disabled="!gateways.length" data-testid="mesh-device-name-create" @click="openName(null)">{{ t('mesh.deviceNames.create') }}</UiButton>
      </template>
      <UiDataTable
        :columns="nameColumns"
        :rows="names || []"
        :label="t('mesh.deviceNames.title')"
        :loading="!loaded && !loadError"
        :empty-title="t('mesh.deviceNames.empty')"
        :settings="false"
        :row-actions="nameActions"
      >
        <template #cell-address="{ row }"><span class="mesh-mono">{{ row.address }}</span></template>
        <template #cell-place="{ row }">{{ placeOf(row) }}</template>
      </UiDataTable>
    </UiSection>

    <UiDialog :open="Boolean(group)" :title="group?.id ? t('mesh.hostGroups.editTitle') : t('mesh.hostGroups.createTitle')" @update:open="group = null">
      <form v-if="group" class="mesh-form" novalidate @submit.prevent="saveGroup">
        <UiTextField v-model="group.name" size="md" :label="t('mesh.hostGroups.name')" data-testid="mesh-host-group-name" />
        <div class="mesh-pair">
          <UiSelect v-model="group.gateway_member_id" size="md" :label="t('mesh.hostGroups.gateway')" :options="gatewayOptions" data-testid="mesh-host-group-gateway" />
          <UiSelect v-model="group.standby_member_id" size="md" :label="t('mesh.hostGroups.standby')" :options="standbyOptions" />
        </div>
        <UiTextField
          v-model="group.segment"
          size="md"
          :label="t('mesh.hostGroups.segment')"
          :help="t('mesh.hostGroups.segmentHelp')"
          :disabled="Boolean(group.id)"
          placeholder="10.127.0.0/24"
        />
        <p v-if="groupError" class="mesh-error" role="alert">{{ groupError }}</p>
      </form>
      <template #footer>
        <UiButton @click="group = null">{{ t('mesh.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" :disabled="saving" data-testid="mesh-host-group-save" @click="saveGroup">{{ t('mesh.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog :open="Boolean(name)" :title="name?.id ? t('mesh.deviceNames.editTitle') : t('mesh.deviceNames.createTitle')" @update:open="name = null">
      <form v-if="name" class="mesh-form" novalidate @submit.prevent="saveName">
        <UiSelect v-model="name.place" size="md" :label="t('mesh.deviceNames.place')" :options="placeOptions" data-testid="mesh-device-name-place" />
        <div class="mesh-pair">
          <UiTextField v-model="name.name" size="md" :label="t('mesh.deviceNames.name')" :help="t('mesh.members.hostnameHelp')" data-testid="mesh-device-name-name" />
          <UiTextField v-model="name.address" size="md" :label="t('mesh.deviceNames.address')" data-testid="mesh-device-name-address" />
        </div>
        <p class="mesh-hint">{{ t('mesh.deviceNames.manualOnly') }}</p>
        <p v-if="nameError" class="mesh-error" role="alert">{{ nameError }}</p>
      </form>
      <template #footer>
        <UiButton @click="name = null">{{ t('mesh.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" :disabled="saving || !name?.place" data-testid="mesh-device-name-save" @click="saveName">{{ t('mesh.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 组网 → 宿主组: the virtual machines of one physical host behind a
// gateway (and an optional standby gateway), with the group's segment; and
// the hand-made names of virtual machines and local devices the gateways
// publish.
import { computed, onMounted, ref } from 'vue'
import { Plus } from '@lucide/vue'
import {
  createMeshDeviceName,
  createMeshHostGroup,
  deleteMeshDeviceName,
  deleteMeshHostGroup,
  listMeshDeviceNames,
  listMeshHostGroups,
  listMeshMembers,
  meshErrorMessage,
  updateMeshDeviceName,
  updateMeshHostGroup
} from '@/api/mesh'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiDataTable, UiDialog, UiSection, UiSelect, UiTextField, useConfirm, useToast } from '@/ui'

defineProps({
  regions: { type: Array, required: true }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const confirm = useConfirm()
const toast = useToast()

const groups = ref(null)
const names = ref(null)
const members = ref([])
const loaded = ref(false)
const loadError = ref(null)
const group = ref(null)
const groupError = ref('')
const name = ref(null)
const nameError = ref('')
const saving = ref(false)

const gateways = computed(() => members.value.filter(member => member.role === 'gateway'))
const gatewayOptions = computed(() => gateways.value.map(member => ({ value: member.id, label: `${member.hostname} (${member.region_code})` })))
const standbyOptions = computed(() => [
  { value: 0, label: t('mesh.hostGroups.noStandby') },
  ...gatewayOptions.value.filter(option => option.value !== group.value?.gateway_member_id)
])
const placeOptions = computed(() => [
  ...(groups.value || []).map(item => ({ value: `group:${item.id}`, label: t('mesh.deviceNames.inGroup', { name: item.name, segment: item.segment }) })),
  ...gateways.value.map(member => ({ value: `gateway:${member.id}`, label: t('mesh.deviceNames.behindGateway', { name: member.hostname }) }))
])

const groupColumns = computed(() => [
  { key: 'name', label: t('mesh.hostGroups.name'), primary: true, hideable: false },
  { key: 'segment', label: t('mesh.hostGroups.segment') },
  { key: 'gateways', label: t('mesh.hostGroups.gateway') },
  { key: 'region_code', label: t('mesh.members.region'), breakpoint: 'md' },
  { key: 'names', label: t('mesh.hostGroups.names'), align: 'end', numeric: true, breakpoint: 'md' }
])
const nameColumns = computed(() => [
  { key: 'name', label: t('mesh.deviceNames.name'), primary: true, hideable: false },
  { key: 'address', label: t('mesh.deviceNames.address') },
  { key: 'place', label: t('mesh.deviceNames.place') }
])

function placeOf(row) {
  if (row.host_group_id) {
    const owner = (groups.value || []).find(item => item.id === row.host_group_id)
    return owner ? t('mesh.deviceNames.inGroup', { name: owner.name, segment: owner.segment }) : ''
  }
  const gateway = members.value.find(member => member.id === row.member_id)
  return gateway ? t('mesh.deviceNames.behindGateway', { name: gateway.hostname }) : ''
}

async function load() {
  loadError.value = null
  try {
    const [groupRows, nameRows, memberRows] = await Promise.all([listMeshHostGroups(), listMeshDeviceNames(), listMeshMembers()])
    groups.value = groupRows || []
    names.value = nameRows || []
    members.value = memberRows || []
    loaded.value = true
  } catch (e) {
    loadError.value = e
  }
}

function groupActions(row) {
  return [
    { key: 'edit', label: t('mesh.edit'), onSelect: () => openGroup(row) },
    { key: 'delete', label: t('mesh.delete'), danger: true, separatorBefore: true, onSelect: () => removeGroup(row) }
  ]
}

function nameActions(row) {
  return [
    { key: 'edit', label: t('mesh.edit'), onSelect: () => openName(row) },
    { key: 'delete', label: t('mesh.delete'), danger: true, separatorBefore: true, onSelect: () => removeName(row) }
  ]
}

function openGroup(row) {
  groupError.value = ''
  group.value = row
    ? { id: row.id, name: row.name, gateway_member_id: row.gateway_member_id, standby_member_id: row.standby_member_id || 0, segment: row.segment }
    : { id: 0, name: '', gateway_member_id: gateways.value[0]?.id, standby_member_id: 0, segment: '' }
}

async function saveGroup() {
  if (!group.value || saving.value) return
  saving.value = true
  groupError.value = ''
  const { id, ...body } = group.value
  try {
    if (id) await updateMeshHostGroup(id, body)
    else await createMeshHostGroup(body)
    toast.success(t('mesh.saved'))
    group.value = null
    await load()
    emit('changed')
  } catch (e) {
    groupError.value = meshErrorMessage(e, t('mesh.saveFailed'))
  } finally {
    saving.value = false
  }
}

function openName(row) {
  nameError.value = ''
  if (row) {
    name.value = { id: row.id, place: row.host_group_id ? `group:${row.host_group_id}` : `gateway:${row.member_id}`, name: row.name, address: row.address }
    return
  }
  name.value = { id: 0, place: placeOptions.value[0]?.value || '', name: '', address: '' }
}

async function saveName() {
  if (!name.value?.place || saving.value) return
  saving.value = true
  nameError.value = ''
  const [kind, raw] = name.value.place.split(':')
  const body = {
    name: name.value.name,
    address: name.value.address,
    host_group_id: kind === 'group' ? Number(raw) : 0,
    member_id: kind === 'gateway' ? Number(raw) : 0
  }
  try {
    if (name.value.id) await updateMeshDeviceName(name.value.id, body)
    else await createMeshDeviceName(body)
    toast.success(t('mesh.saved'))
    name.value = null
    await load()
  } catch (e) {
    nameError.value = meshErrorMessage(e, t('mesh.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function removeGroup(row) {
  const ok = await confirm({
    title: t('mesh.hostGroups.deleteTitle', { name: row.name }),
    message: t('mesh.hostGroups.deleteMessage'),
    confirmLabel: t('mesh.delete'),
    tone: 'danger',
    onConfirm: () => deleteMeshHostGroup(row.id)
  })
  if (!ok) return
  toast.success(t('mesh.deleted'))
  await load()
  emit('changed')
}

async function removeName(row) {
  const ok = await confirm({
    title: t('mesh.deviceNames.deleteTitle', { name: row.name }),
    confirmLabel: t('mesh.delete'),
    tone: 'danger',
    onConfirm: () => deleteMeshDeviceName(row.id)
  })
  if (!ok) return
  toast.success(t('mesh.deleted'))
  await load()
}

onMounted(load)
</script>

<style scoped src="./mesh.css"></style>
<style scoped>
.mesh-stack {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
</style>
