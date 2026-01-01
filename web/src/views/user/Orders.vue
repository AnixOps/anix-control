<template>
  <div class="orders-page">
    <div class="page-header">
      <h1>📦 我的订单</h1>
      <p class="text-secondary">查看并维护您的历史订单记录</p>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <p>正在拉取订单记录...</p>
    </div>

    <div v-else-if="orders.length === 0" class="empty-state">
      <div class="empty-icon">🛒</div>
      <p>您还没有任何订单，前往选购心仪的套餐吧</p>
      <router-link to="/user/plans" class="btn-primary mt-4">立即选购</router-link>
    </div>

    <div v-else class="content-container">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>订单号</th>
              <th>套餐</th>
              <th>周期</th>
              <th>金额</th>
              <th>状态</th>
              <th>创建时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="order in orders" :key="order.id">
              <td class="trade-no">{{ order.trade_no }}</td>
              <td>{{ order.plan?.name || '未知套餐' }}</td>
              <td>{{ formatPeriod(order.period) }}</td>
              <td>¥{{ formatPrice(order.total_amount) }}</td>
              <td>
                <span :class="['status-badge', statusClass(order.status)]">
                  {{ statusText(order.status) }}
                </span>
              </td>
              <td class="time text-secondary">{{ formatDateTime(order.created_at) }}</td>
              <td>
                <div class="action-buttons">
                  <button v-if="order.status === 0" class="btn-sm btn-primary" @click="goPay(order)">去支付</button>
                  <button class="btn-sm btn-ghost" @click="viewDetail(order)">详情</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 订单详情弹窗 -->
    <div v-if="showDetail" class="modal-overlay" @click.self="showDetail = false">
      <div class="modal modal-md">
        <div class="modal-header">
          <h3>订单详情</h3>
          <button class="close-btn" @click="showDetail = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="detail-grid">
            <div class="detail-item">
              <span class="label">订单编号</span>
              <span class="value">{{ currentOrder.trade_no }}</span>
            </div>
            <div class="detail-item">
              <span class="label">订单状态</span>
              <span :class="['value', statusClass(currentOrder.status)]">{{ statusText(currentOrder.status) }}</span>
            </div>
            <div class="detail-item">
              <span class="label">套餐内容</span>
              <span class="value">{{ currentOrder.plan?.name }}</span>
            </div>
            <div class="detail-item">
              <span class="label">购买周期</span>
              <span class="value">{{ formatPeriod(currentOrder.period) }}</span>
            </div>
            <div class="detail-item">
              <span class="label">订单总额</span>
              <span class="value">¥{{ formatPrice(currentOrder.total_amount) }}</span>
            </div>
            <div v-if="currentOrder.discount_amount" class="detail-item">
              <span class="label">优惠抵扣</span>
              <span class="value text-success">-¥{{ formatPrice(currentOrder.discount_amount) }}</span>
            </div>
            <div class="detail-item">
              <span class="label">创建时间</span>
              <span class="value">{{ formatDateTime(currentOrder.created_at) }}</span>
            </div>
            <div v-if="currentOrder.paid_at" class="detail-item">
              <span class="label">支付时间</span>
              <span class="value">{{ formatDateTimeFromUnix(currentOrder.paid_at) }}</span>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-primary" @click="showDetail = false">返回</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getOrders, getOrderDetail } from '@/api/user'

const orders = ref([])
const loading = ref(true)
const showDetail = ref(false)
const currentOrder = ref({})

const loadOrders = async () => {
  loading.value = true
  try {
    const res = await getOrders({ page: 1, page_size: 50 })
    orders.value = res.data?.list || []
  } catch (err) {
    console.error('获取订单失败:', err)
  } finally {
    loading.value = false
  }
}

const statusText = (s) => {
  return ['待支付', '已支付', '已取消', '已完成', '已折价'][s] || '未知'
}

const statusClass = (s) => {
  return ['pending', 'paid', 'cancelled', 'completed', 'discounted'][s] || ''
}

const formatPeriod = (p) => {
  const map = {
    'month': '月付',
    'quarter': '季付',
    'half_year': '半年付',
    'year': '年付',
    'two_year': '两年付',
    'three_year': '三年付',
    'onetime': '一次性'
  }
  return map[p] || p
}

const formatPrice = (cent) => {
  return (cent / 100).toFixed(2)
}

const formatDateTime = (iso) => {
  return new Date(iso).toLocaleString('zh-CN')
}

const formatDateTimeFromUnix = (ts) => {
  return new Date(ts * 1000).toLocaleString('zh-CN')
}

const viewDetail = async (order) => {
  try {
    const res = await getOrderDetail(order.id)
    currentOrder.value = res.data
    showDetail.value = true
  } catch (err) {
    alert('获取详情失败')
  }
}

const goPay = (order) => {
  // TODO: 功能开发中，目前可以引导至支付模块
  alert('支付功能正在集成中，敬请期待')
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

.data-table th, .data-table td {
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

.status-badge.pending { background: rgba(59, 130, 246, 0.1); color: var(--primary-color); }
.status-badge.paid, .status-badge.completed { background: rgba(34, 197, 94, 0.1); color: var(--success-color); }
.status-badge.cancelled { background: rgba(239, 68, 68, 0.1); color: var(--error-color); }

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

.text-success { color: var(--success-color); }

.loading-state, .empty-state {
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

@keyframes rotate { to { transform: rotate(360deg); } }
.empty-icon { font-size: 48px; margin-bottom: 16px; }
</style>
