<template>
  <div class="step-form">
    <p class="step-intro">{{ t('forwardWizard.steps.machine.intro') }}</p>

    <div v-if="machines.length" class="existing-list">
      <p class="existing-label">{{ t('forwardWizard.shared.existingLabel') }}</p>
      <ul>
        <li v-for="machine in machines" :key="machine.id" class="existing-item">
          <div>
            <strong>{{ machine.name }}</strong>
            <span class="existing-meta">{{ machine.host }}:{{ machine.port }}</span>
          </div>
          <button class="btn btn-secondary btn-sm" @click="useExisting(machine)">
            {{ t('forwardWizard.shared.useExisting') }}
          </button>
        </li>
      </ul>
    </div>

    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.ansibleMachines.fields.name') }}</span>
        <input v-model.trim="form.name" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.name')" />
      </label>
      <label class="form-group">
        <span>{{ t('runtime.ansibleMachines.fields.host') }}</span>
        <input v-model.trim="form.host" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.host')" />
      </label>
    </div>
    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.ansibleMachines.fields.reachabilityPort') }}</span>
        <input v-model.trim="form.port" type="number" min="1" max="65535" />
      </label>
      <label class="form-group">
        <span>{{ t('runtime.ansibleMachines.fields.weight') }}</span>
        <input v-model.trim="form.weight" type="number" min="1" />
      </label>
    </div>
    <div class="form-grid">
      <label class="form-group">
        <span>{{ t('runtime.ansibleMachines.fields.region') }}</span>
        <input v-model.trim="form.region" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.region')" />
      </label>
      <label class="form-group">
        <span>{{ t('runtime.ansibleMachines.fields.isp') }}</span>
        <input v-model.trim="form.isp" type="text" :placeholder="t('runtime.ansibleMachines.placeholders.isp')" />
      </label>
    </div>

    <p v-if="formError" class="form-error">{{ formError }}</p>

    <div class="step-actions">
      <button class="btn btn-primary" :disabled="saving" @click="submitForm">
        {{ saving ? t('runtime.ansibleMachines.modal.saveLoading') : t('forwardWizard.steps.machine.createAndContinue') }}
      </button>
    </div>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createAnsibleMachine, getAnsibleMachines } from '@/api/admin'

const emit = defineEmits(['created'])

const { t, translateLiteral } = useAppI18n()

const machines = ref([])
const saving = ref(false)
const formError = ref('')
const form = reactive({ name: '', host: '', port: '', region: '', isp: '', weight: '1' })

function unwrapResponse(response) {
  return response?.data?.data ?? response?.data ?? response
}

function resolveRuntimeError(error, fallbackKey) {
  const text = String(error?.response?.data?.msg || error?.message || '').trim()
  if (!text) return t(fallbackKey)
  return translateLiteral(text)
}

async function loadMachines() {
  try {
    const payload = unwrapResponse(await getAnsibleMachines({ page: 1, page_size: 200, type: 'relay' }))
    const list = Array.isArray(payload?.list) ? payload.list : Array.isArray(payload) ? payload : []
    machines.value = list.map(item => ({
      id: Number(item?.id || 0),
      name: item?.name || '-',
      host: item?.host || '-',
      port: Number(item?.port || 0)
    }))
  } catch {
    machines.value = []
  }
}

function useExisting(machine) {
  emit('created', { id: machine.id, name: machine.name, host: machine.host, type: 'relay' })
}

async function submitForm() {
  formError.value = ''
  if (!form.name || !form.host || !Number(form.port)) {
    formError.value = t('runtime.ansibleMachines.errors.required')
    return
  }
  const payload = {
    name: form.name.trim(),
    type: 'relay',
    host: form.host.trim(),
    port: Number(form.port),
    weight: Number(form.weight || 1)
  }
  if (form.region.trim()) payload.region = form.region.trim()
  if (form.isp.trim()) payload.isp = form.isp.trim()

  saving.value = true
  try {
    const created = unwrapResponse(await createAnsibleMachine(payload))
    emit('created', {
      id: Number(created?.id || 0),
      name: payload.name,
      host: payload.host,
      type: 'relay'
    })
  } catch (error) {
    formError.value = resolveRuntimeError(error, 'runtime.ansibleMachines.errors.saveFailed')
  } finally {
    saving.value = false
  }
}

onMounted(loadMachines)
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
.form-group input { border: 1px solid var(--border-color); border-radius: 12px; background: var(--bg-color); color: var(--text-color); padding: 12px 14px; }
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
