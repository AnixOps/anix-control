<template>
  <form id="wizard-step-form" class="step-form" novalidate @submit.prevent="handleSubmit">
    <p class="step-intro">{{ t('forwardWizard.steps.tunnel.intro') }}</p>

    <WizardExistingList
      v-if="tunnels.length"
      :items="tunnels.map(tunnel => ({ ...tunnel, title: tunnel.name, meta: tunnelTypeLabel(tunnel.type) }))"
      @use="useExisting"
    />

    <div class="form-grid">
      <UiTextField v-model.trim="form.name" :label="t('runtime.tunnel.fields.name')" :placeholder="t('runtime.tunnel.placeholders.name')" :error="errors.name" maxlength="50" required size="md" />
      <UiSelect
        v-if="fixedType === null"
        :model-value="Number(form.type)"
        :label="t('runtime.tunnel.fields.tunnelType')"
        :options="typeOptions"
        :disabled="!runtimeNodeXMode"
        size="md"
        @update:model-value="value => { form.type = Number(value) }"
      />
      <UiTextField
        :model-value="prefillNodeLabel"
        :label="runtimeNodeXMode ? t('runtime.tunnel.meta.ingressNode') : t('runtime.tunnel.fields.executionNode')"
        :help="t('forwardWizard.steps.tunnel.inheritedNodeHint')"
        readonly
        size="md"
      />
      <UiNumberField
        v-model="form.trafficRatio"
        :label="t('runtime.tunnel.fields.trafficRatio')"
        :min="0.1"
        :max="100"
        :step="0.1"
        :format-options="{ useGrouping: false, maximumFractionDigits: 2 }"
        unit="x"
        size="md"
      />
    </div>

    <div v-if="runtimeNodeXMode && effectiveType === 2" class="form-grid">
      <UiSelect v-model="form.protocol" :label="t('runtime.tunnel.fields.protocol')" :options="protocolOptions" size="md" />
      <UiSelect
        :model-value="form.outNodeId || undefined"
        :label="t('runtime.tunnel.fields.egressNode')"
        :placeholder="t('runtime.tunnel.validation.egressRequired')"
        :options="exitNodeSelectOptions"
        :error="errors.outNodeId"
        required
        size="md"
        @update:model-value="value => { form.outNodeId = Number(value) || 0 }"
      />
    </div>

    <div class="form-grid">
      <UiTextField v-model.trim="form.tcpListenAddr" :label="t('runtime.tunnel.fields.tcpListenAddr')" placeholder="[::]" size="md" />
      <UiTextField v-model.trim="form.udpListenAddr" :label="t('runtime.tunnel.fields.udpListenAddr')" placeholder="[::]" size="md" />
    </div>

    <p v-if="submitError" class="form-error" role="alert">{{ submitError }}</p>
  </form>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import {
  createForwardTunnel,
  getAdminForwardTunnelList,
  getAnsibleMachines,
  getForwardNodes,
  getSystemConfig
} from '@/api/admin'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import WizardExistingList from './WizardExistingList.vue'

const props = defineProps({
  prefillNodeId: { type: Number, required: true },
  prefillNodeLabel: { type: String, default: '' },
  fixedType: { type: Number, default: null }
})

const emit = defineEmits(['created'])

const { t, translateLiteral } = useAppI18n()

const runtimeNodeXMode = ref(false)
const runtimeBackend = ref('nftables_ansible')
const nodes = ref([])
const tunnels = ref([])
const submitLoading = ref(false)
const submitError = ref('')

const exitNodeOptions = computed(() =>
  nodes.value.filter(node => node.id > 0 && String(node.type ?? '').toLowerCase() === 'exit')
)

const effectiveType = computed(() => (props.fixedType === null ? Number(form.type) : Number(props.fixedType)))

const form = reactive({
  name: '',
  type: 1,
  inNodeId: props.prefillNodeId,
  outNodeId: 0,
  flow: 1,
  trafficRatio: 1,
  protocol: 'tls',
  tcpListenAddr: '[::]',
  udpListenAddr: '[::]',
  interfaceName: ''
})
const errors = reactive({ name: '', outNodeId: '' })

function parseBooleanConfig(value) {
  if (value === undefined || value === null) return null
  if (typeof value === 'boolean') return value
  const normalized = String(value).trim().toLowerCase()
  if (!normalized) return null
  return ['1', 'true', 'yes', 'on', 'enabled'].includes(normalized)
}

async function loadRuntimeMode() {
  let explicitMode = null
  try {
    const res = await getSystemConfig('forward.runtime.nodex_mode')
    explicitMode = parseBooleanConfig(res.data?.value)
  } catch {
    // ignore, fall back to backend-derived mode
  }
  try {
    const res = await getSystemConfig('forward.runtime_backend')
    runtimeBackend.value = String(res.data?.value || 'nftables_ansible').toLowerCase() || 'nftables_ansible'
    runtimeNodeXMode.value = explicitMode === null ? runtimeBackend.value === 'gost' : explicitMode
    if (props.fixedType !== null) {
      form.type = Number(props.fixedType)
      if (!(runtimeNodeXMode.value && form.type === 2)) form.protocol = ''
    } else if (!runtimeNodeXMode.value) {
      form.type = 1
      form.protocol = ''
    }
  } catch {
    // keep defaults
  }
}

function normalizeNode(raw) {
  return { ...raw, id: Number(raw.id ?? 0), host: raw.host ?? '', status: Number(raw.status ?? 0) }
}

async function loadNodes() {
  try {
    const response = runtimeNodeXMode.value
      ? await getForwardNodes({ page_size: 200, scope: 'nodex' })
      : await getAnsibleMachines({ page_size: 200, type: 'relay' })
    const list = response?.list ?? response?.data?.list ?? response?.data ?? []
    nodes.value = Array.isArray(list) ? list.map(normalizeNode) : []
  } catch {
    nodes.value = []
  }
}

function normalizeTunnel(raw) {
  return {
    id: Number(raw.id ?? 0),
    name: raw.name || `#${raw.id}`,
    type: Number(raw.type ?? 1)
  }
}

async function loadTunnels() {
  try {
    const res = await getAdminForwardTunnelList()
    tunnels.value = res.code === 0 && Array.isArray(res.data) ? res.data.map(normalizeTunnel) : []
  } catch {
    tunnels.value = []
  }
}

function tunnelTypeLabel(type) {
  return Number(type) === 2 ? t('runtime.tunnel.options.tunnelForward') : t('runtime.tunnel.options.portForward')
}

function useExisting(tunnel) {
  emit('created', { id: tunnel.id, name: tunnel.name })
}

function translateMessage(value, fallback) {
  const text = String(value ?? '').trim()
  if (!text) return fallback
  return translateLiteral(text)
}

function validateForm() {
  errors.name = ''
  errors.outNodeId = ''

  if (!form.name.trim()) {
    errors.name = t('runtime.tunnel.validation.nameRequired')
  } else if (form.name.trim().length < 2 || form.name.trim().length > 50) {
    errors.name = t('runtime.tunnel.validation.nameLength')
  }

  if (runtimeNodeXMode.value && effectiveType.value === 2) {
    if (!form.outNodeId) {
      errors.outNodeId = t('runtime.tunnel.validation.egressRequired')
    } else if (Number(form.outNodeId) === Number(form.inNodeId)) {
      errors.outNodeId = t('runtime.tunnel.validation.ingressEgressDifferent')
    }
  }

  return !errors.name && !errors.outNodeId
}

async function handleSubmit() {
  submitError.value = ''
  if (!validateForm()) return

  submitLoading.value = true
  try {
    const typeValue = runtimeNodeXMode.value ? effectiveType.value : 1
    const normalizedOutNode = runtimeNodeXMode.value
      ? (effectiveType.value === 2 ? Number(form.outNodeId) : null)
      : Number(form.inNodeId) || null
    const normalizedInNode = runtimeNodeXMode.value ? Number(form.inNodeId) : 0
    const protocolValue = runtimeNodeXMode.value && effectiveType.value === 2 ? form.protocol : ''

    const requestPayload = {
      name: form.name.trim(),
      flow: Number(form.flow),
      trafficRatio: Number(form.trafficRatio),
      protocol: protocolValue,
      tcpListenAddr: String(form.tcpListenAddr || '[::]').trim(),
      udpListenAddr: String(form.udpListenAddr || '[::]').trim(),
      interfaceName: String(form.interfaceName || '').trim(),
      type: typeValue,
      inNodeId: normalizedInNode,
      outNodeId: normalizedOutNode
    }

    const response = await createForwardTunnel(requestPayload)
    if (response.code !== 0) {
      submitError.value = translateMessage(response.msg, t('runtime.tunnel.messages.actionFailed'))
      return
    }
    emit('created', { id: Number(response.data?.id || 0), name: requestPayload.name })
  } catch {
    submitError.value = t('runtime.tunnel.messages.actionFailed')
  } finally {
    submitLoading.value = false
  }
}

const typeOptions = computed(() => [
  { value: 1, label: t('runtime.tunnel.options.portForward') },
  { value: 2, label: t('runtime.tunnel.options.tunnelForward') }
])
const protocolOptions = ['tls', 'tcp', 'udp', 'ws', 'wss', 'grpc', 'quic']
const exitNodeSelectOptions = computed(() => exitNodeOptions.value.map(node => ({ value: node.id, label: `${node.name} · ${node.host}` })))

// The wizard footer reads these (UI U7: 上一步 / 下一步 at the bottom).
defineExpose({ submit: handleSubmit, busy: submitLoading, canSubmit: true, primaryLabel: computed(() => t('forwardWizard.steps.tunnel.createAndContinue')) })

onMounted(async () => {
  await loadRuntimeMode()
  form.inNodeId = runtimeNodeXMode.value ? props.prefillNodeId : props.prefillNodeId
  await Promise.all([loadNodes(), loadTunnels()])
})
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
