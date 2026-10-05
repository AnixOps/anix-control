<template>
  <div class="node-agent" data-testid="node-agent">
    <NodeNotice v-if="certificate.overdue" tone="warning" data-testid="node-certificate-overdue">
      {{ t('admin.nodes.agent.overdueNotice', { date: format.dateTime(certificate.renewAfter) }) }}
    </NodeNotice>
    <NodeNotice v-else-if="entry && certificate.state === 'revoked'" tone="danger" data-testid="node-certificate-revoked">
      {{ t('admin.nodes.agent.revokedNotice') }}
    </NodeNotice>

    <UiGroupedList :title="t('admin.nodes.agent.title')" heading-tag="h2" :footer="footer">
      <UiGroupedListRow v-if="status === 'error'" :label="t('admin.nodes.agent.loadFailed')" data-testid="node-agent-error">
        <UiButton size="sm" :icon="RotateCw" data-testid="node-agent-retry" @click="load">{{ t('admin.nodes.agent.retry') }}</UiButton>
      </UiGroupedListRow>
      <UiGroupedListRow v-else-if="!entry" :label="status === 'ready' ? t('admin.nodes.agent.noRecord') : t('admin.nodes.agent.loading')" :description="status === 'ready' ? t('admin.nodes.agent.noRecordHint') : ''" data-testid="node-agent-empty" />
      <template v-else>
        <UiGroupedListRow :label="t('admin.nodes.agent.connectionLabel')">
          <template #value><NodeConnectionBadge :entry="entry" status="ready" /></template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.agent.transport')" :value="transportLabel" />
        <UiGroupedListRow :label="t('admin.nodes.agent.lastSeen')">
          <template #value>
            <time v-if="lastSeen" :datetime="lastSeen" :title="format.dateTime(lastSeen)">{{ format.relativeTime(lastSeen) }}</time>
            <span v-else>{{ t('admin.nodes.agent.neverSeen') }}</span>
          </template>
        </UiGroupedListRow>
        <UiGroupedListRow :label="t('admin.nodes.agent.certificateLabel')">
          <template #value>
            <UiBadge :tone="certificate.tone" :dot="false" :label="t(`admin.nodes.agent.certificate.${certificate.overdue ? 'overdue' : certificate.state}`)" data-testid="node-certificate-state" />
          </template>
        </UiGroupedListRow>
        <template v-if="certificate.state !== 'none'">
          <UiGroupedListRow :label="t('admin.nodes.agent.notAfter')" :value="format.dateTime(certificate.notAfter)" data-testid="node-certificate-not-after" />
          <UiGroupedListRow :label="t('admin.nodes.agent.renewAfter')" :value="format.dateTime(certificate.renewAfter)" data-testid="node-certificate-renew-after" />
          <UiGroupedListRow v-if="certificate.revokedAt" :label="t('admin.nodes.agent.revokedAt')" :value="format.dateTime(certificate.revokedAt)" data-testid="node-certificate-revoked-at" />
          <UiGroupedListRow v-if="certificate.revokeReason" :label="t('admin.nodes.agent.revokeReason')" :value="certificate.revokeReason" data-testid="node-certificate-revoke-reason" />
        </template>
      </template>
    </UiGroupedList>
  </div>
</template>

<script setup>
// 概览 → Agent connection: how this node's Agent reaches Control now (mTLS
// stream, API key stream, legacy, third-party, offline) and its newest Agent
// certificate in any state (ends, renews, revoked and why), from
// GET /api/v4/kernel/agents/transports?node=proxy-<id>. It loads after the
// node page and never holds it up: a failure is a row with 重试.
import { computed, onMounted, watch } from 'vue'
import { RotateCw } from '@lucide/vue'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeConnectionBadge from './NodeConnectionBadge.vue'
import NodeNotice from './NodeNotice.vue'
import { certificateOf, nodeRef } from './agentConnection'
import { useNodeTransports } from './useNodeTransports'

const props = defineProps({
  nodeId: { type: [Number, String], required: true }
})
const { t, te } = useAppI18n()
const format = useFormat()
const { entries, status, load: loadTransports } = useNodeTransports()

const entry = computed(() => entries.value.get(nodeRef(props.nodeId)) || null)
const certificate = computed(() => certificateOf(entry.value))
const lastSeen = computed(() => entry.value?.connection?.last_seen_at || null)
const transportLabel = computed(() => {
  const name = entry.value?.connection?.transport
  if (!name) return '—'
  return te(`agentTransports.transports.${name}`) ? t(`agentTransports.transports.${name}`) : name
})
const footer = computed(() => (entry.value ? t('admin.nodes.agent.footer') : ''))

function load() {
  return loadTransports([props.nodeId])
}

onMounted(load)
watch(() => props.nodeId, load)

defineExpose({ load })
</script>

<style scoped>
.node-agent {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 0;
}
</style>
