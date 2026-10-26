<template>
  <div class="traffic-page">
    <UiPageHeader :title="t('portal.traffic.title')" :description="t('portal.traffic.description')" />

    <div class="traffic-page__toolbar">
      <UiSegmentedControl v-model="range" :options="rangeOptions" :aria-label="t('portal.traffic.rangeLabel')" data-traffic-range />
      <p v-if="summary" class="traffic-page__note" role="status" data-traffic-note>
        {{ settings.enabled ? t('portal.traffic.retention', { days: settings.retention_days }) : t('portal.traffic.off') }}
      </p>
    </div>

    <DestinationStatsView
      :summary="summary"
      :loading="loading"
      :error="loadError"
      :empty-title="t('portal.traffic.empty')"
      :empty-description="t('portal.traffic.emptyHint')"
      storage-key="user.traffic.destinations"
      @retry="load"
    />
  </div>
</template>

<script setup>
// 访问统计: the signed-in user's own traffic by destination domain and
// category (GET /api/v4/user/traffic/destinations), so each user can see
// what the nodes record about them and for how long. Late answers of an
// earlier range are ignored.
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { getMyTrafficDestinations } from '@/api/user'
import { useAppI18n } from '@/composables/useAppI18n'
import DestinationStatsView from '@/components/traffic/DestinationStatsView.vue'
import { readDestinationSummary } from '@/components/traffic/destinationStats'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'

const RANGES = Object.freeze({ '24h': 24, '7d': 168, '30d': 720 })

const { t } = useAppI18n()

const range = ref('24h')
const summary = ref(null)
const loading = ref(false)
const loadError = ref(null)

let isActive = true
let requestSeq = 0

const rangeOptions = computed(() => Object.keys(RANGES).map(value => ({ value, label: t(`portal.traffic.range.${value}`) })))
const settings = computed(() => readDestinationSummary(summary.value).settings)

async function load() {
  const seq = ++requestSeq
  loading.value = true
  loadError.value = null
  try {
    const answer = await getMyTrafficDestinations({ hours: RANGES[range.value], limit: 200 })
    if (!isActive || seq !== requestSeq) return
    summary.value = answer
  } catch (err) {
    if (!isActive || seq !== requestSeq) return
    loadError.value = err
    summary.value = null
  } finally {
    if (isActive && seq === requestSeq) loading.value = false
  }
}

watch(range, () => { void load() })

onMounted(() => { void load() })

onUnmounted(() => {
  isActive = false
})
</script>

<style scoped>
.traffic-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.traffic-page__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3) var(--space-4);
}

.traffic-page__note {
  margin: 0;
  color: var(--label-2);
}
</style>
