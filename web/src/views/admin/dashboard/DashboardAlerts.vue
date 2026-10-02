<template>
  <UiCard :title="t('adminDashboard.alerts.title')" :description="t('adminDashboard.alerts.description')" as="section" data-dashboard-alerts>
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
// Certificates and the credential split are not here: no API reports them.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { CircleAlert, CircleCheck, CloudOff, LifeBuoy, ReceiptText, RotateCw, ServerOff } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'

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
  now: { type: Number, default: 0 }
})

const emit = defineEmits(['retry-nodes', 'retry-tickets'])
const { t } = useAppI18n()
const format = useFormat()
const showSkeleton = useDelayedLoading(() => props.nodesLoading && !props.offlineNodes.length && !props.nodesError)

const items = computed(() => {
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
  return list
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
</style>
