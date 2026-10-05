<template>
  <span v-if="status === 'error'" class="node-agent-muted" data-testid="node-connection-unavailable">{{ t('admin.nodes.agent.unavailable') }}</span>
  <UiBadge
    v-else-if="entry"
    :tone="connectionTone(type)"
    :dot="false"
    :label="t(`admin.nodes.agent.connection.${type}`)"
    :title="t(`admin.nodes.agent.connectionHint.${type}`)"
    data-testid="node-connection"
    :data-connection="type"
  />
  <span v-else class="node-agent-muted" data-testid="node-connection-empty">
    <span aria-hidden="true">—</span><span v-if="status === 'loading'" class="visually-hidden">{{ t('admin.nodes.agent.loading') }}</span>
  </span>
</template>

<script setup>
// The connection-type chip of a node: mTLS stream, API key stream, legacy,
// third-party or offline. Until the inventory answers (or when it has no row
// for the node) the cell is a dash; when the request failed it says so.
import { computed } from 'vue'
import UiBadge from '@/ui/UiBadge.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { connectionTone, connectionTypeOf } from './agentConnection'

const props = defineProps({
  entry: { type: Object, default: null },
  status: { type: String, default: 'idle' }
})
const { t } = useAppI18n()
const type = computed(() => connectionTypeOf(props.entry))
</script>

<style scoped>
.node-agent-muted {
  color: var(--label-2);
}
</style>
