<template>
  <section class="space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-sm text-dark-400">Anix Control Kernel</p>
        <h1 class="text-2xl font-bold text-white">Plugins</h1>
        <p class="mt-1 text-sm text-dark-400">Signed plugin releases and lifecycle state</p>
      </div>
      <div v-if="authStore.isKernelAuthenticated" class="flex items-center gap-2">
        <button
          class="inline-flex items-center gap-2 rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="store.loading"
          @click="store.refresh"
        >
          <ArrowPathIcon class="h-4 w-4" :class="store.loading ? 'animate-spin' : ''" />
          Refresh
        </button>
        <button class="rounded-lg border border-dark-600 px-4 py-2 text-sm text-dark-200 hover:border-dark-400" @click="authStore.disconnectKernel()">Disconnect</button>
      </div>
    </div>

    <form v-if="!authStore.isKernelAuthenticated" class="max-w-md space-y-4" @submit.prevent="connectControl">
      <h2 class="text-lg font-semibold text-white">Connect to Anix Control</h2>
      <label class="block text-sm text-dark-300">Control email
        <input v-model="connectEmail" type="email" autocomplete="username" required class="mt-1 w-full rounded-lg border border-dark-600 bg-dark-800 px-3 py-2 text-white" />
      </label>
      <label class="block text-sm text-dark-300">Control password
        <input v-model="connectPassword" type="password" autocomplete="current-password" required class="mt-1 w-full rounded-lg border border-dark-600 bg-dark-800 px-3 py-2 text-white" />
      </label>
      <template v-if="mfaRequired">
        <label v-if="mfaMethods.length > 1" class="block text-sm text-dark-300">Verification method
          <select v-model="mfaMethod" class="mt-1 w-full rounded-lg border border-dark-600 bg-dark-800 px-3 py-2 text-white">
            <option v-for="method in mfaMethods" :key="method" :value="method">{{ method }}</option>
          </select>
        </label>
        <label class="block text-sm text-dark-300">Verification code
          <input v-model="mfaCode" type="text" autocomplete="one-time-code" required class="mt-1 w-full rounded-lg border border-dark-600 bg-dark-800 px-3 py-2 text-white" />
        </label>
      </template>
      <p v-if="connectNotice" role="status" class="text-sm text-amber-300">{{ connectNotice }}</p>
      <p v-if="authStore.kernelError" role="alert" class="text-sm text-red-300">{{ authStore.kernelError }}</p>
      <button type="submit" :disabled="connecting" class="rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
        {{ connecting ? 'Connecting...' : 'Connect' }}
      </button>
    </form>

    <template v-else>
    <div v-if="store.error" role="alert" class="flex items-start justify-between gap-4 rounded-lg border border-red-500/40 bg-red-500/10 p-4 text-sm text-red-200">
      <span>{{ store.error }}</span>
      <button class="text-red-100 underline" @click="store.clearError">Dismiss</button>
    </div>

    <div v-if="!store.loading && !store.plugins.length" class="rounded-xl border border-dark-700 bg-dark-800 p-8 text-center text-dark-300">
      No signed plugins are available for this account.
    </div>

    <div class="grid gap-4 xl:grid-cols-2">
      <article
        v-for="plugin in store.plugins"
        :key="plugin.id"
        class="rounded-xl border border-dark-700 bg-dark-800 p-5"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <CubeIcon class="h-5 w-5 text-primary-400" />
              <h2 class="truncate text-lg font-semibold text-white">{{ plugin.name || plugin.id }}</h2>
            </div>
            <p class="mt-1 text-xs text-dark-400">{{ plugin.id }} · {{ plugin.publisher || 'Unknown publisher' }}</p>
            <p class="mt-3 text-sm text-dark-300">{{ plugin.description || 'No description provided.' }}</p>
          </div>
          <span
            class="shrink-0 rounded-full px-2.5 py-1 text-xs font-medium"
            :class="plugin.official ? 'bg-emerald-500/15 text-emerald-300' : 'bg-amber-500/15 text-amber-300'"
          >
            {{ plugin.official ? 'Official' : 'Unverified' }}
          </span>
        </div>

        <div class="mt-5 grid gap-3 sm:grid-cols-2">
          <div v-for="target in ['control', 'agent']" :key="target" class="min-w-0 border-t border-dark-700 pt-3 sm:border-l sm:border-t-0 sm:pl-3 sm:pt-0 sm:first:border-l-0 sm:first:pl-0">
            <div class="flex items-center justify-between gap-2">
              <span class="text-xs uppercase tracking-wide text-dark-400">{{ target }}</span>
              <span class="text-xs" :class="stateClass(store.installation(plugin.id, target)?.state)">
                {{ store.installation(plugin.id, target)?.state || 'not installed' }}
              </span>
            </div>
            <template v-if="!targetReleases(plugin.id, target).length">
              <p class="mt-2 text-sm text-dark-500">Target not supported by a verified release</p>
            </template>
            <template v-else-if="store.installation(plugin.id, target)">
              <p class="mt-2 text-sm text-white">
                {{ store.installation(plugin.id, target).desired_version || 'No desired version' }}
              </p>
              <p class="mt-1 text-xs text-dark-400">
                observed {{ store.installation(plugin.id, target).observed_version || 'pending' }} · revision {{ store.installation(plugin.id, target).config_revision || 0 }}
              </p>
              <p class="mt-1 text-xs text-dark-400">
                health {{ installationHealth(store.installation(plugin.id, target)) }}
              </p>
              <p v-if="store.installation(plugin.id, target).has_error || store.installation(plugin.id, target).last_error" class="mt-2 text-xs text-red-300">
                Failure: {{ store.installation(plugin.id, target).last_error || 'lifecycle operation failed' }}
              </p>
              <div class="mt-3 flex flex-wrap gap-2">
                <button
                  v-if="store.installation(plugin.id, target).enabled"
                  class="rounded-lg border border-dark-600 px-2 py-1 text-xs text-dark-200 hover:border-red-400 hover:text-red-200 disabled:opacity-50"
                  :disabled="store.actionLoading"
                  @click="runTarget(plugin, target, 'disable')"
                >Disable</button>
                <button
                  v-else
                  class="rounded-lg bg-primary-600 px-2 py-1 text-xs text-white hover:bg-primary-700 disabled:opacity-50"
                  :disabled="store.actionLoading"
                  @click="runTarget(plugin, target, 'enable')"
                >Enable</button>
                <button
                  v-if="store.installation(plugin.id, target).previous_version"
                  class="rounded-lg border border-dark-600 px-2 py-1 text-xs text-dark-200 hover:border-amber-400 disabled:opacity-50"
                  :disabled="store.actionLoading"
                  @click="runTarget(plugin, target, 'rollback')"
                >Rollback</button>
                <button
                  v-if="updateVersion(plugin.id, target, store.installation(plugin.id, target).desired_version)"
                  class="rounded-lg border border-dark-600 px-2 py-1 text-xs text-dark-200 hover:border-primary-400 disabled:opacity-50"
                  :disabled="store.actionLoading"
                  @click="runTarget(plugin, target, 'update', updateVersion(plugin.id, target, store.installation(plugin.id, target).desired_version))"
                >Update to {{ updateVersion(plugin.id, target, store.installation(plugin.id, target).desired_version) }}</button>
                <button
                  class="rounded-lg border border-dark-600 px-2 py-1 text-xs text-dark-200 hover:border-primary-400 disabled:opacity-50"
                  :disabled="store.actionLoading"
                  @click="openConfig(store.installation(plugin.id, target))"
                >Configure</button>
              </div>
            </template>
            <template v-else>
              <p class="mt-2 text-sm text-dark-500">No installation</p>
              <button
                v-if="plugin.official === true"
                class="mt-3 rounded-lg bg-primary-600 px-2 py-1 text-xs text-white hover:bg-primary-700 disabled:opacity-50"
                :disabled="store.actionLoading || !firstVersion(plugin.id, target)"
                @click="installTarget(plugin, target)"
              >Install {{ target }} plugin</button>
              <p v-else class="mt-3 text-xs text-amber-300">Official release required before installation</p>
            </template>
          </div>
        </div>

      </article>
    </div>

    <div v-if="store.lastOperation" class="rounded-lg border border-primary-500/40 bg-primary-500/10 p-3 text-sm text-primary-100" role="status">
      Operation <code>{{ store.lastOperation.id || 'pending' }}</code> is {{ store.lastOperation.state || 'pending' }}.
      <span v-if="store.lastOperation.operation_chain"> Chain: <code>{{ store.lastOperation.operation_chain }}</code>.</span>
      Installation state will update on the next refresh.
    </div>

    <section class="rounded-xl border border-dark-700 bg-dark-800 p-5" aria-labelledby="operation-history-heading">
      <div class="flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <h2 id="operation-history-heading" class="text-lg font-semibold text-white">Recent operations</h2>
          <p class="mt-1 text-sm text-dark-400">Lifecycle operations reported by Anix Control.</p>
        </div>
        <span class="text-xs text-dark-500">{{ store.recentOperations.length }} shown</span>
      </div>
      <div v-if="!store.recentOperations.length" class="mt-4 text-sm text-dark-400">
        No recent operations.
      </div>
      <div v-else class="mt-4 overflow-x-auto">
        <table class="w-full min-w-[38rem] text-left text-sm" aria-label="Recent plugin operations">
          <caption class="sr-only">Recent plugin operations</caption>
          <thead class="border-b border-dark-700 text-xs uppercase tracking-wide text-dark-400">
            <tr>
              <th scope="col" class="px-3 py-2 font-medium">ID</th>
              <th scope="col" class="px-3 py-2 font-medium">Kind</th>
              <th scope="col" class="px-3 py-2 font-medium">State</th>
              <th scope="col" class="px-3 py-2 font-medium">Target</th>
              <th scope="col" class="px-3 py-2 font-medium">Version</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-dark-700">
            <tr v-for="operation in store.recentOperations" :key="operation.id || operation.revision" class="text-dark-200">
              <td class="max-w-56 break-all px-3 py-3 align-top font-mono text-xs text-dark-300">{{ operation.id || 'pending' }}</td>
              <td class="px-3 py-3 align-top text-dark-100">{{ operation.kind || '-' }}</td>
              <td class="px-3 py-3 align-top">
                <span
                  class="font-medium"
                  :class="stateClass(operation.state)"
                  aria-live="polite"
                  :aria-label="operationStatusLabel(operation)"
                >{{ operation.state || 'pending' }}</span>
              </td>
              <td class="px-3 py-3 align-top">{{ operationTarget(operation) }}</td>
              <td class="px-3 py-3 align-top">{{ operationVersion(operation) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div v-if="configTarget" class="fixed inset-0 z-20 flex items-center justify-center bg-black/70 p-4" @click.self="closeConfig">
      <section class="w-full max-w-2xl rounded-xl border border-dark-600 bg-dark-800 p-5" role="dialog" aria-modal="true">
        <div class="flex items-center justify-between gap-4"><h2 class="text-lg font-semibold text-white">Plugin configuration</h2><button class="text-dark-400 hover:text-white" @click="closeConfig">Close</button></div>
        <p class="mt-1 text-sm text-dark-400">{{ configTarget.plugin_id }} · {{ configTarget.target }} · revision {{ configRevision }}</p>
        <textarea v-model="configDraft" class="mt-4 min-h-48 w-full rounded-lg border border-dark-600 bg-dark-900 p-3 font-mono text-sm text-white focus:border-primary-500 focus:outline-none" spellcheck="false" aria-label="Plugin configuration JSON" />
        <p v-if="configError" class="mt-2 text-sm text-red-300">{{ configError }}</p>
        <div class="mt-4 flex justify-end gap-2"><button class="rounded-lg border border-dark-600 px-3 py-2 text-sm text-dark-200" @click="closeConfig">Cancel</button><button class="rounded-lg bg-primary-600 px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="configSaving" @click="saveConfig">{{ configSaving ? 'Saving...' : 'Save configuration' }}</button></div>
      </section>
    </div>
    </template>
  </section>
</template>

<script setup>
import { onMounted, ref, watch } from 'vue'
import { ArrowPathIcon, CubeIcon } from '@heroicons/vue/24/outline'
import { useAuthStore } from '@/stores/auth'
import { usePluginsStore } from '@/stores/plugins'

const authStore = useAuthStore()
const store = usePluginsStore()
const connectEmail = ref(authStore.user?.email || '')
const connectPassword = ref('')
const mfaCode = ref('')
const mfaMethod = ref('')
const mfaMethods = ref([])
const mfaRequired = ref(false)
const connectNotice = ref('')
const connecting = ref(false)
const configTarget = ref(null)
const configDraft = ref('{}')
const configRevision = ref(0)
const configError = ref('')
const configSaving = ref(false)

function releaseSupportsTarget(release, target) {
  if (!release?.manifest) return true
  try {
    return JSON.parse(release.manifest)?.targets?.includes(target) ?? false
  } catch {
    return false
  }
}

function targetReleases(pluginId, target) {
  return store.releasesFor(pluginId).filter((release) => releaseSupportsTarget(release, target))
}

function firstVersion(pluginId, target) {
  return targetReleases(pluginId, target)[0]?.version || ''
}

function updateVersion(pluginId, target, currentVersion) {
  const current = String(currentVersion || '').split(/[.-]/).slice(0, 3).map((part) => Number(part) || 0)
  return targetReleases(pluginId, target)
    .map((release) => ({ ...release, parts: String(release.version || '').split(/[.-]/).slice(0, 3).map((part) => Number(part) || 0) }))
    .filter((release) => {
      if (release.version === currentVersion) return false
      for (let index = 0; index < 3; index += 1) {
        if (release.parts[index] !== (current[index] || 0)) return release.parts[index] > (current[index] || 0)
      }
      return false
    })
    .sort((left, right) => {
      for (let index = 0; index < 3; index += 1) {
        if (left.parts[index] !== right.parts[index]) return left.parts[index] - right.parts[index]
      }
      return 0
    })
    .at(-1)?.version || ''
}

function stateClass(state) {
  if (state === 'succeeded' || state === 'enabled' || state === 'healthy') return 'text-emerald-300'
  if (state === 'failed' || state === 'degraded') return 'text-red-300'
  if (state === 'pending' || state === 'running') return 'text-amber-300'
  return 'text-dark-400'
}

function operationTarget(operation) {
  if (operation?.target) return operation.target
  return operation?.node_id === undefined || operation?.node_id === null ? 'control' : 'agent'
}

function operationVersion(operation) {
  return operation?.target_version || operation?.version || '-'
}

function operationStatusLabel(operation) {
  return `Operation state: ${operation?.state || 'pending'}`
}

function installationHealth(installation) {
  if (!installation) return 'not installed'
  if (installation.health) return installation.health
  if (installation.state === 'healthy' || installation.state === 'enabled' || installation.state === 'succeeded') return 'healthy'
  if (installation.state === 'failed') return 'unhealthy'
  if (installation.state === 'degraded') return 'degraded'
  if (installation.state === 'disabled') return 'disabled'
  return installation.state || 'pending'
}

async function installTarget(plugin, target) {
  const version = firstVersion(plugin.id, target)
  if (!version) return
  await store.saveInstallation({ plugin_id: plugin.id, target, desired_version: version, enabled: target === 'control' ? false : true })
  if (!authStore.isKernelAuthenticated) return
  const installation = store.installation(plugin.id, target)
  if (target === 'control' && installation) await store.runAction(installation, 'enable')
  else await store.refresh()
}

async function runTarget(plugin, target, action, targetVersion = '') {
  const installation = store.installation(plugin.id, target)
  if (!installation) return
  if (target === 'control') {
    await store.runAction(installation, action, targetVersion ? { targetVersion } : {})
    return
  }
  const desiredVersion = action === 'rollback' ? installation.previous_version : targetVersion || installation.desired_version
  await store.saveInstallation({ plugin_id: installation.plugin_id, target, desired_version: desiredVersion, enabled: action !== 'disable' })
  if (authStore.isKernelAuthenticated) await store.refresh()
}

async function openConfig(installation) {
  configTarget.value = installation
  configError.value = ''
  try {
    const result = await store.getConfig(installation.id)
    if (!authStore.isKernelAuthenticated || configTarget.value?.id !== installation.id) return
    configRevision.value = result?.revision ?? installation.config_revision ?? 0
    configDraft.value = typeof result?.config === 'string' ? result.config : JSON.stringify(result?.config || {}, null, 2)
  } catch (requestError) {
    if (!authStore.isKernelAuthenticated || configTarget.value?.id !== installation.id) return
    configError.value = requestError?.response?.data?.error?.message || requestError?.message || 'Unable to load configuration'
  }
}

function closeConfig() { configTarget.value = null; configError.value = '' }

async function connectControl() {
  connecting.value = true
  connectNotice.value = ''
  try {
    const result = await authStore.connectKernel({
      email: connectEmail.value,
      password: connectPassword.value,
      mfaCode: mfaCode.value,
      mfaMethod: mfaMethod.value
    })
    if (result.mfaRequired) {
      mfaRequired.value = true
      mfaMethods.value = result.methods
      mfaMethod.value = result.methods[0] || ''
      mfaCode.value = ''
    } else if (result.enrollmentRequired) {
      connectNotice.value = 'MFA enrollment is required in Anix Control before this account can connect.'
    } else if (result.connected) {
      connectPassword.value = ''
      mfaCode.value = ''
      mfaRequired.value = false
      await store.refresh()
    }
  } finally {
    connecting.value = false
  }
}

async function saveConfig() {
  let config
  try { config = JSON.parse(configDraft.value) } catch { configError.value = 'Configuration must be valid JSON.'; return }
  configSaving.value = true
  configError.value = ''
  try {
    await store.updateConfig(configTarget.value.id, config, configRevision.value)
    closeConfig()
  } catch (requestError) {
    configError.value = requestError?.response?.data?.error?.message || requestError?.message || 'Unable to save configuration'
  } finally { configSaving.value = false }
}

watch(() => authStore.isKernelAuthenticated, (connected) => {
  if (!connected) {
    store.clearState()
    closeConfig()
  }
})

onMounted(() => {
  if (authStore.isKernelAuthenticated) store.refresh()
})
</script>
