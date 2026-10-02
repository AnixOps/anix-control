<template>
  <div class="notify-panel" data-notify-panel="logs">
    <UiSection :title="t('adminNotify.logs.title')" :description="t('adminNotify.logs.description')">
      <template #actions>
        <UiButton :icon="RotateCw" :loading="logsLoading" data-test="notification-logs-refresh" @click="fetchLogs">{{ t('adminNotify.logs.refresh') }}</UiButton>
      </template>
      <UiDataTable
        :columns="columns"
        :rows="logs"
        :label="t('adminNotify.logs.title')"
        :row-label="log => log.title || `#${log.id}`"
        storage-key="admin.notificationLogs"
        :page-size="20"
        :loading="logsLoading"
        :error="logsError"
        :error-title="t('adminNotify.logs.loadFailed')"
        :filtered="Boolean(logFilter.type || logFilter.status)"
        :empty-icon="History"
        :empty-title="t('adminNotify.logs.empty')"
        :empty-description="t('adminNotify.logs.emptyDescription')"
        state-heading-tag="h3"
        @retry="fetchLogs"
        @clear-filters="clearFilters"
      >
        <template #toolbar>
          <UiFilterChips v-model="logFilter.type" :label="t('adminNotify.logs.filterType')" :options="typeChips" @update:model-value="fetchLogs" />
          <UiFilterChips v-model="logFilter.status" :label="t('adminNotify.logs.filterStatus')" :options="statusChips" @update:model-value="fetchLogs" />
        </template>
        <template #cell-type="{ row }">
          <UiBadge tone="info" :dot="false" :label="typeLabel(row.type)" />
        </template>
        <template #cell-status="{ row }">
          <UiBadge :tone="STATUS_TONES[row.status] || 'danger'" :label="statusLabel(row.status)" />
        </template>
      </UiDataTable>
    </UiSection>
  </div>
</template>

<script setup>
// 通知 → 发送记录: every notification sent, filtered by channel and result
// on the server. Endpoint unchanged: GET /admin/notification/logs
// (type, status).
import { computed, onMounted, ref } from 'vue'
import { History, RotateCw } from '@lucide/vue'
import { getNotificationLogs } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiSection from '@/ui/UiSection.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { LOG_STATUSES, NOTIFICATION_TYPES, notifyErrorText, readNotifyPayload } from './notifyResponse'

const { t } = useAppI18n()
const format = useFormat()

const STATUS_TONES = { success: 'success', pending: 'info', failed: 'danger' }

const logs = ref([])
const logsLoading = ref(false)
const logsError = ref(null)
const logFilter = ref({ type: '', status: '' })

const typeLabel = type => (NOTIFICATION_TYPES.includes(type) ? t(`adminNotify.types.${type}`) : (type || '—'))
const statusLabel = status => (LOG_STATUSES.includes(status) ? t(`adminNotify.status.${status}`) : (status || '—'))

const typeChips = computed(() => NOTIFICATION_TYPES.map(value => ({ value, label: typeLabel(value) })))
const statusChips = computed(() => LOG_STATUSES.map(value => ({ value, label: statusLabel(value) })))

const columns = computed(() => [
  { key: 'title', label: t('adminNotify.logs.subject'), primary: true, value: log => log.title || '—' },
  { key: 'recipient', label: t('adminNotify.logs.recipient'), secondary: true, truncate: true, maxWidth: 240, value: log => log.recipient || '—' },
  { key: 'type', label: t('adminNotify.logs.type'), nowrap: true },
  { key: 'status', label: t('adminNotify.logs.status'), nowrap: true },
  { key: 'created_at', label: t('adminNotify.logs.sentAt'), nowrap: true, sortable: true, firstDirection: 'desc', format: value => (value ? format.dateTime(value) : '—') },
  { key: 'id', label: 'ID', numeric: true, hidden: true }
])

const fetchLogs = async () => {
  logsLoading.value = true
  logsError.value = null
  try {
    const res = await getNotificationLogs(logFilter.value)
    const payload = readNotifyPayload(res, t('adminNotify.logs.loadFailed'))
    logs.value = payload.list || []
  } catch (error) {
    logs.value = []
    logsError.value = notifyErrorText(error) || t('adminNotify.logs.loadFailed')
  } finally {
    logsLoading.value = false
  }
}

function clearFilters() {
  logFilter.value = { type: '', status: '' }
  fetchLogs()
}

onMounted(fetchLogs)
</script>

<style scoped>
.notify-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}
</style>
