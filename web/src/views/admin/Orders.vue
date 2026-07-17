<template>
  <div class="page-shell">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminOrders.title') }}</h1>
        <p>{{ t('adminOrders.subtitle') }}</p>
      </div>
    </div>

    <section class="metrics-grid">
      <article class="section-panel metric-card">
        <span class="metric-code primary">ORD</span>
        <strong>{{ stats.total_orders || 0 }}</strong>
        <span>{{ t('adminOrders.stats.totalOrders') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code warning">PND</span>
        <strong>{{ stats.pending_orders || 0 }}</strong>
        <span>{{ t('adminOrders.stats.pendingOrders') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code success">REV</span>
        <strong>{{ formatMoney(stats.total_revenue) }}</strong>
        <span>{{ t('adminOrders.stats.totalRevenue') }}</span>
      </article>
      <article class="section-panel metric-card">
        <span class="metric-code info">DAY</span>
        <strong>{{ formatMoney(stats.today_revenue) }}</strong>
        <span>{{ t('adminOrders.stats.todayRevenue') }}</span>
      </article>
    </section>

    <section class="section-panel filter-panel">
      <div class="filter-row">
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
        <button class="btn" @click="fetchOrders">
          {{ t('adminOrders.actions.search') }}
        </button>
      </div>
    </section>

    <section class="section-panel data-panel">
      <div class="table-wrap">
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
                    class="btn btn-sm"
                    :title="t('adminOrders.actions.markPaid')"
                    :aria-label="t('adminOrders.actions.markPaid')"
                    @click="handleMarkPaid(order)"
                  >
                    {{ t('adminOrders.actions.markPaid') }}
                  </button>
                  <button
                    v-if="Number(order.status) === 0"
                    class="btn btn-sm"
                    :title="t('adminOrders.actions.cancelOrder')"
                    :aria-label="t('adminOrders.actions.cancelOrder')"
                    @click="handleCancel(order)"
                  >
                    {{ t('adminOrders.actions.cancelOrder') }}
                  </button>
                  <button
                    class="btn btn-sm"
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
        <button class="btn btn-sm" :disabled="page <= 1" @click="page -= 1; fetchOrders()">
          {{ t('adminOrders.pagination.prev') }}
        </button>
        <span class="page-info">{{ t('adminOrders.pagination.info', { page, totalPages }) }}</span>
        <button class="btn btn-sm" :disabled="page >= totalPages" @click="page += 1; fetchOrders()">
          {{ t('adminOrders.pagination.next') }}
        </button>
      </div>
    </section>

    <div v-if="showDetailModal" class="modal-overlay" @click.self="showDetailModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('adminOrders.detailModal.title') }}</h3>
          <button class="btn btn-ghost btn-sm close-btn" :title="t('common.actions.close')" :aria-label="t('common.actions.close')" @click="showDetailModal = false">x</button>
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
          <button class="btn" @click="showDetailModal = false">{{ t('common.actions.close') }}</button>
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

const ensureOrderSuccess = (res, fallbackKey) => {
  if (res && typeof res === 'object' && typeof res.code === 'number' && res.code !== 0) {
    throw new Error(res.msg || t(fallbackKey))
  }
  return res
}

const fetchOrders = async () => {
  try {
    const res = ensureOrderSuccess(
      await getOrderList({
        page: page.value,
        page_size: pageSize.value,
        trade_no: filters.value.trade_no,
        email: filters.value.email,
        status: filters.value.status || undefined
      }),
      'adminOrders.messages.fetchOrdersFailed'
    )
    const payload = readOrderPage(res)
    orders.value = payload.list || []
    total.value = payload.total || 0
  } catch (error) {
    console.error(t('adminOrders.messages.fetchOrdersFailed'), error)
  }
}

const fetchStats = async () => {
  try {
    const res = ensureOrderSuccess(
      await getOrderStats(),
      'adminOrders.messages.fetchStatsFailed'
    )
    stats.value = readOrderStats(res)
  } catch (error) {
    console.error(t('adminOrders.messages.fetchStatsFailed'), error)
  }
}

const readOrderPayload = (res) => {
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

const readOrderPage = (res) => {
  const payload = readOrderPayload(res)
  return payload && typeof payload === 'object' ? payload : {}
}

const readOrderStats = (res) => {
  const payload = readOrderPayload(res)
  return payload && typeof payload === 'object' ? payload : {}
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
    ensureOrderSuccess(
      await markOrderPaid(order.id),
      'adminOrders.messages.markPaidFailedShort'
    )
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
    ensureOrderSuccess(
      await cancelOrder(order.id),
      'adminOrders.messages.cancelFailedShort'
    )
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
  return `\u00a5${new Intl.NumberFormat(currentLocale.value, {
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
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
}

.metric-card {
  padding: 18px;
  display: grid;
  gap: 6px;
}

.metric-card strong {
  font-size: 28px;
  line-height: 1.1;
}

.metric-card span:last-child {
  font-size: 13px;
  color: var(--text-secondary);
}

.metric-code {
  width: fit-content;
  min-width: 44px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
}

.metric-code.primary { background: rgba(0, 100, 250, 0.08); color: var(--primary-color); }
.metric-code.success { background: rgba(22, 163, 74, 0.08); color: var(--success-color); }
.metric-code.warning { background: rgba(217, 119, 6, 0.08); color: var(--warning-color); }
.metric-code.info { background: rgba(37, 99, 235, 0.08); color: #2563eb; }

.filter-panel,
.data-panel {
  padding: 18px;
}

.filter-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.search-input {
  flex: 1;
  min-width: 180px;
}

.filter-row select {
  width: 180px;
}

.table-wrap {
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
  vertical-align: top;
}

.data-table th {
  font-weight: 600;
  font-size: 13px;
  color: var(--text-secondary);
  text-transform: uppercase;
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
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  font-size: 12px;
  padding: 0 10px;
  border-radius: 999px;
  font-weight: 700;
}

.status-pending {
  background: rgba(217, 119, 6, 0.08);
  color: var(--warning-color);
}

.status-paid {
  background: rgba(0, 100, 250, 0.08);
  color: var(--primary-color);
}

.status-cancelled {
  background: rgba(220, 38, 38, 0.08);
  color: var(--error-color);
}

.status-completed {
  background: rgba(22, 163, 74, 0.08);
  color: var(--success-color);
}

.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
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
  flex-wrap: wrap;
}

.page-info {
  font-size: 14px;
  color: var(--text-secondary);
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.42);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
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
  min-width: 36px;
  font-size: 18px;
}

.modal-body {
  padding: 20px;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  gap: 16px;
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
  flex-wrap: wrap;
}

@media (max-width: 768px) {
  .detail-row {
    flex-direction: column;
    gap: 4px;
  }
}
</style>
