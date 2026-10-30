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
    <UiSelect
      v-model="form.preset"
      size="md"
      :label="t('admin.nodes.join.quick.preset')"
      :options="presetOptions"
      data-testid="quick-group-preset"
    />
    <fieldset v-if="form.preset !== 'none' && subscriptionGroups.length" class="quick-create__groups">
      <legend>{{ t('admin.nodes.join.quick.subscriptionGroups') }}</legend>
      <UiCheckbox
        v-for="group in subscriptionGroups"
        :key="group.id"
        :model-value="form.subscriptionGroupIds.includes(group.id)"
        :label="group.name"
        @update:model-value="checked => toggle(group.id, checked)"
      />
    </fieldset>
    <p v-if="error" class="form-error" role="alert" data-testid="quick-group-error">{{ error }}</p>
    <div class="quick-create__actions">
      <UiButton :disabled="saving" @click="emit('cancel')">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" type="submit" :loading="saving" data-testid="quick-group-save">{{ t('admin.nodes.join.quick.create') }}</UiButton>
    </div>
  </form>
</template>

<script setup>
// The minimum of a node group, inline in the join dialog: a name and one of
// the protocol presets (or none). The rest is the groups tab's edit sheet.
import { computed, onMounted, reactive, ref } from 'vue'
import { getSubscriptionGroups } from '@/api/admin'
import UiButton from '@/ui/UiButton.vue'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { DEFAULT_PRESET, PRESET_KEYS, buildQuickGroupInput, pickDefaultSubscriptionGroup, readKernelError } from './nodeGroupData'
import { readNodeList } from './nodeData'

const props = defineProps({
  // async (input) => created group; rejects with the API's error.
  create: { type: Function, required: true }
})
const emit = defineEmits(['cancel'])
const { t } = useAppI18n()

const form = reactive({ name: '', preset: DEFAULT_PRESET, subscriptionGroupIds: [] })
const subscriptionGroups = ref([])
const nameError = ref('')
const error = ref('')
const saving = ref(false)

const presetOptions = computed(() => PRESET_KEYS.map(value => ({ value, label: t(`admin.nodes.join.quick.presets.${value}`) })))

function toggle(id, checked) {
  const ids = new Set(form.subscriptionGroupIds)
  if (checked) ids.add(id)
  else ids.delete(id)
  form.subscriptionGroupIds = [...ids]
}

// The default subscription group starts checked, so a new group is sellable
// without another step.
onMounted(async () => {
  try {
    subscriptionGroups.value = readNodeList(await getSubscriptionGroups()).map(group => ({ id: Number(group.id), name: group.name }))
    const preferred = pickDefaultSubscriptionGroup(subscriptionGroups.value)
    if (preferred && !form.subscriptionGroupIds.includes(preferred.id)) form.subscriptionGroupIds = [preferred.id]
  } catch (cause) {
    // Without the list the group joins no subscription group; the groups tab
    // edits that.
    console.error('Failed to load the subscription groups:', cause)
  }
})

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

.quick-create__groups {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  border: 0;
}

.quick-create__groups legend {
  padding: 0;
  margin-bottom: var(--space-2);
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.quick-create__actions {
  display: flex;
  gap: var(--space-2);
  justify-content: flex-end;
}
</style>
