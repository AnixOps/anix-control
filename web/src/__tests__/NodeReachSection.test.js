// 可达性探测 of a node: whether it is a vantage, and what the vantages found.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeReachSection from '@/views/admin/nodes/NodeReachSection.vue'
import NodeReachCell from '@/views/admin/nodes/NodeReachCell.vue'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({
  getKernelReach: vi.fn(),
  setKernelReachVantages: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

const ANSWER = {
  vantages: [{ node_id: 1, region: 'domestic', capable: true }],
  nodes: [{ node_id: 4, status: 'region_unreachable', domestic: { reachable: false, vantages: 1 }, overseas: { reachable: true, vantages: 1 } }]
}

describe('NodeReachSection', () => {
  beforeEach(async () => {
    await setLocale('en')
    Object.values(kernelApi).forEach(fn => fn.mockReset())
  })

  it('shows the status and makes the node a vantage, keeping the others', async () => {
    kernelApi.getKernelReach.mockResolvedValue(ANSWER)
    kernelApi.setKernelReachVantages.mockResolvedValue({ ...ANSWER, vantages: [...ANSWER.vantages, { node_id: 4, region: 'overseas', capable: false }] })
    render(NodeReachSection, { props: { nodeId: 4 } })
    expect((await screen.findByTestId('node-reach-views')).textContent).toContain('Domestic unreachable')
    await userEvent.click(screen.getByRole('radio', { name: 'Overseas vantage' }))
    expect(kernelApi.setKernelReachVantages).toHaveBeenCalledWith([{ node_id: 1, region: 'domestic' }, { node_id: 4, region: 'overseas' }])
    expect(await screen.findByText(/Probing starts once the Agent is connected/)).toBeTruthy()
  })

  it('shows a save error', async () => {
    kernelApi.getKernelReach.mockResolvedValue(ANSWER)
    kernelApi.setKernelReachVantages.mockRejectedValue({ response: { data: { error: { message: 'a listed node does not exist' } } } })
    render(NodeReachSection, { props: { nodeId: 1 } })
    await screen.findByTestId('node-reach-views')
    await userEvent.click(screen.getByRole('radio', { name: 'Not a vantage' }))
    expect(kernelApi.setKernelReachVantages).toHaveBeenCalledWith([])
    expect((await screen.findByRole('alert')).textContent).toBe('a listed node does not exist')
  })
})

describe('NodeReachCell', () => {
  beforeEach(async () => {
    await setLocale('en')
  })

  it('names the status in words and marks a vantage', () => {
    render(NodeReachCell, { props: { entry: ANSWER.nodes[0], vantage: { node_id: 4, region: 'domestic' } } })
    const badge = screen.getByTestId('node-reach')
    expect(badge.textContent).toContain('Domestic unreachable')
    expect(badge.getAttribute('title')).toBe('Domestic: unreachable · Overseas: reachable')
    expect(screen.getByTestId('node-reach-vantage').textContent).toBe('Domestic vantage')
  })

  it('is a dash without an answer', () => {
    render(NodeReachCell, { props: {} })
    expect(screen.getByTestId('node-reach-empty')).toBeTruthy()
  })
})
