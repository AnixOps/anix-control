<template>
  <UiSection :title="t('admin.nodes.logs.title')" :description="t('admin.nodes.logs.description')">
    <template #actions>
      <UiIconButton variant="secondary" :icon="RefreshCw" :label="t('admin.nodes.logs.refresh')" :disabled="loading" data-testid="refresh-logs" @click="refresh" />
    </template>
    <UiDataTable
      :columns="columns"
      :rows="logs"
      :label="t('admin.nodes.logs.tableLabel', { name: node.name })"
      :row-label="log => `${formatLogTime(log)} ${log.source || ''}`"
      storage-key="admin.node-logs"
      manual-pagination
      :page="pagination.page"
      :page-size="pagination.size"
      :total="pagination.total"
      :loading="loading"
      :error="error"
      :error-title="t('admin.nodes.logs.loadFailed')"
      :filtered="Boolean(filter.level || filter.source || filter.search)"
      :empty-icon="ScrollText"
      :empty-title="t('admin.nodes.logs.empty')"
      :empty-description="t('admin.nodes.logs.emptyDescription')"
      state-heading-tag="h3"
      default-density="compact"
      @update:page="changePage"
      @retry="load"
      @clear-filters="clearFilters"
    >
      <template #toolbar>
        <UiSearchField
          v-model="filter.search"
          class="list-page__search"
          :label="t('admin.nodes.logs.filters.search')"
          :placeholder="t('admin.nodes.logs.filters.searchPlaceholder')"
          :shortcut="false"
          @update:model-value="scheduleRefresh"
          @submit="refresh"
        />
        <UiSelect
          v-model="levelValue"
          class="node-logs__level"
          size="md"
          :aria-label="t('admin.nodes.logs.columns.level')"
          :options="levelOptions"
        />
        <UiTextField
          v-model.trim="filter.source"
          class="node-logs__source"
          size="md"
          :aria-label="t('admin.nodes.logs.columns.source')"
          :placeholder="t('admin.nodes.logs.filters.sourcePlaceholder')"
          @keyup.enter="refresh"
          @change="refresh"
        />
      </template>
      <template #cell-level="{ row }">
        <UiBadge :tone="levelTone(row.level)" :label="levelLabel(row.level)" />
      </template>
      <template #cell-message="{ row }">
        <div class="node-log">
          <span class="node-log__message">{{ row.message }}</span>
          <span v-if="row.trace_id" class="node-log__meta">trace <code>{{ row.trace_id }}</code></span>
          <details v-if="row.fields_json || row.fields" class="node-log__fields">
            <summary>{{ t('admin.nodes.logs.columns.fields') }}</summary>
            <pre>{{ formatLogFields(row.fields, row.fields_json) }}</pre>
          </details>
        </div>
      </template>
    </UiDataTable>
  </UiSection>
</template>

<script setup>
// 日志 section of the node page: GET /admin/nodes/:id/logs with the level,
// source and text filters and server pages of 20.
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { RefreshCw, ScrollText } from '@lucide/vue'
import { getNodeLogs } from '@/api/admin'
import UiBadge from '@/ui/UiBadge.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import { readNodePage } from './nodeData'

const props = defineProps({
  node: { type: Object, required: true }
})
const { t } = useAppI18n()
const format = useFormat()

const LOG_LEVELS = ['debug', 'info', 'warning', 'error']
const ALL = 'all'

const logs = ref([])
const loading = ref(false)
const error = ref(null)
const filter = reactive({ level: '', source: '', search: '' })
const pagination = reactive({ page: 1, size: 20, total: 0 })

const levelValue = computed({
  get: () => filter.level || ALL,
  set: (value) => {
    filter.level = value === ALL ? '' : value
    refresh()
  }
})
const levelOptions = computed(() => [
  { value: ALL, label: t('admin.nodes.logs.filters.allLevels') },
  ...LOG_LEVELS.map(level => ({ value: level, label: t(`admin.nodes.logs.levels.${level}`) }))
])

const columns = computed(() => [
  { key: 'time', label: t('admin.nodes.logs.columns.time'), value: formatLogTime, nowrap: true, width: 150, secondary: true },
  { key: 'level', label: t('admin.nodes.logs.columns.level'), nowrap: true, width: 96 },
  { key: 'source', label: t('admin.nodes.logs.columns.source'), value: log => log.source || '—', nowrap: true, breakpoint: 'md' },
  { key: 'message', label: t('admin.nodes.logs.columns.message'), primary: true, hideable: false }
])

function levelLabel(level) {
  return LOG_LEVELS.includes(level) ? t(`admin.nodes.logs.levels.${level}`) : String(level || 'INFO').toUpperCase()
}

function levelTone(level) {
  if (level === 'error') return 'danger'
  if (level === 'warning') return 'warning'
  if (level === 'debug') return 'neutral'
  return 'info'
}

function formatLogTime(log) {
  return format.dateTime(log.logged_at || log.created_at)
}

function formatLogFields(fields, fallback) {
  if (fields) return JSON.stringify(fields, null, 2)
  if (!fallback) return '{}'
  try {
    return JSON.stringify(JSON.parse(fallback), null, 2)
  } catch {
    return fallback
  }
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const payload = readNodePage(await getNodeLogs(props.node.id, {
      page: pagination.page,
      page_size: pagination.size,
      level: filter.level || undefined,
      source: filter.source || undefined,
      search: filter.search || undefined
    }))
    logs.value = payload.list || []
    pagination.total = payload.total || 0
  } catch (e) {
    console.error('Failed to load node logs:', e)
    logs.value = []
    pagination.total = 0
    error.value = e
  } finally {
    loading.value = false
  }
}

let timer = null
function scheduleRefresh() {
  clearTimeout(timer)
  timer = setTimeout(refresh, 300)
}

function refresh() {
  clearTimeout(timer)
  pagination.page = 1
  return load()
}

function changePage(page) {
  pagination.page = page
  load()
}

function clearFilters() {
  filter.level = ''
  filter.source = ''
  filter.search = ''
  refresh()
}

onMounted(load)
onBeforeUnmount(() => clearTimeout(timer))

defineExpose({ load, refresh, logs, pagination, filter })
</script>

<style scoped>
.node-logs__level {
  width: 160px;
}

.node-logs__source {
  width: 180px;
}

.node-log {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.node-log__message {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.node-log__meta {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.node-log__fields summary {
  color: var(--label-2);
  font-size: var(--type-caption-size);
  cursor: pointer;
}

.node-log__fields summary:focus-visible {
  border-radius: var(--radius-xs);
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.node-log__fields pre {
  max-height: 240px;
  margin: var(--space-1) 0 0;
  padding: var(--space-2) var(--space-3);
  overflow: auto;
  border-radius: var(--radius-xs);
  background: var(--bg-grouped);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

@media (max-width: 639.98px) {
  .node-logs__level,
  .node-logs__source {
    width: 100%;
  }
}
</style>
