<template>
  <div class="dashboard-page">
    <div class="page-header">
      <div>
        <h1>我的订阅</h1>
        <p class="text-secondary">查看您的套餐和使用情况</p>
      </div>
      <button class="btn-secondary" @click="refreshData" :disabled="loading">
        {{ loading ? '刷新中...' : '🔄 刷新' }}
      </button>
    </div>

    <!-- 套餐信息 -->
    <div class="subscription-card" :class="{ expired: sub.is_expired }">
      <div class="plan-header">
        <div class="plan-info">
          <div class="plan-name">{{ sub.plan_name || '未订阅' }}</div>
          <div class="plan-status" :class="statusClass">
            {{ statusText }}
          </div>
        </div>
        <div class="plan-expire">
          <div v-if="sub.expired_at > 0">
            <span class="label">到期时间</span>
            <span class="value">{{ formatExpireDate(sub.expired_at) }}</span>
          </div>
          <div v-else class="permanent">永久有效</div>
        </div>
      </div>

      <!-- 流量使用 -->
      <div class="traffic-section">
        <div class="traffic-header">
          <span class="traffic-label">流量使用</span>
          <span class="traffic-value">
            {{ formatBytes(sub.used_traffic) }} / {{ formatBytes(sub.transfer_enable) }}
          </span>
        </div>
        <div class="traffic-bar">
          <div class="traffic-progress" :style="{ width: progressWidth }" :class="progressClass"></div>
        </div>
        <div class="traffic-detail">
          <span>↑ 上传 {{ formatBytes(sub.upload_traffic) }}</span>
          <span>↓ 下载 {{ formatBytes(sub.download_traffic) }}</span>
        </div>
      </div>

      <!-- 剩余时间 -->
      <div class="remaining-section" v-if="!sub.is_expired && sub.days_remaining >= 0">
        <div class="remaining-box">
          <div class="remaining-value">{{ sub.days_remaining }}</div>
          <div class="remaining-label">剩余天数</div>
        </div>
        <div class="remaining-box">
          <div class="remaining-value">{{ remainingPercent }}%</div>
          <div class="remaining-label">流量剩余</div>
        </div>
      </div>
    </div>

    <!-- 快捷操作 -->
    <div class="quick-actions">
      <router-link to="/user/subscribe" class="action-card">
        <span class="action-icon">📋</span>
        <span class="action-text">订阅管理</span>
      </router-link>
      <router-link to="/user/orders" class="action-card">
        <span class="action-icon">📦</span>
        <span class="action-text">我的订单</span>
      </router-link>
      <router-link to="/user/invite" class="action-card">
        <span class="action-icon">🎁</span>
        <span class="action-text">邀请返利</span>
      </router-link>
      <router-link to="/user/settings" class="action-card">
        <span class="action-icon">⚙️</span>
        <span class="action-text">账户设置</span>
      </router-link>
    </div>

    <!-- 缓存提示 -->
    <div class="cache-info" v-if="sub.cached_at">
      <span>数据更新于 {{ formatDateTime(sub.cached_at) }}</span>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getSubscription } from '@/api/user'

const sub = ref({})
const loading = ref(false)

const statusClass = computed(() => {
  if (sub.value.is_expired) return 'expired'
  if (!sub.value.plan_id) return 'none'
  return 'active'
})

const statusText = computed(() => {
  if (sub.value.is_expired) return '已过期'
  if (!sub.value.plan_id) return '未订阅'
  return '有效'
})

const progressWidth = computed(() => {
  const percent = sub.value.usage_percent || 0
  return Math.min(percent, 100) + '%'
})

const progressClass = computed(() => {
  const percent = sub.value.usage_percent || 0
  if (percent >= 90) return 'danger'
  if (percent >= 70) return 'warning'
  return 'normal'
})

const remainingPercent = computed(() => {
  const percent = sub.value.usage_percent || 0
  return Math.max(0, (100 - percent)).toFixed(1)
})

const fetchData = async (refresh = false) => {
  loading.value = true
  try {
    const res = await getSubscription(refresh)
    sub.value = res.data || {}
  } catch (err) {
    console.error('获取订阅信息失败:', err)
  } finally {
    loading.value = false
  }
}

const refreshData = () => {
  fetchData(true)
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

const formatExpireDate = (timestamp) => {
  if (!timestamp) return '永久'
  const date = new Date(timestamp * 1000)
  return date.toLocaleDateString('zh-CN')
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

.subscription-card {
  background: var(--surface-color);
  border-radius: var(--radius-lg);
  padding: 24px;
  margin-bottom: 24px;
  border: 1px solid var(--border-color);
}

.subscription-card.expired {
  border-color: var(--error-color);
  opacity: 0.8;
}

.plan-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 24px;
}

.plan-name {
  font-size: 24px;
  font-weight: 700;
  color: var(--text-color);
  margin-bottom: 8px;
}

.plan-status {
  display: inline-block;
  padding: 4px 12px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-weight: 600;
}

.plan-status.active {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success-color);
}

.plan-status.expired {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.plan-status.none {
  background: rgba(156, 163, 175, 0.15);
  color: var(--text-secondary);
}

.plan-expire {
  text-align: right;
}

.plan-expire .label {
  display: block;
  font-size: 12px;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.plan-expire .value {
  font-size: 16px;
  font-weight: 600;
}

.plan-expire .permanent {
  color: var(--success-color);
  font-weight: 600;
}

.traffic-section {
  background: var(--bg-color);
  border-radius: var(--radius-md);
  padding: 20px;
  margin-bottom: 20px;
}

.traffic-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 12px;
}

.traffic-label {
  font-size: 14px;
  color: var(--text-secondary);
}

.traffic-value {
  font-size: 14px;
  font-weight: 600;
}

.traffic-bar {
  height: 8px;
  background: var(--border-color);
  border-radius: 4px;
  overflow: hidden;
  margin-bottom: 12px;
}

.traffic-progress {
  height: 100%;
  border-radius: 4px;
  transition: width 0.3s ease;
}

.traffic-progress.normal {
  background: var(--primary-color);
}

.traffic-progress.warning {
  background: #f59e0b;
}

.traffic-progress.danger {
  background: var(--error-color);
}

.traffic-detail {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--text-secondary);
}

.remaining-section {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.remaining-box {
  background: var(--bg-color);
  border-radius: var(--radius-md);
  padding: 20px;
  text-align: center;
}

.remaining-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--primary-color);
  margin-bottom: 4px;
}

.remaining-label {
  font-size: 13px;
  color: var(--text-secondary);
}

.quick-actions {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}

.action-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 24px 16px;
  background: var(--surface-color);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  text-decoration: none;
  transition: all 0.2s ease;
}

.action-card:hover {
  border-color: var(--primary-color);
  transform: translateY(-2px);
}

.action-icon {
  font-size: 28px;
}

.action-text {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-color);
}

.cache-info {
  text-align: center;
  padding: 12px;
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

  .plan-header {
    flex-direction: column;
    gap: 16px;
  }

  .plan-expire {
    text-align: left;
  }

  .remaining-value {
    font-size: 24px;
  }

  .quick-actions {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
