import { computed, ref } from 'vue'
import {
  getKernelInstallations,
  getKernelPluginReleases,
  getKernelPlugins,
} from '@/api/kernel'
import { releaseTargets } from '@/utils/kernelPluginRelease'

function asRows(value) {
  return Array.isArray(value) ? value : []
}

function errorMessage(cause) {
  return cause instanceof Error ? cause.message : String(cause || 'Unable to load plugins')
}

export function buildPluginRows(plugins, releases, installations) {
  const pluginRows = asRows(plugins)
  const releaseRows = asRows(releases)
  const installationRows = asRows(installations)

  return pluginRows.map(plugin => {
    const pluginReleases = releaseRows.filter(release => release?.plugin_id === plugin?.id)
    const targetIDs = new Set(pluginReleases.flatMap(releaseTargets))
    const pluginInstallations = installationRows.filter(item => item?.plugin_id === plugin?.id)
    for (const installation of pluginInstallations) {
      if (installation.target) targetIDs.add(installation.target)
    }
    const targets = [...targetIDs].sort().map(target => {
      const targetReleases = pluginReleases.filter(release => releaseTargets(release).includes(target))
      const installation = pluginInstallations.find(item => item.target === target) || null
      return {
        target,
        releases: targetReleases,
        installation,
        latestRelease: targetReleases[0] || null,
      }
    })
    const failed = targets.find(item => item.installation?.last_error)
    const unhealthy = targets.find(item => item.installation && item.installation.state !== 'healthy')

    return {
      key: plugin.id,
      plugin,
      releases: pluginReleases,
      installations: pluginInstallations,
      targets,
      latestRelease: pluginReleases[0] || null,
      health: failed
        ? { state: 'attention', error: failed.installation.last_error }
        : unhealthy
          ? { state: 'attention', error: '' }
          : { state: pluginInstallations.length ? 'healthy' : 'catalogued', error: '' },
    }
  })
}

export function useKernelPlugins() {
  const plugins = ref([])
  const releases = ref([])
  const installations = ref([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const rows = computed(() => buildPluginRows(plugins.value, releases.value, installations.value))

  async function load({ silent = false } = {}) {
    const initialLoad = !loaded.value
    if (!silent) {
      loading.value = true
      error.value = ''
    }

    try {
      const [pluginRows, releaseRows, installationRows] = await Promise.all([
        getKernelPlugins(),
        getKernelPluginReleases(),
        getKernelInstallations(),
      ])
      plugins.value = asRows(pluginRows)
      releases.value = asRows(releaseRows)
      installations.value = asRows(installationRows)
      loaded.value = true
      error.value = ''
    } catch (cause) {
      error.value = errorMessage(cause)
      if (!silent && initialLoad) {
        plugins.value = []
        releases.value = []
        installations.value = []
        loaded.value = false
      }
    } finally {
      if (!silent) loading.value = false
    }
  }

  return {
    plugins,
    releases,
    installations,
    rows,
    loading,
    loaded,
    error,
    load,
  }
}
