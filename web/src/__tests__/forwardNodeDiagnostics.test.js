// 诊断 on the forwarding node page (views/admin/forward/NodeDetail.vue): a
// card with the node's diagnostics, whose forward checks run on one of the
// hops the node hosts.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { setLocale } from '@/i18n'

const api = vi.hoisted(() => ({ getNode: vi.fn(), listRoutes: vi.fn() }))
const kernelApi = vi.hoisted(() => ({
  getKernelNodeDiagnostics: vi.fn(),
  runKernelNodeDiagnostic: vi.fn(),
  rotateKernelAgentCredentials: vi.fn(),
  createKernelAgentInstallToken: vi.fn()
}))
vi.mock('@/api/forwardV4', async importOriginal => ({ ...(await importOriginal()), ...api }))
vi.mock('@/api/kernel', () => kernelApi)
vi.mock('vue-router', async importOriginal => ({
  ...(await importOriginal()),
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ query: {}, hash: '', params: {}, path: '/' }),
  onBeforeRouteLeave: () => {},
  onBeforeRouteUpdate: () => {}
}))

const NodeDetail = (await import('@/views/admin/forward/NodeDetail.vue')).default

const view = {
  node: {
    node_ref: 'forward-41', name: 'hk-edge-01', kind: 'forward', enabled: true, in_inventory: true, host: '203.0.113.41',
    desired_generation: '3', reported: true, reported_generation: '3', record: { transport: 'NODE_TRANSPORT_AGENT' }, capabilities: {}, info: {}
  },
  state: { hops: [{ route_id: 'r-1', hop_index: 1, role: 'relay', engine: 'ENGINE_GOST', listen: { port: 31000 } }] },
  report: {}
}

const CHECK = action => ({ action, mutating: false, params: ['route_id', 'hop_index'], forward: true })
const mounted = []

describe('forwarding node page: diagnostics', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    while (mounted.length) mounted.pop().unmount()
    vi.restoreAllMocks()
  })

  it('checks a hosted hop of the node on its own node ref', async () => {
    api.getNode.mockResolvedValue(view)
    api.listRoutes.mockResolvedValue({ routes: [{ route: { id: 'r-1', name: 'game-hk-tyo', hops: [{}, {}] } }], canDelete: true, truncated: false })
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue({
      node: 'forward-41', connected: true, agent_version: '4.2.0', diagnostics: true, forward_checks: true, services: ['gost'], can_restart: true, history: false, tasks: [],
      actions: [
        { action: 'service_status', mutating: false, params: ['service'], forward: false },
        CHECK('forward.listen'), CHECK('forward.connect')
      ]
    })
    kernelApi.runKernelNodeDiagnostic.mockResolvedValue({
      task_id: 'd-1', node: 'forward-41', action: 'forward.listen', params: {}, success: true, output: 'listening on 0.0.0.0:31000', error: '', duration_ms: 7,
      result: { check: 'forward.listen', status: 'ok', items: [{ target: '0.0.0.0:31000', protocol: 'tcp', status: 'ok', rtt_us: 0 }] }
    })
    const wrapper = mount(NodeDetail, {
      attachTo: document.body,
      props: { nodeRef: 'forward-41' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    mounted.push(wrapper)
    await flushPromises()

    expect(kernelApi.getKernelNodeDiagnostics).toHaveBeenCalledWith('forward-41')
    const section = wrapper.get('[data-testid="node-diagnostics"]')
    expect(section.text()).toContain('Forward checks')
    expect(section.text()).toContain('game-hk-tyo')
    // The card carries the title: the section does not repeat it.
    expect(wrapper.findAll('h2').filter(heading => heading.text() === 'Diagnostics')).toHaveLength(1)

    await wrapper.get('[data-testid="diagnostics-forward_listen"]').trigger('click')
    await flushPromises()
    expect(kernelApi.runKernelNodeDiagnostic).toHaveBeenCalledWith('forward-41', {
      action: 'forward.listen', params: { route_id: 'r-1', hop_index: 1 }, timeoutSeconds: 30
    })
    expect(wrapper.get('[data-testid="diagnostics-output"]').text()).toContain('listening on 0.0.0.0:31000')
  })

  it('lists nftables and ANX, not the retired gost and anixops engines', async () => {
    const engines = [
      { engine: 'ENGINE_NFTABLES', available: true, link_securities: ['LINK_SECURITY_RAW'] },
      { engine: 'ENGINE_GOST', available: false, unavailable_reason: 'removed: gost routes run on the network model' },
      { engine: 'ENGINE_ANIXOPS', available: false, unavailable_reason: 'experimental: not enabled in the Agent configuration' }
    ]
    api.getNode.mockResolvedValue({ ...view, node: { ...view.node, capabilities: { engines } } })
    api.listRoutes.mockResolvedValue({ routes: [], canDelete: true, truncated: false })
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue({ node: 'forward-41', connected: false, actions: [], tasks: [] })
    const wrapper = mount(NodeDetail, {
      attachTo: document.body,
      props: { nodeRef: 'forward-41' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    mounted.push(wrapper)
    await flushPromises()

    const rows = wrapper.findAll('[data-testid="node-engine"]')
    expect(rows.map(row => row.attributes('data-engine'))).toEqual(['ENGINE_NFTABLES', 'ENGINE_NETWORK'])
    expect(rows[1].text()).toContain('Managed by the network model')
    expect(wrapper.text()).not.toContain('removed: gost routes')
  })
})
