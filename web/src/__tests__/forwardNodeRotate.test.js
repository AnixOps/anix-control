// 轮换 Agent 凭据 on the forwarding node page (views/admin/forward/NodeDetail.vue):
// for a forwarding node with an Agent, hidden when the forward API says the
// administrator is not a super administrator (can_delete false), and not
// offered for a proxy node (its page is the proxy node page) or an Ansible
// machine (no Agent).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { setLocale } from '@/i18n'
import { inBody } from './helpers/feedback'

const api = vi.hoisted(() => ({ getNode: vi.fn(), listRoutes: vi.fn() }))
const kernelApi = vi.hoisted(() => ({ rotateKernelAgentCredentials: vi.fn(), createKernelAgentInstallToken: vi.fn() }))
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

const CREDENTIAL = 'anixagt_ForwardNodeCredentialNeverRealAbCd0123456789'

function view(fields = {}) {
  return {
    node: {
      node_ref: 'forward-41', name: 'hk-edge-01', kind: 'forward', enabled: true, in_inventory: true, host: '203.0.113.41',
      desired_generation: '3', reported: true, reported_generation: '3', record: { transport: 'NODE_TRANSPORT_AGENT' }, capabilities: {}, info: {},
      ...fields
    },
    state: { hops: [] },
    report: {}
  }
}

const mounted = []

async function mountPage(nodeView = view(), { canDelete = true } = {}) {
  api.getNode.mockResolvedValue(nodeView)
  api.listRoutes.mockResolvedValue({ routes: [], canDelete, truncated: false })
  const wrapper = mount(NodeDetail, {
    attachTo: document.body,
    props: { nodeRef: nodeView.node.node_ref },
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
  })
  mounted.push(wrapper)
  await flushPromises()
  return wrapper
}

describe('forwarding node page: rotate credentials', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    while (mounted.length) mounted.pop().unmount()
    vi.restoreAllMocks()
  })

  it('rotates a forwarding node\'s credentials by its node ref, with no API key option', async () => {
    kernelApi.rotateKernelAgentCredentials.mockResolvedValue({
      node: 'forward-41', revoked: { certificates: 1, enrollments: 1, link_certificates: 1 }, api_key_rotated: false,
      expires_at: new Date(Date.now() + 3_600_000).toISOString(), credential: CREDENTIAL
    })
    const wrapper = await mountPage()
    const section = wrapper.get('[data-testid="forward-node-rotate"]')
    expect(section.text()).toContain('Revoke what this node’s Agent holds')
    await section.get('[data-testid="rotate-credentials"]').trigger('click')
    await flushPromises()
    expect(inBody('[role="alertdialog"]').text()).toContain('Rotate the credentials of hk-edge-01?')
    expect(document.body.querySelector('[data-testid="rotate-api-key"]')).toBeNull()

    await inBody('[data-confirm-ok]').trigger('click')
    await flushPromises()
    expect(kernelApi.rotateKernelAgentCredentials).toHaveBeenCalledWith({ node: 'forward-41', rotateApiKey: false, ttlSeconds: 3600, reason: '' })
    expect(document.body.querySelector('[data-testid="rotate-credential"] input, input[value^="anixagt_"]').value).toBe(CREDENTIAL)
    // The page behind carries nothing of it.
    expect(wrapper.html()).not.toContain(CREDENTIAL)
    await inBody('[data-testid="rotate-done"]').trigger('click')
    await flushPromises()
    expect(document.body.innerHTML).not.toContain(CREDENTIAL)
  })

  it('is not offered when the forward API says the administrator is not a super administrator', async () => {
    const wrapper = await mountPage(view(), { canDelete: false })
    expect(wrapper.find('[data-testid="forward-node-rotate"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="rotate-credentials"]').exists()).toBe(false)
    // The install command is still there.
    expect(wrapper.find('[data-testid="forward-node-install"]').exists()).toBe(true)
  })

  it('is not offered for a proxy node or an Ansible machine', async () => {
    let wrapper = await mountPage(view({ node_ref: 'proxy-12', kind: 'proxy', name: 'hk-01' }))
    expect(wrapper.find('[data-testid="rotate-credentials"]').exists()).toBe(false)
    wrapper = await mountPage(view({ node_ref: 'forward-9', record: { transport: 'NODE_TRANSPORT_ANSIBLE' } }))
    expect(wrapper.find('[data-testid="rotate-credentials"]').exists()).toBe(false)
  })

  it('is a disabled button, with the reason, for a node that is disabled', async () => {
    const wrapper = await mountPage(view({ enabled: false }))
    expect(wrapper.get('[data-testid="rotate-credentials"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="forward-node-rotate"]').text()).toContain('This node is disabled. Enable it first')
  })
})
