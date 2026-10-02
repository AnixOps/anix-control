<template>
  <UiCard :title="t('adminDashboard.activity.title')" :description="t('adminDashboard.activity.description')" as="section" data-dashboard-activity>
    <template #actions>
      <RouterLink class="dashboard-activity__link" to="/admin/system/audit">{{ t('adminDashboard.activity.open') }}</RouterLink>
    </template>
    <UiErrorState v-if="error" compact heading-tag="h3" :title="t('adminDashboard.activity.loadFailed')" :error="error" @retry="emit('retry')" />
    <UiSkeleton v-else-if="showSkeleton" :lines="4" />
    <ol v-else-if="entries.length" class="dashboard-activity" role="list">
      <li v-for="entry in entries" :key="entry.id" class="dashboard-activity__item">
        <span class="dashboard-activity__dot" :class="{ 'is-failed': failed(entry) }" aria-hidden="true" />
        <div class="dashboard-activity__text">
          <span class="dashboard-activity__title">
            <strong>{{ entry.username || t('adminDashboard.activity.system') }}</strong>
            {{ actionLabel(entry) }}<template v-if="entry.module"> · {{ moduleLabel(entry) }}</template>
            <UiBadge v-if="failed(entry)" tone="danger" :label="t('adminSettings.audit.results.failed')" />
          </span>
          <span v-if="entry.content" class="dashboard-activity__content">{{ entry.content }}</span>
        </div>
        <time class="dashboard-activity__time" :datetime="isoTime(entry.created_at)" :title="format.dateTime(entry.created_at)">{{ format.relativeTime(entry.created_at) }}</time>
      </li>
    </ol>
    <UiEmptyState v-else-if="!loading" compact :icon="History" :title="t('adminDashboard.activity.empty')" :description="t('adminDashboard.activity.emptyDescription')" />
  </UiCard>
</template>

<script setup>
// 最近操作 on the dashboard: the newest audit log entries (who, what, when),
// with labels shared with 系统设置 › 审计日志, which the header link opens.
import { RouterLink } from 'vue-router'
import { History } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiCard from '@/ui/UiCard.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { toDate, useFormat } from '@/ui/composables/useFormat'

const ACTIONS = ['create', 'update', 'delete', 'login', 'logout', 'enable', 'disable', 'reset', 'restore']
const MODULES = ['system', 'forward', 'users', 'user', 'auth', 'nodes', 'node', 'subscription', 'plugins', 'backup', 'orders', 'tickets']

const props = defineProps({
  entries: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  error: { type: [Object, String, null], default: null }
})

const emit = defineEmits(['retry'])
const { t } = useAppI18n()
const format = useFormat()
const showSkeleton = useDelayedLoading(() => props.loading && !props.entries.length)

function labelOf(group, list, value) {
  const raw = String(value || '')
  const key = raw.toLowerCase()
  return list.includes(key) ? t(`adminSettings.audit.${group}.${key}`) : raw
}

const actionLabel = entry => labelOf('actions', ACTIONS, entry.action)
const moduleLabel = entry => labelOf('modules', MODULES, entry.module)
const failed = entry => ['failed', 'failure', 'error'].includes(String(entry.status || '').toLowerCase())

function isoTime(value) {
  const date = toDate(value)
  return date ? date.toISOString() : undefined
}
</script>

<style scoped>
.dashboard-activity {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.dashboard-activity__item {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: var(--space-3);
  align-items: baseline;
  padding: var(--space-3) 0;
  border-bottom: 1px solid var(--separator);
}

.dashboard-activity__item:first-child {
  padding-top: 0;
}

.dashboard-activity__item:last-child {
  padding-bottom: 0;
  border-bottom: 0;
}

.dashboard-activity__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
  transform: translateY(-1px);
}

.dashboard-activity__dot.is-failed {
  background: var(--danger);
}

.dashboard-activity__text {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.dashboard-activity__title {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  color: var(--label-1);
}

.dashboard-activity__title strong {
  font-weight: var(--weight-semibold);
}

.dashboard-activity__content {
  color: var(--label-2);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.dashboard-activity__time {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  white-space: nowrap;
}

.dashboard-activity__link {
  color: var(--accent);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  text-decoration: none;
}

.dashboard-activity__link:hover {
  text-decoration: underline;
}
</style>
