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
// The node area's sections (节点 / 转发概览 / 路由 / DNS / 分流), one
// network area since the 转发 sidebar item was folded into 节点
// (docs/design/node-unification), plus 反向代理, with the polling line of the forwarding
// pages (D5: "更新于 N 秒前" and refresh). DNS holds the providers of entry
// high availability (L2); 分流 the egress routes between proxy nodes. 概览,
// 路由 and DNS need the forward package's v4 API, as their routes do.
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { RefreshCw } from '@lucide/vue'
import { UiIconButton, UiSegmentedControl } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { adminCapabilities } from '@/extensions/runtime'

defineProps({
  area: { type: String, required: true },
  secondsAgo: { type: Number, default: null },
  loading: { type: Boolean, default: false },
  refreshable: { type: Boolean, default: false }
})
const emit = defineEmits(['refresh'])
const router = useRouter()
const { t } = useAppI18n()
const PATHS = { nodes: '/admin/nodes', overview: '/admin/forward/overview', routes: '/admin/forward/routes', dns: '/admin/forward/dns', egress: '/admin/forward/egress', 'reverse-proxies': '/admin/forward/reverse-proxies' }
const FORWARD = new Set(['overview', 'routes', 'dns'])
const options = computed(() => [
  { value: 'nodes', label: t('forwardV4.nav.nodes') },
  { value: 'overview', label: t('forwardV4.nav.overview') },
  { value: 'routes', label: t('forwardV4.nav.routes') },
  { value: 'dns', label: t('forwardDns.nav') },
  { value: 'egress', label: t('egressRoutes.nav') },
  { value: 'reverse-proxies', label: t('reverseProxies.nav') }
].filter(option => !FORWARD.has(option.value) || adminCapabilities.value.includes('forward.v4')))

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
