<template>
  <UiSection :title="t('mesh.members.title')" :description="t('mesh.members.description')" heading-tag="h2" data-testid="mesh-members">
    <template #actions>
      <UiButton variant="primary" size="sm" :icon="Plus" :disabled="!regions.length" data-testid="mesh-member-create" @click="openForm(null)">{{ t('mesh.members.create') }}</UiButton>
    </template>
    <div class="mesh-toolbar">
      <UiSelect v-model="regionFilter" size="md" :label="t('mesh.members.region')" :options="regionFilterOptions" data-testid="mesh-member-filter" />
    </div>
    <UiDataTable
      :columns="columns"
      :rows="members || []"
      :label="t('mesh.members.title')"
      :loading="!members && !loadError"
      :error="members ? null : loadError"
      :error-title="t('mesh.members.loadFailed')"
      :empty-title="t('mesh.members.empty')"
      :settings="false"
      :row-actions="rowActions"
      @retry="load"
    >
      <template #cell-hostname="{ row }">
        <span class="mesh-cell">
          <span>{{ row.hostname }}</span>
          <span class="mesh-muted mesh-mono">{{ row.fqdn }}</span>
        </span>
      </template>
      <template #cell-role="{ row }">{{ t(`mesh.roles.${row.role}`) }}</template>
      <template #cell-address="{ row }">
        <span class="mesh-cell">
          <span class="mesh-mono">{{ row.address || t('mesh.members.noAddress') }}</span>
          <span class="mesh-muted">{{ t(`mesh.addressModes.${row.address_mode}`) }}</span>
        </span>
      </template>
      <template #cell-node="{ row }">
        <span class="mesh-cell">
          <span class="mesh-mono">{{ row.node_ref || t('mesh.members.noAgent') }}</span>
          <span v-if="row.probe_address" class="mesh-muted mesh-mono">{{ row.probe_address }}</span>
        </span>
      </template>
      <template #cell-warnings="{ row }">
        <span class="mesh-badges">
          <UiBadge v-for="warning in row.warnings" :key="warning" tone="warning" :label="t(`mesh.members.warnings.${warning}`)" />
        </span>
      </template>
    </UiDataTable>

    <UiDialog :open="Boolean(form)" :title="form?.id ? t('mesh.members.editTitle') : t('mesh.members.createTitle')" @update:open="form = null">
      <form v-if="form" class="mesh-form" novalidate @submit.prevent="save">
        <div class="mesh-pair">
          <UiSelect v-model="form.region_id" size="md" :label="t('mesh.members.region')" :options="regionOptions" :disabled="Boolean(form.id)" data-testid="mesh-member-region" />
          <UiSelect v-model="form.role" size="md" :label="t('mesh.members.role')" :options="roleOptions" data-testid="mesh-member-role" />
        </div>
        <UiTextField v-model="form.hostname" size="md" :label="t('mesh.members.hostname')" :help="t('mesh.members.hostnameHelp')" data-testid="mesh-member-hostname" />
        <UiSelect v-if="form.role === 'device'" v-model="form.address_mode" size="md" :label="t('mesh.members.addressMode')" :options="modeOptions" />
        <UiTextField
          v-if="effectiveMode === 'fixed'"
          v-model="form.address"
          size="md"
          :label="t('mesh.members.address')"
          :help="t('mesh.members.addressHelp')"
          data-testid="mesh-member-address"
        />
        <UiTextField v-model="form.node_ref" size="md" :label="t('mesh.members.nodeRef')" placeholder="proxy-12" :help="t('mesh.members.nodeRefHelp')" />
        <UiTextField
          v-if="form.role === 'border'"
          v-model="form.probe_address"
          size="md"
          :label="t('mesh.members.probeAddress')"
          placeholder="203.0.113.10:11010"
          :help="t('mesh.members.probeAddressHelp')"
        />
        <p v-if="error" class="mesh-error" role="alert">{{ error }}</p>
      </form>
      <template #footer>
        <UiButton @click="form = null">{{ t('mesh.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" :disabled="saving" data-testid="mesh-member-save" @click="save">{{ t('mesh.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiDialog :open="Boolean(move)" :title="t('mesh.members.moveTitle', { name: move?.hostname || '' })" :description="t('mesh.members.moveDescription')" @update:open="move = null">
      <form v-if="move" class="mesh-form" novalidate @submit.prevent="saveMove">
        <UiSelect v-model="move.region_id" size="md" :label="t('mesh.members.targetRegion')" :options="moveOptions" data-testid="mesh-member-move-region" />
        <UiTextField v-if="move.address_mode === 'fixed'" v-model="move.address" size="md" :label="t('mesh.members.address')" :help="t('mesh.members.addressHelp')" />
        <UiTextField v-model="move.note" size="md" :label="t('mesh.note')" />
        <p v-if="moveError" class="mesh-error" role="alert">{{ moveError }}</p>
      </form>
      <template #footer>
        <UiButton @click="move = null">{{ t('mesh.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="moving" :disabled="moving || !move?.region_id" data-testid="mesh-member-move-save" @click="saveMove">{{ t('mesh.members.move') }}</UiButton>
      </template>
    </UiDialog>
  </UiSection>
</template>

<script setup>
// 组网 → 成员: infrastructure nodes with fixed addresses and devices with
// kept pool addresses; a move to another region changes the address.
import { computed, onMounted, ref, watch } from 'vue'
import { Plus } from '@lucide/vue'
import { createMeshMember, deleteMeshMember, listMeshMembers, meshErrorMessage, moveMeshMember, updateMeshMember } from '@/api/mesh'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiBadge, UiButton, UiDataTable, UiDialog, UiSection, UiSelect, UiTextField, useConfirm, useToast } from '@/ui'

const props = defineProps({
  regions: { type: Array, required: true }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const confirm = useConfirm()
const toast = useToast()

const members = ref(null)
const loadError = ref(null)
const regionFilter = ref(0)
const form = ref(null)
const saving = ref(false)
const error = ref('')
const move = ref(null)
const moving = ref(false)
const moveError = ref('')

const ROLES = ['border', 'gateway', 'server', 'device']

const regionOptions = computed(() => props.regions.map(region => ({ value: region.id, label: `${region.name} (${region.code})` })))
const regionFilterOptions = computed(() => [{ value: 0, label: t('mesh.members.allRegions') }, ...regionOptions.value])
const roleOptions = computed(() => ROLES.map(role => ({ value: role, label: t(`mesh.roles.${role}`) })))
const modeOptions = computed(() => ['pool', 'fixed'].map(mode => ({ value: mode, label: t(`mesh.addressModes.${mode}`) })))
const moveOptions = computed(() => regionOptions.value.filter(option => option.value !== move.value?.from))
const effectiveMode = computed(() => (form.value?.role === 'device' ? form.value.address_mode : 'fixed'))

const columns = computed(() => [
  { key: 'hostname', label: t('mesh.members.hostname'), primary: true, hideable: false },
  { key: 'role', label: t('mesh.members.role') },
  { key: 'address', label: t('mesh.members.address') },
  { key: 'region_code', label: t('mesh.members.region'), breakpoint: 'md' },
  { key: 'node', label: t('mesh.members.node'), breakpoint: 'md' },
  { key: 'warnings', label: t('mesh.members.status') }
])

function rowActions(row) {
  return [
    { key: 'edit', label: t('mesh.edit'), onSelect: () => openForm(row) },
    { key: 'move', label: t('mesh.members.move'), onSelect: () => openMove(row) },
    { key: 'delete', label: t('mesh.delete'), danger: true, separatorBefore: true, onSelect: () => remove(row) }
  ]
}

async function load() {
  loadError.value = null
  try {
    members.value = (await listMeshMembers(regionFilter.value)) || []
  } catch (e) {
    loadError.value = e
    members.value = null
  }
}

function openForm(row) {
  error.value = ''
  form.value = row
    ? {
        id: row.id, region_id: row.region_id, hostname: row.hostname, role: row.role, address_mode: row.address_mode,
        address: row.address_mode === 'fixed' ? row.address || '' : '', node_ref: row.node_ref || '', probe_address: row.probe_address || ''
      }
    : { id: 0, region_id: regionFilter.value || props.regions[0]?.id, hostname: '', role: 'server', address_mode: 'pool', address: '', node_ref: '', probe_address: '' }
}

async function save() {
  if (!form.value || saving.value) return
  saving.value = true
  error.value = ''
  const { id, ...body } = form.value
  if (body.role !== 'device') body.address_mode = 'fixed'
  if (body.address_mode !== 'fixed') body.address = ''
  if (body.role !== 'border') body.probe_address = ''
  try {
    if (id) await updateMeshMember(id, body)
    else await createMeshMember(body)
    toast.success(t('mesh.saved'))
    form.value = null
    await load()
    emit('changed')
  } catch (e) {
    error.value = meshErrorMessage(e, t('mesh.saveFailed'))
  } finally {
    saving.value = false
  }
}

function openMove(row) {
  moveError.value = ''
  move.value = { id: row.id, hostname: row.hostname, from: row.region_id, address_mode: row.address_mode, region_id: undefined, address: '', note: '' }
}

async function saveMove() {
  if (!move.value?.region_id || moving.value) return
  moving.value = true
  moveError.value = ''
  try {
    const change = await moveMeshMember(move.value.id, { region_id: move.value.region_id, address: move.value.address, note: move.value.note })
    toast.success(t('mesh.members.moved', { address: change?.new_address || '' }))
    move.value = null
    await load()
    emit('changed')
  } catch (e) {
    moveError.value = meshErrorMessage(e, t('mesh.saveFailed'))
  } finally {
    moving.value = false
  }
}

async function remove(row) {
  const ok = await confirm({
    title: t('mesh.members.deleteTitle', { name: row.hostname }),
    message: t('mesh.members.deleteMessage'),
    confirmLabel: t('mesh.delete'),
    tone: 'danger',
    onConfirm: () => deleteMeshMember(row.id)
  })
  if (!ok) return
  toast.success(t('mesh.deleted'))
  await load()
  emit('changed')
}

watch(regionFilter, load)
onMounted(load)
</script>

<style scoped src="./mesh.css"></style>
