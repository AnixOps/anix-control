<template>
  <section class="list-page plugin-center" :aria-busy="loading ? 'true' : 'false'">
    <UiPageHeader :title="t('pageTitles.admin.plugins')" :description="t('control.subtitle')">
      <template #actions>
        <UiIconButton
          :icon="RefreshCw"
          variant="secondary"
          data-testid="refresh-plugin-list"
          :label="t('control.actions.refresh')"
          :disabled="loading"
          @click="refreshPluginResources()"
        />
        <UiButton :icon="Waypoints" data-testid="open-route-modes" @click="router.push('/admin/plugins/route-modes')">
          {{ t('routeModes.open') }}
        </UiButton>
        <UiButton variant="primary" :icon="PackagePlus" data-testid="import-plugin-release" @click="openReleaseImport">
          {{ t('control.actions.importRelease') }}
        </UiButton>
      </template>
    </UiPageHeader>

    <div class="plugin-toolbar">
      <UiSearchField
        v-model.trim="search"
        class="list-page__search"
        data-testid="plugin-search"
        :label="t('control.pluginCenter.filters.search')"
      />
      <UiFilterChips
        v-model="healthFilter"
        data-testid="plugin-health-filter"
        :label="t('control.pluginCenter.filters.health')"
        :options="healthChips"
      />
      <UiFilterChips
        v-model="targetFilter"
        data-testid="plugin-target-filter"
        :label="t('control.pluginCenter.filters.target')"
        :options="targetChips"
      />
    </div>

    <p v-if="error" class="plugin-banner is-error" role="alert">{{ error }}</p>
    <p v-else-if="catalogError && rows.length" class="plugin-banner is-error" role="alert">{{ catalogError }}</p>
    <p v-if="notice" class="plugin-banner notice-message" role="status">{{ notice }}</p>
    <p v-if="lastPluginOperation" class="plugin-banner notice-message" data-testid="plugin-operation-status" role="status">
      {{ t('control.messages.operationStatus', {
        id: lastPluginOperation.id,
        state: lastPluginOperation.state || 'pending',
        chain: lastPluginOperation.operation_chain || lastPluginOperation.id,
      }) }}
    </p>
    <section v-if="adminExtensionErrors.length" class="plugin-banner is-error extension-error-band" data-testid="plugin-extension-errors" role="alert">
      <strong>{{ t('control.extensions.errorsTitle') }}</strong>
      <ul>
        <li v-for="(extensionError, index) in adminExtensionErrors" :key="`${extensionError.plugin_id || 'catalog'}-${index}`">
          <code v-if="extensionError.plugin_id">{{ extensionError.plugin_id }}</code>
          {{ extensionError.message }}
        </li>
      </ul>
    </section>

    <UiErrorState
      v-if="catalogError && !rows.length && !loading"
      :title="t('control.pluginCenter.loadFailed')"
      :error="catalogError"
      @retry="refreshPluginResources()"
    />
    <div v-else-if="showSkeleton" class="plugin-grid">
      <UiSkeleton v-for="index in 6" :key="index" variant="card" :label="index === 1 ? t('control.states.loading') : ''" />
    </div>
    <ul
      v-else-if="loaded && filteredRows.length"
      class="plugin-grid"
      data-testid="plugin-list"
      :aria-label="t('control.pluginCenter.listLabel')"
    >
      <li v-for="row in filteredRows" :key="row.key" class="plugin-grid__item">
        <button
          class="plugin-card"
          :data-testid="`plugin-row-${row.plugin.id}`"
          type="button"
          aria-haspopup="dialog"
          @click="openDrawer($event, row)"
        >
          <span class="plugin-card__head">
            <span class="plugin-card__icon" aria-hidden="true">
              <UiIcon :icon="Puzzle" :size="24" />
            </span>
            <span class="plugin-card__title">
              <strong class="plugin-card__name">{{ row.plugin.name || row.plugin.id }}</strong>
              <span class="plugin-card__publisher">
                {{ row.plugin.publisher || row.plugin.id }}
                <UiIcon v-if="row.plugin.official === true" :icon="BadgeCheck" :size="14" :label="t('control.pluginCenter.official')" />
              </span>
            </span>
          </span>
          <span v-if="row.plugin.description" class="plugin-card__description">{{ row.plugin.description }}</span>
          <span class="plugin-card__targets">
            <span v-for="target in row.targets" :key="target.target" class="plugin-card__target">
              {{ target.target }} <code>{{ target.installation?.desired_version || target.latestRelease?.version || '—' }}</code>
            </span>
          </span>
          <span class="plugin-card__foot">
            <UiBadge :tone="healthTone(row.health.state)" :label="healthLabel(row.health.state)" />
            <span class="plugin-card__version">
              <code>{{ row.latestRelease?.version || '—' }}</code>
              · {{ t('control.labels.releases', { count: row.releases.length }) }}
            </span>
          </span>
          <span v-if="row.health.error" class="plugin-card__error">{{ row.health.error }}</span>
        </button>
      </li>
    </ul>
    <UiEmptyState
      v-else-if="loaded && rows.length"
      :icon="SearchX"
      heading-tag="h2"
      :title="t('ui.table.noMatches')"
      :description="t('control.pluginCenter.empty')"
    >
      <template #actions>
        <UiButton data-clear-filters @click="clearFilters">{{ t('ui.table.clearFilters') }}</UiButton>
      </template>
    </UiEmptyState>
    <UiEmptyState
      v-else-if="loaded"
      :icon="Puzzle"
      heading-tag="h2"
      :title="t('control.pluginCenter.emptyCatalog.title')"
      :description="t('control.pluginCenter.emptyCatalog.description')"
    >
      <template #actions>
        <UiButton variant="primary" :icon="PackagePlus" @click="openReleaseImport">{{ t('control.actions.importRelease') }}</UiButton>
      </template>
    </UiEmptyState>

    <OperationTimeline
      :operations="pluginOperations"
      :scoped-operation-i-ds="pluginOperationIDs"
      :busy-operation-i-d="operationBusyID"
      :heading="t('control.pluginCenter.operations.title')"
      :empty-label="t('control.pluginCenter.operations.empty')"
      :show-toggle="false"
      :limit="8"
      data-testid="plugin-operation-history"
      @cancel="cancelOperation"
    />

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

    <UiDialog
      :open="configEditor.open"
      size="lg"
      data-testid="plugin-config-dialog"
      :title="t('control.config.title')"
      :description="`${selectedRow?.plugin?.name || selectedRow?.plugin?.id || ''} / ${configEditor.target?.target || ''}`"
      :dismissible="configCanClose"
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
          data-testid="save-plugin-config"
          :loading="configEditor.saving"
          :disabled="configEditor.loading || !configEditor.valid"
          @click="saveConfig()"
        >
          {{ t('common.actions.save') }}
        </UiButton>
      </template>
    </UiDialog>

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
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { BadgeCheck, PackagePlus, Puzzle, RefreshCw, SearchX, Waypoints } from '@lucide/vue'
import PluginConfigForm from '@/components/admin/PluginConfigForm.vue'
import PluginDetailDrawer from '@/components/admin/PluginDetailDrawer.vue'
import PluginInstallationDialog from '@/components/admin/PluginInstallationDialog.vue'
import PluginReleaseImportDialog from '@/components/admin/PluginReleaseImportDialog.vue'
import OperationTimeline from '@/components/admin/OperationTimeline.vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useListQuery } from '@/composables/useListQuery'
import { useKernelPlugins } from '@/composables/useKernelPlugins'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiEmptyState from '@/ui/UiEmptyState.vue'
import UiErrorState from '@/ui/UiErrorState.vue'
import UiFilterChips from '@/ui/UiFilterChips.vue'
import UiIcon from '@/ui/UiIcon.vue'
import UiIconButton from '@/ui/UiIconButton.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSearchField from '@/ui/UiSearchField.vue'
import UiSkeleton from '@/ui/UiSkeleton.vue'
import { useDelayedLoading } from '@/ui/composables/useDelayedLoading'
import {
  getKernelInstallationConfig,
  getKernelInstallations,
  getKernelOperations,
  cancelKernelOperation,
  registerKernelPluginRelease,
  runKernelInstallationAction,
  updateKernelInstallationConfig,
  uploadKernelPluginReleaseArtifact,
  upsertKernelInstallation,
} from '@/api/kernel'
import { adminExtensionErrors, refreshAdminExtensions } from '@/extensions/runtime'
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

// Search and the chips live in the URL query (plan §9).
const listQuery = useListQuery()
const search = ref(listQuery.read('q'))
const healthFilter = ref(listQuery.read('health', { values: ['healthy', 'attention', 'catalogued'] }))
const targetFilter = ref(listQuery.read('target', { values: ['control', 'agent'] }))
watch([search, healthFilter, targetFilter], () => listQuery.write({ q: search.value.trim(), health: healthFilter.value, target: targetFilter.value }))
const error = ref('')
const notice = ref('')
const selectedRow = ref(null)
const drawerOpen = ref(false)
const drawerTrigger = ref(null)
const busyTarget = ref('')
const operations = ref([])
const trackedOperationIDs = new Set()
const lastPluginOperation = ref(null)
const pendingIdempotencyKeys = new Map()
const operationBusyID = ref('')
const polling = ref(false)
const installationEditor = reactive({ open: false, target: null, saving: false, error: '' })
const configEditor = reactive({ open: false, target: null, schema: {}, value: {}, revision: 0, valid: true, loading: false, saving: false, error: '' })
const releaseImport = reactive({ open: false, saving: false, error: '', registeredRelease: null, inputKey: '', artifactUploaded: false })
const configCanClose = computed(() => !configEditor.saving)

let pollTimer = null
let pollRequestRunning = false
let disposed = false
let configEditorSession = 0

// Search and target narrow the catalog; the health chips count what is left.
const searchedRows = computed(() => rows.value.filter(row => {
  const query = search.value.toLocaleLowerCase()
  const searchable = [row.plugin?.name, row.plugin?.id, row.plugin?.description, row.health?.error]
    .filter(Boolean)
    .join(' ')
    .toLocaleLowerCase()
  const matchesSearch = !query || searchable.includes(query)
  const matchesTarget = !targetFilter.value || row.targets.some(target => target.target === targetFilter.value)
  return matchesSearch && matchesTarget
}))
const filteredRows = computed(() => searchedRows.value.filter(row => !healthFilter.value || row.health?.state === healthFilter.value))

const summary = computed(() => searchedRows.value.reduce((counts, row) => {
  if (row.health?.state in counts) counts[row.health.state] += 1
  return counts
}, { healthy: 0, attention: 0, catalogued: 0 }))
const healthChips = computed(() => [
  { value: 'healthy', label: t('control.pluginCenter.states.healthy'), count: summary.value.healthy },
  { value: 'attention', label: t('control.pluginCenter.states.attention'), count: summary.value.attention },
  { value: 'catalogued', label: t('control.states.catalogued'), count: summary.value.catalogued }
])
const targetChips = [
  { value: 'control', label: 'control' },
  { value: 'agent', label: 'agent' }
]
const showSkeleton = useDelayedLoading(() => loading.value && !loaded.value)

function clearFilters() {
  search.value = ''
  healthFilter.value = ''
  targetFilter.value = ''
}

function healthTone(state) {
  if (state === 'healthy') return 'success'
  if (state === 'attention') return 'warning'
  return 'neutral'
}

const pluginOperations = computed(() => operations.value.filter(isPluginLifecycleOperation))
const pluginOperationIDs = computed(() => pluginOperations.value.map(operation => operation.id).filter(Boolean))

function createIdempotencyToken() {
  return globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function isTransientLifecycleError(cause) {
  const status = cause?.response?.status
  return status === undefined || status === 408 || status === 425 || status === 429 || status >= 500
}

function lifecycleIdempotencyKey(installationID, action, targetVersion = '') {
  const identity = `${installationID}:${action}:${targetVersion || 'current'}`
  const existing = pendingIdempotencyKeys.get(identity)
  if (existing) return { identity, key: existing }
  const key = `webui:${installationID}:${action}:${targetVersion || 'current'}:${createIdempotencyToken()}`
  pendingIdempotencyKeys.set(identity, key)
  return { identity, key }
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
  if (!target || !selectedRow.value || selectedRow.value.plugin?.official !== true) return
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
  let operationIdentity = ''
  try {
    if (target.target === 'control') {
      const idempotency = lifecycleIdempotencyKey(target.installation.id, action, targetVersion)
      operationIdentity = idempotency.identity
      const result = await runKernelInstallationAction(target.installation.id, action, {
        targetVersion,
        idempotencyKey: idempotency.key,
      })
      const operation = result?.operation || result
      if (operation?.id) {
        trackedOperationIDs.add(operation.id)
        lastPluginOperation.value = {
          ...operation,
          operation_chain: result?.operation_chain || operation.operation_chain || operation.id,
        }
      }
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
    if (operationIdentity) pendingIdempotencyKeys.delete(operationIdentity)
    return true
  } catch (cause) {
    if (operationIdentity && !isTransientLifecycleError(cause)) pendingIdempotencyKeys.delete(operationIdentity)
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

async function cancelOperation(operationID) {
  operationBusyID.value = operationID
  error.value = ''
  notice.value = ''
  try {
    const result = await cancelKernelOperation(operationID)
    const operation = result?.operation || result
    if (operation?.id) {
      trackedOperationIDs.add(operation.id)
      lastPluginOperation.value = {
        ...operation,
        operation_chain: result?.operation_chain || operation.operation_chain || operation.id,
      }
    }
    await refreshOperationState()
    notice.value = t('control.messages.cancelRequested')
  } catch (cause) {
    error.value = errorMessage(cause, 'control.errors.cancel')
  } finally {
    operationBusyID.value = ''
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
  pendingIdempotencyKeys.clear()
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
})
</script>

<style scoped>
.plugin-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
}

.plugin-banner {
  margin: 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--fill-1);
  color: var(--label-1);
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.plugin-banner.is-error {
  background: var(--danger-soft);
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
}

.extension-error-band ul {
  display: grid;
  gap: var(--space-1);
  padding-left: var(--space-5);
  margin: var(--space-2) 0 0;
}

/* App Store style catalogue: cards of 280 px and up. */
.plugin-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--space-4);
  padding: 0;
  margin: 0;
  list-style: none;
}

.plugin-grid__item {
  display: flex;
  min-width: 0;
}

.plugin-card {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-3);
  align-items: stretch;
  min-width: 0;
  min-height: 0;
  padding: var(--space-5);
  border: 0;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
  color: var(--label-1);
  font: inherit;
  text-align: left;
  white-space: normal;
  cursor: pointer;
  transition: box-shadow var(--dur-micro) var(--ease-standard), transform var(--dur-micro) var(--ease-standard);
}

.plugin-card:hover {
  background: var(--bg-elevated);
  box-shadow: var(--shadow-2), 0 0 0 0.5px var(--separator);
  transform: translateY(-1px);
}

.plugin-card:active {
  background: var(--bg-elevated);
  transform: none;
}

.plugin-card:focus-visible {
  outline: var(--focus-ring);
  outline-offset: var(--focus-ring-offset);
}

.plugin-card__head {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  min-width: 0;
}

.plugin-card__icon {
  display: grid;
  flex: none;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: var(--radius-sm);
  background: var(--accent-soft);
  color: var(--accent);
}

.plugin-card__title {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.plugin-card__name {
  font-size: var(--type-title-3-size);
  font-weight: var(--weight-semibold);
  line-height: var(--type-title-3-line);
  overflow-wrap: anywhere;
}

.plugin-card__publisher {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

.plugin-card__publisher :deep(svg) {
  color: var(--accent);
}

.plugin-card__description {
  display: -webkit-box;
  overflow: hidden;
  color: var(--label-2);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.plugin-card__targets {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.plugin-card__target {
  display: inline-flex;
  gap: var(--space-1);
  align-items: center;
  padding: var(--space-0-5) var(--space-2);
  border-radius: var(--radius-xs);
  background: var(--fill-1);
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.plugin-card__target code,
.plugin-card__version code {
  color: var(--label-1);
  font-family: var(--font-mono);
}

.plugin-card__foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  justify-content: space-between;
  margin-top: auto;
}

.plugin-card__version {
  color: var(--label-2);
  font-size: var(--type-caption-size);
}

.plugin-card__error {
  color: color-mix(in srgb, var(--danger) 78%, var(--label-1));
  font-size: var(--type-callout-size);
  overflow-wrap: anywhere;
}

.revision-label {
  margin-right: auto;
  color: var(--label-2);
  font-size: var(--type-callout-size);
}

@media (prefers-reduced-motion: reduce) {
  .plugin-card:hover {
    transform: none;
  }
}
</style>
