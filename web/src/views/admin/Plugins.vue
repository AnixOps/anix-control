<template>
  <section class="plugin-center" :aria-busy="loading ? 'true' : 'false'">
    <header class="page-header">
      <div>
        <h1>{{ t('pageTitles.admin.plugins') }}</h1>
        <p class="page-subtitle">{{ t('control.subtitle') }}</p>
      </div>
      <div class="header-actions">
        <button
          class="icon-button"
          data-testid="refresh-plugin-list"
          type="button"
          :aria-label="t('control.actions.refresh')"
          :title="t('control.actions.refresh')"
          :disabled="loading"
          @click="refreshPluginResources()"
        >
          <RefreshCw :class="{ spinning: loading }" :size="18" aria-hidden="true" />
          <span class="sr-only">{{ t('control.actions.refresh') }}</span>
        </button>
        <button class="btn btn-primary" data-testid="import-plugin-release" type="button" @click="openReleaseImport">
          {{ t('control.actions.importRelease') }}
        </button>
      </div>
    </header>

    <div class="plugin-toolbar" role="search" :aria-label="t('pageTitles.admin.plugins')">
      <label class="search-field" for="plugin-search">
        <Search :size="17" aria-hidden="true" />
        <span class="sr-only">{{ t('control.pluginCenter.filters.search') }}</span>
        <input
          id="plugin-search"
          v-model.trim="search"
          data-testid="plugin-search"
          type="search"
          :placeholder="t('control.pluginCenter.filters.search')"
        />
      </label>
      <label class="filter-field" for="plugin-health-filter">
        <span>{{ t('control.pluginCenter.filters.health') }}</span>
        <select id="plugin-health-filter" v-model="healthFilter" data-testid="plugin-health-filter">
          <option value="all">{{ t('control.pluginCenter.filters.allHealth') }}</option>
          <option value="healthy">{{ t('control.pluginCenter.states.healthy') }}</option>
          <option value="attention">{{ t('control.pluginCenter.states.attention') }}</option>
          <option value="catalogued">{{ t('control.states.catalogued') }}</option>
        </select>
      </label>
      <label class="filter-field" for="plugin-target-filter">
        <span>{{ t('control.pluginCenter.filters.target') }}</span>
        <select id="plugin-target-filter" v-model="targetFilter" data-testid="plugin-target-filter">
          <option value="all">{{ t('control.pluginCenter.filters.allTargets') }}</option>
          <option value="control">control</option>
          <option value="agent">agent</option>
        </select>
      </label>
    </div>

    <div class="plugin-summary" :aria-label="t('control.pluginCenter.summary.label')">
      <span class="summary-item healthy">{{ t('control.pluginCenter.summary.healthy', { count: summary.healthy }) }}</span>
      <span class="summary-item attention">{{ t('control.pluginCenter.summary.attention', { count: summary.attention }) }}</span>
      <span class="summary-item catalogued">{{ t('control.pluginCenter.summary.catalogued', { count: summary.catalogued }) }}</span>
    </div>

    <p v-if="pageError" class="error-message" role="alert">{{ pageError }}</p>
    <p v-if="notice" class="notice-message" role="status">{{ notice }}</p>

    <div class="plugin-list" data-testid="plugin-list" role="list">
      <p v-if="loading && !loaded" class="state-message">{{ t('control.states.loading') }}</p>
      <template v-else>
        <div
          v-for="row in filteredRows"
          :key="row.key"
          role="listitem"
        >
          <button
            class="plugin-row"
            :data-testid="`plugin-row-${row.plugin.id}`"
            type="button"
            aria-haspopup="dialog"
            @click="openDrawer($event, row)"
          >
            <span class="plugin-primary">
              <strong>{{ row.plugin.name || row.plugin.id }}</strong>
              <code>{{ row.plugin.id }}</code>
              <span v-if="row.plugin.description" class="plugin-description">{{ row.plugin.description }}</span>
            </span>
            <span class="target-summary">
              <span v-for="target in row.targets" :key="target.target" class="target-chip">
                <span>{{ target.target }}</span>
                <code>{{ target.installation?.desired_version || target.latestRelease?.version || '-' }}</code>
              </span>
              <span v-if="row.targets.length === 0" class="muted">{{ t('control.states.catalogued') }}</span>
            </span>
            <span class="release-summary">
              <code>{{ row.latestRelease?.version || '-' }}</code>
              <span>{{ t('control.labels.releases', { count: row.releases.length }) }}</span>
            </span>
            <span :class="['health-badge', `health-${row.health.state}`]">{{ healthLabel(row.health.state) }}</span>
            <span v-if="row.health.error" class="row-error">{{ row.health.error }}</span>
          </button>
        </div>
        <p v-if="loaded && filteredRows.length === 0" class="state-message">{{ t('control.pluginCenter.empty') }}</p>
      </template>
    </div>

    <PluginDetailDrawer
      :open="drawerOpen"
      :row="selectedRow"
      :busy-target="busyTarget"
      @close="closeDrawer"
      @install="openInstallation"
      @configure="openConfig"
      @lifecycle="handleLifecycle"
    />

    <PluginInstallationDialog
      :open="installationEditor.open"
      :plugin="selectedRow?.plugin"
      :target="installationEditor.target?.target"
      :targets="installationEditor.target ? [installationEditor.target.target] : []"
      :releases="selectedRow?.releases || []"
      :installation="installationEditor.target?.installation"
      :saving="installationEditor.saving"
      :error="installationEditor.error"
      @close="closeInstallation"
      @save="saveInstallation"
    />

    <div v-if="configEditor.open" class="plugin-config-backdrop" @click.self="closeConfig">
      <section
        ref="configDialog"
        class="plugin-config-dialog"
        data-testid="plugin-config-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="plugin-config-title"
        @keydown="handleConfigKeydown"
      >
        <header class="dialog-header">
          <div>
            <h2 id="plugin-config-title">{{ t('control.config.title') }}</h2>
            <p class="dialog-meta">{{ selectedRow?.plugin?.name || selectedRow?.plugin?.id }} / {{ configEditor.target?.target }}</p>
          </div>
          <button
            ref="configCloseButton"
            class="icon-button"
            type="button"
            :aria-label="t('common.actions.close')"
            :title="t('common.actions.close')"
            :disabled="configEditor.saving"
            @click="requestConfigClose"
          >
            <X :size="20" aria-hidden="true" />
            <span class="sr-only">{{ t('common.actions.close') }}</span>
          </button>
        </header>
        <div class="dialog-body">
          <p v-if="configEditor.loading" class="state-message">{{ t('control.config.loading') }}</p>
          <PluginConfigForm
            v-else
            v-model="configEditor.value"
            :schema="configEditor.schema"
            @validity="configEditor.valid = $event"
          />
          <p v-if="configEditor.error" class="dialog-error" role="alert">{{ configEditor.error }}</p>
        </div>
        <footer class="dialog-footer">
          <span class="revision-label">{{ t('control.config.revision', { revision: configEditor.revision }) }}</span>
          <button class="btn" type="button" :disabled="configEditor.saving" @click="requestConfigClose">{{ t('common.actions.cancel') }}</button>
          <button
            class="btn btn-primary"
            data-testid="save-plugin-config"
            type="button"
            :disabled="configEditor.loading || configEditor.saving || !configEditor.valid"
            @click="saveConfig()"
          >
            {{ configEditor.saving ? t('control.actions.saving') : t('common.actions.save') }}
          </button>
        </footer>
      </section>
    </div>

    <PluginReleaseImportDialog
      :open="releaseImport.open"
      :saving="releaseImport.saving"
      :error="releaseImport.error"
      @close="closeReleaseImport"
      @save="importRelease"
    />
  </section>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { RefreshCw, Search, X } from '@lucide/vue'
import PluginConfigForm from '@/components/admin/PluginConfigForm.vue'
import PluginDetailDrawer from '@/components/admin/PluginDetailDrawer.vue'
import PluginInstallationDialog from '@/components/admin/PluginInstallationDialog.vue'
import PluginReleaseImportDialog from '@/components/admin/PluginReleaseImportDialog.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useKernelPlugins } from '@/composables/useKernelPlugins'
import { useModalFocus } from '@/composables/useModalFocus'
import {
  getKernelInstallationConfig,
  getKernelInstallations,
  getKernelOperations,
  registerKernelPluginRelease,
  runKernelInstallationAction,
  updateKernelInstallationConfig,
  uploadKernelPluginReleaseArtifact,
  upsertKernelInstallation,
} from '@/api/kernel'
import { refreshAdminExtensions } from '@/extensions/runtime'
import { parseManifest } from '@/utils/kernelPluginRelease'

const TERMINAL_OPERATION_STATES = new Set(['succeeded', 'completed', 'failed', 'superseded', 'cancelled', 'timed_out', 'expired', 'rolled_back'])
const POLL_INTERVAL_MS = 2000

const { t } = useAppI18n()
const router = useRouter()
const {
  installations,
  rows,
  loading,
  loaded,
  error: catalogError,
  load,
} = useKernelPlugins()

const search = ref('')
const healthFilter = ref('all')
const targetFilter = ref('all')
const error = ref('')
const notice = ref('')
const selectedRow = ref(null)
const drawerOpen = ref(false)
const drawerTrigger = ref(null)
const busyTarget = ref('')
const operations = ref([])
const trackedOperationIDs = new Set()
const polling = ref(false)
const installationEditor = reactive({ open: false, target: null, saving: false, error: '' })
const configEditor = reactive({ open: false, target: null, schema: {}, value: {}, revision: 0, valid: true, loading: false, saving: false, error: '' })
const releaseImport = reactive({ open: false, saving: false, error: '', registeredRelease: null, inputKey: '', artifactUploaded: false })
const configDialog = ref(null)
const configCloseButton = ref(null)
const configOpen = computed(() => configEditor.open)
const configCanClose = computed(() => !configEditor.saving)

let pollTimer = null
let pollRequestRunning = false
let disposed = false
let configEditorSession = 0

const filteredRows = computed(() => rows.value.filter(row => {
  const query = search.value.toLocaleLowerCase()
  const searchable = [row.plugin?.name, row.plugin?.id, row.plugin?.description, row.health?.error]
    .filter(Boolean)
    .join(' ')
    .toLocaleLowerCase()
  const matchesSearch = !query || searchable.includes(query)
  const matchesHealth = healthFilter.value === 'all' || row.health?.state === healthFilter.value
  const matchesTarget = targetFilter.value === 'all' || row.targets.some(target => target.target === targetFilter.value)
  return matchesSearch && matchesHealth && matchesTarget
}))

const summary = computed(() => filteredRows.value.reduce((counts, row) => {
  counts[row.health?.state] += 1
  return counts
}, { healthy: 0, attention: 0, catalogued: 0 }))

const pageError = computed(() => error.value || catalogError.value)

const { handleKeydown: handleConfigKeydown, requestClose: requestConfigClose } = useModalFocus({
  open: configOpen,
  canClose: configCanClose,
  container: configDialog,
  initialFocus: configCloseButton,
  close: () => closeConfig(),
})

function createIdempotencyToken() {
  return globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function errorMessage(cause, fallbackKey) {
  const response = cause?.response?.data
  return response?.error?.message || (typeof response?.error === 'string' ? response.error : '') || response?.message || response?.msg || cause?.message || t(fallbackKey)
}

function healthLabel(state) {
  if (state === 'healthy') return t('control.pluginCenter.states.healthy')
  if (state === 'attention') return t('control.pluginCenter.states.attention')
  return t('control.states.catalogued')
}

function schemaFor(target) {
  const release = target?.releases?.find(item => item.version === target.installation?.desired_version)
  const schema = parseManifest(release).config_schema
  return schema && typeof schema === 'object' && !Array.isArray(schema) ? schema : {}
}

function synchronizeSelectedRow() {
  const pluginID = selectedRow.value?.plugin?.id
  if (!pluginID) return
  const nextRow = rows.value.find(row => row.plugin?.id === pluginID) || null
  selectedRow.value = nextRow
  if (!nextRow) drawerOpen.value = false
}

async function refreshPluginResources(options = {}) {
  const refreshed = await load(options)
  if (!refreshed) return false
  synchronizeSelectedRow()
  return true
}

function openDrawer(event, row) {
  drawerTrigger.value = event.currentTarget
  selectedRow.value = row
  drawerOpen.value = true
}

function closeDrawer() {
  if (busyTarget.value) return
  drawerOpen.value = false
  nextTick(() => drawerTrigger.value?.focus?.())
}

function openInstallation(target) {
  if (!target || !selectedRow.value) return
  Object.assign(installationEditor, { open: true, target, saving: false, error: '' })
}

function closeInstallation() {
  if (!installationEditor.saving) installationEditor.open = false
}

async function afterPluginMutation(message) {
  if (!await refreshPluginResources({ silent: true })) {
    throw new Error(catalogError.value || t('control.errors.load'))
  }
  await refreshOperationState()
  await refreshAdminExtensions(router)
  notice.value = message
}

async function runLifecycle(target, action, targetVersion = '') {
  if (!target?.installation || !selectedRow.value) return false
  busyTarget.value = target.target
  error.value = ''
  notice.value = ''
  try {
    if (target.target === 'control') {
      const result = await runKernelInstallationAction(target.installation.id, action, {
        targetVersion,
        idempotencyKey: `webui:${target.installation.id}:${action}:${createIdempotencyToken()}`,
      })
      if (result?.operation?.id) trackedOperationIDs.add(result.operation.id)
    } else {
      await upsertKernelInstallation({
        plugin_id: selectedRow.value.plugin.id,
        target: target.target,
        desired_version: action === 'rollback' ? target.installation.previous_version : targetVersion || target.installation.desired_version,
        enabled: action !== 'disable',
      })
    }
    await afterPluginMutation(t('control.messages.actionQueued', {
      action: t(`control.actions.${action}`),
      plugin: selectedRow.value.plugin.name || selectedRow.value.plugin.id,
    }))
    return true
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.action')
    return false
  } finally {
    busyTarget.value = ''
  }
}

function handleLifecycle({ installation, action }) {
  const target = selectedRow.value?.targets.find(item => item.installation?.id === installation?.id)
  if (target) void runLifecycle(target, action)
}

async function saveInstallation(input) {
  const target = installationEditor.target
  if (!target || !selectedRow.value) return
  installationEditor.saving = true
  installationEditor.error = ''
  error.value = ''
  notice.value = ''
  try {
    if (target.installation) {
      if (!await runLifecycle(target, 'update', input.version)) {
        installationEditor.error = error.value
        return
      }
    } else {
      await upsertKernelInstallation({
        plugin_id: selectedRow.value.plugin.id,
        target: input.target,
        desired_version: input.version,
        enabled: input.enabled,
      })
      await afterPluginMutation(t('control.messages.installed', { plugin: selectedRow.value.plugin.name || selectedRow.value.plugin.id }))
    }
    installationEditor.open = false
  } catch (cause) {
    installationEditor.error = errorMessage(cause, 'control.errors.install')
  } finally {
    installationEditor.saving = false
  }
}

async function openConfig(target) {
  if (!target?.installation) return
  const session = ++configEditorSession
  const installationID = target.installation.id
  Object.assign(configEditor, {
    open: true,
    target,
    schema: schemaFor(target),
    value: {},
    revision: target.installation.config_revision || 0,
    valid: true,
    loading: true,
    saving: false,
    error: '',
  })
  try {
    const configuration = await getKernelInstallationConfig(installationID)
    if (!isCurrentConfigSession(session, installationID)) return
    const rawConfig = configuration?.config ?? {}
    configEditor.value = typeof rawConfig === 'string' ? JSON.parse(rawConfig || '{}') : rawConfig
    configEditor.revision = configuration?.revision ?? target.installation.config_revision ?? 0
  } catch (cause) {
    if (isCurrentConfigSession(session, installationID)) {
      configEditor.error = errorMessage(cause, 'control.errors.configLoad')
    }
  } finally {
    if (isCurrentConfigSession(session, installationID)) configEditor.loading = false
  }
}

function isCurrentConfigSession(session, installationID) {
  return configEditor.open && configEditorSession === session && configEditor.target?.installation?.id === installationID
}

function closeConfig() {
  if (!configEditor.saving) {
    configEditorSession += 1
    configEditor.open = false
  }
}

async function saveConfig(input = configEditor.value) {
  const target = configEditor.target
  if (!target?.installation || !configEditor.valid) return
  configEditor.saving = true
  configEditor.error = ''
  try {
    const configuration = await updateKernelInstallationConfig(target.installation.id, input, configEditor.revision)
    configEditor.revision = configuration?.revision ?? configEditor.revision
    await afterPluginMutation(t('control.messages.configSaved', { plugin: selectedRow.value?.plugin?.name || selectedRow.value?.plugin?.id }))
    configEditor.open = false
  } catch (cause) {
    configEditor.error = errorMessage(cause, 'control.errors.configSave')
  } finally {
    configEditor.saving = false
  }
}

function openReleaseImport() {
  Object.assign(releaseImport, { open: true, saving: false, error: '', registeredRelease: null, inputKey: '', artifactUploaded: false })
}

function closeReleaseImport() {
  if (!releaseImport.saving) {
    Object.assign(releaseImport, { open: false, registeredRelease: null, inputKey: '', artifactUploaded: false })
  }
}

async function importRelease(input) {
  releaseImport.saving = true
  releaseImport.error = ''
  error.value = ''
  notice.value = ''
  try {
    JSON.parse(input.manifest)
    const inputKey = `${input.manifest}\u0000${input.signature}`
    if (!releaseImport.registeredRelease || releaseImport.inputKey !== inputKey) {
      releaseImport.registeredRelease = await registerKernelPluginRelease(input.manifest, input.signature)
      releaseImport.inputKey = inputKey
      releaseImport.artifactUploaded = false
    }
    const release = releaseImport.registeredRelease
    if (input.artifactBase64 && !releaseImport.artifactUploaded) {
      await uploadKernelPluginReleaseArtifact(release.id, input.artifactBase64)
      releaseImport.artifactUploaded = true
    }
    await afterPluginMutation(t('control.messages.releaseImported', { plugin: release.plugin_id, version: release.version }))
    Object.assign(releaseImport, { open: false, registeredRelease: null, inputKey: '', artifactUploaded: false })
  } catch (cause) {
    releaseImport.error = errorMessage(cause, 'control.errors.releaseImport')
  } finally {
    releaseImport.saving = false
  }
}

function isPluginLifecycleOperation(operation) {
  return typeof operation?.kind === 'string' && operation.kind.startsWith('plugin.')
}

function hasActiveOperations() {
  for (const operationID of trackedOperationIDs) {
    const operation = operations.value.find(item => item.id === operationID)
    if (operation && !TERMINAL_OPERATION_STATES.has(operation.state)) return true
  }
  return operations.value.some(operation => isPluginLifecycleOperation(operation) && !TERMINAL_OPERATION_STATES.has(operation.state))
}

function reconcileTrackedOperations() {
  for (const operationID of [...trackedOperationIDs]) {
    const operation = operations.value.find(item => item.id === operationID)
    if (!operation || TERMINAL_OPERATION_STATES.has(operation.state)) trackedOperationIDs.delete(operationID)
  }
}

function updatePolling() {
  if (disposed) {
    polling.value = false
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
    return
  }
  const shouldPoll = hasActiveOperations()
  polling.value = shouldPoll
  if (shouldPoll && !pollTimer) {
    pollTimer = setInterval(pollOperations, POLL_INTERVAL_MS)
  } else if (!shouldPoll && pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function loadOperationState() {
  try {
    await refreshOperationState()
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.poll')
  }
}

async function refreshOperationState() {
  const operationRows = await getKernelOperations()
  operations.value = Array.isArray(operationRows) ? operationRows : []
  reconcileTrackedOperations()
  updatePolling()
}

async function pollOperations() {
  if (pollRequestRunning || disposed) return
  pollRequestRunning = true
  try {
    const [operationRows, installationRows] = await Promise.all([getKernelOperations(), getKernelInstallations()])
    operations.value = Array.isArray(operationRows) ? operationRows : []
    installations.value = Array.isArray(installationRows) ? installationRows : []
    synchronizeSelectedRow()
    reconcileTrackedOperations()
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.poll')
  } finally {
    pollRequestRunning = false
  }
  updatePolling()
}

onMounted(() => {
  disposed = false
  void refreshPluginResources()
  void loadOperationState()
})

onBeforeUnmount(() => {
  disposed = true
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
})
</script>

<style scoped>
.plugin-center { display: grid; gap: 16px; min-width: 0; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
.page-header h1 { margin: 0; font-size: 24px; line-height: 1.25; }
.page-subtitle, .dialog-meta { margin: 5px 0 0; color: var(--text-secondary); font-size: 13px; }
.header-actions, .plugin-toolbar { display: flex; align-items: end; gap: 8px; flex-wrap: wrap; }
.icon-button, .btn { min-height: 36px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; }
.icon-button { display: inline-grid; width: 36px; place-items: center; padding: 0; }
.btn { padding: 8px 12px; }
.btn-primary { border-color: var(--primary-color); background: var(--primary-color); color: #fff; }
.icon-button:disabled, .btn:disabled { cursor: not-allowed; opacity: .55; }
.spinning { animation: plugin-spin .8s linear infinite; }
@keyframes plugin-spin { to { transform: rotate(360deg); } }
.plugin-toolbar { padding: 12px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); }
.search-field { display: flex; align-items: center; gap: 8px; min-width: min(280px, 100%); flex: 1 1 280px; border: 1px solid var(--border-color); border-radius: 6px; padding: 0 10px; background: var(--bg-color); color: var(--text-secondary); }
.search-field input { min-width: 0; width: 100%; height: 36px; border: 0; outline: 0; background: transparent; color: var(--text-color); font: inherit; }
.filter-field { display: grid; gap: 5px; color: var(--text-secondary); font-size: 12px; font-weight: 700; }
.filter-field select { min-height: 36px; border: 1px solid var(--border-color); border-radius: 6px; background: var(--bg-color); color: var(--text-color); font: inherit; padding: 6px 8px; }
.plugin-summary { display: flex; gap: 12px; flex-wrap: wrap; color: var(--text-secondary); font-size: 12px; }
.summary-item { display: inline-flex; align-items: center; gap: 6px; }
.summary-item::before { width: 7px; height: 7px; border-radius: 50%; background: currentColor; content: ''; }
.healthy { color: var(--success-color); }
.attention { color: var(--warning-color); }
.catalogued { color: var(--text-secondary); }
.error-message, .notice-message, .dialog-error { margin: 0; overflow-wrap: anywhere; }
.error-message, .dialog-error, .row-error { color: var(--error-color); }
.notice-message { color: var(--success-color); }
.plugin-list { display: grid; gap: 8px; }
.plugin-row { display: grid; grid-template-columns: minmax(220px, 1.55fr) minmax(170px, 1fr) minmax(110px, .6fr) auto; align-items: center; gap: 14px; width: 100%; border: 1px solid var(--border-color); border-radius: 6px; background: var(--surface-color); color: var(--text-color); cursor: pointer; padding: 12px 14px; text-align: left; }
.plugin-row:hover, .plugin-row:focus-visible { border-color: var(--primary-color); outline: 2px solid transparent; background: var(--surface-hover); }
.plugin-primary, .release-summary { display: grid; gap: 3px; min-width: 0; }
.plugin-primary strong { overflow-wrap: anywhere; }
.plugin-primary code, .release-summary span, .plugin-description, .muted { color: var(--text-secondary); font-size: 12px; overflow-wrap: anywhere; }
.target-summary { display: flex; flex-wrap: wrap; gap: 6px; }
.target-chip { display: inline-flex; align-items: center; gap: 5px; border: 1px solid var(--border-color); border-radius: 999px; padding: 3px 7px; color: var(--text-secondary); font-size: 11px; }
.target-chip code { color: var(--text-color); }
.health-badge { display: inline-flex; align-items: center; justify-content: center; min-height: 24px; border-radius: 999px; padding: 2px 8px; font-size: 11px; font-weight: 700; white-space: nowrap; }
.health-healthy { background: rgba(22, 163, 74, .1); color: var(--success-color); }
.health-attention { background: rgba(217, 119, 6, .1); color: var(--warning-color); }
.health-catalogued { background: var(--surface-hover); color: var(--text-secondary); }
.row-error { grid-column: 1 / -1; font-size: 12px; line-height: 1.4; }
.state-message { margin: 0; padding: 24px 12px; color: var(--text-secondary); text-align: center; }
.plugin-config-backdrop { position: fixed; inset: 0; z-index: 1300; display: flex; align-items: center; justify-content: center; padding: 20px; background: rgba(15, 23, 42, .62); }
.plugin-config-dialog { width: min(100%, 760px); max-height: calc(100dvh - 40px); overflow-y: auto; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface-color); color: var(--text-color); box-shadow: 0 18px 32px rgba(15, 23, 42, .2); }
.dialog-header, .dialog-footer { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid var(--border-color); }
.dialog-header h2 { margin: 0; font-size: 18px; line-height: 1.35; }
.dialog-body { display: grid; gap: 14px; padding: 20px; }
.dialog-footer { align-items: center; justify-content: flex-end; border-top: 1px solid var(--border-color); border-bottom: 0; }
.revision-label { margin-right: auto; color: var(--text-secondary); font-size: 12px; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 760px) {
  .plugin-center { gap: 12px; }
  .page-header h1 { font-size: 20px; }
  .header-actions { width: 100%; }
  .header-actions .btn { flex: 1; }
  .plugin-toolbar { align-items: stretch; }
  .filter-field { flex: 1 1 140px; }
  .plugin-row { grid-template-columns: 1fr; gap: 9px; }
  .row-error { grid-column: auto; }
  .plugin-config-backdrop { padding: 0; }
  .plugin-config-dialog { width: 100vw; max-height: none; min-height: 100dvh; border: 0; border-radius: 0; display: flex; flex-direction: column; }
  .dialog-body { flex: 1; align-content: start; }
}
</style>
