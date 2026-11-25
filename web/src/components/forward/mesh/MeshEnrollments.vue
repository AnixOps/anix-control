<template>
  <UiSection :title="t('meshAddr.enrollments.title')" :description="t('meshAddr.enrollments.description')" heading-tag="h2" data-testid="mesh-enrollments">
    <div class="mesh-toolbar">
      <span />
      <UiButton variant="primary" :icon="KeyRound" :disabled="!regions.length" data-testid="mesh-enrollment-add" @click="open">{{ t('meshAddr.enrollments.issue') }}</UiButton>
    </div>
    <UiDataTable
      :columns="columns"
      :rows="enrollments || []"
      row-key="id"
      :label="t('meshAddr.enrollments.title')"
      :row-label="row => row.hostname"
      :row-actions="rowActions"
      :loading="!enrollments && !loadError"
      :error="enrollments ? null : loadError"
      :error-title="t('meshAddr.enrollments.loadFailed')"
      :empty-title="t('meshAddr.enrollments.empty')"
      data-testid="mesh-enrollment-table"
      @retry="load"
    >
      <template #cell-hostname="{ row }">
        <span class="mesh-cell">
          <span>{{ row.hostname }}</span>
          <span v-if="row.role === 'full'" class="mesh-muted mesh-mono">{{ t('meshAddr.enrollments.roles.full') }} · {{ row.address || '—' }}</span>
        </span>
      </template>
      <template #cell-region="{ row }">{{ row.region_id ? regionName(row.region_id) : t('meshAddr.enrollments.anyRegion') }}</template>
      <template #cell-state="{ row }">
        <span class="mesh-cell">
          <UiBadge :tone="TONES[enrollmentState(row)]" :label="t(`meshAddr.enrollments.states.${enrollmentState(row)}`)" />
          <span class="mesh-muted">{{ t('meshAddr.enrollments.expires') }} {{ format.dateTime(row.expires_at) }}</span>
        </span>
      </template>
    </UiDataTable>

    <UiSheet :open="Boolean(sheet)" size="md" :title="t('meshAddr.enrollments.issueTitle')" :description="t('meshAddr.enrollments.issueDescription')" :dismissible="!saving" @update:open="value => { if (!value) sheet = null }">
      <div v-if="sheet?.issued" class="mesh-form" data-testid="mesh-enrollment-issued">
        <UiCopyField :label="t('meshAddr.enrollments.token')" :value="sheet.issued.token" :help="t('meshAddr.enrollments.tokenHelp')" />
      </div>
      <form v-else-if="sheet" class="mesh-form" novalidate data-testid="mesh-enrollment-form" @submit.prevent="save">
        <UiSelect v-model="sheet.deviceId" size="md" :label="t('meshAddr.enrollments.device')" :options="deviceOptions" :help="sheet.deviceId !== '0' ? t('meshAddr.enrollments.deviceHelp') : ''" data-testid="mesh-enrollment-device" />
        <template v-if="sheet.deviceId === '0'">
          <UiTextField v-model="sheet.hostname" size="md" :label="t('meshAddr.devices.hostname')" :help="t('meshAddr.devices.hostnameHelp')" required :error="sheet.errors.hostname" data-testid="mesh-enrollment-hostname" />
          <UiSelect v-model="sheet.role" size="md" :label="t('meshAddr.enrollments.role')" :options="roleOptions" :help="sheet.role === 'full' ? t('meshAddr.enrollments.roleHelp') : ''" data-testid="mesh-enrollment-role" />
          <UiSelect v-model="sheet.regionId" size="md" :label="t('meshAddr.common.region')" :options="sheet.role === 'full' ? regionOptions.slice(1) : regionOptions" :required="sheet.role === 'full'" :error="sheet.errors.region" :help="sheet.regionId === '0' && sheet.role !== 'full' ? t('meshAddr.enrollments.anyRegionHelp') : ''" />
          <UiTextField v-if="sheet.role === 'full'" v-model="sheet.address" size="md" :label="t('meshAddr.enrollments.address')" :help="t('meshAddr.enrollments.addressHelp')" placeholder="10.150.0.10" data-testid="mesh-enrollment-address" />
        </template>
        <UiNumberField v-model="sheet.hours" size="md" :label="t('meshAddr.enrollments.hours')" :min="1" :max="168" :help="t('meshAddr.enrollments.hoursHelp')" />
        <p v-if="saveError" class="mesh-error" role="alert">{{ saveError }}</p>
      </form>
      <template #footer>
        <UiButton v-if="sheet?.issued" variant="primary" @click="sheet = null">{{ t('meshAddr.enrollments.done') }}</UiButton>
        <template v-else>
          <UiButton :disabled="saving" @click="sheet = null">{{ t('meshAddr.common.cancel') }}</UiButton>
          <UiButton variant="primary" :loading="saving" data-testid="mesh-enrollment-save" @click="save">{{ t('meshAddr.enrollments.issueConfirm') }}</UiButton>
        </template>
      </template>
    </UiSheet>
  </UiSection>
</template>

<script setup>
// 组网 → 设备 → 接入凭证: one-time credentials devices without an Agent
// enroll with (R31), as an access device or a full node with a fixed
// address (R32); a credential's token is shown once.
import { computed, onMounted, ref, watch } from 'vue'
import { KeyRound, Trash2 } from '@lucide/vue'
import { UiBadge, UiButton, UiCopyField, UiDataTable, UiNumberField, UiSection, UiSelect, UiSheet, UiTextField, useConfirm, useFormat, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createKernelMeshEnrollment, deleteKernelMeshEnrollment, listKernelMeshDevices, listKernelMeshEnrollments } from '@/api/kernel'
import { enrollmentState, meshErrorMessage } from '../meshModel'

const props = defineProps({
  regions: { type: Array, default: () => [] }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()
const devices = ref([])

const TONES = { valid: 'success', used: 'neutral', expired: 'warning' }
const enrollments = ref(null)
const loadError = ref(null)
const sheet = ref(null)
const saving = ref(false)
const saveError = ref('')

const regionName = id => props.regions.find(region => region.id === id)?.name || '—'
const regionOptions = computed(() => [
  { value: '0', label: t('meshAddr.enrollments.anyRegion') },
  ...props.regions.map(region => ({ value: String(region.id), label: `${region.name} (${region.code})` }))
])
const roleOptions = computed(() => ['access', 'full'].map(value => ({ value, label: t(`meshAddr.enrollments.roles.${value}`) })))
const deviceOptions = computed(() => [
  { value: '0', label: t('meshAddr.enrollments.newDevice') },
  ...devices.value.map(device => ({ value: String(device.id), label: device.hostname }))
])
const columns = computed(() => [
  { key: 'hostname', label: t('meshAddr.devices.hostname'), primary: true },
  { key: 'region', label: t('meshAddr.common.region'), secondary: true },
  { key: 'state', label: t('meshAddr.enrollments.state') }
])

// A full node needs a region: choosing the role picks the first one.
watch(() => sheet.value?.role, role => {
  if (role === 'full' && sheet.value.regionId === '0') sheet.value.regionId = String(props.regions[0]?.id || '0')
})

async function load() {
  loadError.value = null
  try {
    enrollments.value = (await listKernelMeshEnrollments()) || []
  } catch (e) {
    loadError.value = { message: meshErrorMessage(e) }
  }
}

async function open() {
  saveError.value = ''
  sheet.value = { deviceId: '0', hostname: '', regionId: '0', role: 'access', address: '', hours: 24, errors: {}, issued: null }
  try {
    devices.value = (await listKernelMeshDevices()) || []
  } catch {
    devices.value = []
  }
}

async function save() {
  const form = sheet.value
  if (!form || saving.value) return
  form.errors = {}
  if (form.deviceId === '0' && !form.hostname.trim()) form.errors.hostname = t('meshAddr.common.required')
  if (form.deviceId === '0' && form.role === 'full' && form.regionId === '0') form.errors.region = t('meshAddr.enrollments.regionRequired')
  if (Object.keys(form.errors).length) return
  saving.value = true
  saveError.value = ''
  try {
    form.issued = await createKernelMeshEnrollment(form)
    await load()
    emit('changed')
  } catch (e) {
    saveError.value = meshErrorMessage(e)
  } finally {
    saving.value = false
  }
}

// A used credential stays as the record of the device it enrolled.
function rowActions(row) {
  if (row.used_at) return []
  return [{
    key: 'delete',
    label: t('meshAddr.enrollments.withdraw'),
    icon: Trash2,
    danger: true,
    onSelect: () => confirm({
      title: t('meshAddr.enrollments.withdrawTitle', { name: row.hostname }),
      message: t('meshAddr.enrollments.withdrawMessage'),
      confirmLabel: t('meshAddr.enrollments.withdrawConfirm'),
      tone: 'danger',
      onConfirm: async () => {
        try {
          await deleteKernelMeshEnrollment(row.id)
        } catch (e) {
          throw new Error(meshErrorMessage(e))
        }
        toast.success(t('meshAddr.common.deleted'))
        await load()
      }
    })
  }]
}

onMounted(load)
</script>

<style scoped src="./meshAddr.css"></style>
