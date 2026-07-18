import { beforeEach, describe, expect, it, vi } from 'vitest'
import { buildPluginRows, useKernelPlugins } from '@/composables/useKernelPlugins'
import { parseManifest, releaseTargets } from '@/utils/kernelPluginRelease'

const kernelApi = vi.hoisted(() => ({
  getKernelPlugins: vi.fn(),
  getKernelPluginReleases: vi.fn(),
  getKernelInstallations: vi.fn(),
}))

vi.mock('@/api/kernel', () => kernelApi)

const plugin = { id: 'protocol-runtime', name: 'Protocol Runtime' }
const controlRelease = {
  id: 2,
  plugin_id: plugin.id,
  version: '1.1.0',
  manifest: JSON.stringify({ targets: ['control'] }),
}
const agentRelease = {
  id: 1,
  plugin_id: plugin.id,
  version: '1.0.0',
  manifest: JSON.stringify({ targets: ['agent'] }),
}
const controlInstallation = {
  id: 1,
  plugin_id: plugin.id,
  target: 'control',
  desired_version: '1.1.0',
  observed_version: '1.1.0',
  state: 'healthy',
  enabled: true,
}
const agentInstallation = {
  id: 2,
  plugin_id: plugin.id,
  target: 'agent',
  desired_version: '1.0.0',
  observed_version: '0.9.0',
  state: 'failed',
  enabled: true,
  last_error: 'agent failed',
}

function mockCatalog() {
  kernelApi.getKernelPlugins.mockResolvedValue([plugin])
  kernelApi.getKernelPluginReleases.mockResolvedValue([controlRelease, agentRelease])
  kernelApi.getKernelInstallations.mockResolvedValue([controlInstallation, agentInstallation])
}

describe('kernel plugin catalog', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mockCatalog()
  })

  it('parses release manifests defensively and keeps only supported targets', () => {
    expect(parseManifest({ manifest: '{invalid' })).toEqual({})
    expect(releaseTargets({ manifest: { targets: ['control', 'agent', 'node'] } })).toEqual(['control', 'agent'])
  })

  it('builds one plugin row with target summaries and the most actionable health error', () => {
    const rows = buildPluginRows([plugin], [controlRelease, agentRelease], [controlInstallation, agentInstallation])

    expect(rows).toHaveLength(1)
    expect(rows[0].targets.map(target => target.target)).toEqual(['agent', 'control'])
    expect(rows[0].health).toEqual({ state: 'attention', error: 'agent failed' })
    expect(rows[0].targets.find(target => target.target === 'control')).toMatchObject({
      latestRelease: controlRelease,
      installation: controlInstallation,
    })
  })

  it('loads only the plugin catalog resources in parallel', async () => {
    const catalog = useKernelPlugins()

    await catalog.load()

    expect(kernelApi.getKernelPlugins).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelPluginReleases).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(1)
    expect(catalog.loaded.value).toBe(true)
    expect(catalog.rows.value).toHaveLength(1)
    expect(catalog.loading.value).toBe(false)
  })

  it('preserves catalog rows when a silent refresh fails', async () => {
    const catalog = useKernelPlugins()
    await catalog.load()
    const initialRows = catalog.rows.value
    kernelApi.getKernelPlugins.mockRejectedValueOnce(new Error('refresh failed'))

    await catalog.load({ silent: true })

    expect(catalog.rows.value).toBe(initialRows)
    expect(catalog.loaded.value).toBe(true)
    expect(catalog.error.value).toBe('refresh failed')
    expect(catalog.loading.value).toBe(false)
  })

  it('reports a silent refresh failure to mutation callers without clearing catalog rows', async () => {
    const catalog = useKernelPlugins()
    await catalog.load()
    const initialRows = catalog.rows.value
    kernelApi.getKernelPlugins.mockRejectedValueOnce(new Error('refresh failed'))

    await expect(catalog.load({ silent: true })).resolves.toBe(false)

    expect(catalog.rows.value).toBe(initialRows)
    expect(catalog.loaded.value).toBe(true)
    expect(catalog.error.value).toBe('refresh failed')
  })

  it('reports and clears a failed initial load', async () => {
    kernelApi.getKernelPlugins.mockRejectedValueOnce(new Error('catalog unavailable'))
    const catalog = useKernelPlugins()

    await catalog.load()

    expect(catalog.rows.value).toEqual([])
    expect(catalog.loaded.value).toBe(false)
    expect(catalog.error.value).toBe('catalog unavailable')
    expect(catalog.loading.value).toBe(false)
  })
})
