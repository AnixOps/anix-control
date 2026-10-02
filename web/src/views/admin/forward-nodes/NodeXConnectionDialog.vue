<template>
  <UiDialog
    :open="open"
    :title="t('runtime.nodeXTopology.connectionModal.title')"
    :dismissible="!testing"
    @update:open="value => { if (!value) close() }"
  >
    <div class="form-grid">
      <UiTextField
        v-model.trim="form.host"
        required
        :label="t('runtime.nodeXTopology.connectionModal.fields.host')"
        :placeholder="t('runtime.nodeXTopology.connectionModal.placeholders.host')"
        :error="errors.host"
      />
      <UiTextField
        v-model.trim="form.apiPort"
        required
        type="number"
        min="1"
        max="65535"
        :label="t('runtime.nodeXTopology.connectionModal.fields.apiPort')"
        :error="errors.apiPort"
      />
      <UiTextField
        v-model.trim="form.apiToken"
        class="form-grid__full"
        autocomplete="off"
        spellcheck="false"
        :label="t('runtime.nodeXTopology.connectionModal.fields.apiToken')"
        :placeholder="form.apiTokenMasked ? t('runtime.nodeXTopology.connectionModal.placeholders.apiTokenHidden') : t('runtime.nodeXTopology.connectionModal.placeholders.apiToken')"
      />
    </div>
    <div
      v-if="result"
      :class="['fn-connection', result.success ? 'is-ok' : 'is-fail']"
      role="status"
      data-test="forward-connection-result"
    >
      <UiBadge :tone="result.success ? 'success' : 'danger'" :label="result.success ? t('runtime.nodeXTopology.connectionModal.success') : t('runtime.nodeXTopology.connectionModal.failed')" />
      <p>{{ result.message }}</p>
      <p v-if="result.serviceCount !== null">{{ t('runtime.nodeXTopology.meta.serviceCount') }}: {{ result.serviceCount }}</p>
    </div>
    <template #footer>
      <UiButton :disabled="testing" @click="close">{{ t('runtime.nodeXTopology.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-test="forward-connection-submit" :loading="testing" @click="submit">
        {{ t('runtime.nodeXTopology.actions.startTest') }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Test a gost management API (POST /admin/forward/nodes/test-connection),
// optionally prefilled from a node. Unchanged request and result.
import { reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { testForwardConnection } from '@/api/admin'
import { isMaskedSecret } from '@/constants/secrets'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useToast } from '@/ui/composables/useToast'
import { errorMessage, isPort, parsePositiveInt, unwrapForwardResponse } from './forwardNodeModel'

const props = defineProps({
  open: { type: Boolean, default: false },
  // A normalized node to prefill host, API port and token, or null.
  node: { type: Object, default: null }
})
const emit = defineEmits(['update:open'])

const { t, translateLiteral } = useAppI18n()
const toast = useToast()

const testing = ref(false)
const result = ref(null)
const form = reactive({ host: '', apiPort: '', apiToken: '', apiTokenMasked: false })
const errors = reactive({ host: '', apiPort: '' })

watch(() => props.open, (open) => {
  if (!open) return
  const node = props.node
  const masked = Boolean(node) && isMaskedSecret(node.apiToken)
  Object.assign(form, {
    host: node?.host && node.host !== '-' ? node.host : '',
    apiPort: node?.apiPort ? String(node.apiPort) : '',
    apiTokenMasked: masked,
    apiToken: masked ? '' : (node?.apiToken || '')
  })
  Object.assign(errors, { host: '', apiPort: '' })
  result.value = null
}, { immediate: true })

function close() {
  if (testing.value) return
  emit('update:open', false)
}

function validate() {
  errors.host = form.host.trim() ? '' : t('runtime.nodeXTopology.validation.connectionHostRequired')
  errors.apiPort = isPort(form.apiPort) ? '' : t('runtime.nodeXTopology.validation.connectionApiPortRange')
  return !errors.host && !errors.apiPort
}

async function submit() {
  if (!validate()) return
  testing.value = true
  try {
    const payload = unwrapForwardResponse(await testForwardConnection({
      host: form.host.trim(),
      api_port: parsePositiveInt(form.apiPort),
      api_token: form.apiToken.trim() || undefined
    }), t('runtime.nodeXTopology.validation.requestFailed'))
    result.value = {
      success: payload.success !== false,
      message:
        translateLiteral(payload.message) ||
        (payload.success === false
          ? t('runtime.nodeXTopology.messages.connectionError')
          : t('runtime.nodeXTopology.messages.connectionSuccess')),
      serviceCount: payload.service_count ?? null
    }
    if (result.value.success) toast.success(result.value.message)
    else toast.error(result.value.message)
  } catch (error) {
    result.value = {
      success: false,
      message: errorMessage(error, t('runtime.nodeXTopology.messages.connectionFailed'), translateLiteral),
      serviceCount: null
    }
    toast.error(result.value.message)
  } finally {
    testing.value = false
  }
}
</script>

<style scoped>
.fn-connection {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: flex-start;
  margin-top: var(--space-5);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--bg-grouped);
}

.fn-connection p {
  margin: 0;
  color: var(--label-1);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
