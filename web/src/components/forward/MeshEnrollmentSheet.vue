<template>
  <UiSheet
    :open="open"
    size="md"
    :title="t('mesh.enrollment.title')"
    :description="t('mesh.enrollment.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <div v-if="issued" class="mesh-sheet" data-testid="mesh-enrollment-issued">
      <UiCopyField :label="t('mesh.enrollment.token')" :value="issued.token" :help="t('mesh.enrollment.tokenHelp')" data-testid="mesh-enrollment-token" />
    </div>
    <form v-else class="mesh-sheet" novalidate data-testid="mesh-enrollment-form" @submit.prevent="save">
      <UiTextField v-model="form.name" size="md" :label="t('mesh.enrollment.name')" required :error="errors.name" />
      <UiSelect v-model="form.kind" size="md" :label="t('mesh.enrollment.kind')" :options="kindOptions" :help="t(`mesh.enrollment.kindHelp.${form.kind}`)" />
      <UiSelect v-model="form.regionId" size="md" :label="t('mesh.enrollment.region')" :options="regionOptions" />
      <UiNumberField v-model="form.hours" size="md" :label="t('mesh.enrollment.hours')" :min="1" :max="168" :help="t('mesh.enrollment.hoursHelp')" />
      <p v-if="saveError" class="fwd-note is-danger" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton v-if="issued" variant="primary" @click="close">{{ t('mesh.enrollment.done') }}</UiButton>
      <template v-else>
        <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
        <UiButton variant="primary" :loading="saving" data-testid="mesh-enrollment-save" @click="save">{{ t('mesh.enrollment.issue') }}</UiButton>
      </template>
    </template>
  </UiSheet>
</template>

<script setup>
// Issues a one-time enrollment credential for a device without an Agent
// and shows its token once.
import { computed, ref, watch } from 'vue'
import { UiButton, UiCopyField, UiNumberField, UiSelect, UiSheet, UiTextField } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { createKernelMeshEnrollment } from '@/api/kernel'
import { meshErrorMessage } from './meshModel'
import './forward.css'

const props = defineProps({
  open: { type: Boolean, default: false },
  regions: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()

const form = ref({})
const errors = ref({})
const saving = ref(false)
const saveError = ref('')
const issued = ref(null)

watch(() => props.open, value => {
  if (!value) return
  form.value = { name: '', kind: 'personal', regionId: 0, hours: 24 }
  errors.value = {}
  saveError.value = ''
  issued.value = null
}, { immediate: true })

const kindOptions = computed(() => ['personal', 'node'].map(kind => ({ value: kind, label: t(`mesh.device.kinds.${kind}`) })))
const regionOptions = computed(() => [
  { value: 0, label: t('mesh.enrollment.anyRegion') },
  ...props.regions.map(region => ({ value: region.id, label: `${region.name} · ${region.cidr}` }))
])

async function save() {
  errors.value = {}
  if (!String(form.value.name || '').trim()) errors.value.name = t('mesh.required')
  if (Object.keys(errors.value).length) return
  saving.value = true
  saveError.value = ''
  try {
    issued.value = await createKernelMeshEnrollment({ ...form.value, name: form.value.name.trim() })
    emit('saved')
  } catch (error) {
    saveError.value = meshErrorMessage(error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.mesh-sheet {
  display: grid;
  gap: var(--space-4);
}
</style>
