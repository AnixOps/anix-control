// 网络 → 组网: the regions load with the page, the settings save, and the
// intranet names show their duplicates.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import Mesh from '@/views/admin/Mesh.vue'
import MeshNames from '@/views/admin/mesh/MeshNames.vue'
import MeshSettings from '@/views/admin/mesh/MeshSettings.vue'
import { setLocale } from '@/i18n'

const meshApi = vi.hoisted(() => ({
  getMesh: vi.fn(),
  updateMeshSettings: vi.fn(),
  getMeshNames: vi.fn(),
  createMeshRegion: vi.fn(),
  updateMeshRegion: vi.fn(),
  deleteMeshRegion: vi.fn(),
  meshErrorMessage: (error, fallback) => error?.response?.data?.error?.message || fallback
}))

vi.mock('@/api/mesh', () => meshApi)

const SETTINGS = { name_suffix: 'mesh.internal', host_group_supernet: '10.127.0.0/16', host_group_prefix: 24, reserved_subnets: '10.126.0.0/16' }
const REGION = {
  id: 3, code: 'eu', name: '欧洲', sort_order: 3, subnet: '10.123.0.0/16', server_range: '10.123.0.0/20', device_pool: '10.123.64.0/18',
  network_name: 'mesh-eu-0123456789ab', security: 'forward_secrecy', border_nodes: 1, members: 3, pool_used: 2, pool_size: 16384,
  warnings: ['few_border_nodes']
}

describe('Mesh', () => {
  beforeEach(async () => {
    await setLocale('en')
    Object.values(meshApi).forEach(fn => fn.mockReset?.())
  })

  it('shows the regions with their warnings and the pending changes', async () => {
    meshApi.getMesh.mockResolvedValue({ settings: SETTINGS, regions: [REGION], host_groups: [], pending_changes: 2 })
    render(Mesh)
    const regions = await screen.findByTestId('mesh-regions')
    expect(regions.textContent).toContain('10.123.0.0/16')
    expect(regions.textContent).toContain('Forward secrecy')
    expect(regions.textContent).toContain('Fewer than two border nodes')
    expect(screen.getByTestId('mesh-tabs').textContent).toContain('Region changes (2)')
  })

  it('saves the settings', async () => {
    meshApi.updateMeshSettings.mockResolvedValue(SETTINGS)
    const { emitted } = render(MeshSettings, { props: { settings: SETTINGS } })
    const reserved = screen.getByLabelText('Reserved ranges')
    expect(reserved.value).toBe('10.126.0.0/16')
    await userEvent.type(reserved, '\n192.168.50.0/24')
    await userEvent.click(screen.getByTestId('mesh-settings-save'))
    expect(meshApi.updateMeshSettings).toHaveBeenCalledWith({
      name_suffix: 'mesh.internal', host_group_supernet: '10.127.0.0/16', host_group_prefix: 24,
      reserved_subnets: ['10.126.0.0/16', '192.168.50.0/24']
    })
    expect(emitted().changed).toBeTruthy()
  })

  it('shows the refusal of the settings', async () => {
    meshApi.updateMeshSettings.mockRejectedValue({ response: { data: { error: { message: 'name_suffix: DNS labels separated by dots' } } } })
    render(MeshSettings, { props: { settings: SETTINGS } })
    await userEvent.click(screen.getByTestId('mesh-settings-save'))
    expect((await screen.findByRole('alert')).textContent).toBe('name_suffix: DNS labels separated by dots')
  })

  it('lists the intranet names and marks duplicates', async () => {
    meshApi.getMeshNames.mockResolvedValue({
      suffix: 'mesh.internal',
      entries: [
        { fqdn: 'db.eu.mesh.internal', name: 'db', region_code: 'eu', address: '10.123.0.3', source: 'member', member_id: 1, duplicate: true },
        { fqdn: 'db.eu.mesh.internal', name: 'db', region_code: 'eu', address: '10.123.0.4', source: 'member', member_id: 2, duplicate: true },
        { fqdn: 'vm-1.eu.mesh.internal', name: 'vm-1', region_code: 'eu', address: '10.127.0.10', source: 'host_group', member_id: 5, duplicate: false }
      ]
    })
    render(MeshNames)
    const section = await screen.findByTestId('mesh-names')
    expect(await screen.findAllByText('vm-1.eu.mesh.internal')).not.toHaveLength(0)
    expect(section.textContent).toContain('10.127.0.10')
    expect(section.textContent).toContain('Duplicate')
    expect(section.textContent).toContain('name.region-code.mesh.internal')
  })
})
