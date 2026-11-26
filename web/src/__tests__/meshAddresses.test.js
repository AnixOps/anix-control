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
  saveKernelMeshSettings: vi.fn(),
  listKernelMeshEnrollments: vi.fn(),
  createKernelMeshEnrollment: vi.fn(),
  revokeKernelMeshDevice: vi.fn(),
  listKernelMeshBridges: vi.fn(),
  listKernelMeshHostGroups: vi.fn(),
  saveKernelMeshBridge: vi.fn(),
  listKernelMeshPaths: vi.fn(),
  saveKernelMeshPath: vi.fn(),
  deleteKernelMeshPath: vi.fn()
}))

vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...api }))

const MeshDevices = (await import('@/components/forward/mesh/MeshDevices.vue')).default
const MeshEnrollments = (await import('@/components/forward/mesh/MeshEnrollments.vue')).default
const MeshNames = (await import('@/components/forward/mesh/MeshNames.vue')).default
const MeshChanges = (await import('@/components/forward/mesh/MeshChanges.vue')).default
const MeshSettings = (await import('@/components/forward/mesh/MeshSettings.vue')).default
const MeshBridges = (await import('@/components/forward/mesh/MeshBridges.vue')).default
const MeshPaths = (await import('@/components/forward/mesh/MeshPaths.vue')).default
const { nameSuffixProblem } = await import('@/components/forward/meshModel')

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

  it('shows an enrolled device\'s reported status', async () => {
    api.listKernelMeshDevices.mockResolvedValue([
      { id: 3, region_id: 1, hostname: 'laptop', fqdn: 'laptop.cn.mesh.internal', address: '10.150.128.1', warnings: [], enrolled: true,
        status: { networks: [{ id: 'cn', running: true }] }, status_received_at: new Date().toISOString() }
    ])
    const wrapper = mount(MeshDevices, { props: { regions: REGIONS, pools: [] }, attachTo: document.body })
    await flushPromises()
    expect(wrapper.get('[data-testid="mesh-devices"]').text()).toContain('运行中')
    wrapper.unmount()
  })

  it('marks a full node and the subnets it advertises', async () => {
    api.listKernelMeshDevices.mockResolvedValue([
      { id: 4, region_id: 1, hostname: 'router', fqdn: 'router.cn.mesh.internal', address: '10.150.0.9', warnings: [], role: 'full', subnets: ['192.168.1.0/24'],
        subnet_request: { subnets: ['192.168.2.0/24', '172.16.0.0/16'], requested_at: '2026-11-25T08:00:00Z' } }
    ])
    api.listKernelMeshEnrollments.mockResolvedValue([
      { id: 5, region_id: 1, hostname: 'router', role: 'full', address: '10.150.0.9', expires_at: '2026-11-26T08:00:00Z', used_at: null, device_id: null }
    ])
    const devices = mount(MeshDevices, { props: { regions: REGIONS, pools: [] }, attachTo: document.body })
    const enrollments = mount(MeshEnrollments, { props: { regions: REGIONS }, attachTo: document.body })
    await flushPromises()
    expect(devices.get('[data-testid="mesh-devices"]').text()).toContain('完整节点 · 通告 192.168.1.0/24')
    expect(devices.get('[data-testid="mesh-device-request"]').text()).toBe('申请通告 192.168.2.0/24, 172.16.0.0/16')
    expect(devices.get('[data-testid="mesh-devices"]').text()).toContain('网段申请待审批')
    expect(enrollments.get('[data-testid="mesh-enrollment-table"]').text()).toContain('完整节点 · 10.150.0.9')
    devices.unmount()
    enrollments.unmount()
  })

  it('lists the enrollment credentials and shows an issued token once', async () => {
    api.listKernelMeshEnrollments.mockResolvedValue([
      { id: 3, region_id: 0, hostname: 'laptop', expires_at: '2026-11-26T08:00:00Z', used_at: '2026-11-25T08:00:00Z', device_id: 5 }
    ])
    api.createKernelMeshEnrollment.mockResolvedValue({ id: 4, hostname: 'office', token: 'mse_ONETIME' })
    const wrapper = mount(MeshEnrollments, { props: { regions: REGIONS }, attachTo: document.body })
    await flushPromises()
    const text = wrapper.get('[data-testid="mesh-enrollment-table"]').text()
    expect(text).toContain('按延迟自动选择')
    expect(text).toContain('已使用')
    await wrapper.get('[data-testid="mesh-enrollment-add"]').trigger('click')
    await flushPromises()
    const input = document.querySelector('[data-testid="mesh-enrollment-hostname"] input') || document.querySelector('[data-testid="mesh-enrollment-form"] input')
    input.value = 'office'
    input.dispatchEvent(new Event('input'))
    document.querySelector('[data-testid="mesh-enrollment-save"]').click()
    await flushPromises()
    expect(api.createKernelMeshEnrollment).toHaveBeenCalledWith(expect.objectContaining({ hostname: 'office', regionId: '0', hours: 24 }))
    expect(document.querySelector('[data-testid="mesh-enrollment-issued"] input').value).toBe('mse_ONETIME')
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
    expect(wrapper.find('[data-testid="mesh-names-suffix-invalid"]').exists()).toBe(false)
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

  it('refuses a domain suffix outside internal before saving and flags a saved one', async () => {
    const wrapper = mount(MeshSettings, {
      props: { settings: { name_suffix: 'mesh.home.arpa', host_group_supernet: '10.127.0.0/16', host_group_prefix: 24, reserved_ranges: [], warnings: ['name_suffix_invalid'] } },
      attachTo: document.body
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="mesh-settings-suffix-invalid"]').exists()).toBe(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.saveKernelMeshSettings).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('后缀须位于 internal 之下且至少两级')
    wrapper.unmount()
  })

  it('marks the names as not delivered while the saved suffix breaks the rule', async () => {
    api.getKernelMeshNames.mockResolvedValue({
      suffix: 'mesh.home.arpa',
      suffix_invalid: true,
      entries: [{ fqdn: 'db.cn.mesh.home.arpa', address: '10.150.0.3', source: 'node', ref: 'proxy-1', duplicate: false }],
      unnamed: []
    })
    const wrapper = mount(MeshNames, { attachTo: document.body })
    await flushPromises()
    expect(wrapper.get('[data-testid="mesh-names-suffix-invalid"]').text()).toContain('暂未下发到节点')
    wrapper.unmount()
  })

  it('checks the domain suffix as the kernel does', () => {
    for (const good of ['mesh.internal', 'Mesh.Internal.', 'MESH.INTERNAL', ' lab.mesh.internal ', 'xn--fiqs8s.internal', `${'a'.repeat(63)}.internal`]) {
      expect(nameSuffixProblem(good), good).toBe('')
    }
    for (const bad of ['internal', 'local', 'com', 'home.arpa', 'mesh.home.arpa', 'mesh.local', 'corp.example', 'internal.mesh', 'xinternal', 'mesh.xinternal']) {
      expect(nameSuffixProblem(bad), bad).toBe('internal')
    }
    // Only ASCII letters are lowercased, as on the server: U+0130 and the
    // Kelvin sign stay as they are and are refused.
    for (const bad of ['', '.mesh.internal', 'mesh..internal', '-mesh.internal', 'mesh_lab.internal', `${'a'.repeat(64)}.internal`, 'MESH.\u0130NTERNAL', '\u212Aube.internal', '中国.internal']) {
      expect(nameSuffixProblem(bad), bad).toBe('labels')
    }
    expect(nameSuffixProblem(`${`${'a'.repeat(63)}.`.repeat(2)}${'a'.repeat(36)}.internal`)).toBe('length')
  })

  it('lists the external network bridges and saves one with its allowed sources', async () => {
    const members = [
      { node_ref: 'proxy-1', node_name: 'edge', address: '10.150.0.1', region: REGIONS[0] },
      { node_ref: 'proxy-2', node_name: 'home', address: '10.150.0.2', region: REGIONS[0] },
      { node_ref: 'proxy-3', node_name: 'paris', address: '10.152.0.1', region: REGIONS[1] }
    ]
    api.listKernelMeshBridges.mockResolvedValue([
      { id: 1, node_ref: 'proxy-1', node_name: 'edge', region_id: 1, interface: 'ext0', routes: ['100.100.1.2/32'],
        allow: [{ entry: 'device:4', kind: 'device', name: 'phone', prefixes: [] }], expanded: [], applied_digest: 'x', confirmed: false,
        held: ['10.150.128.4/32'], warnings: ['bridge_unconfirmed'] }
    ])
    api.listKernelMeshDevices.mockResolvedValue([{ id: 4, region_id: 1, hostname: 'phone', address: '10.150.128.4' }, { id: 5, region_id: 2, hostname: 'laptop', address: '10.152.128.0' }])
    api.listKernelMeshHostGroups.mockResolvedValue([{ id: 6, name: 'vms', segment: '10.127.0.0/24', region_id: 1 }])
    api.saveKernelMeshBridge.mockResolvedValue({ id: 2 })
    const wrapper = mount(MeshBridges, { props: { regions: REGIONS, members }, attachTo: document.body })
    await flushPromises()
    const text = wrapper.get('[data-testid="mesh-bridge-table"]').text()
    expect(text).toContain('100.100.1.2/32')
    expect(text).toContain('外部网络')
    expect(text).toContain('未确认')
    expect(text).toContain('待确认地址：10.150.128.4/32')

    await wrapper.get('[data-testid="mesh-bridge-add"]').trigger('click')
    await flushPromises()
    const allow = document.querySelector('[data-testid="mesh-bridge-allow"]')
    expect(allow.textContent).toContain('edge')
    expect(allow.textContent).toContain('phone')
    expect(allow.textContent).toContain('vms')
    expect(allow.textContent).not.toContain('laptop')
    expect(allow.textContent).not.toContain('paris')
    const form = document.querySelector('[data-testid="mesh-bridge-form"]')
    const iface = form.querySelector('input[placeholder="ext0"]')
    iface.value = 'eth1'
    iface.dispatchEvent(new Event('input'))
    const routes = form.querySelector('textarea')
    routes.value = '192.168.50.0/24'
    routes.dispatchEvent(new Event('input'))
    allow.querySelectorAll('[role="checkbox"]')[1].click()
    await flushPromises()
    document.querySelector('[data-testid="mesh-bridge-save"]').click()
    await flushPromises()
    expect(api.saveKernelMeshBridge).toHaveBeenCalledWith(null, { node: 'proxy-2', iface: 'eth1', routes: ['192.168.50.0/24'], allow: ['device:4'] })
    expect(wrapper.emitted().changed).toBeTruthy()
    wrapper.unmount()
  })

  it('lists the custom paths with their state and warnings', async () => {
    const members = [
      { node_ref: 'proxy-1', node_name: 'edge', address: '10.150.0.1', region: REGIONS[0] },
      { node_ref: 'proxy-2', node_name: 'home', address: '10.150.0.2', region: REGIONS[0] },
      { node_ref: 'proxy-3', node_name: 'hop', address: '10.150.0.3', region: REGIONS[0] }
    ]
    api.listKernelMeshDevices.mockResolvedValue([])
    api.listKernelMeshPaths.mockResolvedValue([
      { id: 1, source: 'node:proxy-1', source_name: 'edge', region_id: 1, target_kind: 'peer', target: 'node:proxy-2', target_name: 'home',
        via: [{ entry: 'node:proxy-3', name: 'hop', address: '10.150.0.3' }], warnings: [],
        status: { state: 'first_hop_only', reason: 'unreachable', unreachable: '10.150.0.3' } },
      { id: 2, source: 'node:proxy-1', source_name: 'edge', region_id: 1, target_kind: 'subnet', target: '192.168.50.0/24', target_name: '192.168.50.0/24',
        via: [{ entry: 'node:proxy-9', name: '', address: '' }, { entry: 'node:proxy-3', name: 'hop', address: '10.150.0.3' }], warnings: ['path_member_missing'], status: null },
      { id: 3, source: 'device:4', source_name: 'router', region_id: 1, target_kind: 'peer', target: 'node:proxy-2', target_name: 'home',
        via: [{ entry: 'node:proxy-3', name: 'hop', address: '10.150.0.3' }], warnings: ['path_unsupported'],
        status: { state: 'fallback', reason: 'other_reason', unreachable: null } }
    ])
    const wrapper = mount(MeshPaths, { props: { regions: REGIONS, members }, attachTo: document.body })
    await flushPromises()
    const text = wrapper.get('[data-testid="mesh-path-table"]').text()
    expect(text).toContain('仅保证首段')
    expect(text).toContain('途经节点无法连接')
    expect(text).toContain('无法连接：hop')
    expect(text).toContain('192.168.50.0/24')
    expect(text).toContain('node:proxy-9 → hop')
    expect(text).toContain('尚未上报')
    expect(text).toContain('成员缺失，未下发')
    expect(text).toContain('已回落')
    expect(text).toContain('other_reason')
    expect(text).toContain('内核不支持')
    wrapper.unmount()
  })

  it('saves a custom path through a full node', async () => {
    const members = [
      { node_ref: 'proxy-1', node_name: 'edge', address: '10.150.0.1', region: REGIONS[0] },
      { node_ref: 'proxy-2', node_name: 'home', address: '10.150.0.2', region: REGIONS[0] },
      { node_ref: 'proxy-3', node_name: 'paris', address: '10.152.0.1', region: REGIONS[1] }
    ]
    api.listKernelMeshDevices.mockResolvedValue([{ id: 4, region_id: 1, hostname: 'router', role: 'full', address: '10.150.0.9' }, { id: 5, region_id: 1, hostname: 'phone', role: 'access', address: '10.150.128.0' }])
    api.listKernelMeshPaths.mockResolvedValue([])
    api.saveKernelMeshPath.mockResolvedValue({ id: 1 })
    const wrapper = mount(MeshPaths, { props: { regions: REGIONS, members }, attachTo: document.body })
    await flushPromises()
    await wrapper.get('[data-testid="mesh-path-add"]').trigger('click')
    await flushPromises()
    const form = document.querySelector('[data-testid="mesh-path-form"]')
    const via = form.querySelector('textarea')
    expect(form.textContent).toContain('router = device:4')
    expect(form.textContent).not.toContain('paris')
    expect(form.textContent).not.toContain('phone')
    via.value = 'device:4'
    via.dispatchEvent(new Event('input'))
    await flushPromises()
    document.querySelector('[data-testid="mesh-path-save"]').click()
    await flushPromises()
    expect(api.saveKernelMeshPath).toHaveBeenCalledWith(null, { source: 'node:proxy-1', target: 'node:proxy-2', via: ['device:4'] })
    expect(wrapper.emitted().changed).toBeTruthy()
    wrapper.unmount()
  })
})
