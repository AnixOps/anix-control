import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AgentInstallSheet from '@/components/admin/AgentInstallSheet.vue'
import { setLocale } from '@/i18n'
import { inBody } from './helpers/feedback'

const kernelApi = vi.hoisted(() => ({ createKernelAgentInstallToken: vi.fn() }))
vi.mock('@/api/kernel', () => kernelApi)

const TOKEN = 'anixagt_TESTTOKENTESTTOKENTESTTOKEN'
const base = `curl -fsSL https://ctl.example.com/install.sh | sudo bash -s -- --control https://ctl.example.com --node forward-41 --token ${TOKEN}`

function issued(overrides = {}) {
  return {
    credential: TOKEN,
    node: 'forward-41',
    expires_at: '2026-10-03T13:00:00Z',
    agent_version: 'v4.2.0',
    commands: [
      { mirror: 'control', command: base, available: true },
      { mirror: 'cn', command: `${base} --mirror cn`, available: false, note: 'agent_install.cn_mirror_url is not set' },
      { mirror: 'github', command: `${base} --mirror github`, available: true }
    ],
    script: { url: 'https://ctl.example.com/install.sh', signature_url: 'https://ctl.example.com/install.sh.sig', signed: true },
    ...overrides
  }
}

const mounted = []

async function mountSheet(props = {}) {
  const wrapper = mount(AgentInstallSheet, {
    attachTo: document.body,
    props: { open: true, node: 'forward-41', nodeLabel: 'relay-hk', ...props }
  })
  mounted.push(wrapper)
  await flushPromises()
  return wrapper
}

describe('Agent install sheet', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    await setLocale('en')
  })

  afterEach(() => {
    while (mounted.length) mounted.pop().unmount()
  })

  it('issues a one-hour token and shows the command per mirror', async () => {
    kernelApi.createKernelAgentInstallToken.mockResolvedValue(issued())
    await mountSheet()
    expect(inBody('[data-testid="agent-install-sheet"]').text()).toContain('relay-hk')
    expect(inBody('[data-testid="agent-install-result"]').exists()).toBe(false)

    await inBody('[data-testid="agent-install-generate"]').trigger('click')
    await flushPromises()

    expect(kernelApi.createKernelAgentInstallToken).toHaveBeenCalledWith('forward-41', 3600)
    const result = inBody('[data-testid="agent-install-result"]')
    expect(result.text()).toContain('shown only here')
    expect(result.text()).toContain('verify it with /install.sh.sig')
    expect(result.text()).toContain('inet v2b_forward')
    expect(inBody('[data-testid="agent-install-command"]').text()).toContain(base)
    expect(inBody('[data-testid="agent-install-command"]').text()).not.toContain('--mirror')
    expect(inBody('[data-testid="agent-install-fallback"]').exists()).toBe(false)

    const mirrors = [...document.body.querySelectorAll('[data-testid="agent-install-mirror"] button, [data-testid="agent-install-mirror"] [role="radio"]')]
    const cn = mirrors.find(element => element.textContent.includes('Mainland mirror'))
    expect(cn).toBeTruthy()
    cn.click()
    await flushPromises()
    expect(inBody('[data-testid="agent-install-command"]').text()).toContain('--mirror cn')
    expect(inBody('[data-testid="agent-install-fallback"]').text()).toContain('cn_mirror_url')
  })

  it('shows the server error and no command', async () => {
    kernelApi.createKernelAgentInstallToken.mockRejectedValue({
      response: { data: { error: { code: 'super_admin_required', message: 'only a super administrator may issue install tokens' } } }
    })
    await mountSheet()
    await inBody('[data-testid="agent-install-generate"]').trigger('click')
    await flushPromises()
    expect(inBody('[data-testid="agent-install-error"]').text()).toContain('only a super administrator')
    expect(inBody('[data-testid="agent-install-result"]').exists()).toBe(false)
  })

  it('drops the token when the sheet closes', async () => {
    kernelApi.createKernelAgentInstallToken.mockResolvedValue(issued({ script: { signed: false } }))
    const wrapper = await mountSheet()
    await inBody('[data-testid="agent-install-generate"]').trigger('click')
    await flushPromises()
    expect(inBody('[data-testid="agent-install-result"]').text()).toContain('no release signature')

    const close = [...document.body.querySelectorAll('.ui-sheet__footer button')].find(button => button.textContent.includes('Close'))
    close.click()
    await flushPromises()
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(document.body.textContent).not.toContain(TOKEN)
    expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false])
  })
})
