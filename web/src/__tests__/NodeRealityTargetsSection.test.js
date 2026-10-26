// 目标站 of a node (Reality target probing): the current targets, a probe
// run with the ranked results, applying a target after confirmation, and
// turning automatic selection on.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import NodeRealityTargetsSection from '@/views/admin/nodes/NodeRealityTargetsSection.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'
import { resetConfirms } from '@/ui/composables/useConfirm'

const kernelApi = vi.hoisted(() => ({
  getKernelRealityTargets: vi.fn(),
  probeKernelRealityTargets: vi.fn(),
  applyKernelRealityTarget: vi.fn(),
  setKernelRealityTargetAuto: vi.fn()
}))

vi.mock('@/api/kernel', () => kernelApi)

const CURRENT = [{ protocol_id: 41, name: 'DE Reality', server_name: 'www.microsoft.com', dest: 'www.microsoft.com:443' }]
const NEAR = {
  server_name: 'near.example.com', port: 443, verdict: 'ok', score: 70, used_by: 0, probed_at: '2026-10-26T17:00:00Z',
  result: { handshake_ms: 4, curve: 'X25519MLKEM768' }
}
const BAD = { server_name: 'bad.example.com', port: 443, verdict: 'failed', score: 0, used_by: 0, result: { failures: ['h2', 'redirect'] } }

function listing(report = {}, support = { connected: true, capable: true, agent_version: '5.0.0' }) {
  return { report: { node_id: 57, node_address: '82.139.194.175', current: CURRENT, recommended: null, targets: [], ...report }, support, default_candidates: ['a.example.com', 'b.example.com'] }
}

function renderSection() {
  return render({
    components: { NodeRealityTargetsSection, UiHost },
    template: '<div><NodeRealityTargetsSection node-ref="proxy-57" name="DE" /><UiHost /></div>'
  })
}

describe('NodeRealityTargetsSection', () => {
  beforeEach(async () => {
    await setLocale('zh-CN')
    Object.values(kernelApi).forEach(fn => fn.mockReset())
  })

  afterEach(() => {
    resetConfirms()
  })

  it('probes the candidates and ranks the answers', async () => {
    kernelApi.getKernelRealityTargets.mockResolvedValue(listing())
    kernelApi.probeKernelRealityTargets.mockResolvedValue(listing({ recommended: NEAR, targets: [NEAR, BAD] }))
    renderSection()
    expect((await screen.findByTestId('reality-current')).textContent).toContain('www.microsoft.com')
    expect(screen.getByTestId('reality-no-results')).toBeTruthy()

    const user = userEvent.setup()
    await user.type(screen.getByLabelText('自定义候选'), 'near.example.com, bad.example.com')
    await user.click(screen.getByTestId('reality-probe'))
    expect(kernelApi.probeKernelRealityTargets).toHaveBeenCalledWith('proxy-57', { candidates: ['near.example.com', 'bad.example.com'], includeDefaults: true })
    expect((await screen.findByTestId('reality-recommended')).textContent).toContain('near.example.com')
    expect((screen.getByTestId('reality-target-bad.example.com')).textContent).toContain('不支持 h2')
    expect((screen.getByTestId('reality-target-bad.example.com')).textContent).toContain('跳转到其他域名')
    expect(screen.queryByTestId('reality-apply-41-bad.example.com')).toBeNull()
  })

  it('says when the Agent cannot probe', async () => {
    kernelApi.getKernelRealityTargets.mockResolvedValue(listing({}, { connected: true, capable: false, agent_version: '5.0.0-rc.6' }))
    renderSection()
    expect((await screen.findByTestId('reality-unsupported')).textContent).toContain('5.0.0-rc.6')
    expect(screen.queryByTestId('reality-probe')).toBeNull()
  })

  it('applies a target after confirmation', async () => {
    kernelApi.getKernelRealityTargets.mockResolvedValue(listing({ recommended: NEAR, targets: [NEAR] }))
    kernelApi.applyKernelRealityTarget.mockResolvedValue({ previous: CURRENT[0], applied: { ...CURRENT[0], server_name: 'near.example.com' } })
    renderSection()
    const user = userEvent.setup()
    await user.click(await screen.findByTestId('reality-apply-41-near.example.com'))
    const dialog = await screen.findByRole('alertdialog')
    expect((dialog).textContent).toContain('客户端需要更新订阅')
    await user.click(within(dialog).getByRole('button', { name: '应用' }))
    await waitFor(() => expect(kernelApi.applyKernelRealityTarget).toHaveBeenCalledWith('proxy-57', { protocolId: 41, serverName: 'near.example.com', port: 443 }))
    expect((await screen.findByTestId('reality-applied')).textContent).toContain('near.example.com')
  })

  it('turns automatic selection on after confirmation', async () => {
    kernelApi.getKernelRealityTargets.mockResolvedValue({ ...listing(), auto: { enabled: false, fail_streak: 0 } })
    kernelApi.setKernelRealityTargetAuto.mockResolvedValue({ enabled: true, fail_streak: 0 })
    renderSection()
    const user = userEvent.setup()
    const toggle = within(await screen.findByTestId('reality-auto')).getByRole('switch')
    expect(toggle.getAttribute('aria-checked')).toBe('false')
    await user.click(toggle)
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog.textContent).toContain('更新订阅')
    await user.click(within(dialog).getByRole('button', { name: '开启' }))
    await waitFor(() => expect(kernelApi.setKernelRealityTargetAuto).toHaveBeenCalledWith('proxy-57', true))
    expect((await screen.findByTestId('reality-auto')).textContent).toContain('几分钟内')
  })
})
