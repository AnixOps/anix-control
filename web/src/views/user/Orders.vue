<template>
  <div class="orders-page">
    <div class="page-header">
      <h1>{{ t('user.orders.title') }}</h1>
      <p class="text-secondary">{{ t('user.orders.subtitle') }}</p>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>{{ t('user.orders.loading') }}</p>
    </div>

    <div v-else-if="orders.length === 0" class="empty-state">
      <div class="empty-icon">0</div>
      <p>{{ t('user.orders.empty') }}</p>
      <router-link to="/user/plans" class="btn-primary mt-4">{{ t('user.orders.buyNow') }}</router-link>
    </div>

    <div v-else class="content-container">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('user.orders.headers.tradeNo') }}</th>
              <th>{{ t('user.orders.headers.plan') }}</th>
              <th>{{ t('user.orders.headers.period') }}</th>
              <th>{{ t('user.orders.headers.amount') }}</th>
              <th>{{ t('user.orders.headers.status') }}</th>
              <th>{{ t('user.orders.headers.createdAt') }}</th>
              <th>{{ t('user.orders.headers.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="order in orders" :key="order.id">
              <td class="trade-no">{{ order.trade_no }}</td>
              <td>{{ order.plan?.name || t('user.orders.unknownPlan') }}</td>
              <td>{{ formatPeriod(order.period) }}</td>
              <td>¥{{ formatPrice(order.total_amount) }}</td>
              <td>
                <span :class="['status-badge', statusClass(order.status)]">{{ statusText(order.status) }}</span>
              </td>
              <td class="time text-secondary">{{ formatDateTime(order.created_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button v-if="order.status === 0" class="btn-sm btn-primary" @click="goPay(order)">{{ t('common.actions.payNow') }}</button>
                  <button class="btn-sm btn-ghost" @click="viewDetail(order)">{{ t('common.actions.details') }}</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="showDetail" class="modal-overlay" @click.self="showDetail = false">
      <div class="modal modal-md">
        <div class="modal-header">
          <h3>{{ t('user.orders.detailTitle') }}</h3>
          <button class="close-btn" @click="showDetail = false">×</button>
        </div>
        <div class="modal-body">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="label">{{ t('user.orders.labels.tradeNo') }}</span>
              <span class="value">{{ currentOrder.trade_no }}</span>
            </div>
            <div class="detail-item">
              <span class="label">{{ t('user.orders.labels.status') }}</span>
              <span :class="['value', statusClass(currentOrder.status)]">{{ statusText(currentOrder.status) }}</span>
            </div>
            <div class="detail-item">
              <span class="label">{{ t('user.orders.labels.plan') }}</span>
              <span class="value">{{ currentOrder.plan?.name || '-' }}</span>
            </div>
            <div class="detail-item">
              <span class="label">{{ t('user.orders.labels.period') }}</span>
              <span class="value">{{ formatPeriod(currentOrder.period) }}</span>
            </div>
            <div class="detail-item">
              <span class="label">{{ t('user.orders.labels.totalAmount') }}</span>
              <span class="value">¥{{ formatPrice(currentOrder.total_amount) }}</span>
            </div>
            <div v-if="currentOrder.discount_amount" class="detail-item">
              <span class="label">{{ t('user.orders.labels.discount') }}</span>
              <span class="value text-success">-¥{{ formatPrice(currentOrder.discount_amount) }}</span>
            </div>
            <div class="detail-item">
              <span class="label">{{ t('user.orders.labels.createdAt') }}</span>
              <span class="value">{{ formatDateTime(currentOrder.created_at) }}</span>
            </div>
            <div v-if="currentOrder.paid_at" class="detail-item">
              <span class="label">{{ t('user.orders.labels.paidAt') }}</span>
              <span class="value">{{ formatDateTime(currentOrder.paid_at) }}</span>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-primary" @click="showDetail = false">{{ t('common.actions.back') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { getOrderDetail, getOrders } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()
const orders = ref([])
const loading = ref(true)
const showDetail = ref(false)
const currentOrder = ref({})

async function loadOrders() {
  loading.value = true
  try {
    const res = await getOrders({ page: 1, page_size: 50 })
    orders.value = res.data?.list || []
  } catch (err) {
    console.error('Failed to load orders:', err)
  } finally {
    loading.value = false
  }
}

function statusText(status) {
  return [
    t('common.states.pending'),
    t('common.states.paid'),
    t('common.states.cancelled'),
    t('common.states.completed'),
    t('common.states.discounted')
  ][status] || t('common.states.unknown')
}

function statusClass(status) {
  return ['pending', 'paid', 'cancelled', 'completed', 'discounted'][status] || ''
}

function formatPeriod(period) {
  const map = {
    month: t('common.periods.month'),
    quarter: t('common.periods.quarter'),
    half_year: t('common.periods.halfYear'),
    year: t('common.periods.year'),
    two_year: t('common.periods.twoYear'),
    three_year: t('common.periods.threeYear'),
    onetime: t('common.periods.onetime')
  }
  return map[period] || period || '-'
}

function formatPrice(amount) {
  return ((amount || 0) / 100).toFixed(2)
}

async function viewDetail(order) {
  try {
    const res = await getOrderDetail(order.id)
    currentOrder.value = res.data || {}
    showDetail.value = true
  } catch {
    alert(t('common.messages.loadFailed'))
  }
}

function goPay() {
  alert(t('common.messages.paymentPending'))
}

onMounted(() => {
  loadOrders()
})
</script>

<style scoped>
.orders-page {
  max-width: 1100px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: 32px;
}

.table-container {
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 16px;
  text-align: left;
  border-bottom: 1px solid var(--border-color);
}

.data-table th {
  background: var(--bg-color);
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  text-transform: uppercase;
}

.trade-no {
  font-family: monospace;
  font-size: 13px;
}

.status-badge {
  font-size: 12px;
  font-weight: 600;
  padding: 4px 10px;
  border-radius: 6px;
}

.status-badge.pending {
  background: rgba(59, 130, 246, 0.1);
  color: var(--primary-color);
}

.status-badge.paid,
.status-badge.completed {
  background: rgba(34, 197, 94, 0.1);
  color: var(--success-color);
}

.status-badge.cancelled {
  background: rgba(239, 68, 68, 0.1);
  color: var(--error-color);
}

.detail-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-item {
  display: flex;
  justify-content: space-between;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border-color);
}

.detail-item:last-child {
  border-bottom: none;
}

.detail-item .label {
  color: var(--text-secondary);
}

.detail-item .value {
  font-weight: 600;
}

.text-success {
  color: var(--success-color);
}

.loading-state,
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 100px 0;
  color: var(--text-secondary);
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid rgba(59, 130, 246, 0.1);
  border-top-color: var(--primary-color);
  border-radius: 50%;
  animation: rotate 1s linear infinite;
  margin-bottom: 16px;
}

@keyframes rotate {
  to {
    transform: rotate(360deg);
  }
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
}
</style>
