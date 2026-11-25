<template>
  <UiSection :title="t('mesh.settings.title')" heading-tag="h2" data-testid="mesh-settings">
    <form class="mesh-form" novalidate @submit.prevent="save">
      <UiTextField v-model="form.name_suffix" size="md" :label="t('mesh.settings.nameSuffix')" :help="t('mesh.settings.nameSuffixHelp')" data-testid="mesh-settings-suffix" />
      <div class="mesh-pair">
        <UiTextField v-model="form.host_group_supernet" size="md" :label="t('mesh.settings.hostGroupSupernet')" data-testid="mesh-settings-supernet" />
        <UiNumberField v-model="form.host_group_prefix" size="md" :label="t('mesh.settings.hostGroupPrefix')" :min="8" :max="30" />
      </div>
      <p class="mesh-hint">{{ t('mesh.settings.hostGroupHelp') }}</p>
      <UiTextarea v-model="form.reserved" :rows="3" :label="t('mesh.settings.reservedSubnets')" :help="t('mesh.settings.reservedSubnetsHelp')" data-testid="mesh-settings-reserved" />
      <p v-if="error" class="mesh-error" role="alert">{{ error }}</p>
      <div>
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="saving" data-testid="mesh-settings-save">{{ t('mesh.save') }}</UiButton>
      </div>
    </form>
  </UiSection>
</template>

<script setup>
// 组网 → 设置: the intranet domain suffix, the host group supernet and
// prefix, and the ranges no region may overlap.
import { reactive, ref, watch } from 'vue'
import { meshErrorMessage, updateMeshSettings } from '@/api/mesh'
import { useAppI18n } from '@/composables/useAppI18n'
import { UiButton, UiNumberField, UiSection, UiTextarea, UiTextField, useToast } from '@/ui'

const props = defineProps({
  settings: { type: Object, required: true }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const toast = useToast()

const form = reactive({ name_suffix: '', host_group_supernet: '', host_group_prefix: 24, reserved: '' })
const saving = ref(false)
const error = ref('')

watch(() => props.settings, settings => {
  form.name_suffix = settings.name_suffix || ''
  form.host_group_supernet = settings.host_group_supernet || ''
  form.host_group_prefix = settings.host_group_prefix || 24
  form.reserved = String(settings.reserved_subnets || '').split(',').filter(Boolean).join('\n')
}, { immediate: true })

async function save() {
  if (saving.value) return
  saving.value = true
  error.value = ''
  try {
    await updateMeshSettings({
      name_suffix: form.name_suffix,
      host_group_supernet: form.host_group_supernet,
      host_group_prefix: Number(form.host_group_prefix) || 0,
      reserved_subnets: form.reserved.split(/[\s,]+/).map(item => item.trim()).filter(Boolean)
    })
    toast.success(t('mesh.saved'))
    emit('changed')
  } catch (e) {
    error.value = meshErrorMessage(e, t('mesh.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped src="./mesh.css"></style>
