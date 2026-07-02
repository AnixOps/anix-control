<template>
  <div class="step-form">
    <p class="step-intro">{{ t('forwardWizard.steps.tunnel.intro') }}</p>

    <div v-if="tunnels.length" class="existing-list">
      <p class="existing-label">{{ t('forwardWizard.shared.existingLabel') }}</p>
      <ul>
        <li v-for="tunnel in tunnels" :key="tunnel.id" class="existing-item">
          <div>
            <strong>{{ tunnel.name }}</strong>
            <span class="existing-meta">{{ tunnelTypeLabel(tunnel.type) }}</span>
          </div>
          <button class="btn btn-secondary btn-sm" @click="useExisting(tunnel)">
            {{ t('forwardWizard.shared.useExisting') }}
          </button>
        </li>
      </ul>
    </div>

    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.tunnel.fields.name') }}</span>
        <input v-model.trim="form.name" type="text" maxlength="50" :placeholder="t('runtime.tunnel.placeholders.name')" />
        <p v-if="errors.name" class="form-error">{{ errors.name }}</p>
      </label>
      <label v-if="fixedType === null" class="form-group">
        <span>{{ t('runtime.tunnel.fields.tunnelType') }}</span>
        <select v-model.number="form.type" :disabled="!runtimeNodeXMode">
          <option :value="1">{{ t('runtime.tunnel.options.portForward') }}</option>
          <option :value="2">{{ t('runtime.tunnel.options.tunnelForward') }}</option>
        </select>
      </label>
    </div>

    <div class="form-grid">
      <label class="form-group">
        <span>{{ runtimeNodeXMode ? t('runtime.tunnel.meta.ingressNode') : t('runtime.tunnel.fields.executionNode') }}</span>
        <select v-model.number="form.inNodeId" disabled>
          <option :value="prefillNodeId">{{ prefillNodeLabel }}</option>
        </select>
        <p class="hint">{{ t('forwardWizard.steps.tunnel.inheritedNodeHint') }}</p>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.tunnel.fields.trafficRatio') }}</span>
        <input v-model.number="form.trafficRatio" type="number" min="0.1" max="100" step="0.1" />
      </label>
    </div>

    <div v-if="runtimeNodeXMode && effectiveType === 2" class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.tunnel.fields.protocol') }}</span>
        <select v-model="form.protocol">
          <option value="tls">tls</option>
          <option value="tcp">tcp</option>
          <option value="udp">udp</option>
          <option value="ws">ws</option>
          <option value="wss">wss</option>
          <option value="grpc">grpc</option>
          <option value="quic">quic</option>
        </select>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.tunnel.fields.egressNode') }}</span>
        <select v-model.number="form.outNodeId">
          <option :value="0">{{ t('runtime.tunnel.validation.egressRequired') }}</option>
          <option v-for="node in exitNodeOptions" :key="node.id" :value="node.id">
            {{ node.name }} · {{ node.host }}
          </option>
        </select>
        <p v-if="errors.outNodeId" class="form-error">{{ errors.outNodeId }}</p>
      </label>
    </div>

    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.tunnel.fields.tcpListenAddr') }}</span>
        <input v-model.trim="form.tcpListenAddr" type="text" placeholder="[::]" />
      </label>
      <label class="form-group">
        <span>{{ t('runtime.tunnel.fields.udpListenAddr') }}</span>
        <input v-model.trim="form.udpListenAddr" type="text" placeholder="[::]" />
      </label>
    </div>

    <p v-if="submitError" class="form-error">{{ submitError }}</p>

    <div class="step-actions">
      <button class="btn btn-primary" :disabled="submitLoading" @click="handleSubmit">
        {{ submitLoading ? t('runtime.tunnel.modal.submitLoading') : t('forwardWizard.steps.tunnel.createAndContinue') }}
      </button>
    </div>
  </div>
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

onMounted(async () => {
  await loadRuntimeMode()
  form.inNodeId = runtimeNodeXMode.value ? props.prefillNodeId : props.prefillNodeId
  await Promise.all([loadNodes(), loadTunnels()])
})
</script>

<style scoped>
.step-form { display: flex; flex-direction: column; gap: 16px; }
.step-intro { margin: 0; color: var(--text-secondary); line-height: 1.6; }
.existing-list { border: 1px solid var(--border-color); border-radius: 14px; padding: 12px 14px; background: var(--bg-color); }
.existing-label { margin: 0 0 8px; font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--text-secondary); }
.existing-list ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
.existing-item { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.existing-meta { margin-left: 8px; color: var(--text-secondary); font-size: 13px; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.form-group { display: flex; flex-direction: column; gap: 8px; }
.form-group input, .form-group select { border: 1px solid var(--border-color); border-radius: 12px; background: var(--bg-color); color: var(--text-color); padding: 12px 14px; }
.hint { margin: 0; font-size: 12px; color: var(--text-secondary); }
.form-error { margin: 0; color: #b91c1c; }
.step-actions { display: flex; justify-content: flex-end; }
.btn { display: inline-flex; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 12px; padding: 10px 16px; cursor: pointer; }
.btn-primary { background: var(--primary-color); color: #fff; }
.btn-secondary { background: var(--surface-color); color: var(--text-color); border-color: var(--border-color); }
.btn-sm { padding: 6px 12px; font-size: 12px; }
@media (max-width: 720px) {
  .form-grid { grid-template-columns: 1fr; }
}
</style>
