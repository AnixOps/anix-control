<template>
  <UiSection :title="t('admin.nodes.credentials.title')" :description="t('admin.nodes.credentials.description')">
    <div class="node-credentials">
      <UiGroupedList :footer="t('admin.nodes.credentials.auditFooter')">
        <UiGroupedListRow :label="t('admin.nodes.credentials.nodeId')" :value="String(node.id)" />
        <UiGroupedListRow
          :label="t('admin.nodes.credentials.apiKey')"
          :description="apiKey ? t('admin.nodes.credentials.apiKeyShown') : t('admin.nodes.credentials.apiKeyHidden')"
        >
          <UiButton v-if="!apiKey" size="sm" :icon="Eye" :loading="loading" data-testid="reveal-api-key" @click="reveal">{{ t('admin.nodes.credentials.reveal') }}</UiButton>
        </UiGroupedListRow>
      </UiGroupedList>
      <UiCopyField
        v-if="apiKey"
        :value="apiKey"
        :label="t('admin.nodes.credentials.apiKey')"
        :help="t('admin.nodes.credentials.apiKeyHelp')"
        secret
        size="md"
        data-testid="node-api-key"
      />
      <p v-if="error" class="form-error" role="alert" data-testid="credentials-error">{{ error }}</p>

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
// 凭据 section of the node page. The node's API key is read only when asked
// (GET /admin/nodes/:id/credentials writes a "reveal" audit entry), then
// shown masked with a reveal toggle and copy. The shared secret is never
// shown, as before the redesign; protocol secrets stay masked (********)
// in the protocol editor. 轮换 Agent 凭据 revokes what the node's Agent holds
// and issues a one-time credential (NodeRotateCredentials).
import { computed, ref } from 'vue'
import { Eye } from '@lucide/vue'
import { getNodeCredentials } from '@/api/admin'
import UiButton from '@/ui/UiButton.vue'
import UiCopyField from '@/ui/UiCopyField.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSection from '@/ui/UiSection.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeRotateCredentials from './NodeRotateCredentials.vue'
import { NODE_STATUS, readNodeApiError, readNodePayload } from './nodeData'

const props = defineProps({
  node: { type: Object, required: true }
})
const emit = defineEmits(['open-protocols'])
const { t } = useAppI18n()

// A disabled node's Agent credentials were revoked when it was disabled, and
// the route refuses to issue new ones (409 node_disabled): enable it first.
const agentDisabled = computed(() => Number(props.node.status) === NODE_STATUS.disabled)

const apiKey = ref('')
const loading = ref(false)
const error = ref('')

async function reveal() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    const payload = readNodePayload(await getNodeCredentials(props.node.id))
    const key = String(payload?.api_key || '')
    if (!key) throw new Error(t('admin.nodes.credentials.noKey'))
    apiKey.value = key
  } catch (e) {
    error.value = t('admin.nodes.credentials.revealFailed', { message: readNodeApiError(e) })
  } finally {
    loading.value = false
  }
}

defineExpose({ reveal, apiKey })
</script>

<style scoped>
.node-credentials {
  display: flex;
  max-width: 760px;
  flex-direction: column;
  gap: var(--space-6);
}
</style>
