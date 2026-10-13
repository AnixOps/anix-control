<template>
  <section class="dialog-section" data-testid="join-nodes" aria-live="polite">
    <h3 class="dialog-section__title">{{ t('admin.nodes.join.nodes.title') }}</h3>

    <p v-if="!loaded" class="join-nodes__waiting" role="status">
      <UiSpinner :size="16" /><span>{{ t('admin.nodes.join.loading') }}</span>
    </p>
    <div v-else-if="nodes.length === 0" class="join-nodes__waiting" role="status" data-testid="join-waiting">
      <UiSpinner :size="16" />
      <span>
        <strong>{{ t('admin.nodes.join.nodes.waiting') }}</strong>
        <span class="join-nodes__hint">{{ t('admin.nodes.join.nodes.waitingHint') }}</span>
      </span>
    </div>
    <template v-else>
      <ul class="join-nodes__list">
        <li v-for="node in shown" :key="node.node" class="join-node" :data-node="node.node" :data-approval="node.approval">
          <span class="join-node__headline">
            <strong>{{ t('admin.nodes.join.nodes.joined', { name: node.name }) }}</strong>
            <UiBadge :tone="node.approval === 'pending' ? 'warning' : 'success'" :dot="false" :label="t(`admin.nodes.approval.${node.approval === 'pending' ? 'pending' : 'approved'}`)" />
            <UiBadge :tone="node.online ? 'success' : 'neutral'" :dot="false" :label="t(node.online ? 'admin.nodes.join.nodes.online' : 'admin.nodes.join.nodes.connecting')" />
          </span>
          <span class="join-node__meta">
            <code>{{ node.node }}</code>
            <span>{{ node.host || '—' }}</span>
            <span v-if="node.agent_version">{{ node.agent_version }}</span>
          </span>
          <span v-if="node.approval === 'pending'" class="join-node__actions">
            <UiButton size="sm" variant="primary" :loading="busy.includes(node.node)" data-testid="join-approve" @click="emit('approve', node)">
              {{ t('admin.nodes.approval.approve') }}
            </UiButton>
            <UiButton size="sm" :disabled="busy.includes(node.node)" data-testid="join-reject" @click="emit('reject', node)">
              {{ t('admin.nodes.approval.reject') }}
            </UiButton>
          </span>
        </li>
      </ul>
      <p v-if="nodes.length > shown.length" class="join-nodes__hint">{{ t('admin.nodes.join.nodes.more', { count: nodes.length - shown.length }) }}</p>
    </template>
  </section>
</template>

<script setup>
// The machines of the selected group, newest first, as the dialog polls them:
// "等待机器接入…" until the first arrives, then "hk-01 已接入" with its node
// id, address, approval and whether it is online. A machine waiting for
// approval carries 批准 and 拒绝.
import { computed } from 'vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiSpinner from '@/ui/UiSpinner.vue'
import { useAppI18n } from '@/composables/useAppI18n'

const SHOWN = 8

const props = defineProps({
  nodes: { type: Array, default: () => [] },
  loaded: { type: Boolean, default: false },
  // Node names (proxy-<id>) with a decision in flight.
  busy: { type: Array, default: () => [] }
})
const emit = defineEmits(['approve', 'reject'])
const { t } = useAppI18n()

const shown = computed(() => props.nodes.slice(0, SHOWN))
</script>

<style scoped>
.join-nodes__waiting {
  display: flex;
  gap: var(--space-3);
  align-items: flex-start;
  margin: 0;
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.join-nodes__waiting :deep(.ui-spinner) {
  margin-top: 2px;
}

.join-nodes__hint {
  display: block;
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.join-nodes__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.join-node {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-4);
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: var(--fill-1);
  font-size: var(--type-callout-size);
}

.join-node__headline,
.join-node__meta,
.join-node__actions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-1) var(--space-2);
  align-items: center;
  min-width: 0;
}

.join-node__meta {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.join-node__meta code {
  font-family: var(--font-mono);
}
</style>
