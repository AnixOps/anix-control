<template>
  <div class="settings-section" data-settings-panel="backup">
    <UiSection :title="t('adminSettings.backup.auto.title')" :description="t('adminSettings.backup.auto.description')">
      <UiSkeleton v-if="showConfigSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="backupConfigError"
        compact
        heading-tag="h3"
        :title="t('adminSettings.backup.auto.loadFailed')"
        :error="backupConfigError"
        @retry="fetchBackupConfig"
      />
      <template v-else-if="!backupConfigLoading">
        <UiGroupedList>
          <UiGroupedListRow :label="t('adminSettings.backup.auto.enabled')" :description="t('adminSettings.backup.auto.enabledHelp')" label-for="backup-enabled">
            <UiSwitch id="backup-enabled" v-model="backupConfig.enabled" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminSettings.backup.auto.interval')" label-for="backup-interval">
            <UiNumberField
              class="settings-number"
              id="backup-interval"
              v-model="backupConfig.interval"
              size="md"
              :min="1"
              :unit="t('adminSettings.backup.auto.hours')"
              :error="intervalError"
            />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminSettings.backup.auto.keepCount')" :description="t('adminSettings.backup.auto.keepCountHelp')" label-for="backup-keep-count">
            <UiNumberField
              class="settings-number"
              id="backup-keep-count"
              v-model="backupConfig.keep_count"
              size="md"
              :min="1"
              :unit="t('adminSettings.backup.auto.copies')"
              :error="keepCountError"
            />
          </UiGroupedListRow>
        </UiGroupedList>

        <UiGroupedList :title="t('adminSettings.backup.contents.title')">
          <UiGroupedListRow :label="t('adminSettings.backup.contents.database')" label-for="backup-database">
            <UiSwitch id="backup-database" v-model="backupConfig.backup_database" />
          </UiGroupedListRow>
          <UiGroupedListRow :label="t('adminSettings.backup.contents.files')" label-for="backup-files">
            <UiSwitch id="backup-files" v-model="backupConfig.backup_files" />
          </UiGroupedListRow>
        </UiGroupedList>

        <UiGroupedList :title="t('adminSettings.backup.storage.title')">
          <UiGroupedListRow :label="t('adminSettings.backup.storage.type')">
            <UiSegmentedControl
              v-model="backupConfig.storage_type"
              size="sm"
              :aria-label="t('adminSettings.backup.storage.type')"
              :options="storageTypeOptions"
            />
          </UiGroupedListRow>
          <UiGroupedListRow v-if="backupConfig.storage_type === 'local'" stacked>
            <UiTextField
              id="backup-storage-path"
              v-model="backupConfig.storage_path"
              size="md"
              :label="t('adminSettings.backup.storage.path')"
              :placeholder="t('adminSettings.backup.storage.pathPlaceholder')"
              :help="t('adminSettings.backup.storage.pathHelp')"
            />
          </UiGroupedListRow>
          <template v-else>
            <UiGroupedListRow stacked>
              <div class="form-grid backup-s3">
                <UiTextField
                  id="backup-s3-bucket"
                  v-model="backupConfig.s3_bucket"
                  size="md"
                  required
                  :label="t('adminSettings.backup.storage.bucket')"
                  :placeholder="t('adminSettings.backup.storage.bucketPlaceholder')"
                  :error="bucketError"
                />
                <UiTextField
                  id="backup-s3-region"
                  v-model="backupConfig.s3_region"
                  size="md"
                  :label="t('adminSettings.backup.storage.region')"
                  :placeholder="t('adminSettings.backup.storage.regionPlaceholder')"
                />
                <UiTextField
                  id="backup-s3-endpoint"
                  v-model="backupConfig.s3_endpoint"
                  class="form-grid__full"
                  size="md"
                  type="url"
                  :label="t('adminSettings.backup.storage.endpoint')"
                  :placeholder="t('adminSettings.backup.storage.endpointPlaceholder')"
                  :error="endpointError"
                />
                <UiTextField
                  id="backup-s3-access-key"
                  v-model="backupConfig.s3_access_key"
                  size="md"
                  autocomplete="off"
                  :label="t('adminSettings.backup.storage.accessKey')"
                  :placeholder="backupConfig.s3_access_key_has_value ? t('adminSettings.backup.storage.keepCurrent') : ''"
                  :help="sensitiveHelp('s3_access_key')"
                />
                <UiPasswordField
                  id="backup-s3-secret-key"
                  v-model="backupConfig.s3_secret_key"
                  size="md"
                  autocomplete="new-password"
                  :label="t('adminSettings.backup.storage.secretKey')"
                  :help="sensitiveHelp('s3_secret_key')"
                />
              </div>
            </UiGroupedListRow>
          </template>
        </UiGroupedList>
      </template>
    </UiSection>

    <UiSection :title="t('adminSettings.backup.list.title')" :description="t('adminSettings.backup.list.description')">
      <template #actions>
        <UiButton variant="primary" :icon="DatabaseBackup" :loading="creatingBackup" data-test="backup-now" @click="createBackupRequest">{{ t('adminSettings.backup.list.backupNow') }}</UiButton>
      </template>
      <dl class="list-page__summary backup-stats" data-test="backup-stats">
        <div class="stat-item"><dt>{{ t('adminSettings.backup.stats.totalCount') }}</dt><dd class="stat-value"><strong>{{ format.number(backupStats.total_count || 0) }}</strong></dd></div>
        <div class="stat-item"><dt>{{ t('adminSettings.backup.stats.totalSize') }}</dt><dd class="stat-value"><strong>{{ formatSize(backupStats.total_size) }}</strong></dd></div>
        <div class="stat-item"><dt>{{ t('adminSettings.backup.stats.lastBackup') }}</dt><dd class="stat-value"><strong>{{ backupStats.last_backup ? format.dateTime(backupStats.last_backup) : '—' }}</strong></dd></div>
      </dl>
      <UiDataTable
        :columns="backupColumns"
        :rows="backups"
        :label="t('adminSettings.backup.list.title')"
        :row-label="backup => backup.filename"
        storage-key="admin.system.backups"
        :page-size="20"
        :loading="backupsLoading"
        :error="backupsError"
        :error-title="t('adminSettings.backup.list.loadFailed')"
        :empty-icon="Archive"
        :empty-title="t('adminSettings.backup.list.empty')"
        :empty-description="t('adminSettings.backup.list.emptyDescription')"
        state-heading-tag="h3"
        :row-actions="backupActions"
        @retry="fetchBackups"
      >
        <template #cell-status="{ row }">
          <UiBadge :tone="BACKUP_TONES[row.status] || 'neutral'" :label="getStatusLabel(row.status)" />
        </template>
        <template #empty-actions>
          <UiButton variant="primary" :icon="DatabaseBackup" :loading="creatingBackup" @click="createBackupRequest">{{ t('adminSettings.backup.list.backupNow') }}</UiButton>
        </template>
      </UiDataTable>
    </UiSection>

    <SettingsSaveBar
      :visible="backupDirty"
      :saving="backupSaving"
      :invalid="backupInvalid"
      @save="saveBackupConfig"
      @discard="discardBackupConfig"
    />
  </div>
</template>

<script setup>
// 系统设置 → 备份: the automatic backup settings (saved with the save bar),
// the backup statistics and the backups (back up now, restore, delete).
// Endpoints unchanged: GET/PUT /admin/backup/config, POST /admin/backup,
// GET /admin/backups, GET /admin/backup/stats, POST
// /admin/backups/:id/restore, DELETE /admin/backups/:id.
import { computed, onMounted, ref } from 'vue'
import { Archive, DatabaseBackup, History, Trash2 } from '@lucide/vue'
import {
  createBackup,
  deleteBackup,
  getBackupConfig,
  getBackups,
  getBackupStats,
  restoreBackup,
  updateBackupConfig
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import SettingsSaveBar from '@/components/admin/settings/SettingsSaveBar.vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiPasswordField from '@/ui/UiPasswordField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { ensureSystemMutation, ensureSystemSuccess, readBackupPayload, readSystemPayload, systemErrorText } from './systemResponse'

const { t, translateLiteral } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const BACKUP_TONES = { pending: 'info', completed: 'success', failed: 'danger' }

const createBackupConfigForm = (source = {}) => ({
  enabled: Boolean(source.enabled),
  interval: Number.isFinite(Number(source.interval)) && Number(source.interval) > 0
    ? Number(source.interval)
    : Number.isFinite(Number(source.retention_days)) && Number(source.retention_days) > 0
      ? Number(source.retention_days)
      : 24,
  keep_count: Number.isFinite(Number(source.keep_count)) && Number(source.keep_count) > 0
    ? Number(source.keep_count)
    : Number.isFinite(Number(source.retention_days)) && Number(source.retention_days) > 0
      ? Number(source.retention_days)
      : 7,
  backup_database: source.backup_database !== false,
  backup_files: Boolean(source.backup_files),
  storage_type: String(source.storage_type || 'local').trim().toLowerCase() || 'local',
  storage_path: String(source.storage_path || 'backups'),
  s3_bucket: String(source.s3_bucket || ''),
  s3_region: String(source.s3_region || ''),
  s3_endpoint: String(source.s3_endpoint || ''),
  s3_access_key: source.s3_access_key_sensitive ? '' : String(source.s3_access_key || ''),
  s3_secret_key: source.s3_secret_key_sensitive ? '' : String(source.s3_secret_key || ''),
  s3_access_key_display_value: String(source.s3_access_key_display_value || ''),
  s3_secret_key_display_value: String(source.s3_secret_key_display_value || ''),
  s3_access_key_sensitive: Boolean(source.s3_access_key_sensitive),
  s3_secret_key_sensitive: Boolean(source.s3_secret_key_sensitive),
  s3_access_key_has_value: Boolean(source.s3_access_key_has_value),
  s3_secret_key_has_value: Boolean(source.s3_secret_key_has_value)
})

const backupConfig = ref(createBackupConfigForm())
const savedBackupConfig = ref(JSON.stringify(backupConfig.value))
const backupConfigLoading = ref(false)
const backupConfigError = ref(null)
const backupSaving = ref(false)
const showConfigSkeleton = useDelayedLoading(backupConfigLoading)

const backups = ref([])
const backupsLoading = ref(false)
const backupsError = ref(null)
const backupStats = ref({})
const creatingBackup = ref(false)

const storageTypeOptions = computed(() => [
  { value: 'local', label: t('adminSettings.backup.storage.local') },
  { value: 's3', label: t('adminSettings.backup.storage.s3') }
])

const translateText = (value, fallback) => {
  const text = String(value ?? '').trim()
  return text ? translateLiteral(text) : fallback
}
const resolveSystemError = (error, fallbackKey) => translateText(systemErrorText(error), t(fallbackKey))

// Validation as you type ------------------------------------------------------

const positiveInteger = value => Number.isInteger(Number(value)) && Number(value) >= 1 && value !== null && value !== ''
const intervalError = computed(() => (positiveInteger(backupConfig.value.interval) ? '' : t('adminSettings.backup.auto.positiveInteger')))
const keepCountError = computed(() => (positiveInteger(backupConfig.value.keep_count) ? '' : t('adminSettings.backup.auto.positiveInteger')))
const bucketError = computed(() => (
  backupConfig.value.storage_type === 's3' && !String(backupConfig.value.s3_bucket || '').trim()
    ? t('adminSettings.backup.storage.bucketRequired')
    : ''
))
const endpointError = computed(() => {
  const value = String(backupConfig.value.s3_endpoint || '').trim()
  if (backupConfig.value.storage_type !== 's3' || !value) return ''
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' ? '' : t('adminSettings.backup.storage.endpointInvalid')
  } catch {
    return t('adminSettings.backup.storage.endpointInvalid')
  }
})
const backupInvalid = computed(() => Boolean(intervalError.value || keepCountError.value || bucketError.value || endpointError.value))
const backupDirty = computed(() => JSON.stringify(backupConfig.value) !== savedBackupConfig.value)

function sensitiveHelp(field) {
  if (!backupConfig.value[`${field}_sensitive`]) return ''
  return backupConfig.value[`${field}_has_value`]
    ? t('adminSettings.backup.storage.sensitiveWithValue')
    : t('adminSettings.backup.storage.sensitiveWithoutValue')
}

// Payload ---------------------------------------------------------------------

const hasEmptySensitiveBackupField = (form) => {
  if (!form || typeof form !== 'object') return false
  const accessKeyEmpty = Boolean(form.s3_access_key_sensitive && form.s3_access_key_has_value && String(form.s3_access_key || '').trim() === '')
  const secretKeyEmpty = Boolean(form.s3_secret_key_sensitive && form.s3_secret_key_has_value && String(form.s3_secret_key || '').trim() === '')
  return accessKeyEmpty || secretKeyEmpty
}

const buildBackupConfigPayload = (form) => {
  const normalizedStorageType = String(form.storage_type || 'local').trim().toLowerCase() || 'local'
  const preserveExistingSensitive = hasEmptySensitiveBackupField(form)
  return {
    enabled: !!form.enabled,
    auto_backup: !!form.enabled,
    interval: Number.isFinite(Number(form.interval)) && Number(form.interval) > 0 ? Number(form.interval) : 24,
    keep_count: Number.isFinite(Number(form.keep_count)) && Number(form.keep_count) > 0 ? Number(form.keep_count) : 7,
    backup_database: !!form.backup_database,
    backup_files: !!form.backup_files,
    storage_type: normalizedStorageType,
    storage_path: normalizedStorageType === 'local' ? String(form.storage_path || '').trim() : String(form.storage_path || ''),
    s3_bucket: normalizedStorageType === 's3' ? String(form.s3_bucket || '').trim() : String(form.s3_bucket || ''),
    s3_region: normalizedStorageType === 's3' ? String(form.s3_region || '').trim() : String(form.s3_region || ''),
    s3_endpoint: normalizedStorageType === 's3' ? String(form.s3_endpoint || '').trim() : String(form.s3_endpoint || ''),
    s3_access_key: normalizedStorageType === 's3' ? String(form.s3_access_key || '').trim() : String(form.s3_access_key || ''),
    s3_secret_key: normalizedStorageType === 's3' ? String(form.s3_secret_key || '').trim() : String(form.s3_secret_key || ''),
    preserve_existing_sensitive: preserveExistingSensitive
  }
}

const readBackupList = (res, fallbackKey) => {
  const payload = readBackupPayload(res, t(fallbackKey))
  return Array.isArray(payload?.list) ? payload.list : []
}

const formatSize = (bytes) => {
  if (!bytes) return '0 B'
  return format.bytes(Number(bytes))
}

const getStatusLabel = (status) => {
  switch (status) {
    case 'pending': return t('adminSettings.backup.status.pending')
    case 'completed': return t('adminSettings.backup.status.completed')
    case 'failed': return t('adminSettings.backup.status.failed')
    default: return status || '—'
  }
}

const backupColumns = computed(() => [
  { key: 'filename', label: t('adminSettings.backup.list.filename'), primary: true, sortable: true, hideable: false, truncate: true, minWidth: 200, maxWidth: 360 },
  { key: 'status', label: t('adminSettings.backup.list.status'), secondary: true, nowrap: true },
  { key: 'size', label: t('adminSettings.backup.list.size'), numeric: true, align: 'end', sortable: true, nowrap: true, format: value => formatSize(value) },
  { key: 'created_at', label: t('adminSettings.backup.list.createdAt'), nowrap: true, sortable: true, firstDirection: 'desc', format: value => (value ? format.dateTime(value) : '—') },
  { key: 'id', label: 'ID', numeric: true, hidden: true }
])

const backupActions = backup => [
  { key: 'restore', label: t('adminSettings.backup.list.restore'), icon: History, disabled: backup.status !== 'completed', onSelect: () => restoreBackupRequest(backup) },
  { key: 'delete', label: t('common.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteBackupRequest(backup) }
]

// Requests --------------------------------------------------------------------

function commitBackupConfig(form) {
  backupConfig.value = form
  savedBackupConfig.value = JSON.stringify(form)
}

const fetchBackupConfig = async () => {
  backupConfigLoading.value = true
  backupConfigError.value = null
  try {
    const res = await getBackupConfig()
    const payload = readBackupPayload(res, t('adminSettings.backup.auto.loadFailed'))
    if (payload && typeof payload === 'object') {
      commitBackupConfig(createBackupConfigForm(payload))
    }
  } catch (err) {
    backupConfigError.value = resolveSystemError(err, 'adminSettings.backup.auto.loadFailed')
  } finally {
    backupConfigLoading.value = false
  }
}

const saveBackupConfig = async () => {
  if (backupSaving.value || backupInvalid.value) return
  backupSaving.value = true
  try {
    await ensureSystemMutation(
      updateBackupConfig(buildBackupConfigPayload(backupConfig.value)),
      t('adminSettings.backup.auto.saveFailed')
    )
    // Saved secrets are stored now: blank fields keep them from here on.
    const saved = { ...backupConfig.value }
    for (const field of ['s3_access_key', 's3_secret_key']) {
      if (saved[`${field}_sensitive`] && String(saved[field] || '').trim()) {
        saved[`${field}_has_value`] = true
        saved[field] = ''
      }
    }
    commitBackupConfig(saved)
    toast.success(t('adminSettings.backup.auto.saved'))
  } catch (err) {
    toast.error(resolveSystemError(err, 'adminSettings.backup.auto.saveFailed'))
  } finally {
    backupSaving.value = false
  }
}

function discardBackupConfig() {
  backupConfig.value = JSON.parse(savedBackupConfig.value)
}

useUnsavedChanges(backupDirty, { discard: discardBackupConfig })

const fetchBackups = async () => {
  backupsLoading.value = true
  backupsError.value = null
  try {
    const res = await getBackups()
    backups.value = readBackupList(res, 'adminSettings.backup.list.loadFailed')
  } catch (err) {
    backups.value = []
    backupsError.value = resolveSystemError(err, 'adminSettings.backup.list.loadFailed')
  } finally {
    backupsLoading.value = false
  }
}

const readBackupStats = (res) => {
  const payload = readSystemPayload(res, t('adminSettings.backup.stats.loadFailed'))
  return payload && typeof payload === 'object' && !Array.isArray(payload) ? payload : {}
}

const fetchBackupStats = async () => {
  try {
    const res = await getBackupStats()
    backupStats.value = readBackupStats(res)
  } catch (err) {
    // The numbers above the table are a summary; the table shows load errors.
    backupStats.value = {}
    console.error(t('adminSettings.backup.stats.loadFailed'), err)
  }
}

const createBackupRequest = async () => {
  if (creatingBackup.value) return
  creatingBackup.value = true
  try {
    await ensureSystemMutation(createBackup(), t('adminSettings.backup.list.startFailed'))
    toast.success(t('adminSettings.backup.list.started'))
    fetchBackups()
    fetchBackupStats()
  } catch (err) {
    toast.error(resolveSystemError(err, 'adminSettings.backup.list.startFailed'))
  } finally {
    creatingBackup.value = false
  }
}

const confirmBackupAction = (options, request, fallbackKey) => confirm({
  tone: 'danger',
  ...options,
  onConfirm: async () => {
    try {
      ensureSystemSuccess(await request(), t(fallbackKey))
    } catch (err) {
      throw new Error(resolveSystemError(err, fallbackKey))
    }
  }
})

const deleteBackupRequest = async (backup) => {
  const confirmed = await confirmBackupAction({
    title: t('adminSettings.backup.confirm.deleteTitle', { filename: backup.filename }),
    message: t('adminSettings.backup.confirm.deleteMessage'),
    confirmLabel: t('adminSettings.backup.confirm.deleteAction')
  }, () => deleteBackup(backup.id), 'adminSettings.backup.list.deleteFailed')
  if (!confirmed) return
  toast.success(t('adminSettings.backup.list.deleted', { filename: backup.filename }))
  fetchBackups()
  fetchBackupStats()
}

const restoreBackupRequest = async (backup) => {
  const confirmed = await confirmBackupAction({
    title: t('adminSettings.backup.confirm.restoreTitle', { filename: backup.filename }),
    message: t('adminSettings.backup.confirm.restoreMessage'),
    confirmLabel: t('adminSettings.backup.confirm.restoreAction')
  }, () => restoreBackup(backup.id), 'adminSettings.backup.list.restoreFailed')
  if (!confirmed) return
  toast.success(t('adminSettings.backup.list.restored'))
}

onMounted(() => {
  fetchBackupConfig()
  fetchBackups()
  fetchBackupStats()
})
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.settings-section :deep(.ui-section) {
  gap: var(--space-5);
}

.backup-s3 {
  width: 100%;
}

.backup-stats {
  margin: 0;
}

.backup-stats .stat-item {
  display: flex;
  gap: var(--space-1);
}

.backup-stats dd {
  margin: 0;
}

.settings-number {
  width: 160px;
  max-width: 100%;
}
</style>
