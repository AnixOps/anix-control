// The admin shell's banner for the certificates the panel serves itself: it
// shows only while one fails (with the reason and a fix in the reader's
// language), is being requested, fails to renew or is not publicly trusted,
// and offers a retry; it stays away while every certificate is valid, when
// the panel manages none, and when the status cannot be read.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import PanelCertificateBanner from '@/components/admin/PanelCertificateBanner.vue'

const kernel = vi.hoisted(() => ({
  getPanelStatus: vi.fn(),
  retryPanelCertificates: vi.fn()
}))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernel }))

const certificate = (extra = {}) => ({
  name: 'panel.example.com', names: ['panel.example.com'], roles: ['web', 'grpc'], source: 'acme',
  state: 'valid', serving: 'acme', publicly_trusted: true, not_after: '2027-01-10T00:00:00Z',
  renewal_failing: false, last_error: '', problem: '', fix: '', ...extra
})

const status = (...certificates) => ({ certificate: { mode: 'acme', state: 'valid', domain: 'panel.example.com', certificates } })

async function mountBanner() {
  const view = render(PanelCertificateBanner)
  await flushPromises()
  return view
}

describe('PanelCertificateBanner', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.clearAllMocks()
    vi.restoreAllMocks()
  })

  it('stays away while every certificate is valid, with none, and on errors', async () => {
    kernel.getPanelStatus.mockResolvedValue(status(certificate()))
    const { unmount } = await mountBanner()
    expect(screen.queryByTestId('panel-certificate-banner')).toBeNull()
    unmount()

    kernel.getPanelStatus.mockResolvedValue({ certificate: { mode: 'external', state: 'not_configured', certificates: [] } })
    const second = await mountBanner()
    expect(screen.queryByTestId('panel-certificate-banner')).toBeNull()
    second.unmount()

    kernel.getPanelStatus.mockRejectedValue(Object.assign(new Error('forbidden'), { response: { status: 403 } }))
    await mountBanner()
    expect(screen.queryByTestId('panel-certificate-banner')).toBeNull()
  })

  it('shows a failed certificate with its reason and the fix', async () => {
    kernel.getPanelStatus.mockResolvedValue(status(
      certificate({ name: 'grpc.example.com', roles: ['grpc'] }),
      certificate({
        state: 'failed', serving: 'self_signed', publicly_trusted: false, not_after: null,
        last_error: 'the CA refused the certificate: http-01: Timeout during connect', problem: 'unreachable', fix: 'Open both ports.'
      })
    ))
    await mountBanner()
    const banner = screen.getByTestId('panel-certificate-banner')
    expect(banner.getAttribute('data-state')).toBe('failed')
    expect(banner.getAttribute('role')).toBe('alert')
    expect(banner.textContent).toContain('panel.example.com')
    expect(banner.textContent).toContain('nodes cannot join')
    expect(screen.getByTestId('panel-certificate-reason').textContent).toContain('Timeout during connect')
    // The console's own words for the problem, not the server's English.
    expect(screen.getByTestId('panel-certificate-fix').textContent).toContain('ports 443 and 80')
  })

  it('falls back to the server fix for a problem it has no words for', async () => {
    kernel.getPanelStatus.mockResolvedValue(status(certificate({ state: 'failed', problem: 'something_new', fix: 'Do the new thing.' })))
    await mountBanner()
    expect(screen.getByTestId('panel-certificate-fix').textContent).toBe('Do the new thing.')
  })

  it('says a first request is under way, without a retry', async () => {
    kernel.getPanelStatus.mockResolvedValue(status(certificate({ state: 'pending', serving: 'self_signed', not_after: null })))
    await mountBanner()
    const banner = screen.getByTestId('panel-certificate-banner')
    expect(banner.getAttribute('data-state')).toBe('pending')
    expect(banner.textContent).toContain('Requesting the certificate for panel.example.com')
    expect(screen.queryByTestId('panel-certificate-retry')).toBeNull()
  })

  it('warns of a failing renewal and of a certificate nodes do not trust', async () => {
    kernel.getPanelStatus.mockResolvedValue(status(certificate({ renewal_failing: true, last_error: 'dns: NXDOMAIN', problem: 'dns_unresolved' })))
    const first = await mountBanner()
    expect(screen.getByTestId('panel-certificate-banner').getAttribute('data-state')).toBe('renewal_failing')
    expect(screen.getByTestId('panel-certificate-fix').textContent).toContain('A (and AAAA) record for panel.example.com')
    first.unmount()

    kernel.getPanelStatus.mockResolvedValue(status(certificate({ source: 'files', serving: 'file', publicly_trusted: false, problem: 'untrusted' })))
    await mountBanner()
    expect(screen.getByTestId('panel-certificate-banner').getAttribute('data-state')).toBe('untrusted')
  })

  it('requests again and says when the hourly limit holds it back', async () => {
    const user = userEvent.setup()
    kernel.getPanelStatus.mockResolvedValue(status(certificate({ state: 'failed', problem: 'unreachable' })))
    kernel.retryPanelCertificates.mockResolvedValueOnce({ retry: { retrying: ['panel.example.com'], waiting: {}, healthy: [] } })
    await mountBanner()
    await user.click(screen.getByTestId('panel-certificate-retry'))
    await flushPromises()
    expect(kernel.retryPanelCertificates).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId('panel-certificate-retry-note').textContent).toContain('Requested again')

    kernel.retryPanelCertificates.mockResolvedValueOnce({ retry: { retrying: [], waiting: { 'panel.example.com': '2026-10-13T10:00:00Z' }, healthy: [] } })
    await user.click(screen.getByTestId('panel-certificate-retry'))
    await flushPromises()
    expect(screen.getByTestId('panel-certificate-retry-note').textContent).toContain('Three requests were made within the hour')

    kernel.retryPanelCertificates.mockRejectedValueOnce(new Error('network'))
    await user.click(screen.getByTestId('panel-certificate-retry'))
    await flushPromises()
    expect(screen.getByTestId('panel-certificate-retry-note').textContent).toContain('anix-control tls retry')
  })
})
