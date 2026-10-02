<template>
  <UiDialog
    :open="open"
    :title="group ? t('adminSubscriptionGroups.groupDialog.editTitle') : t('adminSubscriptionGroups.groupDialog.createTitle')"
    :dismissible="!groupSaving"
    @update:open="value => { if (!value) close() }"
  >
    <form id="subscription-group-form" class="form-grid" novalidate @submit.prevent="saveGroup">
      <UiTextField
        id="subscription-group-name"
        v-model="groupForm.name"
        class="form-grid__full"
        required
        :label="t('adminSubscriptionGroups.groupDialog.name')"
        :placeholder="t('adminSubscriptionGroups.groupDialog.namePlaceholder')"
        :error="nameTouched ? nameError : ''"
        @blur="nameTouched = true"
      />
      <UiTextarea
        id="subscription-group-description"
        v-model="groupForm.description"
        class="form-grid__full"
        :rows="3"
        :label="t('adminSubscriptionGroups.groupDialog.description')"
      />
      <UiNumberField
        id="subscription-group-priority"
        v-model="groupForm.priority"
        :label="t('adminSubscriptionGroups.groupDialog.priority')"
        :help="t('adminSubscriptionGroups.groupDialog.priorityHelp')"
      />
      <div class="group-enable">
        <UiSwitch v-model="groupForm.enable" :label="t('adminSubscriptionGroups.groupDialog.enabled')" />
      </div>
      <p v-if="groupFormError" class="form-error form-grid__full" role="alert" data-test="group-form-error">{{ groupFormError }}</p>
    </form>
    <template #footer>
      <UiButton :disabled="groupSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
      <UiButton variant="primary" type="submit" form="subscription-group-form" data-test="save-group" :loading="groupSaving">{{ t('common.actions.save') }}</UiButton>
    </template>
  </UiDialog>
</template>

<script setup>
// Create or edit a subscription group. POST /admin/subscription/groups,
// PUT /admin/subscription/groups/:id; fields unchanged (name, description,
// priority, enable as 0/1). A failed save stays in the dialog.
import { computed, reactive, ref, watch } from 'vue'
import adminApi from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiNumberField from '@/ui/UiNumberField.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useToast } from '@/ui/composables/useToast'
import { ensureSubscriptionSuccess } from './subscriptionShared'

const props = defineProps({
  open: { type: Boolean, default: false },
  // The group to edit; null creates one.
  group: { type: Object, default: null }
})
const emit = defineEmits(['update:open', 'saved'])

const { t } = useAppI18n()
const toast = useToast()
const groupSaving = ref(false)
const groupFormError = ref('')
const nameTouched = ref(false)
const groupForm = reactive({ id: null, name: '', description: '', priority: 0, enable: true })

const nameError = computed(() => (String(groupForm.name || '').trim() ? '' : t('adminSubscriptionGroups.groupDialog.nameRequired')))

watch(() => props.open, open => {
  if (!open) return
  const group = props.group
  Object.assign(groupForm, group
    ? { id: group.id, name: group.name, description: group.description || '', priority: group.priority ?? 0, enable: group.enable === 1 }
    : { id: null, name: '', description: '', priority: 0, enable: true })
  groupFormError.value = ''
  nameTouched.value = false
}, { immediate: true })

function close() {
  if (groupSaving.value) return
  emit('update:open', false)
}

async function saveGroup() {
  if (groupSaving.value) return
  nameTouched.value = true
  if (nameError.value) return
  groupSaving.value = true
  groupFormError.value = ''
  const failed = t('adminSubscriptionGroups.messages.saveFailed')
  try {
    const data = {
      name: groupForm.name,
      description: groupForm.description,
      priority: groupForm.priority,
      enable: groupForm.enable ? 1 : 0
    }
    if (groupForm.id) {
      ensureSubscriptionSuccess(await adminApi.updateSubscriptionGroup(groupForm.id, data), failed)
    } else {
      ensureSubscriptionSuccess(await adminApi.createSubscriptionGroup(data), failed)
    }
    groupSaving.value = false
    toast.success(t('adminSubscriptionGroups.messages.groupSaved'))
    emit('update:open', false)
    emit('saved', { ...data, id: groupForm.id })
  } catch {
    groupFormError.value = failed
  } finally {
    groupSaving.value = false
  }
}
</script>

<style scoped>
.group-enable {
  display: flex;
  align-items: center;
  padding-top: var(--space-6);
}

@media (max-width: 639.98px) {
  .group-enable {
    padding-top: 0;
  }
}
</style>
