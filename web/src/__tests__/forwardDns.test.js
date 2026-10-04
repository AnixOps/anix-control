import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UiCheckbox from '@/ui/UiCheckbox.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiSwitch from '@/ui/UiSwitch.vue'
import { answerConfirms, inBody, toastMessages } from './helpers/feedback'

// Entry HA through DNS in the forwarding UI (L2): the schema-driven
// provider form, write-only credentials, super-administrator gating, the
// editor's binding picker and the route page's 入口高可用 card.

const api = vi.hoisted(() => ({
  listNodes: vi.fn(),
  getRoute: vi.fn(),
  previewRoute: vi.fn(),
  createRoute: vi.fn(),
  updateRoute: vi.fn(),
  listDnsKinds: vi.fn(),
  listDnsProviders: vi.fn(),
  createDnsProvider: vi.fn(),
  updateDnsProvider: vi.fn(),
  deleteDnsProvider: vi.fn(),
  listDnsBindings: vi.fn(),
  createDnsBinding: vi.fn(),
  updateDnsBinding: vi.fn(),
  deleteDnsBinding: vi.fn(),
  routeDns: vi.fn()
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

const { ForwardApiError, bindingBody, credentialsBody } = await import('@/api/forwardV4')
const model = await import('@/components/forward/dnsModel')
const DnsProviders = (await import('@/views/admin/forward/DnsProviders.vue')).default
const DnsProviderSheet = (await import('@/components/forward/DnsProviderSheet.vue')).default
const EntryHaCard = (await import('@/components/forward/EntryHaCard.vue')).default
const RouteEditor = (await import('@/views/admin/forward/RouteEditor.vue')).default
const DnsBindingPicker = (await import('@/components/forward/DnsBindingPicker.vue')).default

const KINDS = [
  { kind: 'DNS_PROVIDER_KIND_CLOUDFLARE', name: 'Cloudflare', config: ['endpoint'], required_config: [], credentials: ['api_token'] },
  { kind: 'DNS_PROVIDER_KIND_ALIDNS', name: 'Alibaba Cloud DNS', config: ['endpoint'], required_config: [], credentials: ['access_key_id', 'access_key_secret'] },
  { kind: 'DNS_PROVIDER_KIND_WEBHOOK', name: 'Webhook', config: ['url'], required_config: ['url'], credentials: ['secret'] }
]
const CLOUDFLARE = { id: '1', name: 'cloudflare-main', kind: 'DNS_PROVIDER_KIND_CLOUDFLARE', credential_names: ['api_token'], bindings: 2, updated_at_unix_ms: '1759579200000' }
const HOOK = { id: '2', name: 'hook', kind: 'DNS_PROVIDER_KIND_WEBHOOK', config: { url: 'https://hook.example.com/dns' }, credential_names: ['secret'] }
const BINDING = {
  id: '4', route_id: '01J', provider_id: '1', zone: 'example.net', record_name: 'edge.example.net', mode: 'DNS_BINDING_MODE_DDNS',
  record_types: ['DNS_RECORD_TYPE_A'], ttl: 60, created_at_unix_ms: '1759000000000', updated_at_unix_ms: '1759000000000'
}
const t = (key, values) => (values ? `${key} ${JSON.stringify(values)}` : key)

const mounted = []
function track(wrapper) {
  mounted.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.resetAllMocks()
  route.query = {}
  route.hash = ''
  api.listNodes.mockResolvedValue({ nodes: [], canDelete: true })
  api.listDnsKinds.mockResolvedValue(KINDS)
  api.listDnsProviders.mockResolvedValue([CLOUDFLARE, HOOK])
  api.listDnsBindings.mockResolvedValue([])
  api.previewRoute.mockResolvedValue({ states: [], allocations: [], violations: [], warnings: [] })
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  vi.useRealTimers()
})

describe('dnsModel', () => {
  it('builds the form from the kind schema, stored credentials as ********', () => {
    const form = model.providerForm(CLOUDFLARE, KINDS)
    expect(form.config).toEqual({ endpoint: '' })
    expect(form.credentials).toEqual({ api_token: model.CREDENTIAL_PLACEHOLDER })
    const fresh = model.switchKind(model.providerForm(null, KINDS), 'DNS_PROVIDER_KIND_ALIDNS', KINDS)
    expect(fresh.credentials).toEqual({ access_key_id: '', access_key_secret: '' })
  })

  it('leaves out credentials still at the placeholder and empty config', () => {
    const form = { ...model.providerForm(HOOK, KINDS), name: ' hook ' }
    expect(model.providerRequest(form, KINDS)).toEqual({
      provider: { name: 'hook', kind: 'DNS_PROVIDER_KIND_WEBHOOK', config: { url: 'https://hook.example.com/dns' } },
      credentials: {}
    })
    form.credentials.secret = 'new-secret'
    expect(model.providerRequest(form, KINDS).credentials).toEqual({ secret: 'new-secret' })
  })

  it('requires required_config and the credentials of a new provider only', () => {
    const fresh = model.switchKind({ ...model.providerForm(null, KINDS), name: 'x' }, 'DNS_PROVIDER_KIND_WEBHOOK', KINDS)
    expect(model.providerErrors(fresh, KINDS, 'Required')).toEqual({ 'config.url': 'Required', 'credentials.secret': 'Required' })
    const stored = model.providerForm(HOOK, KINDS)
    expect(model.providerErrors(stored, KINDS, 'Required')).toEqual({})
    stored.credentials.secret = ''
    expect(model.providerErrors(stored, KINDS, 'Required')).toEqual({})
  })

  it('writes a new DDNS binding on the entry hostname and an update as the whole stored binding', () => {
    const draft = { ...model.bindingDraft(null, 'edge.example.net'), enabled: true, provider_id: '1' }
    expect(draft.zone).toBe('example.net')
    expect(model.bindingRequest(draft, '01J', 'edge.example.net')).toEqual({
      route_id: '01J', provider_id: '1', zone: 'example.net', record_name: 'edge.example.net', mode: 'DNS_BINDING_MODE_DDNS',
      record_types: ['DNS_RECORD_TYPE_A'], ttl: 60, paused: false
    })
    const edit = { ...model.bindingDraft(BINDING), record_types: ['DNS_RECORD_TYPE_AAAA', 'DNS_RECORD_TYPE_A'], ttl: 120 }
    expect(model.bindingChanged(edit, BINDING)).toBe(true)
    expect(model.bindingRequest(edit, '01J', 'edge.example.net', BINDING)).toEqual({ ...BINDING, record_types: ['DNS_RECORD_TYPE_A', 'DNS_RECORD_TYPE_AAAA'], ttl: 120, paused: false })
    expect(model.bindingChanged(model.bindingDraft(BINDING), BINDING)).toBe(false)
  })

  it('checks the binding draft', () => {
    const draft = { ...model.bindingDraft(null, 'edge.example.net'), enabled: true, record_types: [], ttl: 90000, zone: 'other.org' }
    expect(model.bindingErrors(draft, 'edge.example.net', t)).toEqual({
      provider_id: 'forwardDns.binding.required',
      zone: 'forwardDns.binding.outsideZone',
      record_types: 'forwardDns.binding.typeRequired',
      ttl: 'forwardDns.binding.ttlRange {"max":86400}'
    })
    const cname = { ...draft, provider_id: '1', zone: 'ha.example.org', mode: 'DNS_BINDING_MODE_CNAME', record_name: '', record_types: ['DNS_RECORD_TYPE_A'], ttl: 60 }
    expect(model.bindingErrors(cname, 'hk.customer.example', t)).toEqual({ record_name: 'forwardDns.binding.required' })
    expect(model.bindingErrors({ ...cname, record_name: 'r1.ha.example.org' }, 'hk.customer.example', t)).toEqual({})
    expect(model.bindingErrors({ ...draft, enabled: false }, '', t)).toEqual({})
  })

  it('sends only the binding fields and the credentials that carry a value', () => {
    expect(bindingBody({ ...BINDING, extra: 1 })).toEqual({ id: '4', route_id: '01J', provider_id: '1', zone: 'example.net', record_name: 'edge.example.net', mode: 'DNS_BINDING_MODE_DDNS', record_types: ['DNS_RECORD_TYPE_A'], ttl: 60 })
    expect(credentialsBody({ api_token: 'x', other: '' })).toEqual({ api_token: 'x' })
  })
})

describe('DNS provider sheet', () => {
  function mountSheet(provider = null) {
    return track(mount(DnsProviderSheet, { props: { open: true, provider, kinds: KINDS }, attachTo: document.body }))
  }

  it('shows the fields of the chosen kind', async () => {
    const wrapper = mountSheet()
    await flushPromises()
    expect(inBody('input[data-credential-field="api_token"]').exists()).toBe(true)
    wrapper.findComponent({ name: 'UiRadioGroup' }).vm.$emit('update:modelValue', 'DNS_PROVIDER_KIND_WEBHOOK')
    await flushPromises()
    expect(document.body.querySelector('input[data-credential-field="api_token"]')).toBeNull()
    expect(inBody('input[data-config-field="url"]').attributes('required')).toBeDefined()
    await inBody('[data-testid="forward-dns-provider-save"]').trigger('click')
    await flushPromises()
    expect(api.createDnsProvider).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('Required')

    await inBody('input').setValue('hook-b')
    await inBody('input[data-config-field="url"]').setValue('https://hook.example.com/b')
    await inBody('input[data-credential-field="secret"]').setValue('s3cret')
    api.createDnsProvider.mockResolvedValue({ provider: { id: '3' } })
    await inBody('[data-testid="forward-dns-provider-save"]').trigger('click')
    await flushPromises()
    const [provider, credentials, options] = api.createDnsProvider.mock.calls[0]
    expect(provider).toEqual({ name: 'hook-b', kind: 'DNS_PROVIDER_KIND_WEBHOOK', config: { url: 'https://hook.example.com/b' } })
    expect(credentials).toEqual({ secret: 's3cret' })
    expect(options.idempotencyKey).toMatch(/^fwd-/)
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('keeps a stored credential shown as ******** unless replaced', async () => {
    api.updateDnsProvider.mockResolvedValue({ provider: CLOUDFLARE })
    const wrapper = mountSheet(CLOUDFLARE)
    await flushPromises()
    const token = inBody('input[data-credential-field="api_token"]')
    expect(token.element.value).toBe('********')
    expect(token.attributes('type')).toBe('password')
    expect(wrapper.findComponent({ name: 'UiRadioGroup' }).props('disabled')).toBe(true)
    await inBody('[data-testid="forward-dns-provider-save"]').trigger('click')
    await flushPromises()
    expect(api.updateDnsProvider.mock.calls[0].slice(0, 3)).toEqual(['1', { name: 'cloudflare-main', kind: 'DNS_PROVIDER_KIND_CLOUDFLARE', config: {} }, {}])

    wrapper.setProps({ open: false })
    await flushPromises()
    wrapper.setProps({ open: true })
    await flushPromises()
    await inBody('input[data-credential-field="api_token"]').setValue('new-token')
    await inBody('[data-testid="forward-dns-provider-save"]').trigger('click')
    await flushPromises()
    expect(api.updateDnsProvider.mock.calls[1][2]).toEqual({ api_token: 'new-token' })
  })

  it('shows a 403 as super administrators only and blocks saving', async () => {
    api.updateDnsProvider.mockRejectedValue(new ForwardApiError({ status: 403, code: 'super_admin_required', message: 'super administrator required' }))
    const wrapper = mountSheet(CLOUDFLARE)
    await flushPromises()
    await inBody('[data-testid="forward-dns-provider-save"]').trigger('click')
    await flushPromises()
    expect(inBody('[data-testid="forward-dns-forbidden"]').text()).toBe('Only a super administrator may add, change or delete DNS providers.')
    expect(wrapper.emitted('forbidden')).toHaveLength(1)
    expect(inBody('[data-testid="forward-dns-provider-save"]').attributes('disabled')).toBeDefined()
  })

  it('names a refused write by its violation code', async () => {
    api.createDnsProvider.mockRejectedValue(new ForwardApiError({ status: 409, code: 'refused', violations: [{ field: 'credentials', code: 'secret_store_unavailable', message: 'no kek' }] }))
    mountSheet()
    await flushPromises()
    await inBody('input').setValue('cf')
    await inBody('input[data-credential-field="api_token"]').setValue('tok')
    await inBody('[data-testid="forward-dns-provider-save"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('Control has no key-encryption key')
  })
})

describe('DNS providers page', () => {
  it('lists providers without secrets and gates writes on can_delete (D7)', async () => {
    api.listNodes.mockResolvedValue({ nodes: [], canDelete: false })
    const wrapper = track(mount(DnsProviders, { attachTo: document.body }))
    await flushPromises()
    expect(wrapper.get('[data-testid="forward-dns-readonly"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="forward-dns-add"]').attributes('disabled')).toBeDefined()
    const table = wrapper.findComponent(UiDataTable)
    const actions = table.props('rowActions')(table.props('rows')[0])
    expect(actions.find(item => item.key === 'edit').disabled).toBe(true)
    expect(actions.find(item => item.key === 'delete').label).toBe('Delete (super administrators only)')
    expect(wrapper.text()).toContain('API token')
    expect(wrapper.text()).not.toContain('********')
  })

  it('deletes an unused provider after typing its name, and not one in use', async () => {
    const confirms = answerConfirms(true)
    api.deleteDnsProvider.mockResolvedValue({ deleted: '2' })
    const wrapper = track(mount(DnsProviders, { attachTo: document.body }))
    await flushPromises()
    const table = wrapper.findComponent(UiDataTable)
    const [used, unused] = table.props('rows')
    expect(table.props('rowActions')(used).find(item => item.key === 'delete')).toMatchObject({ disabled: true, label: 'Delete (used by 2 bindings)' })
    await table.props('rowActions')(unused).find(item => item.key === 'delete').onSelect()
    await flushPromises()
    expect(confirms.last()).toMatchObject({ tone: 'danger', requireText: 'hook' })
    expect(api.deleteDnsProvider).toHaveBeenCalledWith('2', expect.objectContaining({ idempotencyKey: expect.stringMatching(/^fwd-/) }))
    expect(toastMessages('success')).toContain('Deleted hook')
  })

  it('turns a 403 on delete into the read-only state', async () => {
    const confirms = answerConfirms(true)
    api.deleteDnsProvider.mockRejectedValue(new ForwardApiError({ status: 403, code: 'super_admin_required' }))
    const wrapper = track(mount(DnsProviders, { attachTo: document.body }))
    await flushPromises()
    const table = wrapper.findComponent(UiDataTable)
    await table.props('rowActions')(table.props('rows')[1]).find(item => item.key === 'delete').onSelect()
    await flushPromises()
    expect(confirms.errors[0].message).toContain('super administrator')
    expect(wrapper.find('[data-testid="forward-dns-readonly"]').exists()).toBe(true)
  })
})

describe('entry HA card', () => {
  const DEGRADED = {
    route_id: '01J', entry_hostname: 'edge.example.net', state: 'degraded',
    binding: BINDING,
    records: [{ type: 'DNS_RECORD_TYPE_A', published: ['203.0.113.41'], desired: [] }],
    nodes: [
      { node_ref: 'forward-41', addresses: ['203.0.113.41'], healthy: false, in_rotation: true, bad_streak: 2, reason: 'offline' },
      { node_ref: 'forward-42', addresses: [], reason: 'no_address' }
    ],
    published_at_unix_ms: String(Date.now() - 600_000), evaluated_at_unix_ms: String(Date.now() - 5_000),
    last_error: 'cloudflare: 403 Forbidden', last_error_at_unix_ms: String(Date.now() - 60_000), next_attempt_at_unix_ms: String(Date.now() + 30_000)
  }

  function mountCard(props = {}) {
    return track(mount(EntryHaCard, { props: { routeId: '01J', canDelete: true, nodeName: ref => ({ 'forward-41': 'hk-edge-01' }[ref] || ref), ...props }, attachTo: document.body }))
  }

  it('renders the state, published against desired records, reasons and the last error', async () => {
    api.routeDns.mockResolvedValue(DEGRADED)
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get('[data-testid="forward-entry-ha-state"]').text()).toBe('Degraded')
    expect(wrapper.text()).toContain('No entry is healthy')
    expect(wrapper.text()).toContain('203.0.113.41')
    expect(wrapper.text()).toContain('None healthy: published kept')
    const node = wrapper.get('[data-node-ref="forward-41"]').text()
    expect(node).toContain('hk-edge-01')
    expect(node).toContain('Offline')
    expect(node).toContain('In rotation')
    expect(node).toContain('Unhealthy 2/3, leaving')
    expect(wrapper.get('[data-node-ref="forward-42"]').text()).toContain('No public address')
    expect(wrapper.get('[data-testid="forward-entry-ha-error"]').text()).toContain('cloudflare: 403 Forbidden')
    expect(wrapper.get('[data-testid="forward-entry-ha-error"]').text()).toContain('Next attempt')
  })

  it.each([
    ['ok', 'OK'], ['pending', 'Pending'], ['error', 'Error'], ['rate_limited', 'Rate limited'], ['paused', 'Paused'],
    ['route_missing', 'Route missing'], ['hostname_mismatch', 'Hostname mismatch']
  ])('labels the %s state', async (state, label) => {
    api.routeDns.mockResolvedValue({ ...DEGRADED, state, last_error: '' })
    const wrapper = mountCard()
    await flushPromises()
    expect(wrapper.get('[data-testid="forward-entry-ha-state"]').text()).toBe(label)
  })

  it('offers binding on an unbound route and the CNAME target with a copy button', async () => {
    api.routeDns.mockResolvedValueOnce({ route_id: '01J', entry_hostname: 'edge.example.net', state: 'unbound' })
    const unbound = mountCard()
    await flushPromises()
    expect(unbound.text()).toContain('edge.example.net is not bound to a DNS provider')
    expect(unbound.find('[data-testid="forward-entry-ha-unbind"]').exists()).toBe(false)

    api.routeDns.mockResolvedValueOnce({ ...DEGRADED, state: 'ok', last_error: '', cname_target: 'r1.ha.example.org', binding: { ...BINDING, mode: 'DNS_BINDING_MODE_CNAME', record_name: 'r1.ha.example.org' } })
    const cname = mountCard()
    await flushPromises()
    const box = cname.get('[data-testid="forward-entry-ha-cname"]')
    expect(box.find('input').element.value).toBe('r1.ha.example.org')
    expect(box.text()).toContain('Copy')
  })

  it('polls every 30 s', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    api.routeDns.mockResolvedValue(DEGRADED)
    mountCard()
    await vi.advanceTimersByTimeAsync(10)
    expect(api.routeDns).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(29_000)
    expect(api.routeDns).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1_000)
    expect(api.routeDns).toHaveBeenCalledTimes(2)
  })

  it('unbinds with purge, and only for a super administrator', async () => {
    api.routeDns.mockResolvedValue(DEGRADED)
    const readOnly = mountCard({ canDelete: false })
    await flushPromises()
    expect(readOnly.get('[data-testid="forward-entry-ha-unbind"]').attributes('disabled')).toBeDefined()
    readOnly.unmount()
    mounted.pop()

    api.deleteDnsBinding.mockRejectedValueOnce(new ForwardApiError({ status: 502, code: 'dns_purge_failed' })).mockResolvedValueOnce({ deleted: '4' })
    const wrapper = mountCard()
    await flushPromises()
    await wrapper.get('[data-testid="forward-entry-ha-unbind"]').trigger('click')
    await flushPromises()
    await inBody('[data-testid="forward-entry-ha-unbind-confirm"]').trigger('click')
    await flushPromises()
    expect(api.deleteDnsBinding.mock.calls[0][0]).toBe('4')
    expect(api.deleteDnsBinding.mock.calls[0][1].purge).toBe(true)
    expect(document.body.textContent).toContain('The DNS provider did not delete the records. The binding is kept.')
    await inBody('[data-testid="forward-entry-ha-purge"] button, [data-testid="forward-entry-ha-purge"]').trigger('click')
    await inBody('[data-testid="forward-entry-ha-unbind-confirm"]').trigger('click')
    await flushPromises()
    expect(api.deleteDnsBinding.mock.calls[1][1].purge).toBe(false)
    expect(toastMessages('success')).toContain('Unbound edge.example.net')
  })
})

describe('route editor binding picker (D14)', () => {
  const NODES = [
    { node_ref: 'forward-41', name: 'hk-edge-01', enabled: true, in_inventory: true, info: { engines: [{ engine: 'ENGINE_GOST', available: true, link_securities: ['LINK_SECURITY_RAW'] }] } },
    { node_ref: 'forward-42', name: 'hk-edge-02', enabled: true, in_inventory: true, info: { engines: [{ engine: 'ENGINE_GOST', available: true, link_securities: ['LINK_SECURITY_RAW'] }] } }
  ]
  const ROUTE = {
    id: '01J', owner: 'admin', revision: '3', name: 'web-edge-ha',
    listen: { port: 30080, protocol: 'L4_PROTOCOL_TCP', entry_hostname: 'edge.example.net' },
    hops: [{ role: 'HOP_ROLE_ENTRY', engine: 'ENGINE_GOST', node_refs: ['forward-41', 'forward-42'] }],
    targets: [{ host: 'origin.example.com', port: 443 }]
  }

  async function mountEditor() {
    api.listNodes.mockResolvedValue({ nodes: NODES, canDelete: true })
    api.getRoute.mockResolvedValue({ route: ROUTE })
    const wrapper = track(mount(RouteEditor, { props: { id: '01J' }, attachTo: document.body }))
    await flushPromises()
    await new Promise(resolve => setTimeout(resolve, 1100))
    await flushPromises()
    return wrapper
  }
  const component = (wrapper, type, label) => wrapper.findAllComponents(type).find(item => item.props('label') === label)

  it('binds the entry hostname after the route, without rewriting an unchanged route', async () => {
    api.createDnsBinding.mockResolvedValue({ binding: { ...BINDING } })
    const wrapper = await mountEditor()
    expect(wrapper.find('[data-testid="forward-dns-picker"]').exists()).toBe(true)
    component(wrapper, UiSwitch, 'Keep this hostname on the healthy entries').vm.$emit('update:modelValue', true)
    await flushPromises()
    // No provider yet: saving waits for one.
    expect(wrapper.get('[data-testid="forward-save"]').attributes('disabled')).toBeDefined()
    expect(component(wrapper, UiSelect, 'DNS provider').props('options').map(option => option.label)).toEqual(['cloudflare-main · Cloudflare', 'hook · Webhook'])
    component(wrapper, UiSelect, 'DNS provider').vm.$emit('update:modelValue', '1')
    await flushPromises()
    component(wrapper, UiCheckbox, 'AAAA').vm.$emit('update:modelValue', true)
    await flushPromises()
    await wrapper.get('[data-testid="forward-save"]').trigger('click')
    await flushPromises()
    expect(api.updateRoute).not.toHaveBeenCalled()
    expect(api.createDnsBinding).toHaveBeenCalledWith({
      route_id: '01J', provider_id: '1', zone: 'example.net', record_name: 'edge.example.net', mode: 'DNS_BINDING_MODE_DDNS',
      record_types: ['DNS_RECORD_TYPE_A', 'DNS_RECORD_TYPE_AAAA'], ttl: 60, paused: false
    }, expect.objectContaining({ idempotencyKey: expect.stringMatching(/^fwd-/) }))
    expect(router.push).toHaveBeenCalledWith('/admin/forward/routes/01J')
  })

  it('changes a stored binding with the whole object, its identity locked', async () => {
    api.listDnsBindings.mockResolvedValue([BINDING])
    api.updateDnsBinding.mockResolvedValue({ binding: BINDING })
    const wrapper = await mountEditor()
    expect(component(wrapper, UiSelect, 'DNS provider')).toBeUndefined()
    expect(wrapper.get('[data-testid="forward-dns-identity"]').text()).toContain('cloudflare-main · Cloudflare')
    expect(wrapper.text()).toContain('Provider, zone, name and mode cannot change')
    component(wrapper, UiSwitch, 'Paused').vm.$emit('update:modelValue', true)
    await flushPromises()
    await wrapper.get('[data-testid="forward-save"]').trigger('click')
    await flushPromises()
    expect(api.updateDnsBinding).toHaveBeenCalledWith('4', { ...BINDING, paused: true }, expect.anything())
  })

  it('keeps saving possible when a bound route’s hostname moves, with the mismatch note', async () => {
    api.listDnsBindings.mockResolvedValue([BINDING])
    const wrapper = await mountEditor()
    await wrapper.get('#fwd-f-listen-entry-hostname').setValue('hk.other.example')
    await new Promise(resolve => setTimeout(resolve, 1100))
    await flushPromises()
    expect(wrapper.get('[data-testid="forward-dns-mismatch"]').text()).toContain('edge.example.net')
    expect(wrapper.get('[data-testid="forward-save"]').attributes('disabled')).toBeUndefined()
  })

  it('shows no Required error on a freshly enabled binding, only after the field was left', async () => {
    const wrapper = await mountEditor()
    component(wrapper, UiSwitch, 'Keep this hostname on the healthy entries').vm.$emit('update:modelValue', true)
    await flushPromises()
    const picker = wrapper.get('[data-testid="forward-dns-picker"]')
    // The provider is required and empty, so Save waits: but no error yet.
    expect(wrapper.get('[data-testid="forward-save"]').attributes('disabled')).toBeDefined()
    expect(picker.find('.ui-field__error').text()).toBe('')
    expect(picker.text()).not.toContain('Required')
    // Leaving the provider field untouched shows why.
    await picker.get('#fwd-f-dns-provider-id').trigger('focusout')
    expect(picker.get('#fwd-f-dns-provider-id').element.closest('.ui-field').querySelector('.ui-field__error').textContent).toContain('Required')
  })

  it('moves the zone with the entry hostname until the user types one', async () => {
    const wrapper = await mountEditor()
    component(wrapper, UiSwitch, 'Keep this hostname on the healthy entries').vm.$emit('update:modelValue', true)
    await flushPromises()
    const zone = () => wrapper.get('#fwd-f-dns-zone').element.value
    expect(zone()).toBe('example.net')

    await wrapper.get('#fwd-f-listen-entry-hostname').setValue('edge.example.org')
    await flushPromises()
    expect(zone()).toBe('example.org')

    await wrapper.get('#fwd-f-dns-zone').setValue('custom.example.test')
    await wrapper.get('#fwd-f-listen-entry-hostname').setValue('edge.example.com')
    await flushPromises()
    expect(zone()).toBe('custom.example.test')
  })

  it('shows the CNAME target to copy in CNAME mode', async () => {
    const wrapper = await mountEditor()
    component(wrapper, UiSwitch, 'Keep this hostname on the healthy entries').vm.$emit('update:modelValue', true)
    await flushPromises()
    wrapper.findAllComponents({ name: 'UiRadioGroup' }).find(item => item.props('label') === 'Mode').vm.$emit('update:modelValue', 'DNS_BINDING_MODE_CNAME')
    await flushPromises()
    await wrapper.get('#fwd-f-dns-record-name').setValue('r1.ha.example.net')
    await flushPromises()
    const box = wrapper.get('[data-testid="forward-dns-cname"]')
    expect(box.text()).toContain('edge.example.net')
    expect(box.find('input').element.value).toBe('r1.ha.example.net')
  })

  it('reports a failed binding write after the route saved', async () => {
    api.createDnsBinding.mockRejectedValue(new ForwardApiError({ status: 409, code: 'refused', violations: [{ field: 'binding.route_id', code: 'binding_exists' }] }))
    const wrapper = await mountEditor()
    component(wrapper, UiSwitch, 'Keep this hostname on the healthy entries').vm.$emit('update:modelValue', true)
    await flushPromises()
    component(wrapper, UiSelect, 'DNS provider').vm.$emit('update:modelValue', '1')
    await flushPromises()
    await wrapper.get('[data-testid="forward-save"]').trigger('click')
    await flushPromises()
    expect(toastMessages('error')[0]).toContain('The route is saved, but its DNS binding was not')
    expect(router.push).toHaveBeenCalledWith('/admin/forward/routes/01J')
  })
})

describe('binding picker: errors wait for the user, the zone follows the hostname', () => {
  function mountPicker(props = {}) {
    const wrapper = track(mount(DnsBindingPicker, {
      attachTo: document.body,
      props: {
        modelValue: { ...model.bindingDraft(null, 'edge.example.net'), enabled: true },
        hostname: 'edge.example.net',
        errors: { provider_id: 'Required' },
        'onUpdate:modelValue': value => wrapper.setProps({ modelValue: value }),
        ...props
      },
      global: { stubs: { RouterLink: true } }
    }))
    return wrapper
  }
  const fieldError = (wrapper, id) => wrapper.get(`#${id}`).element.closest('.ui-field').querySelector('.ui-field__error').textContent.trim()
  const lastZone = wrapper => wrapper.emitted('update:modelValue').at(-1)[0].zone

  it('hides an empty field’s error until focus has left it', async () => {
    const wrapper = mountPicker()
    await flushPromises()
    expect(fieldError(wrapper, 'fwd-f-dns-provider-id')).toBe('')
    expect(wrapper.get('#fwd-f-dns-provider-id').attributes('aria-invalid')).toBeUndefined()

    await wrapper.get('#fwd-f-dns-provider-id').trigger('focusout')
    expect(fieldError(wrapper, 'fwd-f-dns-provider-id')).toBe('Required')
    expect(wrapper.get('#fwd-f-dns-provider-id').attributes('aria-invalid')).toBe('true')
  })

  it('does not count focus moving into the open provider list as leaving the field', async () => {
    const wrapper = mountPicker()
    await flushPromises()
    const list = document.createElement('div')
    list.setAttribute('role', 'listbox')
    const option = document.createElement('div')
    list.append(option)
    document.body.append(list)
    await wrapper.get('#fwd-f-dns-provider-id').trigger('focusout', { relatedTarget: option })
    expect(fieldError(wrapper, 'fwd-f-dns-provider-id')).toBe('')
    list.remove()
    // Focus leaving for the page does.
    await wrapper.get('#fwd-f-dns-provider-id').trigger('focusout', { relatedTarget: document.body })
    expect(fieldError(wrapper, 'fwd-f-dns-provider-id')).toBe('Required')
  })

  it('holds the CNAME name’s and the zone’s errors back until each field was left', async () => {
    const wrapper = mountPicker({
      modelValue: { ...model.bindingDraft(null, 'hk.customer.example'), enabled: true, provider_id: '1', mode: 'DNS_BINDING_MODE_CNAME' },
      hostname: 'hk.customer.example',
      errors: { record_name: 'Required', zone: 'The name must be inside the zone.' }
    })
    await flushPromises()
    expect(fieldError(wrapper, 'fwd-f-dns-record-name')).toBe('')
    expect(fieldError(wrapper, 'fwd-f-dns-zone')).toBe('')
    await wrapper.get('#fwd-f-dns-record-name').trigger('focusout')
    expect(fieldError(wrapper, 'fwd-f-dns-record-name')).toBe('Required')
    expect(fieldError(wrapper, 'fwd-f-dns-zone')).toBe('')
    await wrapper.get('#fwd-f-dns-zone').trigger('focusout')
    expect(fieldError(wrapper, 'fwd-f-dns-zone')).toBe('The name must be inside the zone.')
  })

  it('starts clean again when the binding is turned off and on', async () => {
    const wrapper = mountPicker()
    await flushPromises()
    await wrapper.get('#fwd-f-dns-provider-id').trigger('focusout')
    expect(fieldError(wrapper, 'fwd-f-dns-provider-id')).toBe('Required')
    wrapper.findComponent(UiSwitch).vm.$emit('update:modelValue', false)
    await flushPromises()
    wrapper.findComponent(UiSwitch).vm.$emit('update:modelValue', true)
    await flushPromises()
    expect(fieldError(wrapper, 'fwd-f-dns-provider-id')).toBe('')
  })

  it('shows the errors that come from the user’s own choices at once', async () => {
    const wrapper = mountPicker({ errors: { ttl: 'From 1 to 86400 seconds.', record_types: 'Choose at least one record type.' } })
    await flushPromises()
    expect(fieldError(wrapper, 'fwd-f-dns-ttl')).toBe('From 1 to 86400 seconds.')
    expect(wrapper.get('[data-field="dns.record_types"]').find('.ui-field__error').text()).toBe('Choose at least one record type.')
  })

  it('guesses the zone again when the hostname changes while the binding is on', async () => {
    const wrapper = mountPicker()
    await flushPromises()
    expect(wrapper.get('#fwd-f-dns-zone').element.value).toBe('example.net')
    await wrapper.setProps({ hostname: 'edge.example.org' })
    expect(lastZone(wrapper)).toBe('example.org')
    expect(wrapper.get('#fwd-f-dns-zone').element.value).toBe('example.org')

    // Typed one label at a time, the guess keeps up.
    for (const typed of ['ha', 'ha.exa', 'ha.example', 'ha.example.net', 'eu.ha.example.net']) {
      await wrapper.setProps({ hostname: typed })
    }
    expect(lastZone(wrapper)).toBe('example.net')
  })

  it('never overwrites a zone the user typed', async () => {
    const wrapper = mountPicker()
    await flushPromises()
    await wrapper.get('#fwd-f-dns-zone').setValue('custom.example.test')
    const emitted = wrapper.emitted('update:modelValue').length
    await wrapper.setProps({ hostname: 'edge.example.org' })
    expect(wrapper.emitted('update:modelValue')).toHaveLength(emitted)
    expect(wrapper.get('#fwd-f-dns-zone').element.value).toBe('custom.example.test')
  })

  it('guesses an emptied zone, and keeps the guess in step while the binding is off', async () => {
    const wrapper = mountPicker({ modelValue: { ...model.bindingDraft(null, 'edge.example.net'), enabled: false, zone: '' } })
    await flushPromises()
    await wrapper.setProps({ hostname: 'edge.example.org' })
    expect(lastZone(wrapper)).toBe('example.org')
    // Turning it on later does not bring back the first hostname’s zone.
    wrapper.findComponent(UiSwitch).vm.$emit('update:modelValue', true)
    await flushPromises()
    expect(wrapper.get('#fwd-f-dns-zone').element.value).toBe('example.org')
  })

  it('leaves a bound route’s zone alone', async () => {
    const wrapper = mountPicker({ modelValue: model.bindingDraft(BINDING), stored: BINDING, errors: {} })
    await flushPromises()
    await wrapper.setProps({ hostname: 'hk.other.example' })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
