<template>
  <UiSheet
    :open="open"
    size="lg"
    data-testid="route-mismatch-sheet"
    :title="t('routeModes.mismatches.title')"
    :description="route ? `${route.method} ${route.path}` : ''"
    @update:open="value => emit('update:open', value)"
  >
    <template #header-actions>
      <UiIconButton
        :icon="RefreshCw"
        variant="secondary"
        data-testid="route-mismatch-refresh"
        :label="t('control.actions.refresh')"
        :disabled="loading"
        @click="load()"
      />
    </template>
    <div class="route-mismatch">
      <p class="route-mismatch__note" data-testid="route-mismatch-privacy">
        {{ t('routeModes.mismatches.privacy', { days: retentionDays, max: maxPerRoute }) }}
      </p>
      <code v-if="route" class="route-mismatch__route">{{ route.route_id }}</code>

      <UiDataTable
        :columns="columns"
        :rows="samples"
        row-key="sample_id"
        :row-label="sample => format.dateTime(sample.observed_at)"
        :label="t('routeModes.mismatches.label')"
        :loading="loading"
        :error="error"
        :error-title="t('routeModes.mismatches.loadFailed')"
        :empty-icon="SearchCheck"
        :empty-title="t('routeModes.mismatches.empty')"
        :empty-description="t('routeModes.mismatches.emptyDescription')"
        :settings="false"
        :page-size="10"
        activatable
        flat
        data-testid="route-mismatch-samples"
        @row-activate="sample => (selectedID = sample.sample_id)"
        @retry="load()"
      >
        <template #cell-status="{ row }">
          <span class="route-mismatch__status">
            {{ row.legacy_status }} → <span :class="{ 'is-warning': row.native_status !== row.legacy_status }">{{ row.native_status }}</span>
          </span>
        </template>
        <template #cell-diff="{ row }">
          <UiBadge
            :tone="row.diff?.length ? 'warning' : 'neutral'"
            :dot="false"
            :label="t('routeModes.mismatches.fields', { count: row.diff?.length ?? 0 })"
          />
        </template>
      </UiDataTable>

      <section v-if="selected" class="route-mismatch__detail" data-testid="route-mismatch-detail">
        <h3 class="route-mismatch__heading">{{ t('routeModes.mismatches.detail') }}</h3>
        <dl class="route-mismatch__meta">
          <div>
            <dt>{{ t('routeModes.mismatches.observed') }}</dt>
            <dd>{{ format.dateTime(selected.observed_at) }}</dd>
          </div>
          <div>
            <dt>{{ t('routeModes.mismatches.request') }}</dt>
            <dd><code>{{ selected.method }} {{ selected.path || '—' }}</code></dd>
          </div>
          <div>
            <dt>{{ t('routeModes.mismatches.requestID') }}</dt>
            <dd><code>{{ selected.request_id || '—' }}</code></dd>
          </div>
          <div>
            <dt>{{ t('routeModes.mismatches.version') }}</dt>
            <dd>{{ selected.package_version || '—' }}</dd>
          </div>
        </dl>
        <UiBadge v-if="selected.diff_truncated" tone="warning" :label="t('routeModes.mismatches.truncated')" />
        <UiCodeBlock
          :code="diffText"
          :label="t('routeModes.mismatches.diff')"
          :copy-label="t('routeModes.mismatches.copyDiff')"
          max-height="320px"
          wrap
        />
      </section>
    </div>
  </UiSheet>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { RefreshCw, SearchCheck } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiSheet from '@/ui/UiSheet.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { getKernelRouteModeMismatches } from '@/api/kernel'

const props = defineProps({
  open: { type: Boolean, default: false },
  packageId: { type: String, default: '' },
  // { route_id, method, path }
  route: { type: Object, default: null }
})
const emit = defineEmits(['update:open'])

const SAMPLE_LIMIT = 100

const { t } = useAppI18n()
const format = useFormat()

const samples = ref([])
const loading = ref(false)
const error = ref('')
const selectedID = ref('')
const retentionDays = ref(7)
const maxPerRoute = ref(SAMPLE_LIMIT)
let request = 0

const columns = computed(() => [
  { key: 'time', label: t('routeModes.mismatches.observed'), primary: true, value: row => row.observed_at, format: value => format.dateTime(value), sortable: true, firstDirection: 'desc', nowrap: true },
  { key: 'status', label: t('routeModes.mismatches.status'), value: row => `${row.legacy_status} ${row.native_status}`, nowrap: true },
  { key: 'diff', label: t('routeModes.mismatches.diffColumn'), value: row => row.diff?.length ?? 0, numeric: true, nowrap: true }
])

const selected = computed(() => samples.value.find(sample => sample.sample_id === selectedID.value) || null)
const diffText = computed(() => JSON.stringify(selected.value?.diff ?? [], null, 2))

async function load() {
  if (!props.packageId || !props.route?.route_id) return
  const current = ++request
  loading.value = true
  error.value = ''
  try {
    const data = await getKernelRouteModeMismatches({ packageID: props.packageId, routeID: props.route.route_id, limit: SAMPLE_LIMIT })
    if (current !== request) return
    samples.value = Array.isArray(data?.samples) ? data.samples : []
    if (Number.isFinite(data?.retention_days)) retentionDays.value = data.retention_days
    if (Number.isFinite(data?.max_per_route)) maxPerRoute.value = data.max_per_route
    if (!samples.value.some(sample => sample.sample_id === selectedID.value)) {
      selectedID.value = samples.value[0]?.sample_id || ''
    }
  } catch (cause) {
    if (current !== request) return
    const response = cause?.response?.data
    error.value = response?.error?.message || cause?.message || t('routeModes.mismatches.loadFailed')
  } finally {
    if (current === request) loading.value = false
  }
}

watch(() => [props.open, props.packageId, props.route?.route_id], ([open]) => {
  if (open) load()
}, { immediate: true })

defineExpose({ load })
</script>

<style scoped>
.route-mismatch {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

.route-mismatch__note {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.route-mismatch__route {
  overflow-wrap: anywhere;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.route-mismatch__status {
  font-variant-numeric: tabular-nums;
}

.route-mismatch__status .is-warning {
  color: var(--warning);
}

.route-mismatch__detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}

.route-mismatch__heading {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-body-size);
  font-weight: var(--weight-semibold);
}

.route-mismatch__meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-2) var(--space-4);
  margin: 0;
}

.route-mismatch__meta dt {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.route-mismatch__meta dd {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
