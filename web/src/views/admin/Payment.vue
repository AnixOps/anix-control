<template>
  <div class="payment-page">
    <div class="page-header">
      <h1>{{ t('adminPayment.title') }}</h1>
      <p class="text-secondary">{{ t('adminPayment.subtitle') }}</p>
    </div>

    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'gateways' }]" @click="activeTab = 'gateways'">
        {{ t('adminPayment.tabs.gateways') }}
      </button>
      <button :class="['tab', { active: activeTab === 'records' }]" @click="activeTab = 'records'">
        {{ t('adminPayment.tabs.records') }}
      </button>
      <button :class="['tab', { active: activeTab === 'stats' }]" @click="activeTab = 'stats'">
        {{ t('adminPayment.tabs.stats') }}
      </button>
    </div>

    <div v-show="activeTab === 'gateways'">
      <div class="toolbar">
        <button class="btn-primary" @click="openGatewayModal()">
          + {{ t('adminPayment.actions.createGateway') }}
        </button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('adminPayment.gateways.table.id') }}</th>
              <th>{{ t('adminPayment.gateways.table.name') }}</th>
              <th>{{ t('adminPayment.gateways.table.type') }}</th>
              <th>{{ t('adminPayment.gateways.table.feeRate') }}</th>
              <th>{{ t('adminPayment.gateways.table.minAmount') }}</th>
              <th>{{ t('adminPayment.gateways.table.maxAmount') }}</th>
              <th>{{ t('adminPayment.gateways.table.status') }}</th>
              <th>{{ t('adminPayment.gateways.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="gateway in gateways" :key="gateway.id">
              <td>{{ gateway.id }}</td>
              <td>{{ gateway.name || '-' }}</td>
              <td>
                <span :class="['type-badge', gateway.type]">
                  {{ getGatewayTypeLabel(gateway.type) }}
                </span>
              </td>
              <td>{{ formatFeeRate(gateway.fee_rate) }}</td>
              <td>{{ formatMoney(gateway.min_amount) }}</td>
              <td>{{ formatMoney(gateway.max_amount) }}</td>
              <td>
                <span :class="['status-badge', gateway.enabled ? 'status-active' : 'status-disabled']">
                  {{ gateway.enabled ? t('adminPayment.status.enabled') : t('adminPayment.status.disabled') }}
                </span>
              </td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn-sm btn-secondary"
                    :title="gateway.enabled ? t('adminPayment.actions.disable') : t('adminPayment.actions.enable')"
                    :aria-label="gateway.enabled ? t('adminPayment.actions.disable') : t('adminPayment.actions.enable')"
                    @click="toggleGatewayStatus(gateway)"
                  >
                    {{ gateway.enabled ? t('adminPayment.actions.disable') : t('adminPayment.actions.enable') }}
                  </button>
                  <button
                    class="btn-sm btn-ghost"
                    :title="t('adminPayment.actions.edit')"
                    :aria-label="t('adminPayment.actions.edit')"
                    @click="openGatewayModal(gateway)"
                  >
                    {{ t('adminPayment.actions.edit') }}
                  </button>
                  <button
                    class="btn-sm btn-danger"
                    :title="t('adminPayment.actions.delete')"
                    :aria-label="t('adminPayment.actions.delete')"
                    @click="deleteGatewayItem(gateway)"
                  >
                    {{ t('adminPayment.actions.delete') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="gateways.length === 0">
              <td colspan="8" class="empty-row">{{ t('adminPayment.gateways.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-show="activeTab === 'records'">
      <div class="toolbar">
        <select v-model="recordFilter.status">
          <option value="">{{ t('adminPayment.records.filters.allStatuses') }}</option>
          <option v-for="status in paymentStatuses" :key="status" :value="status">
            {{ getPaymentStatusLabel(status) }}
          </option>
        </select>
        <select v-model="recordFilter.gateway_type">
          <option value="">{{ t('adminPayment.records.filters.allTypes') }}</option>
          <option v-for="type in gatewayTypes" :key="type" :value="type">
            {{ getGatewayTypeLabel(type) }}
          </option>
        </select>
        <button class="btn-secondary" @click="fetchRecords">{{ t('adminPayment.actions.search') }}</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('adminPayment.records.table.id') }}</th>
              <th>{{ t('adminPayment.records.table.tradeNo') }}</th>
              <th>{{ t('adminPayment.records.table.userId') }}</th>
              <th>{{ t('adminPayment.records.table.gateway') }}</th>
              <th>{{ t('adminPayment.records.table.amount') }}</th>
              <th>{{ t('adminPayment.records.table.status') }}</th>
              <th>{{ t('adminPayment.records.table.createdAt') }}</th>
              <th>{{ t('adminPayment.records.table.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="record in records" :key="record.id">
              <td>{{ record.id }}</td>
              <td>{{ record.trade_no || '-' }}</td>
              <td>{{ record.user_id ?? '-' }}</td>
              <td>{{ getGatewayTypeLabel(record.gateway_type) }}</td>
              <td>{{ formatMoney(record.amount) }}</td>
              <td>
                <span :class="['status-badge', `status-${record.status}`]">
                  {{ getPaymentStatusLabel(record.status) }}
                </span>
              </td>
              <td>{{ formatTime(record.created_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn-sm btn-ghost"
                    :title="t('adminPayment.actions.details')"
                    :aria-label="t('adminPayment.actions.details')"
                    @click="viewRecord(record)"
                  >
                    {{ t('adminPayment.actions.details') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="records.length === 0">
              <td colspan="8" class="empty-row">{{ t('adminPayment.records.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-show="activeTab === 'stats'">
      <div class="stats-grid">
        <div class="stat-card">
          <div class="stat-value">{{ formatMoney(stats.total_amount) }}</div>
          <div class="stat-label">{{ t('adminPayment.stats.totalAmount') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats.total_orders || 0 }}</div>
          <div class="stat-label">{{ t('adminPayment.stats.totalOrders') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats.success_orders || 0 }}</div>
          <div class="stat-label">{{ t('adminPayment.stats.successOrders') }}</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ formatSuccessRate(stats.success_rate) }}</div>
          <div class="stat-label">{{ t('adminPayment.stats.successRate') }}</div>
        </div>
      </div>

      <div class="chart-section">
        <h3>{{ t('adminPayment.stats.gatewayDistribution') }}</h3>
        <div class="gateway-stats">
          <div v-for="[type, item] in gatewayStatsEntries" :key="type" class="gateway-stat-item">
            <span class="gateway-name">{{ getGatewayTypeLabel(type) }}</span>
            <div class="progress-bar">
              <div class="progress-fill" :style="{ width: `${getGatewayPercent(type)}%` }"></div>
            </div>
            <span class="gateway-amount">{{ formatMoney(item?.amount) }}</span>
          </div>
          <div v-if="gatewayStatsEntries.length === 0" class="empty-state">
            {{ t('adminPayment.stats.empty') }}
          </div>
        </div>
      </div>
    </div>

    <div v-if="showGatewayModal" class="modal-overlay" @click.self="showGatewayModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ editingGateway ? t('adminPayment.modal.editTitle') : t('adminPayment.modal.createTitle') }}</h3>
          <button class="close-btn" @click="showGatewayModal = false">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('adminPayment.modal.fields.name') }} <span class="required">*</span></label>
            <input
              v-model="gatewayForm.name"
              type="text"
              :placeholder="t('adminPayment.modal.placeholders.name')"
            />
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('adminPayment.modal.fields.type') }}</label>
              <select v-model="gatewayForm.type">
                <option v-for="type in gatewayTypes" :key="type" :value="type">
                  {{ getGatewayTypeLabel(type) }}
                </option>
              </select>
            </div>
            <div class="form-group">
              <label>{{ t('adminPayment.modal.fields.feeRate') }}</label>
              <input
                v-model.number="gatewayForm.fee_rate"
                type="number"
                step="0.001"
                :placeholder="t('adminPayment.modal.placeholders.feeRate')"
              />
            </div>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label>{{ t('adminPayment.modal.fields.minAmount') }}</label>
              <input
                v-model.number="gatewayForm.min_amount"
                type="number"
                :placeholder="t('adminPayment.modal.placeholders.minAmount')"
              />
            </div>
            <div class="form-group">
              <label>{{ t('adminPayment.modal.fields.maxAmount') }}</label>
              <input
                v-model.number="gatewayForm.max_amount"
                type="number"
                :placeholder="t('adminPayment.modal.placeholders.maxAmount')"
              />
            </div>
          </div>
          <div class="form-group">
            <label>{{ t('adminPayment.modal.fields.configJson') }}</label>
            <textarea
              v-model="gatewayForm.config_json"
              rows="4"
              :placeholder="t('adminPayment.modal.placeholders.configJson')"
            ></textarea>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showGatewayModal = false">{{ t('common.actions.cancel') }}</button>
          <button @click="saveGateway">{{ t('common.actions.save') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
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

const { t, formatDateTime } = useAppI18n()

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
  error?.message ||
  t(fallbackKey)
)

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

const getGatewayTypeLabel = (type) => (gatewayTypeKeyMap[type] ? t(gatewayTypeKeyMap[type]) : type || '-')
const getPaymentStatusLabel = (status) => (paymentStatusKeyMap[status] ? t(paymentStatusKeyMap[status]) : status || '-')

const formatTime = (value) => {
  if (!value) return '-'
  return formatDateTime(value)
}

const formatMoney = (value) => {
  const amount = Number(value || 0)
  return `${t('adminPayment.currencySymbol')}${amount.toFixed(2)}`
}

const formatFeeRate = (value) => `${(Number(value || 0) * 100).toFixed(2)}%`

const formatSuccessRate = (value) => `${Number(value || 0).toFixed(1)}%`

const getGatewayPercent = (type) => {
  const totalAmount = Number(stats.value.total_amount || 0)
  const gatewayAmount = Number(stats.value.by_gateway?.[type]?.amount || 0)
  if (!totalAmount || !gatewayAmount) return 0
  return Number(((gatewayAmount / totalAmount) * 100).toFixed(1))
}

const fetchGateways = async () => {
  try {
    const res = await getPaymentGateways()
    gateways.value = res.data?.list || []
  } catch (error) {
    console.error(t('adminPayment.messages.fetchGatewaysFailed'), error)
  }
}

const fetchRecords = async () => {
  try {
    const res = await getPaymentRecords(recordFilter.value)
    records.value = res.data?.list || []
  } catch (error) {
    console.error(t('adminPayment.messages.fetchRecordsFailed'), error)
  }
}

const fetchStats = async () => {
  try {
    const res = await getPaymentStats()
    stats.value = res.data || {}
  } catch (error) {
    console.error(t('adminPayment.messages.fetchStatsFailed'), error)
  }
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
  showGatewayModal.value = true
}

const saveGateway = async () => {
  try {
    const payload = { ...gatewayForm.value }
    if (payload.config_json) {
      try {
        payload.config = JSON.parse(payload.config_json)
      } catch (error) {
        window.alert(t('adminPayment.messages.invalidConfigJson'))
        return
      }
    }
    delete payload.config_json

    if (editingGateway.value) {
      await updatePaymentGateway(editingGateway.value.id, payload)
    } else {
      await createPaymentGateway(payload)
    }
    window.alert(t('adminPayment.messages.gatewaySaveSuccess'))
    showGatewayModal.value = false
    await fetchGateways()
  } catch (error) {
    window.alert(
      t('adminPayment.messages.gatewaySaveFailed', {
        message: resolveApiError(error, 'adminPayment.messages.gatewaySaveFailedShort')
      })
    )
  }
}

const toggleGatewayStatus = async (gateway) => {
  try {
    await togglePaymentGateway(gateway.id, !gateway.enabled)
    await fetchGateways()
  } catch (error) {
    window.alert(
      t('adminPayment.messages.toggleFailed', {
        message: resolveApiError(error, 'adminPayment.messages.toggleFailedShort')
      })
    )
  }
}

const deleteGatewayItem = async (gateway) => {
  if (!window.confirm(t('adminPayment.messages.deleteConfirm', { name: gateway.name }))) return
  try {
    await deletePaymentGateway(gateway.id)
    await fetchGateways()
  } catch (error) {
    window.alert(
      t('adminPayment.messages.deleteFailed', {
        message: resolveApiError(error, 'adminPayment.messages.deleteFailedShort')
      })
    )
  }
}

const viewRecord = (record) => {
  window.alert([
    t('adminPayment.records.detail.title'),
    `${t('adminPayment.records.detail.tradeNo')}: ${record.trade_no || '-'}`,
    `${t('adminPayment.records.detail.amount')}: ${formatMoney(record.amount)}`,
    `${t('adminPayment.records.detail.status')}: ${getPaymentStatusLabel(record.status)}`
  ].join('\n'))
}

onMounted(() => {
  fetchGateways()
  fetchRecords()
  fetchStats()
})
</script>

<style scoped>
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

.gateway-stats {
  margin-top: 16px;
}

.gateway-stat-item {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
}

.gateway-name {
  width: 80px;
  color: var(--text-secondary);
}

.progress-bar {
  flex: 1;
  height: 8px;
  background: var(--border-color);
  border-radius: 4px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--primary-color);
  transition: width 0.3s;
}

.gateway-amount {
  width: 100px;
  text-align: right;
  font-weight: 500;
}

.type-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 4px;
}

.type-badge.alipay {
  background: rgba(0, 132, 255, 0.15);
  color: #0084ff;
}

.type-badge.wechat {
  background: rgba(7, 193, 96, 0.15);
  color: #07c160;
}

.type-badge.stripe {
  background: rgba(99, 91, 255, 0.15);
  color: #635bff;
}

.type-badge.usdt {
  background: rgba(38, 161, 123, 0.15);
  color: #26a17b;
}

.type-badge.epay {
  background: rgba(255, 153, 0, 0.15);
  color: #ff9900;
}

.action-buttons {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.btn-danger {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  border: 1px solid rgba(239, 68, 68, 0.2);
}

.btn-danger:hover {
  background: rgba(239, 68, 68, 0.18);
}

.empty-state {
  color: var(--text-secondary);
  padding: 12px 0;
}

.status-pending {
  background: rgba(251, 191, 36, 0.15);
  color: #fbbf24;
}

.status-paid {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}

.status-failed {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.status-refunded {
  background: rgba(107, 114, 128, 0.15);
  color: #6b7280;
}
</style>
