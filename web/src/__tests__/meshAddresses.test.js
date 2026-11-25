import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { setLocale } from '@/i18n'

// 组网 sections: the devices with their pool addresses, the intranet names
// with their duplicates, the region changes and the settings.

const api = vi.hoisted(() => ({
  listKernelMeshDevices: vi.fn(),
  createKernelMeshDevice: vi.fn(),
  getKernelMeshNames: vi.fn(),
  listKernelMeshChanges: vi.fn(),
  decideKernelMeshChange: vi.fn(),
  saveKernelMeshSettings: vi.fn()
}))

vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...api }))

const MeshDevices = (await import('@/components/forward/mesh/MeshDevices.vue')).default
const MeshNames = (await import('@/components/forward/mesh/MeshNames.vue')).default
const MeshChanges = (await import('@/components/forward/mesh/MeshChanges.vue')).default
const MeshSettings = (await import('@/components/forward/mesh/MeshSettings.vue')).default

const REGIONS = [{ id: 1, code: 'cn', name: '中国大陆', cidr: '10.150.0.0/16' }, { id: 2, code: 'eu', name: '欧洲', cidr: '10.152.0.0/16' }]

describe('mesh addresses', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    Object.values(api).forEach(fn => fn.mockReset())
  })

  it('lists the devices with their pool addresses and warnings', async () => {
    api.listKernelMeshDevices.mockResolvedValue([
      { id: 1, region_id: 1, hostname: 'phone', fqdn: 'phone.cn.mesh.internal', address: '10.150.128.0', address_used_at: '2026-11-25T08:00:00Z', warnings: [] },
      { id: 2, region_id: 1, hostname: 'old', fqdn: 'old.cn.mesh.internal', address: null, warnings: ['no_address'] }
    ])
    const wrapper = mount(MeshDevices, { props: { regions: REGIONS, pools: [{ region_id: 1, size: 32767, used: 1, low: false }] }, attachTo: document.body })
    await flushPromises()
    const text = wrapper.get('[data-testid="mesh-devices"]').text()
    expect(text).toContain('中国大陆 · 10.150.128.0')
    expect(text).toContain('phone.cn.mesh.internal')
    expect(text).toContain('地址已回收')
    expect(text).toContain('已用 1 / 32767')
    wrapper.unmount()
  })

  it('marks duplicate names and lists the unnamed nodes', async () => {
    api.getKernelMeshNames.mockResolvedValue({
      suffix: 'mesh.internal',
      entries: [
        { fqdn: 'db.cn.mesh.internal', address: '10.150.0.3', source: 'node', ref: 'proxy-1', duplicate: true },
        { fqdn: 'db.cn.mesh.internal', address: '10.127.0.9', source: 'host_group', ref: '4', duplicate: true },
        { fqdn: 'vm.cn.mesh.internal', address: '192.168.1.2', source: 'local', ref: '5', duplicate: false }
      ],
      unnamed: [{ node_ref: 'proxy-9', node_name: '香港 01' }]
    })
    const wrapper = mount(MeshNames, { attachTo: document.body })
    await flushPromises()
    const text = wrapper.get('[data-testid="mesh-names"]').text()
    expect(text).toContain('重名')
    expect(text).toContain('局域网设备')
    expect(text).toContain('名称.地区代码.mesh.internal')
    expect(wrapper.get('[data-testid="mesh-names-unnamed"]').text()).toContain('香港 01')
    wrapper.unmount()
  })

  it('lists the pending region changes', async () => {
    api.listKernelMeshChanges.mockResolvedValue([
      { id: 3, node_ref: 'proxy-1', from_region_id: 1, to_region_id: 2, source: 'member', status: 'pending', reason: 'relocated', old_address: '10.150.0.1', new_address: '', created_at: '2026-11-25T08:00:00Z' }
    ])
    const wrapper = mount(MeshChanges, { props: { regions: REGIONS, members: [{ node_ref: 'proxy-1', node_name: 'hk-01' }] }, attachTo: document.body })
    await flushPromises()
    expect(api.listKernelMeshChanges).toHaveBeenCalledWith('pending')
    const text = wrapper.get('[data-testid="mesh-changes"]').text()
    expect(text).toContain('hk-01')
    expect(text).toContain('中国大陆 → 欧洲')
    expect(text).toContain('成员申请')
    wrapper.unmount()
  })

  it('saves the settings with the reserved ranges', async () => {
    api.saveKernelMeshSettings.mockResolvedValue({})
    const wrapper = mount(MeshSettings, {
      props: { settings: { name_suffix: 'mesh.internal', host_group_supernet: '10.127.0.0/16', host_group_prefix: 24, reserved_ranges: ['10.126.0.0/16'] } },
      attachTo: document.body
    })
    await flushPromises()
    await wrapper.get('textarea').setValue('10.126.0.0/16\n192.168.50.0/24')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.saveKernelMeshSettings).toHaveBeenCalledTimes(1)
    const [input] = api.saveKernelMeshSettings.mock.calls[0]
    expect(input.nameSuffix).toBe('mesh.internal')
    expect(input.reservedRanges.filter(Boolean)).toEqual(['10.126.0.0/16', '192.168.50.0/24'])
    expect(wrapper.emitted().changed).toBeTruthy()
    wrapper.unmount()
  })
})
