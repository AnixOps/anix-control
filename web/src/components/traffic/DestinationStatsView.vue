<template>
  <div class="destination-stats" data-destination-stats>
    <section class="destination-stats__metrics" :aria-label="t('portal.destinationStats.summary.label')">
      <UiMetricCard :label="t('portal.destinationStats.summary.total')" :value="format.bytes(total)" :detail="totalDetail" :loading="loading && !summary" data-summary="total" />
      <UiMetricCard :label="t('portal.destinationStats.summary.topCategory')" :value="topCategory ? categoryLabel(topCategory.category) : '—'" :detail="topCategory ? share(topCategory) : ''" :loading="loading && !summary" data-summary="category" />
      <UiMetricCard :label="t('portal.destinationStats.summary.domains')" :value="format.number(domainCount, { empty: '0' })" :detail="t('portal.destinationStats.summary.connections', { count: format.number(connections, { empty: '0' }) })" :loading="loading && !summary" data-summary="domains" />
    </section>

    <UiCard :title="t('portal.destinationStats.categories.title')" :description="t('portal.destinationStats.categories.description')" as="section">
      <LoadError v-if="error && !summary" :title="t('portal.destinationStats.loadFailed')" :error="error" compact @retry="emit('retry')" />
      <UiSkeleton v-else-if="loading && !summary" variant="text" :lines="4" />
      <UiEmptyState v-else-if="!categories.length" :title="emptyTitle || t('portal.destinationStats.empty')" :description="emptyDescription" compact />
      <ul v-else class="destination-stats__categories" data-destination-categories>
        <li v-for="row in categories" :key="row.category" class="destination-stats__category" :data-category="row.category">
          <span class="destination-stats__category-name">
            <span class="destination-stats__swatch" :style="{ background: categoryColor(row.category) }" aria-hidden="true" />
            {{ categoryLabel(row.category) }}
          </span>
          <span class="destination-stats__bar" aria-hidden="true">
            <span class="destination-stats__bar-fill" :style="{ width: `${percent(row)}%`, background: categoryColor(row.category) }" />
          </span>
          <span class="destination-stats__category-value tabular-nums">{{ format.bytes(bytesOf(row)) }} · {{ share(row) }}</span>
        </li>
      </ul>
    </UiCard>

    <UiSection :title="t('portal.destinationStats.domains.title')" :description="t('portal.destinationStats.domains.description')">
      <UiDataTable
        :columns="domainColumns"
        :rows="domainRows"
        row-key="key"
        :label="t('portal.destinationStats.domains.label')"
        :row-label="row => row.label"
        :loading="loading"
        :error="summary ? null : error"
        :error-title="t('portal.destinationStats.loadFailed')"
        :empty-title="emptyTitle || t('portal.destinationStats.empty')"
        :empty-description="emptyDescription"
        :page-size="20"
        :storage-key="storageKey"
        data-destination-domains
        @retry="emit('retry')"
      >
        <template #cell-label="{ row }">
          <span :class="{ 'destination-stats__muted': !row.domain }">{{ row.label }}</span>
        </template>
        <template #cell-category="{ row }">
          <span class="destination-stats__category-name">
            <span class="destination-stats__swatch" :style="{ background: categoryColor(row.category) }" aria-hidden="true" />
            {{ categoryLabel(row.category) }}
          </span>
        </template>
      </UiDataTable>
    </UiSection>
  </div>
</template>

<script setup>
// The traffic of one user (or all) by category and destination domain, as
// GET /api/v4/admin/traffic/destinations and /api/v4/user/traffic/
// destinations answer it (docs/reference/destination-stats.md): three
// figures, the share of each category, and the domains by bytes. The
// parent loads the summary; this only draws it.
import { computed } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import LoadError from '@/components/common/LoadError.vue'
import UiCard from '@/ui/UiCard.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiMetricCard from '@/ui/UiMetricCard.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { CATEGORY_COLORS, bytesOf, readDestinationSummary } from './destinationStats'

const props = defineProps({
  summary: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  error: { type: [Object, String, null], default: null },
  emptyTitle: { type: String, default: '' },
  emptyDescription: { type: String, default: '' },
  storageKey: { type: String, default: '' }
})

const emit = defineEmits(['retry'])

const { t, te } = useAppI18n()
const format = useFormat()

const data = computed(() => readDestinationSummary(props.summary))
const total = computed(() => data.value.upload + data.value.download)
const connections = computed(() => data.value.connections)
const domainCount = computed(() => data.value.domain_count)
const categories = computed(() => data.value.categories)
const topCategory = computed(() => categories.value.find(row => row.category !== 'unknown') || categories.value[0] || null)
const totalDetail = computed(() => t('portal.destinationStats.summary.split', { upload: format.bytes(data.value.upload), download: format.bytes(data.value.download) }))

function categoryLabel(category) {
  const key = `portal.destinationStats.category.${category}`
  return te(key) ? t(key) : category
}

function categoryColor(category) {
  return CATEGORY_COLORS[category] || CATEGORY_COLORS.unknown
}

function percent(row) {
  return total.value > 0 ? Math.max(1, Math.round((bytesOf(row) / total.value) * 100)) : 0
}

function share(row) {
  return total.value > 0 ? format.percent(bytesOf(row) / total.value, { precision: 1 }) : '—'
}

const domainRows = computed(() => data.value.domains.map(row => ({
  ...row,
  key: row.domain || '__other__',
  label: row.domain || t('portal.destinationStats.domains.other'),
  total: bytesOf(row)
})))

const domainColumns = computed(() => [
  { key: 'label', label: t('portal.destinationStats.domains.domain'), primary: true, hideable: false, truncate: true, maxWidth: 360 },
  { key: 'category', label: t('portal.destinationStats.domains.category'), value: row => categoryLabel(row.category) },
  { key: 'total', label: t('portal.destinationStats.domains.total'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => format.bytes(value) },
  { key: 'upload', label: t('portal.destinationStats.domains.upload'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => format.bytes(value) },
  { key: 'download', label: t('portal.destinationStats.domains.download'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => format.bytes(value) },
  { key: 'connections', label: t('portal.destinationStats.domains.connections'), numeric: true, align: 'end', sortable: true, firstDirection: 'desc', format: value => format.number(value, { empty: '0' }) }
])
</script>

<style scoped>
.destination-stats {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.destination-stats__metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-4);
}

.destination-stats__categories {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
  list-style: none;
}

.destination-stats__category {
  display: grid;
  grid-template-columns: minmax(96px, 140px) 1fr auto;
  align-items: center;
  gap: var(--space-3);
}

.destination-stats__category-name {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}

.destination-stats__swatch {
  flex: none;
  width: 10px;
  height: 10px;
  border-radius: var(--radius-xs);
}

.destination-stats__bar {
  height: 8px;
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--fill-1);
}

.destination-stats__bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
}

.destination-stats__category-value {
  color: var(--label-2);
  white-space: nowrap;
}

.destination-stats__muted {
  color: var(--label-2);
}

@media (max-width: 560px) {
  .destination-stats__category {
    grid-template-columns: 1fr auto;
  }

  .destination-stats__bar {
    grid-column: 1 / -1;
    grid-row: 2;
  }
}
</style>
