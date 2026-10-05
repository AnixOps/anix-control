<template>
  <UiCard :title="t('adminDashboard.alerts.title')" :description="t('adminDashboard.alerts.description')" as="section" data-dashboard-alerts>
    <div class="dashboard-alerts__bar">
      <UiSegmentedControl
        :model-value="view"
        size="sm"
        :aria-label="t('adminDashboard.alerts.view')"
        :options="viewOptions"
        data-alerts-view
        @update:model-value="emit('update:view', $event)"
      />
      <UiBadge v-if="showSummary" :tone="summaryTone" :label="summaryLabel" data-alerts-summary />
    </div>
    <UiSkeleton v-if="showSkeleton" :lines="3" />
    <ul v-else-if="items.length" class="dashboard-alerts" role="list">
      <li v-for="item in items" :key="item.key" class="dashboard-alerts__item" :data-alert="item.key">
        <UiIcon :icon="item.icon" :size="18" class="dashboard-alerts__icon" :class="`is-${item.tone}`" />
        <div class="dashboard-alerts__text">
          <RouterLink v-if="item.to" :to="item.to" class="dashboard-alerts__title">{{ item.title }}</RouterLink>
          <span v-else class="dashboard-alerts__title">{{ item.title }}</span>
          <span v-if="item.hint" class="dashboard-alerts__hint">{{ item.hint }}</span>
        </div>
        <UiButton v-if="item.retry" size="sm" :icon="RotateCw" @click="item.retry()">{{ t('ui.error.retry') }}</UiButton>
      </li>
    </ul>
    <UiEmptyState
      v-else-if="view === 'resolved'"
      compact
      :icon="History"
      :title="t('adminDashboard.alerts.noResolved')"
      :description="t('adminDashboard.alerts.noResolvedDescription')"
    />
    <UiEmptyState
      v-else
      compact
      :icon="CircleCheck"
      :title="t('adminDashboard.alerts.allClear')"
      :description="t('adminDashboard.alerts.allClearDescription')"
    />
  </UiCard>
</template>

<script setup>
// 需要处理 on the dashboard: offline nodes (from the node list's status),
// tickets waiting for a reply, orders awaiting payment (commercial), and
// traffic reports that stopped (the hourly API's latest_log_at). A block
// that failed to load says so with 重试 instead of looking all clear.
// The kernel's own alerts (GET /api/v4/kernel/alerts: certificates that were
// not renewed, CAs near their end, a credential split or identity cutover left
// half done) join the list, critical ones first; 已解决 shows the resolved
// history. They load on their own in Dashboard.vue: a failure shows one item
// with 重试 and never blocks the rest.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { CircleAlert, CircleCheck, CloudOff, History, LifeBuoy, ReceiptText, RotateCw, ServerOff } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { alertIcon, alertRoute, alertText, alertTone } from './kernelAlerts'

const SHOWN_NODES = 4
// No report for two hours while reports exist: the report path stalled.
const STALE_SECONDS = 2 * 3600

const props = defineProps({
  offlineNodes: { type: Array, default: () => [] },
  nodesError: { type: [Object, String, null], default: null },
  nodesLoading: { type: Boolean, default: false },
  openTickets: { type: Array, default: () => [] },
  ticketsError: { type: [Object, String, null], default: null },
  pendingOrders: { type: Number, default: 0 },
  latestReportAt: { type: Number, default: 0 },
  now: { type: Number, default: 0 },
  // The kernel's alerts of the current view (active or resolved).
  kernelAlerts: { type: Array, default: () => [] },
  alertsSummary: { type: Object, default: () => ({ active: 0, critical: 0, warning: 0 }) },
  alertsError: { type: [Object, String, null], default: null },
  alertsLoading: { type: Boolean, default: false },
  view: { type: String, default: 'active' }
})

const emit = defineEmits(['retry-nodes', 'retry-tickets', 'retry-alerts', 'update:view'])
const { t } = useAppI18n()
const format = useFormat()
const resolvedView = computed(() => props.view === 'resolved')
const showSkeleton = useDelayedLoading(() => (resolvedView.value
  ? props.alertsLoading && !props.kernelAlerts.length && !props.alertsError
  : (props.nodesLoading && !props.offlineNodes.length && !props.nodesError) || (props.alertsLoading && !props.kernelAlerts.length && !props.alertsError)))

const viewOptions = computed(() => [
  { value: 'active', label: t('adminDashboard.alerts.viewActive') },
  { value: 'resolved', label: t('adminDashboard.alerts.viewResolved') }
])

// The count badge comes from the server's summary, which counts every active
// alert whatever the list shows.
const showSummary = computed(() => !resolvedView.value && Number(props.alertsSummary?.active) > 0)
const summaryTone = computed(() => (Number(props.alertsSummary?.critical) > 0 ? 'danger' : 'warning'))
const summaryLabel = computed(() => (Number(props.alertsSummary?.critical) > 0
  ? t('adminDashboard.alerts.summaryCritical', { count: format.number(Number(props.alertsSummary.active)), critical: format.number(Number(props.alertsSummary.critical)) })
  : t('adminDashboard.alerts.summary', { count: format.number(Number(props.alertsSummary.active)) })))

function kernelItem(alert) {
  const { title, hint } = alertText(alert, { t, format })
  const resolved = alert.status === 'resolved'
  return {
    key: `kernel-${alert.id ?? alert.key}`,
    icon: alertIcon(alert),
    tone: resolved ? 'neutral' : alertTone(alert),
    title,
    hint,
    to: alertRoute(alert) || undefined
  }
}

const alertsFailedItem = computed(() => (props.alertsError
  ? { key: 'alerts-failed', icon: CircleAlert, tone: 'warning', title: t('adminDashboard.alerts.alertsFailed'), retry: () => emit('retry-alerts') }
  : null))

const items = computed(() => {
  if (resolvedView.value) {
    return alertsFailedItem.value ? [alertsFailedItem.value] : props.kernelAlerts.map(kernelItem)
  }
  const list = []
  if (props.nodesError) {
    list.push({ key: 'nodes-failed', icon: CircleAlert, tone: 'danger', title: t('adminDashboard.alerts.nodesFailed'), retry: () => emit('retry-nodes') })
  }
  for (const node of props.offlineNodes.slice(0, SHOWN_NODES)) {
    list.push({
      key: `node-${node.id}`,
      icon: ServerOff,
      tone: 'danger',
      title: t('adminDashboard.alerts.offlineNode', { name: node.name || `#${node.id}` }),
      hint: node.last_check_at
        ? t('adminDashboard.alerts.lastSeen', { time: format.relativeTime(node.last_check_at) })
        : t('adminDashboard.alerts.neverSeen'),
      to: `/admin/nodes/${node.id}`
    })
  }
  if (props.offlineNodes.length > SHOWN_NODES) {
    list.push({
      key: 'nodes-more',
      icon: ServerOff,
      tone: 'danger',
      title: t('adminDashboard.alerts.moreOffline', { count: props.offlineNodes.length - SHOWN_NODES }),
      to: { path: '/admin/nodes', query: { status: 'offline' } }
    })
  }
  if (props.ticketsError) {
    list.push({ key: 'tickets-failed', icon: CircleAlert, tone: 'danger', title: t('adminDashboard.alerts.ticketsFailed'), retry: () => emit('retry-tickets') })
  } else if (props.openTickets.length) {
    const oldest = props.openTickets.reduce((min, ticket) => Math.min(min, Number(ticket.created_at || Infinity)), Infinity)
    list.push({
      key: 'tickets',
      icon: LifeBuoy,
      tone: 'warning',
      title: t('adminDashboard.alerts.openTickets', { count: props.openTickets.length }),
      hint: Number.isFinite(oldest) ? t('adminDashboard.alerts.openTicketsHint', { time: format.relativeTime(oldest) }) : '',
      to: '/admin/tickets'
    })
  }
  if (props.pendingOrders > 0) {
    list.push({
      key: 'orders',
      icon: ReceiptText,
      tone: 'warning',
      title: t('adminDashboard.alerts.pendingOrders', { count: props.pendingOrders }),
      to: '/admin/orders'
    })
  }
  const now = props.now || Math.floor(Date.now() / 1000)
  if (props.latestReportAt && now - props.latestReportAt > STALE_SECONDS) {
    list.push({
      key: 'traffic-stale',
      icon: CloudOff,
      tone: 'warning',
      title: t('adminDashboard.alerts.trafficStale'),
      hint: t('adminDashboard.alerts.trafficStaleHint', { time: format.dateTime(props.latestReportAt) }),
      to: '/admin/monitor/traffic'
    })
  }
  // Kernel alerts join the list: danger first, the rest in the order given.
  const kernel = props.kernelAlerts.map(kernelItem)
  if (alertsFailedItem.value) kernel.push(alertsFailedItem.value)
  const merged = [...list, ...kernel]
  return [...merged.filter(item => item.tone === 'danger'), ...merged.filter(item => item.tone !== 'danger')]
})
</script>

<style scoped>
.dashboard-alerts {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.dashboard-alerts__item {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  padding: var(--space-3) 0;
  border-bottom: 1px solid var(--separator);
}

.dashboard-alerts__item:first-child {
  padding-top: 0;
}

.dashboard-alerts__item:last-child {
  padding-bottom: 0;
  border-bottom: 0;
}

.dashboard-alerts__icon {
  flex: none;
  margin-top: 2px;
}

.dashboard-alerts__icon.is-danger {
  color: var(--danger);
}

.dashboard-alerts__icon.is-warning {
  color: var(--warning);
}

.dashboard-alerts__icon.is-neutral {
  color: var(--label-2);
}

.dashboard-alerts__bar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-3);
}

.dashboard-alerts__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.dashboard-alerts__title {
  color: var(--label-1);
  font-weight: var(--weight-medium);
  overflow-wrap: anywhere;
  text-decoration: none;
}

a.dashboard-alerts__title:hover {
  color: var(--accent);
  text-decoration: underline;
}

.dashboard-alerts__hint {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .dashboard-alerts__title {
    position: relative;
  }

  .dashboard-alerts__title::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
