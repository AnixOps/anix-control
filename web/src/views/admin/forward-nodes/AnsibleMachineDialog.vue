<template>
  <UiDialog
    :open="open"
    :title="editing ? t('runtime.ansibleMachines.modal.titleEdit') : t('runtime.ansibleMachines.modal.titleAdd')"
    :description="t('runtime.ansibleMachines.inventoryHint')"
    :dismissible="!saving"
    @update:open="value => { if (!value) close() }"
  >
    <div class="form-grid">
      <UiTextField v-model.trim="form.name" required :label="t('runtime.ansibleMachines.fields.name')" :placeholder="t('runtime.ansibleMachines.placeholders.name')" />
      <UiTextField v-model.trim="form.host" required :label="t('runtime.ansibleMachines.fields.host')" :placeholder="t('runtime.ansibleMachines.placeholders.host')" />
      <UiTextField v-model.trim="form.port" required type="number" min="1" max="65535" :label="t('runtime.ansibleMachines.fields.reachabilityPort')" :help="t('runtime.ansibleMachines.fields.reachabilityHelp')" />
      <UiTextField v-model.trim="form.weight" type="number" min="1" :label="t('runtime.ansibleMachines.fields.weight')" />
      <UiTextField v-model.trim="form.region" :label="t('runtime.ansibleMachines.fields.region')" :placeholder="t('runtime.ansibleMachines.placeholders.region')" />
      <UiTextField v-model.trim="form.isp" :label="t('runtime.ansibleMachines.fields.isp')" :placeholder="t('runtime.ansibleMachines.placeholders.isp')" />
    </div>
    <p v-if="formError" class="form-error" role="alert">{{ formError }}</p>
    <template #footer>
      <UiButton :disabled="saving" @click="close">{{ t('runtime.ansibleMachines.modal.cancel') }}</UiButton>
      <UiButton variant="primary" data-test="ansible-machine-save" :loading="saving" @click="submit">{{ t('runtime.ansibleMachines.modal.save') }}</UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Add or edit an Ansible machine (/admin/forward/ansible-machines): a relay
// in the Ansible inventory scope. SSH credentials stay in the inventory.
// Same calls and payload as the U6 page; extracted in U7 so the list and
// the detail page share it.
import { computed, reactive, ref, watch } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createAnsibleMachine, getAnsibleMachine, updateAnsibleMachine } from '@/api/admin'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { normalizeMachine, unwrapPayload } from './forwardNodeModel'

const props = defineProps({
  open: { type: Boolean, default: false },
  machineId: { type: Number, default: null }
})
const emit = defineEmits(['update:open', 'saved'])

const { t, translateLiteral } = useAppI18n()

const saving = ref(false)
const formError = ref('')
const form = reactive(createForm())
const editing = computed(() => Boolean(props.machineId))

function createForm() {
  return { id: null, name: '', host: '', port: '', region: '', isp: '', weight: '1' }
}

function resolveError(error, fallbackKey) {
  const text = String(error?.response?.data?.msg || error?.message || '').trim()
  return text ? translateLiteral(text) : t(fallbackKey)
}

watch(() => [props.open, props.machineId], async ([open, machineId]) => {
  if (!open) return
  Object.assign(form, createForm())
  formError.value = ''
  if (!machineId) return
  try {
    const detail = normalizeMachine(unwrapPayload(await getAnsibleMachine(machineId)))
    Object.assign(form, {
      id: detail.id,
      name: detail.name,
      host: detail.host,
      port: detail.port ? String(detail.port) : '',
      region: detail.region,
      isp: detail.isp,
      weight: String(detail.weight || 1)
    })
  } catch (error) {
    formError.value = resolveError(error, 'runtime.ansibleMachines.errors.detailFailed')
  }
}, { immediate: true })

function close(force = false) {
  if (saving.value && force !== true) return
  emit('update:open', false)
}

async function submit() {
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
    if (editing.value && form.id) {
      await updateAnsibleMachine(form.id, payload)
    } else {
      await createAnsibleMachine(payload)
    }
    const saved = form.name.trim()
    saving.value = false
    close(true)
    emit('saved', saved)
  } catch (error) {
    formError.value = resolveError(error, 'runtime.ansibleMachines.errors.saveFailed')
  } finally {
    saving.value = false
  }
}
</script>
