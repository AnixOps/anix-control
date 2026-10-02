<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminPayment.title')" :description="t('adminPayment.subtitle')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" @click="openGatewayModal()">{{ t('adminPayment.actions.createGateway') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiTabs v-model="activeTab" :items="tabItems" :aria-label="t('adminPayment.tabs.label')" :unmount-on-hide="false">
      <template #gateways>
        <UiDataTable
          :columns="gatewayColumns"
          :rows="gateways"
          :label="t('adminPayment.gateways.label')"
          :row-label="gateway => gateway.name || String(gateway.id)"
          storage-key="admin.payment.gateways"
          :loading="gatewaysLoading"
          :error="gatewaysError"
          :error-title="t('adminPayment.messages.fetchGatewaysFailed')"
          :empty-icon="CreditCard"
          :empty-title="t('adminPayment.gateways.empty')"
          :empty-description="t('adminPayment.gateways.emptyDescription')"
          activatable
          :row-actions="gatewayActions"
          @row-activate="openGatewayModal"
          @retry="fetchGateways"
        >
          <template #empty-actions>
            <UiButton variant="primary" :icon="Plus" @click="openGatewayModal()">{{ t('adminPayment.actions.createGateway') }}</UiButton>
          </template>
          <template #cell-enabled="{ row }">
            <UiBadge :tone="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? t('adminPayment.status.enabled') : t('adminPayment.status.disabled')" />
          </template>
        </UiDataTable>
      </template>

      <template #records>
        <UiDataTable
          :columns="recordColumns"
          :rows="records"
          :label="t('adminPayment.records.label')"
          :row-label="record => record.trade_no || String(record.id)"
          storage-key="admin.payment.records"
          :page-size="20"
          :loading="recordsLoading"
          :error="recordsError"
          :error-title="t('adminPayment.messages.fetchRecordsFailed')"
          :filtered="Boolean(recordFilter.status || recordFilter.gateway_type)"
          :empty-icon="Receipt"
          :empty-title="t('adminPayment.records.empty')"
          :empty-description="t('adminPayment.records.emptyDescription')"
          activatable
          :row-actions="record => [{ key: 'details', label: t('adminPayment.actions.details'), icon: Eye, onSelect: () => viewRecord(record) }]"
          @row-activate="viewRecord"
          @retry="fetchRecords"
          @clear-filters="clearRecordFilters"
        >
          <template #toolbar>
            <UiFilterChips v-model="recordFilter.status" :label="t('adminPayment.records.filters.status')" :options="statusChips" />
            <UiFilterChips v-model="recordFilter.gateway_type" :label="t('adminPayment.records.filters.type')" :options="typeChips" />
          </template>
          <template #cell-trade_no="{ value }">
            <span class="trade-no">{{ value }}</span>
          </template>
          <template #cell-status="{ row }">
            <UiBadge :tone="recordTone(row.status)" :label="getPaymentStatusLabel(row.status)" />
          </template>
        </UiDataTable>
      </template>

      <template #stats>
        <UiErrorState v-if="statsError" :title="t('adminPayment.messages.fetchStatsFailed')" :error="statsError" @retry="fetchStats" />
        <div v-else-if="showStatsSkeleton" class="stats-grid">
          <UiSkeleton v-for="n in 4" :key="n" variant="card" />
        </div>
        <template v-else>
          <div class="stats-grid" data-test="payment-stats">
            <UiCard class="stat-card">
              <span class="stat-label">{{ t('adminPayment.stats.totalAmount') }}</span>
              <strong class="stat-value">{{ formatMoney(stats.total_amount) }}</strong>
            </UiCard>
            <UiCard class="stat-card">
              <span class="stat-label">{{ t('adminPayment.stats.totalOrders') }}</span>
              <strong class="stat-value">{{ format.number(stats.total_orders || 0) }}</strong>
            </UiCard>
            <UiCard class="stat-card">
              <span class="stat-label">{{ t('adminPayment.stats.successOrders') }}</span>
              <strong class="stat-value">{{ format.number(stats.success_orders || 0) }}</strong>
            </UiCard>
            <UiCard class="stat-card">
              <span class="stat-label">{{ t('adminPayment.stats.successRate') }}</span>
              <strong class="stat-value">{{ formatSuccessRate(stats.success_rate) }}</strong>
            </UiCard>
          </div>
          <UiCard :title="t('adminPayment.stats.gatewayDistribution')" heading-tag="h2">
            <ul v-if="gatewayStatsEntries.length" class="gateway-stats">
              <li v-for="[type, item] in gatewayStatsEntries" :key="type" class="gateway-stat">
                <span class="gateway-stat__name">{{ getGatewayTypeLabel(type) }}</span>
                <UiUsageBar
                  class="gateway-stat__bar"
                  :value="getGatewayPercent(type)"
                  :max="100"
                  :warn-at="101"
                  :danger-at="101"
                  :text="`${formatMoney(item?.amount)} · ${getGatewayPercent(type)}%`"
                />
              </li>
            </ul>
            <UiEmptyState v-else compact :icon="ChartPie" :title="t('adminPayment.stats.empty')" />
          </UiCard>
        </template>
      </template>
    </UiTabs>

    <UiDialog
      v-model:open="showGatewayModal"
      :title="editingGateway ? t('adminPayment.modal.editTitle') : t('adminPayment.modal.createTitle')"
      :dismissible="!gatewaySaving"
    >
      <div class="form-grid">
        <UiTextField
          id="payment-gateway-name"
          v-model="gatewayForm.name"
          class="form-grid__full"
          required
          :label="t('adminPayment.modal.fields.name')"
          :placeholder="t('adminPayment.modal.placeholders.name')"
        />
        <UiSelect id="payment-gateway-type" v-model="gatewayForm.type" :label="t('adminPayment.modal.fields.type')" :options="gatewayTypeOptions" />
        <UiTextField
          id="payment-gateway-fee"
          v-model.number="gatewayForm.fee_rate"
          type="number"
          step="0.001"
          :label="t('adminPayment.modal.fields.feeRate')"
          :help="t('adminPayment.modal.placeholders.feeRate')"
        />
        <UiTextField id="payment-gateway-min" v-model.number="gatewayForm.min_amount" type="number" :label="t('adminPayment.modal.fields.minAmount')" :help="t('adminPayment.modal.placeholders.minAmount')" />
        <UiTextField id="payment-gateway-max" v-model.number="gatewayForm.max_amount" type="number" :label="t('adminPayment.modal.fields.maxAmount')" :help="t('adminPayment.modal.placeholders.maxAmount')" />
        <UiTextarea
          id="payment-gateway-config"
          v-model="gatewayForm.config_json"
          class="form-grid__full config-json"
          :rows="5"
          :label="t('adminPayment.modal.fields.configJson')"
          :placeholder="t('adminPayment.modal.placeholders.configJson')"
          :error="configJsonError"
          data-test="payment-gateway-config"
        />
      </div>
      <p v-if="gatewayError" class="form-error" role="alert" data-test="payment-gateway-error">{{ gatewayError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="gatewaySaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="payment-gateway-save" :loading="gatewaySaving" @click="saveGateway">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>

    <UiSheet
      :open="Boolean(viewingRecord)"
      grouped
      :title="t('adminPayment.records.detail.title')"
      :description="viewingRecord?.trade_no || ''"
      @update:open="value => { if (!value) viewingRecord = null }"
    >
      <template #header-actions>
        <UiBadge v-if="viewingRecord" :tone="recordTone(viewingRecord.status)" :label="getPaymentStatusLabel(viewingRecord.status)" />
      </template>
      <div v-if="viewingRecord" data-test="payment-record-detail">
        <UiGroupedList>
          <UiGroupedListRow :label="t('adminPayment.records.detail.tradeNo')" :value="viewingRecord.trade_no || '—'" />
          <UiGroupedListRow :label="t('adminPayment.records.detail.amount')" :value="formatMoney(viewingRecord.amount)" />
          <UiGroupedListRow :label="t('adminPayment.records.detail.status')" :value="getPaymentStatusLabel(viewingRecord.status)" />
          <UiGroupedListRow :label="t('adminPayment.records.table.gateway')" :value="getGatewayTypeLabel(viewingRecord.gateway_type)" />
          <UiGroupedListRow :label="t('adminPayment.records.table.userId')" :value="viewingRecord.user_id ?? '—'" />
          <UiGroupedListRow :label="t('adminPayment.records.table.createdAt')" :value="formatTime(viewingRecord.created_at)" />
        </UiGroupedList>
      </div>
    </UiSheet>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ChartPie, CreditCard, Eye, Pencil, Plus, Power, PowerOff, Receipt, Trash2 } from '@lucide/vue'
import {
  createPaymentGateway,
  deletePaymentGateway,
  getPaymentGateways,
  getPaymentRecords,
  getPaymentStats,
  togglePaymentGateway,
  updatePaymentGateway
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTabs from '@/ui/UiTabs.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import UiUsageBar from '@/ui/UiUsageBar.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const gatewayTypes = ['alipay', 'wechat', 'stripe', 'usdt', 'epay']
const paymentStatuses = ['pending', 'paid', 'failed', 'refunded']

const activeTab = ref('gateways')
const gateways = ref([])
const records = ref([])
const stats = ref({})

const recordFilter = ref({ status: '', gateway_type: '' })

const showGatewayModal = ref(false)
const editingGateway = ref(null)
const gatewayForm = ref(createGatewayForm())
const gatewaySaving = ref(false)
const gatewayError = ref('')
const configJsonError = ref('')
const viewingRecord = ref(null)
const gatewaysLoading = ref(false)
const gatewaysError = ref(null)
const recordsLoading = ref(false)
const recordsError = ref(null)
const statsLoading = ref(false)
const statsError = ref(null)
const showStatsSkeleton = useDelayedLoading(statsLoading)

const tabItems = computed(() => [
  { value: 'gateways', label: t('adminPayment.tabs.gateways') },
  { value: 'records', label: t('adminPayment.tabs.records') },
  { value: 'stats', label: t('adminPayment.tabs.stats') }
])
const gatewayTypeOptions = computed(() => gatewayTypes.map(type => ({ value: type, label: getGatewayTypeLabel(type) })))
const gatewayColumns = computed(() => [
  { key: 'name', label: t('adminPayment.gateways.table.name'), primary: true, sortable: true },
  { key: 'enabled', label: t('adminPayment.gateways.table.status'), secondary: true, sortable: true, sortValue: gateway => (gateway.enabled ? 1 : 0) },
  { key: 'type', label: t('adminPayment.gateways.table.type'), sortable: true, value: gateway => getGatewayTypeLabel(gateway.type) },
  { key: 'fee_rate', label: t('adminPayment.gateways.table.feeRate'), numeric: true, align: 'end', sortable: true, format: value => formatFeeRate(value), sortValue: gateway => Number(gateway.fee_rate || 0) },
  { key: 'min_amount', label: t('adminPayment.gateways.table.minAmount'), numeric: true, align: 'end', format: value => formatMoney(value) },
  { key: 'max_amount', label: t('adminPayment.gateways.table.maxAmount'), numeric: true, align: 'end', format: value => formatMoney(value) },
  { key: 'id', label: t('adminPayment.gateways.table.id'), numeric: true, sortable: true, hidden: true }
])
const gatewayActions = gateway => [
  { key: 'edit', label: t('adminPayment.actions.edit'), icon: Pencil, onSelect: () => openGatewayModal(gateway) },
  gateway.enabled
    ? { key: 'disable', label: t('adminPayment.actions.disable'), icon: PowerOff, onSelect: () => toggleGatewayStatus(gateway) }
    : { key: 'enable', label: t('adminPayment.actions.enable'), icon: Power, onSelect: () => toggleGatewayStatus(gateway) },
  { key: 'delete', label: t('adminPayment.confirm.deleteAction'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteGatewayItem(gateway) }
]
const recordColumns = computed(() => [
  { key: 'trade_no', label: t('adminPayment.records.table.tradeNo'), primary: true, sortable: true },
  { key: 'status', label: t('adminPayment.records.table.status'), secondary: true, sortable: true },
  { key: 'gateway_type', label: t('adminPayment.records.table.gateway'), value: record => getGatewayTypeLabel(record.gateway_type), sortable: true },
  { key: 'amount', label: t('adminPayment.records.table.amount'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => formatMoney(value), sortValue: record => Number(record.amount || 0) },
  { key: 'user_id', label: t('adminPayment.records.table.userId'), numeric: true, sortable: true },
  { key: 'created_at', label: t('adminPayment.records.table.createdAt'), numeric: true, nowrap: true, sortable: true, firstDirection: 'desc', format: value => formatTime(value) },
  { key: 'id', label: t('adminPayment.records.table.id'), numeric: true, sortable: true, hidden: true }
])
// Status and gateway type are the record list's own query parameters.
const statusChips = computed(() => paymentStatuses.map(status => ({ value: status, label: getPaymentStatusLabel(status) })))
const typeChips = computed(() => gatewayTypes.map(type => ({ value: type, label: getGatewayTypeLabel(type) })))
const recordTone = status => ({ pending: 'warning', paid: 'success', failed: 'danger', refunded: 'neutral' }[status] || 'neutral')
const clearRecordFilters = () => {
  recordFilter.value.status = ''
  recordFilter.value.gateway_type = ''
}
watch(() => [recordFilter.value.status, recordFilter.value.gateway_type], () => fetchRecords())

function createGatewayForm(source = {}) {
  return {
    name: '',
    type: 'alipay',
    fee_rate: 0,
    min_amount: 0,
    max_amount: 0,
    config_json: '',
    ...source
  }
}

const gatewayStatsEntries = computed(() => Object.entries(stats.value.by_gateway || {}))

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.msg ||
  error?.msg ||
  error?.message ||
  t(fallbackKey)
)

const readPanelEnvelopeError = (res) => {
  const candidates = [res, res?.data]
  for (const candidate of candidates) {
    if (!candidate || typeof candidate !== 'object') continue
    if (!Object.prototype.hasOwnProperty.call(candidate, 'code')) continue
    if (Number(candidate.code) === 0) return null
    return candidate.msg || candidate.error || ''
  }
  return null
}

const ensurePaymentSuccess = (res, fallbackKey) => {
  const message = readPanelEnvelopeError(res)
  if (message !== null) {
    throw new Error(message || t(fallbackKey))
  }
  return res
}

const gatewayTypeKeyMap = {
  alipay: 'adminPayment.types.alipay',
  wechat: 'adminPayment.types.wechat',
  stripe: 'adminPayment.types.stripe',
  usdt: 'adminPayment.types.usdt',
  epay: 'adminPayment.types.epay'
}

const paymentStatusKeyMap = {
  pending: 'adminPayment.status.pending',
  paid: 'adminPayment.status.paid',
  failed: 'adminPayment.status.failed',
  refunded: 'adminPayment.status.refunded'
}

const getGatewayTypeLabel = (type) => (gatewayTypeKeyMap[type] ? t(gatewayTypeKeyMap[type]) : type || '—')
const getPaymentStatusLabel = (status) => (paymentStatusKeyMap[status] ? t(paymentStatusKeyMap[status]) : status || '—')

const formatTime = value => format.dateTime(value)

// The payment API answers in yuan, not cents.
const formatMoney = value => format.money(Number(value || 0), { cents: false })

const formatFeeRate = (value) => `${(Number(value || 0) * 100).toFixed(2)}%`

const formatSuccessRate = (value) => `${Number(value || 0).toFixed(1)}%`

const getGatewayPercent = (type) => {
  const totalAmount = Number(stats.value.total_amount || 0)
  const gatewayAmount = Number(stats.value.by_gateway?.[type]?.amount || 0)
  if (!totalAmount || !gatewayAmount) return 0
  return Number(((gatewayAmount / totalAmount) * 100).toFixed(1))
}

const readPaymentPayload = (res) => {
  if (!res || typeof res !== 'object') return {}
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data && typeof res.data === 'object' ? res.data : {}
  }
  if (res.data && typeof res.data === 'object' && Object.prototype.hasOwnProperty.call(res.data, 'data')) {
    return res.data.data && typeof res.data.data === 'object' ? res.data.data : {}
  }
  return res.data && typeof res.data === 'object' ? res.data : res
}

const readPaymentGatewayList = (res) => {
  const payload = readPaymentPayload(res)
  if (Array.isArray(payload)) return payload
  return Array.isArray(payload?.list) ? payload.list : []
}

const readPaymentRecordList = (res) => {
  const payload = readPaymentPayload(res)
  if (Array.isArray(payload)) return payload
  return Array.isArray(payload?.list) ? payload.list : []
}

const fetchGateways = async () => {
  gatewaysLoading.value = true
  try {
    const res = ensurePaymentSuccess(
      await getPaymentGateways(),
      'adminPayment.messages.fetchGatewaysFailed'
    )
    gateways.value = readPaymentGatewayList(res)
    gatewaysError.value = null
  } catch (error) {
    gatewaysError.value = error
  } finally {
    gatewaysLoading.value = false
  }
}

const fetchRecords = async () => {
  recordsLoading.value = true
  try {
    const res = ensurePaymentSuccess(
      await getPaymentRecords(recordFilter.value),
      'adminPayment.messages.fetchRecordsFailed'
    )
    records.value = readPaymentRecordList(res)
    recordsError.value = null
  } catch (error) {
    recordsError.value = error
  } finally {
    recordsLoading.value = false
  }
}

const fetchStats = async () => {
  statsLoading.value = true
  try {
    const res = ensurePaymentSuccess(
      await getPaymentStats(),
      'adminPayment.messages.fetchStatsFailed'
    )
    stats.value = readPaymentStats(res)
    statsError.value = null
  } catch (error) {
    statsError.value = error
  } finally {
    statsLoading.value = false
  }
}

const readPaymentStats = (res) => {
  const payload = readPaymentPayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

const openGatewayModal = (gateway = null) => {
  if (gateway) {
    editingGateway.value = gateway
    gatewayForm.value = createGatewayForm({
      name: gateway.name,
      type: gateway.type,
      fee_rate: gateway.fee_rate,
      min_amount: gateway.min_amount,
      max_amount: gateway.max_amount,
      config_json: typeof gateway.config === 'string' ? gateway.config : JSON.stringify(gateway.config || {})
    })
  } else {
    editingGateway.value = null
    gatewayForm.value = createGatewayForm()
  }
  gatewayError.value = ''
  configJsonError.value = ''
  showGatewayModal.value = true
}

const saveGateway = async () => {
  if (gatewaySaving.value) return
  gatewayError.value = ''
  configJsonError.value = ''
  const payload = { ...gatewayForm.value }
  if (payload.config_json) {
    try {
      payload.config = JSON.parse(payload.config_json)
    } catch (error) {
      configJsonError.value = t('adminPayment.messages.invalidConfigJson')
      return
    }
  }
  gatewaySaving.value = true
  try {
    delete payload.config_json

    if (editingGateway.value) {
      ensurePaymentSuccess(
        await updatePaymentGateway(editingGateway.value.id, payload),
        'adminPayment.messages.gatewaySaveFailedShort'
      )
    } else {
      ensurePaymentSuccess(
        await createPaymentGateway(payload),
        'adminPayment.messages.gatewaySaveFailedShort'
      )
    }
    toast.success(t('adminPayment.messages.gatewaySaveSuccess'))
    showGatewayModal.value = false
    await fetchGateways()
  } catch (error) {
    gatewayError.value = t('adminPayment.messages.gatewaySaveFailed', {
      message: resolveApiError(error, 'adminPayment.messages.gatewaySaveFailedShort')
    })
  } finally {
    gatewaySaving.value = false
  }
}

const setGatewayEnabled = async (gateway, enabled) => {
  try {
    ensurePaymentSuccess(
      await togglePaymentGateway(gateway.id, enabled),
      'adminPayment.messages.toggleFailedShort'
    )
    await fetchGateways()
    return true
  } catch (error) {
    toast.error(t('adminPayment.messages.toggleFailed', {
      message: resolveApiError(error, 'adminPayment.messages.toggleFailedShort')
    }))
    return false
  }
}

// Enable and disable undo each other: no confirmation, 撤销 in the toast.
const toggleGatewayStatus = async (gateway) => {
  const enabled = !gateway.enabled
  if (!(await setGatewayEnabled(gateway, enabled))) return
  toast.success(
    t(enabled ? 'adminPayment.messages.gatewayEnabled' : 'adminPayment.messages.gatewayDisabled', { name: gateway.name }),
    { undo: () => setGatewayEnabled(gateway, !enabled) }
  )
}

const deleteGatewayItem = async (gateway) => {
  const confirmed = await confirm({
    title: t('adminPayment.confirm.deleteTitle', { name: gateway.name }),
    message: t('adminPayment.confirm.deleteMessage'),
    confirmLabel: t('adminPayment.confirm.deleteAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        ensurePaymentSuccess(
          await deletePaymentGateway(gateway.id),
          'adminPayment.messages.deleteFailedShort'
        )
      } catch (error) {
        throw new Error(resolveApiError(error, 'adminPayment.messages.deleteFailedShort'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminPayment.messages.gatewayDeleted', { name: gateway.name }))
  await fetchGateways()
}

const viewRecord = (record) => {
  viewingRecord.value = record
}

onMounted(() => {
  fetchGateways()
  fetchRecords()
  fetchStats()
})
</script>

<style scoped>
.trade-no {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-4);
  margin-bottom: var(--space-6);
}

.stat-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.stat-label {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.stat-value {
  font-size: var(--type-title-2-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-title-2-line);
  font-variant-numeric: tabular-nums;
}

.gateway-stats {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: 0;
  margin: 0;
  list-style: none;
}

.gateway-stat {
  display: grid;
  grid-template-columns: minmax(96px, 160px) minmax(0, 1fr);
  gap: var(--space-4);
  align-items: center;
}

.gateway-stat__name {
  font-weight: var(--weight-medium);
}

.config-json :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

@media (max-width: 639.98px) {
  .gateway-stat {
    grid-template-columns: 1fr;
    gap: var(--space-1);
  }
}
</style>
