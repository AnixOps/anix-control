<template>
  <div class="dashboard-page">
    <div class="page-header">
      <div>
        <h1>{{ t('user.dashboard.title') }}</h1>
        <p class="text-secondary">{{ t('user.dashboard.subtitle') }}</p>
      </div>
      <button class="btn-secondary" :disabled="loading" @click="refreshData">
        {{ loading ? t('user.dashboard.refreshing') : t('user.dashboard.refresh') }}
      </button>
    </div>

    <div class="subscription-card" :class="{ expired: sub.is_expired }">
      <div class="plan-header">
        <div class="plan-info">
          <div class="plan-name">{{ sub.plan_name || t('common.states.none') }}</div>
          <div class="plan-status" :class="statusClass">{{ statusText }}</div>
        </div>
        <div class="plan-expire">
          <div v-if="sub.expired_at > 0">
            <span class="label">{{ t('common.labels.expiresAt') }}</span>
            <span class="value">{{ formatExpireDate(sub.expired_at) }}</span>
          </div>
          <div v-else class="permanent">{{ t('common.states.permanent') }}</div>
        </div>
      </div>

      <div class="traffic-section">
        <div class="traffic-header">
          <span class="traffic-label">{{ t('user.dashboard.trafficUsage') }}</span>
          <span class="traffic-value">
            {{ formatBytes(sub.used_traffic) }} / {{ formatBytes(sub.transfer_enable) }}
          </span>
        </div>
        <div class="traffic-bar">
          <div class="traffic-progress" :style="{ width: progressWidth }" :class="progressClass"></div>
        </div>
        <div class="traffic-detail">
          <span>{{ t('user.dashboard.upload') }} {{ formatBytes(sub.upload_traffic) }}</span>
          <span>{{ t('user.dashboard.download') }} {{ formatBytes(sub.download_traffic) }}</span>
        </div>
      </div>

      <div v-if="!sub.is_expired && sub.days_remaining >= 0" class="remaining-section">
        <div class="remaining-box">
          <div class="remaining-value">{{ sub.days_remaining }}</div>
          <div class="remaining-label">{{ t('user.dashboard.remainingDays') }}</div>
        </div>
        <div class="remaining-box">
          <div class="remaining-value">{{ remainingPercent }}%</div>
          <div class="remaining-label">{{ t('user.dashboard.trafficRemaining') }}</div>
        </div>
      </div>
    </div>

    <div class="quick-actions">
      <router-link to="/user/subscribe" class="action-card">
        <span class="action-icon">S</span>
        <span class="action-text">{{ t('user.dashboard.quickActions.subscribe') }}</span>
      </router-link>
      <router-link to="/user/orders" class="action-card">
        <span class="action-icon">O</span>
        <span class="action-text">{{ t('user.dashboard.quickActions.orders') }}</span>
      </router-link>
      <router-link to="/user/tickets" class="action-card">
        <span class="action-icon">T</span>
        <span class="action-text">{{ t('user.dashboard.quickActions.tickets') }}</span>
      </router-link>
      <router-link to="/user/knowledge" class="action-card">
        <span class="action-icon">K</span>
        <span class="action-text">{{ t('user.dashboard.quickActions.knowledge') }}</span>
      </router-link>
    </div>

    <div v-if="sub.cached_at" class="cache-info">
      {{ t('user.dashboard.cacheUpdatedAt', { value: formatDateTime(sub.cached_at) }) }}
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getSubscription } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDate, formatDateTime } = useAppI18n()
const sub = ref({})
const loading = ref(false)

const statusClass = computed(() => {
  if (sub.value.is_expired) return 'expired'
  if (!sub.value.plan_id) return 'none'
  return 'active'
})

const statusText = computed(() => {
  if (sub.value.is_expired) return t('common.states.expired')
  if (!sub.value.plan_id) return t('common.states.none')
  return t('common.states.active')
})

const progressWidth = computed(() => `${Math.min(sub.value.usage_percent || 0, 100)}%`)

const progressClass = computed(() => {
  const percent = sub.value.usage_percent || 0
  if (percent >= 90) return 'danger'
  if (percent >= 70) return 'warning'
  return 'normal'
})

const remainingPercent = computed(() => Math.max(0, 100 - (sub.value.usage_percent || 0)).toFixed(1))

async function fetchData(refresh = false) {
  loading.value = true
  try {
    const res = await getSubscription(refresh)
    sub.value = res.data || {}
  } catch (err) {
    console.error('Failed to fetch subscription info:', err)
  } finally {
    loading.value = false
  }
}

function refreshData() {
  fetchData(true)
}

function formatBytes(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(2)} ${units[index]}`
}

function formatExpireDate(timestamp) {
  if (!timestamp) {
    return t('common.states.permanent')
  }
  return formatDate(timestamp, { year: 'numeric', month: 'short', day: 'numeric' })
}

onMounted(() => {
  fetchData()
})
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
  font-size: 18px;
  font-weight: 700;
  color: var(--primary-color);
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
