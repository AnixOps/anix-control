<template>
  <section class="operation-timeline" data-testid="operation-timeline" :aria-labelledby="headingId">
    <header class="timeline-header">
      <h2 :id="headingId" class="timeline-title">{{ heading || (expanded ? t('control.activity.all') : t('control.activity.scoped')) }}</h2>
      <UiButton
        v-if="showToggle"
        size="sm"
        :icon="History"
        data-testid="show-all-activity"
        :aria-expanded="expanded ? 'true' : 'false'"
        @click="expanded = !expanded"
      >
        {{ expanded ? t('control.activity.showScoped') : t('control.activity.showAll') }}
      </UiButton>
    </header>

    <ol v-if="visibleOperations.length" class="timeline-list">
      <li
        v-for="operation in visibleOperations"
        :key="operation.id"
        class="timeline-item"
        :class="`is-${stateTone(operation.state)}`"
        :data-testid="`operation-row-${operation.id}`"
      >
        <span class="timeline-marker" aria-hidden="true" />
        <div class="timeline-body">
          <div class="timeline-line">
            <code class="timeline-kind">{{ operation.kind || '-' }}</code>
            <UiBadge :tone="stateTone(operation.state)" :label="operation.state || '-'" />
            <time class="timeline-time" :datetime="operation.deadline_at || operation.created_at || undefined">
              {{ formatDate(operation.deadline_at || operation.created_at) }}
            </time>
          </div>
          <dl class="timeline-fields">
            <div>
              <dt>{{ t('control.table.plugin') }}</dt>
              <dd>{{ operation.plugin_id || '-' }}</dd>
            </div>
            <div>
              <dt>{{ t('control.table.target') }}</dt>
              <dd>{{ operationTarget(operation) }}</dd>
            </div>
            <div>
              <dt>{{ t('control.table.version') }}</dt>
              <dd><code>{{ operation.target_version || '-' }}</code></dd>
            </div>
            <div>
              <dt>{{ t('control.table.revision') }}</dt>
              <dd>{{ operation.revision ?? '-' }}</dd>
            </div>
          </dl>
          <p class="timeline-meta">
            <code>{{ operation.id }}</code>
            <code :data-testid="`operation-chain-${operation.id}`">{{ t('control.table.chain') }}: {{ operation.operation_chain || operation.id || '-' }}</code>
          </p>
          <p v-if="operation.last_error" class="timeline-error">{{ operation.last_error }}</p>
        </div>
        <UiButton
          v-if="isCancellable(operation)"
          class="timeline-cancel"
          size="sm"
          variant="danger-soft"
          :data-testid="`cancel-operation-${operation.id}`"
          :disabled="busyOperationID === operation.id"
          @click="cancel(operation)"
        >
          {{ t('control.actions.cancel') }}
        </UiButton>
      </li>
    </ol>
    <UiEmptyState
      v-else
      compact
      :icon="History"
      heading-tag="h3"
      :title="emptyLabel || (expanded ? t('control.empty.operations') : t('control.activity.empty'))"
    />
  </section>
</template>

<script setup>
// The kernel operation ledger as a vertical timeline (UI U8), newest first:
// a marker and a state badge (the state word, not colour alone), the kind,
// plugin, target, version, revision, time, the operation chain, the last
// error, and 取消操作 for pending / dispatching / running operations.
// Shared by 部署编排 (scoped to the open deployment or node, 显示全部活动
// shows the rest) and 插件中心 (plugin operations, no toggle, at most 8).
// An ordered list, not a UiDataTable: it is listed in
// scripts/data-table-pages.mjs so a bare <table> does not come back.
import { computed, ref, watch } from 'vue'
import { History } from '@lucide/vue'
import { useId } from 'reka-ui'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'

const CANCELLABLE_OPERATION_STATES = new Set(['pending', 'dispatching', 'running'])

const props = defineProps({
  operations: { type: Array, default: () => [] },
  scopedOperationIDs: { type: Array, default: () => [] },
  busyOperationID: { type: [String, Number], default: '' },
  heading: { type: String, default: '' },
  emptyLabel: { type: String, default: '' },
  showToggle: { type: Boolean, default: true },
  limit: { type: Number, default: 0 },
})
const emit = defineEmits(['cancel'])
const { t, formatDateTime } = useAppI18n()
const expanded = ref(false)
const headingId = useId(undefined, 'operation-timeline')
const scopedIDs = computed(() => new Set(props.scopedOperationIDs.map(String)))
const sortedOperations = computed(() => {
  const sorted = props.operations.slice().sort((left, right) => {
    const leftTime = Date.parse(left?.created_at || '') || 0
    const rightTime = Date.parse(right?.created_at || '') || 0
    return rightTime - leftTime || Number(right?.id || 0) - Number(left?.id || 0)
  })
  return props.limit > 0 ? sorted.slice(0, props.limit) : sorted
})
const visibleOperations = computed(() => (expanded.value && props.showToggle)
  ? sortedOperations.value
  : sortedOperations.value.filter(operation => scopedIDs.value.has(String(operation.id))))

watch(() => props.scopedOperationIDs, () => {
  expanded.value = false
})

function isCancellable(operation) {
  return CANCELLABLE_OPERATION_STATES.has(operation?.state)
}

function cancel(operation) {
  if (!isCancellable(operation) || props.busyOperationID === operation.id) return
  emit('cancel', operation.id)
}

function operationTarget(operation) {
  if (operation?.target) return operation.target
  return operation?.node_id === undefined || operation?.node_id === null ? 'control' : 'agent'
}

function formatDate(value) {
  if (!value) return '-'
  return formatDateTime(value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }) || '-'
}

function stateTone(state) {
  if (state === 'healthy' || state === 'enabled' || state === 'succeeded' || state === 'completed') return 'success'
  if (state === 'failed' || state === 'superseded' || state === 'disabled' || state === 'cancel_requested' || state === 'cancelled' || state === 'timed_out') return 'danger'
  return 'warning'
}
</script>

<style scoped>
.operation-timeline {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
  padding: var(--space-5);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.timeline-header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
}

.timeline-title {
  margin: 0;
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-title-3-line);
}

.timeline-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.timeline-item {
  position: relative;
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr) auto;
  gap: var(--space-3);
  align-items: start;
  padding-bottom: var(--space-5);
}

.timeline-item:last-child {
  padding-bottom: 0;
}

/* The rail between markers. */
.timeline-item:not(:last-child)::before {
  position: absolute;
  top: 18px;
  bottom: 0;
  left: 7px;
  width: 2px;
  background: var(--separator);
  content: '';
}

.timeline-marker {
  width: 12px;
  height: 12px;
  margin: 4px 2px 0;
  border: 2px solid var(--bg-elevated);
  border-radius: 50%;
  background: var(--warning);
  box-shadow: 0 0 0 1px var(--warning);
}

.is-success .timeline-marker {
  background: var(--success);
  box-shadow: 0 0 0 1px var(--success);
}

.is-danger .timeline-marker {
  background: var(--danger);
  box-shadow: 0 0 0 1px var(--danger);
}

.timeline-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.timeline-line {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.timeline-kind {
  color: var(--label-1);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.timeline-time {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  font-variant-numeric: tabular-nums;
}

.timeline-fields {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-5);
  margin: 0;
  font-size: var(--type-caption-size);
}

.timeline-fields div {
  display: flex;
  gap: var(--space-1);
  min-width: 0;
}

.timeline-fields dt {
  color: var(--label-2);
}

.timeline-fields dd {
  margin: 0;
  color: var(--label-1);
  overflow-wrap: anywhere;
}

.timeline-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.timeline-meta code,
.timeline-fields code {
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
}

.timeline-error {
  margin: 0;
  color: var(--danger);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

@media (max-width: 639.98px) {
  .operation-timeline {
    padding: var(--space-4);
  }

  .timeline-item {
    grid-template-columns: 16px minmax(0, 1fr);
  }

  .timeline-cancel {
    grid-column: 2;
    justify-self: start;
  }
}
</style>
