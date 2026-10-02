<template>
  <form id="wizard-step-form" class="step-form" novalidate @submit.prevent="submitForm">
    <p class="step-intro">{{ t('forwardWizard.steps.machine.intro') }}</p>

    <WizardExistingList
      v-if="machines.length"
      :items="machines.map(machine => ({ ...machine, title: machine.name, meta: `${machine.host}:${machine.port}` }))"
      @use="useExisting"
    />

    <div class="form-grid">
      <UiTextField v-model.trim="form.name" :label="t('runtime.ansibleMachines.fields.name')" :placeholder="t('runtime.ansibleMachines.placeholders.name')" required size="md" />
      <UiTextField v-model.trim="form.host" :label="t('runtime.ansibleMachines.fields.host')" :placeholder="t('runtime.ansibleMachines.placeholders.host')" required size="md" />
      <UiTextField v-model.trim="form.port" type="number" min="1" max="65535" inputmode="numeric" :label="t('runtime.ansibleMachines.fields.reachabilityPort')" required size="md" />
      <UiTextField v-model.trim="form.weight" type="number" min="1" inputmode="numeric" :label="t('runtime.ansibleMachines.fields.weight')" size="md" />
      <UiTextField v-model.trim="form.region" :label="t('runtime.ansibleMachines.fields.region')" :placeholder="t('runtime.ansibleMachines.placeholders.region')" size="md" />
      <UiTextField v-model.trim="form.isp" :label="t('runtime.ansibleMachines.fields.isp')" :placeholder="t('runtime.ansibleMachines.placeholders.isp')" size="md" />
    </div>

    <p v-if="formError" class="form-error" role="alert">{{ formError }}</p>
  </form>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createAnsibleMachine, getAnsibleMachines } from '@/api/admin'
import UiTextField from '@/ui/UiTextField.vue'
import WizardExistingList from './WizardExistingList.vue'

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

// The wizard footer reads these (UI U7: 上一步 / 下一步 at the bottom).
defineExpose({ submit: submitForm, busy: saving, canSubmit: true, primaryLabel: computed(() => t('forwardWizard.steps.machine.createAndContinue')) })

onMounted(loadMachines)
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
