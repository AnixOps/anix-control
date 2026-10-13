<template>
  <UiSection :title="t('admin.nodes.deploySection.title')" :description="t('admin.nodes.deploySection.description')">
    <div class="node-deploy-section">
      <UiCard :title="t('admin.nodes.deploySection.move')" heading-tag="h3">
        <p class="node-deploy-section__text">{{ t('admin.nodes.deploySection.moveText') }}</p>
        <template #actions>
          <UiButton size="sm" variant="primary" :icon="Terminal" data-testid="open-agent-install" @click="installOpen = true">{{ t('admin.nodes.deploySection.moveAction') }}</UiButton>
        </template>
      </UiCard>

      <UiCard :title="t('admin.nodes.deploySection.certificate')" heading-tag="h3">
        <div class="node-deploy-section__certificate" data-testid="deploy-certificate">
          <NodeCertificateCell :entry="entry" :status="status" />
        </div>
        <NodeNotice v-if="certificateText" :tone="certificateTone" data-testid="deploy-certificate-notice">{{ certificateText }}</NodeNotice>
        <p v-else class="node-deploy-section__text">{{ t('admin.nodes.deploySection.certificateText') }}</p>
      </UiCard>

      <UiCard :title="t('admin.nodes.deploySection.uninstall')" heading-tag="h3">
        <p class="node-deploy-section__text">{{ t('admin.nodes.deploySection.uninstallText') }}</p>
        <UiCodeBlock
          :label="t('admin.nodes.deploySection.uninstallLabel')"
          :code="UNINSTALL_COMMAND"
          :copy-label="t('admin.nodes.deploySection.copy')"
          wrap
          max-height="120px"
          data-testid="uninstall-command"
        />
      </UiCard>
    </div>
    <AgentInstallSheet v-model:open="installOpen" :node="`proxy-${node.id}`" :node-label="node.name" />
  </UiSection>
</template>

<script setup>
// 部署 section of the node page: move this node to another machine (a
// single-use enrollment command bound to the node), the uninstall command, and
// the state of its Agent certificate. New machines are not added here: they
// join a node group from 节点 › 接入节点.
import { computed, onMounted, ref, watch } from 'vue'
import { Terminal } from '@lucide/vue'
import UiButton from '@/ui/UiButton.vue'
import UiCard from '@/ui/UiCard.vue'
import UiCodeBlock from '@/ui/UiCodeBlock.vue'
import UiSection from '@/ui/UiSection.vue'
import AgentInstallSheet from '@/components/admin/AgentInstallSheet.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useFormat } from '@/ui/composables/useFormat'
import NodeCertificateCell from './NodeCertificateCell.vue'
import NodeNotice from './NodeNotice.vue'
import { certificateOf, nodeRef } from './agentConnection'
import { useNodeTransports } from './useNodeTransports'

// What `anix-agent uninstall` removes is listed by the Agent itself; --purge
// also drops its identity and state.
const UNINSTALL_COMMAND = 'sudo anix-agent uninstall'

const props = defineProps({
  node: { type: Object, required: true }
})
const { t } = useAppI18n()
const format = useFormat()
const installOpen = ref(false)
const { entries, status, load } = useNodeTransports()

const entry = computed(() => entries.value.get(nodeRef(props.node.id)) || null)
const certificate = computed(() => certificateOf(entry.value))
// What the state means for the next step; empty for a certificate in order.
const certificateText = computed(() => {
  if (!entry.value) return ''
  const { state, overdue, renewAfter } = certificate.value
  if (overdue) return t('admin.nodes.agent.overdueNotice', { date: format.dateTime(renewAfter) })
  if (state === 'revoked' || state === 'expired') return t('admin.nodes.deploySection.certificateGone')
  if (state === 'none') return t('admin.nodes.deploySection.certificateNone')
  return ''
})
const certificateTone = computed(() => (certificate.value.state === 'valid' ? 'warning' : 'danger'))

onMounted(() => load([props.node.id]))
watch(() => props.node.id, id => load([id]))
</script>

<style scoped>
.node-deploy-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
  max-width: 720px;
}

.node-deploy-section__text {
  margin: 0 0 var(--space-3);
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.node-deploy-section__certificate {
  margin-bottom: var(--space-3);
}
</style>
