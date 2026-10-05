import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TelegramTestPanel from '@/views/admin/notifications/TelegramTestPanel.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({ testTelegramBot: vi.fn() }))
vi.mock('@/api/admin', () => adminApi)

const refusal = (status, code, headers = {}) => Object.assign(new Error(`status ${status}`), { response: { status, data: { error: { code, message: 'server text' } }, headers } })

describe('Telegram test message button', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.useFakeTimers()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  const send = async wrapper => {
    await wrapper.get('[data-testid="telegram-test-send"]').trigger('click')
    await flushPromises()
  }

  it('sends an empty body (the caller’s own chat) and shows the server’s sentence in the class tone', async () => {
    adminApi.testTelegramBot.mockResolvedValue({ class: 'ok', ok: true, message: 'Telegram accepted the test message.', target: 'self' })
    const wrapper = mount(TelegramTestPanel)
    expect(wrapper.find('[data-testid="telegram-test-result"]').exists()).toBe(false)
    await send(wrapper)
    expect(adminApi.testTelegramBot).toHaveBeenCalledWith()
    const result = wrapper.get('[data-testid="telegram-test-result"]')
    expect(result.attributes('data-tone')).toBe('success')
    expect(result.attributes('role')).toBe('status')
    expect(result.text()).toContain('Delivered')
    expect(result.text()).toContain('Telegram accepted the test message.')
    wrapper.unmount()
  })

  it.each([
    ['chat_not_found', 'warning', 'Chat not found', 'Check that your Telegram account is still linked'],
    ['bot_blocked', 'warning', 'Bot blocked', 'press Start'],
    ['rate_limited', 'warning', 'Telegram is limiting the bot', 'Wait a moment'],
    ['not_configured', 'warning', 'Bot not set up', 'Enter the bot token'],
    ['invalid_token', 'danger', 'Invalid bot token', 'BotFather'],
    ['unknown', 'danger', 'Unexpected answer', 'Control logs'],
    ['brand_new_class', 'danger', 'Unexpected answer', 'Control logs']
  ])('class %s is %s', async (cls, tone, label, hint) => {
    adminApi.testTelegramBot.mockResolvedValue({ class: cls, ok: false, message: 'Fixed sentence.' })
    const wrapper = mount(TelegramTestPanel)
    await send(wrapper)
    const result = wrapper.get('[data-testid="telegram-test-result"]')
    expect(result.attributes('data-tone')).toBe(tone)
    expect(result.text()).toContain(label)
    expect(result.text()).toContain('Fixed sentence.')
    expect(result.text()).toContain(hint)
    wrapper.unmount()
  })

  it('explains a network error by its reason', async () => {
    adminApi.testTelegramBot.mockResolvedValue({ class: 'network_error', ok: false, reason: 'dns', message: 'Telegram could not be reached from this server.' })
    const wrapper = mount(TelegramTestPanel)
    await send(wrapper)
    const result = wrapper.get('[data-testid="telegram-test-result"]')
    expect(result.attributes('data-tone')).toBe('danger')
    expect(result.text()).toContain('Telegram not reachable')
    expect(result.text()).toContain('api.telegram.org did not resolve')
    wrapper.unmount()
  })

  it('tells the administrator to link Telegram first on 409', async () => {
    adminApi.testTelegramBot.mockRejectedValue(refusal(409, 'telegram_not_bound'))
    const wrapper = mount(TelegramTestPanel)
    await send(wrapper)
    const result = wrapper.get('[data-testid="telegram-test-result"]')
    expect(result.attributes('data-tone')).toBe('warning')
    expect(result.text()).toContain('Telegram not linked')
    expect(result.text()).toContain('/bind')
    // Not rate limited: the button stays usable.
    expect(wrapper.get('[data-testid="telegram-test-send"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('disables the button while the request runs', async () => {
    let resolve
    adminApi.testTelegramBot.mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mount(TelegramTestPanel)
    await wrapper.get('[data-testid="telegram-test-send"]').trigger('click')
    const button = wrapper.get('[data-testid="telegram-test-send"]')
    expect(button.attributes('aria-busy')).toBe('true')
    await button.trigger('click')
    expect(adminApi.testTelegramBot).toHaveBeenCalledTimes(1)
    resolve({ class: 'ok', ok: true, message: 'Telegram accepted the test message.' })
    await flushPromises()
    expect(wrapper.get('[data-testid="telegram-test-send"]').attributes('aria-busy')).toBeUndefined()
    wrapper.unmount()
  })

  it('honours Retry-After on 429: disabled, counts down, then enabled again', async () => {
    adminApi.testTelegramBot.mockRejectedValue(refusal(429, 'rate_limited', { 'retry-after': '3' }))
    const wrapper = mount(TelegramTestPanel)
    await send(wrapper)
    expect(wrapper.get('[data-testid="telegram-test-result"]').text()).toContain('Too many tests')
    expect(wrapper.get('[data-testid="telegram-test-send"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="telegram-test-wait"]').text()).toBe('Try again in 3 s')
    await wrapper.get('[data-testid="telegram-test-send"]').trigger('click')
    expect(adminApi.testTelegramBot).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.get('[data-testid="telegram-test-wait"]').text()).toBe('Try again in 2 s')
    await vi.advanceTimersByTimeAsync(2000)
    expect(wrapper.find('[data-testid="telegram-test-wait"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="telegram-test-send"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('shows a failed request’s server message, and never the request', async () => {
    adminApi.testTelegramBot.mockRejectedValue(refusal(502, 'bad_gateway'))
    const wrapper = mount(TelegramTestPanel)
    await send(wrapper)
    const result = wrapper.get('[data-testid="telegram-test-result"]')
    expect(result.attributes('data-tone')).toBe('danger')
    expect(result.text()).toContain('server text')
    wrapper.unmount()
  })

  it('warns that the test uses the saved settings when the form has unsaved changes', async () => {
    const wrapper = mount(TelegramTestPanel, { props: { dirty: true } })
    expect(wrapper.get('[data-testid="telegram-test-dirty"]').text()).toContain('saved bot settings')
    wrapper.unmount()
  })

  it('speaks Chinese too', async () => {
    await setLocale('zh-CN')
    adminApi.testTelegramBot.mockResolvedValue({ class: 'bot_blocked', ok: false, message: 'Fixed.' })
    const wrapper = mount(TelegramTestPanel)
    expect(wrapper.text()).toContain('发送测试消息')
    await send(wrapper)
    expect(wrapper.get('[data-testid="telegram-test-result"]').text()).toContain('机器人被屏蔽')
    wrapper.unmount()
    await setLocale('en')
  })
})
