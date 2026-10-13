<template>
  <div class="settings-section" data-settings-panel="backup">
    <UiSection :title="t('adminSettings.backup.title')" :description="t('adminSettings.backup.description')">
      <template #actions>
        <UiButton
          variant="primary"
          :icon="DatabaseBackup"
          :loading="creating"
          :disabled="!supported"
          data-test="backup-now"
          @click="createNow"
        >{{ t('adminSettings.backup.backupNow') }}</UiButton>
      </template>

      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="loadError"
        compact
        heading-tag="h3"
        :title="t('adminSettings.backup.loadFailed')"
        :error="loadError"
        @retry="load"
      />
      <NodeNotice v-else-if="loaded && !supported" tone="info" data-test="backup-unsupported">
        <strong>{{ t('adminSettings.backup.unsupported') }}</strong>
        <span class="backup-notice__line">{{ t('adminSettings.backup.unsupportedHint') }}</span>
      </NodeNotice>
      <template v-else-if="loaded">
        <dl class="backup-summary" :aria-label="t('adminSettings.backup.summary.label')" data-test="backup-summary">
          <div class="backup-summary__item" data-test="backup-last-verified">
            <dt>{{ t('adminSettings.backup.summary.lastVerified') }}</dt>
            <dd>
              <strong v-if="lastVerified" :title="format.dateTime(lastVerified.created_at)">{{ format.relativeTime(lastVerified.created_at) }}</strong>
              <strong v-else>{{ t('adminSettings.backup.summary.none') }}</strong>
              <UiBadge
                :tone="summary.stale ? 'warning' : 'success'"
                :label="summary.stale ? t('adminSettings.backup.summary.stale', { hours: summary.stale_after_hours || 36 }) : t('adminSettings.backup.summary.current')"
                data-test="backup-freshness"
              />
            </dd>
          </div>
          <div class="backup-summary__item">
            <dt>{{ t('adminSettings.backup.summary.nextDaily') }}</dt>
            <dd><strong>{{ summary.next_daily_at ? format.dateTime(summary.next_daily_at) : '—' }}</strong></dd>
          </div>
          <div class="backup-summary__item">
            <dt>{{ t('adminSettings.backup.summary.directory') }}</dt>
            <dd><code class="backup-summary__path">{{ summary.directory || '—' }}</code></dd>
          </div>
        </dl>

        <UiDataTable
          :columns="columns"
          :rows="backups"
          :label="t('adminSettings.backup.list.title')"
          :row-label="backup => backup.file"
          storage-key="admin.system.backups"
          :page-size="20"
          :empty-icon="Archive"
          :empty-title="t('adminSettings.backup.list.empty')"
          :empty-description="t('adminSettings.backup.list.emptyDescription')"
          state-heading-tag="h3"
          :row-actions="rowActions"
        >
          <template #cell-type="{ row }">{{ typeLabel(row.type) }}</template>
          <template #cell-verify_status="{ row }">
            <UiBadge
              :tone="VERIFY_TONES[row.verify_status] || 'neutral'"
              :label="verifyLabel(row.verify_status)"
              :title="row.verify_error || undefined"
              :data-verify="row.verify_status"
            />
          </template>
        </UiDataTable>
        <p class="backup-retention">{{ retentionText }}</p>
      </template>
    </UiSection>

    <UiSection :title="t('adminSettings.backup.restore.title')" :description="t('adminSettings.backup.restore.description')">
      <UiCodeBlock :label="t('adminSettings.backup.restore.commands')" :code="restoreCommands" max-height="" data-test="backup-restore-commands" />
    </UiSection>
  </div>
</template>

<script setup>
// 系统设置 → 备份 (docs/reference/backups.md): the bundles Control keeps in
// data_dir/backups with their verification, the last verified backup and
// the next automatic one, 立即备份, and a bundle's download (a super
// administrator only: it holds the root key). There is no restore here: a
// restore replaces the database, so it runs on the server with Control
// stopped (`anix-control backup restore`); the page shows the commands.
// Endpoints: GET/POST /api/v5/backups, GET /api/v5/backups/:id/download.
import { computed, onMounted, ref } from 'vue'
import { Archive, DatabaseBackup, Download } from '@lucide/vue'
import { createPanelBackup, downloadPanelBackup, getPanelBackups } from '@/api/kernel'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import NodeNotice from '@/views/admin/nodes/NodeNotice.vue'

const { t, te } = useAppI18n()
const format = useFormat()
const toast = useToast()

const VERIFY_TONES = { passed: 'success', failed: 'danger', pending: 'info' }

const loading = ref(false)
const loaded = ref(false)
const loadError = ref(null)
const supported = ref(true)
const backups = ref([])
const summary = ref({})
const retention = ref({})
const creating = ref(false)
const showSkeleton = useDelayedLoading(() => loading.value && !loaded.value)

const lastVerified = computed(() => summary.value?.last_verified || null)

const typeLabel = type => (te(`adminSettings.backup.types.${type}`) ? t(`adminSettings.backup.types.${type}`) : type || '—')
const verifyLabel = status => (te(`adminSettings.backup.verify.${status}`) ? t(`adminSettings.backup.verify.${status}`) : status || '—')

const columns = computed(() => [
  { key: 'file', label: t('adminSettings.backup.list.file'), primary: true, hideable: false, truncate: true, minWidth: 220, maxWidth: 420 },
  { key: 'type', label: t('adminSettings.backup.list.type'), nowrap: true },
  { key: 'verify_status', label: t('adminSettings.backup.list.verified'), secondary: true, nowrap: true },
  { key: 'size', label: t('adminSettings.backup.list.size'), numeric: true, align: 'end', sortable: true, nowrap: true, format: value => format.bytes(Number(value || 0)) },
  { key: 'created_at', label: t('adminSettings.backup.list.createdAt'), nowrap: true, sortable: true, firstDirection: 'desc', format: value => (value ? format.dateTime(value) : '—') }
])

const rowActions = backup => [
  { key: 'download', label: t('adminSettings.backup.list.download'), icon: Download, onSelect: () => download(backup) }
]

const retentionText = computed(() => t('adminSettings.backup.retention', {
  daily: retention.value.daily ?? 7,
  weekly: retention.value.weekly ?? 4,
  days: retention.value.pre_migration_days ?? 30,
  manual: retention.value.manual ?? 3
}))

const restoreCommands = computed(() => {
  const directory = summary.value?.directory || '/var/lib/anix-control/backups'
  const file = backups.value.find(backup => backup.verify_status === 'passed')?.file || '<bundle>.tar'
  return [
    'sudo systemctl stop anix-control',
    `sudo anix-control backup restore ${directory}/${file} --replace`,
    'sudo systemctl start anix-control'
  ].join('\n')
})

// The kernel API answers { data } (getPanelBackups returns what is in it)
// and { error: { code, message } }.
function errorCode(error) {
  return error?.response?.data?.error?.code || ''
}

function errorMessage(error, fallbackKey) {
  return error?.response?.data?.error?.message || error?.message || t(fallbackKey)
}

async function load() {
  loading.value = true
  loadError.value = null
  try {
    const answer = await getPanelBackups()
    const data = answer && typeof answer === 'object' ? answer : {}
    supported.value = data.supported !== false
    backups.value = Array.isArray(data.backups) ? data.backups : []
    summary.value = data.summary && typeof data.summary === 'object' ? data.summary : {}
    retention.value = data.retention && typeof data.retention === 'object' ? data.retention : {}
    loaded.value = true
  } catch (error) {
    loadError.value = errorMessage(error, 'adminSettings.backup.loadFailed')
  } finally {
    loading.value = false
  }
}

async function createNow() {
  if (creating.value) return
  creating.value = true
  try {
    const answer = await createPanelBackup()
    const file = answer?.backup?.file || ''
    toast.success(t('adminSettings.backup.created', { file }))
    await load()
  } catch (error) {
    toast.error(errorCode(error) === 'backup_in_progress' ? t('adminSettings.backup.busy') : errorMessage(error, 'adminSettings.backup.createFailed'))
  } finally {
    creating.value = false
  }
}

async function download(backup) {
  try {
    const { blob, filename } = await downloadPanelBackup(backup.id)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename || backup.file
    link.click()
    URL.revokeObjectURL(url)
  } catch (error) {
    if (error?.response?.status === 403) {
      toast.error(t('adminSettings.backup.list.superAdminRequired'))
      return
    }
    toast.error(t('adminSettings.backup.list.downloadFailed'))
  }
}

onMounted(load)
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

.backup-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
  gap: var(--space-4);
  margin: 0;
}

.backup-summary__item {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.backup-summary__item dt {
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.backup-summary__item dd {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  margin: 0;
}

.backup-summary__path {
  overflow-wrap: anywhere;
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
}

.backup-retention {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.backup-notice__line {
  display: block;
}
</style>
