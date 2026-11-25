<template>
  <UiSection :title="t('meshAddr.devices.title')" :description="t('meshAddr.devices.description')" heading-tag="h2" data-testid="mesh-devices">
    <div class="mesh-toolbar">
      <span class="mesh-badges">
        <UiBadge
          v-for="pool in pools"
          :key="pool.region_id"
          :tone="pool.low ? 'warning' : 'neutral'"
          :label="`${regionName(pool.region_id)} · ${t('meshAddr.devices.poolUsage', { used: pool.used, size: pool.size })}`"
        />
      </span>
      <UiButton variant="primary" :icon="Plus" :disabled="!regions.length" data-testid="mesh-device-add" @click="openSheet('add', null)">{{ t('meshAddr.devices.add') }}</UiButton>
    </div>
    <UiDataTable
      :columns="columns"
      :rows="devices || []"
      row-key="id"
      :label="t('meshAddr.devices.title')"
      :row-label="row => row.hostname"
      :row-actions="rowActions"
      :loading="!devices && !loadError"
      :error="devices ? null : loadError"
      :error-title="t('meshAddr.devices.loadFailed')"
      :empty-title="t('meshAddr.devices.empty')"
      data-testid="mesh-device-table"
      @retry="load"
    >
      <template #cell-device="{ row }">
        <span class="mesh-cell">
          <span>{{ row.hostname }}</span>
          <span class="mesh-muted mesh-mono">{{ row.fqdn }}</span>
          <span v-if="row.role === 'full'" class="mesh-muted">{{ t('meshAddr.devices.fullNode') }}<template v-if="row.subnets?.length"> · {{ t('meshAddr.devices.advertised', { subnets: row.subnets.join(', ') }) }}</template></span>
          <span v-if="row.last_seen_at" class="mesh-muted">{{ t('meshAddr.devices.lastSeen') }} {{ format.dateTime(row.last_seen_at) }}</span>
        </span>
      </template>
      <template #cell-address="{ row }">
        <span class="mesh-cell">
          <span class="mesh-mono">{{ regionName(row.region_id) }} · {{ row.address || t('meshAddr.common.none') }}</span>
          <span v-if="row.address_used_at" class="mesh-muted">{{ t('meshAddr.devices.lastUsed') }} {{ format.dateTime(row.address_used_at) }}</span>
        </span>
      </template>
      <template #cell-warnings="{ row }">
        <span class="mesh-badges">
          <UiBadge v-if="row.enrolled" :tone="STATE_TONES[deviceStatus(row).state]" :label="t(`mesh.page.status.${deviceStatus(row).state}`)" />
          <UiBadge v-for="code in row.enrolled ? deviceStatus(row).alerts : []" :key="code" tone="danger" :label="alertLabel(code)" />
          <UiBadge v-for="code in row.warnings" :key="code" tone="warning" :label="t(`meshAddr.devices.warnings.${code}`)" />
        </span>
      </template>
    </UiDataTable>

    <UiSheet :open="Boolean(sheet)" size="md" :title="sheetTitle" :description="sheetDescription" :dismissible="!saving" @update:open="value => { if (!value) sheet = null }">
      <form v-if="sheet" class="mesh-form" novalidate data-testid="mesh-device-form" @submit.prevent="save">
        <UiTextarea v-if="sheet.mode === 'subnets'" v-model="sheet.subnets" :rows="6" :label="t('meshAddr.devices.subnetsLabel')" :help="subnetsHelp" placeholder="192.168.1.0/24" data-testid="mesh-device-subnets" />
        <UiSelect v-else-if="sheet.mode !== 'edit'" v-model="sheet.regionId" size="md" :label="t('meshAddr.common.region')" :options="regionOptions" required :error="sheet.errors.region" />
        <template v-if="sheet.mode === 'add' || sheet.mode === 'edit'">
          <UiTextField v-model="sheet.hostname" size="md" :label="t('meshAddr.devices.hostname')" required :error="sheet.errors.hostname" data-testid="mesh-device-hostname" />
          <UiTextField v-model="sheet.deviceKey" size="md" :label="t('meshAddr.devices.deviceKey')" :help="t('meshAddr.devices.deviceKeyHelp')" />
        </template>
        <p v-if="saveError" class="mesh-error" role="alert">{{ saveError }}</p>
      </form>
      <template #footer>
        <UiButton :disabled="saving" @click="sheet = null">{{ t('meshAddr.common.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" data-testid="mesh-device-save" @click="save">{{ t('meshAddr.common.save') }}</UiButton>
      </template>
    </UiSheet>
  </UiSection>
</template>

<script setup>
// 组网 → 设备: personal devices with their pool addresses (R29); an address
// stays the device's until it has gone unused for 30 days. Full nodes
// without an Agent (R32) keep a fixed address and advertise the subnets an
// administrator confirms here (R17).
import { computed, onMounted, ref } from 'vue'
import { Ban, MapPinPlus, MoveRight, Network, Pencil, Plus, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiDataTable, UiSection, UiSelect, UiSheet, UiTextarea, UiTextField, useConfirm, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { assignKernelMeshDeviceAddress, createKernelMeshDevice, deleteKernelMeshDevice, listKernelMeshDevices, moveKernelMeshDevice, revokeKernelMeshDevice, setKernelMeshDeviceSubnets, updateKernelMeshDevice } from '@/api/kernel'
import { deviceStatus, inSharedAddressSpace, meshErrorMessage } from '../meshModel'

const props = defineProps({
  regions: { type: Array, default: () => [] },
  pools: { type: Array, default: () => [] }
})
const emit = defineEmits(['changed'])
const { t, te } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const devices = ref(null)
const loadError = ref(null)
const sheet = ref(null)
const saving = ref(false)
const saveError = ref('')

const STATE_TONES = { none: 'neutral', running: 'success', pending: 'warning', stopped: 'warning', lost: 'danger' }
const alertLabel = code => (te(`mesh.page.alerts.${code}`) ? t(`mesh.page.alerts.${code}`) : code)
const regionName = id => props.regions.find(region => region.id === id)?.name || '—'
const regionOptions = computed(() => props.regions.map(region => ({ value: String(region.id), label: `${region.name} (${region.code})` })))
const columns = computed(() => [
  { key: 'device', label: t('meshAddr.devices.hostname'), primary: true },
  { key: 'address', label: t('meshAddr.common.address'), secondary: true },
  { key: 'warnings', label: t('meshAddr.changes.status'), breakpoint: 'md' }
])
const sheetTitle = computed(() => {
  if (!sheet.value) return ''
  if (sheet.value.mode === 'add') return t('meshAddr.devices.addTitle')
  const name = sheet.value.device.hostname
  const titles = { move: 'meshAddr.devices.moveTitle', subnets: 'meshAddr.devices.subnetsTitle', edit: 'meshAddr.devices.editTitle' }
  return t(titles[sheet.value.mode], { name })
})
// A typed subnet in 100.64.0.0/10 is warned of before saving.
const subnetsHelp = computed(() => String(sheet.value?.subnets || '').split(/[\s,]+/).some(inSharedAddressSpace) ? t('meshAddr.common.sharedAddressSpaceHelp') : '')
const sheetDescription = computed(() => {
  const descriptions = { move: 'meshAddr.devices.moveDescription', subnets: 'meshAddr.devices.subnetsDescription' }
  return t(descriptions[sheet.value?.mode] || 'meshAddr.devices.hostnameHelp')
})

async function load() {
  loadError.value = null
  try {
    devices.value = (await listKernelMeshDevices()) || []
  } catch (e) {
    loadError.value = { message: meshErrorMessage(e) }
  }
}

function openSheet(mode, device) {
  saveError.value = ''
  sheet.value = {
    mode,
    device,
    regionId: mode === 'add' ? String(props.regions[0]?.id || '') : '',
    hostname: device?.hostname || '',
    deviceKey: device?.device_key || '',
    subnets: (device?.subnets || []).join('\n'),
    errors: {}
  }
}

async function save() {
  const form = sheet.value
  if (!form || saving.value) return
  form.errors = {}
  if ((form.mode === 'add' || form.mode === 'move') && !form.regionId) form.errors.region = t('meshAddr.common.required')
  if ((form.mode === 'add' || form.mode === 'edit') && !form.hostname.trim()) form.errors.hostname = t('meshAddr.common.required')
  if (Object.keys(form.errors).length) return
  saving.value = true
  saveError.value = ''
  try {
    if (form.mode === 'add') await createKernelMeshDevice(form.regionId, form)
    else if (form.mode === 'edit') await updateKernelMeshDevice(form.device.id, form)
    else if (form.mode === 'subnets') await setKernelMeshDeviceSubnets(form.device.id, form.subnets.split(/[\s,]+/))
    else await moveKernelMeshDevice(form.device.id, form.regionId)
    toast.success(t(form.mode === 'subnets' ? 'meshAddr.devices.subnetsSaved' : 'meshAddr.common.saved'))
    sheet.value = null
    await load()
    emit('changed')
  } catch (e) {
    saveError.value = meshErrorMessage(e)
  } finally {
    saving.value = false
  }
}

async function assignAddress(row) {
  try {
    await assignKernelMeshDeviceAddress(row.id)
  } catch (e) {
    toast.error(meshErrorMessage(e))
    return
  }
  toast.success(t('meshAddr.devices.assigned'))
  await load()
  emit('changed')
}

function rowActions(row) {
  return [
    ...(row.address && !(row.warnings || []).includes('reserved_address') ? [] : [{ key: 'assign', label: t('meshAddr.devices.assign'), icon: MapPinPlus, onSelect: () => assignAddress(row) }]),
    { key: 'edit', label: t('meshAddr.devices.edit'), icon: Pencil, onSelect: () => openSheet('edit', row) },
    ...(row.role === 'full' ? [{ key: 'subnets', label: t('meshAddr.devices.subnets'), icon: Network, onSelect: () => openSheet('subnets', row) }] : []),
    { key: 'move', label: t('meshAddr.devices.move'), icon: MoveRight, onSelect: () => openSheet('move', row) },
    ...(row.enrolled
      ? [{
          key: 'revoke',
          label: t('meshAddr.devices.revoke'),
          icon: Ban,
          danger: true,
          separatorBefore: true,
          onSelect: () => confirm({
            title: t('meshAddr.devices.revokeTitle', { name: row.hostname }),
            message: t('meshAddr.devices.revokeMessage'),
            confirmLabel: t('meshAddr.devices.revokeConfirm'),
            tone: 'danger',
            onConfirm: async () => {
              try {
                await revokeKernelMeshDevice(row.id)
              } catch (e) {
                throw new Error(meshErrorMessage(e))
              }
              toast.success(t('meshAddr.devices.revoked'))
              await load()
            }
          })
        }]
      : []),
    {
      key: 'delete',
      label: t('meshAddr.common.delete'),
      icon: Trash2,
      danger: true,
      separatorBefore: true,
      onSelect: () => confirm({
        title: t('meshAddr.devices.deleteTitle', { name: row.hostname }),
        message: t('meshAddr.devices.deleteMessage'),
        confirmLabel: t('meshAddr.common.deleteConfirm'),
        tone: 'danger',
        onConfirm: async () => {
          try {
            await deleteKernelMeshDevice(row.id)
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
