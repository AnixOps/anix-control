import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { setLocale } from '@/i18n'

// 组网: the regions and the nodes in them with their reported status, and
// the notice in a build without the mesh.

const api = vi.hoisted(() => ({
  listKernelMeshRegions: vi.fn(),
  getKernelAdminNodes: vi.fn(),
  saveKernelMeshRegion: vi.fn(),
  deleteKernelMeshRegion: vi.fn(),
  addKernelMeshMember: vi.fn(),
  updateKernelMeshMember: vi.fn(),
  removeKernelMeshMember: vi.fn(),
  listKernelMeshEnrollments: vi.fn(),
  listKernelMeshRegionChanges: vi.fn(),
  createKernelMeshEnrollment: vi.fn(),
  deleteKernelMeshEnrollment: vi.fn(),
  moveKernelMeshDevice: vi.fn(),
  revokeKernelMeshDevice: vi.fn(),
  decideKernelMeshRegionChange: vi.fn()
}))
const router = vi.hoisted(() => ({ push: vi.fn() }))

vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...api }))
vi.mock('vue-router', async importOriginal => ({ ...(await importOriginal()), useRouter: () => router, useRoute: () => ({ query: {}, params: {}, path: '/' }) }))

const MeshRegions = (await import('@/views/admin/forward/MeshRegions.vue')).default

const fresh = () => new Date(Date.now() - 10_000).toISOString()

describe('MeshRegions', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    Object.values(api).forEach(fn => fn.mockReset())
    api.listKernelMeshEnrollments.mockResolvedValue([])
    api.listKernelMeshRegionChanges.mockResolvedValue([])
    api.getKernelAdminNodes.mockResolvedValue({ list: [{ id: 1, name: 'hk-01', host: 'hk.example.com' }, { id: 2, name: 'jp-01' }] })
  })

  it('lists the regions and the nodes with their status', async () => {
    api.listKernelMeshRegions.mockResolvedValue([
      { id: 1, code: 'cn', name: '中国大陆', cidr: '10.150.0.0/16', enabled: true, members: [
        { id: 11, node_ref: 'proxy-1', node_name: 'hk-01', address: '10.150.0.1', home: true, boundary: true,
          status: { networks: [{ id: 'cn', running: false, alerts: [{ code: 'address_unmanaged' }] }] }, status_received_at: fresh() }
      ] },
      { id: 2, code: 'eu', name: '欧洲', cidr: '10.152.0.0/16', enabled: false, members: [
        { id: 21, node_ref: 'proxy-1', address: '10.152.0.1', home: false }
      ] }
    ])
    const wrapper = mount(MeshRegions, { attachTo: document.body })
    await flushPromises()
    const regions = wrapper.get('[data-testid="mesh-region-table"]').text()
    expect(regions).toContain('10.150.0.0/16')
    expect(regions).toContain('边界节点少于两台')
    expect(regions).toContain('未启用')
    const members = wrapper.get('[data-testid="mesh-member-table"]').text()
    expect(members).toContain('中国大陆 · 10.150.0.1')
    expect(members).toContain('欧洲 · 10.152.0.1')
    expect(members).toContain('未运行')
    expect(members).toContain('地址未受管')
    wrapper.unmount()
  })

  it('lists the devices, the pending region changes and the credentials, and shows an issued token once', async () => {
    api.listKernelMeshRegions.mockResolvedValue([
      { id: 1, code: 'cn', name: '中国大陆', cidr: '10.150.0.0/16', enabled: true, members: [], devices: [
        { id: 5, kind: 'personal', name: 'laptop', address: '10.150.128.0', status: { networks: [{ id: 'cn', running: true }] }, status_received_at: fresh() }
      ] },
      { id: 2, code: 'eu', name: '欧洲', cidr: '10.152.0.0/16', enabled: true, members: [], devices: [] }
    ])
    api.listKernelMeshRegionChanges.mockResolvedValue([
      { id: 8, device_id: 5, from_region_id: 1, to_region_id: 2, reason: '办公地点变更', state: 'pending' },
      { id: 7, device_id: 5, from_region_id: 2, to_region_id: 1, state: 'approved' }
    ])
    api.listKernelMeshEnrollments.mockResolvedValue([
      { id: 3, region_id: 0, kind: 'personal', name: 'laptop', expires_at: fresh(), used_at: fresh(), device_id: 5 }
    ])
    api.createKernelMeshEnrollment.mockResolvedValue({ id: 4, kind: 'node', name: 'office', token: 'mse_ONETIME' })
    const wrapper = mount(MeshRegions, { attachTo: document.body })
    await flushPromises()
    const devices = wrapper.get('[data-testid="mesh-device-table"]').text()
    expect(devices).toContain('laptop')
    expect(devices).toContain('个人设备')
    expect(devices).toContain('中国大陆 · 10.150.128.0')
    expect(devices).toContain('运行中')
    const changes = wrapper.get('[data-testid="mesh-change-table"]').text()
    expect(changes).toContain('中国大陆 → 欧洲')
    expect(changes).toContain('办公地点变更')
    expect(changes).not.toContain('欧洲 → 中国大陆')
    const enrollments = wrapper.get('[data-testid="mesh-enrollment-table"]').text()
    expect(enrollments).toContain('按延迟自动选择')
    expect(enrollments).toContain('已使用')

    await wrapper.get('[data-testid="mesh-enrollment-add"]').trigger('click')
    await flushPromises()
    const form = document.querySelector('[data-testid="mesh-enrollment-form"]')
    const name = form.querySelector('input')
    name.value = 'office'
    name.dispatchEvent(new Event('input'))
    document.querySelector('[data-testid="mesh-enrollment-save"]').click()
    await flushPromises()
    expect(api.createKernelMeshEnrollment).toHaveBeenCalledWith({ name: 'office', kind: 'personal', regionId: 0, hours: 24 })
    expect(document.querySelector('[data-testid="mesh-enrollment-issued"] input').value).toBe('mse_ONETIME')
    wrapper.unmount()
  })

  it('says when the build has no mesh', async () => {
    api.listKernelMeshRegions.mockRejectedValue({ response: { status: 404, data: { error: { code: 'feature_disabled' } } } })
    const wrapper = mount(MeshRegions, { attachTo: document.body })
    await flushPromises()
    expect(wrapper.text()).toContain('当前版本未提供组网功能。')
    expect(wrapper.find('[data-testid="mesh-region-add"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
