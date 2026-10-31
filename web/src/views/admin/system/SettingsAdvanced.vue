<template>
  <div class="settings-section" data-settings-panel="advanced">
    <NodeNotice tone="warning" data-test="advanced-warning">
      <strong>{{ t('adminSettings.advanced.warningTitle') }}</strong>
      <span>{{ t('adminSettings.advanced.warning') }}</span>
    </NodeNotice>

    <UiSection :title="t('adminSettings.advanced.configs.title')" :description="t('adminSettings.advanced.configs.description')">
      <template #actions>
        <UiButton :icon="Plus" data-test="system-config-add" @click="openConfigModal()">{{ t('adminSettings.advanced.configs.add') }}</UiButton>
      </template>
      <UiDataTable
        :columns="configColumns"
        :rows="filteredConfigs"
        row-key="key"
        :label="t('adminSettings.advanced.configs.title')"
        :row-label="config => config.key"
        storage-key="admin.system.configs"
        :page-size="20"
        :loading="configsLoading"
        :error="configsError"
        :error-title="t('adminSettings.advanced.configs.loadFailed')"
        :filtered="Boolean(configSearch)"
        :empty-icon="SlidersHorizontal"
        :empty-title="t('adminSettings.advanced.configs.empty')"
        :empty-description="t('adminSettings.advanced.configs.emptyDescription')"
        state-heading-tag="h3"
        :row-actions="configActions"
        @retry="fetchConfigs"
        @clear-filters="configSearch = ''"
      >
        <template #toolbar>
          <UiSearchField
            v-model="configSearch"
            class="list-page__search"
            :shortcut="false"
            :label="t('adminSettings.advanced.configs.search')"
            :placeholder="t('adminSettings.advanced.configs.search')"
          />
        </template>
        <template #cell-key="{ row }">
          <code class="settings-mono">{{ row.key }}</code>
        </template>
      </UiDataTable>
    </UiSection>

    <UiDialog
      v-model:open="showConfigModal"
      :title="editingConfig ? t('adminSettings.advanced.configs.editTitle') : t('adminSettings.advanced.configs.createTitle')"
      :dismissible="!configSaving"
    >
      <form id="system-config-form" class="form-grid" data-test="system-config-form" novalidate @submit.prevent="saveConfig">
        <UiTextField
          id="system-config-key"
          v-model="configForm.key"
          class="form-grid__full settings-mono-field"
          required
          :disabled="!!editingConfig"
          :label="t('adminSettings.advanced.configs.key')"
          :placeholder="t('adminSettings.advanced.configs.keyPlaceholder')"
          :error="configKeyError"
        />
        <UiTextarea
          id="system-config-value"
          v-model="configForm.value"
          class="form-grid__full settings-mono-field"
          :rows="3"
          :label="t('adminSettings.advanced.configs.value')"
          :placeholder="t('adminSettings.advanced.configs.valuePlaceholder')"
          :help="editingConfig && configForm.sensitive
            ? (configForm.has_value ? t('adminSettings.advanced.configs.sensitiveWithValue') : t('adminSettings.advanced.configs.sensitiveWithoutValue'))
            : ''"
        />
        <UiTextField
          id="system-config-description"
          v-model="configForm.description"
          class="form-grid__full"
          :label="t('adminSettings.advanced.configs.descriptionField')"
          :placeholder="t('adminSettings.advanced.configs.descriptionPlaceholder')"
        />
        <p v-if="configError" class="form-error form-grid__full" role="alert" data-test="system-config-error">{{ configError }}</p>
      </form>
      <template #footer="{ close }">
        <UiButton :disabled="configSaving" @click="close">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton variant="primary" type="submit" form="system-config-form" data-test="system-config-save" :loading="configSaving">{{ t('common.actions.save') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 系统设置 → 高级: every system configuration key as a raw key/value table
// (add, edit, delete), for troubleshooting only. The everyday options are in
// the other sections; the scheduler's internal cursors are not listed.
// Endpoints: GET/PUT/DELETE /admin/system/configs[/:key].
import { computed, onMounted, ref } from 'vue'
import { Pencil, SlidersHorizontal, Plus, Trash2 } from '@lucide/vue'
import { deleteSystemConfig, getSystemConfigs, setSystemConfig } from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import NodeNotice from '@/views/admin/nodes/NodeNotice.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { ensureSystemMutation, ensureSystemSuccess, readSystemPayload, systemErrorText } from './systemResponse'

const { t, translateLiteral } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const configs = ref([])
const configsLoading = ref(false)
const configsError = ref(null)
const configSearch = ref('')
const showConfigModal = ref(false)
const configSaving = ref(false)
const configError = ref('')
const configKeyError = ref('')
const editingConfig = ref(null)
const configForm = ref(emptyConfigForm())

function emptyConfigForm() {
  return { key: '', value: '', description: '', type: 'string', group: '', sensitive: false, has_value: false }
}

const translateText = (value, fallback = '-') => {
  const text = String(value ?? '').trim()
  return text ? translateLiteral(text) : fallback
}
const resolveSystemError = (error, fallbackKey) => translateText(systemErrorText(error), t(fallbackKey))

// System configuration keys --------------------------------------------------

const truncateValue = (value) => {
  if (value === undefined || value === null || value === '') return '—'
  const str = String(value)
  return str.length > 50 ? `${str.substring(0, 50)}…` : str
}

const getConfigDisplayValue = (config) => {
  if (!config || typeof config !== 'object') return ''
  return config.display_value ?? config.value
}

const filteredConfigs = computed(() => {
  if (!configSearch.value) return configs.value
  const search = configSearch.value.toLowerCase()
  return configs.value.filter(c =>
    c.key?.toLowerCase().includes(search) ||
    c.description?.toLowerCase().includes(search)
  )
})

const configColumns = computed(() => [
  { key: 'key', label: t('adminSettings.advanced.configs.key'), primary: true, sortable: true, hideable: false, truncate: true, minWidth: 160, maxWidth: 300 },
  { key: 'value', label: t('adminSettings.advanced.configs.value'), secondary: true, truncate: true, maxWidth: 280, value: config => truncateValue(getConfigDisplayValue(config)) },
  { key: 'description', label: t('adminSettings.advanced.configs.descriptionField'), breakpoint: 'md', value: config => translateText(config.description, '—') },
  { key: 'updated_at', label: t('adminSettings.advanced.configs.updatedAt'), nowrap: true, breakpoint: 'lg', sortable: true, firstDirection: 'desc', format: value => (value ? format.dateTime(value) : '—') }
])

const configActions = config => [
  { key: 'edit', label: t('common.actions.edit'), icon: Pencil, onSelect: () => openConfigModal(config) },
  { key: 'delete', label: t('common.actions.delete'), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => deleteConfig(config) }
]

const fetchConfigs = async () => {
  configsLoading.value = true
  configsError.value = null
  try {
    const payload = readSystemPayload(
      await getSystemConfigs(),
      t('adminSettings.advanced.configs.loadFailed')
    )
    const list = Array.isArray(payload.list) ? payload.list : []
    configs.value = list.map((config) => ({
      ...config,
      description: config.description || config.remark || '',
      sensitive: Boolean(config.sensitive),
      has_value: typeof config.has_value === 'boolean'
        ? config.has_value
        : String(config.value || '').trim() !== ''
    }))
  } catch (err) {
    configs.value = []
    configsError.value = resolveSystemError(err, 'adminSettings.advanced.configs.loadFailed')
  } finally {
    configsLoading.value = false
  }
}

const openConfigModal = (config = null) => {
  if (config) {
    editingConfig.value = config
    const sensitive = Boolean(config.sensitive)
    const rawValue = config.value ?? ''
    configForm.value = {
      key: config.key || '',
      value: sensitive ? '' : rawValue,
      description: config.description || config.remark || '',
      type: config.type || 'string',
      group: config.group || '',
      sensitive,
      has_value: typeof config.has_value === 'boolean'
        ? config.has_value
        : String(rawValue).trim() !== ''
    }
  } else {
    editingConfig.value = null
    configForm.value = emptyConfigForm()
  }
  configError.value = ''
  configKeyError.value = ''
  showConfigModal.value = true
}

const saveConfig = async () => {
  if (configSaving.value) return
  configKeyError.value = String(configForm.value.key || '').trim() ? '' : t('adminSettings.advanced.configs.keyRequired')
  if (configKeyError.value) return
  configSaving.value = true
  configError.value = ''
  try {
    const value = typeof configForm.value.value === 'string'
      ? configForm.value.value
      : String(configForm.value.value ?? '')
    const preserveExisting = Boolean(
      editingConfig.value &&
      configForm.value.sensitive &&
      configForm.value.has_value &&
      value.trim() === ''
    )

    await ensureSystemMutation(
      setSystemConfig(configForm.value.key, {
        value,
        type: configForm.value.type || 'string',
        group: configForm.value.group || '',
        description: configForm.value.description || '',
        preserve_existing: preserveExisting
      }),
      t('adminSettings.advanced.configs.saveFailedShort')
    )
    showConfigModal.value = false
    toast.success(t('adminSettings.advanced.configs.saved', { key: configForm.value.key }))
    fetchConfigs()
  } catch (err) {
    configError.value = t('adminSettings.advanced.configs.saveFailed', {
      message: resolveSystemError(err, 'adminSettings.advanced.configs.saveFailedShort')
    })
  } finally {
    configSaving.value = false
  }
}

const deleteConfig = async (config) => {
  const confirmed = await confirm({
    tone: 'danger',
    title: t('adminSettings.advanced.configs.deleteTitle', { key: config.key }),
    message: t('adminSettings.advanced.configs.deleteMessage'),
    confirmLabel: t('adminSettings.advanced.configs.deleteAction'),
    onConfirm: async () => {
      try {
        ensureSystemSuccess(await deleteSystemConfig(config.key), t('adminSettings.advanced.configs.deleteFailed'))
      } catch (err) {
        throw new Error(resolveSystemError(err, 'adminSettings.advanced.configs.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminSettings.advanced.configs.deleted', { key: config.key }))
  fetchConfigs()
}

onMounted(fetchConfigs)
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.settings-section :deep(.ui-section) {
  gap: var(--space-5);
}

.settings-mono,
.settings-mono-field :deep(input),
.settings-mono-field :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
