<template>
  <div class="settings-section" data-settings-panel="general">
    <UiSection :title="t('adminSettings.general.subscription.title')" :description="t('adminSettings.general.subscription.description')">
      <UiSkeleton v-if="showSubscriptionSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="subscriptionLoadError"
        compact
        heading-tag="h3"
        :title="t('adminSettings.general.subscription.loadFailed')"
        :error="subscriptionLoadError"
        @retry="loadSubscriptionDomainSettings"
      />
      <template v-else-if="!subscriptionSettingsLoading">
        <UiGroupedList>
          <UiGroupedListRow :label="t('adminSettings.general.subscription.path')" :value="subscriptionPath" />
          <UiGroupedListRow :label="t('adminSettings.general.subscription.currentHost')" :value="subscriptionCurrentHost || '—'" />
        </UiGroupedList>
        <UiGroupedList :footer="t('adminSettings.general.subscription.domainsHelp')">
          <UiGroupedListRow stacked>
            <UiTextarea
              id="settings-subscription-domains"
              v-model="subscriptionDomainsText"
              class="settings-domains"
              :rows="4"
              :label="t('adminSettings.general.subscription.domains')"
              :placeholder="t('adminSettings.general.subscription.domainsPlaceholder')"
              :error="subscriptionDomainFieldError"
              data-test="subscription-domains"
            />
          </UiGroupedListRow>
        </UiGroupedList>
        <UiGroupedList :title="t('adminSettings.general.subscription.preview')">
          <UiGroupedListRow v-if="normalizedSubscriptionDomains.length === 0" :label="t('adminSettings.general.subscription.previewEmpty')" />
          <UiGroupedListRow v-for="domain in normalizedSubscriptionDomains" v-else :key="domain">
            <template #label><code class="settings-mono">{{ `${subscriptionPreviewProtocol}//${domain}${subscriptionPath}` }}</code></template>
          </UiGroupedListRow>
        </UiGroupedList>
        <p v-if="subscriptionDomainError" class="form-error" role="alert" data-test="subscription-domain-error">{{ subscriptionDomainError }}</p>
      </template>
    </UiSection>

    <UiSection :title="t('adminSettings.general.configs.title')" :description="t('adminSettings.general.configs.description')">
      <template #actions>
        <UiButton :icon="Plus" data-test="system-config-add" @click="openConfigModal()">{{ t('adminSettings.general.configs.add') }}</UiButton>
      </template>
      <UiDataTable
        :columns="configColumns"
        :rows="filteredConfigs"
        row-key="key"
        :label="t('adminSettings.general.configs.title')"
        :row-label="config => config.key"
        storage-key="admin.system.configs"
        :page-size="20"
        :loading="configsLoading"
        :error="configsError"
        :error-title="t('adminSettings.general.configs.loadFailed')"
        :filtered="Boolean(configSearch)"
        :empty-icon="SlidersHorizontal"
        :empty-title="t('adminSettings.general.configs.empty')"
        :empty-description="t('adminSettings.general.configs.emptyDescription')"
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
            :label="t('adminSettings.general.configs.search')"
            :placeholder="t('adminSettings.general.configs.search')"
          />
        </template>
        <template #cell-key="{ row }">
          <code class="settings-mono">{{ row.key }}</code>
        </template>
      </UiDataTable>
    </UiSection>

    <SettingsSaveBar
      :visible="subscriptionDirty"
      :saving="subscriptionSettingsSaving"
      :invalid="Boolean(subscriptionDomainFieldError)"
      @save="saveSubscriptionDomainSettings"
      @discard="discardSubscriptionDomains"
    />

    <UiDialog
      v-model:open="showConfigModal"
      :title="editingConfig ? t('adminSettings.general.configs.editTitle') : t('adminSettings.general.configs.createTitle')"
      :dismissible="!configSaving"
    >
      <form id="system-config-form" class="form-grid" data-test="system-config-form" novalidate @submit.prevent="saveConfig">
        <UiTextField
          id="system-config-key"
          v-model="configForm.key"
          class="form-grid__full settings-mono-field"
          required
          :disabled="!!editingConfig"
          :label="t('adminSettings.general.configs.key')"
          :placeholder="t('adminSettings.general.configs.keyPlaceholder')"
          :error="configKeyError"
        />
        <UiTextarea
          id="system-config-value"
          v-model="configForm.value"
          class="form-grid__full settings-mono-field"
          :rows="3"
          :label="t('adminSettings.general.configs.value')"
          :placeholder="t('adminSettings.general.configs.valuePlaceholder')"
          :help="editingConfig && configForm.sensitive
            ? (configForm.has_value ? t('adminSettings.general.configs.sensitiveWithValue') : t('adminSettings.general.configs.sensitiveWithoutValue'))
            : ''"
        />
        <UiTextField
          id="system-config-description"
          v-model="configForm.description"
          class="form-grid__full"
          :label="t('adminSettings.general.configs.descriptionField')"
          :placeholder="t('adminSettings.general.configs.descriptionPlaceholder')"
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
// 系统设置 → 通用: the subscription domains (an edit with the save bar) and
// every system configuration key (a table with add, edit and delete).
// Endpoints unchanged: GET /admin/system/subscription-settings,
// GET/PUT/DELETE /admin/system/configs[/:key].
import { computed, onMounted, ref } from 'vue'
import { Pencil, Plus, SlidersHorizontal, Trash2 } from '@lucide/vue'
import {
  deleteSystemConfig,
  getSubscriptionSettings,
  getSystemConfig,
  getSystemConfigs,
  setSystemConfig
} from '@/api/admin'
import { useAppI18n } from '@/composables/useAppI18n'
import { useUnsavedChanges } from '@/composables/useUnsavedChanges'
import SettingsSaveBar from '@/components/admin/settings/SettingsSaveBar.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import UiTextField from '@/ui/UiTextField.vue'
import UiTextarea from '@/ui/UiTextarea.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { ensureSystemMutation, ensureSystemSuccess, readSystemPayload, systemErrorText } from './systemResponse'

const { t, translateLiteral } = useAppI18n()
const format = useFormat()
const toast = useToast()
const confirm = useConfirm()

const subscriptionDomainsConfigKey = 'app.subscribe_domains'
const subscriptionPath = ref('/s')
const subscriptionCurrentHost = ref(typeof window !== 'undefined' ? window.location.host : '')
const subscriptionDomainsText = ref('')
const savedSubscriptionDomainsText = ref('')
const subscriptionSettingsLoading = ref(false)
const subscriptionSettingsSaving = ref(false)
const subscriptionLoadError = ref(null)
const subscriptionDomainError = ref('')
const showSubscriptionSkeleton = useDelayedLoading(subscriptionSettingsLoading)

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

// Subscription domains -------------------------------------------------------

function splitDomainInput(value) {
  const text = String(value || '').trim()
  if (!text) return []
  return text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').replace(/[,;]/g, '\n')
    .split('\n').map(item => item.trim()).filter(Boolean)
}

function hostOf(line) {
  try {
    const url = new URL(line.includes('://') ? line : `https://${line}`)
    return (url.host || line).trim().replace(/\/+$/, '')
  } catch {
    return line.trim().replace(/\/+$/, '')
  }
}

const parseSubscriptionDomainInput = (value) => {
  const unique = []
  const seen = new Set()
  for (const line of splitDomainInput(value)) {
    const host = hostOf(line)
    if (!host || seen.has(host)) continue
    seen.add(host)
    unique.push(host)
  }
  return unique
}

const HOST_PATTERN = /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?::\d{1,5})?$|^\[[0-9a-f:.]+\](?::\d{1,5})?$/i

const normalizedSubscriptionDomains = computed(() => parseSubscriptionDomainInput(subscriptionDomainsText.value))
const invalidSubscriptionDomains = computed(() => splitDomainInput(subscriptionDomainsText.value).filter(line => !HOST_PATTERN.test(hostOf(line))))
const subscriptionDomainFieldError = computed(() => (
  invalidSubscriptionDomains.value.length
    ? t('adminSettings.general.subscription.invalidDomains', { domains: invalidSubscriptionDomains.value.join(', ') })
    : ''
))
const subscriptionDirty = computed(() => subscriptionDomainsText.value !== savedSubscriptionDomainsText.value)
const subscriptionPreviewProtocol = computed(() => {
  if (typeof window === 'undefined') return 'https:'
  return window.location.protocol || 'https:'
})

const readSubscriptionSettings = (res) => {
  if (!res || typeof res !== 'object') return { subscribe_path: '/s', subscribe_domains: [] }
  const payload = Object.prototype.hasOwnProperty.call(res, 'code') ? res.data : (res.data ?? res)
  if (!payload || typeof payload !== 'object') return { subscribe_path: '/s', subscribe_domains: [] }
  return {
    subscribe_path: payload.subscribe_path || '/s',
    subscribe_domains: Array.isArray(payload.subscribe_domains) ? payload.subscribe_domains : []
  }
}

function setLoadedDomains(text) {
  subscriptionDomainsText.value = text
  savedSubscriptionDomainsText.value = text
}

const loadSubscriptionDomainSettings = async () => {
  subscriptionSettingsLoading.value = true
  subscriptionLoadError.value = null
  subscriptionDomainError.value = ''
  try {
    const [settingsRes, rawConfigRes] = await Promise.allSettled([
      getSubscriptionSettings(),
      getSystemConfig(subscriptionDomainsConfigKey)
    ])

    if (settingsRes.status === 'fulfilled') {
      const payload = readSubscriptionSettings(settingsRes.value)
      subscriptionPath.value = payload.subscribe_path || '/s'
    }

    if (rawConfigRes.status === 'fulfilled') {
      const rawValue = rawConfigRes.value.data?.value
      if (typeof rawValue === 'string' && rawValue.trim()) {
        try {
          const parsed = JSON.parse(rawValue)
          setLoadedDomains(Array.isArray(parsed) ? parsed.join('\n') : rawValue)
        } catch {
          setLoadedDomains(rawValue)
        }
      } else {
        const settingsPayload = settingsRes.status === 'fulfilled' ? readSubscriptionSettings(settingsRes.value) : {}
        const domains = Array.isArray(settingsPayload.subscribe_domains) ? settingsPayload.subscribe_domains : []
        setLoadedDomains(domains.join('\n'))
      }
    } else if (settingsRes.status === 'fulfilled') {
      const settingsPayload = readSubscriptionSettings(settingsRes.value)
      const domains = Array.isArray(settingsPayload.subscribe_domains) ? settingsPayload.subscribe_domains : []
      setLoadedDomains(domains.join('\n'))
    } else {
      throw rawConfigRes.reason || settingsRes.reason || new Error('failed to load subscription settings')
    }
  } catch (err) {
    subscriptionLoadError.value = resolveSystemError(err, 'adminSettings.general.subscription.loadFailed')
  } finally {
    subscriptionSettingsLoading.value = false
  }
}

const saveSubscriptionDomainSettings = async () => {
  if (subscriptionSettingsSaving.value || subscriptionDomainFieldError.value) return
  subscriptionSettingsSaving.value = true
  subscriptionDomainError.value = ''
  const failed = t('adminSettings.general.subscription.saveFailed')
  try {
    const domains = normalizedSubscriptionDomains.value
    if (domains.length === 0) {
      try {
        await ensureSystemMutation(deleteSystemConfig(subscriptionDomainsConfigKey), failed)
      } catch {
        await ensureSystemMutation(
          setSystemConfig(subscriptionDomainsConfigKey, {
            value: '',
            type: 'string',
            group: 'app',
            description: 'Alternate subscription domains'
          }),
          failed
        )
      }
    } else {
      await ensureSystemMutation(
        setSystemConfig(subscriptionDomainsConfigKey, {
          value: JSON.stringify(domains),
          type: 'json',
          group: 'app',
          description: 'Alternate subscription domains'
        }),
        failed
      )
    }
    await Promise.all([
      loadSubscriptionDomainSettings(),
      fetchConfigs()
    ])
    toast.success(t('adminSettings.general.subscription.saved'))
  } catch (err) {
    subscriptionDomainError.value = resolveSystemError(err, 'adminSettings.general.subscription.saveFailed')
  } finally {
    subscriptionSettingsSaving.value = false
  }
}

function discardSubscriptionDomains() {
  subscriptionDomainsText.value = savedSubscriptionDomainsText.value
  subscriptionDomainError.value = ''
}

useUnsavedChanges(subscriptionDirty, { discard: discardSubscriptionDomains })

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
  { key: 'key', label: t('adminSettings.general.configs.key'), primary: true, sortable: true, hideable: false, truncate: true, minWidth: 160, maxWidth: 300 },
  { key: 'value', label: t('adminSettings.general.configs.value'), secondary: true, truncate: true, maxWidth: 280, value: config => truncateValue(getConfigDisplayValue(config)) },
  { key: 'description', label: t('adminSettings.general.configs.descriptionField'), breakpoint: 'md', value: config => translateText(config.description, '—') },
  { key: 'updated_at', label: t('adminSettings.general.configs.updatedAt'), nowrap: true, breakpoint: 'lg', sortable: true, firstDirection: 'desc', format: value => (value ? format.dateTime(value) : '—') }
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
      t('adminSettings.general.configs.loadFailed')
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
    configsError.value = resolveSystemError(err, 'adminSettings.general.configs.loadFailed')
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
  configKeyError.value = String(configForm.value.key || '').trim() ? '' : t('adminSettings.general.configs.keyRequired')
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
      t('adminSettings.general.configs.saveFailedShort')
    )
    showConfigModal.value = false
    toast.success(t('adminSettings.general.configs.saved', { key: configForm.value.key }))
    fetchConfigs()
  } catch (err) {
    configError.value = t('adminSettings.general.configs.saveFailed', {
      message: resolveSystemError(err, 'adminSettings.general.configs.saveFailedShort')
    })
  } finally {
    configSaving.value = false
  }
}

const deleteConfig = async (config) => {
  const confirmed = await confirm({
    tone: 'danger',
    title: t('adminSettings.general.configs.deleteTitle', { key: config.key }),
    message: t('adminSettings.general.configs.deleteMessage'),
    confirmLabel: t('adminSettings.general.configs.deleteAction'),
    onConfirm: async () => {
      try {
        ensureSystemSuccess(await deleteSystemConfig(config.key), t('adminSettings.general.configs.deleteFailed'))
      } catch (err) {
        throw new Error(resolveSystemError(err, 'adminSettings.general.configs.deleteFailed'))
      }
    }
  })
  if (!confirmed) return
  toast.success(t('adminSettings.general.configs.deleted', { key: config.key }))
  fetchConfigs()
}

onMounted(() => {
  loadSubscriptionDomainSettings()
  fetchConfigs()
})

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

.settings-domains {
  width: 100%;
}

.settings-mono,
.settings-mono-field :deep(input),
.settings-mono-field :deep(textarea) {
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}
</style>
