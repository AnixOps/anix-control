<template>
  <div class="ui-data-table" :class="[`is-${density}`, { 'is-phone': isPhone }]" :data-density="density">
    <div v-if="$slots.toolbar || $slots['toolbar-end'] || showSettings" class="ui-data-table__toolbar">
      <div class="ui-data-table__toolbar-main">
        <slot name="toolbar" />
      </div>
      <div v-if="$slots['toolbar-end'] || showSettings" class="ui-data-table__toolbar-end">
        <slot name="toolbar-end" />
        <UiMenu v-if="showSettings" :label="t('ui.table.settings')" :icon="SlidersHorizontal" size="md" data-table-settings>
          <DropdownMenuLabel v-if="hideableColumns.length" class="ui-menu__group-label">{{ t('ui.table.columns') }}</DropdownMenuLabel>
          <DropdownMenuCheckboxItem
            v-for="column in hideableColumns"
            :key="column.key"
            class="ui-menu__item"
            :model-value="!hidden.includes(column.key)"
            :data-column-toggle="column.key"
            @update:model-value="value => setColumnVisible(column.key, value)"
            @select="event => event.preventDefault()"
          >
            <span class="ui-menu__label">{{ column.label }}</span>
            <DropdownMenuItemIndicator class="ui-menu__check">
              <UiIcon :icon="Check" :size="16" />
            </DropdownMenuItemIndicator>
          </DropdownMenuCheckboxItem>
          <DropdownMenuSeparator v-if="hideableColumns.length" class="ui-menu__separator" />
          <DropdownMenuLabel class="ui-menu__group-label">{{ t('ui.table.density') }}</DropdownMenuLabel>
          <DropdownMenuRadioGroup :model-value="density" @update:model-value="value => { density = value }">
            <DropdownMenuRadioItem
              v-for="option in DENSITIES"
              :key="option"
              class="ui-menu__item"
              :value="option"
              :data-density-option="option"
              @select="event => event.preventDefault()"
            >
              <span class="ui-menu__label">{{ t(`ui.table.densities.${option}`) }}</span>
              <DropdownMenuItemIndicator class="ui-menu__check">
                <UiIcon :icon="Check" :size="16" />
              </DropdownMenuItemIndicator>
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </UiMenu>
      </div>
    </div>

    <div class="ui-data-table__surface" :class="{ 'is-flat': flat }" :aria-busy="loading ? 'true' : undefined">
      <UiErrorState
        v-if="error"
        :title="errorTitle || t('ui.table.loadFailed')"
        :error="error"
        :heading-tag="stateHeadingTag"
        @retry="emit('retry')"
      />
      <UiSkeleton
        v-else-if="showSkeleton"
        variant="table-row"
        :rows="skeletonRows"
        :columns="Math.min(visibleColumns.length, isPhone ? 2 : 6) || 1"
        :label="t('ui.table.loading', { label })"
      />
      <div v-else-if="loading && !pageRows.length" class="ui-data-table__pending" />
      <template v-else-if="!pageRows.length">
        <slot name="empty" :filtered="filtered">
          <UiEmptyState
            v-if="filtered"
            :icon="SearchX"
            :title="t('ui.table.noMatches')"
            :description="t('ui.table.noMatchesHint')"
            :heading-tag="stateHeadingTag"
          >
            <template #actions>
              <UiButton data-clear-filters @click="emit('clear-filters')">{{ t('ui.table.clearFilters') }}</UiButton>
            </template>
          </UiEmptyState>
          <UiEmptyState
            v-else
            :icon="emptyIcon || Inbox"
            :title="emptyTitle || t('ui.table.empty')"
            :description="emptyDescription"
            :heading-tag="stateHeadingTag"
          >
            <template v-if="$slots['empty-actions']" #actions>
              <slot name="empty-actions" />
            </template>
          </UiEmptyState>
        </slot>
      </template>

      <ul v-else-if="isPhone" class="ui-data-table__cards" :aria-label="label" :class="{ 'is-refreshing': refreshing }">
        <li
          v-for="(row, index) in pageRows"
          :key="keyOf(row, index)"
          class="ui-data-table__card"
          :class="{ 'is-selected': isSelected(row, index) }"
          :data-row-key="keyOf(row, index)"
        >
          <div class="ui-data-table__card-head">
            <span v-if="selectable" class="ui-data-table__card-check">
              <UiCheckbox
                :model-value="isSelected(row, index)"
                :aria-label="t('ui.table.selectRow', { name: nameOf(row) })"
                @update:model-value="value => toggleRow(row, index, value)"
              />
            </span>
            <div class="ui-data-table__card-title-wrap">
              <component
                :is="activatable ? 'button' : 'span'"
                :type="activatable ? 'button' : undefined"
                class="ui-data-table__card-title"
                :class="{ 'is-action': activatable }"
                @click="activatable && emit('row-activate', row)"
              >
                <slot v-if="primaryColumn" :name="`cell-${primaryColumn.key}`" :row="row" :value="cellValue(row, primaryColumn)" :card="true">{{ cellText(row, primaryColumn) }}</slot>
              </component>
              <span v-if="secondaryColumn" class="ui-data-table__card-subtitle">
                <slot :name="`cell-${secondaryColumn.key}`" :row="row" :value="cellValue(row, secondaryColumn)" :card="true">{{ cellText(row, secondaryColumn) }}</slot>
              </span>
            </div>
            <UiMenu
              v-if="rowActions && actionsFor(row).length"
              :label="t('ui.table.rowActions', { name: nameOf(row) })"
              :items="actionsFor(row)"
              size="md"
              data-row-actions
            />
          </div>
          <dl v-if="cardColumns.length" class="ui-data-table__card-fields">
            <div v-for="column in cardColumns" :key="column.key" class="ui-data-table__card-field">
              <dt>{{ column.label }}</dt>
              <dd :class="{ 'tabular-nums': column.numeric }">
                <slot :name="`cell-${column.key}`" :row="row" :value="cellValue(row, column)" :card="true">{{ cellText(row, column) }}</slot>
              </dd>
            </div>
          </dl>
        </li>
      </ul>

      <table v-else class="ui-data-table__table" :class="{ 'is-sticky': stickyHeader, 'is-refreshing': refreshing }">
        <caption class="visually-hidden">{{ label }}</caption>
        <thead>
          <tr>
            <th v-if="selectable" scope="col" class="ui-data-table__select-cell">
              <UiCheckbox
                :model-value="pageSelectionState"
                :aria-label="t('ui.table.selectAll')"
                data-select-all
                @update:model-value="togglePage"
              />
            </th>
            <th
              v-for="column in visibleColumns"
              :key="column.key"
              scope="col"
              :class="columnClass(column)"
              :style="columnStyle(column)"
              :aria-sort="ariaSort(sortState, column)"
            >
              <button
                v-if="column.sortable"
                type="button"
                class="ui-data-table__sort"
                :data-sort="column.key"
                @click="toggleSort(column)"
              >
                <span>{{ column.label }}</span>
                <UiIcon class="ui-data-table__sort-icon" :icon="sortIcon(column)" :size="14" />
              </button>
              <template v-else>{{ column.label }}</template>
            </th>
            <th v-if="rowActions" scope="col" class="ui-data-table__actions-cell">
              <span class="visually-hidden">{{ t('ui.table.actions') }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(row, index) in pageRows"
            :key="keyOf(row, index)"
            :ref="el => setRowRef(el, index)"
            :class="{ 'is-selected': isSelected(row, index), 'is-activatable': activatable }"
            :tabindex="activatable ? (index === focusIndex ? 0 : -1) : undefined"
            :data-row-key="keyOf(row, index)"
            @click="onRowClick($event, row, index)"
            @keydown="onRowKeydown($event, row, index)"
            @focus="focusIndex = index"
          >
            <td v-if="selectable" class="ui-data-table__select-cell">
              <UiCheckbox
                :model-value="isSelected(row, index)"
                :aria-label="t('ui.table.selectRow', { name: nameOf(row) })"
                @update:model-value="value => toggleRow(row, index, value)"
              />
            </td>
            <td v-for="column in visibleColumns" :key="column.key" :class="columnClass(column)" :style="cellStyle(column)" :title="column.truncate ? cellText(row, column) : undefined">
              <slot :name="`cell-${column.key}`" :row="row" :value="cellValue(row, column)" :card="false">{{ cellText(row, column) }}</slot>
            </td>
            <td v-if="rowActions" class="ui-data-table__actions-cell">
              <UiMenu
                v-if="actionsFor(row).length"
                :label="t('ui.table.rowActions', { name: nameOf(row) })"
                :items="actionsFor(row)"
                data-row-actions
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <UiPagination
      v-if="paginated && !error && totalRows > 0"
      class="ui-data-table__pagination"
      :page="currentPage"
      :total="totalRows"
      :page-size="pageSize"
      @update:page="setPage"
    />

    <p class="visually-hidden" aria-live="polite">{{ selectionAnnouncement }}</p>

    <Teleport to="body" :disabled="!teleportBulkBar">
      <Transition name="ui-bulk">
        <div
          v-if="selectable && selectedKeys.length"
          class="ui-bulk-bar"
          :class="{ 'is-under-overlay': modalOpen }"
          role="region"
          :aria-label="t('ui.table.bulkActions')"
          data-bulk-bar
          @keydown.esc="clearSelection"
        >
          <span class="ui-bulk-bar__count tabular-nums">{{ t('ui.table.selected', { count: selectedKeys.length }) }}</span>
          <UiButton
            v-if="canSelectAllMatching"
            variant="tertiary"
            size="sm"
            class="ui-bulk-bar__all"
            data-select-all-matching
            @click="selectAllMatching"
          >{{ t('ui.table.selectAllMatching', { count: totalRows }) }}</UiButton>
          <div v-if="$slots['bulk-actions']" class="ui-bulk-bar__actions">
            <slot name="bulk-actions" :selected="selectedKeys" :rows="selectedRows" :clear="clearSelection" />
          </div>
          <UiIconButton :icon="X" :label="t('ui.table.clearSelection')" size="sm" class="ui-bulk-bar__clear" data-clear-selection @click="clearSelection" />
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup>
// Data table of a list page (plan §6, §7.1). Column definitions drive the
// header, the cells, the phone cards, sorting and the column settings:
//
//   { key, label, sortable, firstDirection: 'desc', align: 'end', numeric,
//     width, minWidth, maxWidth, nowrap (one line), truncate (one line
//     with an ellipsis, the full text as the cell's title; give maxWidth),
//     value: row => …, format: (value, row) => text,
//     sortValue: row => …, hideable (default true), hidden (default off),
//     breakpoint: 'md' | 'lg' (hidden in the table below it),
//     primary / secondary (card title / subtitle), card: false }
//
// - a real <table> with a caption, scope="col" and aria-sort on sortable
//   headers (the header is a button: none → asc → desc → none);
// - client-side sorting and pagination, or server-side with manualSort /
//   manualPagination (+ total), emitting update:sort / update:page;
// - selection with a checkbox column and a floating bulk bar (slot
//   bulk-actions, "全选所有 N 条" when every row is loaded, Esc clears);
// - a "…" row menu (rowActions: row => [{ key, label, icon, danger, onSelect }]);
// - activatable rows: click or Enter emits row-activate, ↑/↓/Home/End move
//   between rows, Space toggles the selection;
// - states: an error with 重试 / 复制错误详情, skeleton rows after 300 ms,
//   empty (or "no matches" + 清除筛选 when `filtered`);
// - sticky header under the shell's top bar; comfortable / compact density
//   and hidden columns remembered per `storageKey`;
// - phones (< 640 px): one card per row (title, subtitle, three fields, "…").
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  DropdownMenuCheckboxItem, DropdownMenuItemIndicator, DropdownMenuLabel,
  DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator
} from 'reka-ui'
import { ArrowDown, ArrowUp, ChevronsUpDown, Check, Inbox, SearchX, SlidersHorizontal, X } from '@lucide/vue'
import UiButton from './UiButton.vue'
import UiCheckbox from './UiCheckbox.vue'
import UiEmptyState from './UiEmptyState.vue'
import UiErrorState from './UiErrorState.vue'
import UiIcon from './UiIcon.vue'
import UiIconButton from './UiIconButton.vue'
import UiMenu from './UiMenu.vue'
import UiPagination from './UiPagination.vue'
import UiSkeleton from './UiSkeleton.vue'
import { useDelayedLoading } from './composables/useDelayedLoading'
import { PHONE_QUERY, useMediaQuery } from './composables/useMediaQuery'
import { useModalOpen } from './composables/useModalOpen'
import { DENSITIES, useTablePreferences } from './composables/useTablePreferences'
import { ariaSort, cellText, cellValue, nextSort, rowKeyOf, sortRows } from './internal/tableModel'

const props = defineProps({
  columns: { type: Array, required: true },
  rows: { type: Array, default: () => [] },
  rowKey: { type: [String, Function], default: 'id' },
  // Names the table (caption) and the card list.
  label: { type: String, required: true },
  // Name of a row for "选择 {name}" and "{name} 的操作"; default: the primary cell.
  rowLabel: { type: Function, default: null },
  loading: { type: Boolean, default: false },
  error: { type: [Object, String, null], default: null },
  errorTitle: { type: String, default: '' },
  emptyTitle: { type: String, default: '' },
  emptyDescription: { type: String, default: '' },
  emptyIcon: { type: [Object, Function], default: null },
  // Filters or a search are active: the empty state offers 清除筛选.
  filtered: { type: Boolean, default: false },
  stateHeadingTag: { type: String, default: 'h2' },
  sort: { type: Object, default: undefined },
  manualSort: { type: Boolean, default: false },
  page: { type: Number, default: undefined },
  // 0 = no pagination.
  pageSize: { type: Number, default: 0 },
  total: { type: Number, default: undefined },
  manualPagination: { type: Boolean, default: false },
  selectable: { type: Boolean, default: false },
  selected: { type: Array, default: undefined },
  rowActions: { type: Function, default: null },
  activatable: { type: Boolean, default: false },
  storageKey: { type: String, default: '' },
  defaultDensity: { type: String, default: 'comfortable' },
  stickyHeader: { type: Boolean, default: true },
  skeletonRows: { type: Number, default: 5 },
  // Show the table settings menu (columns, density).
  settings: { type: Boolean, default: true },
  // Fields shown on a phone card besides the title and subtitle.
  cardFields: { type: Number, default: 3 },
  // A table inside a card or a sheet: no surface of its own.
  flat: { type: Boolean, default: false },
  teleportBulkBar: { type: Boolean, default: true }
})

const emit = defineEmits(['update:sort', 'update:page', 'update:selected', 'row-activate', 'retry', 'clear-filters'])
const { t } = useI18n()

const isPhone = useMediaQuery(PHONE_QUERY)
// The bulk bar sits under the overlays (--z-sticky < --z-drawer / --z-modal)
// and fades out while one is open, so it never looks like part of a dialog.
const modalOpen = useModalOpen()
const { hidden, density } = useTablePreferences(props.storageKey, {
  hidden: props.columns.filter(column => column.hidden).map(column => column.key),
  density: props.defaultDensity
})

// Local mirrors: the table works controlled (v-model) or on its own.
const sortState = ref(props.sort || { key: '', direction: '' })
watch(() => props.sort, (value) => { if (value) sortState.value = value })
const pageState = ref(props.page || 1)
watch(() => props.page, (value) => { if (value) pageState.value = value })
const selectedState = ref(props.selected ? [...props.selected] : [])
watch(() => props.selected, (value) => { if (value) selectedState.value = [...value] })

const hideableColumns = computed(() => props.columns.filter(column => column.hideable !== false && !column.primary))
const visibleColumns = computed(() => props.columns.filter(column => column.hideable === false || column.primary || !hidden.value.includes(column.key)))
const showSettings = computed(() => props.settings)
const primaryColumn = computed(() => props.columns.find(column => column.primary) || visibleColumns.value[0] || null)
const secondaryColumn = computed(() => props.columns.find(column => column.secondary && column.key !== primaryColumn.value?.key) || null)
const cardColumns = computed(() => visibleColumns.value
  .filter(column => column !== primaryColumn.value && column !== secondaryColumn.value && column.card !== false)
  .slice(0, props.cardFields))

const sortedRows = computed(() => (props.manualSort ? props.rows : sortRows(props.rows, props.columns, sortState.value)))
const paginated = computed(() => props.pageSize > 0)
const totalRows = computed(() => (props.manualPagination ? Number(props.total || 0) : props.rows.length))
const pageCount = computed(() => (paginated.value ? Math.max(1, Math.ceil(totalRows.value / props.pageSize)) : 1))
const currentPage = computed(() => Math.min(Math.max(1, pageState.value), pageCount.value))
const pageRows = computed(() => {
  if (!paginated.value || props.manualPagination) return sortedRows.value
  const start = (currentPage.value - 1) * props.pageSize
  return sortedRows.value.slice(start, start + props.pageSize)
})

// Client-side: the page shrinks under the current page (a filter, a
// delete) → follow it.
watch(pageCount, (count) => {
  if (!props.manualPagination && pageState.value > count) setPage(count)
})

const showSkeleton = useDelayedLoading(() => props.loading && !props.rows.length)
const refreshing = useDelayedLoading(() => props.loading && props.rows.length > 0)

function keyOf(row, index) {
  return rowKeyOf(row, props.rowKey, index)
}

function nameOf(row) {
  if (props.rowLabel) return props.rowLabel(row)
  return primaryColumn.value ? cellText(row, primaryColumn.value) : ''
}

function actionsFor(row) {
  const items = props.rowActions ? props.rowActions(row) : []
  return Array.isArray(items) ? items.filter(item => item && !item.hidden) : []
}

function columnClass(column) {
  return {
    [`is-align-${column.align || 'start'}`]: true,
    'tabular-nums': column.numeric,
    'is-nowrap': column.nowrap,
    'is-truncate': column.truncate,
    [`is-from-${column.breakpoint}`]: Boolean(column.breakpoint),
    [column.class || '']: Boolean(column.class)
  }
}

const cssLength = value => (typeof value === 'number' ? `${value}px` : value)

function columnStyle(column) {
  const style = {}
  if (column.width) style.width = cssLength(column.width)
  if (column.minWidth) style.minWidth = cssLength(column.minWidth)
  return Object.keys(style).length ? style : undefined
}

// Body cells also take maxWidth, which truncate needs to stop growing.
function cellStyle(column) {
  const style = { ...(columnStyle(column) || {}) }
  if (column.maxWidth) style.maxWidth = cssLength(column.maxWidth)
  return Object.keys(style).length ? style : undefined
}

function sortIcon(column) {
  if (sortState.value.key !== column.key || !sortState.value.direction) return ChevronsUpDown
  return sortState.value.direction === 'desc' ? ArrowDown : ArrowUp
}

function toggleSort(column) {
  const next = nextSort(sortState.value, column)
  sortState.value = next
  emit('update:sort', next)
  if (paginated.value && !props.manualPagination) setPage(1)
}

function setPage(page) {
  pageState.value = page
  emit('update:page', page)
}

function setColumnVisible(key, visible) {
  hidden.value = visible ? hidden.value.filter(item => item !== key) : [...new Set([...hidden.value, key])]
}

// Selection ---------------------------------------------------------------
const selectedKeys = computed(() => selectedState.value)
const selectedSet = computed(() => new Set(selectedState.value))
// Rows seen on any page, so the bulk bar has the selected rows of earlier pages.
const rowCache = new Map()
watch(() => props.rows, (rows) => {
  rows.forEach((row, index) => rowCache.set(keyOf(row, index), row))
}, { immediate: true })
const selectedRows = computed(() => selectedState.value.map(key => rowCache.get(key)).filter(Boolean))

function setSelected(keys) {
  selectedState.value = keys
  emit('update:selected', keys)
}

function isSelected(row, index) {
  return selectedSet.value.has(keyOf(row, index))
}

function toggleRow(row, index, value) {
  const key = keyOf(row, index)
  const on = value === true
  if (on && !selectedSet.value.has(key)) setSelected([...selectedState.value, key])
  if (!on && selectedSet.value.has(key)) setSelected(selectedState.value.filter(item => item !== key))
}

const pageKeys = computed(() => pageRows.value.map((row, index) => keyOf(row, index)))
const pageSelectionState = computed(() => {
  const keys = pageKeys.value
  if (!keys.length) return false
  const count = keys.filter(key => selectedSet.value.has(key)).length
  if (count === 0) return false
  return count === keys.length ? true : 'indeterminate'
})

function togglePage(value) {
  const keys = pageKeys.value
  if (value === true) setSelected([...new Set([...selectedState.value, ...keys])])
  else {
    const drop = new Set(keys)
    setSelected(selectedState.value.filter(key => !drop.has(key)))
  }
}

// "全选所有 N 条" only when every row is loaded (client-side lists).
const canSelectAllMatching = computed(() => !props.manualPagination && paginated.value
  && totalRows.value > pageKeys.value.length
  && selectedState.value.length < totalRows.value
  && pageSelectionState.value === true)

function selectAllMatching() {
  setSelected(sortedRows.value.map((row, index) => keyOf(row, index)))
}

function clearSelection() {
  setSelected([])
}

const selectionAnnouncement = computed(() => (selectedState.value.length ? t('ui.table.selected', { count: selectedState.value.length }) : ''))

// Keyboard ------------------------------------------------------------------
const focusIndex = ref(0)
const rowRefs = []
watch(pageRows, (rows) => {
  rowRefs.length = rows.length
  if (focusIndex.value >= rows.length) focusIndex.value = Math.max(0, rows.length - 1)
})

function setRowRef(el, index) {
  rowRefs[index] = el
}

const INTERACTIVE = 'button, a, input, select, textarea, label, [role="checkbox"], [role="menuitem"], [role="switch"], [contenteditable="true"]'

function onRowClick(event, row, index) {
  if (!props.activatable) return
  if (event.target instanceof Element && event.target.closest(INTERACTIVE)) return
  focusIndex.value = index
  emit('row-activate', row)
}

async function moveFocus(index) {
  const next = Math.min(Math.max(0, index), pageRows.value.length - 1)
  focusIndex.value = next
  await nextTick()
  rowRefs[next]?.focus()
}

function onRowKeydown(event, row, index) {
  if (!props.activatable || event.target !== event.currentTarget) return
  switch (event.key) {
    case 'Enter':
      event.preventDefault()
      emit('row-activate', row)
      break
    case ' ':
      if (props.selectable) {
        event.preventDefault()
        toggleRow(row, index, !isSelected(row, index))
      }
      break
    case 'ArrowDown':
      event.preventDefault()
      moveFocus(index + 1)
      break
    case 'ArrowUp':
      event.preventDefault()
      moveFocus(index - 1)
      break
    case 'Home':
      event.preventDefault()
      moveFocus(0)
      break
    case 'End':
      event.preventDefault()
      moveFocus(pageRows.value.length - 1)
      break
    default:
  }
}

defineExpose({ clearSelection, setPage })
</script>

<style scoped>
.ui-data-table {
  --ui-row-height: 52px;
  --ui-cell-pad-x: var(--space-4);

  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}

.ui-data-table.is-compact {
  --ui-row-height: 40px;
  --ui-cell-pad-x: var(--space-3);
}

.ui-data-table__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
}

.ui-data-table__toolbar-main {
  display: flex;
  flex: 1 1 320px;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  min-width: 0;
}

.ui-data-table__toolbar-end {
  display: flex;
  gap: var(--space-2);
  align-items: center;
  margin-left: auto;
}

.ui-data-table__surface {
  min-width: 0;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.ui-data-table__surface.is-flat {
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

.ui-data-table__pending {
  min-height: calc(var(--ui-row-height) * 3);
}

/* Table ------------------------------------------------------------------ */
.ui-data-table__table {
  width: 100%;
  border-spacing: 0;
  border-collapse: separate;
  font-size: var(--type-body-size);
}

.ui-data-table__table.is-refreshing tbody,
.ui-data-table__cards.is-refreshing {
  opacity: 0.6;
  transition: opacity var(--dur-toggle) var(--ease-standard);
}

.ui-data-table__table th {
  height: 40px;
  padding: 0 var(--ui-cell-pad-x);
  border-bottom: 1px solid var(--separator);
  background: var(--bg-elevated);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  text-align: left;
  white-space: nowrap;
}

.ui-data-table__table.is-sticky th {
  position: sticky;
  top: var(--shell-topbar-height, 0px);
  z-index: 1;
}

.ui-data-table__table th:first-child {
  border-top-left-radius: var(--radius-md);
}

.ui-data-table__table th:last-child {
  border-top-right-radius: var(--radius-md);
}

.ui-data-table__surface.is-flat th {
  border-radius: 0;
  background: transparent;
}

.ui-data-table__sort {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  height: 28px;
  min-height: 0;
  padding: 0 var(--space-1);
  margin: 0 calc(-1 * var(--space-1));
  border: 0;
  border-radius: var(--radius-xs);
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: pointer;
}

.ui-data-table__sort:hover {
  color: var(--label-1);
}

.ui-data-table__sort:focus-visible {
  outline: var(--focus-ring);
  outline-offset: 0;
}

.ui-data-table__sort-icon {
  color: var(--label-3);
}

th[aria-sort='ascending'] .ui-data-table__sort,
th[aria-sort='descending'] .ui-data-table__sort {
  color: var(--label-1);
}

th[aria-sort='ascending'] .ui-data-table__sort-icon,
th[aria-sort='descending'] .ui-data-table__sort-icon {
  color: var(--accent);
}

.ui-data-table__table td {
  height: var(--ui-row-height);
  padding: var(--space-2) var(--ui-cell-pad-x);
  border-bottom: 1px solid var(--separator);
  color: var(--label-1);
  vertical-align: middle;
  overflow-wrap: anywhere;
}

.ui-data-table.is-compact .ui-data-table__table td {
  padding-top: var(--space-1);
  padding-bottom: var(--space-1);
  font-size: var(--type-callout-size);
}

.ui-data-table__table tbody tr:last-child td {
  border-bottom: 0;
}

.ui-data-table__table tbody tr:last-child td:first-child {
  border-bottom-left-radius: var(--radius-md);
}

.ui-data-table__table tbody tr:last-child td:last-child {
  border-bottom-right-radius: var(--radius-md);
}

.ui-data-table__table tbody tr {
  transition: background-color var(--dur-micro) var(--ease-standard);
}

.ui-data-table__table tbody tr.is-activatable {
  cursor: pointer;
}

.ui-data-table__table tbody tr:hover {
  background: var(--fill-1);
}

.ui-data-table__table tbody tr.is-selected {
  /* A lighter tint than --accent-soft keeps status badges and secondary
     text at 4.5:1 on selected rows in both themes; the checkbox and the
     accent bar on the first cell mark the selection too. */
  background: color-mix(in srgb, var(--accent-soft) 40%, transparent);
}

.ui-data-table__table tbody tr.is-selected > td:first-child {
  box-shadow: inset 3px 0 0 var(--accent);
}

.ui-data-table__table tbody tr:focus-visible {
  outline: var(--focus-ring);
  outline-offset: -2px;
}

.is-align-end {
  text-align: right;
}

.ui-data-table__table th.is-align-end {
  text-align: right;
}

.is-align-center,
.ui-data-table__table th.is-align-center {
  text-align: center;
}

.is-nowrap {
  white-space: nowrap;
}

.is-truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ui-data-table__select-cell {
  width: 44px;
  padding-right: 0 !important;
}

.ui-data-table__actions-cell {
  width: 52px;
  padding-left: 0 !important;
  text-align: right;
}

@media (max-width: 1067.98px) {
  .ui-data-table__table .is-from-lg {
    display: none;
  }
}

@media (max-width: 833.98px) {
  .ui-data-table__table .is-from-md {
    display: none;
  }
}

/* Phone cards ------------------------------------------------------------ */
.ui-data-table__cards {
  display: flex;
  flex-direction: column;
  padding: 0;
  margin: 0;
  list-style: none;
}

.ui-data-table__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--separator);
}

.ui-data-table__card:last-child {
  border-bottom: 0;
}

.ui-data-table__card.is-selected {
  background: color-mix(in srgb, var(--accent-soft) 40%, transparent);
  box-shadow: inset 3px 0 0 var(--accent);
}

.ui-data-table__card:first-child.is-selected {
  border-top-left-radius: var(--radius-md);
  border-top-right-radius: var(--radius-md);
}

.ui-data-table__card:last-child.is-selected {
  border-bottom-right-radius: var(--radius-md);
  border-bottom-left-radius: var(--radius-md);
}

.ui-data-table__card-head {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
}

.ui-data-table__card-check {
  display: flex;
  flex: none;
  align-items: center;
  min-height: 32px;
}

.ui-data-table__card-title-wrap {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.ui-data-table__card-title {
  display: block;
  width: 100%;
  min-width: 0;
  min-height: 0;
  border-radius: 0;
  white-space: normal;
  padding: 0;
  border: 0;
  background: none;
  color: var(--label-1);
  font: inherit;
  font-weight: var(--weight-semibold);
  text-align: left;
  overflow-wrap: anywhere;
}

.ui-data-table__card-title.is-action {
  min-height: 32px;
  cursor: pointer;
}

.ui-data-table__card-title:hover,
.ui-data-table__card-title:active {
  background: none;
}

.ui-data-table__card-title.is-action:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.ui-data-table__card-subtitle {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.ui-data-table__card-fields {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: var(--space-2) var(--space-4);
  margin: 0;
}

.ui-data-table__card-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.ui-data-table__card-field dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.ui-data-table__card-field dd {
  margin: 0;
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

/* Bulk bar (teleported to body, so these selectors are not scoped to the
   table's markup alone; the scoped attribute still applies) ------------- */
.ui-bulk-bar {
  position: fixed;
  bottom: calc(var(--space-6) + env(safe-area-inset-bottom, 0px));
  left: 50%;
  /* Layer order: above the sticky header, below drawers and modals. */
  z-index: calc(var(--z-sticky) + 1);
  display: flex;
  gap: var(--space-2);
  align-items: center;
  max-width: calc(100vw - 2 * var(--space-4));
  padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
  border-radius: var(--radius-pill);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-3), 0 0 0 1px var(--separator);
  color: var(--label-1);
  transform: translateX(-50%);
}

@supports ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .ui-bulk-bar {
    background: color-mix(in srgb, var(--bg-elevated) 88%, transparent);
    -webkit-backdrop-filter: saturate(180%) blur(24px);
    backdrop-filter: saturate(180%) blur(24px);
  }
}

.ui-bulk-bar.is-under-overlay {
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--dur-toggle) var(--ease-standard);
}

.ui-bulk-bar__count {
  font-weight: var(--weight-semibold);
  white-space: nowrap;
}

.ui-bulk-bar__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
}

.ui-bulk-enter-active,
.ui-bulk-leave-active {
  transition: opacity var(--dur-toggle) var(--ease-emphasized), transform var(--dur-toggle) var(--ease-emphasized);
}

.ui-bulk-enter-from,
.ui-bulk-leave-to {
  opacity: 0;
  transform: translate(-50%, 24px);
}

@media (max-width: 639.98px) {
  .ui-bulk-bar {
    right: var(--space-4);
    left: var(--space-4);
    flex-wrap: wrap;
    justify-content: space-between;
    border-radius: var(--radius-lg);
    transform: none;
  }

  .ui-bulk-enter-from,
  .ui-bulk-leave-to {
    transform: translateY(24px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .ui-bulk-enter-from,
  .ui-bulk-leave-to {
    transform: translateX(-50%);
  }
}
</style>
