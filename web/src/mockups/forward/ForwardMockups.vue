<template>
  <div class="fwd-mock">
    <nav class="fwd-mock__bar" data-mockup-chrome aria-label="设计稿页面">
      <UiBadge tone="info" label="F5b 设计稿 · 模拟数据" />
      <RouterLink
        v-for="link in links"
        :key="link.label"
        class="fwd-mock__link"
        :to="link.to"
        :aria-current="isCurrent(link) ? 'page' : undefined"
      >{{ link.label }}</RouterLink>
    </nav>

    <div class="fwd-mock__nav">
      <UiSegmentedControl
        :model-value="area"
        :options="areas"
        aria-label="转发"
        @update:model-value="goArea"
      />
      <span class="fwd-mock__stamp">设计稿 · 不调用后端</span>
    </div>

    <component :is="screenComponent" :key="route.fullPath" />
  </div>
</template>

<script setup>
// F5b forwarding UI mockups (gate H16): one dev-only route that renders each
// proposed screen with mocked /api/v4/forward data. Not the final UI; see
// docs/design/forward-ui/README.md.
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { UiBadge, UiSegmentedControl } from '@/ui'
import OverviewScreen from './screens/OverviewScreen.vue'
import RoutesScreen from './screens/RoutesScreen.vue'
import RouteEditorScreen from './screens/RouteEditorScreen.vue'
import RouteDetailScreen from './screens/RouteDetailScreen.vue'
import NodesScreen from './screens/NodesScreen.vue'
import NodeDetailScreen from './screens/NodeDetailScreen.vue'

const BASE = '/admin/__mockups/forward'
const route = useRoute()
const router = useRouter()

const SCREENS = {
  overview: { component: OverviewScreen, area: 'overview' },
  routes: { component: RoutesScreen, area: 'routes' },
  route: { component: RouteDetailScreen, area: 'routes' },
  editor: { component: RouteEditorScreen, area: 'routes' },
  nodes: { component: NodesScreen, area: 'nodes' },
  node: { component: NodeDetailScreen, area: 'nodes' }
}

const screen = computed(() => (SCREENS[route.params.screen] ? route.params.screen : 'overview'))
const screenComponent = computed(() => SCREENS[screen.value].component)
const area = computed(() => SCREENS[screen.value].area)
const areas = [
  { value: 'overview', label: '概览' },
  { value: 'routes', label: '路由' },
  { value: 'nodes', label: '节点' }
]

function goArea(value) {
  router.push(`${BASE}/${value}`)
}

const links = [
  { label: '1 路由列表', to: `${BASE}/routes` },
  { label: '1b 空状态', to: `${BASE}/routes?state=empty` },
  { label: '2 编辑器（有问题）', to: `${BASE}/editor` },
  { label: '2b 编辑器（可保存）', to: `${BASE}/editor?state=valid` },
  { label: '3 路由详情', to: `${BASE}/route` },
  { label: '4 节点列表', to: `${BASE}/nodes` },
  { label: '4b 节点详情', to: `${BASE}/node` },
  { label: '5 概览', to: `${BASE}/overview` }
]

function isCurrent(link) {
  return router.resolve(link.to).fullPath === route.fullPath
}
</script>

<style>
/* Shared by the mockup screens (dev only). */
.fwd-mock {
  display: grid;
  gap: var(--space-5);
  min-width: 0;
}

.fwd-mock__bar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  align-items: center;
  padding: var(--space-2) var(--space-3);
  border: 1px dashed var(--separator-strong);
  border-radius: var(--radius-sm);
  font-size: var(--type-caption-size);
}

.fwd-mock__link {
  color: var(--accent);
  text-decoration: none;
}

.fwd-mock__link[aria-current='page'] {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
}

.fwd-mock__nav {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  justify-content: space-between;
}

.fwd-mock__stamp {
  color: var(--label-3);
  font-size: var(--type-caption-size);
}

.fwd-screen {
  display: grid;
  gap: var(--space-5);
  min-width: 0;
}

.fwd-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-4);
}

.fwd-two {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--space-4);
}

.fwd-muted {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.fwd-mono {
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

.fwd-cell-stack {
  display: grid;
  gap: 2px;
  min-width: 0;
}

.fwd-note {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.fwd-note.is-warning {
  background: var(--warning-soft);
  color: var(--label-1);
}

.fwd-note.is-danger {
  background: var(--danger-soft);
  color: var(--label-1);
}

.fwd-list {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.fwd-list__item {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  justify-content: space-between;
  padding: var(--space-3) 0;
  border-bottom: 1px solid var(--separator);
}

.fwd-list__item:last-child {
  border-bottom: 0;
}

@media (max-width: 833.98px) {
  .fwd-two {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
