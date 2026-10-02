<template>
  <div class="list-page">
    <UiPageHeader :title="t('adminInvite.title')" :description="t('adminInvite.subtitle')" />

    <UiTabs v-model="activeTab" variant="segmented" :aria-label="t('adminInvite.tabs.label')" :items="tabItems">
      <template #withdrawals>
        <UiDataTable
          :columns="withdrawalColumns"
          :rows="withdrawals"
          :label="t('adminInvite.withdrawals.label')"
          :row-label="item => t('adminInvite.withdrawals.rowName', { id: item.id })"
          storage-key="admin.invite.withdrawals"
          :page-size="20"
          :loading="withdrawalsLoading"
          :error="withdrawalsError"
          :error-title="t('adminInvite.messages.fetchWithdrawalsFailed')"
          :filtered="Boolean(withdrawalFilter.status)"
          :empty-icon="Wallet"
          :empty-title="t('adminInvite.withdrawals.empty')"
          :empty-description="t('adminInvite.withdrawals.emptyDescription')"
          :row-actions="withdrawalActions"
          @retry="fetchWithdrawals"
          @clear-filters="withdrawalFilter.status = ''"
        >
          <template #toolbar>
            <UiFilterChips v-model="withdrawalFilter.status" :label="t('adminInvite.withdrawals.filters.label')" :options="statusChips" />
          </template>
          <template #cell-status="{ row }">
            <UiBadge :tone="statusTone(row.status)" :label="getStatusLabel(row.status)" />
          </template>
        </UiDataTable>
      </template>

      <template #stats>
        <UiErrorState v-if="statsError" :title="t('adminInvite.messages.fetchStatsFailed')" :error="statsError" @retry="fetchStats" />
        <div v-else class="invite-stats">
          <div class="invite-stats__grid">
            <div v-for="card in statCards" :key="card.key" class="invite-stat" :data-stat="card.key">
              <span class="invite-stat__label">{{ card.label }}</span>
              <strong class="invite-stat__value tabular-nums">{{ card.value }}</strong>
            </div>
          </div>
          <section class="dialog-section" aria-labelledby="invite-ranking-title">
            <h2 id="invite-ranking-title" class="dialog-section__title">{{ t('adminInvite.stats.rankingTitle') }}</h2>
            <UiDataTable
              :columns="rankingColumns"
              :rows="rankedInviters"
              row-key="user_id"
              :label="t('adminInvite.stats.rankingTitle')"
              :loading="statsLoading"
              :empty-icon="Trophy"
              :empty-title="t('adminInvite.stats.empty')"
              :settings="false"
              state-heading-tag="h3"
            />
          </section>
        </div>
      </template>

      <template #config>
        <UiErrorState v-if="configError" :title="t('adminInvite.messages.fetchConfigFailed')" :error="configError" @retry="fetchConfig" />
        <UiSkeleton v-else-if="showConfigSkeleton" variant="text" :lines="6" />
        <form v-else class="invite-config" @submit.prevent="saveConfig">
          <UiCard as="section" :title="t('adminInvite.config.inviteTitle')">
            <div class="form-grid">
              <div class="form-grid__full">
                <UiSwitch v-model="config.enabled" :label="t('adminInvite.config.enabled')" />
              </div>
              <UiTextField v-model="config.code_prefix" size="md" :label="t('adminInvite.config.codePrefix')" :placeholder="t('adminInvite.placeholders.codePrefix')" />
              <UiTextField v-model.number="config.code_length" size="md" type="number" min="4" max="16" :label="t('adminInvite.config.codeLength')" :help="t('adminInvite.config.codeLengthHelp')" />
            </div>
          </UiCard>
          <UiCard as="section" :title="t('adminInvite.config.commissionTitle')">
            <div class="form-grid">
              <UiSelect v-model="config.commission_type" size="md" :label="t('adminInvite.config.commissionType')" :options="commissionTypeOptions" />
              <UiTextField v-if="config.commission_type === 'fixed'" v-model.number="config.commission_fixed" size="md" type="number" step="0.01" :prefix="t('adminInvite.currencySymbol')" :label="t('adminInvite.config.commissionFixed')" />
              <UiTextField v-model.number="config.commission_rate" size="md" type="number" min="0" max="100" step="0.1" suffix="%" :label="t('adminInvite.config.commissionRate')" :help="t('adminInvite.config.commissionRateHelp')" />
            </div>
          </UiCard>
          <UiCard as="section" :title="t('adminInvite.config.withdrawTitle')">
            <div class="form-grid">
              <UiTextField v-model.number="config.min_withdraw" size="md" type="number" step="0.01" :prefix="t('adminInvite.currencySymbol')" :label="t('adminInvite.config.minWithdraw')" />
              <UiTextField v-model.number="config.withdraw_fee" size="md" type="number" min="0" max="100" step="0.1" suffix="%" :label="t('adminInvite.config.withdrawFee')" />
              <fieldset class="invite-methods form-grid__full">
                <legend class="invite-methods__legend">{{ t('adminInvite.config.withdrawMethods') }}</legend>
                <UiCheckbox
                  v-for="method in METHODS"
                  :key="method"
                  :model-value="config.withdraw_methods.includes(method)"
                  :label="getMethodLabel(method)"
                  @update:model-value="value => toggleMethod(method, value)"
                />
              </fieldset>
            </div>
          </UiCard>
          <div class="invite-config__actions">
            <UiButton variant="primary" :loading="configSaving" data-test="invite-config-save" @click="saveConfig">{{ t('common.actions.save') }}</UiButton>
          </div>
        </form>
      </template>
    </UiTabs>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { Check, Trophy, Wallet, X } from '@lucide/vue'
import { getInviteConfig, getInviteStats, getWithdrawals, processWithdrawal, updateInviteConfig } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTabs from '@/ui/UiTabs.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'

const { t } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const defaultInviteConfig = Object.freeze({
  enabled: false,
  code_prefix: 'INV',
  code_length: 8,
  commission_rate: 10,
  commission_type: 'percent',
  commission_fixed: 0,
  min_withdraw: 10,
  withdraw_fee: 0,
  withdraw_methods: ['alipay']
})

const METHODS = ['alipay', 'wechat', 'bank']
// The list comes first (list page template): withdrawals to review, then
// the stats, then the rules.
const activeTab = ref('withdrawals')
const withdrawals = ref([])
const withdrawalsLoading = ref(false)
const withdrawalsError = ref(null)
const stats = ref({})
const statsLoading = ref(false)
const statsError = ref(null)
const configLoading = ref(false)
const configError = ref(null)
const configSaving = ref(false)
const showConfigSkeleton = useDelayedLoading(configLoading)

const withdrawalFilter = ref({ status: '' })
const config = ref(createInviteConfig())

function createInviteConfig(source = {}) {
  return {
    ...defaultInviteConfig,
    ...source,
    withdraw_methods: normalizeWithdrawMethods(source.withdraw_methods ?? defaultInviteConfig.withdraw_methods),
    commission_rate: normalizeCommissionRate(source)
  }
}

function normalizeWithdrawMethods(value) {
  if (Array.isArray(value)) {
    return value.filter(Boolean)
  }
  if (typeof value === 'string' && value.trim()) {
    return value.split(',').map((item) => item.trim()).filter(Boolean)
  }
  return [...defaultInviteConfig.withdraw_methods]
}

function normalizeCommissionRate(source = {}) {
  if (source.commission_rate != null && Number.isFinite(Number(source.commission_rate))) {
    const value = Number(source.commission_rate)
    return value <= 1 && source.commission_rate_ratio == null ? value * 100 : value
  }
  if (source.commission_rate_ratio != null && Number.isFinite(Number(source.commission_rate_ratio))) {
    return Number(source.commission_rate_ratio) * 100
  }
  return defaultInviteConfig.commission_rate
}

const topInviters = computed(() => (
  Array.isArray(stats.value.top_inviters) ? stats.value.top_inviters : []
))
const rankedInviters = computed(() => topInviters.value.map((item, index) => ({ ...item, rank: index + 1 })))

const tabItems = computed(() => [
  { value: 'withdrawals', label: t('adminInvite.tabs.withdrawals') },
  { value: 'stats', label: t('adminInvite.tabs.stats') },
  { value: 'config', label: t('adminInvite.tabs.config') }
])
const statusChips = computed(() => ['pending', 'approved', 'rejected'].map(value => ({ value, label: getStatusLabel(value) })))
const commissionTypeOptions = computed(() => [
  { value: 'percent', label: t('adminInvite.types.percent') },
  { value: 'fixed', label: t('adminInvite.types.fixed') }
])
const statusTone = status => ({ pending: 'warning', approved: 'success', rejected: 'neutral' })[status] || 'neutral'
const withdrawalColumns = computed(() => [
  { key: 'id', label: t('adminInvite.withdrawals.table.id'), primary: true, sortable: true, numeric: true, firstDirection: 'desc', format: value => `#${value}` },
  { key: 'status', label: t('adminInvite.withdrawals.table.status'), secondary: true, sortable: true },
  { key: 'amount', label: t('adminInvite.withdrawals.table.amount'), sortable: true, numeric: true, align: 'end', firstDirection: 'desc', format: value => formatMoney(value) },
  { key: 'user_id', label: t('adminInvite.withdrawals.table.userId'), sortable: true, numeric: true },
  { key: 'method', label: t('adminInvite.withdrawals.table.method'), format: value => getMethodLabel(value) },
  { key: 'account', label: t('adminInvite.withdrawals.table.account'), format: value => maskAccount(value) },
  { key: 'created_at', label: t('adminInvite.withdrawals.table.createdAt'), sortable: true, nowrap: true, format: value => formatTime(value), breakpoint: 'lg' }
])
const withdrawalActions = item => (item.status === 'pending'
  ? [
      { key: 'approve', label: t('adminInvite.actions.approve'), icon: Check, onSelect: () => processWithdrawalRequest(item, true) },
      { key: 'reject', label: t('adminInvite.actions.reject'), icon: X, danger: true, separatorBefore: true, onSelect: () => processWithdrawalRequest(item, false) }
    ]
  : [])
const rankingColumns = computed(() => [
  { key: 'rank', label: t('adminInvite.stats.table.rank'), numeric: true, sortable: true, width: 80 },
  { key: 'user_id', label: t('adminInvite.stats.table.userId'), primary: true, numeric: true },
  { key: 'invite_count', label: t('adminInvite.stats.table.inviteCount'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc' },
  { key: 'commission', label: t('adminInvite.stats.table.commission'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => formatMoney(value) }
])
const statCards = computed(() => [
  { key: 'invites', label: t('adminInvite.stats.totalInvites'), value: format.number(Number(stats.value.total_invites || 0)) },
  { key: 'total', label: t('adminInvite.stats.totalCommission'), value: formatMoney(stats.value.total_commission) },
  { key: 'pending', label: t('adminInvite.stats.pendingCommission'), value: formatMoney(stats.value.pending_commission) },
  { key: 'withdrawn', label: t('adminInvite.stats.withdrawnCommission'), value: formatMoney(stats.value.withdrawn_commission) }
])

const toggleMethod = (method, on) => {
  const current = config.value.withdraw_methods.filter(item => item !== method)
  config.value.withdraw_methods = on === true ? METHODS.filter(item => item === method || current.includes(item)).concat(current.filter(item => !METHODS.includes(item))) : current
}

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const getMethodLabel = (method) => {
  switch (method) {
    case 'alipay':
      return t('adminInvite.methods.alipay')
    case 'wechat':
      return t('adminInvite.methods.wechat')
    case 'bank':
      return t('adminInvite.methods.bank')
    default:
      return method || '-'
  }
}

const getStatusLabel = (status) => {
  switch (status) {
    case 'pending':
      return t('adminInvite.status.pending')
    case 'approved':
      return t('adminInvite.status.approved')
    case 'rejected':
      return t('adminInvite.status.rejected')
    default:
      return status || '-'
  }
}

const formatTime = time => format.dateTime(time)

const formatMoney = (value) => {
  const amount = Number(value || 0)
  const currencySymbol = t('adminInvite.currencySymbol')
  return `${currencySymbol}${amount.toFixed(2)}`
}

const maskAccount = (account) => {
  if (!account) return '-'
  if (account.length <= 4) return account
  return `${account.substring(0, 2)}***${account.substring(account.length - 2)}`
}

const readInviteConfig = (res) => {
  const payload = readInvitePayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

const readInvitePayload = (res) => {
  if (!res || typeof res !== 'object') {
    return null
  }
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data ?? null
  }
  if (
    res.data &&
    typeof res.data === 'object' &&
    Object.prototype.hasOwnProperty.call(res.data, 'data')
  ) {
    return res.data.data ?? null
  }
  return res.data ?? res
}

const fetchConfig = async () => {
  configLoading.value = true
  try {
    const res = await getInviteConfig()
    const payload = readInviteConfig(res)
    if (payload) {
      config.value = createInviteConfig(payload)
    }
    configError.value = null
  } catch (error) {
    configError.value = error
  } finally {
    configLoading.value = false
  }
}

const saveConfig = async () => {
  if (configSaving.value) return
  configSaving.value = true
  try {
    const payload = {
      ...config.value,
      commission_rate_ratio: Number(config.value.commission_rate || 0) / 100
    }
    await updateInviteConfig(payload)
    toast.success(t('adminInvite.messages.saveSuccess'))
  } catch (error) {
    toast.error(t('adminInvite.messages.saveFailed', { message: resolveApiError(error, 'adminInvite.messages.saveFailedShort') }))
  } finally {
    configSaving.value = false
  }
}

const fetchWithdrawals = async () => {
  withdrawalsLoading.value = true
  try {
    const res = await getWithdrawals(withdrawalFilter.value)
    withdrawals.value = readWithdrawals(res)
    withdrawalsError.value = null
  } catch (error) {
    withdrawalsError.value = error
  } finally {
    withdrawalsLoading.value = false
  }
}
watch(() => withdrawalFilter.value.status, () => fetchWithdrawals())

const readWithdrawals = (res) => {
  const payload = readInvitePayload(res)
  if (Array.isArray(payload)) {
    return payload
  }
  if (payload && typeof payload === 'object') {
    return payload.list || payload.data || []
  }
  return []
}

// Approving or rejecting a withdrawal is final: ask first, with the amount
// and the (masked) account in the question.
const processWithdrawalRequest = async (item, approve) => {
  const facts = { id: item.id, amount: formatMoney(item.amount), account: maskAccount(item.account) }
  const confirmed = await confirm({
    title: t(approve ? 'adminInvite.confirm.approveTitle' : 'adminInvite.confirm.rejectTitle', facts),
    message: t(approve ? 'adminInvite.confirm.approveMessage' : 'adminInvite.confirm.rejectMessage', facts),
    confirmLabel: t(approve ? 'adminInvite.actions.approve' : 'adminInvite.actions.reject'),
    tone: approve ? 'default' : 'danger',
    onConfirm: async () => {
      try {
        await processWithdrawal(item.id, { approve })
      } catch (error) {
        throw new Error(resolveApiError(error, approve ? 'adminInvite.messages.approveFailedShort' : 'adminInvite.messages.rejectFailedShort'))
      }
    }
  })
  if (!confirmed) return
  toast.success(approve ? t('adminInvite.messages.approveSuccess') : t('adminInvite.messages.rejectSuccess'))
  await fetchWithdrawals()
  await fetchStats()
}

const fetchStats = async () => {
  statsLoading.value = true
  try {
    const res = await getInviteStats()
    stats.value = readInviteStats(res)
    statsError.value = null
  } catch (error) {
    statsError.value = error
  } finally {
    statsLoading.value = false
  }
}

const readInviteStats = (res) => {
  const payload = readInvitePayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

onMounted(() => {
  fetchConfig()
  fetchWithdrawals()
  fetchStats()
})
</script>

<style scoped>
.invite-stats {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.invite-stats__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-4);
}

.invite-stat {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.invite-stat__label {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.invite-stat__value {
  font-size: var(--type-title-2-size);
  font-weight: var(--type-title-2-weight);
  line-height: var(--type-title-2-line);
}

.invite-config {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  max-width: 760px;
}

.invite-methods {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3) var(--space-6);
  padding: 0;
  margin: 0;
  border: 0;
}

.invite-methods__legend {
  width: 100%;
  margin-bottom: var(--space-2);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
}

.invite-config__actions {
  display: flex;
  justify-content: flex-end;
}
</style>
