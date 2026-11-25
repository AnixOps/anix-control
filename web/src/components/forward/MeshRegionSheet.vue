<template>
  <UiSheet
    :open="open"
    size="md"
    :title="editing ? t('mesh.region.editTitle', { name: region.name }) : t('mesh.region.createTitle')"
    :description="t('mesh.region.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <form class="mesh-sheet" novalidate data-testid="mesh-region-form" @submit.prevent="save">
      <UiTextField v-model="form.name" size="md" :label="t('mesh.region.name')" required :error="errors.name" />
      <UiTextField v-model="form.code" size="md" :label="t('mesh.region.code')" required :help="t('mesh.region.codeHelp')" :error="errors.code" />
      <UiTextField v-model="form.cidr" size="md" :label="t('mesh.region.cidr')" required placeholder="10.150.0.0/16" :help="t('mesh.region.cidrHelp')" :error="errors.cidr" />
      <UiNumberField v-model="form.sort" size="md" :label="t('mesh.region.sort')" :min="0" />
      <UiSwitch v-model="form.enabled" :label="t('mesh.region.enabled')" />
      <p v-if="saveError" class="fwd-note is-danger" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="mesh-region-save" @click="save">{{ editing ? t('mesh.region.save') : t('mesh.region.create') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// A mesh region: name, code, range, order and whether it is enabled.
import { computed, ref, watch } from 'vue'
import { UiButton, UiNumberField, UiSheet, UiSwitch, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { saveKernelMeshRegion } from '@/api/kernel'
import { meshErrorMessage } from './meshModel'
import './forward.css'

const props = defineProps({
  open: { type: Boolean, default: false },
  // null for a new region.
  region: { type: Object, default: null }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const editing = computed(() => Boolean(props.region?.id))
const form = ref({})
const errors = ref({})
const saving = ref(false)
const saveError = ref('')

watch(() => props.open, value => {
  if (!value) return
  const region = props.region || {}
  form.value = { name: region.name || '', code: region.code || '', cidr: region.cidr || '', sort: region.sort || 0, enabled: Boolean(region.enabled) }
  errors.value = {}
  saveError.value = ''
}, { immediate: true })

async function save() {
  errors.value = {}
  for (const key of ['name', 'code', 'cidr']) {
    if (!String(form.value[key] || '').trim()) errors.value[key] = t('mesh.required')
  }
  if (Object.keys(errors.value).length) return
  saving.value = true
  saveError.value = ''
  try {
    await saveKernelMeshRegion(props.region?.id, form.value)
    toast.success(t('mesh.toast.saved'))
    emit('saved')
    emit('update:open', false)
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
