<template>
  <form class="quick-create" novalidate data-testid="quick-group-form" @submit.prevent="submit">
    <UiTextField
      v-model="form.name"
      size="md"
      required
      :label="t('admin.nodes.join.quick.name')"
      :placeholder="t('admin.nodes.join.quick.namePlaceholder')"
      :error="nameError"
      data-testid="quick-group-name"
    />
    <p v-if="error" class="form-error" role="alert" data-testid="quick-group-error">{{ error }}</p>
    <div class="quick-create__actions">
      <UiButton :disabled="saving" @click="emit('cancel')">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" type="submit" :loading="saving" data-testid="quick-group-save">{{ t('admin.nodes.join.quick.create') }}</UiButton>
    </div>
  </form>
</template>

<script setup>
// The minimum of a node group, inline in the join dialog: a name. The rest is
// the groups tab's edit sheet.
import { reactive, ref } from 'vue'
import UiButton from '@/ui/UiButton.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { buildQuickGroupInput, readKernelError } from './nodeGroupData'

const props = defineProps({
  // async (input) => created group; rejects with the API's error.
  create: { type: Function, required: true }
})
const emit = defineEmits(['cancel'])
const { t } = useAppI18n()

const form = reactive({ name: '' })
const nameError = ref('')
const error = ref('')
const saving = ref(false)

async function submit() {
  if (saving.value) return
  error.value = ''
  nameError.value = String(form.name || '').trim() ? '' : t('admin.nodes.form.required')
  if (nameError.value) return
  saving.value = true
  try {
    await props.create(buildQuickGroupInput(form))
  } catch (cause) {
    const problem = readKernelError(cause)
    error.value = problem.code === 'name_taken' ? t('admin.nodes.groups.nameTaken') : t('admin.nodes.join.quick.failed', { message: problem.message })
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.quick-create {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  border-radius: var(--radius-md);
  background: var(--fill-1);
}

.quick-create__actions {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
}
</style>
