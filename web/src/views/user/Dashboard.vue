<template>
  <div class="page-shell">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('user.dashboard.title') }}</h1>
        <p>{{ t('user.dashboard.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" :disabled="loading" @click="refreshData">
        {{ loading ? t('user.dashboard.refreshing') : t('user.dashboard.refresh') }}
      </button>
    </div>

    <section class="section-panel subscription-card" :class="{ expired: sub.is_expired }">
      <div class="subscription-header">
        <div>
          <div class="plan-name">{{ sub.plan_name || t('common.states.none') }}</div>
          <div class="plan-status" :class="statusClass">{{ statusText }}</div>
        </div>
        <div class="plan-expire">
          <span class="expire-label">{{ t('common.labels.expiresAt') }}</span>
          <strong>{{ sub.expired_at > 0 ? formatExpireDate(sub.expired_at) : t('common.states.permanent') }}</strong>
        </div>
      </div>

      <div class="usage-panel">
        <div class="usage-head">
          <span>{{ t('user.dashboard.trafficUsage') }}</span>
          <strong>{{ formatBytes(sub.used_traffic) }} / {{ formatBytes(sub.transfer_enable) }}</strong>
        </div>
        <div class="usage-bar">
          <div class="usage-progress" :style="{ width: progressWidth }" :class="progressClass"></div>
        </div>
        <div class="usage-foot">
          <span>{{ t('user.dashboard.upload') }} {{ formatBytes(sub.upload_traffic) }}</span>
          <span>{{ t('user.dashboard.download') }} {{ formatBytes(sub.download_traffic) }}</span>
        </div>
      </div>

      <div v-if="!sub.is_expired && sub.days_remaining >= 0" class="summary-grid">
        <div class="summary-box">
          <div class="summary-value">{{ sub.days_remaining }}</div>
          <div class="summary-label">{{ t('user.dashboard.remainingDays') }}</div>
        </div>
        <div class="summary-box">
          <div class="summary-value">{{ remainingPercent }}%</div>
          <div class="summary-label">{{ t('user.dashboard.trafficRemaining') }}</div>
        </div>
      </div>
    </section>

    <section class="quick-actions-grid">
      <router-link v-for="item in quickActions" :key="item.to" :to="item.to" class="section-panel quick-action">
        <span class="quick-action-icon">{{ item.icon }}</span>
        <span class="quick-action-label">{{ item.label }}</span>
      </router-link>
    </section>

    <div v-if="sub.cached_at" class="cache-line">
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

const quickActions = computed(() => ([
  { to: '/user/subscribe', icon: 'SB', label: t('user.dashboard.quickActions.subscribe') },
  { to: '/user/orders', icon: 'OR', label: t('user.dashboard.quickActions.orders') },
  { to: '/user/tickets', icon: 'TK', label: t('user.dashboard.quickActions.tickets') },
  { to: '/user/knowledge', icon: 'KB', label: t('user.dashboard.quickActions.knowledge') }
]))

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
.subscription-card {
  padding: 24px;
}

.subscription-card.expired {
  border-color: rgba(220, 38, 38, 0.28);
}

.subscription-header,
.usage-head,
.usage-foot {
  display: flex;
  justify-content: space-between;
  gap: 16px;
}

.subscription-header {
  align-items: flex-start;
}

.plan-name {
  font-size: 28px;
  line-height: 1.1;
  font-weight: 700;
}

.plan-status {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  margin-top: 10px;
  padding: 0 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.plan-status.active {
  color: var(--success-color);
  background: rgba(22, 163, 74, 0.08);
}

.plan-status.expired {
  color: var(--error-color);
  background: rgba(220, 38, 38, 0.08);
}

.plan-status.none {
  color: var(--text-secondary);
  background: rgba(148, 163, 184, 0.12);
}

.plan-expire {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}

.expire-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.usage-panel {
  margin-top: 24px;
  padding: 18px;
  border-radius: 8px;
  background: var(--surface-muted);
}

.usage-head strong {
  font-size: 15px;
}

.usage-head span,
.usage-foot {
  color: var(--text-secondary);
  font-size: 13px;
}

.usage-bar {
  height: 10px;
  margin: 14px 0 12px;
  background: #dbe5f4;
  border-radius: 999px;
  overflow: hidden;
}

.usage-progress {
  height: 100%;
  border-radius: 999px;
  transition: width 0.25s ease;
}

.usage-progress.normal {
  background: var(--primary-color);
}

.usage-progress.warning {
  background: var(--warning-color);
}

.usage-progress.danger {
  background: var(--error-color);
}

.summary-grid,
.quick-actions-grid {
  display: grid;
  gap: 16px;
}

.summary-grid {
  margin-top: 20px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.summary-box {
  padding: 18px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: #fff;
}

.summary-value {
  font-size: 28px;
  line-height: 1.1;
  font-weight: 700;
  color: var(--primary-color);
}

.summary-label {
  margin-top: 6px;
  color: var(--text-secondary);
  font-size: 13px;
}

.quick-actions-grid {
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
}

.quick-action {
  min-height: 120px;
  padding: 20px;
  text-decoration: none;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  color: inherit;
  transition: all 0.2s ease;
}

.quick-action:hover {
  border-color: var(--border-strong);
  box-shadow: var(--shadow-md);
}

.quick-action-icon {
  width: 38px;
  height: 38px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: var(--primary-soft);
  color: var(--primary-color);
  font-size: 12px;
  font-weight: 800;
}

.quick-action-label {
  font-size: 15px;
  font-weight: 700;
}

.cache-line {
  font-size: 13px;
  color: var(--text-secondary);
}

@media (max-width: 768px) {
  .subscription-header,
  .usage-head,
  .usage-foot {
    flex-direction: column;
  }

  .plan-expire {
    align-items: flex-start;
  }

  .summary-grid {
    grid-template-columns: 1fr;
  }
}
</style>
