<template>
  <UiSheet
    :open="open"
    size="md"
    :title="node ? t('admin.nodes.form.titleEdit') : t('admin.nodes.form.titleCreate')"
    :description="node ? node.name : t('admin.nodes.form.createDescription')"
    :dismissible="!saving"
    data-testid="node-form-sheet"
    @update:open="value => { if (!value) close() }"
  >
    <form class="node-form" novalidate @submit.prevent="save">
      <div class="form-grid">
        <UiTextField
          v-model="form.name"
          class="form-grid__full"
          size="md"
          required
          :label="t('admin.nodes.form.fields.name')"
          :placeholder="t('admin.nodes.form.placeholders.name')"
          :error="errors.name"
          @blur="validate('name')"
        />
        <UiTextField
          v-model="form.address"
          class="form-grid__full"
          size="md"
          required
          :label="t('admin.nodes.form.fields.address')"
          :placeholder="t('admin.nodes.form.placeholders.address')"
          :error="errors.address"
          @blur="validate('address')"
        />
        <UiNumberField v-model="form.rate" size="md" :min="0" :step="0.1" :format-options="{ useGrouping: false, maximumFractionDigits: 2 }" :label="t('admin.nodes.form.fields.rate')" :help="t('admin.nodes.form.help.rate')" />
        <UiNumberField v-model="form.sort" size="md" :label="t('admin.nodes.form.fields.sort')" :help="t('admin.nodes.form.help.sort')" />
        <UiTextField
          v-model="form.tags"
          class="form-grid__full"
          size="md"
          :label="t('admin.nodes.form.fields.tags')"
          :placeholder="t('admin.nodes.form.placeholders.tags')"
        />
        <UiSelect
          v-model="parentValue"
          class="form-grid__full"
          size="md"
          :label="t('admin.nodes.form.fields.parent')"
          :help="t('admin.nodes.form.parentHint')"
          :options="parentOptions"
        />
        <UiNumberField
          v-model="form.monthly_limit_gb"
          size="md"
          :min="0"
          :step="0.1"
          :format-options="{ useGrouping: false, maximumFractionDigits: 2 }"
          unit="GB"
          :label="t('admin.nodes.form.fields.monthlyLimit')"
          :help="t('admin.nodes.form.help.monthlyLimit')"
        />
        <UiNumberField
          v-model="form.monthly_reset_day"
          size="md"
          :min="1"
          :max="28"
          :label="t('admin.nodes.form.fields.monthlyResetDay')"
          :help="t('admin.nodes.form.help.monthlyResetDay')"
        />
        <UiSelect
          v-if="node"
          v-model="form.status"
          class="form-grid__full"
          size="md"
          :label="t('admin.nodes.form.fields.status')"
          :options="statusOptions"
          data-testid="node-form-status"
        />
      </div>
      <p v-if="formError" class="form-error" role="alert" data-testid="node-form-error">{{ formError }}</p>
    </form>
    <template #footer>
      <UiButton :disabled="saving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" data-testid="save-node" :loading="saving" @click="save">
        {{ node ? t('admin.nodes.form.save') : t('admin.nodes.form.create') }}
      </UiButton>
    </template>
  </UiSheet>
</template>

<script setup>
// Add or edit a node (POST /admin/nodes, PUT /admin/nodes/:id) in a sheet,
// with the fields and request body of the pre-redesign dialog. The parent
// list leaves out the node itself and its descendants.
import { computed, reactive, ref, watch } from 'vue'
import { createNode, updateNode } from '@/api/admin'
import UiButton from '@/ui/UiButton.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSheet from '@/ui/UiSheet.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useToast } from '@/ui/composables/useToast'
import { useAppI18n } from '@/composables/useAppI18n'
import { buildNodePayload, nodeFormFrom, parentCandidatesFor, readNodeApiError } from './nodeData'

const props = defineProps({
  open: { type: Boolean, default: false },
  // The node to edit; null adds one.
  node: { type: Object, default: null },
  // Nodes that can be the parent.
  candidates: { type: Array, default: () => [] }
})
const emit = defineEmits(['update:open', 'saved'])
const { t } = useAppI18n()
const toast = useToast()

const form = reactive(nodeFormFrom(null))
const errors = reactive({ name: '', address: '' })
const formError = ref('')
const saving = ref(false)

// "No parent" is 0 in the select (a select option cannot be empty).
const NO_PARENT = 0
const parentValue = computed({
  get: () => form.parent_id || NO_PARENT,
  set: (value) => { form.parent_id = value || null }
})
const parentOptions = computed(() => [
  { value: NO_PARENT, label: t('admin.nodes.form.parentNone') },
  ...parentCandidatesFor(props.candidates, props.node).map(candidate => ({
    value: candidate.id,
    label: `${candidate.name} (${candidate.address || candidate.host || '—'})`
  }))
])
const statusOptions = computed(() => ['pending', 'online', 'offline', 'disabled'].map((key, value) => ({
  value,
  label: t(`admin.nodes.statusText.${key}`)
})))

watch(() => props.open, (open) => {
  if (!open) return
  Object.assign(form, nodeFormFrom(props.node))
  errors.name = ''
  errors.address = ''
  formError.value = ''
}, { immediate: true })

function validate(field) {
  errors[field] = String(form[field] || '').trim() ? '' : t('admin.nodes.form.required')
  return !errors[field]
}

function close() {
  if (saving.value) return
  emit('update:open', false)
}

async function save() {
  if (saving.value) return
  formError.value = ''
  const ok = [validate('name'), validate('address')].every(Boolean)
  if (!ok) return
  saving.value = true
  const editing = Boolean(props.node)
  const payload = buildNodePayload(form, { editing })
  try {
    if (editing) {
      await updateNode(props.node.id, payload)
    } else {
      await createNode(payload)
    }
    saving.value = false
    emit('update:open', false)
    toast.success(t(editing ? 'admin.nodes.messages.nodeSaved' : 'admin.nodes.messages.nodeCreated', { name: payload.name }))
    emit('saved', { payload, editing })
  } catch (e) {
    formError.value = t('admin.nodes.messages.saveFailed', { message: readNodeApiError(e) })
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.node-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
</style>
