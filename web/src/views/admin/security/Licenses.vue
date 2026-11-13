<template>
  <div class="licenses" data-security-panel="licenses">
    <UiSection :title="t('adminLicenses.title')" :description="t('adminLicenses.description')">
      <UiSkeleton v-if="loading" variant="card" :label="t('adminLicenses.loading')" />
      <UiErrorState v-else-if="loadError" :title="t('adminLicenses.loadFailed')" :error="loadError" @retry="load" />
      <div v-else class="licenses__settings">
        <p v-if="!settings.root_key" class="licenses__warning" role="status" data-testid="licenses-no-root-key">{{ t('adminLicenses.noRootKey') }}</p>
        <UiSwitch
          :model-value="settings.enabled"
          :label="t('adminLicenses.enabled')"
          :description="t('adminLicenses.enabledHelp')"
          :disabled="saving"
          data-testid="licenses-enabled"
          @update:model-value="value => save({ enabled: value })"
        />
        <UiSwitch
          :model-value="settings.require"
          :label="t('adminLicenses.require')"
          :description="t('adminLicenses.requireHelp')"
          :disabled="saving || !settings.enabled"
          data-testid="licenses-require"
          @update:model-value="value => save({ require: value })"
        />

        <div class="licenses__form">
          <UiNumberField
            v-model="trialDays"
            :label="t('adminLicenses.trialDays')"
            :help="t('adminLicenses.trialDaysHelp')"
            :min="0"
            :max="settings.max_trial_days"
            :disabled="saving"
            data-testid="licenses-trial-days"
          />
          <fieldset class="licenses__plans" data-testid="licenses-plans">
            <legend class="licenses__legend">{{ t('adminLicenses.plans') }}</legend>
            <p class="licenses__hint">{{ t('adminLicenses.plansHelp') }}</p>
            <p v-if="plansError" class="licenses__hint">{{ t('adminLicenses.plansLoadFailed') }}</p>
            <UiCheckbox
              v-for="plan in plans"
              :key="plan.id"
              :model-value="planIds.includes(plan.id)"
              :label="plan.name || `#${plan.id}`"
              :disabled="saving"
              @update:model-value="checked => togglePlan(plan.id, checked)"
            />
          </fieldset>
          <div class="licenses__actions">
            <UiButton :disabled="!formChanged || trialDays === null" :loading="saving" data-testid="licenses-save" @click="saveForm">{{ t('adminLicenses.save') }}</UiButton>
          </div>
        </div>

        <UiCopyField v-if="settings.ca" :value="settings.ca" :label="t('adminLicenses.ca')" :help="t('adminLicenses.caHelp')" data-testid="licenses-ca" />
        <p class="licenses__hint" data-testid="licenses-counts">{{ t('adminLicenses.counts', settings.devices) }}</p>
        <p class="licenses__hint">{{ t('adminLicenses.rules', { validity: settings.validity_days, window: settings.seat_window_days }) }}</p>
        <p v-if="saveError" class="licenses__error" role="alert">{{ saveError }}</p>
      </div>
    </UiSection>

    <UiSection :title="t('adminLicenses.devices.title')" :description="t('adminLicenses.devices.description')">
      <UiDataTable
        :columns="columns"
        :rows="devices"
        :label="t('adminLicenses.devices.label')"
        :row-label="row => row.device_id"
        storage-key="admin.security.licenses"
        manual-pagination
        :page="page"
        :page-size="PAGE_SIZE"
        :total="total"
        :loading="devicesLoading"
        :error="devicesError"
        :error-title="t('adminLicenses.devices.loadFailed')"
        :empty-icon="MonitorSmartphone"
        :empty-title="t('adminLicenses.devices.emptyTitle')"
        :empty-description="t('adminLicenses.devices.emptyDescription')"
        :row-actions="rowActions"
        :card-fields="4"
        state-heading-tag="h3"
        data-testid="licenses-devices"
        @update:page="changePage"
        @retry="loadDevices"
      >
        <template #toolbar-end>
          <UiButton size="md" :icon="RotateCw" :loading="devicesLoading" data-testid="licenses-refresh" @click="loadDevices">{{ t('adminLicenses.devices.refresh') }}</UiButton>
        </template>
        <template #cell-device_id="{ row }">
          <code class="licenses__mono">{{ row.device_id }}</code>
        </template>
        <template #cell-state="{ row }">
          <span class="licenses__state">
            <UiBadge :tone="STATE_TONES[deviceState(row)]" :label="t(`adminLicenses.devices.states.${deviceState(row)}`)" />
            <UiBadge v-if="row.trial" tone="info" :dot="false" :label="t('adminLicenses.devices.trial')" />
          </span>
        </template>
        <template #cell-not_after="{ row }">
          <span v-if="row.not_after" :title="format.dateTime(row.not_after * 1000)">{{ format.date(row.not_after * 1000) }}</span>
          <span v-else class="licenses__hint">{{ t('adminLicenses.devices.none') }}</span>
        </template>
        <template #cell-last_seen_at="{ row }">
          <span :title="format.dateTime(row.last_seen_at * 1000)">{{ format.relativeTime(row.last_seen_at * 1000) }}</span>
        </template>
      </UiDataTable>
    </UiSection>
  </div>
</template>

<script setup>
// 安全 → 设备授权: ANX device licensing (internal/entitlement, anx spec
// §2.4.1), off by default. The settings (on, nodes require a license, trial
// days, plans that include ANX) and the license CA; below, the devices that
// asked for a license, with revoke and restore in the row menu.
//
// Endpoints: GET|PUT /api/v4/kernel/entitlements/anx, GET
// /api/v4/kernel/entitlements/anx/devices, POST
// /api/v4/kernel/entitlements/anx/devices/:id/(revoke|restore).
import { computed, onMounted, ref } from 'vue'
import { Ban, MonitorSmartphone, RotateCcw, RotateCw } from '@lucide/vue'
import { getPlans } from '@/api/admin'
import {
  getKernelEntitlement,
  listKernelEntitlementDevices,
  setKernelEntitlement,
  setKernelEntitlementDeviceRevoked
} from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const PAGE_SIZE = 20
const STATE_TONES = { seated: 'success', idle: 'neutral', revoked: 'danger' }

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const settings = ref({ enabled: false, require: false, trial_days: 3, plan_ids: [], ca: '', root_key: false, devices: { total: 0, seated: 0, revoked: 0 }, validity_days: 7, seat_window_days: 30, max_trial_days: 30 })
const loading = ref(true)
const loadError = ref(null)
const saving = ref(false)
const saveError = ref('')

// The form part (trial days, plans) is saved with its button.
const trialDays = ref(3)
const planIds = ref([])
const plans = ref([])
const plansError = ref(false)

const devices = ref([])
const total = ref(0)
const page = ref(1)
const devicesLoading = ref(false)
const devicesError = ref(null)

function errorMessage(cause) {
  return cause?.response?.data?.error?.message || cause?.message || ''
}

function adopt(view) {
  settings.value = view
  trialDays.value = view.trial_days
  planIds.value = [...(view.plan_ids || [])]
}

const formChanged = computed(() => {
  const saved = [...(settings.value.plan_ids || [])].sort((a, b) => a - b).join(',')
  const current = [...planIds.value].sort((a, b) => a - b).join(',')
  return trialDays.value !== settings.value.trial_days || saved !== current
})

async function load() {
  loading.value = true
  loadError.value = null
  try {
    adopt(await getKernelEntitlement())
  } catch (cause) {
    loadError.value = cause
  } finally {
    loading.value = false
  }
}

async function loadPlans() {
  try {
    const res = await getPlans()
    const payload = res?.data?.data ?? res?.data ?? res
    plans.value = Array.isArray(payload) ? payload : []
    plansError.value = false
  } catch {
    plansError.value = true
  }
}

async function save(patch) {
  saving.value = true
  saveError.value = ''
  try {
    adopt(await setKernelEntitlement(patch))
    toast.success(t('adminLicenses.saved'))
  } catch (cause) {
    saveError.value = t('adminLicenses.saveFailed', { message: errorMessage(cause) })
  } finally {
    saving.value = false
  }
}

function togglePlan(id, checked) {
  planIds.value = checked ? [...new Set([...planIds.value, id])] : planIds.value.filter(value => value !== id)
}

function saveForm() {
  return save({ trial_days: trialDays.value, plan_ids: [...planIds.value] })
}

async function loadDevices() {
  devicesLoading.value = true
  try {
    const answer = await listKernelEntitlementDevices({ page: page.value, pageSize: PAGE_SIZE })
    devices.value = Array.isArray(answer?.items) ? answer.items : []
    total.value = Number(answer?.total || 0)
    devicesError.value = null
  } catch (cause) {
    devices.value = []
    devicesError.value = cause
  } finally {
    devicesLoading.value = false
  }
}

function changePage(value) {
  page.value = value
  loadDevices()
}

function deviceState(row) {
  if (row.revoked) return 'revoked'
  return row.seated ? 'seated' : 'idle'
}

function userText(row) {
  return row.email || t('adminLicenses.devices.userFallback', { id: row.user_id })
}

const columns = computed(() => [
  { key: 'email', label: t('adminLicenses.devices.user'), primary: true, hideable: false, minWidth: 160, value: row => userText(row) },
  { key: 'device_id', label: t('adminLicenses.devices.device'), secondary: true, nowrap: true },
  { key: 'alg', label: t('adminLicenses.devices.alg'), nowrap: true },
  { key: 'state', label: t('adminLicenses.devices.state'), nowrap: true, value: row => t(`adminLicenses.devices.states.${deviceState(row)}`) },
  { key: 'not_after', label: t('adminLicenses.devices.notAfter'), nowrap: true },
  { key: 'last_seen_at', label: t('adminLicenses.devices.lastSeen'), nowrap: true }
])

const rowActions = row => [
  { key: 'revoke', label: t('adminLicenses.devices.revoke'), icon: Ban, danger: true, hidden: row.revoked, onSelect: () => revoke(row) },
  { key: 'restore', label: t('adminLicenses.devices.restore'), icon: RotateCcw, hidden: !row.revoked, onSelect: () => restore(row) }
]

async function revoke(row) {
  let changed = true
  const confirmed = await confirm({
    title: t('adminLicenses.devices.revokeTitle', { device: row.device_id }),
    message: t('adminLicenses.devices.revokeMessage', { user: userText(row) }),
    confirmLabel: t('adminLicenses.devices.revoke'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        changed = (await setKernelEntitlementDeviceRevoked(row.id, true))?.changed !== false
      } catch (cause) {
        throw new Error(t('adminLicenses.devices.actionFailed', { message: errorMessage(cause) }))
      }
    }
  })
  if (!confirmed) return
  toast.success(t(changed ? 'adminLicenses.devices.revoked' : 'adminLicenses.devices.unchanged'))
  await Promise.all([loadDevices(), load()])
}

// Restoring gives the device back its renewals, so it needs no confirmation.
async function restore(row) {
  try {
    const answer = await setKernelEntitlementDeviceRevoked(row.id, false)
    toast.success(t(answer?.changed === false ? 'adminLicenses.devices.unchanged' : 'adminLicenses.devices.restored'))
  } catch (cause) {
    toast.error(t('adminLicenses.devices.actionFailed', { message: errorMessage(cause) }))
    return
  }
  await Promise.all([loadDevices(), load()])
}

onMounted(() => {
  load()
  loadPlans()
  loadDevices()
})
</script>

<style scoped>
.licenses {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}

.licenses__settings,
.licenses__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.licenses__plans {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  border: 0;
}

.licenses__legend {
  padding: 0;
  font-weight: var(--weight-semibold);
}

.licenses__actions {
  display: flex;
  justify-content: flex-end;
}

.licenses__hint,
.licenses__warning,
.licenses__error {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.licenses__warning {
  color: var(--label-1);
}

.licenses__error {
  color: var(--danger);
}

.licenses__mono {
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.licenses__state {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}
</style>
