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
  removeKernelMeshMember: vi.fn()
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

  it('says when the build has no mesh', async () => {
    api.listKernelMeshRegions.mockRejectedValue({ response: { status: 404, data: { error: { code: 'feature_disabled' } } } })
    const wrapper = mount(MeshRegions, { attachTo: document.body })
    await flushPromises()
    expect(wrapper.text()).toContain('当前版本未提供组网功能。')
    expect(wrapper.find('[data-testid="mesh-region-add"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
