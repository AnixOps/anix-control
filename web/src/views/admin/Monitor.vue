<template>
  <div class="monitor-page">
    <UiPageHeader :title="t('adminMonitor.title')" :description="t('adminMonitor.subtitle')" />
    <UiTabs
      class="monitor-page__sections"
      variant="segmented"
      :model-value="section"
      :aria-label="t('adminMonitor.sectionsLabel')"
      :items="sectionOptions"
      data-monitor-sections
      @update:model-value="goTo"
    >
      <template v-for="option in sectionOptions" :key="option.value" #[option.value]>
        <component :is="PANELS[option.value]" />
      </template>
    </UiTabs>
  </div>
</template>

<script setup>
// 流量与监控 (plan §4.2, §8.2): the old 实时监控 (Monitor), 小时流量
// (TrafficHourly) and 转发可观测性 (Observability) pages as sections of one
// page, by dimension — 实时节点 (the live WebSocket fleet), 用户流量
// (hourly traffic of all users or one), 节点延迟 (the prober's node
// targets) and 转发 (topology, ingress comparison, runtime jobs). The
// section is in the path (/admin/monitor/:section; 实时节点 is
// /admin/monitor) and the time range of the traffic and latency sections in
// ?range=. /admin/traffic-hourly and /admin/forward/observability redirect
// here. Sections mount when shown, so the WebSocket is open only on
// 实时节点 and each section loads its own data.
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppI18n } from '@/composables/useAppI18n'
import { ADMIN_PAGE_SECTIONS } from '@/navigation/sections'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiTabs from '@/ui/UiTabs.vue'
import MonitorForward from './monitor/MonitorForward.vue'
import MonitorLatency from './monitor/MonitorLatency.vue'
import MonitorLive from './monitor/MonitorLive.vue'
import MonitorTraffic from './monitor/MonitorTraffic.vue'

const PANELS = { live: MonitorLive, traffic: MonitorTraffic, latency: MonitorLatency, forward: MonitorForward }
const DEFAULT_SECTION = 'live'

const { t } = useAppI18n()
const route = useRoute()
const router = useRouter()

const sectionOptions = computed(() => ADMIN_PAGE_SECTIONS.monitor.map(entry => ({
  value: entry.id.replace(/^monitor-/, ''),
  label: t(entry.labelKey)
})))
const requested = computed(() => String(route.params.section || ''))
const section = computed(() => (PANELS[requested.value] ? requested.value : DEFAULT_SECTION))

// Keep ?range= when switching sections.
function goTo(value) {
  if (value === section.value) return
  const path = value === DEFAULT_SECTION ? '/admin/monitor' : `/admin/monitor/${value}`
  router.push({ path, query: route.query.range ? { range: route.query.range } : {} })
}

watch(requested, value => {
  if (value && !PANELS[value]) router.replace('/admin/monitor')
}, { immediate: true })
</script>

<style scoped>
.monitor-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}
</style>

<style>
/* Shared by the section components in views/admin/monitor/. */
.monitor-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
}

.monitor-section__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
}

.monitor-section__end {
  margin-left: auto;
}

.monitor-section__picker {
  flex: 1 1 220px;
  max-width: 320px;
}

.monitor-section__metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 200px), 1fr));
  gap: var(--space-4);
}

.monitor-section__strong {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.monitor-section__code {
  color: var(--label-2);
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}
</style>
