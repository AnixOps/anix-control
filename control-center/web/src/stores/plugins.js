import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { kernelPluginsApi } from '@/api'

function asArray(value) {
  if (Array.isArray(value)) return value
  if (Array.isArray(value?.items)) return value.items
  return []
}

function operationKey(action, installationId, targetVersion = '') {
  const suffix = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `control-center:${action}:${installationId}:${targetVersion || 'current'}:${suffix}`
}

const terminalOperationStates = new Set([
  'succeeded', 'failed', 'canceled', 'cancelled', 'timed_out', 'timeout',
  'rolled_back', 'rollback_failed'
])

function wait(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds))
}

export const usePluginsStore = defineStore('plugins', () => {
  const plugins = ref([])
  const releases = ref([])
  const installations = ref([])
  const extensions = ref([])
  const operations = ref([])
  const loading = ref(false)
  const actionLoading = ref(false)
  const error = ref('')
  const lastOperation = ref(null)
  let stateGeneration = 0
  const pendingOperationKeys = new Map()

  const recentOperations = computed(() => operations.value.slice(0, 8))

  const installationMap = computed(() => new Map(
    installations.value.map((installation) => [
      `${installation.plugin_id}:${installation.target}`,
      installation
    ])
  ))

  function clearError() {
    error.value = ''
  }

  function clearState() {
    stateGeneration += 1
    plugins.value = []
    releases.value = []
    installations.value = []
    extensions.value = []
    operations.value = []
    lastOperation.value = null
    loading.value = false
    actionLoading.value = false
    pendingOperationKeys.clear()
    clearError()
  }

  async function refresh() {
    const generation = stateGeneration
    loading.value = true
    clearError()
    try {
      const [pluginRows, releaseRows, installationRows] = await Promise.all([
        kernelPluginsApi.list(),
        kernelPluginsApi.releases(),
        kernelPluginsApi.installations()
      ])
      // Extension discovery is optional. A missing trust root or disabled
      // WebUI must not hide the lifecycle catalog and installation state.
      let extensionRows = []
      try {
        extensionRows = await kernelPluginsApi.extensions()
      } catch {
        extensionRows = []
      }
      if (generation !== stateGeneration) return false
      plugins.value = asArray(pluginRows)
      releases.value = asArray(releaseRows)
      installations.value = asArray(installationRows)
      extensions.value = asArray(extensionRows)
      if (typeof kernelPluginsApi.operations === 'function') {
        try {
          const operationRows = asArray(await kernelPluginsApi.operations())
          if (generation !== stateGeneration) return false
          operations.value = operationRows
          if (lastOperation.value?.id) {
            const current = operationRows.find((operation) => operation.id === lastOperation.value.id)
            if (current) lastOperation.value = { ...lastOperation.value, ...current }
          }
        } catch {
          // Operation history is supplementary; lifecycle state remains useful
          // when an installation refresh succeeds but history is unavailable.
        }
      }
      return true
    } catch (requestError) {
      if (generation !== stateGeneration) return false
      error.value = requestError?.response?.data?.error?.message
        || requestError?.response?.data?.error
        || requestError?.message
        || 'Unable to load plugin state'
      return false
    } finally {
      if (generation === stateGeneration) loading.value = false
    }
  }

  function installation(pluginId, target = 'control') {
    return installationMap.value.get(`${pluginId}:${target}`)
  }

  function releasesFor(pluginId) {
    return releases.value.filter((release) => release.plugin_id === pluginId)
  }

  async function saveInstallation(payload) {
    const generation = stateGeneration
    clearError()
    try {
      const result = await kernelPluginsApi.upsertInstallation(payload)
      if (generation !== stateGeneration) return null
      const updated = result?.data || result
      if (result?.operation_id) {
        lastOperation.value = {
          id: result.operation_id,
          state: 'pending',
          operation_chain: result.operation_chain || result.operation_id
        }
      }
      const index = installations.value.findIndex((item) => item.id === updated?.id)
      if (index >= 0) installations.value[index] = updated
      else if (updated) installations.value.push(updated)
      return updated
    } catch (requestError) {
      if (generation !== stateGeneration) return null
      error.value = requestError?.response?.data?.error?.message
        || requestError?.response?.data?.error
        || requestError?.message
        || 'Unable to save plugin installation'
      throw requestError
    }
  }

  async function settleOperation(operationId, generation) {
    if (!operationId) return
    for (let attempt = 0; attempt < 4; attempt += 1) {
      if (generation !== stateGeneration) return
      const state = lastOperation.value?.id === operationId ? lastOperation.value.state : ''
      if (terminalOperationStates.has(state)) return
      if (attempt > 0) await wait(250)
      if (generation !== stateGeneration) return
      await refresh()
    }
  }

  async function runAction(installationRow, action, options = {}) {
    if (!installationRow?.id) throw new Error('A plugin installation is required')
    const generation = stateGeneration
    actionLoading.value = true
    clearError()
    const targetVersion = options.targetVersion || ''
    const operationIdentity = `${installationRow.id}:${action}:${targetVersion}`
    const explicitKey = Boolean(options.idempotencyKey)
    const idempotencyKey = options.idempotencyKey
      || pendingOperationKeys.get(operationIdentity)
      || operationKey(action, installationRow.id, targetVersion)
    if (!explicitKey) pendingOperationKeys.set(operationIdentity, idempotencyKey)
    try {
      const result = await kernelPluginsApi.action(installationRow.id, action, {
        ...options,
        idempotencyKey
      })
      if (generation !== stateGeneration) return result
      const operation = result?.operation || result
      lastOperation.value = operation && typeof operation === 'object'
        ? { ...operation, operation_chain: result?.operation_chain || operation.operation_chain || '' }
        : operation
      const index = installations.value.findIndex((item) => item.id === installationRow.id)
      if (index >= 0 && result?.installation) installations.value[index] = result.installation
      await refresh()
      await settleOperation(lastOperation.value?.id, generation)
      if (!explicitKey) pendingOperationKeys.delete(operationIdentity)
      return result
    } catch (requestError) {
      if (generation !== stateGeneration) return null
      const status = requestError?.response?.status
      const retryable = status === undefined || status === 408 || status === 425 || status === 429 || status >= 500
      if (!explicitKey && !retryable) pendingOperationKeys.delete(operationIdentity)
      error.value = requestError?.response?.data?.error?.message
        || requestError?.response?.data?.error
        || requestError?.message
        || `Unable to ${action} plugin`
      throw requestError
    } finally {
      if (generation === stateGeneration) actionLoading.value = false
    }
  }

  async function getConfig(installationId) {
    return kernelPluginsApi.getConfig(installationId)
  }

  async function updateConfig(installationId, config, expectedRevision) {
    const generation = stateGeneration
    clearError()
    try {
      const result = await kernelPluginsApi.updateConfig(installationId, config, expectedRevision)
      if (generation !== stateGeneration) return result
      if (result?.operation_id) {
        lastOperation.value = {
          id: result.operation_id,
          state: 'pending',
          operation_chain: result.operation_chain || result.operation_id
        }
      }
      await refresh()
      return result
    } catch (requestError) {
      if (generation !== stateGeneration) return null
      error.value = requestError?.response?.data?.error?.message
        || requestError?.response?.data?.error
        || requestError?.message
        || 'Unable to update plugin configuration'
      throw requestError
    }
  }

  return {
    plugins,
    releases,
    installations,
    extensions,
    operations,
    recentOperations,
    loading,
    actionLoading,
    error,
    lastOperation,
    installationMap,
    clearError,
    clearState,
    refresh,
    installation,
    releasesFor,
    saveInstallation,
    runAction,
    getConfig,
    updateConfig
  }
})
