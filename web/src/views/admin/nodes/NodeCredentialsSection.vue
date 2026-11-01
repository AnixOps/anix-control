<template>
  <UiSection :title="t('admin.nodes.credentials.title')" :description="t('admin.nodes.credentials.description')">
    <div class="node-credentials">
      <UiGroupedList>
        <UiGroupedListRow :label="t('admin.nodes.credentials.nodeId')" :value="String(node.id)" />
      </UiGroupedList>

      <UiGroupedList :title="t('admin.nodes.rotate.title')" :footer="t('admin.nodes.rotate.footer')">
        <UiGroupedListRow
          :label="t('admin.nodes.rotate.row')"
          :description="agentDisabled ? t('admin.nodes.rotate.disabledHint') : t('admin.nodes.rotate.rowHint')"
        >
          <NodeRotateCredentials :node="`proxy-${node.id}`" :node-label="node.name" :disabled="agentDisabled" />
        </UiGroupedListRow>
      </UiGroupedList>

      <UiGroupedList :title="t('admin.nodes.credentials.protocolSecrets')" :footer="t('admin.nodes.credentials.protocolSecretsFooter')">
        <UiGroupedListRow :label="t('admin.nodes.credentials.protocolSecretsRow')" @click="emit('open-protocols')" />
      </UiGroupedList>
    </div>
  </UiSection>
</template>

<script setup>
// 凭据 section of the node page. A node authenticates with its Agent's mTLS
// certificate only; it has no API key or shared secret. Protocol secrets
// stay masked (********) in the protocol editor. 轮换 Agent 凭据 revokes what
// the node's Agent holds and issues a one-time credential
// (NodeRotateCredentials).
import { computed } from 'vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSection from '@/ui/UiSection.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeRotateCredentials from './NodeRotateCredentials.vue'
import { NODE_STATUS } from './nodeData'

const props = defineProps({
  node: { type: Object, required: true }
})
const emit = defineEmits(['open-protocols'])
const { t } = useAppI18n()

// A disabled node's Agent credentials were revoked when it was disabled, and
// the route refuses to issue new ones (409 node_disabled): enable it first.
const agentDisabled = computed(() => Number(props.node.status) === NODE_STATUS.disabled)
</script>

<style scoped>
.node-credentials {
  display: flex;
  max-width: 760px;
  flex-direction: column;
  gap: var(--space-6);
}
</style>
