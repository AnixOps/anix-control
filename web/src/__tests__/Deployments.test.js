import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Deployments from '@/views/admin/Deployments.vue'

const kernelApi = vi.hoisted(() => ({
  applyKernelDeployment: vi.fn(),
  cancelKernelOperation: vi.fn(),
  createKernelTopology: vi.fn(),
  createKernelTopologyRevision: vi.fn(),
  deleteKernelNodeAssignment: vi.fn(),
  diagnoseKernelTopologyDeployment: vi.fn(),
  getKernelDeploymentStatus: vi.fn(),
  getKernelDeployments: vi.fn(),
  getKernelInstallations: vi.fn(),
  getKernelNodeAssignments: vi.fn(),
  getKernelOperations: vi.fn(),
  getKernelPluginReleases: vi.fn(),
  getKernelPlugins: vi.fn(),
  getKernelScopes: vi.fn(),
  getKernelTopologies: vi.fn(),
  getKernelTopologyRevision: vi.fn(),
  getKernelTopologyRevisions: vi.fn(),
  planKernelDeployment: vi.fn(),
  previewKernelTopologyDeployment: vi.fn(),
  rollbackKernelDeployment: vi.fn(),
  upsertKernelNodeAssignment: vi.fn(),
  validateKernelTopology: vi.fn(),
}))

const adminApi = vi.hoisted(() => ({ getNodes: vi.fn() }))

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('@/api/admin', () => adminApi)

const node = { id: 11, name: 'Shanghai entry', host: '10.0.0.11' }
const plugin = { id: 'gost-mesh', name: 'GOST Mesh', publisher: 'AnixOps' }
const release = {
  id: 3,
  plugin_id: plugin.id,
  version: '1.0.0',
  manifest: JSON.stringify({ id: plugin.id, version: '1.0.0', targets: ['agent'] }),
}
const installation = {
  id: 5,
  plugin_id: plugin.id,
  target: 'agent',
  desired_version: '1.0.0',
  observed_version: '1.0.0',
  config_revision: 6,
  state: 'healthy',
  enabled: true,
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((nextResolve, nextReject) => {
    resolve = nextResolve
    reject = nextReject
  })
  return { promise, reject, resolve }
}

function resolveEmptyState() {
  kernelApi.getKernelTopologies.mockResolvedValue([])
  kernelApi.getKernelDeployments.mockResolvedValue([])
  kernelApi.getKernelOperations.mockResolvedValue([])
  kernelApi.getKernelScopes.mockResolvedValue([])
  adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [] } })
  kernelApi.getKernelPlugins.mockResolvedValue([])
  kernelApi.getKernelPluginReleases.mockResolvedValue([])
  kernelApi.getKernelInstallations.mockResolvedValue([])
  kernelApi.getKernelNodeAssignments.mockResolvedValue([])
  kernelApi.getKernelTopologyRevisions.mockResolvedValue([])
  kernelApi.getKernelTopologyRevision.mockResolvedValue({ revision: {}, vertices: [], edges: [] })
  kernelApi.validateKernelTopology.mockResolvedValue({ valid: true, issues: [] })
  kernelApi.diagnoseKernelTopologyDeployment.mockResolvedValue({ valid: true, issues: [], checks: [], steps: [] })
  kernelApi.previewKernelTopologyDeployment.mockResolvedValue({ valid: true, issues: [], checks: [], steps: [] })
  kernelApi.createKernelTopology.mockResolvedValue({ id: 1, name: 'New topology', service_scope: 'forward' })
  kernelApi.createKernelTopologyRevision.mockResolvedValue({ id: 1, revision: 1, message: 'saved' })
  kernelApi.planKernelDeployment.mockResolvedValue({ id: 1, topology_id: 1, revision_id: 1, state: 'planned' })
  kernelApi.getKernelDeploymentStatus.mockResolvedValue({ deployment: { id: 1, state: 'planned' }, steps: [] })
  kernelApi.applyKernelDeployment.mockResolvedValue({ id: 1, state: 'applying' })
  kernelApi.rollbackKernelDeployment.mockResolvedValue({ id: 1, state: 'rollback_requested' })
  kernelApi.upsertKernelNodeAssignment.mockResolvedValue({ id: 1 })
  kernelApi.deleteKernelNodeAssignment.mockResolvedValue(undefined)
  kernelApi.cancelKernelOperation.mockResolvedValue({ id: 'op-1', state: 'cancel_requested' })
}

function resolveAssignmentState(assignments = []) {
  kernelApi.getKernelScopes.mockResolvedValue([{ id: 'forward', name: 'Forward', plugin_id: plugin.id }])
  adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [node] } })
  kernelApi.getKernelPlugins.mockResolvedValue([plugin])
  kernelApi.getKernelPluginReleases.mockResolvedValue([release])
  kernelApi.getKernelInstallations.mockResolvedValue([installation])
  kernelApi.getKernelNodeAssignments.mockResolvedValue(assignments)
}

function mountDeployments() {
  return mount(Deployments, { attachTo: document.body })
}

async function openTargets(wrapper) {
  await wrapper.get('[data-testid="deployment-targets"]').trigger('click')
  await flushPromises()
}

async function openAssignmentDrawer(wrapper) {
  await wrapper.get('[data-testid="new-assignment"]').trigger('click')
  await flushPromises()
  return wrapper.get('[data-testid="assignment-drawer"]')
}

describe('Deployments', () => {
  beforeEach(() => {
    vi.useRealTimers()
    vi.resetAllMocks()
    document.body.innerHTML = ''
    resolveEmptyState()
  })

  it('loads deployment resources without eagerly loading assignment form metadata', async () => {
    const wrapper = mountDeployments()
    await flushPromises()

    expect(kernelApi.getKernelTopologies).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelDeployments).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelScopes).toHaveBeenCalledTimes(1)
    expect(adminApi.getNodes).toHaveBeenCalledWith({ page: 1, page_size: 200 })
    expect(kernelApi.getKernelNodeAssignments).not.toHaveBeenCalled()
    expect(kernelApi.getKernelPlugins).not.toHaveBeenCalled()
    expect(kernelApi.getKernelPluginReleases).not.toHaveBeenCalled()
    expect(kernelApi.getKernelInstallations).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="deployment-topologies"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('links deployment tabs to their panels and supports roving keyboard navigation', async () => {
    resolveAssignmentState()
    const wrapper = mountDeployments()
    await flushPromises()
    const topologiesTab = wrapper.get('[data-testid="deployment-topologies"]')
    const targetsTab = wrapper.get('[data-testid="deployment-targets"]')

    expect(topologiesTab.attributes('id')).toBe('deployment-tab-topologies')
    expect(topologiesTab.attributes('aria-controls')).toBe('deployment-panel-topologies')
    expect(targetsTab.attributes('id')).toBe('deployment-tab-targets')
    expect(targetsTab.attributes('aria-controls')).toBe('deployment-panel-targets')
    expect(wrapper.get('[data-testid="deployment-topology-panel"]').attributes('id')).toBe('deployment-panel-topologies')
    expect(wrapper.get('[data-testid="deployment-topology-panel"]').attributes('aria-labelledby')).toBe('deployment-tab-topologies')
    expect(wrapper.get('[data-testid="deployment-target-panel"]').attributes('id')).toBe('deployment-panel-targets')
    expect(wrapper.get('[data-testid="deployment-target-panel"]').attributes('aria-labelledby')).toBe('deployment-tab-targets')

    await topologiesTab.trigger('keydown', { key: 'ArrowRight' })
    await flushPromises()
    expect(targetsTab.attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(targetsTab.element)

    await targetsTab.trigger('keydown', { key: 'ArrowLeft' })
    await flushPromises()
    expect(topologiesTab.attributes('aria-selected')).toBe('true')
    expect(document.activeElement).toBe(topologiesTab.element)

    await topologiesTab.trigger('keydown', { key: 'End' })
    await flushPromises()
    expect(targetsTab.attributes('aria-selected')).toBe('true')

    await targetsTab.trigger('keydown', { key: 'Home' })
    await flushPromises()
    expect(topologiesTab.attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('loads selected-node assignments on entering Targets and loads form metadata only when opening the drawer', async () => {
    resolveAssignmentState()
    const wrapper = mountDeployments()
    await flushPromises()

    await openTargets(wrapper)
    expect(kernelApi.getKernelNodeAssignments).toHaveBeenCalledWith(11)
    expect(kernelApi.getKernelPlugins).not.toHaveBeenCalled()
    expect(kernelApi.getKernelPluginReleases).not.toHaveBeenCalled()
    expect(kernelApi.getKernelInstallations).not.toHaveBeenCalled()

    const drawer = await openAssignmentDrawer(wrapper)
    expect(drawer.exists()).toBe(true)
    expect(kernelApi.getKernelPlugins).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelPluginReleases).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelInstallations).toHaveBeenCalledTimes(1)
    expect(drawer.get('#assignment-node').element.value).toBe('11')
    expect(drawer.get('#assignment-plugin').element.value).toBe('gost-mesh')
    expect(drawer.get('#assignment-scope').element.value).toBe('forward')
    expect(drawer.get('#assignment-role').element.value).toBe('relay')
    expect(drawer.get('#assignment-version').element.value).toBe('1.0.0')
    expect(drawer.get('#assignment-config-revision').element.value).toBe('6')
    wrapper.unmount()
  })

  it('loads default-node assignments when Targets opens before initial nodes resolve', async () => {
    const nodesRequest = deferred()
    adminApi.getNodes.mockReturnValue(nodesRequest.promise)
    const wrapper = mountDeployments()

    await wrapper.get('[data-testid="deployment-targets"]').trigger('click')
    expect(kernelApi.getKernelNodeAssignments).not.toHaveBeenCalled()

    nodesRequest.resolve({ code: 0, data: { list: [node] } })
    await flushPromises()
    expect(kernelApi.getKernelNodeAssignments).toHaveBeenCalledWith(11)
    wrapper.unmount()
  })

  it('keeps the current target rows when an older assignment request resolves late', async () => {
    const firstAssignments = deferred()
    const secondNode = { id: 22, name: 'Tokyo entry', host: '10.0.0.22' }
    const firstAssignment = { id: 7, node_id: 11, service_scope: 'forward', plugin_id: 'gost-mesh', role: 'relay', enabled: true }
    const secondAssignment = { id: 8, node_id: 22, service_scope: 'forward', plugin_id: 'nat-egress', role: 'egress', enabled: true }
    adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [node, secondNode] } })
    kernelApi.getKernelNodeAssignments.mockImplementation(nodeID => {
      return Number(nodeID) === 11 ? firstAssignments.promise : Promise.resolve([secondAssignment])
    })
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="deployment-targets"]').trigger('click')
    await nextTick()
    const picker = wrapper.get('#assignment-node-filter')
    picker.element.value = '22'
    await picker.trigger('change')
    await flushPromises()
    expect(wrapper.get('[data-testid="deployment-target-panel"]').text()).toContain('nat-egress')

    firstAssignments.resolve([firstAssignment])
    await flushPromises()
    expect(wrapper.get('[data-testid="deployment-target-panel"]').text()).toContain('nat-egress')
    expect(wrapper.get('[data-testid="deployment-target-panel"]').text()).not.toContain('gost-mesh')
    wrapper.unmount()
  })

  it('creates an assignment and keeps an assignment failure visible in its drawer', async () => {
    resolveAssignmentState()
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)
    const drawer = await openAssignmentDrawer(wrapper)

    await drawer.get('#assignment-rollout-group').setValue('canary-a')
    await drawer.get('[data-testid="save-assignment"]').trigger('click')
    await flushPromises()

    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenCalledWith(11, {
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      enabled: true,
      rollout_group: 'canary-a',
    })
    expect(kernelApi.getKernelNodeAssignments.mock.calls.length).toBeGreaterThan(1)

    kernelApi.upsertKernelNodeAssignment.mockRejectedValueOnce({
      response: { data: { error: { message: 'agent package artifact is missing' } } },
    })
    await wrapper.get('[data-testid="new-assignment"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="save-assignment"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="assignment-drawer"]').text()).toContain('agent package artifact is missing')
    wrapper.unmount()
  })

  it('selects and refreshes the chosen target after creating an assignment there', async () => {
    const secondNode = { id: 22, name: 'Tokyo entry', host: '10.0.0.22' }
    const secondNodeAssignment = {
      id: 8,
      node_id: 22,
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      rollout_group: 'tokyo-canary',
      enabled: true,
    }
    resolveAssignmentState()
    adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [node, secondNode] } })
    kernelApi.getKernelNodeAssignments.mockImplementation(nodeID => Promise.resolve(Number(nodeID) === 22 ? [secondNodeAssignment] : []))
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)
    const drawer = await openAssignmentDrawer(wrapper)

    await drawer.get('#assignment-node').setValue('22')
    await drawer.get('[data-testid="save-assignment"]').trigger('click')
    await flushPromises()

    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenCalledWith(22, expect.objectContaining({ plugin_id: 'gost-mesh' }))
    expect(wrapper.get('#assignment-node-filter').element.value).toBe('22')
    expect(kernelApi.getKernelNodeAssignments.mock.calls.filter(([nodeID]) => nodeID === 22)).toHaveLength(1)
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="deployment-target-panel"]').text()).toContain('tokyo-canary')
    wrapper.unmount()
  })

  it('keeps a cross-target assignment operation tracked when an older poll resolves late', async () => {
    vi.useFakeTimers()
    const stalePoll = deferred()
    const secondNode = { id: 22, name: 'Tokyo entry', host: '10.0.0.22' }
    const activeAssignmentOperation = { id: 'assignment-op-22', kind: 'plugin.enable', node_id: 22, state: 'running' }
    resolveAssignmentState()
    adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [node, secondNode] } })
    kernelApi.getKernelNodeAssignments.mockImplementation(nodeID => Promise.resolve(Number(nodeID) === 22 ? [{
      id: 8,
      node_id: 22,
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      enabled: true,
    }] : []))
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([{ id: 'prior-target-op', kind: 'plugin.enable', node_id: 11, state: 'running' }])
      .mockReturnValueOnce(stalePoll.promise)
      .mockResolvedValueOnce([activeAssignmentOperation])
      .mockResolvedValueOnce([activeAssignmentOperation])
    kernelApi.upsertKernelNodeAssignment.mockResolvedValue({ operation: activeAssignmentOperation })
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)

    vi.advanceTimersByTime(2000)
    await nextTick()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)

    const drawer = await openAssignmentDrawer(wrapper)
    await drawer.get('#assignment-node').setValue('22')
    await drawer.get('[data-testid="save-assignment"]').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(3)

    stalePoll.resolve([])
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(4)
    wrapper.unmount()
  })

  it('keeps a post-save operation when an older manual refresh resolves late', async () => {
    vi.useFakeTimers()
    const staleRefresh = deferred()
    const activeAssignmentOperation = { id: 'assignment-op-refresh', kind: 'plugin.enable', node_id: 11, state: 'running' }
    resolveAssignmentState()
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([])
      .mockReturnValueOnce(staleRefresh.promise)
      .mockResolvedValueOnce([activeAssignmentOperation])
      .mockResolvedValueOnce([activeAssignmentOperation])
    kernelApi.upsertKernelNodeAssignment.mockResolvedValue({ operation: activeAssignmentOperation })
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)
    const drawer = await openAssignmentDrawer(wrapper)

    await wrapper.get('[data-testid="refresh-deployments"]').trigger('click')
    await nextTick()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)

    await drawer.get('[data-testid="save-assignment"]').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(3)
    expect(wrapper.get('[data-testid="operation-row-assignment-op-refresh"]').exists()).toBe(true)

    staleRefresh.resolve([])
    await flushPromises()
    expect(wrapper.get('[data-testid="operation-row-assignment-op-refresh"]').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(4)
    wrapper.unmount()
  })

  it('tracks selected-node operations after an assignment mutation until they become terminal', async () => {
    vi.useFakeTimers()
    resolveAssignmentState()
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([{ id: 'assignment-op', kind: 'plugin.enable', node_id: 11, state: 'running' }])
      .mockResolvedValueOnce([{ id: 'assignment-op', kind: 'plugin.enable', node_id: 11, state: 'completed' }])
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)
    const drawer = await openAssignmentDrawer(wrapper)

    await drawer.get('[data-testid="save-assignment"]').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)
    expect(kernelApi.getKernelTopologies).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelDeployments).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(3)
    expect(kernelApi.getKernelTopologies).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelDeployments).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('polls an already-running operation for the selected target until it becomes terminal', async () => {
    vi.useFakeTimers()
    resolveAssignmentState()
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([{ id: 'existing-target-op', kind: 'plugin.enable', node_id: 11, state: 'running' }])
      .mockResolvedValueOnce([{ id: 'existing-target-op', kind: 'plugin.enable', node_id: 11, state: 'completed' }])
    const wrapper = mountDeployments()
    await flushPromises()

    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(1)
    await openTargets(wrapper)
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('keeps an assignment mutation bound to its initiating target after a target switch', async () => {
    const mutation = deferred()
    const assignment = {
      id: 7,
      node_id: 11,
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      rollout_group: '',
      enabled: true,
    }
    const secondNode = { id: 22, name: 'Tokyo entry', host: '10.0.0.22' }
    adminApi.getNodes.mockResolvedValue({ code: 0, data: { list: [node, secondNode] } })
    kernelApi.getKernelNodeAssignments.mockResolvedValue([assignment])
    kernelApi.upsertKernelNodeAssignment.mockReturnValueOnce(mutation.promise)
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)

    await wrapper.get('[data-testid="toggle-assignment-7"]').trigger('click')
    await nextTick()
    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenCalledWith(11, expect.objectContaining({ enabled: false }))
    const picker = wrapper.get('#assignment-node-filter')
    expect(picker.attributes('disabled')).toBeDefined()

    wrapper.vm.$.setupState.selectedNodeID = 22
    await nextTick()
    await flushPromises()
    expect(picker.element.value).toBe('22')
    mutation.resolve({ id: 7 })
    await flushPromises()

    expect(kernelApi.getKernelNodeAssignments.mock.calls.filter(([nodeID]) => nodeID === 22)).toHaveLength(0)
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('edits, toggles, and deletes a selected-node assignment without changing identity fields', async () => {
    const assignment = {
      id: 7,
      node_id: 11,
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      rollout_group: 'canary-a',
      enabled: false,
    }
    resolveAssignmentState([assignment])
    const confirmSpy = vi.spyOn(globalThis, 'confirm').mockReturnValue(true)
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)

    await wrapper.get('[data-testid="edit-assignment-7"]').trigger('click')
    await flushPromises()
    const drawer = wrapper.get('[data-testid="assignment-drawer"]')
    for (const selector of ['#assignment-node', '#assignment-plugin', '#assignment-scope', '#assignment-role']) {
      expect(drawer.get(selector).attributes('disabled')).toBeDefined()
    }
    await drawer.get('#assignment-config-revision').setValue('8')
    await drawer.get('#assignment-rollout-group').setValue('canary-b')
    await drawer.get('[data-testid="save-assignment"]').trigger('click')
    await flushPromises()
    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenLastCalledWith(11, expect.objectContaining({
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_config_revision: 8,
      rollout_group: 'canary-b',
      enabled: false,
    }))

    await wrapper.get('[data-testid="toggle-assignment-7"]').trigger('click')
    await flushPromises()
    expect(kernelApi.upsertKernelNodeAssignment).toHaveBeenLastCalledWith(11, expect.objectContaining({
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      rollout_group: 'canary-a',
      enabled: true,
    }))

    await wrapper.get('[data-testid="delete-assignment-7"]').trigger('click')
    await flushPromises()
    expect(kernelApi.deleteKernelNodeAssignment).toHaveBeenCalledWith(11, 7)
    wrapper.unmount()
    confirmSpy.mockRestore()
  })

  it('requires explicit confirmation before deleting an assignment', async () => {
    const assignment = {
      id: 7,
      node_id: 11,
      service_scope: 'forward',
      plugin_id: 'gost-mesh',
      role: 'relay',
      desired_version: '1.0.0',
      desired_config_revision: 6,
      rollout_group: 'canary-a',
      enabled: true,
    }
    resolveAssignmentState([assignment])
    const confirmSpy = vi.spyOn(globalThis, 'confirm')
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true)
    const wrapper = mountDeployments()
    await flushPromises()
    await openTargets(wrapper)

    await wrapper.get('[data-testid="delete-assignment-7"]').trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    expect(kernelApi.deleteKernelNodeAssignment).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="delete-assignment-7"]').trigger('click')
    await flushPromises()
    expect(kernelApi.deleteKernelNodeAssignment).toHaveBeenCalledWith(11, 7)
    wrapper.unmount()
    confirmSpy.mockRestore()
  })

  it('creates a topology and opens its revision workspace', async () => {
    kernelApi.getKernelScopes.mockResolvedValue([{ id: 'forward', name: 'Forward' }])
    kernelApi.createKernelTopology.mockResolvedValue({ id: 5, name: 'New topology', service_scope: 'forward' })
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="new-topology"]').trigger('click')
    await wrapper.get('#new-topology-name').setValue('New topology')
    await wrapper.get('#create-topology').trigger('click')
    await flushPromises()

    expect(kernelApi.createKernelTopology).toHaveBeenCalledWith({
      name: 'New topology',
      service_scope: 'forward',
      description: '',
    })
    expect(wrapper.get('[data-testid="topology-workspace"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps a successfully created topology open when its list refresh fails', async () => {
    kernelApi.getKernelScopes.mockResolvedValue([{ id: 'forward', name: 'Forward' }])
    kernelApi.getKernelTopologies
      .mockResolvedValueOnce([])
      .mockRejectedValueOnce(new Error('topology list refresh failed'))
    kernelApi.createKernelTopology.mockResolvedValue({ id: 5, name: 'New topology', service_scope: 'forward' })
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="new-topology"]').trigger('click')
    await wrapper.get('#new-topology-name').setValue('New topology')
    await wrapper.get('#create-topology').trigger('click')
    await flushPromises()

    expect(kernelApi.createKernelTopology).toHaveBeenCalledTimes(1)
    expect(wrapper.get('#topology-editor-title').exists()).toBe(true)
    expect(wrapper.text()).toContain('topology list refresh failed')
    wrapper.unmount()
  })

  it('loads a revision, diagnoses it, saves it, and plans with kernel request field names', async () => {
    kernelApi.getKernelTopologies.mockResolvedValue([{ id: 3, name: 'CN dedicated', service_scope: 'forward', active_revision_id: 10 }])
    kernelApi.getKernelTopologyRevisions.mockResolvedValue([{ id: 10, topology_id: 3, revision: 1, message: 'initial' }])
    kernelApi.getKernelTopologyRevision.mockImplementation(async (_topologyID, revisionID) => ({
      revision: { id: revisionID, message: revisionID === 11 ? 'canary' : 'initial' },
      vertices: [{ key: 'entry', kind: 'plugin', node_id: 11, plugin_id: 'nftables-forward', role: 'cn_dedicated_nftables', config: '{}' }],
      edges: [],
    }))
    kernelApi.createKernelTopologyRevision.mockResolvedValue({ id: 11, topology_id: 3, revision: 2, message: 'canary' })
    kernelApi.planKernelDeployment.mockResolvedValue({ id: 22, topology_id: 3, revision_id: 11, state: 'planned' })
    kernelApi.getKernelDeploymentStatus.mockResolvedValue({ deployment: { id: 22, topology_id: 3, revision_id: 11, state: 'planned' }, steps: [] })
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="edit-topology-3"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('#topology-editor-json').element.value).toContain('entry')
    await wrapper.get('#topology-revision-message').setValue('canary')
    await wrapper.get('#topology-rollout-group').setValue('canary-a')
    await wrapper.get('#topology-diagnose').trigger('click')
    await flushPromises()
    expect(kernelApi.validateKernelTopology).toHaveBeenCalledWith(expect.objectContaining({
      message: 'canary',
      vertices: [expect.objectContaining({ config: '{}' })],
    }))
    expect(kernelApi.diagnoseKernelTopologyDeployment).toHaveBeenCalledWith(3, 10, {
      rolloutGroup: 'canary-a',
      failurePolicy: 'stop_and_rollback',
    })

    await wrapper.get('#topology-save-revision').trigger('click')
    await flushPromises()
    expect(kernelApi.createKernelTopologyRevision).toHaveBeenCalledWith(3, expect.objectContaining({ message: 'canary' }))
    await wrapper.get('#topology-rollout-group').setValue('canary-a')
    await wrapper.get('#topology-plan').trigger('click')
    await flushPromises()
    expect(kernelApi.planKernelDeployment).toHaveBeenCalledWith(expect.objectContaining({
      topology_id: 3,
      revision_id: 11,
      rollout_group: 'canary-a',
      failure_policy: 'stop_and_rollback',
    }))
    expect(kernelApi.getKernelDeploymentStatus).toHaveBeenCalledWith(22)
    wrapper.unmount()
  })

  it('refreshes the selected deployment after apply and only rolls back after an explicit confirmation', async () => {
    kernelApi.getKernelTopologies.mockResolvedValue([{ id: 4, name: 'Mesh', service_scope: 'forward', active_revision_id: 30 }])
    kernelApi.getKernelDeployments.mockResolvedValue([{ id: 44, topology_id: 4, revision_id: 30, state: 'planned', rollout_group: 'canary' }])
    kernelApi.getKernelTopologyRevisions.mockResolvedValue([{ id: 30, topology_id: 4, revision: 1, message: 'initial' }])
    kernelApi.getKernelTopologyRevision.mockResolvedValue({ revision: { id: 30, message: 'initial' }, vertices: [], edges: [] })
    kernelApi.getKernelDeploymentStatus.mockResolvedValue({ deployment: { id: 44, topology_id: 4, state: 'planned' }, steps: [{ id: 1 }] })
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="view-deployment-44"]').trigger('click')
    await flushPromises()
    await wrapper.get('#topology-apply').trigger('click')
    await flushPromises()
    expect(kernelApi.applyKernelDeployment).toHaveBeenCalledWith(44)
    expect(kernelApi.getKernelDeploymentStatus.mock.calls.length).toBeGreaterThan(1)

    await wrapper.get('#topology-rollback').trigger('click')
    expect(kernelApi.rollbackKernelDeployment).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-rollback"]').trigger('click')
    await flushPromises()
    expect(kernelApi.rollbackKernelDeployment).toHaveBeenCalledWith(44)
    wrapper.unmount()
  })

  it('does not attach a stale deployment status after switching topology editors', async () => {
    const firstStatus = deferred()
    kernelApi.getKernelTopologies.mockResolvedValue([
      { id: 4, name: 'Mesh', service_scope: 'forward', active_revision_id: 30 },
      { id: 5, name: 'Tokyo mesh', service_scope: 'forward', active_revision_id: 40 },
    ])
    kernelApi.getKernelDeployments.mockResolvedValue([{ id: 44, topology_id: 4, revision_id: 30, state: 'planned' }])
    kernelApi.getKernelTopologyRevisions.mockImplementation(topologyID => Promise.resolve([
      { id: Number(topologyID) === 4 ? 30 : 40, topology_id: topologyID, revision: 1, message: 'initial' },
    ]))
    kernelApi.getKernelTopologyRevision.mockResolvedValue({ revision: { id: 30, message: 'initial' }, vertices: [], edges: [] })
    kernelApi.getKernelDeploymentStatus.mockImplementation(deploymentID => {
      if (Number(deploymentID) === 44) return firstStatus.promise
      return Promise.resolve({ deployment: { id: deploymentID, state: 'planned' }, steps: [] })
    })
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="view-deployment-44"]').trigger('click')
    await flushPromises()
    expect(kernelApi.getKernelDeploymentStatus).toHaveBeenCalledWith(44)

    await wrapper.get('[data-testid="edit-topology-5"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('.workspace-meta').text()).toContain('Tokyo mesh')
    expect(wrapper.find('.deployment-status').exists()).toBe(false)

    firstStatus.resolve({ deployment: { id: 44, topology_id: 4, state: 'applying' }, steps: [] })
    await flushPromises()
    expect(wrapper.get('.workspace-meta').text()).toContain('Tokyo mesh')
    expect(wrapper.find('.deployment-status').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows selected deployment activity from the kernel topology_deployment_id field', async () => {
    kernelApi.getKernelTopologies.mockResolvedValue([{ id: 4, name: 'Mesh', service_scope: 'forward', active_revision_id: 30 }])
    kernelApi.getKernelDeployments.mockResolvedValue([{ id: 44, topology_id: 4, revision_id: 30, state: 'planned' }])
    kernelApi.getKernelOperations.mockResolvedValue([{ id: 'deployment-op', kind: 'plugin.configure', topology_deployment_id: 44, state: 'running' }])
    kernelApi.getKernelTopologyRevisions.mockResolvedValue([{ id: 30, topology_id: 4, revision: 1, message: 'initial' }])
    kernelApi.getKernelTopologyRevision.mockResolvedValue({ revision: { id: 30, message: 'initial' }, vertices: [], edges: [] })
    kernelApi.getKernelDeploymentStatus.mockResolvedValue({ deployment: { id: 44, topology_id: 4, state: 'planned' }, steps: [] })
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="view-deployment-44"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="operation-row-deployment-op"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not poll an opened planned deployment', async () => {
    vi.useFakeTimers()
    kernelApi.getKernelTopologies.mockResolvedValue([{ id: 4, name: 'Mesh', service_scope: 'forward', active_revision_id: 30 }])
    kernelApi.getKernelDeployments.mockResolvedValue([{ id: 44, topology_id: 4, revision_id: 30, state: 'planned' }])
    kernelApi.getKernelTopologyRevisions.mockResolvedValue([{ id: 30, topology_id: 4, revision: 1, message: 'initial' }])
    kernelApi.getKernelTopologyRevision.mockResolvedValue({ revision: { id: 30, message: 'initial' }, vertices: [], edges: [] })
    kernelApi.getKernelDeploymentStatus.mockResolvedValue({ deployment: { id: 44, topology_id: 4, state: 'planned' }, steps: [] })
    const wrapper = mountDeployments()
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="view-deployment-44"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('cancels visible activity and polls only the active deployment until it becomes terminal', async () => {
    vi.useFakeTimers()
    kernelApi.getKernelOperations
      .mockResolvedValueOnce([{ id: 'op-1', kind: 'deployment.apply', deployment_id: 44, state: 'running' }])
      .mockResolvedValueOnce([{ id: 'op-1', kind: 'deployment.apply', deployment_id: 44, state: 'completed' }])
    kernelApi.getKernelDeployments.mockResolvedValue([{ id: 44, topology_id: 4, state: 'applying' }])
    const wrapper = mountDeployments()
    await flushPromises()

    await wrapper.get('[data-testid="show-all-activity"]').trigger('click')
    await wrapper.get('[data-testid="cancel-operation-op-1"]').trigger('click')
    await flushPromises()
    expect(kernelApi.cancelKernelOperation).toHaveBeenCalledWith('op-1')

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(kernelApi.getKernelOperations).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('keeps a server load failure visible instead of showing an empty topology state', async () => {
    let rejectRequest
    kernelApi.getKernelTopologies.mockReturnValue(new Promise((resolve, reject) => { rejectRequest = reject }))
    const wrapper = mountDeployments()
    await nextTick()
    expect(wrapper.attributes('aria-busy')).toBe('true')
    rejectRequest({ response: { data: { error: { message: 'kernel unavailable' } } } })
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('kernel unavailable')
    expect(wrapper.text()).not.toContain('No topologies')
    wrapper.unmount()
  })
})
