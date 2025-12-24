<template>
  <div class="dashboard-page">
    <div class="page-header">
      <div>
        <h1>控制台</h1>
        <p class="text-secondary">系统运行概览</p>
      </div>
      <button class="btn-secondary" @click="refreshData" :disabled="loading">
        {{ loading ? '刷新中...' : '🔄 刷新数据' }}
      </button>
    </div>

    <!-- 核心指标 -->
    <div class="stats-grid">
      <div class="stat-card primary">
        <div class="stat-icon">👥</div>
        <div class="stat-info">
          <div class="stat-value">{{ formatNumber(stats.total_users) }}</div>
          <div class="stat-label">总用户数</div>
        </div>
        <div class="stat-detail">
          今日新增 <span class="highlight">+{{ stats.today_new_users || 0 }}</span>
        </div>
      </div>

      <div class="stat-card success">
        <div class="stat-icon">✅</div>
        <div class="stat-info">
          <div class="stat-value">{{ formatNumber(stats.active_users) }}</div>
          <div class="stat-label">有效用户</div>
        </div>
        <div class="stat-detail">
          过期 {{ stats.expired_users || 0 }} · 封禁 {{ stats.banned_users || 0 }}
        </div>
      </div>

      <div class="stat-card info">
        <div class="stat-icon">🖥️</div>
        <div class="stat-info">
          <div class="stat-value">{{ stats.active_nodes || 0 }} / {{ stats.total_nodes || 0 }}</div>
          <div class="stat-label">活跃节点</div>
        </div>
        <div class="stat-detail">
          在线用户 <span class="highlight">{{ formatNumber(stats.online_users) }}</span>
        </div>
      </div>

      <div class="stat-card warning">
        <div class="stat-icon">💰</div>
        <div class="stat-info">
          <div class="stat-value">¥{{ formatMoney(stats.monthly_income) }}</div>
          <div class="stat-label">本月收入</div>
        </div>
        <div class="stat-detail">
          今日 ¥{{ formatMoney(stats.today_income) }} · 总计 ¥{{ formatMoney(stats.total_revenue) }}
        </div>
      </div>
    </div>

    <!-- 订单概览 -->
    <div class="section-grid">
      <div class="section-card">
        <h3>📋 订单概览</h3>
        <div class="order-stats">
          <div class="order-stat-item">
            <span class="label">总订单</span>
            <span class="value">{{ formatNumber(stats.total_orders) }}</span>
          </div>
          <div class="order-stat-item">
            <span class="label">待支付</span>
            <span class="value pending">{{ stats.pending_orders || 0 }}</span>
          </div>
          <div class="order-stat-item">
            <span class="label">已完成</span>
            <span class="value success">{{ stats.paid_orders || 0 }}</span>
          </div>
        </div>
      </div>

      <div class="section-card">
        <h3>📊 流量统计</h3>
        <div class="traffic-stats">
          <div class="traffic-item">
            <span class="label">总使用流量</span>
            <span class="value">{{ formatBytes(stats.total_traffic_used) }}</span>
          </div>
          <div class="traffic-item">
            <span class="label">今日流量</span>
            <span class="value">{{ formatBytes(stats.today_traffic) }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 缓存信息 -->
    <div class="cache-info" v-if="stats.cached_at">
      <span>📌 数据缓存于 {{ formatDateTime(stats.cached_at) }}</span>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getDashboard } from '@/api/admin'

const stats = ref({})
const loading = ref(false)

const fetchData = async (refresh = false) => {
  loading.value = true
  try {
    const res = await getDashboard(refresh)
    stats.value = res.data || {}
  } catch (err) {
    console.error('获取数据失败:', err)
  } finally {
    loading.value = false
  }
}

const refreshData = () => {
  fetchData(true)
}

const formatNumber = (num) => {
  if (!num) return '0'
  return num.toLocaleString()
}

const formatMoney = (cents) => {
  if (!cents) return '0.00'
  return (cents / 100).toFixed(2)
}

const formatBytes = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (bytes >= 1024 && i < units.length - 1) {
    bytes /= 1024
    i++
  }
  return bytes.toFixed(2) + ' ' + units[i]
}

const formatDateTime = (dateStr) => {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  return date.toLocaleString('zh-CN')
}

onMounted(() => fetchData())
</script>

<style scoped>
.dashboard-page {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  font-size: 24px;
  font-weight: 600;
  margin-bottom: 4px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 20px;
  margin-bottom: 24px;
}

.stat-card {
  background: var(--surface-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  border: 1px solid var(--border-color);
  position: relative;
  overflow: hidden;
}

.stat-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
}

.stat-card.primary::before { background: var(--primary-color); }
.stat-card.success::before { background: var(--success-color); }
.stat-card.info::before { background: #3b82f6; }
.stat-card.warning::before { background: #f59e0b; }

.stat-icon {
  font-size: 28px;
}

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-color);
  line-height: 1.2;
}

.stat-label {
  font-size: 14px;
  color: var(--text-secondary);
  margin-top: 4px;
}

.stat-detail {
  font-size: 13px;
  color: var(--text-secondary);
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
}

.stat-detail .highlight {
  color: var(--primary-color);
  font-weight: 600;
}

.section-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 20px;
  margin-bottom: 24px;
}

.section-card {
  background: var(--surface-color);
  border-radius: var(--radius-lg);
  padding: 20px;
  border: 1px solid var(--border-color);
}

.section-card h3 {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 16px;
}

.order-stats, .traffic-stats {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.order-stat-item, .traffic-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: var(--bg-color);
  border-radius: var(--radius-md);
}

.order-stat-item .label, .traffic-item .label {
  color: var(--text-secondary);
  font-size: 14px;
}

.order-stat-item .value, .traffic-item .value {
  font-size: 18px;
  font-weight: 600;
}

.order-stat-item .value.pending { color: #f59e0b; }
.order-stat-item .value.success { color: var(--success-color); }

.cache-info {
  text-align: center;
  padding: 12px;
  background: var(--surface-color);
  border-radius: var(--radius-md);
  font-size: 12px;
  color: var(--text-secondary);
}

/* 移动端适配 */
@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
  }

  .stats-grid {
    grid-template-columns: 1fr;
  }

  .stat-value {
    font-size: 24px;
  }
}
</style>
