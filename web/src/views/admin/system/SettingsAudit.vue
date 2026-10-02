<template>
  <div class="settings-section" data-settings-panel="audit">
    <UiSection :title="t('adminSettings.audit.title')" :description="t('adminSettings.audit.description')">
      <template #actions>
        <UiButton :icon="RotateCw" :loading="auditLoading" data-test="audit-refresh" @click="refreshAuditLogs">{{ t('adminSettings.audit.refresh') }}</UiButton>
      </template>
      <UiDataTable
        :columns="auditColumns"
        :rows="auditLogs"
        :label="t('adminSettings.audit.title')"
        :row-label="log => `#${log.id ?? ''} ${log.action || ''}`"
        storage-key="admin.system.audit"
        manual-pagination
        :page="auditFilters.page"
        :page-size="auditFilters.page_size"
        :total="auditTotal"
        :loading="auditLoading"
        :error="auditError"
        :error-title="t('adminSettings.audit.loadFailed')"
        :filtered="filtered"
        :empty-icon="ScrollText"
        :empty-title="t('adminSettings.audit.empty')"
        :empty-description="t('adminSettings.audit.emptyDescription')"
        state-heading-tag="h3"
        @update:page="changeAuditPage"
        @retry="fetchAuditLogs"
        @clear-filters="clearAuditFilters"
      >
        <template #toolbar>
          <UiSearchField
            v-model="auditFilters.action"
            class="list-page__search"
            :label="t('adminSettings.audit.filterAction')"
            data-test="audit-filter-action"
            @update:model-value="scheduleFilter"
            @submit="applyAuditFilters"
          />
          <UiSearchField
            v-model="auditFilters.target_type"
            class="list-page__search"
            :shortcut="false"
            :label="t('adminSettings.audit.filterTargetType')"
            data-test="audit-filter-target"
            @update:model-value="scheduleFilter"
            @submit="applyAuditFilters"
          />
          <UiSelect
            :model-value="auditFilters.page_size"
            class="audit-page-size"
            size="md"
            :aria-label="t('adminSettings.audit.pageSize')"
            :options="pageSizeOptions"
            @update:model-value="changeAuditPageSize"
          />
        </template>
        <template #cell-status="{ row }">
          <UiBadge v-if="row.status" :tone="statusTone(row.status)" :label="row.status" />
          <span v-else>—</span>
        </template>
        <template #cell-content="{ row }">
          <span class="audit-content">{{ row.content || '—' }}</span>
        </template>
      </UiDataTable>
    </UiSection>
  </div>
</template>

<script setup>
// 系统设置 → 审计日志: the operation audit log as a server-paged UiDataTable,
// filtered by action and target type (300 ms after typing, or Enter) with a
// page size choice. The filters and the page are in the URL query.
// Endpoint unchanged: GET /admin/system/audit-logs (page, page_size,
// action, target_type).
import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { routerKey } from 'vue-router'
import { RotateCw, ScrollText } from '@lucide/vue'
import { getSystemAuditLogs } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { ensureSystemSuccess, systemErrorText } from './systemResponse'

const { t, translateLiteral } = useAppI18n()
const format = useFormat()
// Optional: the section also works outside a router (unit tests).
const router = inject(routerKey, null)

const auditPageSizeOptions = [20, 50, 100]
const auditLogs = ref([])
const auditLoading = ref(false)
const auditError = ref(null)
const auditTotal = ref(0)
const auditFilters = ref(readQuery())
let filterTimer = null

const pageSizeOptions = computed(() => auditPageSizeOptions.map(size => ({ value: size, label: t('adminSettings.audit.perPage', { size }) })))
const filtered = computed(() => Boolean(String(auditFilters.value.action || '').trim() || String(auditFilters.value.target_type || '').trim()))
const auditTotalPages = computed(() => {
  const pageSize = Number(auditFilters.value.page_size) || 20
  const total = Number(auditTotal.value) || 0
  return Math.max(1, Math.ceil(total / pageSize))
})

const auditColumns = computed(() => [
  { key: 'action', label: t('adminSettings.audit.columns.action'), primary: true, hideable: false, value: log => log.action || '—' },
  { key: 'content', label: t('adminSettings.audit.columns.content'), secondary: true },
  { key: 'username', label: t('adminSettings.audit.columns.username'), value: log => log.username || '—' },
  { key: 'module', label: t('adminSettings.audit.columns.module'), breakpoint: 'lg', value: log => log.module || '—' },
  { key: 'target_type', label: t('adminSettings.audit.columns.targetType'), breakpoint: 'md', value: log => log.target_type || '—' },
  { key: 'status', label: t('adminSettings.audit.columns.status') },
  { key: 'ip', label: t('adminSettings.audit.columns.ip'), hidden: true, value: log => log.ip || '—' },
  { key: 'created_at', label: t('adminSettings.audit.columns.createdAt'), nowrap: true, format: value => (value ? format.dateTime(value) : '—') },
  { key: 'id', label: 'ID', numeric: true, hidden: true, value: log => log.id ?? '—' }
])

function readQuery() {
  const query = router?.currentRoute?.value?.query || {}
  const page = Number(query.page)
  const size = Number(query.size)
  return {
    action: typeof query.action === 'string' ? query.action : '',
    target_type: typeof query.target === 'string' ? query.target : '',
    page: Number.isInteger(page) && page > 0 ? page : 1,
    page_size: auditPageSizeOptions.includes(size) ? size : 20
  }
}

// Keep the filters shareable: write them to the query (replace, no history).
function writeQuery() {
  const current = router?.currentRoute?.value
  if (!current) return
  const query = { ...current.query }
  const set = (key, value, empty) => {
    if (value === empty || value === '' || value === undefined) delete query[key]
    else query[key] = String(value)
  }
  set('action', String(auditFilters.value.action || '').trim(), '')
  set('target', String(auditFilters.value.target_type || '').trim(), '')
  set('page', auditFilters.value.page, 1)
  set('size', auditFilters.value.page_size, 20)
  Promise.resolve(router.replace({ path: current.path, query })).catch(() => {})
}

function statusTone(status) {
  const value = String(status).toLowerCase()
  if (['success', 'ok', 'succeeded'].includes(value)) return 'success'
  if (['failed', 'failure', 'error'].includes(value)) return 'danger'
  return 'neutral'
}

const resolveSystemError = (error, fallbackKey) => {
  const text = String(systemErrorText(error) ?? '').trim()
  return text ? translateLiteral(text) : t(fallbackKey)
}

const fetchAuditLogs = async () => {
  auditLoading.value = true
  auditError.value = null
  try {
    const params = {
      page: Number(auditFilters.value.page) || 1,
      page_size: Number(auditFilters.value.page_size) || 20
    }
    const action = String(auditFilters.value.action || '').trim()
    const targetType = String(auditFilters.value.target_type || '').trim()
    if (action) params.action = action
    if (targetType) params.target_type = targetType

    const res = ensureSystemSuccess(
      await getSystemAuditLogs(params),
      t('adminSettings.audit.loadFailed')
    )
    const payload = res.data?.data || res.data || {}
    const list = Array.isArray(payload.list) ? payload.list : []
    auditLogs.value = list.map((item) => ({
      id: item?.id ?? null,
      action: item?.action ?? '',
      module: item?.module ?? '',
      target_type: item?.target_type ?? '',
      username: item?.username ?? '',
      content: item?.content ?? '',
      ip: item?.ip ?? '',
      status: item?.status ?? '',
      created_at: item?.created_at ?? ''
    }))
    auditTotal.value = Number(payload.total) || 0
    if (Number.isFinite(Number(payload.page)) && Number(payload.page) > 0) {
      auditFilters.value.page = Number(payload.page)
    }
    if (Number.isFinite(Number(payload.page_size)) && Number(payload.page_size) > 0) {
      auditFilters.value.page_size = Number(payload.page_size)
    }
  } catch (err) {
    const message = resolveSystemError(err, 'adminSettings.audit.loadFailed')
    auditLogs.value = []
    auditTotal.value = 0
    auditError.value = message
  } finally {
    auditLoading.value = false
  }
}

const refreshAuditLogs = async () => {
  await fetchAuditLogs()
}

const applyAuditFilters = async () => {
  clearTimeout(filterTimer)
  auditFilters.value.page = 1
  writeQuery()
  await fetchAuditLogs()
}

function scheduleFilter() {
  clearTimeout(filterTimer)
  filterTimer = setTimeout(applyAuditFilters, 300)
}

function clearAuditFilters() {
  auditFilters.value.action = ''
  auditFilters.value.target_type = ''
  applyAuditFilters()
}

const changeAuditPage = async (page) => {
  const next = Number(page)
  if (!Number.isFinite(next)) return
  const bounded = Math.min(Math.max(1, next), auditTotalPages.value)
  if (bounded === auditFilters.value.page) return
  auditFilters.value.page = bounded
  writeQuery()
  await fetchAuditLogs()
}

const changeAuditPageSize = async (size) => {
  const next = Number(size ?? auditFilters.value.page_size)
  auditFilters.value.page_size = Number.isFinite(next) && next > 0 ? next : 20
  auditFilters.value.page = 1
  writeQuery()
  await fetchAuditLogs()
}

onMounted(fetchAuditLogs)
onBeforeUnmount(() => clearTimeout(filterTimer))
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.audit-page-size {
  flex: 0 0 auto;
  width: 140px;
}

.audit-content {
  display: -webkit-box;
  overflow: hidden;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}

@media (max-width: 639.98px) {
  .audit-page-size {
    width: 100%;
  }
}
</style>
