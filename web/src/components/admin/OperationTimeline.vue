<template>
  <section class="operation-timeline" data-testid="operation-timeline" :aria-label="t('control.activity.title')">
    <header class="timeline-header">
      <div>
        <h2>{{ expanded ? t('control.activity.all') : t('control.activity.scoped') }}</h2>
      </div>
      <button
        class="btn timeline-toggle"
        data-testid="show-all-activity"
        type="button"
        :aria-expanded="expanded ? 'true' : 'false'"
        @click="expanded = !expanded"
      >
        <History :size="16" aria-hidden="true" />
        <span>{{ expanded ? t('control.activity.showScoped') : t('control.activity.showAll') }}</span>
      </button>
    </header>

    <div class="timeline-table-wrap">
      <table class="timeline-table">
        <thead>
          <tr>
            <th>{{ t('control.table.operation') }}</th>
            <th>{{ t('control.table.plugin') }}</th>
            <th>{{ t('control.table.revision') }}</th>
            <th>{{ t('control.table.deadline') }}</th>
            <th>{{ t('control.table.state') }}</th>
            <th>{{ t('control.table.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="operation in visibleOperations" :key="operation.id" :data-testid="`operation-row-${operation.id}`">
            <td><code>{{ operation.kind || '-' }}</code><code class="secondary-cell">{{ operation.id }}</code></td>
            <td>{{ operation.plugin_id || '-' }}</td>
            <td>{{ operation.revision ?? '-' }}</td>
            <td>{{ formatDate(operation.deadline_at || operation.created_at) }}</td>
            <td>
              <span :class="['state-badge', stateClass(operation.state)]">{{ operation.state || '-' }}</span>
              <span v-if="operation.last_error" class="row-error">{{ operation.last_error }}</span>
            </td>
            <td>
              <button
                v-if="isCancellable(operation)"
                class="btn btn-danger"
                :data-testid="`cancel-operation-${operation.id}`"
                type="button"
                :disabled="busyOperationID === operation.id"
                @click="cancel(operation)"
              >
                {{ t('control.actions.cancel') }}
              </button>
            </td>
          </tr>
          <tr v-if="visibleOperations.length === 0">
            <td colspan="6" class="empty-row">{{ expanded ? t('control.empty.operations') : t('control.activity.empty') }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { History } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'

const CANCELLABLE_OPERATION_STATES = new Set(['pending', 'dispatching', 'running'])

const props = defineProps({
  operations: { type: Array, default: () => [] },
  scopedOperationIDs: { type: Array, default: () => [] },
  busyOperationID: { type: [String, Number], default: '' },
})
const emit = defineEmits(['cancel'])
const { t, formatDateTime } = useAppI18n()
const expanded = ref(false)
const scopedIDs = computed(() => new Set(props.scopedOperationIDs.map(String)))
const sortedOperations = computed(() => props.operations.slice().sort((left, right) => {
  const leftTime = Date.parse(left?.created_at || '') || 0
  const rightTime = Date.parse(right?.created_at || '') || 0
  return rightTime - leftTime || Number(right?.id || 0) - Number(left?.id || 0)
}))
const visibleOperations = computed(() => expanded.value
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

function formatDate(value) {
  if (!value) return '-'
  return formatDateTime(value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }) || '-'
}

function stateClass(state) {
  if (state === 'healthy' || state === 'enabled' || state === 'succeeded' || state === 'completed') return 'state-active'
  if (state === 'failed' || state === 'superseded' || state === 'disabled' || state === 'cancel_requested' || state === 'cancelled' || state === 'timed_out') return 'state-error'
  return 'state-pending'
}
</script>

<style scoped>
.operation-timeline { display: grid; gap: 12px; min-width: 0; }
.timeline-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.timeline-header h2 { margin: 0; font-size: 16px; }
.timeline-toggle { display: inline-flex; align-items: center; gap: 7px; }
.timeline-table-wrap { overflow-x: auto; border: 1px solid var(--border-color); border-radius: 8px; }
.timeline-table { width: 100%; min-width: 820px; border-collapse: collapse; font-size: 13px; }
.timeline-table th, .timeline-table td { padding: 10px 12px; border-bottom: 1px solid var(--border-color); text-align: left; vertical-align: top; }
.timeline-table th { color: var(--text-secondary); font-size: 11px; font-weight: 700; text-transform: uppercase; }
.timeline-table tbody tr:last-child td { border-bottom: 0; }
.secondary-cell, .row-error { display: block; margin-top: 3px; overflow-wrap: anywhere; }
.secondary-cell { color: var(--text-secondary); font-size: 11px; }
.row-error { color: var(--error-color); font-size: 11px; }
.state-badge { display: inline-flex; border-radius: 6px; padding: 3px 7px; font-size: 12px; }
.state-active { color: var(--success-color); background: rgba(22, 163, 74, .1); }
.state-error { color: var(--error-color); background: rgba(220, 38, 38, .1); }
.state-pending { color: var(--warning-color); background: rgba(217, 119, 6, .1); }
.empty-row { color: var(--text-secondary); text-align: center; }
.btn { min-height: 32px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; padding: 6px 10px; }
.btn-danger { border-color: var(--error-color); color: var(--error-color); }
.btn:disabled { cursor: not-allowed; opacity: .55; }
</style>
