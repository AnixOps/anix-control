<template>
  <UiSection :title="title" :description="description" data-test="runtime-jobs">
    <UiDataTable
      :columns="columns"
      :rows="jobs"
      :label="title"
      :row-label="job => `#${job.id} ${job.action}`"
      :storage-key="storageKey"
      :loading="loading"
      :empty-icon="ListChecks"
      :empty-title="t('forwardNodesPage.runtimePage.emptyJobs')"
      :empty-description="emptyText || t('forwardNodesPage.runtimePage.emptyJobsDescription')"
      state-heading-tag="h3"
      :settings="false"
    >
      <template #cell-job="{ row }">
        <span class="rt-job">#{{ row.id }} {{ row.action }}</span>
      </template>
      <template #cell-status="{ row }">
        <UiBadge :status="statusOf(row.status)" :label="statusLabel(row.status)" />
      </template>
      <template #cell-message="{ row }">
        <code v-if="row.message" class="rt-message">{{ translate(row.message) }}</code>
        <span v-else>—</span>
      </template>
    </UiDataTable>
  </UiSection>
</template>

<script setup>
// Latest runtime jobs (GET /admin/forward/runtime/jobs, ten of one backend)
// for the NodeX and local runtime pages (UI U7): a read-only table.
import { computed } from 'vue'
import { ListChecks } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSection from '@/ui/UiSection.vue'
import { useFormat } from '@/ui/composables/useFormat'

defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  jobs: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  emptyText: { type: String, default: '' },
  storageKey: { type: String, default: '' }
})

const { t, translateLiteral } = useAppI18n()
const format = useFormat()

const columns = computed(() => [
  { key: 'job', label: t('forwardNodesPage.runtimePage.columns.job'), primary: true },
  { key: 'status', label: t('forwardNodesPage.runtimePage.columns.status'), secondary: true },
  {
    key: 'target',
    label: t('forwardNodesPage.runtimePage.columns.target'),
    value: job => `forward ${job.forwardId || '-'} / tunnel ${job.tunnelId || '-'} / node ${job.nodeId || '-'}`
  },
  { key: 'time', label: t('forwardNodesPage.runtimePage.columns.time'), nowrap: true, value: job => formatJobTime(job) },
  { key: 'message', label: t('forwardNodesPage.runtimePage.columns.message'), breakpoint: 'md' }
])

function translate(value) {
  return translateLiteral(value) || value
}

function statusOf(status) {
  switch (Number(status)) {
    case 0: return 'pending'
    case 1: return 'pending'
    case 2: return 'online'
    case 3: return 'error'
    default: return 'disabled'
  }
}

function statusLabel(status) {
  switch (Number(status)) {
    case 0: return t('runtime.shared.pending')
    case 1: return t('runtime.shared.running')
    case 2: return t('runtime.shared.success')
    case 3: return t('runtime.shared.failed')
    default: return t('runtime.shared.unknown')
  }
}

function formatJobTime(job) {
  const raw = job?.completedAt || job?.updatedAt || job?.createdAt
  return raw ? format.dateTime(raw) || String(raw) : ''
}
</script>

<style scoped>
.rt-job {
  font-variant-numeric: tabular-nums;
}

.rt-message {
  display: inline-block;
  max-width: 420px;
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
  overflow-wrap: anywhere;
  white-space: normal;
}
</style>
