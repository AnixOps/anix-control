<template>
  <form id="wizard-step-form" class="step-form" novalidate @submit.prevent="submitForm">
    <p class="step-intro">{{ t('forwardWizard.steps.node.intro') }}</p>

    <WizardExistingList
      v-if="nodes.length"
      :items="nodes.map(node => ({ ...node, title: node.name, meta: `${node.host}:${node.port} · ${node.type}` }))"
      @use="useExisting"
    />

    <div class="form-grid">
      <UiTextField v-model.trim="form.name" :label="t('runtime.nodeXTopology.nodeModal.fields.name')" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.name')" :error="errors.name" required size="md" />
      <UiSelect v-model="form.type" :label="t('runtime.nodeXTopology.nodeModal.fields.type')" :options="typeOptions" size="md" />
      <UiTextField v-model.trim="form.host" :label="t('runtime.nodeXTopology.nodeModal.fields.host')" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.host')" :error="errors.host" required size="md" />
      <UiTextField v-model.trim="form.port" type="number" min="1" max="65535" inputmode="numeric" :label="t('runtime.nodeXTopology.nodeModal.fields.servicePort')" :error="errors.port" required size="md" />
      <UiTextField v-model.trim="form.apiPort" type="number" min="1" max="65535" inputmode="numeric" :label="t('runtime.nodeXTopology.nodeModal.fields.apiPort')" :help="t('runtime.nodeXTopology.nodeModal.hints.apiPort')" :error="errors.apiPort" required size="md" />
      <UiTextField v-model.trim="form.apiToken" :label="t('runtime.nodeXTopology.nodeModal.fields.apiToken')" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.apiToken')" autocomplete="off" size="md" />
      <UiTextField v-model.trim="form.metricsPort" type="number" min="1" max="65535" inputmode="numeric" :label="t('runtime.nodeXTopology.nodeModal.fields.metricsPort')" :help="t('runtime.nodeXTopology.nodeModal.hints.metricsPort')" size="md" />
      <UiTextField v-model.trim="form.region" :label="t('runtime.nodeXTopology.nodeModal.fields.region')" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.region')" size="md" />
      <UiTextField v-model.trim="form.isp" :label="t('runtime.nodeXTopology.nodeModal.fields.isp')" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.isp')" size="md" />
    </div>
  </form>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createForwardNode, getForwardNodes } from '@/api/admin'
import { isMaskedSecret } from '@/constants/secrets'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import WizardExistingList from './WizardExistingList.vue'

const emit = defineEmits(['created'])

const { t } = useAppI18n()

const nodeXScopeParams = Object.freeze({ params: { scope: 'nodex' } })

const nodes = ref([])
const saving = ref(false)
const form = reactive({
  name: '',
  type: 'relay',
  host: '',
  port: '',
  apiPort: '',
  apiToken: '',
  metricsPort: '',
  region: '',
  isp: '',
  weight: '1'
})
const errors = reactive({ name: '', host: '', port: '', apiPort: '' })

function unwrapResponse(response) {
  if (!response) return {}
  if (typeof response.code === 'number') {
    if (response.code !== 0) {
      throw new Error(response.msg || t('runtime.nodeXTopology.validation.requestFailed'))
    }
    return response.data ?? response
  }
  if (response.data && typeof response.data === 'object' && !Array.isArray(response.data)) {
    return response.data
  }
  return response
}

function parsePositiveInt(value) {
  const text = String(value ?? '').trim()
  if (!text) return null
  const parsed = Number(text)
  if (!Number.isInteger(parsed) || parsed <= 0) return null
  return parsed
}

async function loadNodes() {
  try {
    const payload = unwrapResponse(await getForwardNodes({ page: 1, page_size: 200, scope: 'nodex' }))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    nodes.value = list.map(item => ({
      id: Number(item?.id || 0),
      name: item?.name || '-',
      host: item?.host || '-',
      port: Number(item?.port || 0),
      type: String(item?.type || 'relay').toLowerCase() === 'exit' ? 'exit' : 'relay'
    }))
  } catch {
    nodes.value = []
  }
}

function useExisting(node) {
  emit('created', { id: node.id, name: node.name, host: node.host, type: node.type })
}

function validateForm() {
  errors.name = ''
  errors.host = ''
  errors.port = ''
  errors.apiPort = ''

  if (!form.name.trim()) {
    errors.name = t('runtime.nodeXTopology.validation.nodeNameRequired')
  }
  if (!form.host.trim()) {
    errors.host = t('runtime.nodeXTopology.validation.nodeHostRequired')
  }
  if (!parsePositiveInt(form.port) || parsePositiveInt(form.port) > 65535) {
    errors.port = t('runtime.nodeXTopology.validation.nodePortRange')
  }
  if (!String(form.apiPort).trim()) {
    errors.apiPort = t('runtime.nodeXTopology.validation.nodeApiPortRequired')
  } else if (!parsePositiveInt(form.apiPort) || parsePositiveInt(form.apiPort) > 65535) {
    errors.apiPort = t('runtime.nodeXTopology.validation.nodeApiPortRange')
  }

  return !errors.name && !errors.host && !errors.port && !errors.apiPort
}

async function submitForm() {
  if (!validateForm()) return

  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      type: form.type,
      host: form.host.trim(),
      port: parsePositiveInt(form.port),
      weight: parsePositiveInt(form.weight) || 1,
      api_port: parsePositiveInt(form.apiPort)
    }
    if (form.apiToken.trim()) payload.api_token = form.apiToken.trim()
    if (parsePositiveInt(form.metricsPort)) payload.metrics_port = parsePositiveInt(form.metricsPort)
    if (form.region.trim()) payload.region = form.region.trim()
    if (form.isp.trim()) payload.isp = form.isp.trim()

    const created = unwrapResponse(await createForwardNode(payload, nodeXScopeParams))
    // A generated token is shown once, in the answer that creates the node.
    const token = String(created?.api_token || '')
    emit('created', {
      id: Number(created?.id || 0),
      name: payload.name,
      host: payload.host,
      type: payload.type,
      apiToken: !payload.api_token && token && !isMaskedSecret(token) ? token : ''
    })
  } catch (error) {
    errors.name = errors.name || (error?.message ? String(error.message) : '')
  } finally {
    saving.value = false
  }
}

const typeOptions = computed(() => [
  { value: 'relay', label: t('runtime.nodeXTopology.filters.relay') },
  { value: 'exit', label: t('runtime.nodeXTopology.filters.exit') }
])

// The wizard footer reads these (UI U7: 上一步 / 下一步 at the bottom).
defineExpose({ submit: submitForm, busy: saving, canSubmit: true, primaryLabel: computed(() => t('forwardWizard.steps.node.createAndContinue')) })

onMounted(loadNodes)
</script>

<style scoped>
.step-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.step-intro {
  margin: 0;
  color: var(--label-2);
}

.form-error {
  margin: 0;
}
</style>
