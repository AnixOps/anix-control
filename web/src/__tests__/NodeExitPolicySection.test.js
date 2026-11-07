// 出口策略 of a node: refusing mainland-China destinations, off by default,
// with the refusals the node's Agent reported.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeExitPolicySection from '@/views/admin/nodes/NodeExitPolicySection.vue'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({
  getKernelNodeExitPolicy: vi.fn(),
  setKernelNodeExitPolicy: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

const OFF = { node_id: 3, block_mainland: false, mainland_blocked_total: 0, mainland_blocked_at: null }

describe('NodeExitPolicySection', () => {
  beforeEach(async () => {
    await setLocale('en')
    Object.values(kernelApi).forEach(fn => fn.mockReset())
  })

  it('is off by default and turns the block on', async () => {
    kernelApi.getKernelNodeExitPolicy.mockResolvedValue(OFF)
    kernelApi.setKernelNodeExitPolicy.mockResolvedValue({ ...OFF, block_mainland: true })
    render(NodeExitPolicySection, { props: { nodeRef: 'proxy-3' } })
    const toggle = await screen.findByRole('switch')
    expect(toggle.getAttribute('aria-checked')).toBe('false')
    expect(screen.getByTestId('node-exit-policy-count').textContent).toContain('No refused connection')
    expect(screen.getByText(/Needs an Agent and kernel of 5.0.2/)).toBeTruthy()
    await userEvent.click(toggle)
    expect(kernelApi.setKernelNodeExitPolicy).toHaveBeenCalledWith('proxy-3', true)
    expect((await screen.findByRole('switch')).getAttribute('aria-checked')).toBe('true')
  })

  it('shows the refused connections and a save error', async () => {
    kernelApi.getKernelNodeExitPolicy.mockResolvedValue({ ...OFF, block_mainland: true, mainland_blocked_total: 12, mainland_blocked_at: '2026-11-07T08:00:00Z' })
    kernelApi.setKernelNodeExitPolicy.mockRejectedValue({ response: { data: { error: { message: 'database down' } } } })
    render(NodeExitPolicySection, { props: { nodeRef: 'proxy-3' } })
    expect((await screen.findByTestId('node-exit-policy-count')).textContent).toContain('12 connections refused')
    await userEvent.click(screen.getByRole('switch'))
    expect((await screen.findByRole('alert')).textContent).toBe('database down')
  })
})
