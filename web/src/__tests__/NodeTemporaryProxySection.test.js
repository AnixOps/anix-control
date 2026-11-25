// 临时 SOCKS5 代理 of a node: open one on an internal address, see its
// password once, and close it before its end.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeTemporaryProxySection from '@/views/admin/nodes/NodeTemporaryProxySection.vue'
import { setLocale } from '@/i18n'

const kernelApi = vi.hoisted(() => ({
  listKernelNodeTemporaryProxies: vi.fn(),
  openKernelNodeTemporaryProxy: vi.fn(),
  closeKernelNodeTemporaryProxy: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

const PROXY = { id: 4, listen: '10.126.126.1', port: 1080, username: 'tmp-abcdEFGH', expires_at: '2026-11-25T08:15:00Z', created_by: 7, created_at: '2026-11-25T08:00:00Z' }

describe('NodeTemporaryProxySection', () => {
  beforeEach(async () => {
    await setLocale('en')
    Object.values(kernelApi).forEach(fn => fn.mockReset())
  })

  it('opens a proxy and shows its credentials once', async () => {
    kernelApi.listKernelNodeTemporaryProxies.mockResolvedValueOnce([]).mockResolvedValueOnce([PROXY])
    kernelApi.openKernelNodeTemporaryProxy.mockResolvedValue({ ...PROXY, password: 'p4ssw0rdp4ssw0rdp4ssw0rd' })
    render(NodeTemporaryProxySection, { props: { nodeRef: 'proxy-3' } })
    expect((await screen.findByTestId('node-temporary-proxies-empty')).textContent).toContain('No open temporary proxy')
    const openButton = screen.getByTestId('node-temporary-proxies-open')
    expect(openButton.disabled).toBe(true)

    await userEvent.type(screen.getByLabelText('Listen address'), '10.126.126.1')
    await userEvent.click(openButton)
    expect(kernelApi.openKernelNodeTemporaryProxy).toHaveBeenCalledWith('proxy-3', { listen: '10.126.126.1', port: 1080, minutes: 15 })
    const shown = await screen.findByTestId('node-temporary-proxies-opened')
    expect(shown.textContent).toContain('The password is shown only once.')
    expect(screen.getByDisplayValue('10.126.126.1:1080')).toBeTruthy()
    expect(screen.getByDisplayValue('tmp-abcdEFGH')).toBeTruthy()
    expect(screen.getByDisplayValue('p4ssw0rdp4ssw0rdp4ssw0rd')).toBeTruthy()
    expect((await screen.findByTestId('node-temporary-proxies-list')).textContent).toContain('10.126.126.1:1080')
  })

  it('shows the refusal of an open and closes a proxy', async () => {
    kernelApi.listKernelNodeTemporaryProxies.mockResolvedValue([PROXY])
    kernelApi.openKernelNodeTemporaryProxy.mockRejectedValue({ response: { data: { error: { message: 'the node already uses this port' } } } })
    kernelApi.closeKernelNodeTemporaryProxy.mockResolvedValue({ ...PROXY, closed_at: '2026-11-25T08:05:00Z' })
    render(NodeTemporaryProxySection, { props: { nodeRef: 'proxy-3' } })
    expect((await screen.findByTestId('node-temporary-proxies-list')).textContent).toContain('tmp-abcdEFGH')

    await userEvent.type(screen.getByLabelText('Listen address'), 'fd00::1')
    await userEvent.click(screen.getByTestId('node-temporary-proxies-open'))
    expect((await screen.findByRole('alert')).textContent).toBe('the node already uses this port')

    await userEvent.click(screen.getByTestId('node-temporary-proxies-close-4'))
    expect(kernelApi.closeKernelNodeTemporaryProxy).toHaveBeenCalledWith('proxy-3', 4)
    expect(await screen.findByTestId('node-temporary-proxies-empty')).toBeTruthy()
  })
})
