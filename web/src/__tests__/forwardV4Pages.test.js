import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSelect from '@/ui/UiSelect.vue'
import { setEdition } from '@/composables/useEdition'
import { setForwardFlags } from '@/components/forward/useForwardFlags'
import { inBody, toasts } from './helpers/feedback'

const api = vi.hoisted(() => ({
  listRoutes: vi.fn(),
  listNodes: vi.fn(),
  getRoute: vi.fn(),
  getNode: vi.fn(),
  previewRoute: vi.fn(),
  createRoute: vi.fn(),
  updateRoute: vi.fn(),
  pauseRoute: vi.fn(),
  resumeRoute: vi.fn(),
  deleteRoute: vi.fn(),
  trafficStats: vi.fn(),
  observabilityTargets: vi.fn(),
  listDnsBindings: vi.fn(),
  listDnsProviders: vi.fn(),
  listDnsKinds: vi.fn()
}))
const router = vi.hoisted(() => ({ push: vi.fn() }))
const route = vi.hoisted(() => ({ query: {}, hash: '', params: {}, path: '/' }))

vi.mock('@/api/forwardV4', async importOriginal => ({ ...(await importOriginal()), ...api }))
vi.mock('vue-router', async importOriginal => ({
  ...(await importOriginal()),
  useRouter: () => router,
  useRoute: () => route,
  onBeforeRouteLeave: () => {},
  onBeforeRouteUpdate: () => {}
}))

const { ForwardApiError } = await import('@/api/forwardV4')
const { resetForwardDataCache } = await import('@/components/forward/forwardData')
const Routes = (await import('@/views/admin/forward/Routes.vue')).default
const RouteEditor = (await import('@/views/admin/forward/RouteEditor.vue')).default
const Overview = (await import('@/views/admin/forward/Overview.vue')).default
const HopCard = (await import('@/components/forward/HopCard.vue')).default

const NODES = [
  { node_ref: 'forward-41', name: 'hk-edge-01', enabled: true, in_inventory: true, desired_generation: '18', reported: true, reported_generation: '18', info: { engines: [{ engine: 'ENGINE_NFTABLES', available: true, link_securities: ['LINK_SECURITY_RAW'] }, { engine: 'ENGINE_GOST', available: true, link_securities: ['LINK_SECURITY_RAW', 'LINK_SECURITY_TLS'] }] } },
  { node_ref: 'forward-61', name: 'tyo-exit-01', enabled: true, in_inventory: true, desired_generation: '12', info: { engines: [{ engine: 'ENGINE_GOST', available: true, link_securities: ['LINK_SECURITY_RAW', 'LINK_SECURITY_TLS'] }, { engine: 'ENGINE_ANIXOPS', available: true, link_securities: ['LINK_SECURITY_ANIXOPS'] }] } }
]

function storedRoute(id, fields = {}) {
  return {
    id, owner: 'admin', revision: '7', name: id,
    listen: { port: 30443, protocol: 'L4_PROTOCOL_TCP' },
    hops: [{ role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_GOST', node_refs: ['forward-41'] }],
    targets: [{ host: 'origin.example.com', port: 443 }],
    ...fields
  }
}

const mounted = []
function track(wrapper) {
  mounted.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.resetAllMocks()
  resetForwardDataCache()
  route.query = {}
  route.hash = ''
  api.listNodes.mockResolvedValue({ nodes: NODES, canDelete: true })
  api.trafficStats.mockResolvedValue({ totals: [], series: [], truncated: false })
  api.observabilityTargets.mockResolvedValue({ targets: [] })
  api.previewRoute.mockResolvedValue({ states: [], allocations: [], violations: [], warnings: [] })
  api.listDnsBindings.mockResolvedValue([])
  api.listDnsProviders.mockResolvedValue([])
  api.listDnsKinds.mockResolvedValue([])
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  setForwardFlags({ enableAnixOps: false })
  vi.useRealTimers()
})

describe('route list bulk actions (D9)', () => {
  it('pauses each route with its own Idempotency-Key, four at a time, and offers a retry for failures', async () => {
    const routes = Array.from({ length: 6 }, (_, index) => ({ route: storedRoute(`r${index}`) }))
    api.listRoutes.mockResolvedValue({ routes, canDelete: true, truncated: false })
    let running = 0
    let peak = 0
    api.pauseRoute.mockImplementation(async id => {
      running++
      peak = Math.max(peak, running)
      await new Promise(resolve => setTimeout(resolve, 5))
      running--
      if (id === 'r4') throw new ForwardApiError({ status: 409, code: 'revision_conflict' })
      return {}
    })
    const wrapper = track(mount(Routes, { attachTo: document.body }))
    await flushPromises()
    wrapper.findComponent(UiDataTable).vm.$emit('update:selected', routes.map(item => item.route.id))
    await flushPromises()
    await inBody('[data-testid="forward-bulk-pause"]').trigger('click')
    await vi.waitFor(() => expect(api.pauseRoute).toHaveBeenCalledTimes(6))
    await flushPromises()
    expect(peak).toBe(4)
    const keys = api.pauseRoute.mock.calls.map(call => call[1].idempotencyKey)
    expect(new Set(keys).size).toBe(6)
    await vi.waitFor(() => expect(toasts('warning').length).toBe(1))
    const toast = toasts('warning')[0]
    expect(toast.message).toContain('Paused 5, 1 failed.')
    expect(toast.action.label).toBe('Retry 1 failed')
  })

  it('shows the delete action disabled when the caller is not a super administrator (D7)', async () => {
    api.listRoutes.mockResolvedValue({ routes: [{ route: storedRoute('r0') }], canDelete: false, truncated: false })
    const wrapper = track(mount(Routes, { attachTo: document.body }))
    await flushPromises()
    const actions = wrapper.findComponent(UiDataTable).props('rowActions')(wrapper.findComponent(UiDataTable).props('rows')[0])
    const remove = actions.find(item => item.key === 'delete')
    expect(remove.disabled).toBe(true)
    expect(remove.label).toBe('Delete (super administrators only)')
  })

  it('offers a quota raise, not resume, on an enforced route (D8)', async () => {
    api.listRoutes.mockResolvedValue({ routes: [{ route: storedRoute('r0', { paused: true }), enforced: 'quota' }], canDelete: true, truncated: false })
    const wrapper = track(mount(Routes, { attachTo: document.body }))
    await flushPromises()
    const table = wrapper.findComponent(UiDataTable)
    const actions = table.props('rowActions')(table.props('rows')[0])
    expect(actions.map(item => item.key)).toContain('limits')
    expect(actions.map(item => item.key)).not.toContain('pause')
    expect(actions.find(item => item.key === 'limits').label).toBe('Raise quota…')
    expect(wrapper.text()).toContain('Quota used up')
  })
})

describe('route editor', () => {
  async function mountEditor() {
    const wrapper = track(mount(RouteEditor, { props: { id: '01J' }, attachTo: document.body }))
    await flushPromises()
    // The preview runs 1 s after the draft loads.
    await new Promise(resolve => setTimeout(resolve, 1100))
    await flushPromises()
    return wrapper
  }

  it('maps preview violations to their fields and blocks saving', async () => {
    api.getRoute.mockResolvedValue({ route: storedRoute('01J') })
    api.previewRoute.mockResolvedValue({
      states: [], allocations: [], warnings: [],
      violations: [{ field: 'listen.port', code: 'port_reserved', message: 'port 443 is reserved on forward-41' }, { field: 'targets[0].host', code: 'mystery_code', message: 'host refused' }]
    })
    const wrapper = await mountEditor()
    expect(api.previewRoute).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="forward-violations"]').text()).toContain('2 problems to fix before saving')
    expect(wrapper.text()).toContain('The port is reserved on the node')
    // An unknown code shows the API's message.
    expect(wrapper.text()).toContain('host refused')
    expect(wrapper.get('[data-testid="forward-save"]').attributes('disabled')).toBeDefined()
  })

  it('keeps the form on a revision conflict and reapplies with a new key (D10)', async () => {
    api.getRoute
      .mockResolvedValueOnce({ route: storedRoute('01J') })
      .mockResolvedValueOnce({ route: storedRoute('01J', { revision: '8', limits: { max_conns: 100 } }) })
    api.updateRoute
      .mockRejectedValueOnce(new ForwardApiError({ status: 409, code: 'revision_conflict', message: 'stale' }))
      .mockResolvedValueOnce({ route: storedRoute('01J', { revision: '9', name: 'renamed' }) })
    const wrapper = await mountEditor()
    await wrapper.get('#fwd-f-name').setValue('renamed')
    await new Promise(resolve => setTimeout(resolve, 1100))
    await flushPromises()
    await wrapper.get('[data-testid="forward-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="forward-conflict"]').text()).toContain('Someone else changed this route (revision 7 → 8)')
    expect(wrapper.get('#fwd-f-name').element.value).toBe('renamed')
    const firstKey = api.updateRoute.mock.calls[0][2].idempotencyKey

    await wrapper.get('[data-testid="forward-conflict-review"]').trigger('click')
    await flushPromises()
    expect(inBody('.editor-diff').text()).toContain('limits.max_conns')
    await inBody('[data-testid="forward-conflict-reapply"]').trigger('click')
    await flushPromises()
    expect(api.updateRoute).toHaveBeenCalledTimes(2)
    const [, body, options] = api.updateRoute.mock.calls[1]
    expect(body.revision).toBe('8')
    expect(body.name).toBe('renamed')
    expect(options.idempotencyKey).not.toBe(firstKey)
    expect(router.push).toHaveBeenCalledWith('/admin/forward/routes/01J')
  })

  it('reuses the Idempotency-Key when the same save is retried (D10)', async () => {
    api.getRoute.mockResolvedValue({ route: storedRoute('01J') })
    api.updateRoute
      .mockRejectedValueOnce(new ForwardApiError({ code: 'network' }))
      .mockResolvedValueOnce({ route: storedRoute('01J', { revision: '8' }) })
    const wrapper = await mountEditor()
    await wrapper.get('[data-testid="forward-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="forward-save-error"]').text()).toContain('Network error')
    await wrapper.get('[data-testid="forward-save"]').trigger('click')
    await flushPromises()
    const [first, second] = api.updateRoute.mock.calls.map(call => call[2].idempotencyKey)
    expect(second).toBe(first)
  })
})

describe('anixops engine (D12)', () => {
  function engineOptions(props) {
    const wrapper = track(mount(HopCard, {
      props: { hop: { _key: 'k', engine: 'ENGINE_GOST', node_refs: ['forward-61'], portMode: 'auto', dial_address: '', ingress: { security: 'LINK_SECURITY_RAW' } }, index: 1, total: 2, nodes: NODES, errorText: () => '', ...props },
      attachTo: document.body
    }))
    return wrapper.findAllComponents(UiSelect).find(select => select.props('label') === 'Engine').props('options')
  }

  it('is offered only with the experimental flag, as 「anixops（实验）」', () => {
    expect(engineOptions({}).map(option => option.value)).toEqual(['ENGINE_NFTABLES', 'ENGINE_GOST'])
    const options = engineOptions({ enableAnixOps: true })
    expect(options.map(option => option.value)).toEqual(['ENGINE_NFTABLES', 'ENGINE_GOST', 'ENGINE_ANIXOPS'])
    expect(options[2].label).toBe('anixops (experimental)')
  })

  it('stays shown on a hop that already uses it', () => {
    const wrapper = track(mount(HopCard, {
      props: { hop: { _key: 'k', engine: 'ENGINE_ANIXOPS', node_refs: ['forward-61'], portMode: 'auto', dial_address: '', ingress: { security: 'LINK_SECURITY_ANIXOPS' } }, index: 1, total: 2, nodes: NODES, errorText: () => '' },
      attachTo: document.body
    }))
    const selects = wrapper.findAllComponents(UiSelect)
    expect(selects.find(select => select.props('label') === 'Engine').props('options').map(option => option.value)).toContain('ENGINE_ANIXOPS')
    expect(selects.find(select => select.props('label') === 'Ingress link').props('options').map(option => option.value)).toContain('LINK_SECURITY_ANIXOPS')
  })
})

describe('overview edition gating', () => {
  beforeEach(() => {
    api.listRoutes.mockResolvedValue({ routes: [], canDelete: true, truncated: false })
  })

  it('links to plans only in the commercial edition', async () => {
    const community = track(mount(Overview, { attachTo: document.body }))
    await flushPromises()
    expect(community.find('[data-testid="forward-commercial-link"]').exists()).toBe(false)
    community.unmount()
    mounted.pop()

    setEdition('commercial')
    const commercial = track(mount(Overview, { attachTo: document.body }))
    await flushPromises()
    expect(commercial.get('[data-testid="forward-commercial-link"]').text()).toContain('Plans')
  })
})
