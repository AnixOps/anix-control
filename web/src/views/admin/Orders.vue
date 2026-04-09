<template>
  <div class="orders-page">
    <div class="page-header">
      <h1>{{ t('adminOrders.title') }}</h1>
      <p class="text-secondary">{{ t('adminOrders.subtitle') }}</p>
    </div>

    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-info">
          <div class="stat-value">{{ stats.total_orders || 0 }}</div>
          <div class="stat-label">{{ t('adminOrders.stats.totalOrders') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-info">
          <div class="stat-value">{{ stats.pending_orders || 0 }}</div>
          <div class="stat-label">{{ t('adminOrders.stats.pendingOrders') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-info">
          <div class="stat-value">{{ formatMoney(stats.total_revenue) }}</div>
          <div class="stat-label">{{ t('adminOrders.stats.totalRevenue') }}</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-info">
          <div class="stat-value">{{ formatMoney(stats.today_revenue) }}</div>
          <div class="stat-label">{{ t('adminOrders.stats.todayRevenue') }}</div>
        </div>
      </div>
    </div>

    <div class="filter-bar">
      <input
        v-model="filters.trade_no"
        type="text"
        :placeholder="t('adminOrders.filters.tradeNo')"
        class="search-input"
        @keyup.enter="fetchOrders"
      />
      <input
        v-model="filters.email"
        type="text"
        :placeholder="t('adminOrders.filters.email')"
        class="search-input"
        @keyup.enter="fetchOrders"
      />
      <select v-model="filters.status" @change="fetchOrders">
        <option value="">{{ t('adminOrders.filters.allStatus') }}</option>
        <option value="0">{{ t('adminOrders.status.pending') }}</option>
        <option value="1">{{ t('adminOrders.status.paid') }}</option>
        <option value="2">{{ t('adminOrders.status.cancelled') }}</option>
        <option value="3">{{ t('adminOrders.status.completed') }}</option>
      </select>
      <button class="btn-secondary" @click="fetchOrders">
        {{ t('adminOrders.actions.search') }}
      </button>
    </div>

    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>{{ t('adminOrders.table.tradeNo') }}</th>
            <th>{{ t('adminOrders.table.user') }}</th>
            <th>{{ t('adminOrders.table.plan') }}</th>
            <th>{{ t('adminOrders.table.period') }}</th>
            <th>{{ t('adminOrders.table.amount') }}</th>
            <th>{{ t('adminOrders.table.status') }}</th>
            <th>{{ t('adminOrders.table.createdAt') }}</th>
            <th>{{ t('adminOrders.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="order in orders" :key="order.id">
            <td>
              <div class="order-no">{{ order.trade_no }}</div>
            </td>
            <td>{{ order.user?.email || '-' }}</td>
            <td>{{ order.plan?.name || '-' }}</td>
            <td>{{ getPeriodText(order.period) }}</td>
            <td>
              <div class="amount">{{ formatMoney(order.total_amount) }}</div>
            </td>
            <td>
              <span :class="['status-badge', getStatusClass(order.status)]">
                {{ getStatusText(order.status) }}
              </span>
            </td>
            <td>{{ formatTimestamp(order.created_at) }}</td>
            <td>
              <div class="action-buttons">
                <button
                  v-if="Number(order.status) === 0"
                  class="btn-sm btn-ghost"
                  :title="t('adminOrders.actions.markPaid')"
                  :aria-label="t('adminOrders.actions.markPaid')"
                  @click="handleMarkPaid(order)"
                >
                  {{ t('adminOrders.actions.markPaid') }}
                </button>
                <button
                  v-if="Number(order.status) === 0"
                  class="btn-sm btn-ghost"
                  :title="t('adminOrders.actions.cancelOrder')"
                  :aria-label="t('adminOrders.actions.cancelOrder')"
                  @click="handleCancel(order)"
                >
                  {{ t('adminOrders.actions.cancelOrder') }}
                </button>
                <button
                  class="btn-sm btn-ghost"
                  :title="t('common.actions.details')"
                  :aria-label="t('common.actions.details')"
                  @click="viewDetail(order)"
                >
                  {{ t('common.actions.details') }}
                </button>
              </div>
            </td>
          </tr>
          <tr v-if="orders.length === 0">
            <td colspan="8" class="empty-row">{{ t('adminOrders.empty.noData') }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="pagination">
      <button
        class="btn-sm btn-secondary"
        :disabled="page <= 1"
        @click="page -= 1; fetchOrders()"
      >
        {{ t('adminOrders.pagination.prev') }}
      </button>
      <span class="page-info">{{ t('adminOrders.pagination.info', { page, totalPages }) }}</span>
      <button
        class="btn-sm btn-secondary"
        :disabled="page >= totalPages"
        @click="page += 1; fetchOrders()"
      >
        {{ t('adminOrders.pagination.next') }}
      </button>
    </div>

    <div v-if="showDetailModal" class="modal-overlay" @click.self="showDetailModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('adminOrders.detailModal.title') }}</h3>
          <button class="close-btn" @click="showDetailModal = false">×</button>
        </div>
        <div class="modal-body">
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.tradeNo') }}</span>
            <span class="value">{{ selectedOrder.trade_no }}</span>
          </div>
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.userEmail') }}</span>
            <span class="value">{{ selectedOrder.user?.email || '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.plan') }}</span>
            <span class="value">{{ selectedOrder.plan?.name || '-' }}</span>
          </div>
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.period') }}</span>
            <span class="value">{{ getPeriodText(selectedOrder.period) }}</span>
          </div>
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.amount') }}</span>
            <span class="value amount">{{ formatMoney(selectedOrder.total_amount) }}</span>
          </div>
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.status') }}</span>
            <span :class="['value', 'status-badge', getStatusClass(selectedOrder.status)]">
              {{ getStatusText(selectedOrder.status) }}
            </span>
          </div>
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.type') }}</span>
            <span class="value">{{ getTypeText(selectedOrder.type) }}</span>
          </div>
          <div class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.createdAt') }}</span>
            <span class="value">{{ formatTimestamp(selectedOrder.created_at) }}</span>
          </div>
          <div v-if="selectedOrder.paid_at" class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.paidAt') }}</span>
            <span class="value">{{ formatPaidAt(selectedOrder.paid_at) }}</span>
          </div>
          <div v-if="selectedOrder.callback_no" class="detail-row">
            <span class="label">{{ t('adminOrders.detailModal.callbackNo') }}</span>
            <span class="value">{{ selectedOrder.callback_no }}</span>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showDetailModal = false">{{ t('common.actions.close') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { cancelOrder, getOrderList, getOrderStats, markOrderPaid } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { currentLocale, t, formatDateTime } = useAppI18n()

const orders = ref([])
const stats = ref({})
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const filters = ref({
  trade_no: '',
  email: '',
  status: ''
})
const showDetailModal = ref(false)
const selectedOrder = ref({})

const totalPages = computed(() => Math.ceil(total.value / pageSize.value) || 1)

const resolveApiError = (error, fallbackKey) => (
  error?.response?.data?.error ||
  error?.response?.data?.message ||
  error?.response?.data?.msg ||
  error?.message ||
  t(fallbackKey)
)

const fetchOrders = async () => {
  try {
    const res = await getOrderList({
      page: page.value,
      page_size: pageSize.value,
      trade_no: filters.value.trade_no,
      email: filters.value.email,
      status: filters.value.status || undefined
    })
    orders.value = res.data?.list || []
    total.value = res.data?.total || 0
  } catch (error) {
    console.error(t('adminOrders.messages.fetchOrdersFailed'), error)
  }
}

const fetchStats = async () => {
  try {
    const res = await getOrderStats()
    stats.value = res.data || {}
  } catch (error) {
    console.error(t('adminOrders.messages.fetchStatsFailed'), error)
  }
}

const viewDetail = (order) => {
  selectedOrder.value = order
  showDetailModal.value = true
}

const handleMarkPaid = async (order) => {
  if (!window.confirm(t('adminOrders.messages.markPaidConfirm', { tradeNo: order.trade_no }))) {
    return
  }

  try {
    await markOrderPaid(order.id)
    window.alert(t('adminOrders.messages.markPaidSuccess'))
    await fetchOrders()
    await fetchStats()
  } catch (error) {
    window.alert(t('adminOrders.messages.markPaidFailed', {
      message: resolveApiError(error, 'adminOrders.messages.markPaidFailedShort')
    }))
  }
}

const handleCancel = async (order) => {
  if (!window.confirm(t('adminOrders.messages.cancelConfirm', { tradeNo: order.trade_no }))) {
    return
  }

  try {
    await cancelOrder(order.id)
    await fetchOrders()
    await fetchStats()
  } catch (error) {
    window.alert(t('adminOrders.messages.cancelFailed', {
      message: resolveApiError(error, 'adminOrders.messages.cancelFailedShort')
    }))
  }
}

const getStatusClass = (status) => {
  const classes = {
    0: 'status-pending',
    1: 'status-paid',
    2: 'status-cancelled',
    3: 'status-completed'
  }
  return classes[Number(status)] || ''
}

const getStatusText = (status) => {
  switch (Number(status)) {
    case 0:
      return t('adminOrders.status.pending')
    case 1:
      return t('adminOrders.status.paid')
    case 2:
      return t('adminOrders.status.cancelled')
    case 3:
      return t('adminOrders.status.completed')
    default:
      return t('adminOrders.status.unknown')
  }
}

const getTypeText = (type) => {
  switch (Number(type)) {
    case 1:
      return t('adminOrders.types.new')
    case 2:
      return t('adminOrders.types.renew')
    case 3:
      return t('adminOrders.types.upgrade')
    case 4:
      return t('adminOrders.types.resetTraffic')
    default:
      return t('adminOrders.types.unknown')
  }
}

const getPeriodText = (period) => {
  switch (period) {
    case 'month':
      return t('common.periods.month')
    case 'quarter':
      return t('common.periods.quarter')
    case 'half_year':
      return t('common.periods.halfYear')
    case 'year':
      return t('common.periods.year')
    case 'two_year':
      return t('common.periods.twoYear')
    case 'three_year':
      return t('common.periods.threeYear')
    case 'onetime':
      return t('common.periods.onetime')
    default:
      return period || '-'
  }
}

const formatMoney = (cents) => {
  const amount = Number(cents || 0) / 100
  return `¥${new Intl.NumberFormat(currentLocale.value, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(amount)}`
}

const formatTimestamp = (value) => {
  if (!value) return '-'
  return formatDateTime(value)
}

const formatPaidAt = (value) => {
  const numericValue = Number(value)
  if (!value) return '-'
  if (Number.isFinite(numericValue) && numericValue < 1e12) {
    return formatTimestamp(numericValue * 1000)
  }
  return formatTimestamp(value)
}

onMounted(() => {
  fetchOrders()
  fetchStats()
})
</script>

<style scoped>
.orders-page {
  max-width: 1400px;
}

.page-header {
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 24px;
  margin-bottom: 4px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

.stat-card {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  padding: 20px;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
}

.stat-label {
  font-size: 13px;
  color: var(--text-secondary);
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}

.search-input {
  flex: 1;
  min-width: 150px;
}

.filter-bar select {
  min-width: 120px;
}

.table-container {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow-x: auto;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 14px 16px;
  text-align: left;
  border-bottom: 1px solid var(--border-color);
}

.data-table th {
  background: var(--bg-color);
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.data-table tr:hover {
  background: var(--bg-color);
}

.order-no {
  font-family: monospace;
  font-size: 13px;
}

.amount {
  font-weight: 600;
  color: var(--success-color);
}

.status-badge {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 20px;
  font-weight: 500;
}

.status-pending {
  background: rgba(245, 158, 11, 0.15);
  color: var(--warning-color);
}

.status-paid {
  background: rgba(59, 130, 246, 0.15);
  color: var(--primary-color);
}

.status-cancelled {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.status-completed {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.action-buttons {
  display: flex;
  gap: 4px;
}

.empty-row {
  text-align: center;
  color: var(--text-secondary);
  padding: 40px !important;
}

.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  margin-top: 20px;
}

.page-info {
  font-size: 14px;
  color: var(--text-secondary);
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px;
  border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
  font-size: 18px;
  font-weight: 600;
}

.close-btn {
  background: transparent;
  border: none;
  font-size: 18px;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 4px;
}

.modal-body {
  padding: 20px;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  padding: 12px 0;
  border-bottom: 1px solid var(--border-color);
}

.detail-row:last-child {
  border-bottom: none;
}

.detail-row .label {
  color: var(--text-secondary);
  font-size: 14px;
}

.detail-row .value {
  font-weight: 500;
  font-size: 14px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 20px;
  border-top: 1px solid var(--border-color);
}
</style>
