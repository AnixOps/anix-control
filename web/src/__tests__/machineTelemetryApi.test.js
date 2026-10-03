// The services table's API helpers: the capability check, the read-modify-
// write of the per-node settings and the pattern check.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getNodeServices, nodeHasServicesTable, saveNodeServicesSettings, validGlob, withNodeServices
} from '@/api/machineTelemetry'

const kernel = vi.hoisted(() => ({
  getKernelInstallationConfig: vi.fn(),
  getKernelInstallations: vi.fn(),
  getKernelNodeAssignments: vi.fn(),
  getKernelPluginReleases: vi.fn(),
  updateKernelInstallationConfig: vi.fn()
}))
const request = vi.hoisted(() => vi.fn())

vi.mock('@/api/kernel', () => kernel)
vi.mock('@/utils/request', () => ({ default: request }))

const release = (version, capabilities) => ({
  plugin_id: 'machine-telemetry', version, manifest: JSON.stringify({ id: 'machine-telemetry', version, capabilities })
})

describe('machine-telemetry services API', () => {
  beforeEach(() => {
    for (const fn of Object.values(kernel)) fn.mockReset()
    request.mockReset()
  })

  it('reads the table from the package route', async () => {
    request.mockResolvedValue({ data: { node_id: 5, units: [] } })
    expect(await getNodeServices(5)).toEqual({ node_id: 5, units: [] })
    expect(request).toHaveBeenCalledWith({ baseURL: '/api/v3', url: '/plugins/machine-telemetry/nodes/5/services', method: 'get' })
  })

  it('finds the services table only in an enabled assignment of a release that declares it', async () => {
    kernel.getKernelPluginReleases.mockResolvedValue([release('4.0.0', ['telemetry.read']), release('4.1.0', ['telemetry.read', 'telemetry.systemd.read'])])
    kernel.getKernelNodeAssignments.mockResolvedValue([{ plugin_id: 'machine-telemetry', desired_version: '4.1.0', enabled: true }])
    expect(await nodeHasServicesTable(5)).toBe(true)
    expect(kernel.getKernelNodeAssignments).toHaveBeenCalledWith(5)
    expect(kernel.getKernelPluginReleases).toHaveBeenCalledWith('machine-telemetry')

    kernel.getKernelNodeAssignments.mockResolvedValue([{ plugin_id: 'machine-telemetry', desired_version: '4.0.0', enabled: true }])
    expect(await nodeHasServicesTable(5)).toBe(false)
    kernel.getKernelNodeAssignments.mockResolvedValue([{ plugin_id: 'machine-telemetry', desired_version: '4.1.0', enabled: false }])
    expect(await nodeHasServicesTable(5)).toBe(false)
    kernel.getKernelNodeAssignments.mockResolvedValue([{ plugin_id: 'gost-mesh', desired_version: '4.1.0', enabled: true }])
    kernel.getKernelPluginReleases.mockClear()
    expect(await nodeHasServicesTable(5)).toBe(false)
    expect(kernel.getKernelPluginReleases).not.toHaveBeenCalled()
  })

  it('replaces only the node’s entry and saves at the revision it read', async () => {
    kernel.getKernelInstallations.mockResolvedValue([
      { id: 3, plugin_id: 'machine-telemetry', target: 'control' },
      { id: 4, plugin_id: 'machine-telemetry', target: 'agent' }
    ])
    kernel.getKernelInstallationConfig.mockResolvedValue({
      revision: 7,
      config: { interval_seconds: 30, systemd_services: { nodes: { 9: { enabled: true } } } }
    })
    kernel.updateKernelInstallationConfig.mockResolvedValue({ revision: 8 })
    await saveNodeServicesSettings(5, { enabled: true, include: [' nginx*.service ', '', 'nginx*.service'], exclude: [] })
    expect(kernel.getKernelInstallationConfig).toHaveBeenCalledWith(4)
    expect(kernel.updateKernelInstallationConfig).toHaveBeenCalledWith(4, {
      interval_seconds: 30,
      systemd_services: { nodes: { 9: { enabled: true }, 5: { enabled: true, include: ['nginx*.service'] } } }
    }, 7)
  })

  it('refuses to save without an Agent installation', async () => {
    kernel.getKernelInstallations.mockResolvedValue([{ id: 3, plugin_id: 'machine-telemetry', target: 'control' }])
    await expect(saveNodeServicesSettings(5, { enabled: true })).rejects.toMatchObject({ code: 'no_installation' })
    expect(kernel.updateKernelInstallationConfig).not.toHaveBeenCalled()
  })

  it('builds the document from nothing', () => {
    expect(withNodeServices(null, 12, { enabled: false })).toEqual({ systemd_services: { nodes: { 12: { enabled: false } } } })
  })

  it('checks patterns', () => {
    for (const glob of ['*', 'nginx.service', 'nginx*.service', 'ssh?.service', '[a-c]*.service', '[^x]*', 'a@b\\x2d1.service']) {
      expect(validGlob(glob), glob).toBe(true)
    }
    for (const glob of ['', '[', 'a[', 'x\\', 'a b', 'a/b', 'ü.service', '[]', 'x'.repeat(257)]) {
      expect(validGlob(glob), glob).toBe(false)
    }
  })
})
