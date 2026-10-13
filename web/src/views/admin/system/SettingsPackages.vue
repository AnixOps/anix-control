<template>
  <div class="settings-section" data-settings-panel="packages">
    <UiSection :title="t('adminSettings.packages.title')" :description="t('adminSettings.packages.description')">
      <template #actions>
        <UiButton :icon="RefreshCw" :disabled="loading" data-testid="packages-refresh" @click="load()">
          {{ t('adminSettings.packages.refresh') }}
        </UiButton>
      </template>
      <UiSkeleton v-if="showSkeleton" variant="card" :label="t('adminSettings.loading')" />
      <UiErrorState
        v-else-if="loadError && !rows.length"
        compact
        heading-tag="h3"
        :title="t('adminSettings.packages.loadFailed')"
        :error="loadError"
        @retry="load()"
      />
      <UiEmptyState
        v-else-if="loaded && !rows.length"
        compact
        :icon="Package"
        :title="t('adminSettings.packages.empty.title')"
        :description="t('adminSettings.packages.empty.description')"
      />
      <UiGroupedList v-else-if="rows.length" data-testid="package-list">
        <UiGroupedListRow
          v-for="row in rows"
          :key="row.id"
          :data-package="row.pluginID"
          :data-testid="`package-row-${row.pluginID}`"
        >
          <template #label>{{ row.name }}</template>
          <template #description>
            <span class="package-meta"><code>{{ row.pluginID }}</code> · {{ row.versionText }}</span>
            <span v-if="row.error" class="package-error">{{ row.error }}</span>
          </template>
          <template #value>
            <UiBadge :tone="row.tone" :label="row.stateLabel" />
          </template>
          <span class="package-actions">
            <UiButton
              v-if="row.configurable"
              size="sm"
              :disabled="Boolean(busyID)"
              :aria-label="t('adminSettings.packages.configureNamed', { name: row.name })"
              data-action="configure"
              @click="openConfig(row)"
            >
              {{ t('adminSettings.packages.configure') }}
            </UiButton>
            <UiButton
              size="sm"
              :icon="RotateCw"
              :loading="busyID === row.id"
              :disabled="!row.enabled || Boolean(busyID)"
              :aria-label="t('adminSettings.packages.restartNamed', { name: row.name })"
              data-action="restart"
              @click="restart(row)"
            >
              {{ t('adminSettings.packages.restart') }}
            </UiButton>
          </span>
        </UiGroupedListRow>
      </UiGroupedList>
      <p v-if="loadError && rows.length" class="package-banner" role="alert">{{ loadError }}</p>
      <section v-if="adminExtensionErrors.length" class="package-banner" data-testid="package-extension-errors" role="alert">
        <strong>{{ t('control.extensions.errorsTitle') }}</strong>
        <ul>
          <li v-for="(extensionError, index) in adminExtensionErrors" :key="`${extensionError.plugin_id || 'catalog'}-${index}`">
            <code v-if="extensionError.plugin_id">{{ extensionError.plugin_id }}</code>
            {{ extensionError.message }}
          </li>
        </ul>
      </section>
      <p class="package-note">{{ t('adminSettings.packages.note') }}</p>
    </UiSection>

    <UiDialog
      :open="configEditor.open"
      size="lg"
      data-testid="package-config-dialog"
      :title="t('adminSettings.packages.config.title', { name: configEditor.row?.name || '' })"
      :description="configEditor.row?.pluginID || ''"
      :dismissible="!configEditor.saving"
      @update:open="value => { if (!value) closeConfig() }"
    >
      <UiSkeleton v-if="configEditor.loading" :label="t('control.config.loading')" />
      <PluginConfigForm
        v-else
        v-model="configEditor.value"
        :schema="configEditor.schema"
        @validity="configEditor.valid = $event"
      />
      <p v-if="configEditor.error" class="form-error" role="alert">{{ configEditor.error }}</p>
      <template #footer>
        <span class="revision-label">{{ t('control.config.revision', { revision: configEditor.revision }) }}</span>
        <UiButton :disabled="configEditor.saving" @click="closeConfig">{{ t('common.actions.cancel') }}</UiButton>
        <UiButton
          variant="primary"
          data-testid="save-package-config"
          :loading="configEditor.saving"
          :disabled="configEditor.loading || !configEditor.valid"
          @click="saveConfig"
        >
          {{ t('common.actions.save') }}
        </UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
// 系统设置 → 内置包: the official packages Control installs, upgrades and
// enables at every start (docs/architecture/plugin-kernel-contract.md,
// "Built-in Packages"). Read-only status (version, state, last error), plus
// what the single-binary design keeps for them (section 4.3): restarting a
// host and editing the configuration of a package that declares settings.
// There is nothing to import, install, upgrade, roll back or disable.
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Package, RefreshCw, RotateCw } from '@lucide/vue'
import {
  getKernelInstallationConfig,
  getKernelInstallations,
  getKernelPluginReleases,
  getKernelPlugins,
  restartKernelInstallation,
  updateKernelInstallationConfig
} from '@/api/kernel'
import PluginConfigForm from '@/components/admin/PluginConfigForm.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { adminExtensionErrors, refreshAdminExtensions } from '@/extensions/runtime'
import { parseManifest } from '@/utils/kernelPluginRelease'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiGroupedList from '@/ui/UiGroupedList.vue'
import UiGroupedListRow from '@/ui/UiGroupedListRow.vue'
import UiSection from '@/ui/UiSection.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useConfirm } from '@/ui/composables/useConfirm'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import { useToast } from '@/ui/composables/useToast'

const POLL_INTERVAL_MS = 2000
const MAX_POLLS = 30

const { t } = useAppI18n()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const plugins = ref([])
const releases = ref([])
const installations = ref([])
const loading = ref(false)
const loaded = ref(false)
const loadError = ref('')
const busyID = ref(0)
const showSkeleton = useDelayedLoading(() => loading.value && !loaded.value)
const configEditor = reactive({ open: false, row: null, schema: {}, value: {}, revision: 0, valid: true, loading: false, saving: false, error: '' })

let pollTimer = null
let polls = 0
let disposed = false
let configSession = 0

function list(value) {
  return Array.isArray(value) ? value : []
}

function errorMessage(cause, fallbackKey) {
  const response = cause?.response?.data
  return response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || response?.message || cause?.message || t(fallbackKey)
}

// The state badge: the health the kernel derives from the installation's
// state, in words (colour is never the only signal).
const HEALTH = {
  healthy: { tone: 'success', key: 'healthy' },
  pending: { tone: 'info', key: 'pending' },
  unhealthy: { tone: 'danger', key: 'failed' },
  degraded: { tone: 'warning', key: 'degraded' },
  disabled: { tone: 'neutral', key: 'disabled' }
}

function schemaFor(pluginID, version) {
  const release = releases.value.find(item => item?.plugin_id === pluginID && item?.version === version)
  const schema = parseManifest(release).config_schema
  return schema && typeof schema === 'object' && !Array.isArray(schema) ? schema : {}
}

// The built-in packages are the Control installations; an Agent
// installation an earlier release left is not one.
const rows = computed(() => list(installations.value)
  .filter(item => item?.target === 'control')
  .map(item => {
    const plugin = list(plugins.value).find(entry => entry?.id === item.plugin_id) || {}
    const health = HEALTH[item.health] || { tone: 'neutral', key: '' }
    const schema = schemaFor(item.plugin_id, item.desired_version)
    const observed = item.observed_version || ''
    const desired = item.desired_version || ''
    return {
      id: item.id,
      pluginID: item.plugin_id,
      name: plugin.name || item.plugin_id,
      enabled: item.enabled === true,
      desired,
      versionText: observed && observed !== desired
        ? t(item.health === 'pending' ? 'adminSettings.packages.versionChanging' : 'adminSettings.packages.versionNotRunning', { from: observed, to: desired })
        : t('adminSettings.packages.version', { version: desired || '—' }),
      tone: health.tone,
      stateLabel: health.key ? t(`adminSettings.packages.states.${health.key}`) : (item.state || '—'),
      error: item.last_error || '',
      pending: item.health === 'pending',
      configurable: Boolean(schema.properties && Object.keys(schema.properties).length),
      schema
    }
  })
  .sort((left, right) => left.name.localeCompare(right.name)))

async function load({ silent = false } = {}) {
  if (!silent) loading.value = true
  try {
    const [pluginRows, releaseRows, installationRows] = await Promise.all([getKernelPlugins(), getKernelPluginReleases(), getKernelInstallations()])
    plugins.value = list(pluginRows)
    releases.value = list(releaseRows)
    installations.value = list(installationRows)
    loaded.value = true
    loadError.value = ''
  } catch (cause) {
    loadError.value = errorMessage(cause, 'adminSettings.packages.loadFailed')
  } finally {
    if (!silent) loading.value = false
  }
  schedulePoll()
}

// While a package is pending (a host starting after a restart), refresh
// until it settles, for at most a minute.
function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
  if (disposed || !rows.value.some(row => row.pending) || polls >= MAX_POLLS) {
    polls = 0
    return
  }
  polls += 1
  pollTimer = setTimeout(() => { void load({ silent: true }) }, POLL_INTERVAL_MS)
}

function idempotencyKey(row) {
  const token = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `webui:${row.id}:restart:${token}`
}

async function restart(row) {
  const restarted = await confirm({
    title: t('adminSettings.packages.confirm.title', { name: row.name }),
    message: t('adminSettings.packages.confirm.message'),
    confirmLabel: t('adminSettings.packages.restart'),
    onConfirm: async () => {
      busyID.value = row.id
      try {
        await restartKernelInstallation(row.id, idempotencyKey(row))
      } catch (cause) {
        throw new Error(errorMessage(cause, 'adminSettings.packages.restartFailed'))
      } finally {
        busyID.value = 0
      }
    }
  })
  if (!restarted) return
  toast.success(t('adminSettings.packages.restarted', { name: row.name }))
  polls = 0
  await load({ silent: true })
}

async function openConfig(row) {
  const session = ++configSession
  Object.assign(configEditor, { open: true, row, schema: row.schema, value: {}, revision: 0, valid: true, loading: true, saving: false, error: '' })
  try {
    const configuration = await getKernelInstallationConfig(row.id)
    if (session !== configSession || !configEditor.open) return
    const raw = configuration?.config ?? {}
    configEditor.value = typeof raw === 'string' ? JSON.parse(raw || '{}') : raw
    configEditor.revision = configuration?.revision ?? 0
  } catch (cause) {
    if (session === configSession) configEditor.error = errorMessage(cause, 'control.errors.configLoad')
  } finally {
    if (session === configSession) configEditor.loading = false
  }
}

function closeConfig() {
  if (configEditor.saving) return
  configSession += 1
  configEditor.open = false
}

async function saveConfig() {
  const row = configEditor.row
  if (!row || !configEditor.valid) return
  configEditor.saving = true
  configEditor.error = ''
  try {
    const configuration = await updateKernelInstallationConfig(row.id, configEditor.value, configEditor.revision)
    configEditor.revision = configuration?.revision ?? configEditor.revision
    configEditor.saving = false
    closeConfig()
    toast.success(t('control.messages.configSaved', { plugin: row.name }))
    await load({ silent: true })
    await refreshAdminExtensions(router)
  } catch (cause) {
    configEditor.error = errorMessage(cause, 'control.errors.configSave')
  } finally {
    configEditor.saving = false
  }
}

onMounted(() => {
  disposed = false
  void load()
})

onBeforeUnmount(() => {
  disposed = true
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
})
</script>

<style scoped>
.settings-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-12);
  min-width: 0;
}

.package-meta {
  display: block;
  overflow-wrap: anywhere;
}

.package-meta code {
  font-family: var(--font-mono);
}

.package-error {
  display: block;
  margin-top: var(--space-1);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  overflow-wrap: anywhere;
}

.package-actions {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  justify-content: flex-end;
}

.package-banner {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.package-banner ul {
  display: grid;
  gap: var(--space-1);
  padding-left: var(--space-5);
  margin: var(--space-2) 0 0;
}

.package-note {
  margin: 0;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.revision-label {
  margin-right: auto;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}
</style>
