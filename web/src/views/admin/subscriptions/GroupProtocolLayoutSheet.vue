<template>
  <UiSheet
    :open="open"
    size="md"
    :title="t('adminSubscriptionGroups.layout.title')"
    :description="t('adminSubscriptionGroups.layout.description', { group: group.name })"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <form class="layout-sheet" novalidate data-test="protocol-layout-form" @submit.prevent="save">
      <p v-if="!rows.length" class="layout-sheet__empty">{{ t('adminSubscriptionGroups.protocols.empty') }}</p>
      <ol v-else class="layout-sheet__list" :aria-label="t('adminSubscriptionGroups.layout.listLabel')">
        <li
          v-for="(row, index) in rows"
          :key="row.id"
          class="layout-sheet__item"
          :class="{ 'is-dragging': dragIndex === index }"
          :draggable="armedIndex === index"
          :data-test="`layout-row-${row.id}`"
          @dragstart="onDragStart($event, index)"
          @dragenter.prevent="onDragEnter(index)"
          @dragover.prevent
          @drop.prevent
          @dragend="onDragEnd"
        >
          <button
            type="button"
            class="layout-sheet__handle"
            :aria-label="t('adminSubscriptionGroups.layout.handle', { name: rowTitle(row) })"
            :title="t('adminSubscriptionGroups.layout.handleHelp')"
            @pointerdown="armedIndex = index"
            @pointerup="armedIndex = -1"
            @keydown.up.prevent="move(index, index - 1)"
            @keydown.down.prevent="move(index, index + 1)"
          >
            <GripVertical :size="16" aria-hidden="true" />
          </button>
          <div class="layout-sheet__body">
            <div class="layout-sheet__meta">
              <span class="layout-sheet__index">{{ index + 1 }}</span>
              <span class="layout-sheet__title">{{ rowTitle(row) }}</span>
              <span class="layout-sheet__type">{{ String(row.type || '').toUpperCase() }} :{{ row.port }}</span>
            </div>
            <UiTextField
              v-model="row.display_name"
              size="sm"
              :label="t('adminSubscriptionGroups.layout.displayName')"
              :placeholder="defaultProtocolName(row)"
              :error="errorOf(row)"
              :data-test="`layout-name-${row.id}`"
            />
          </div>
          <div class="layout-sheet__moves">
            <UiIconButton :icon="ChevronUp" size="sm" :label="t('adminSubscriptionGroups.layout.moveUp')" :disabled="index === 0" @click="move(index, index - 1)" />
            <UiIconButton :icon="ChevronDown" size="sm" :label="t('adminSubscriptionGroups.layout.moveDown')" :disabled="index === rows.length - 1" @click="move(index, index + 1)" />
          </div>
        </li>
      </ol>
      <p class="layout-sheet__live" aria-live="polite">{{ announcement }}</p>
      <p v-if="saveError" class="layout-sheet__error" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" :disabled="!rows.length" data-test="save-protocol-layout" @click="save">{{ t('adminSubscriptionGroups.layout.save') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// 排序与名称: drag (or ↑/↓ on the handle, or the arrow buttons) a group's
// node protocols into the order its subscription lists them, and give each
// the name subscribers see; empty keeps "<node> - <protocol>". Saves the
// whole list (PUT /api/v4/admin/subscription-groups/:id/protocol-layout).
import { ref, watch } from 'vue'
import { ChevronDown, ChevronUp, GripVertical } from '@lucide/vue'
import { saveGroupProtocolLayout } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiIconButton, UiSheet, UiTextField, useToast } from '@/ui'
import { MAX_DISPLAY_NAME, defaultProtocolName, layoutRequest, moveItem } from './groupProtocolLayout'

const props = defineProps({
  open: { type: Boolean, default: false },
  group: { type: Object, required: true },
  // The group's protocols in the group's order, each with display_name.
  protocols: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const rows = ref([])
const saving = ref(false)
const saveError = ref('')
const announcement = ref('')
const armedIndex = ref(-1)
const dragIndex = ref(-1)

watch(() => props.open, value => {
  if (!value) return
  rows.value = props.protocols.map(protocol => ({ ...protocol, display_name: protocol.display_name || '' }))
  saveError.value = ''
  announcement.value = ''
}, { immediate: true })

function rowTitle(row) {
  return row.node?.name || t('adminSubscriptionGroups.protocols.nodeIdFallback', { id: row.node_id })
}

function errorOf(row) {
  const name = String(row.display_name || '').trim()
  if ([...name].length > MAX_DISPLAY_NAME) return t('adminSubscriptionGroups.layout.tooLong', { max: MAX_DISPLAY_NAME })
  if (/[\u0000-\u001f\u007f]/.test(name)) return t('adminSubscriptionGroups.layout.controlChar')
  return ''
}

function move(from, to) {
  if (to < 0 || to >= rows.value.length) return
  rows.value = moveItem(rows.value, from, to)
  announcement.value = t('adminSubscriptionGroups.layout.moved', { name: rowTitle(rows.value[to]), position: to + 1, total: rows.value.length })
}

function onDragStart(event, index) {
  dragIndex.value = index
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(rows.value[index]?.id ?? ''))
  }
}

function onDragEnter(index) {
  if (dragIndex.value < 0 || index === dragIndex.value) return
  rows.value = moveItem(rows.value, dragIndex.value, index)
  dragIndex.value = index
}

function onDragEnd() {
  if (dragIndex.value >= 0) {
    const row = rows.value[dragIndex.value]
    if (row) announcement.value = t('adminSubscriptionGroups.layout.moved', { name: rowTitle(row), position: dragIndex.value + 1, total: rows.value.length })
  }
  dragIndex.value = -1
  armedIndex.value = -1
}

async function save() {
  if (saving.value || rows.value.some(row => errorOf(row))) return
  saving.value = true
  saveError.value = ''
  try {
    const answer = await saveGroupProtocolLayout(props.group.id, layoutRequest(rows.value))
    toast.success(t('adminSubscriptionGroups.layout.saved'))
    emit('saved', answer?.protocols || [])
    emit('update:open', false)
  } catch (error) {
    const answer = error?.response?.data
    saveError.value = answer?.error?.message || answer?.message || error?.message || t('adminSubscriptionGroups.layout.saveFailed')
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.layout-sheet {
  display: grid;
  gap: var(--space-3);
}

.layout-sheet__list {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.layout-sheet__item {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  padding: var(--space-3);
  border: 1px solid var(--separator);
  border-radius: var(--radius-md);
  background: var(--bg-elevated, transparent);
  min-width: 0;
}

.layout-sheet__item.is-dragging {
  opacity: 0.5;
  border-style: dashed;
}

.layout-sheet__handle {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  margin-top: 2px;
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--label-2);
  cursor: grab;
  touch-action: none;
}

.layout-sheet__handle:active {
  cursor: grabbing;
}

.layout-sheet__handle:focus-visible {
  outline: 2px solid var(--focus-ring, var(--accent));
  outline-offset: 1px;
}

.layout-sheet__body {
  display: grid;
  flex: 1;
  gap: var(--space-2);
  min-width: 0;
}

.layout-sheet__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: baseline;
  min-width: 0;
}

.layout-sheet__index {
  color: var(--label-3, var(--label-2));
  font-variant-numeric: tabular-nums;
}

.layout-sheet__title {
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.layout-sheet__type {
  color: var(--label-2);
  font-size: var(--type-footnote-size, var(--type-callout-size));
}

.layout-sheet__moves {
  display: flex;
  flex: none;
  flex-direction: column;
  gap: var(--space-1);
}

.layout-sheet__empty,
.layout-sheet__live {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.layout-sheet__live:empty {
  display: none;
}

.layout-sheet__error {
  margin: 0;
  color: var(--red, var(--danger));
}
</style>
