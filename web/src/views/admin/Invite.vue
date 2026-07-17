<template>
  <div class="invite-page">
    <div class="page-header">
      <h1>{{ t('adminInvite.title') }}</h1>
      <p class="text-secondary">{{ t('adminInvite.subtitle') }}</p>
    </div>

    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'config' }]" @click="activeTab = 'config'">
        {{ t('adminInvite.tabs.config') }}
      </button>
      <button :class="['tab', { active: activeTab === 'withdrawals' }]" @click="activeTab = 'withdrawals'">
        {{ t('adminInvite.tabs.withdrawals') }}
      </button>
      <button :class="['tab', { active: activeTab === 'stats' }]" @click="activeTab = 'stats'">
        {{ t('adminInvite.tabs.stats') }}
      </button>
    </div>

    <div v-show="activeTab === 'config'">
      <div class="config-section">
        <h3>{{ t('adminInvite.config.inviteTitle') }}</h3>
        <div class="form-group">
          <label class="checkbox-label">
            <input v-model="config.enabled" type="checkbox" />
            <span>{{ t('adminInvite.config.enabled') }}</span>
          </label>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label>{{ t('adminInvite.config.codePrefix') }}</label>
            <input
              v-model="config.code_prefix"
              type="text"
              :placeholder="t('adminInvite.placeholders.codePrefix')"
            />
          </div>
          <div class="form-group">
            <label>{{ t('adminInvite.config.codeLength') }}</label>
            <input v-model.number="config.code_length" type="number" min="4" max="16" />
          </div>
        </div>

        <h4>{{ t('adminInvite.config.commissionTitle') }}</h4>
        <div class="form-row">
          <div class="form-group">
            <label>{{ t('adminInvite.config.commissionRate') }}</label>
            <input v-model.number="config.commission_rate" type="number" min="0" max="100" step="0.1" />
            <p class="help-text">{{ t('adminInvite.config.commissionRateHelp') }}</p>
          </div>
          <div class="form-group">
            <label>{{ t('adminInvite.config.commissionType') }}</label>
            <select v-model="config.commission_type">
              <option value="percent">{{ t('adminInvite.types.percent') }}</option>
              <option value="fixed">{{ t('adminInvite.types.fixed') }}</option>
            </select>
          </div>
        </div>
        <div v-if="config.commission_type === 'fixed'" class="form-group">
          <label>{{ t('adminInvite.config.commissionFixed') }}</label>
          <input v-model.number="config.commission_fixed" type="number" step="0.01" />
        </div>

        <h4>{{ t('adminInvite.config.withdrawTitle') }}</h4>
        <div class="form-row">
          <div class="form-group">
            <label>{{ t('adminInvite.config.minWithdraw') }}</label>
            <input v-model.number="config.min_withdraw" type="number" step="0.01" />
          </div>
          <div class="form-group">
            <label>{{ t('adminInvite.config.withdrawFee') }}</label>
            <input v-model.number="config.withdraw_fee" type="number" min="0" max="100" step="0.1" />
          </div>
        </div>
        <div class="form-group">
          <label>{{ t('adminInvite.config.withdrawMethods') }}</label>
          <div class="checkbox-group">
            <label class="checkbox-label">
              <input v-model="config.withdraw_methods" type="checkbox" value="alipay" />
              <span>{{ t('adminInvite.methods.alipay') }}</span>
            </label>
            <label class="checkbox-label">
              <input v-model="config.withdraw_methods" type="checkbox" value="wechat" />
              <span>{{ t('adminInvite.methods.wechat') }}</span>
            </label>
            <label class="checkbox-label">
              <input v-model="config.withdraw_methods" type="checkbox" value="bank" />
              <span>{{ t('adminInvite.methods.bank') }}</span>
            </label>
          </div>
        </div>

        <div class="form-actions">
          <button class="btn-primary" @click="saveConfig">{{ t('common.actions.save') }}</button>
        </div>
      </div>
    </div>

    <div v-show="activeTab === 'withdrawals'">
      <div class="toolbar">
        <select v-model="withdrawalFilter.status">
          <option value="">{{ t('adminInvite.withdrawals.filters.all') }}</option>
          <option value="pending">{{ t('adminInvite.status.pending') }}</option>
          <option value="approved">{{ t('adminInvite.status.approved') }}</option>
          <option value="rejected">{{ t('adminInvite.status.rejected') }}</option>
        </select>
        <button class="btn-secondary" @click="fetchWithdrawals">{{ t('adminInvite.actions.search') }}</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('adminInvite.withdrawals.table.id') }}</th>
              <th>{{ t('adminInvite.withdrawals.table.userId') }}</th>
              <th>{{ t('adminInvite.withdrawals.table.amount') }}</th>
              <th>{{ t('adminInvite.withdrawals.table.method') }}</th>
              <th>{{ t('adminInvite.withdrawals.table.account') }}</th>
              <th>{{ t('adminInvite.withdrawals.table.status') }}</th>
              <th>{{ t('adminInvite.withdrawals.table.createdAt') }}</th>
              <th>{{ t('adminInvite.withdrawals.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in withdrawals" :key="item.id">
              <td>{{ item.id }}</td>
              <td>{{ item.user_id }}</td>
              <td>{{ formatMoney(item.amount) }}</td>
              <td>{{ getMethodLabel(item.method) }}</td>
              <td>{{ maskAccount(item.account) }}</td>
              <td>
                <span :class="['status-badge', `status-${item.status}`]">
                  {{ getStatusLabel(item.status) }}
                </span>
              </td>
              <td>{{ formatTime(item.created_at) }}</td>
              <td>
                <div v-if="item.status === 'pending'" class="action-buttons">
                  <button
                    class="btn-sm btn-primary"
                    :title="t('adminInvite.actions.approve')"
                    :aria-label="t('adminInvite.actions.approve')"
                    @click="processWithdrawalRequest(item, true)"
                  >
                    {{ t('adminInvite.actions.approve') }}
                  </button>
                  <button
                    class="btn-sm btn-danger"
                    :title="t('adminInvite.actions.reject')"
                    :aria-label="t('adminInvite.actions.reject')"
                    @click="processWithdrawalRequest(item, false)"
                  >
                    {{ t('adminInvite.actions.reject') }}
                  </button>
                </div>
                <span v-else class="text-secondary">-</span>
              </td>
            </tr>
            <tr v-if="withdrawals.length === 0">
              <td colspan="8" class="empty-row">{{ t('adminInvite.withdrawals.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-show="activeTab === 'stats'">
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">{{ stats.total_invites || 0 }}</div>
          <div class="stat-label">{{ t('adminInvite.stats.totalInvites') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ formatMoney(stats.total_commission) }}</div>
          <div class="stat-label">{{ t('adminInvite.stats.totalCommission') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ formatMoney(stats.pending_commission) }}</div>
          <div class="stat-label">{{ t('adminInvite.stats.pendingCommission') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ formatMoney(stats.withdrawn_commission) }}</div>
          <div class="stat-label">{{ t('adminInvite.stats.withdrawnCommission') }}</div>
        </div>
      </div>

      <div class="chart-section">
        <h3>{{ t('adminInvite.stats.rankingTitle') }}</h3>
        <div class="table-container">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ t('adminInvite.stats.table.rank') }}</th>
                <th>{{ t('adminInvite.stats.table.userId') }}</th>
                <th>{{ t('adminInvite.stats.table.inviteCount') }}</th>
                <th>{{ t('adminInvite.stats.table.commission') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(item, index) in topInviters" :key="item.user_id">
                <td>{{ index + 1 }}</td>
                <td>{{ item.user_id }}</td>
                <td>{{ item.invite_count }}</td>
                <td>{{ formatMoney(item.commission) }}</td>
              </tr>
              <tr v-if="topInviters.length === 0">
                <td colspan="4" class="empty-row">{{ t('adminInvite.stats.empty') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getInviteConfig, getInviteStats, getWithdrawals, processWithdrawal, updateInviteConfig } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()

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

const activeTab = ref('config')
const withdrawals = ref([])
const stats = ref({})

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

const formatTime = (time) => {
  if (!time) return '-'
  return formatDateTime(time)
}

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
  try {
    const res = await getInviteConfig()
    const payload = readInviteConfig(res)
    if (payload) {
      config.value = createInviteConfig(payload)
    }
  } catch (error) {
    console.error(t('adminInvite.messages.fetchConfigFailed'), error)
  }
}

const saveConfig = async () => {
  try {
    const payload = {
      ...config.value,
      commission_rate_ratio: Number(config.value.commission_rate || 0) / 100
    }
    await updateInviteConfig(payload)
    window.alert(t('adminInvite.messages.saveSuccess'))
  } catch (error) {
    window.alert(t('adminInvite.messages.saveFailed', { message: resolveApiError(error, 'adminInvite.messages.saveFailedShort') }))
  }
}

const fetchWithdrawals = async () => {
  try {
    const res = await getWithdrawals(withdrawalFilter.value)
    withdrawals.value = readWithdrawals(res)
  } catch (error) {
    console.error(t('adminInvite.messages.fetchWithdrawalsFailed'), error)
  }
}

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

const processWithdrawalRequest = async (item, approve) => {
  const confirmMessage = approve
    ? t('adminInvite.messages.approveConfirm')
    : t('adminInvite.messages.rejectConfirm')

  if (!window.confirm(confirmMessage)) return

  try {
    await processWithdrawal(item.id, { approve })
    window.alert(approve ? t('adminInvite.messages.approveSuccess') : t('adminInvite.messages.rejectSuccess'))
    await fetchWithdrawals()
    await fetchStats()
  } catch (error) {
    window.alert(
      approve
        ? t('adminInvite.messages.approveFailed', { message: resolveApiError(error, 'adminInvite.messages.approveFailedShort') })
        : t('adminInvite.messages.rejectFailed', { message: resolveApiError(error, 'adminInvite.messages.rejectFailedShort') })
    )
  }
}

const fetchStats = async () => {
  try {
    const res = await getInviteStats()
    stats.value = readInviteStats(res)
  } catch (error) {
    console.error(t('adminInvite.messages.fetchStatsFailed'), error)
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
.config-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
}

.config-section h3 {
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-color);
}

.config-section h4 {
  margin-top: 24px;
  margin-bottom: 12px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 18px;
  height: 18px;
}

.checkbox-group {
  display: flex;
  gap: 16px;
  margin-top: 8px;
}

.help-text {
  color: var(--text-secondary);
  font-size: 12px;
  margin-top: 4px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 20px;
  margin-bottom: 30px;
}

.stat-card {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  text-align: center;
  border: 1px solid var(--border-color);
}

.stat-value {
  font-size: 2rem;
  font-weight: 700;
  color: var(--primary-color);
}

.stat-label {
  color: var(--text-secondary);
  margin-top: 8px;
}

.chart-section {
  background: var(--surface-color);
  padding: 24px;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
}

.chart-section h3 {
  margin-bottom: 16px;
}

.status-pending {
  background: rgba(251, 191, 36, 0.15);
  color: #fbbf24;
}

.status-approved {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}

.status-rejected {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.btn-danger {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  border: 1px solid rgba(239, 68, 68, 0.2);
}

.btn-danger:hover {
  background: rgba(239, 68, 68, 0.2);
}
</style>
