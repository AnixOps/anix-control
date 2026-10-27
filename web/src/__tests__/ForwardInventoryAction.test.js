// 用作转发节点… on a proxy node's page: the dialog says the node and its
// subscriptions do not change, and warns about a node without an Agent.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import ForwardInventoryAction from '@/components/forward/ForwardInventoryAction.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'

const forwardApi = vi.hoisted(() => ({
  listNodes: vi.fn(),
  setNodeSettings: vi.fn(),
  newIdempotencyKey: () => 'key'
}))

vi.mock('@/api/forwardV4', () => forwardApi)
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

function mount() {
  render({
    components: { ForwardInventoryAction, UiHost },
    template: '<div><ForwardInventoryAction node-ref="proxy-50" name="KFC JP" /><UiHost /></div>'
  })
}

describe('ForwardInventoryAction', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    forwardApi.listNodes.mockReset()
  })

  it('warns that a node without an Agent cannot forward', async () => {
    forwardApi.listNodes.mockResolvedValue({ nodes: [{ node_ref: 'proxy-50', in_inventory: false, negotiated: false }] })
    mount()
    await userEvent.setup().click(await screen.findByTestId('forward-inventory-join'))
    expect(await screen.findByText(/订阅也不会变/)).toBeTruthy()
    expect(screen.getByText(/没有 Agent/)).toBeTruthy()
  })

  it('leaves the warning out for a node whose Agent forwards', async () => {
    forwardApi.listNodes.mockResolvedValue({ nodes: [{ node_ref: 'proxy-50', in_inventory: false, negotiated: true }] })
    mount()
    await userEvent.setup().click(await screen.findByTestId('forward-inventory-join'))
    expect(await screen.findByText(/订阅也不会变/)).toBeTruthy()
    expect(screen.queryByText(/没有 Agent/)).toBeNull()
  })
})
