// 转发端口 of a node: the port range the forward planner allocates from,
// editable on the node page instead of only the built-in range.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeForwardPortsSection from '@/views/admin/nodes/NodeForwardPortsSection.vue'
import { ForwardApiError } from '@/api/forwardV4'
import { setLocale } from '@/i18n'

const forwardApi = vi.hoisted(() => ({
  getNode: vi.fn(),
  setNodeSettings: vi.fn()
}))

vi.mock('@/api/forwardV4', async importOriginal => ({ ...(await importOriginal()), ...forwardApi }))

const builtIn = { node_ref: 'proxy-62', kind: 'proxy', info: { port_range: { first: 30000, last: 39999 } } }

describe('NodeForwardPortsSection', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    Object.values(forwardApi).forEach(fn => fn.mockReset())
  })

  it('shows the range in force as values and saves a new one as the node\'s own', async () => {
    forwardApi.getNode.mockResolvedValue({ node: { ...builtIn, settings: { addresses: ['203.0.113.7'], labels: { link: 'iepl' } } } })
    forwardApi.setNodeSettings.mockImplementation(async (_, settings) => ({ node: { ...builtIn, settings } }))
    render(NodeForwardPortsSection, { props: { nodeRef: 'proxy-62' } })
    const first = await screen.findByLabelText('端口范围起')
    expect(first.value).toBe('30000')
    expect(screen.getByLabelText('端口范围止').value).toBe('39999')
    expect(screen.getByTestId('node-forward-ports-source').textContent).toContain('内置的 30000–39999')

    const user = userEvent.setup()
    await user.clear(first)
    await user.type(first, '40000')
    const last = screen.getByLabelText('端口范围止')
    await user.clear(last)
    await user.type(last, '40999')
    await user.click(screen.getByTestId('node-forward-ports-save'))
    expect(forwardApi.setNodeSettings).toHaveBeenCalledWith('proxy-62', {
      port_range: { first: 40000, last: 40999 },
      reserved_ports: [],
      addresses: ['203.0.113.7'],
      labels: { link: 'iepl' }
    }, expect.anything())
    expect(await screen.findByText('已保存，经过这个节点的线路已重新规划。')).toBeTruthy()
    expect(screen.getByTestId('node-forward-ports-source').textContent).toContain('这个节点自己设置的范围')
  })

  it('refuses a range with one end', async () => {
    forwardApi.getNode.mockResolvedValue({ node: { node_ref: 'proxy-62', kind: 'proxy' } })
    render(NodeForwardPortsSection, { props: { nodeRef: 'proxy-62' } })
    const user = userEvent.setup()
    await user.type(await screen.findByLabelText('端口范围起'), '40000')
    await user.click(screen.getByTestId('node-forward-ports-save'))
    expect(forwardApi.setNodeSettings).not.toHaveBeenCalled()
    expect(screen.getByText('起止端口都要填写')).toBeTruthy()
  })

  it('hides itself when Control has no forwarding', async () => {
    forwardApi.getNode.mockRejectedValue(new ForwardApiError({ status: 404, code: 'not_found' }))
    render(NodeForwardPortsSection, { props: { nodeRef: 'proxy-62' } })
    await vi.waitFor(() => expect(forwardApi.getNode).toHaveBeenCalled())
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(screen.queryByTestId('node-forward-ports')).toBeNull()
  })
})
