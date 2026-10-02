<template>
  <UiDialog
    :open="open"
    size="lg"
    :title="editing ? t('runtime.nodeXTopology.nodeModal.titleEdit') : t('runtime.nodeXTopology.nodeModal.titleAdd')"
    :dismissible="!saving"
    @update:open="value => { if (!value) close() }"
  >
    <div v-if="loading" class="fn-dialog-loading" role="status">{{ t('runtime.nodeXTopology.nodeModal.loading') }}</div>
    <template v-else>
      <div class="form-grid">
        <UiTextField
          v-model.trim="form.name"
          required
          :label="t('runtime.nodeXTopology.nodeModal.fields.name')"
          :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.name')"
          :error="errors.name"
        />
        <UiSelect
          v-model="form.type"
          :label="t('runtime.nodeXTopology.nodeModal.fields.type')"
          :options="typeOptions"
        />
        <UiTextField
          v-model.trim="form.host"
          required
          :label="t('runtime.nodeXTopology.nodeModal.fields.host')"
          :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.host')"
          :error="errors.host"
        />
        <UiTextField
          v-model.trim="form.port"
          required
          type="number"
          min="1"
          max="65535"
          :label="t('runtime.nodeXTopology.nodeModal.fields.servicePort')"
          :error="errors.port"
        />
        <UiTextField
          v-model.trim="form.apiPort"
          required
          type="number"
          min="1"
          max="65535"
          :label="t('runtime.nodeXTopology.nodeModal.fields.apiPort')"
          :help="t('runtime.nodeXTopology.nodeModal.hints.apiPort')"
          :error="errors.apiPort"
        />
        <UiTextField
          v-model.trim="form.apiToken"
          autocomplete="off"
          spellcheck="false"
          :label="t('runtime.nodeXTopology.nodeModal.fields.apiToken')"
          :placeholder="form.apiTokenMasked ? t('runtime.nodeXTopology.nodeModal.placeholders.apiTokenKept') : t('runtime.nodeXTopology.nodeModal.placeholders.apiToken')"
          :help="form.apiTokenMasked ? t('runtime.nodeXTopology.nodeModal.hints.apiTokenKept') : ''"
        />
        <UiTextField
          v-model.trim="form.metricsPort"
          type="number"
          min="1"
          max="65535"
          :label="t('runtime.nodeXTopology.nodeModal.fields.metricsPort')"
          :help="t('runtime.nodeXTopology.nodeModal.hints.metricsPort')"
        />
        <UiTextField
          v-model.trim="form.weight"
          type="number"
          min="1"
          :label="t('runtime.nodeXTopology.nodeModal.fields.weight')"
        />
        <UiTextField
          v-model.trim="form.region"
          :label="t('runtime.nodeXTopology.nodeModal.fields.region')"
          :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.region')"
        />
        <UiTextField
          v-model.trim="form.isp"
          :label="t('runtime.nodeXTopology.nodeModal.fields.isp')"
          :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.isp')"
        />
        <UiTextField
          v-model.trim="form.bandwidth"
          type="number"
          min="0"
          :label="t('runtime.nodeXTopology.nodeModal.fields.bandwidth')"
        />
        <UiTextField
          v-model.trim="form.maxConn"
          type="number"
          min="0"
          :label="t('runtime.nodeXTopology.nodeModal.fields.maxConnections')"
        />
      </div>
    </template>
    <template #footer>
      <UiButton :disabled="saving" @click="close">{{ t('runtime.nodeXTopology.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-test="forward-node-submit" :loading="saving" :disabled="loading" @click="submit">
        {{ editing ? t('runtime.nodeXTopology.actions.saveChanges') : t('runtime.nodeXTopology.actions.createNode') }}
      </UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Add or edit a NodeX relay/exit node. Same calls and payload as before
// U7: GET/PUT/POST /admin/forward/nodes(?scope=nodex); an empty token keeps
// the stored one when it came back masked.
import { computed, reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createForwardNode, getForwardNode, updateForwardNode } from '@/api/admin'
import { isMaskedSecret } from '@/constants/secrets'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useToast } from '@/ui/composables/useToast'
import {
  NODEX_SCOPE,
  errorMessage,
  isPort,
  normalizeNode,
  parseNonNegativeInt,
  parsePositiveInt,
  unwrapForwardResponse
} from './forwardNodeModel'

const props = defineProps({
  open: { type: Boolean, default: false },
  // null: add a node; an id: edit that node.
  nodeId: { type: Number, default: null }
})
const emit = defineEmits(['update:open', 'saved'])

const { t, translateLiteral } = useAppI18n()
const toast = useToast()

const loading = ref(false)
const saving = ref(false)
const editing = computed(() => Boolean(props.nodeId))
const form = reactive(emptyForm())
const errors = reactive({ name: '', host: '', port: '', apiPort: '' })

const typeOptions = computed(() => [
  { value: 'relay', label: t('runtime.nodeXTopology.filters.relay') },
  { value: 'exit', label: t('runtime.nodeXTopology.filters.exit') }
])

function emptyForm() {
  return {
    id: null,
    name: '',
    type: 'relay',
    host: '',
    port: '',
    apiPort: '',
    apiToken: '',
    // The node answers mask the stored token (MASKED_SECRET): the field then
    // starts empty, and saving it empty keeps the stored token.
    apiTokenMasked: false,
    metricsPort: '',
    region: '',
    isp: '',
    bandwidth: '',
    weight: '1',
    maxConn: ''
  }
}

function failure(error, key) {
  return errorMessage(error, t(key), translateLiteral)
}

function reset() {
  Object.assign(form, emptyForm())
  Object.assign(errors, { name: '', host: '', port: '', apiPort: '' })
}

function fill(node) {
  const masked = isMaskedSecret(node.apiToken)
  Object.assign(form, {
    id: node.id,
    name: node.name,
    type: node.type,
    host: node.host,
    port: node.port ? String(node.port) : '',
    apiPort: node.apiPort ? String(node.apiPort) : '',
    apiTokenMasked: masked,
    apiToken: masked ? '' : (node.apiToken || ''),
    metricsPort: node.metricsPort ? String(node.metricsPort) : '',
    region: node.region || '',
    isp: node.isp || '',
    bandwidth: node.bandwidth ? String(node.bandwidth) : '',
    weight: node.weight ? String(node.weight) : '1',
    maxConn: node.maxConn ? String(node.maxConn) : ''
  })
}

watch(() => [props.open, props.nodeId], async ([open, nodeId]) => {
  if (!open) return
  reset()
  if (!nodeId) return
  loading.value = true
  try {
    fill(normalizeNode(unwrapForwardResponse(await getForwardNode(nodeId, NODEX_SCOPE), t('runtime.nodeXTopology.validation.requestFailed'))))
  } catch (error) {
    toast.error(failure(error, 'runtime.nodeXTopology.messages.loadNodeDetailFailed'))
    close(true)
  } finally {
    loading.value = false
  }
}, { immediate: true })

function close(force = false) {
  if (!force && saving.value) return
  emit('update:open', false)
}

function validate() {
  errors.name = form.name.trim() ? '' : t('runtime.nodeXTopology.validation.nodeNameRequired')
  errors.host = form.host.trim() ? '' : t('runtime.nodeXTopology.validation.nodeHostRequired')
  errors.port = isPort(form.port) ? '' : t('runtime.nodeXTopology.validation.nodePortRange')
  if (!String(form.apiPort).trim()) errors.apiPort = t('runtime.nodeXTopology.validation.nodeApiPortRequired')
  else errors.apiPort = isPort(form.apiPort) ? '' : t('runtime.nodeXTopology.validation.nodeApiPortRange')
  return !errors.name && !errors.host && !errors.port && !errors.apiPort
}

function buildPayload() {
  const payload = {
    name: form.name.trim(),
    type: form.type,
    host: form.host.trim(),
    port: parsePositiveInt(form.port),
    weight: parsePositiveInt(form.weight) || 1
  }
  const apiPort = parsePositiveInt(form.apiPort)
  const metricsPort = parsePositiveInt(form.metricsPort)
  const bandwidth = parseNonNegativeInt(form.bandwidth)
  const maxConn = parseNonNegativeInt(form.maxConn)
  if (apiPort) payload.api_port = apiPort
  if (form.apiToken.trim()) payload.api_token = form.apiToken.trim()
  if (metricsPort) payload.metrics_port = metricsPort
  if (form.region.trim()) payload.region = form.region.trim()
  if (form.isp.trim()) payload.isp = form.isp.trim()
  if (bandwidth !== null) payload.bandwidth = bandwidth
  if (maxConn !== null) payload.max_conn = maxConn
  return payload
}

async function submit() {
  if (!validate()) return
  saving.value = true
  try {
    const payload = buildPayload()
    if (editing.value && form.id) {
      await updateForwardNode(form.id, payload, NODEX_SCOPE)
      toast.success(t('runtime.nodeXTopology.messages.nodeUpdated'))
    } else {
      const created = unwrapForwardResponse(await createForwardNode(payload, NODEX_SCOPE), t('runtime.nodeXTopology.validation.requestFailed'))
      // A generated token is shown once, in the answer that creates the node.
      const token = String(created?.api_token || '')
      if (!payload.api_token && token && !isMaskedSecret(token)) {
        toast.success(t('runtime.nodeXTopology.messages.nodeCreatedWithToken', { token }))
      } else {
        toast.success(t('runtime.nodeXTopology.messages.nodeCreated'))
      }
    }
    saving.value = false
    close(true)
    emit('saved')
  } catch (error) {
    toast.error(failure(error, 'runtime.nodeXTopology.messages.saveNodeFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.fn-dialog-loading {
  padding: var(--space-6) 0;
  color: var(--label-2);
  text-align: center;
}
</style>
