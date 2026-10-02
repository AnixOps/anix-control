<template>
  <nav class="fn-modes" :aria-label="t('forwardNodesPage.modes.label')" data-forward-nodes-modes>
    <ul class="fn-modes__track">
      <li v-for="mode in modes" :key="mode.id">
        <RouterLink
          :to="mode.to"
          class="fn-modes__link"
          :class="{ 'is-current': mode.id === current }"
          :aria-current="mode.id === current ? 'page' : undefined"
          :data-mode="mode.id"
        >
          {{ t(mode.labelKey) }}
        </RouterLink>
      </li>
    </ul>
    <RouterLink to="/admin/forward/agents" class="fn-modes__aside">
      <span>{{ t('forwardSuite.nav.nodeXAgents') }}</span>
      <UiIcon :icon="ArrowUpRight" :size="14" />
    </RouterLink>
  </nav>
</template>

<script setup>
// 转发节点 run-mode switch (UI U7). The four forward-node pages keep their
// own routes (execution-plane boundary, AGENTS.md): this is a set of links
// styled as a segmented control, the current one marked aria-current="page".
// NodeX Agents has its own sidebar item, so it is a plain link at the end.
import { ArrowUpRight } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiIcon from '@/ui/UiIcon.vue'

defineProps({
  // 'nodex' | 'ansible' | 'local' | 'nodexRuntime'
  current: { type: String, default: '' }
})

const { t } = useAppI18n()

const modes = [
  { id: 'nodex', to: '/admin/forward/nodes', labelKey: 'forwardSuite.nav.nodeXTopology' },
  { id: 'ansible', to: '/admin/forward/ansible-machines', labelKey: 'forwardSuite.nav.ansibleMachines' },
  { id: 'local', to: '/admin/forward/local', labelKey: 'forwardSuite.nav.localRuntime' },
  { id: 'nodexRuntime', to: '/admin/forward/nodex', labelKey: 'forwardSuite.nav.nodeXRuntime' }
]
</script>

<style scoped>
.fn-modes {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3) var(--space-5);
  align-items: center;
  min-width: 0;
}

.fn-modes__track {
  display: inline-flex;
  gap: var(--space-0-5);
  max-width: 100%;
  margin: 0;
  padding: var(--space-0-5);
  overflow-x: auto;
  border-radius: var(--radius-pill);
  background: var(--fill-1);
  list-style: none;
  scrollbar-width: none;
}

.fn-modes__track::-webkit-scrollbar {
  display: none;
}

.fn-modes__track li {
  flex: none;
}

.fn-modes__link {
  display: inline-flex;
  align-items: center;
  height: 30px;
  padding: 0 var(--space-4);
  border-radius: var(--radius-pill);
  /* 4.5:1 on the track's fill over any page background (as UiTabs segmented). */
  color: color-mix(in srgb, var(--label-2) 85%, var(--label-1));
  font-size: var(--type-callout-size);
  font-weight: var(--weight-medium);
  text-decoration: none;
  white-space: nowrap;
  transition:
    background-color var(--dur-toggle) var(--ease-standard),
    color var(--dur-toggle) var(--ease-standard);
}

.fn-modes__link:hover {
  color: var(--label-1);
}

.fn-modes__link.is-current {
  background: var(--bg-elevated);
  color: var(--label-1);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.fn-modes__link:focus-visible,
.fn-modes__aside:focus-visible {
  outline: var(--focus-ring);
  outline-offset: 0;
}

.fn-modes__aside {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  border-radius: var(--radius-xs);
  color: var(--accent);
  font-size: var(--type-callout-size);
  text-decoration: none;
}

.fn-modes__aside:hover {
  text-decoration: underline;
}

@media (pointer: coarse) {
  .fn-modes__link {
    height: 40px;
  }
}

@media (forced-colors: active) {
  .fn-modes__link.is-current {
    text-decoration: underline;
  }
}

/* Touch: a 44 px hit area around the link without changing the look. */
@media (pointer: coarse) {
  .fn-modes__link,
  .fn-modes__aside {
    position: relative;
  }

  .fn-modes__link::after,
  .fn-modes__aside::after {
    position: absolute;
    top: 50%;
    left: 50%;
    width: max(100%, var(--size-control-lg));
    height: max(100%, var(--size-control-lg));
    transform: translate(-50%, -50%);
    content: '';
  }
}
</style>
