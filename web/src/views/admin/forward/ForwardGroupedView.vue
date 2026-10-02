<template>
  <div class="forward-groups" data-test="forward-grouped-view">
    <section
      v-for="userGroup in groups"
      :key="userGroup.userKey"
      class="forward-group"
      :aria-labelledby="`forward-group-${userGroup.userKey}`"
    >
      <header class="forward-group__head">
        <div class="forward-group__title-wrap">
          <h2 :id="`forward-group-${userGroup.userKey}`" class="forward-group__title">{{ userGroup.userName }}</h2>
          <p class="forward-group__summary">{{ t('runtime.forward.group.summary', { tunnels: userGroup.tunnelGroups.length, forwards: userGroup.total }) }}</p>
        </div>
        <UiBadge tone="neutral" :dot="false" :label="t('runtime.forward.group.userTag')" />
      </header>

      <details
        v-for="tunnelGroup in userGroup.tunnelGroups"
        :key="`${userGroup.userKey}-${tunnelGroup.tunnelId}`"
        class="forward-tunnel-group"
        open
      >
        <summary class="forward-tunnel-group__summary">
          <UiIcon class="forward-tunnel-group__chevron" :icon="ChevronRight" :size="16" />
          <span class="forward-tunnel-group__name">{{ tunnelGroup.tunnelName }}</span>
          <span class="forward-tunnel-group__meta">{{ t('runtime.forward.group.tunnelMeta', { id: tunnelGroup.tunnelId }) }}</span>
          <UiBadge
            class="forward-tunnel-group__count"
            :tone="tunnelGroup.running === tunnelGroup.forwards.length ? 'success' : 'neutral'"
            :label="t('runtime.forward.group.runningCount', { running: tunnelGroup.running, total: tunnelGroup.forwards.length })"
          />
        </summary>
        <ForwardRulesTable
          :rows="tunnelGroup.forwards"
          :label="`${userGroup.userName} · ${tunnelGroup.tunnelName}`"
          :settings="false"
          hide-tunnel
          flat
          state-heading-tag="h3"
          @edit="forward => emit('edit', forward)"
          @diagnose="forward => emit('diagnose', forward)"
          @delete="forward => emit('delete', forward)"
          @toggle="forward => emit('toggle', forward)"
          @address="payload => emit('address', payload)"
        />
      </details>
    </section>
  </div>
</template>

<script setup>
// Grouped view of the forward rules (flux-panel forward.tsx "分组"): one
// section per user, one collapsible group per tunnel, each a flat table.
import { ChevronRight } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiIcon from '@/ui/UiIcon.vue'
import ForwardRulesTable from './ForwardRulesTable.vue'

defineProps({
  // [{ userKey, userName, total, tunnelGroups: [{ tunnelId, tunnelName, running, forwards }] }]
  groups: { type: Array, default: () => [] }
})

const emit = defineEmits(['edit', 'diagnose', 'delete', 'toggle', 'address'])
const { t } = useAppI18n()
</script>

<style scoped>
.forward-groups {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.forward-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-5);
  border-radius: var(--radius-lg);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.forward-group__head {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  justify-content: space-between;
}

.forward-group__title-wrap {
  min-width: 0;
}

.forward-group__title {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-title-3-line);
  overflow-wrap: anywhere;
}

.forward-group__summary {
  margin: var(--space-1) 0 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.forward-tunnel-group {
  border-top: 0.5px solid var(--separator);
}

.forward-tunnel-group__summary {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-3);
  align-items: center;
  min-height: 44px;
  padding: var(--space-2) var(--space-1);
  border-radius: var(--radius-sm);
  list-style: none;
  cursor: pointer;
}

.forward-tunnel-group__summary::-webkit-details-marker {
  display: none;
}

.forward-tunnel-group__summary:hover {
  background: var(--fill-1);
}

.forward-tunnel-group__summary:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.forward-tunnel-group__chevron {
  color: var(--label-2);
  transition: transform var(--dur-toggle) var(--ease-standard);
}

.forward-tunnel-group[open] .forward-tunnel-group__chevron {
  transform: rotate(90deg);
}

.forward-tunnel-group__name {
  color: var(--label-1);
  font-weight: var(--weight-semibold);
  overflow-wrap: anywhere;
}

.forward-tunnel-group__meta {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.forward-tunnel-group__count {
  margin-inline-start: auto;
}

.forward-tunnel-group[open] {
  padding-bottom: var(--space-3);
}

@media (max-width: 639.98px) {
  .forward-group {
    padding: var(--space-4);
  }
}

@media (prefers-reduced-motion: reduce) {
  .forward-tunnel-group__chevron {
    transition: none;
  }
}
</style>
