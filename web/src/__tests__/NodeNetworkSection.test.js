// 网络 of a node: the egress interface, source address, gateway and hop
// port that the automatic routes through the node use.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeNetworkSection from '@/views/admin/nodes/NodeNetworkSection.vue'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({
  getKernelNodeNetwork: vi.fn(),
  setKernelNodeNetwork: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

const EMPTY = { interface: '', source_v4: '', gateway_v4: '', source_v6: '', gateway_v6: '', hop_port: 0 }

describe('NodeNetworkSection', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    Object.values(kernelApi).forEach(fn => fn.mockReset())
  })

  it('saves the interface, source and gateway', async () => {
    kernelApi.getKernelNodeNetwork.mockResolvedValue(EMPTY)
    kernelApi.setKernelNodeNetwork.mockImplementation(async (_, settings) => settings)
    render(NodeNetworkSection, { props: { nodeRef: 'proxy-864' } })
    const user = userEvent.setup()
    await user.type(await screen.findByLabelText('出口网卡'), 'eth1')
    await user.type(screen.getByLabelText('IPv4 源地址'), '10.0.0.7')
    await user.type(screen.getByLabelText('IPv4 网关'), '10.0.0.1')
    await user.click(screen.getByTestId('node-network-save'))
    expect(kernelApi.setKernelNodeNetwork).toHaveBeenCalledWith('proxy-864', {
      ...EMPTY, interface: 'eth1', source_v4: '10.0.0.7', gateway_v4: '10.0.0.1'
    })
    expect(await screen.findByText('已保存，经过这个节点的线路已重新规划。')).toBeTruthy()
  })

  it('shows the violations by field', async () => {
    kernelApi.getKernelNodeNetwork.mockResolvedValue(EMPTY)
    kernelApi.setKernelNodeNetwork.mockRejectedValue({
      response: { data: { error: { code: 'invalid_request', violations: [{ field: 'interface', message: 'a source address leaves by an interface: name it' }] } } }
    })
    render(NodeNetworkSection, { props: { nodeRef: 'proxy-864' } })
    const user = userEvent.setup()
    await user.type(await screen.findByLabelText('IPv4 源地址'), '10.0.0.7')
    await user.click(screen.getByTestId('node-network-save'))
    expect(await screen.findByText('a source address leaves by an interface: name it')).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toContain('有字段需要修改')
  })

  it('offers the interfaces the Agent detected', async () => {
    kernelApi.getKernelNodeNetwork.mockResolvedValue({
      ...EMPTY,
      detected: [
        { name: 'eth0', addresses_v4: ['203.0.113.7'], addresses_v6: [], gateway_v4: '203.0.113.1', gateway_v6: '' },
        { name: 'eth1', addresses_v4: ['10.0.0.5'], addresses_v6: ['2001:db8::5'], gateway_v4: '10.0.0.1', gateway_v6: '' }
      ],
      detected_at: '2026-10-30T18:00:00Z'
    })
    kernelApi.setKernelNodeNetwork.mockImplementation(async (_, settings) => settings)
    render(NodeNetworkSection, { props: { nodeRef: 'proxy-864' } })
    const user = userEvent.setup()
    await user.click(await screen.findByRole('combobox', { name: '出口网卡' }))
    await user.click(await screen.findByRole('option', { name: /eth1/ }))
    expect(screen.getByRole('combobox', { name: 'IPv4 源地址' }).textContent).toContain('10.0.0.5')
    expect(screen.getByRole('combobox', { name: 'IPv4 网关' }).textContent).toContain('10.0.0.1')
    await user.click(screen.getByTestId('node-network-save'))
    expect(kernelApi.setKernelNodeNetwork).toHaveBeenCalledWith('proxy-864', {
      ...EMPTY, interface: 'eth1', source_v4: '10.0.0.5', gateway_v4: '10.0.0.1', source_v6: '2001:db8::5'
    })
  })

  it('switches to typing the settings', async () => {
    kernelApi.getKernelNodeNetwork.mockResolvedValue({
      ...EMPTY, detected: [{ name: 'eth0', addresses_v4: ['203.0.113.7'], addresses_v6: [], gateway_v4: '', gateway_v6: '' }]
    })
    render(NodeNetworkSection, { props: { nodeRef: 'proxy-864' } })
    const user = userEvent.setup()
    await user.click(await screen.findByTestId('node-network-manual'))
    await user.type(screen.getByLabelText('出口网卡'), 'ens19')
    expect(screen.getByTestId('node-network-pick')).toBeTruthy()
  })
})
