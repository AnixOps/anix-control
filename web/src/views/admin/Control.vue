<template>
  <div class="control-page" :aria-busy="loading ? 'true' : 'false'">
    <div class="page-header">
      <h1>{{ t('pageTitles.admin.control') }}</h1>
      <button class="btn btn-sm" type="button" :disabled="loading" @click="load">
        {{ loading ? t('control.actions.refreshing') : t('control.actions.refresh') }}
      </button>
    </div>

    <div class="tabs" role="tablist" :aria-label="t('pageTitles.admin.control')">
      <button
        v-for="(item, index) in tabs"
        :id="`control-tab-${item.key}`"
        :key="item.key"
        class="tab"
        :class="{ active: tab === item.key }"
        type="button"
        role="tab"
        :aria-controls="`control-panel-${item.key}`"
        :aria-selected="tab === item.key"
        :tabindex="tab === item.key ? 0 : -1"
        @click="tab = item.key"
        @keydown="moveTab($event, index)"
      >
        {{ item.label }}
      </button>
    </div>

    <p v-if="error" class="error-message" role="alert">{{ error }}</p>

    <div
      v-show="tab === 'plugins'"
      id="control-panel-plugins"
      class="table-container"
      role="tabpanel"
      aria-labelledby="control-tab-plugins"
      tabindex="0"
    >
      <table class="data-table plugins-table">
        <thead>
          <tr>
            <th>{{ t('control.table.plugin') }}</th>
            <th>{{ t('control.table.publisher') }}</th>
            <th>{{ t('control.table.installation') }}</th>
            <th>{{ t('control.table.desiredVersion') }}</th>
            <th>{{ t('control.table.observedVersion') }}</th>
            <th>{{ t('control.table.state') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading && !loaded" class="state-row"><td colspan="6">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="row in pluginRows" v-else :key="row.key">
            <td>
              <span class="primary-cell">{{ row.plugin.name || row.plugin.id }}</span>
              <code class="secondary-cell">{{ row.plugin.id }}</code>
            </td>
            <td>{{ row.plugin.publisher || '-' }}</td>
            <td>{{ row.installation?.target || '-' }}</td>
            <td>{{ row.installation?.desired_version || '-' }}</td>
            <td>{{ row.installation?.observed_version || '-' }}</td>
            <td><span :class="['status-badge', stateClass(row.installation?.state)]">{{ row.installation?.state || t('control.states.catalogued') }}</span></td>
          </tr>
          <tr v-if="loaded && pluginRows.length === 0"><td colspan="6" class="empty-row">{{ t('control.empty.plugins') }}</td></tr>
        </tbody>
      </table>
    </div>

    <div
      v-show="tab === 'scopes'"
      id="control-panel-scopes"
      class="table-container"
      role="tabpanel"
      aria-labelledby="control-tab-scopes"
      tabindex="0"
    >
      <table class="data-table scopes-table">
        <thead><tr><th>{{ t('control.table.scope') }}</th><th>{{ t('control.table.owner') }}</th><th>{{ t('control.table.description') }}</th></tr></thead>
        <tbody>
          <tr v-if="loading && !loaded" class="state-row"><td colspan="3">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="scope in scopes" v-else :key="scope.id">
            <td><span class="primary-cell">{{ scope.name || scope.id }}</span><code class="secondary-cell">{{ scope.id }}</code></td>
            <td>{{ scope.plugin_id || '-' }}</td>
            <td class="description-cell">{{ scope.description || '-' }}</td>
          </tr>
          <tr v-if="loaded && scopes.length === 0"><td colspan="3" class="empty-row">{{ t('control.empty.scopes') }}</td></tr>
        </tbody>
      </table>
    </div>

    <div
      v-show="tab === 'topologies'"
      id="control-panel-topologies"
      class="table-container"
      role="tabpanel"
      aria-labelledby="control-tab-topologies"
      tabindex="0"
    >
      <table class="data-table topologies-table">
        <thead><tr><th>{{ t('control.table.topology') }}</th><th>{{ t('control.table.scope') }}</th><th>{{ t('control.table.activeRevision') }}</th><th>{{ t('control.table.description') }}</th></tr></thead>
        <tbody>
          <tr v-if="loading && !loaded" class="state-row"><td colspan="4">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="topology in topologies" v-else :key="topology.id">
            <td>{{ topology.name || '-' }}</td><td>{{ topology.service_scope || '-' }}</td><td>{{ topology.active_revision_id || '-' }}</td><td class="description-cell">{{ topology.description || '-' }}</td>
          </tr>
          <tr v-if="loaded && topologies.length === 0"><td colspan="4" class="empty-row">{{ t('control.empty.topologies') }}</td></tr>
        </tbody>
      </table>
    </div>

    <div
      v-show="tab === 'operations'"
      id="control-panel-operations"
      class="table-container"
      role="tabpanel"
      aria-labelledby="control-tab-operations"
      tabindex="0"
    >
      <table class="data-table operations-table">
        <thead><tr><th>{{ t('control.table.operation') }}</th><th>{{ t('control.table.plugin') }}</th><th>{{ t('control.table.revision') }}</th><th>{{ t('control.table.deadline') }}</th><th>{{ t('control.table.state') }}</th></tr></thead>
        <tbody>
          <tr v-if="loading && !loaded" class="state-row"><td colspan="5">{{ t('control.states.loading') }}</td></tr>
          <tr v-for="operation in operations" v-else :key="operation.id">
            <td><code class="identifier">{{ operation.kind || '-' }}</code></td><td class="identifier">{{ operation.plugin_id || '-' }}</td><td>{{ operation.revision ?? '-' }}</td><td>{{ formatDate(operation.deadline_at) }}</td>
            <td><span :class="['status-badge', stateClass(operation.state)]">{{ operation.state || '-' }}</span></td>
          </tr>
          <tr v-if="loaded && operations.length === 0"><td colspan="5" class="empty-row">{{ t('control.empty.operations') }}</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { getKernelInstallations, getKernelOperations, getKernelPlugins, getKernelScopes, getKernelTopologies } from '@/api/kernel'

const { t, formatDateTime } = useAppI18n()
const tab = ref('plugins')
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const plugins = ref([])
const installations = ref([])
const scopes = ref([])
const topologies = ref([])
const operations = ref([])

const tabs = computed(() => [
  { key: 'plugins', label: t('control.tabs.plugins') },
  { key: 'scopes', label: t('control.tabs.scopes') },
  { key: 'topologies', label: t('control.tabs.topologies') },
  { key: 'operations', label: t('control.tabs.operations') }
])

const pluginRows = computed(() => plugins.value.flatMap((plugin) => {
  const matches = installations.value.filter(item => item.plugin_id === plugin.id)
  if (matches.length === 0) {
    return [{ key: `${plugin.id}:catalogue`, plugin, installation: null }]
  }
  return matches.map(installation => ({
    key: `${plugin.id}:${installation.id || installation.target}`,
    plugin,
    installation
  }))
}))

function stateClass(state) {
  if (state === 'healthy' || state === 'enabled' || state === 'completed') return 'status-active'
  if (state === 'failed' || state === 'disabled' || state === 'cancel_requested') return 'status-error'
  return 'status-pending'
}

function formatDate(value) {
  if (!value) return '-'
  return formatDateTime(value, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }) || '-'
}

function moveTab(event, index) {
  let nextIndex = index
  if (event.key === 'ArrowRight') nextIndex = (index + 1) % tabs.value.length
  else if (event.key === 'ArrowLeft') nextIndex = (index - 1 + tabs.value.length) % tabs.value.length
  else if (event.key === 'Home') nextIndex = 0
  else if (event.key === 'End') nextIndex = tabs.value.length - 1
  else return

  event.preventDefault()
  tab.value = tabs.value[nextIndex].key
  document.getElementById(`control-tab-${tab.value}`)?.focus()
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [pluginRowsValue, installationRows, scopeRows, topologyRows, operationRows] = await Promise.all([
      getKernelPlugins(), getKernelInstallations(), getKernelScopes(), getKernelTopologies(), getKernelOperations()
    ])
    plugins.value = Array.isArray(pluginRowsValue) ? pluginRowsValue : []
    installations.value = Array.isArray(installationRows) ? installationRows : []
    scopes.value = Array.isArray(scopeRows) ? scopeRows : []
    topologies.value = Array.isArray(topologyRows) ? topologyRows : []
    operations.value = Array.isArray(operationRows) ? operationRows : []
    loaded.value = true
  } catch (cause) {
    error.value = cause?.response?.data?.error?.message || cause?.message || t('control.errors.load')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.control-page { display: grid; gap: 16px; min-width: 0; }
.page-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.page-header h1 { margin: 0; font-size: 24px; line-height: 1.25; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border-color, #d7dde7); overflow-x: auto; }
.tab { min-height: 40px; border: 0; border-bottom: 2px solid transparent; border-radius: 0; background: transparent; padding: 9px 12px; color: var(--text-secondary, #596579); cursor: pointer; white-space: nowrap; }
.tab:hover { background: var(--surface-hover); }
.tab.active { border-bottom-color: var(--primary-color, #2563eb); color: var(--text-color, #172033); font-weight: 600; }
.error-message { margin: 0; color: var(--error-color, #b42318); overflow-wrap: anywhere; }
.plugins-table { min-width: 820px; }
.scopes-table { min-width: 620px; }
.topologies-table { min-width: 680px; }
.operations-table { min-width: 760px; }
.primary-cell, .secondary-cell { display: block; }
.secondary-cell { width: fit-content; margin-top: 3px; color: var(--text-secondary); font-size: 11px; }
.description-cell { min-width: 220px; max-width: 520px; overflow-wrap: anywhere; }
.identifier { overflow-wrap: anywhere; }
.state-row td { color: var(--text-secondary); text-align: center; }
.status-active { background: rgba(22, 163, 74, 0.1); color: var(--success-color); }
.status-error { background: rgba(220, 38, 38, 0.1); color: var(--error-color); }
.status-pending { background: rgba(217, 119, 6, 0.1); color: var(--warning-color); }
@media (max-width: 640px) {
  .control-page { gap: 12px; }
  .page-header { align-items: flex-start; }
  .page-header h1 { font-size: 20px; }
  .tab { padding-inline: 10px; }
}
</style>
