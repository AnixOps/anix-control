<template>
  <div class="step-form">
    <p class="step-intro">{{ t('forwardWizard.steps.node.intro') }}</p>

    <div v-if="nodes.length" class="existing-list">
      <p class="existing-label">{{ t('forwardWizard.shared.existingLabel') }}</p>
      <ul>
        <li v-for="node in nodes" :key="node.id" class="existing-item">
          <div>
            <strong>{{ node.name }}</strong>
            <span class="existing-meta">{{ node.host }}:{{ node.port }} · {{ node.type }}</span>
          </div>
          <button class="btn btn-secondary btn-sm" @click="useExisting(node)">
            {{ t('forwardWizard.shared.useExisting') }}
          </button>
        </li>
      </ul>
    </div>

    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.name') }}</span>
        <input v-model.trim="form.name" type="text" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.name')" />
        <small v-if="errors.name" class="field-error">{{ errors.name }}</small>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.type') }}</span>
        <select v-model="form.type">
          <option value="relay">{{ t('runtime.nodeXTopology.filters.relay') }}</option>
          <option value="exit">{{ t('runtime.nodeXTopology.filters.exit') }}</option>
        </select>
      </label>
    </div>
    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.host') }}</span>
        <input v-model.trim="form.host" type="text" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.host')" />
        <small v-if="errors.host" class="field-error">{{ errors.host }}</small>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.servicePort') }}</span>
        <input v-model.trim="form.port" type="number" min="1" max="65535" />
        <small v-if="errors.port" class="field-error">{{ errors.port }}</small>
      </label>
    </div>
    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.apiPort') }}</span>
        <input v-model.trim="form.apiPort" type="number" min="1" max="65535" />
        <small v-if="errors.apiPort" class="field-error">{{ errors.apiPort }}</small>
        <small v-else class="field-hint">{{ t('runtime.nodeXTopology.nodeModal.hints.apiPort') }}</small>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.apiToken') }}</span>
        <input v-model.trim="form.apiToken" type="text" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.apiToken')" />
      </label>
    </div>
    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.metricsPort') }}</span>
        <input v-model.trim="form.metricsPort" type="number" min="1" max="65535" />
        <small class="field-hint">{{ t('runtime.nodeXTopology.nodeModal.hints.metricsPort') }}</small>
      </label>
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.region') }}</span>
        <input v-model.trim="form.region" type="text" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.region')" />
      </label>
      <label class="form-group">
        <span>{{ t('runtime.nodeXTopology.nodeModal.fields.isp') }}</span>
        <input v-model.trim="form.isp" type="text" :placeholder="t('runtime.nodeXTopology.nodeModal.placeholders.isp')" />
      </label>
    </div>

    <div class="step-actions">
      <button class="btn btn-primary" :disabled="saving" @click="submitForm">
        {{ saving ? t('runtime.nodeXTopology.nodeModal.saveLoading') : t('forwardWizard.steps.node.createAndContinue') }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createForwardNode, getForwardNodes } from '@/api/admin'

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
    emit('created', {
      id: Number(created?.id || 0),
      name: payload.name,
      host: payload.host,
      type: payload.type
    })
  } catch (error) {
    errors.name = errors.name || (error?.message ? String(error.message) : '')
  } finally {
    saving.value = false
  }
}

onMounted(loadNodes)
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
.field-error { color: #b91c1c; }
.field-hint { color: var(--text-secondary); }
.step-actions { display: flex; justify-content: flex-end; }
.btn { display: inline-flex; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 12px; padding: 10px 16px; cursor: pointer; }
.btn-primary { background: var(--primary-color); color: #fff; }
.btn-secondary { background: var(--surface-color); color: var(--text-color); border-color: var(--border-color); }
.btn-sm { padding: 6px 12px; font-size: 12px; }
@media (max-width: 720px) {
  .form-grid { grid-template-columns: 1fr; }
}
</style>
