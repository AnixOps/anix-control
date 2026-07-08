<template>
  <div class="page-shell">
    <div class="page-toolbar">
      <div>
        <h1>{{ t('adminDashboard.title') }}</h1>
        <p>{{ t('adminDashboard.subtitle') }}</p>
      </div>
      <button class="btn btn-primary" @click="refreshData" :disabled="loading">
        {{ loading ? t('adminDashboard.actions.refreshing') : t('adminDashboard.actions.refresh') }}
      </button>
    </div>

    <section class="metrics-grid">
      <article v-for="metric in metrics" :key="metric.label" class="metric-card section-panel">
        <div class="metric-top">
          <span class="metric-tag" :class="metric.tone">{{ metric.code }}</span>
          <span class="metric-label">{{ metric.label }}</span>
        </div>
        <div class="metric-value">{{ metric.value }}</div>
        <div class="metric-detail">{{ metric.detail }}</div>
      </article>
    </section>

    <section class="detail-grid">
      <article class="section-panel detail-card">
        <div class="detail-header">
          <h2>{{ t('adminDashboard.orders.title') }}</h2>
        </div>
        <div class="detail-list">
          <div class="detail-row">
            <span>{{ t('adminDashboard.orders.total') }}</span>
            <strong>{{ formatNumber(stats.total_orders) }}</strong>
          </div>
          <div class="detail-row">
            <span>{{ t('adminDashboard.orders.pending') }}</span>
            <strong class="text-warning">{{ stats.pending_orders || 0 }}</strong>
          </div>
          <div class="detail-row">
            <span>{{ t('adminDashboard.orders.completed') }}</span>
            <strong class="text-success">{{ stats.paid_orders || 0 }}</strong>
          </div>
        </div>
      </article>

      <article class="section-panel detail-card">
        <div class="detail-header">
          <h2>{{ t('adminDashboard.traffic.title') }}</h2>
        </div>
        <div class="detail-list">
          <div class="detail-row">
            <span>{{ t('adminDashboard.traffic.totalUsed') }}</span>
            <strong>{{ formatBytes(stats.total_traffic_used) }}</strong>
          </div>
          <div class="detail-row">
            <span>{{ t('adminDashboard.traffic.today') }}</span>
            <strong>{{ formatBytes(stats.today_traffic) }}</strong>
          </div>
          <div v-if="stats.cached_at" class="detail-row muted-row">
            <span>{{ t('adminDashboard.cache.cachedAt', { time: formatDateTime(stats.cached_at) }) }}</span>
          </div>
        </div>
      </article>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getDashboard } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'

const { t, formatDateTime } = useAppI18n()
const stats = ref({})
const loading = ref(false)

const metrics = computed(() => ([
  {
    code: 'USR',
    tone: 'primary',
    label: t('adminDashboard.stats.totalUsers'),
    value: formatNumber(stats.value.total_users),
    detail: `${t('adminDashboard.stats.todayNewUsers')} +${stats.value.today_new_users || 0}`
  },
  {
    code: 'ACT',
    tone: 'success',
    label: t('adminDashboard.stats.activeUsers'),
    value: formatNumber(stats.value.active_users),
    detail: t('adminDashboard.stats.expiredBanned', {
      expired: stats.value.expired_users || 0,
      banned: stats.value.banned_users || 0
    })
  },
  {
    code: 'NOD',
    tone: 'info',
    label: t('adminDashboard.stats.activeNodes'),
    value: `${stats.value.active_nodes || 0} / ${stats.value.total_nodes || 0}`,
    detail: `${t('adminDashboard.stats.onlineUsers')} ${formatNumber(stats.value.online_users)}`
  },
  {
    code: 'REV',
    tone: 'warning',
    label: t('adminDashboard.stats.monthlyIncome'),
    value: `¥${formatMoney(stats.value.monthly_income)}`,
    detail: t('adminDashboard.stats.incomeSummary', {
      today: formatMoney(stats.value.today_income),
      total: formatMoney(stats.value.total_revenue)
    })
  }
]))

const fetchData = async (refresh = false) => {
  loading.value = true
  try {
    const res = await getDashboard(refresh)
    stats.value = readDashboardStats(res)
  } catch (err) {
    console.error(t('adminDashboard.messages.fetchFailed'), err)
  } finally {
    loading.value = false
  }
}

const refreshData = () => {
  fetchData(true)
}

function readDashboardStats(res) {
  if (!res || typeof res !== 'object') {
    return {}
  }
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  return payload && typeof payload === 'object' ? payload : {}
}

function formatNumber(num) {
  if (!num) return '0'
  return num.toLocaleString()
}

function formatMoney(cents) {
  if (!cents) return '0.00'
  return (cents / 100).toFixed(2)
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

onMounted(() => fetchData())
</script>

<style scoped>
.metrics-grid,
.detail-grid {
  display: grid;
  gap: 20px;
}

.metrics-grid {
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
}

.detail-grid {
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
}

.metric-card,
.detail-card {
  padding: 20px;
}

.metric-card {
  min-height: 168px;
}

.metric-top,
.detail-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.metric-top {
  gap: 12px;
}

.metric-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 48px;
  height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 800;
}

.metric-tag.primary {
  color: var(--primary-color);
  background: rgba(0, 100, 250, 0.08);
}

.metric-tag.success {
  color: var(--success-color);
  background: rgba(22, 163, 74, 0.08);
}

.metric-tag.info {
  color: #2563eb;
  background: rgba(37, 99, 235, 0.08);
}

.metric-tag.warning {
  color: var(--warning-color);
  background: rgba(217, 119, 6, 0.08);
}

.metric-label,
.detail-row span {
  color: var(--text-secondary);
}

.metric-value {
  margin-top: 18px;
  font-size: 34px;
  line-height: 1.1;
  font-weight: 700;
}

.metric-detail {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--border-color);
  color: var(--text-secondary);
  font-size: 13px;
}

.detail-header {
  margin-bottom: 16px;
}

.detail-header h2 {
  font-size: 18px;
  line-height: 1.2;
}

.detail-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.detail-row {
  min-height: 48px;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--surface-muted);
}

.muted-row {
  justify-content: flex-start;
  font-size: 13px;
}

@media (max-width: 768px) {
  .metric-value {
    font-size: 28px;
  }
}
</style>
