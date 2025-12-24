<template>
  <div class="orders-page">
    <div class="page-header">
      <h1>订单管理</h1>
      <p class="text-secondary">管理所有订单</p>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon">📋</div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.total_orders || 0 }}</div>
          <div class="stat-label">总订单数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">⏳</div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.pending_orders || 0 }}</div>
          <div class="stat-label">待支付</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">💰</div>
        <div class="stat-info">
          <div class="stat-value">¥{{ formatMoney(stats.total_revenue) }}</div>
          <div class="stat-label">总收入</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon">📈</div>
        <div class="stat-info">
          <div class="stat-value">¥{{ formatMoney(stats.today_revenue) }}</div>
          <div class="stat-label">今日收入</div>
        </div>
      </div>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <input
        v-model="filters.trade_no"
        type="text"
        placeholder="搜索订单号..."
        class="search-input"
        @keyup.enter="fetchOrders"
      />
      <input
        v-model="filters.email"
        type="text"
        placeholder="搜索用户邮箱..."
        class="search-input"
        @keyup.enter="fetchOrders"
      />
      <select v-model="filters.status" @change="fetchOrders">
        <option value="">全部状态</option>
        <option value="0">待支付</option>
        <option value="1">已支付</option>
        <option value="2">已取消</option>
        <option value="3">已完成</option>
      </select>
      <button class="btn-secondary" @click="fetchOrders">
        🔍 搜索
      </button>
    </div>

    <!-- 订单列表 -->
    <div class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>订单号</th>
            <th>用户</th>
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
            <td>
              <div class="order-no">{{ order.trade_no }}</div>
            </td>
            <td>{{ order.user?.email || '-' }}</td>
            <td>{{ order.plan?.name || '-' }}</td>
            <td>{{ getPeriodText(order.period) }}</td>
            <td>
              <div class="amount">¥{{ formatMoney(order.total_amount) }}</div>
            </td>
            <td>
              <span :class="['status-badge', getStatusClass(order.status)]">
                {{ getStatusText(order.status) }}
              </span>
            </td>
            <td>{{ formatDateTime(order.created_at) }}</td>
            <td>
              <div class="action-buttons">
                <button 
                  v-if="order.status === 0"
                  class="btn-sm btn-ghost" 
                  @click="handleMarkPaid(order)" 
                  title="手动开通"
                >
                  ✅
                </button>
                <button 
                  v-if="order.status === 0"
                  class="btn-sm btn-ghost" 
                  @click="handleCancel(order)" 
                  title="取消订单"
                >
                  ❌
                </button>
                <button class="btn-sm btn-ghost" @click="viewDetail(order)" title="查看详情">
                  👁️
                </button>
              </div>
            </td>
          </tr>
          <tr v-if="orders.length === 0">
            <td colspan="8" class="empty-row">暂无订单</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 分页 -->
    <div class="pagination">
      <button 
        class="btn-sm btn-secondary" 
        :disabled="page <= 1"
        @click="page--; fetchOrders()"
      >
        上一页
      </button>
      <span class="page-info">第 {{ page }} 页 / 共 {{ totalPages }} 页</span>
      <button 
        class="btn-sm btn-secondary"
        :disabled="page >= totalPages"
        @click="page++; fetchOrders()"
      >
        下一页
      </button>
    </div>

    <!-- 详情弹窗 -->
    <div v-if="showDetailModal" class="modal-overlay" @click.self="showDetailModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>订单详情</h3>
          <button class="close-btn" @click="showDetailModal = false">✕</button>
        </div>
        <div class="modal-body">
          <div class="detail-row">
            <span class="label">订单号</span>
            <span class="value">{{ selectedOrder.trade_no }}</span>
          </div>
          <div class="detail-row">
            <span class="label">用户邮箱</span>
            <span class="value">{{ selectedOrder.user?.email }}</span>
          </div>
          <div class="detail-row">
            <span class="label">套餐</span>
            <span class="value">{{ selectedOrder.plan?.name }}</span>
          </div>
          <div class="detail-row">
            <span class="label">周期</span>
            <span class="value">{{ getPeriodText(selectedOrder.period) }}</span>
          </div>
          <div class="detail-row">
            <span class="label">金额</span>
            <span class="value amount">¥{{ formatMoney(selectedOrder.total_amount) }}</span>
          </div>
          <div class="detail-row">
            <span class="label">状态</span>
            <span :class="['value', 'status-badge', getStatusClass(selectedOrder.status)]">
              {{ getStatusText(selectedOrder.status) }}
            </span>
          </div>
          <div class="detail-row">
            <span class="label">订单类型</span>
            <span class="value">{{ getTypeText(selectedOrder.type) }}</span>
          </div>
          <div class="detail-row">
            <span class="label">创建时间</span>
            <span class="value">{{ formatDateTime(selectedOrder.created_at) }}</span>
          </div>
          <div v-if="selectedOrder.paid_at" class="detail-row">
            <span class="label">支付时间</span>
            <span class="value">{{ formatDateTime(selectedOrder.paid_at * 1000) }}</span>
          </div>
          <div v-if="selectedOrder.callback_no" class="detail-row">
            <span class="label">外部交易号</span>
            <span class="value">{{ selectedOrder.callback_no }}</span>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showDetailModal = false">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getOrderList, getOrderStats, markOrderPaid, cancelOrder } from '@/api/admin'

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
  } catch (err) {
    console.error('获取订单列表失败:', err)
  }
}

const fetchStats = async () => {
  try {
    const res = await getOrderStats()
    stats.value = res.data || {}
  } catch (err) {
    console.error('获取统计失败:', err)
  }
}

const viewDetail = (order) => {
  selectedOrder.value = order
  showDetailModal.value = true
}

const handleMarkPaid = async (order) => {
  if (!confirm(`确定要手动开通订单 ${order.trade_no} 吗？这将为用户开通对应套餐。`)) return
  try {
    await markOrderPaid(order.id)
    alert('订单已开通')
    fetchOrders()
    fetchStats()
  } catch (err) {
    alert('操作失败: ' + (err.response?.data?.message || err.message))
  }
}

const handleCancel = async (order) => {
  if (!confirm(`确定要取消订单 ${order.trade_no} 吗？`)) return
  try {
    await cancelOrder(order.id)
    fetchOrders()
    fetchStats()
  } catch (err) {
    alert('操作失败')
  }
}

const getStatusClass = (status) => {
  const classes = {
    0: 'status-pending',
    1: 'status-paid',
    2: 'status-cancelled',
    3: 'status-completed'
  }
  return classes[status] || ''
}

const getStatusText = (status) => {
  const texts = {
    0: '待支付',
    1: '已支付',
    2: '已取消',
    3: '已完成'
  }
  return texts[status] || '未知'
}

const getTypeText = (type) => {
  const texts = {
    1: '新购',
    2: '续费',
    3: '升级',
    4: '重置流量'
  }
  return texts[type] || '未知'
}

const getPeriodText = (period) => {
  const texts = {
    'month': '月付',
    'quarter': '季付',
    'half_year': '半年付',
    'year': '年付',
    'two_year': '两年付',
    'three_year': '三年付',
    'onetime': '一次性'
  }
  return texts[period] || period
}

const formatMoney = (cents) => {
  if (!cents) return '0.00'
  return (cents / 100).toFixed(2)
}

const formatDateTime = (datetime) => {
  if (!datetime) return '-'
  return new Date(datetime).toLocaleString('zh-CN')
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
  display: flex;
  align-items: center;
  gap: 16px;
}

.stat-icon {
  font-size: 32px;
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

/* 弹窗样式 */
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
