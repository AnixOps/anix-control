<template>
  <div
    class="forward-rules-table"
    :class="{ 'is-dragging': draggingId !== null }"
    @dragover="onDragOver"
    @drop="onDrop"
  >
    <TooltipProvider>
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
          <span class="rule-name__text" :class="{ 'is-truncated': !card }" :title="card ? undefined : row.name">{{ row.name }}</span>
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
      <template #cell-status="{ row, card }">
        <span class="rule-status" :class="{ 'is-card': card }">
          <span class="rule-status__line">
            <UiSwitch
              :model-value="row.serviceRunning"
              :disabled="isForwardToggleDisabled(row)"
              :aria-label="t('runtime.forward.table.toggleService', { name: row.name })"
              data-test="forward-service-switch"
              @update:model-value="emit('toggle', row)"
            />
            <TooltipRoot v-if="!card && statusDetail(row)" :delay-duration="200">
              <TooltipTrigger as-child>
                <button type="button" class="rule-status__badge-button" data-test="forward-status-detail">
                  <UiBadge :tone="combinedStatus(row).tone" :label="combinedStatus(row).text" />
                  <span class="visually-hidden">{{ statusDetail(row) }}</span>
                </button>
              </TooltipTrigger>
              <TooltipPortal>
                <TooltipContent class="rule-status__tooltip" side="top" :side-offset="6" :collision-padding="12">
                  {{ statusDetail(row) }}
                </TooltipContent>
              </TooltipPortal>
            </TooltipRoot>
            <span v-else class="rule-status__badge">
              <UiBadge :tone="combinedStatus(row).tone" :label="combinedStatus(row).text" />
            </span>
          </span>
          <span v-if="card && statusDetail(row)" class="rule-status__detail" :title="statusDetail(row)">{{ statusDetail(row) }}</span>
        </span>
      </template>
      <template #cell-traffic="{ row }">
        <span class="rule-traffic">
          <span>{{ t('runtime.forward.labels.inbound') }} {{ formatFlow(row.inFlow || 0) }}</span>
          <span>{{ t('runtime.forward.labels.outbound') }} {{ formatFlow(row.outFlow || 0) }}</span>
        </span>
      </template>
    </UiDataTable>
    </TooltipProvider>
  </div>
</template>

<script setup>
// The forward rules as a UiDataTable (UI U7). The direct view uses it with
// selection, the bulk bar and drag-to-reorder (handle in the name cell;
// "上移 / 下移" in the row menu do the same from the keyboard and on touch);
// the grouped view uses it flat, once per tunnel group. Every action is
// emitted: the page keeps the state and the flux-panel API calls.
import { computed, ref } from 'vue'
import { TooltipContent, TooltipPortal, TooltipProvider, TooltipRoot, TooltipTrigger } from 'reka-ui'
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

// One badge per row: the worst of the forward status and the runtime state
// (异常 / 同步失败 > 执行中 / 待下发 > 暂停 > 已应用 / 已同步 > 正常). The
// other state and the runtime message are the detail: a tooltip on the
// badge in the table (also read by screen readers), a second line on cards.
function combinedStatus(forward) {
  const status = presenters.statusMeta(forward.status)
  const runtime = presenters.runtimeMeta(forward)
  if (status.tone === 'danger') return status
  if (runtime && runtime.tone !== 'success') return runtime
  if (Number(forward.status) !== 1) return status
  return runtime || status
}

function statusDetail(forward) {
  const shown = combinedStatus(forward).text
  const states = [presenters.statusMeta(forward.status).text, presenters.runtimeMeta(forward)?.text]
    .filter(text => text && text !== shown)
  return [...states, presenters.runtimeSummary(forward)].filter(Boolean).join(' · ')
}

const columns = computed(() => allColumns.value.filter(column => !(props.hideTunnel && column.key === 'tunnel')))
const allColumns = computed(() => [
  { key: 'name', label: t('runtime.forward.table.ruleName'), primary: true, hideable: false },
  { key: 'tunnel', label: t('runtime.forward.table.tunnel'), secondary: true, nowrap: true, value: forward => forward.tunnelName || t('runtime.forward.references.tunnel', { id: forward.tunnelId }) },
  { key: 'ingress', label: t('runtime.forward.table.ingress'), value: forward => formatInAddress(forward.inIp, forward.inPort) },
  { key: 'target', label: t('runtime.forward.table.target'), value: forward => formatRemoteAddress(forward.remoteAddr) },
  { key: 'strategy', label: t('runtime.forward.table.policy'), value: forward => presenters.strategyText(forward.strategy), nowrap: true, breakpoint: 'lg', card: false },
  { key: 'status', label: t('runtime.forward.table.status'), nowrap: true },
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

/* One line in the table (the full name is the title and the row's label). */
.rule-name__text.is-truncated {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

/* Undo the global button rule (inline-flex, centred, pill, fill). */
.address-button {
  display: inline;
  max-width: 100%;
  min-height: 0;
  border-radius: 0;
  font-weight: var(--weight-regular);
  line-height: inherit;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  text-align: start;
  /* A whole address is the column's minimum width in the table; on phone
     cards (two addresses side by side) a long one still breaks. */
  overflow-wrap: break-word;
  white-space: normal;
  cursor: pointer;
}

.address-button:hover,
.address-button:active {
  background: none;
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
  gap: var(--space-2);
  align-items: center;
  white-space: nowrap;
}

/* An opaque base under the translucent badge keeps its contrast on hovered
   and selected rows. */
.rule-status__badge,
.rule-status__badge-button {
  display: inline-flex;
  border-radius: var(--radius-pill);
  background: var(--bg-elevated);
}

.rule-status__badge-button,
.rule-status__badge-button:hover,
.rule-status__badge-button:active {
  background: var(--bg-elevated);
}

.rule-status__badge-button {
  min-height: 0;
  padding: 0;
  border: 0;
  cursor: help;
}

.rule-status__badge-button:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.rule-status__detail {
  display: block;
  max-width: 100%;
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-caption-size);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rule-traffic {
  display: flex;
  flex-direction: column;
  font-size: var(--type-callout-size);
  line-height: 1.35;
  white-space: nowrap;
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .address-button {
    position: relative;
  }

  .address-button::after {
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

<style>
/* The tooltip is portalled to <body>, outside the scoped styles. */
.rule-status__tooltip {
  z-index: var(--z-tooltip);
  max-width: 320px;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--label-1);
  box-shadow: var(--shadow-2);
  color: var(--bg);
  font-size: var(--type-caption-size);
  line-height: 1.45;
  overflow-wrap: anywhere;
}

</style>
