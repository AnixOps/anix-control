<template>
  <div class="notify-panel" data-notify-panel="templates">
    <UiSection :title="t('adminNotify.templates.title')" :description="t('adminNotify.templates.description')">
      <template #actions>
        <UiButton variant="primary" :icon="Plus" data-test="notification-template-create" @click="openTemplateModal()">{{ t('adminNotify.templates.create') }}</UiButton>
      </template>
      <UiDataTable
        :columns="columns"
        :rows="filteredTemplates"
        :label="t('adminNotify.templates.title')"
        :row-label="template => template.name || `#${template.id}`"
        storage-key="admin.notificationTemplates"
        :page-size="20"
        :loading="templatesLoading"
        :error="templatesError"
        :error-title="t('adminNotify.templates.loadFailed')"
        :filtered="Boolean(typeFilter)"
        :empty-icon="FileText"
        :empty-title="t('adminNotify.templates.empty')"
        :empty-description="t('adminNotify.templates.emptyDescription')"
        state-heading-tag="h3"
        :row-actions="templateActions"
        @retry="fetchTemplates"
        @clear-filters="typeFilter = ''"
      >
        <template #toolbar>
          <UiFilterChips v-model="typeFilter" :label="t('adminNotify.templates.filterLabel')" :options="typeChips" />
        </template>
        <template #cell-type="{ row }">
          <UiBadge tone="info" :dot="false" :label="typeLabel(row.type)" />
        </template>
        <template #cell-enabled="{ row }">
          <UiBadge :tone="row.enabled ? 'success' : 'neutral'" :label="row.enabled ? t('adminNotify.status.enabled') : t('adminNotify.status.disabled')" />
        </template>
        <template #empty-actions>
          <UiButton variant="primary" :icon="Plus" @click="openTemplateModal()">{{ t('adminNotify.templates.create') }}</UiButton>
        </template>
      </UiDataTable>
    </UiSection>

    <UiDialog
      v-model:open="showTemplateModal"
      :title="editingTemplate ? t('adminNotify.templates.editTitle') : t('adminNotify.templates.createTitle')"
      :dismissible="!templateSaving"
    >
      <form id="notification-template-form" class="form-grid" novalidate @submit.prevent="saveTemplate">
        <UiTextField
          id="notification-template-name"
          v-model="templateForm.name"
          class="form-grid__full"
          required
          data-test="notification-template-name"
          :label="t('adminNotify.templates.name')"
          :placeholder="t('adminNotify.templates.namePlaceholder')"
          :error="nameError"
        />
        <UiSelect id="notification-template-type" v-model="templateForm.type" :label="t('adminNotify.templates.type')" :options="typeOptions" />
        <UiSelect id="notification-template-event" v-model="templateForm.event" :label="t('adminNotify.templates.event')" :options="eventOptions" />
        <UiTextField
          id="notification-template-title"
          v-model="templateForm.title"
          class="form-grid__full"
          :label="t('adminNotify.templates.subject')"
          :placeholder="t('adminNotify.templates.subjectPlaceholder')"
        />
        <UiTextarea
          id="notification-template-content"
          v-model="templateForm.content"
          class="form-grid__full"
          :rows="5"
          :label="t('adminNotify.templates.content')"
          :placeholder="t('adminNotify.templates.contentPlaceholder')"
        />
        <div class="form-grid__full">
          <UiSwitch v-model="templateForm.enabled" :label="t('adminNotify.templates.enabled')" />
        </div>
        <p v-if="templateError" class="form-error form-grid__full" role="alert" data-test="notification-template-error">{{ templateError }}</p>
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="templateSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" type="submit" form="notification-template-form" data-test="notification-template-save" :loading="templateSaving">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 通知 → 模板: the message templates of every channel (e-mail, Telegram,
// webhook) per event, with type chips, a dialog to create or edit and a
// confirmed delete. Endpoints unchanged: GET/POST
// /admin/notification/templates, PUT/DELETE /admin/notification/templates/:id.
import { computed, onMounted, ref } from 'vue'
import { FileText, Pencil, Plus, Trash2 } from '@lucide/vue'
import { createNotificationTemplate, deleteNotificationTemplate, getNotificationTemplates, updateNotificationTemplate } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useToast } from '@/ui/composables/useToast'
import { EVENT_KEYS, NOTIFICATION_EVENTS, NOTIFICATION_TYPES, ensureNotifySuccess, notifyErrorText, readNotifyPayload } from './notifyResponse'

const { t } = useAppI18n()
const toast = useToast()
const confirm = useConfirm()

const templates = ref([])
const templatesLoading = ref(false)
const templatesError = ref(null)
const typeFilter = ref('')
const showTemplateModal = ref(false)
const editingTemplate = ref(null)
const templateForm = ref(createTemplateForm())
const templateSaving = ref(false)
const templateError = ref('')
const nameError = ref('')

function createTemplateForm(source = {}) {
  return { name: '', type: 'email', event: 'user.register', title: '', content: '', enabled: true, ...source }
}

const typeLabel = type => (NOTIFICATION_TYPES.includes(type) ? t(`adminNotify.types.${type}`) : (type || '—'))
const eventLabel = event => (EVENT_KEYS[event] ? t(`adminNotify.events.${EVENT_KEYS[event]}`) : (event || '—'))

const typeOptions = computed(() => NOTIFICATION_TYPES.map(value => ({ value, label: typeLabel(value) })))
const eventOptions = computed(() => NOTIFICATION_EVENTS.map(value => ({ value, label: eventLabel(value) })))
const typeChips = computed(() => NOTIFICATION_TYPES.map(value => ({
  value,
  label: typeLabel(value),
  count: templates.value.filter(template => template.type === value).length
})))
const filteredTemplates = computed(() => (typeFilter.value ? templates.value.filter(template => template.type === typeFilter.value) : templates.value))

const columns = computed(() => [
  { key: 'name', label: t('adminNotify.templates.name'), primary: true, sortable: true, hideable: false, value: template => template.name || '—' },
  { key: 'event', label: t('adminNotify.templates.event'), secondary: true, sortable: true, value: template => eventLabel(template.event) },
  { key: 'type', label: t('adminNotify.templates.type') },
  { key: 'enabled', label: t('adminNotify.templates.state') },
  { key: 'id', label: 'ID', numeric: true, hidden: true }
])

const templateActions = template => [
  { key: 'edit', label: t('common.actions.edit'), icon: Pencil, onSelect: () => openTemplateModal(template) },
  { key: 'delete', label: t('common.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteTemplateItem(template) }
]

const fetchTemplates = async () => {
  templatesLoading.value = true
  templatesError.value = null
  try {
    const res = await getNotificationTemplates()
    const payload = readNotifyPayload(res, t('adminNotify.templates.loadFailed'))
    templates.value = payload.list || []
  } catch (error) {
    templates.value = []
    templatesError.value = notifyErrorText(error) || t('adminNotify.templates.loadFailed')
  } finally {
    templatesLoading.value = false
  }
}

const openTemplateModal = (template = null) => {
  editingTemplate.value = template
  templateForm.value = createTemplateForm(template || {})
  templateError.value = ''
  nameError.value = ''
  showTemplateModal.value = true
}

const saveTemplate = async () => {
  if (templateSaving.value) return
  nameError.value = String(templateForm.value.name || '').trim() ? '' : t('adminNotify.templates.nameRequired')
  if (nameError.value) return
  templateSaving.value = true
  templateError.value = ''
  const failed = t('adminNotify.templates.saveFailedShort')
  try {
    if (editingTemplate.value) {
      ensureNotifySuccess(await updateNotificationTemplate(editingTemplate.value.id, templateForm.value), failed)
    } else {
      ensureNotifySuccess(await createNotificationTemplate(templateForm.value), failed)
    }
    toast.success(t('adminNotify.templates.saved'))
    showTemplateModal.value = false
    await fetchTemplates()
  } catch (error) {
    templateError.value = t('adminNotify.templates.saveFailed', { message: notifyErrorText(error) || failed })
  } finally {
    templateSaving.value = false
  }
}

const deleteTemplateItem = async (template) => {
  const confirmed = await confirm({
    title: t('adminNotify.templates.deleteTitle', { name: template.name }),
    message: t('adminNotify.templates.deleteMessage'),
    confirmLabel: t('adminNotify.templates.deleteAction'),
    tone: 'danger',
    onConfirm: async () => {
      try {
        ensureNotifySuccess(await deleteNotificationTemplate(template.id), t('adminNotify.templates.deleteFailed'))
      } catch (error) {
        throw new Error(notifyErrorText(error) || t('adminNotify.templates.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminNotify.templates.deleted', { name: template.name }))
  await fetchTemplates()
}

onMounted(fetchTemplates)
</script>

<style scoped>
.notify-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-8);
  min-width: 0;
}
</style>
