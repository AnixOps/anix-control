<template>
  <div
    class="forward-rules-table"
    :class="{ 'is-dragging': draggingId !== null }"
    @dragover="onDragOver"
    @drop="onDrop"
  >
    <UiDataTable
      :columns="columns"
      :rows="rows"
      :label="label"
      :row-label="forward => forward.name"
      :storage-key="storageKey"
      :loading="loading"
      :error="error"
      :error-title="errorTitle"
      :filtered="filtered"
      :empty-icon="ArrowLeftRight"
      :empty-title="emptyTitle"
      :empty-description="emptyDescription"
      :selectable="selectable"
      :selected="selected"
      :row-actions="rowActions"
      :settings="settings"
      :flat="flat"
      :sticky-header="!flat"
      :card-fields="4"
      :state-heading-tag="stateHeadingTag"
      activatable
      @update:selected="value => emit('update:selected', value)"
      @row-activate="forward => emit('edit', forward)"
      @retry="emit('retry')"
      @clear-filters="emit('clear-filters')"
    >
      <template v-if="$slots.toolbar" #toolbar>
        <slot name="toolbar" />
      </template>
      <template v-if="$slots['toolbar-end']" #toolbar-end>
        <slot name="toolbar-end" />
      </template>
      <template v-if="$slots['empty-actions']" #empty-actions>
        <slot name="empty-actions" />
      </template>
      <template v-if="$slots['bulk-actions']" #bulk-actions="scope">
        <slot name="bulk-actions" v-bind="scope" />
      </template>

      <template #cell-name="{ row, card }">
        <span class="rule-name">
          <span
            v-if="reorderable && !card"
            class="rule-name__handle"
            :class="{ 'is-drag-source': draggingId === row.id, 'is-drag-over': dragOverId === row.id && draggingId !== row.id }"
            draggable="true"
            :title="t('runtime.forward.card.dragHandleTitle')"
            aria-hidden="true"
            data-test="forward-drag-handle"
            @click.stop
            @dragstart="onDragStart($event, row.id)"
            @dragend="onDragEnd"
          >
            <UiIcon :icon="GripVertical" :size="16" />
          </span>
          <span class="rule-name__text">{{ row.name }}</span>
        </span>
      </template>
      <template #cell-ingress="{ row }">
        <button
          v-if="formatInAddress(row.inIp, row.inPort)"
          type="button"
          class="address-button"
          :aria-label="t('runtime.forward.table.copyAddress', { title: t('runtime.forward.card.ingressAddressTitle'), address: formatInAddress(row.inIp, row.inPort) })"
          @click="emit('address', { value: row.inIp, port: row.inPort, title: t('runtime.forward.card.ingressAddressTitle') })"
        >{{ formatInAddress(row.inIp, row.inPort) }}</button>
        <span v-else>—</span>
      </template>
      <template #cell-target="{ row }">
        <button
          v-if="formatRemoteAddress(row.remoteAddr)"
          type="button"
          class="address-button"
          :aria-label="t('runtime.forward.table.copyAddress', { title: t('runtime.forward.card.targetAddressTitle'), address: formatRemoteAddress(row.remoteAddr) })"
          @click="emit('address', { value: row.remoteAddr, title: t('runtime.forward.card.targetAddressTitle') })"
        >{{ formatRemoteAddress(row.remoteAddr) }}</button>
        <span v-else>—</span>
      </template>
      <template #cell-status="{ row }">
        <span class="rule-status">
          <span class="rule-status__line">
            <UiSwitch
              :model-value="row.serviceRunning"
              :disabled="isForwardToggleDisabled(row)"
              :aria-label="t('runtime.forward.table.toggleService', { name: row.name })"
              data-test="forward-service-switch"
              @update:model-value="emit('toggle', row)"
            />
            <UiBadge :tone="presenters.statusMeta(row.status).tone" :label="presenters.statusMeta(row.status).text" />
            <UiBadge v-if="presenters.runtimeMeta(row)" :tone="presenters.runtimeMeta(row).tone" :label="presenters.runtimeMeta(row).text" />
          </span>
          <span v-if="presenters.runtimeSummary(row)" class="rule-status__summary">{{ presenters.runtimeSummary(row) }}</span>
        </span>
      </template>
      <template #cell-traffic="{ row }">
        <span class="rule-traffic">
          <span>{{ t('runtime.forward.labels.inbound') }} {{ formatFlow(row.inFlow || 0) }}</span>
          <span>{{ t('runtime.forward.labels.outbound') }} {{ formatFlow(row.outFlow || 0) }}</span>
        </span>
      </template>
    </UiDataTable>
  </div>
</template>

<script setup>
// The forward rules as a UiDataTable (UI U7). The direct view uses it with
// selection, the bulk bar and drag-to-reorder (handle in the name cell;
// "上移 / 下移" in the row menu do the same from the keyboard and on touch);
// the grouped view uses it flat, once per tunnel group. Every action is
// emitted: the page keeps the state and the flux-panel API calls.
import { computed, ref } from 'vue'
import { ArrowDown, ArrowLeftRight, ArrowUp, GripVertical, Pencil, Stethoscope, Trash2 } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import {
  createForwardPresenters,
  formatFlow,
  formatInAddress,
  formatRemoteAddress,
  isForwardToggleDisabled
} from './forwardModel'

const props = defineProps({
  rows: { type: Array, default: () => [] },
  label: { type: String, required: true },
  storageKey: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  error: { type: [Object, String, null], default: null },
  errorTitle: { type: String, default: '' },
  filtered: { type: Boolean, default: false },
  emptyTitle: { type: String, default: '' },
  emptyDescription: { type: String, default: '' },
  selectable: { type: Boolean, default: false },
  selected: { type: Array, default: undefined },
  // Drag handle and 上移 / 下移 (direct view only, like flux-panel).
  reorderable: { type: Boolean, default: false },
  // The grouped view names the tunnel in the group header.
  hideTunnel: { type: Boolean, default: false },
  settings: { type: Boolean, default: true },
  flat: { type: Boolean, default: false },
  stateHeadingTag: { type: String, default: 'h2' }
})

const emit = defineEmits(['update:selected', 'edit', 'diagnose', 'delete', 'toggle', 'address', 'reorder', 'drag-change', 'retry', 'clear-filters'])

const { t, translateLiteral } = useAppI18n()
const presenters = createForwardPresenters(t, translateLiteral)

const columns = computed(() => allColumns.value.filter(column => !(props.hideTunnel && column.key === 'tunnel')))
const allColumns = computed(() => [
  { key: 'name', label: t('runtime.forward.table.ruleName'), primary: true, hideable: false, width: 200 },
  { key: 'tunnel', label: t('runtime.forward.table.tunnel'), secondary: true, nowrap: true, value: forward => forward.tunnelName || t('runtime.forward.references.tunnel', { id: forward.tunnelId }) },
  { key: 'ingress', label: t('runtime.forward.table.ingress'), value: forward => formatInAddress(forward.inIp, forward.inPort) },
  { key: 'target', label: t('runtime.forward.table.target'), value: forward => formatRemoteAddress(forward.remoteAddr) },
  { key: 'strategy', label: t('runtime.forward.table.policy'), value: forward => presenters.strategyText(forward.strategy), nowrap: true, breakpoint: 'lg', card: false },
  { key: 'status', label: t('runtime.forward.table.status') },
  { key: 'traffic', label: t('runtime.forward.table.traffic'), numeric: true, breakpoint: 'md' }
])

function rowActions(forward) {
  const index = props.rows.findIndex(item => item.id === forward.id)
  return [
    { key: 'edit', label: t('runtime.forward.actions.edit'), icon: Pencil, onSelect: () => emit('edit', forward) },
    { key: 'diagnose', label: t('runtime.forward.actions.diagnose'), icon: Stethoscope, onSelect: () => emit('diagnose', forward) },
    { key: 'move-up', label: t('runtime.forward.actions.moveUp'), icon: ArrowUp, hidden: !props.reorderable, disabled: index <= 0, separatorBefore: true, onSelect: () => emit('reorder', forward.id, props.rows[index - 1]?.id) },
    { key: 'move-down', label: t('runtime.forward.actions.moveDown'), icon: ArrowDown, hidden: !props.reorderable, disabled: index < 0 || index >= props.rows.length - 1, onSelect: () => emit('reorder', forward.id, props.rows[index + 1]?.id) },
    { key: 'delete', label: t('runtime.forward.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => emit('delete', forward) }
  ]
}

// Drag to reorder (HTML5 drag and drop, as before U7): the handle starts
// the drag; the row under the pointer is found from its data-row-key.
const draggingId = ref(null)
const dragOverId = ref(null)

function rowIdFromEvent(event) {
  const row = event.target instanceof Element ? event.target.closest('[data-row-key]') : null
  if (!row) return null
  const id = Number(row.getAttribute('data-row-key'))
  return Number.isFinite(id) ? id : null
}

function onDragStart(event, id) {
  if (!props.reorderable) return
  draggingId.value = id
  dragOverId.value = id
  emit('drag-change', id)
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(id))
  }
}

function onDragOver(event) {
  if (draggingId.value === null) return
  const id = rowIdFromEvent(event)
  if (id === null) return
  event.preventDefault()
  dragOverId.value = id
}

function onDrop(event) {
  if (draggingId.value === null) return
  event.preventDefault()
  const activeId = draggingId.value
  const overId = rowIdFromEvent(event)
  onDragEnd()
  if (overId !== null && overId !== activeId) emit('reorder', activeId, overId)
}

function onDragEnd() {
  if (draggingId.value !== null) emit('drag-change', null)
  draggingId.value = null
  dragOverId.value = null
}
</script>

<style scoped>
.rule-name {
  display: inline-flex;
  gap: var(--space-2);
  align-items: center;
  min-width: 0;
  font-weight: var(--weight-medium);
}

.rule-name__text {
  overflow-wrap: anywhere;
}

.rule-name__handle {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 28px;
  border-radius: var(--radius-xs);
  color: var(--label-2);
  cursor: grab;
}

.rule-name__handle:hover {
  background: var(--fill-1);
  color: var(--label-1);
}

.rule-name__handle.is-drag-source {
  cursor: grabbing;
  opacity: 0.5;
}

/* The row under the pointer while dragging. */
.forward-rules-table :deep(tr:has(.is-drag-over)) {
  box-shadow: inset 0 2px 0 var(--accent);
}

.address-button {
  max-width: 100%;
  min-height: 0;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  text-align: start;
  overflow-wrap: anywhere;
  cursor: pointer;
}

.address-button:hover {
  text-decoration: underline;
}

.address-button:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.rule-status {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.rule-status__line {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.rule-status__summary {
  max-width: 26ch;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
}

.rule-traffic {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  white-space: nowrap;
}
</style>
