// Node list (UI U7): the list template on UiDataTable, server search and
// status chips in the URL, rows that open the node page, and the
// registration key and parent-node deployment sheets.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { inBody } from './helpers/feedback'
import Nodes from '@/views/admin/Nodes.vue'

const adminApi = vi.hoisted(() => ({
  getNodes: vi.fn(),
  getNode: vi.fn(),
  getNodeStats: vi.fn(),
  getNodeLogs: vi.fn(),
  createNode: vi.fn(),
  updateNode: vi.fn(),
  deleteNode: vi.fn(),
  syncNodeProtocol: vi.fn(),
  getNodeCredentials: vi.fn(),
  getNodeProtocols: vi.fn(),
  createNodeProtocol: vi.fn(),
  updateNodeProtocol: vi.fn(),
  deleteNodeProtocol: vi.fn(),
  getProtocolTemplates: vi.fn(),
  getAuthKeys: vi.fn(),
  generateAuthKey: vi.fn(),
  generateWireGuardKeypair: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

const Stub = { template: '<div />' }

async function mountNodes(path = '/admin/nodes') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/nodes', component: Nodes },
      { path: '/admin/nodes/:id', component: Stub }
    ]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(Nodes, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

function parseAgentConfigSnippet(snippet) {
  return JSON.parse(String(snippet).slice(String(snippet).indexOf('\n') + 1))
}

enableAutoUnmount(afterEach)

describe('Nodes.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    adminApi.getNodes.mockResolvedValue({ data: { list: [], total: 0 } })
    adminApi.getNodeStats.mockResolvedValue({ data: { total: 0, online: 0, offline: 0, pending: 0 } })
    adminApi.getNodeCredentials.mockResolvedValue({ data: { api_key: '' } })
    adminApi.getAuthKeys.mockResolvedValue({ data: [] })
  })

  it('stops follow-up requests and shows the error state when the nodes do not load', async () => {
    adminApi.getNodes.mockRejectedValueOnce(new Error('401'))

    const { wrapper } = await mountNodes()

    expect(adminApi.getNodes).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodeStats).not.toHaveBeenCalled()
    expect(adminApi.getAuthKeys).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Couldn’t load nodes')

    adminApi.getNodes.mockResolvedValueOnce({ data: { list: [{ id: 1, name: 'hk-01', host: 'hk.example' }], total: 1 } })
    await wrapper.findAll('button').find(button => button.text() === 'Try again').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('hk-01')
    expect(adminApi.getNodeStats).toHaveBeenCalledTimes(1)
  })

  it('reads the list and the stats from legacy and panel envelopes', async () => {
    adminApi.getNodes.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { list: [{ id: 2, name: 'Envelope Node', host: 'env.example', protocols: [{ id: 1 }, { id: 2 }] }], total: 3 },
      ts: 1783526400000
    })
    adminApi.getNodeStats.mockResolvedValueOnce({ data: { total: 12, online: 7, offline: 4, pending: 1 } })

    const { wrapper } = await mountNodes()

    expect(wrapper.vm.nodes[0]).toMatchObject({ name: 'Envelope Node', address: 'env.example' })
    expect(wrapper.vm.pagination.total).toBe(3)
    expect(wrapper.find('[data-testid="node-stats"]').text()).toContain('12')
    expect(wrapper.find('tbody').text()).toContain('env.example')

    adminApi.getNodeStats.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { total: 21, online: 18, offline: 2, pending: 1 }, ts: 1 })
    await wrapper.vm.loadStats()
    expect(wrapper.vm.stats).toMatchObject({ total: 21, online: 18, offline: 2, pending: 1 })
  })

  it('searches and filters on the server and keeps the list state in the URL', async () => {
    const { wrapper, router } = await mountNodes('/admin/nodes?q=hk&status=offline&page=2')

    expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 2, page_size: 20, search: 'hk', status: 2 })

    await wrapper.findAll('[aria-pressed]').find(chip => chip.text() === 'Disabled').trigger('click')
    await flushPromises()
    expect(adminApi.getNodes).toHaveBeenLastCalledWith({ page: 1, page_size: 20, search: 'hk', status: 3 })
    expect(router.currentRoute.value.query).toEqual({ q: 'hk', status: 'disabled' })
  })

  it('opens the node page from a row', async () => {
    adminApi.getNodes.mockResolvedValue({ data: { list: [{ id: 5, name: 'hk-01', host: 'hk.example', status: 1 }], total: 1 } })
    const { wrapper, router } = await mountNodes()

    await wrapper.find('tbody tr').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/nodes/5')
  })

  it('shows a registration key once: the list masks keys, a generated key stays shown', async () => {
    adminApi.getAuthKeys.mockResolvedValue({ data: [{ id: 3, key: '********', used: 1 }] })
    adminApi.generateAuthKey.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { id: 4, name: 'Panel key', key: 'fresh-registration-key' },
      ts: 1783526400000
    })

    const { wrapper } = await mountNodes()
    await wrapper.find('[data-testid="open-auth-key"]').trigger('click')
    await flushPromises()

    const deploy = wrapper.vm.deploy
    expect(deploy.authKey).toBe('')
    expect(deploy.authKeyMasked).toBe(true)
    expect(inBody('[data-testid="auth-key-empty"]').text()).toContain('Hidden (********)')
    expect(parseAgentConfigSnippet(deploy.configSnippet).Nodes[0].AuthKey).toBe('<your-auth-key>')

    await inBody('[data-testid="generate-auth-key"]').trigger('click')
    await flushPromises()

    expect(adminApi.generateAuthKey).toHaveBeenCalledWith(expect.objectContaining({ expire_days: 0 }))
    expect(adminApi.generateAuthKey.mock.calls[0][0].name).toBeTruthy()
    expect(deploy.authKey).toBe('fresh-registration-key')
    // Masked in a password field with a reveal toggle.
    const field = inBody('[data-testid="auth-key-value"] input')
    expect(field.attributes('type')).toBe('password')
    expect(field.element.value).toBe('fresh-registration-key')
    expect(parseAgentConfigSnippet(deploy.configSnippet).Nodes[0].AuthKey).toBe('fresh-registration-key')

    // Reading the masked list again keeps the key this page issued.
    adminApi.getAuthKeys.mockResolvedValue({ data: [{ id: 4, key: '********', used: 0 }, { id: 3, key: '********', used: 1 }] })
    await deploy.loadAuthKeysPreview()
    expect(deploy.authKey).toBe('fresh-registration-key')
  })

  it('reads the parent nodes’ API keys into the Ansible inventory', async () => {
    adminApi.getNodes.mockResolvedValue({
      data: { list: [{ id: 9, name: 'Root Node', host: 'root.example' }, { id: 10, name: 'Child', host: 'child.example', parent_id: 9 }], total: 2 }
    })
    adminApi.getNodeCredentials.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { api_key: 'node-api-key' }, ts: 1 })

    const { wrapper } = await mountNodes()
    await wrapper.find('[data-testid="open-deploy"]').trigger('click')
    await flushPromises()

    expect(adminApi.getNodeCredentials).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodeCredentials).toHaveBeenCalledWith(9)
    expect(wrapper.vm.deploy.deployRows).toHaveLength(1)
    expect(wrapper.vm.deploy.deployInventoryPreview).toContain('root-node ansible_host=root.example')
    expect(wrapper.vm.deploy.deployInventoryPreview).toContain('node_id=9 api_key=node-api-key')
    expect(inBody('[data-testid="node-deploy-sheet"]').text()).toContain('inventory.ini')
  })
})
