<template>
  <UiSection :title="t('meshAddr.settings.title')" heading-tag="h2" data-testid="mesh-settings">
    <form class="mesh-form" novalidate @submit.prevent="save">
      <UiTextField v-model="form.nameSuffix" size="md" :label="t('meshAddr.settings.nameSuffix')" :help="t('meshAddr.settings.nameSuffixHelp')" data-testid="mesh-settings-suffix" />
      <UiTextField v-model="form.hostGroupSupernet" size="md" :label="t('meshAddr.settings.hostGroupSupernet')" :help="supernetHelp" />
      <UiNumberField v-model="form.hostGroupPrefix" size="md" :label="t('meshAddr.settings.hostGroupPrefix')" :min="8" :max="30" />
      <UiTextarea v-model="form.reserved" :rows="3" :label="t('meshAddr.settings.reservedRanges')" :help="t('meshAddr.settings.reservedRangesHelp')" data-testid="mesh-settings-reserved" />
      <p v-if="error" class="mesh-error" role="alert">{{ error }}</p>
      <div>
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="saving" data-testid="mesh-settings-save">{{ t('meshAddr.common.save') }}</UiButton>
      </div>
    </form>
  </UiSection>
</template>

<script setup>
// 组网 → 设置: the intranet domain suffix, the host group range and segment
// length, and the ranges no region or host group may overlap.
import { computed, reactive, ref, watch } from 'vue'
import { UiButton, UiNumberField, UiSection, UiTextarea, UiTextField, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { saveKernelMeshSettings } from '@/api/kernel'
import { inSharedAddressSpace, meshErrorMessage } from '../meshModel'

const props = defineProps({
  settings: { type: Object, default: null }
})
const emit = defineEmits(['changed'])
const { t } = useAppI18n()
const toast = useToast()

const form = reactive({ nameSuffix: '', hostGroupSupernet: '', hostGroupPrefix: 24, reserved: '' })
const saving = ref(false)
const error = ref('')
const supernetHelp = computed(() => inSharedAddressSpace(form.hostGroupSupernet) ? `${t('meshAddr.settings.hostGroupHelp')} ${t('meshAddr.common.sharedAddressSpaceHelp')}` : t('meshAddr.settings.hostGroupHelp'))

watch(() => props.settings, settings => {
  if (!settings) return
  form.nameSuffix = settings.name_suffix || ''
  form.hostGroupSupernet = settings.host_group_supernet || ''
  form.hostGroupPrefix = settings.host_group_prefix || 24
  form.reserved = (settings.reserved_ranges || []).join('\n')
}, { immediate: true })

async function save() {
  if (saving.value) return
  saving.value = true
  error.value = ''
  try {
    await saveKernelMeshSettings({ ...form, reservedRanges: form.reserved.split(/[\s,]+/) })
    toast.success(t('meshAddr.common.saved'))
    emit('changed')
  } catch (e) {
    error.value = meshErrorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped src="./meshAddr.css"></style>
