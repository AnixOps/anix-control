<template>
  <div class="fwd-area-nav">
    <UiSegmentedControl
      :model-value="area"
      :options="options"
      :aria-label="t('forwardV4.nav.label')"
      @update:model-value="go"
    />
    <span v-if="secondsAgo !== null && secondsAgo !== undefined" class="fwd-area-nav__updated" aria-live="off">
      {{ secondsAgo < 5 ? t('forwardV4.updated.now') : t('forwardV4.updated.ago', { n: secondsAgo }) }}
    </span>
    <UiIconButton v-if="refreshable" :icon="RefreshCw" size="sm" :label="t('forwardV4.updated.refresh')" :disabled="loading" @click="emit('refresh')" />
  </div>
</template>

<script setup>
// The forwarding area's sections (概览 / 路由 / 节点 / DNS), like the other
// suites, with the polling line (D5: "更新于 N 秒前" and refresh). DNS holds
// the providers of entry high availability (L2).
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { RefreshCw } from '@lucide/vue'
import { UiIconButton, UiSegmentedControl } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'

defineProps({
  area: { type: String, required: true },
  secondsAgo: { type: Number, default: null },
  loading: { type: Boolean, default: false },
  refreshable: { type: Boolean, default: false }
})
const emit = defineEmits(['refresh'])
const router = useRouter()
const { t } = useAppI18n()
const PATHS = { overview: '/admin/forward/overview', routes: '/admin/forward/routes', nodes: '/admin/forward/inventory', dns: '/admin/forward/dns' }
const options = computed(() => [
  { value: 'overview', label: t('forwardV4.nav.overview') },
  { value: 'routes', label: t('forwardV4.nav.routes') },
  { value: 'nodes', label: t('forwardV4.nav.nodes') },
  { value: 'dns', label: t('forwardDns.nav') }
])

function go(value) {
  if (PATHS[value]) router.push(PATHS[value])
}
</script>

<style scoped>
.fwd-area-nav {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
}

.fwd-area-nav__updated {
  margin-left: auto;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}
</style>
