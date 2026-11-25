<template>
  <UiSheet
    :open="open"
    size="md"
    :title="editing ? t('mesh.member.moveTitle', { name: member.node_name || member.node_ref }) : t('mesh.member.addTitle')"
    :description="t('mesh.member.description')"
    :dismissible="!saving"
    @update:open="value => emit('update:open', value)"
  >
    <form class="mesh-sheet" novalidate data-testid="mesh-member-form" @submit.prevent="save">
      <UiSelect v-if="!editing" v-model="form.node" size="md" :label="t('mesh.member.node')" :options="nodeOptions" required :error="errors.node" data-testid="mesh-member-node" />
      <UiSelect v-model="form.regionId" size="md" :label="t('mesh.member.region')" :options="regionOptions" required :error="errors.region" data-testid="mesh-member-region" />
      <UiSwitch v-model="form.boundary" :label="t('mesh.member.boundary')" :description="t('mesh.member.boundaryHelp')" />
      <p v-if="saveError" class="fwd-note is-danger" role="alert">{{ saveError }}</p>
    </form>
    <template #footer="{ close }">
      <UiButton :disabled="saving" @click="close">{{ t('forwardV4.common.cancel') }}</UiButton>
      <UiButton variant="primary" :loading="saving" data-testid="mesh-member-save" @click="save">{{ editing ? t('mesh.member.save') : t('mesh.member.add') }}</UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// Adds a node to a mesh region, or moves a member to another region; both
// set whether the node is a boundary node.
import { computed, ref, watch } from 'vue'
import { UiButton, UiSelect, UiSheet, UiSwitch, useToast } from '@/ui'
import { useAppI18n } from '@/composables/useAppI18n'
import { addKernelMeshMember, updateKernelMeshMember } from '@/api/kernel'
import { meshErrorMessage } from './meshModel'
import './forward.css'

const props = defineProps({
  open: { type: Boolean, default: false },
  // A members-table row to move, or null to add a node.
  member: { type: Object, default: null },
  regions: { type: Array, default: () => [] },
  // The nodes not in the mesh yet: [{ ref, name, host }].
  nodes: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const editing = computed(() => Boolean(props.member?.id))
const form = ref({})
const errors = ref({})
const saving = ref(false)
const saveError = ref('')

watch(() => props.open, value => {
  if (!value) return
  form.value = { node: '', regionId: props.member?.region_id || props.regions[0]?.id || null, boundary: Boolean(props.member?.boundary) }
  errors.value = {}
  saveError.value = ''
}, { immediate: true })

const nodeOptions = computed(() => props.nodes.map(node => ({ value: node.ref, label: `${node.name || node.ref}${node.host ? ` · ${node.host}` : ''}` })))
const regionOptions = computed(() => props.regions.map(region => ({ value: region.id, label: `${region.name} · ${region.cidr}` })))

async function save() {
  errors.value = {}
  if (!editing.value && !form.value.node) errors.value.node = t('mesh.required')
  if (!form.value.regionId) errors.value.region = t('mesh.required')
  if (Object.keys(errors.value).length) return
  saving.value = true
  saveError.value = ''
  try {
    if (editing.value) await updateKernelMeshMember(props.member.id, form.value)
    else await addKernelMeshMember(form.value.regionId, form.value)
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
