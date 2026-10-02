import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Telegram from '@/views/admin/Telegram.vue'
import { setLocale } from '@/i18n'
import { answerConfirms, toastMessages, toasts } from './helpers/feedback'
import { runAction } from '@/ui/composables/useToast'

const adminApi = vi.hoisted(() => ({
  broadcastTelegram: vi.fn(),
  deleteTelegramWebhook: vi.fn(),
  getTelegramBot: vi.fn(),
  getTelegramUsers: vi.fn(),
  sendTelegramNotification: vi.fn(),
  setTelegramWebhook: vi.fn(),
  updateTelegramBot: vi.fn(),
  updateTelegramUserNotify: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

describe('Admin Telegram', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getTelegramBot.mockResolvedValue({
      data: {
        token: 'legacy-token',
        admin_ids: [1001, 1002],
        welcome_message: 'Legacy welcome'
      }
    })
    adminApi.getTelegramUsers.mockResolvedValue({
      data: {
        list: [{
          id: 1,
          telegram_id: 2001,
          user_id: 7,
          user_email: 'legacy@example.com',
          notify_enabled: true,
          notify_expire: true,
          notify_traffic: true,
          notify_ticket: true
        }],
        total: 1
      }
    })
    adminApi.broadcastTelegram.mockResolvedValue({ data: { success: 0, failed: 0 } })
    adminApi.deleteTelegramWebhook.mockResolvedValue({})
    adminApi.sendTelegramNotification.mockResolvedValue({})
    adminApi.setTelegramWebhook.mockResolvedValue({})
    adminApi.updateTelegramBot.mockResolvedValue({})
    adminApi.updateTelegramUserNotify.mockResolvedValue({
      data: {
        notify_enabled: false,
        notify_expire: false,
        notify_traffic: false,
        notify_ticket: false
      }
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads bot config and users from legacy and panel envelope payloads', async () => {
    const wrapper = mount(Telegram)
    await flushPromises()

    expect(wrapper.vm.botConfig.token).toBe('legacy-token')
    expect(wrapper.vm.botConfig.admin_ids).toEqual([1001, 1002])
    expect(wrapper.vm.users).toHaveLength(1)
    expect(wrapper.vm.users[0].user_email).toBe('legacy@example.com')

    adminApi.getTelegramBot.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        token: 'panel-token',
        admin_ids: [3001],
        welcome_message: 'Panel welcome'
      },
      ts: 1783526400000
    })
    adminApi.getTelegramUsers.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        list: [{
          id: 2,
          telegram_id: 4001,
          user_id: 8,
          user_email: 'panel@example.com',
          notify_enabled: false,
          notify_expire: false,
          notify_traffic: false,
          notify_ticket: false
        }],
        total: 1
      },
      ts: 1783526400000
    })

    await wrapper.vm.fetchBotConfig()
    await wrapper.vm.fetchUsers()
    await flushPromises()

    expect(wrapper.vm.botConfig.token).toBe('panel-token')
    expect(wrapper.vm.botConfig.admin_ids).toEqual([3001])
    expect(wrapper.vm.users).toHaveLength(1)
    expect(wrapper.vm.users[0].user_email).toBe('panel@example.com')

    wrapper.unmount()
  })

  it('updates notify state and broadcast counts from panel envelope payloads', async () => {
    const wrapper = mount(Telegram)
    await flushPromises()

    adminApi.updateTelegramUserNotify.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        notify_enabled: false,
        notify_expire: false,
        notify_traffic: false,
        notify_ticket: false
      },
      ts: 1783526400000
    })

    await wrapper.vm.toggleUserNotify(wrapper.vm.users[0])
    expect(adminApi.updateTelegramUserNotify).toHaveBeenCalledWith(1, { notify_enabled: false })
    expect(wrapper.vm.users[0].notify_enabled).toBe(false)
    expect(wrapper.vm.users[0].notify_expire).toBe(false)

    adminApi.broadcastTelegram.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { message: 'broadcast completed', success: 3, failed: 1 },
      ts: 1783526400000
    })
    wrapper.vm.notifyForm.type = 'broadcast'
    wrapper.vm.notifyForm.message = 'hello'

    // A broadcast asks first; declining sends nothing.
    const confirms = answerConfirms(false)
    await wrapper.vm.sendNotification()
    expect(confirms.last()).toMatchObject({ title: 'Broadcast this message to every bound user?', confirmLabel: 'Broadcast' })
    expect(adminApi.broadcastTelegram).not.toHaveBeenCalled()

    confirms.answer(true)
    await wrapper.vm.sendNotification()
    expect(adminApi.broadcastTelegram).toHaveBeenCalledWith('hello')
    // Some deliveries failed: a warning, not a success.
    expect(toastMessages('warning')).toEqual(['Broadcast finished. Success: 3, failed: 1'])
    expect(wrapper.vm.notifyForm.message).toBe('')

    wrapper.unmount()
  })

  it('keeps config, webhook, and single-message write flows working', async () => {
    const wrapper = mount(Telegram)
    await flushPromises()

    await wrapper.vm.saveBotConfig()
    expect(adminApi.updateTelegramBot).toHaveBeenCalledWith(wrapper.vm.botConfig)
    expect(toastMessages('success')).toContain('Telegram bot config saved')

    await wrapper.vm.setWebhookConfig()
    expect(adminApi.setTelegramWebhook).toHaveBeenCalledWith('http://localhost:3000/api/v2/telegram/webhook')
    expect(toastMessages('success')).toContain('Webhook configured successfully')

    await wrapper.vm.deleteWebhookConfig()
    expect(adminApi.deleteTelegramWebhook).toHaveBeenCalled()
    const deleted = toasts('success').find(toast => toast.message === 'Webhook deleted successfully')
    expect(deleted.action?.undo).toBe(true)
    // 撤销 sets the webhook again.
    adminApi.setTelegramWebhook.mockClear()
    await runAction(deleted.id)
    expect(adminApi.setTelegramWebhook).toHaveBeenCalledTimes(1)

    wrapper.vm.notifyForm.type = 'single'
    wrapper.vm.notifyForm.telegram_id = '2001'
    wrapper.vm.notifyForm.message = 'hello'
    await wrapper.vm.sendNotification()
    expect(adminApi.sendTelegramNotification).toHaveBeenCalledWith({
      telegram_id: '2001',
      message: 'hello'
    })
    expect(toastMessages('success')).toContain('Message sent')

    wrapper.unmount()
  })

  it('logs panel envelope load failures instead of accepting empty config payloads', async () => {
    adminApi.getTelegramBot.mockResolvedValueOnce({
      code: -1,
      msg: 'telegram config failed',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Telegram)
    await flushPromises()

    expect(wrapper.vm.botConfig.token).toBe('')
    expect(console.error).toHaveBeenCalledWith(
      'Failed to load Telegram bot config',
      expect.any(Error)
    )

    wrapper.unmount()
  })

  it('does not report write success when a panel envelope mutation fails', async () => {
    adminApi.updateTelegramBot.mockResolvedValueOnce({
      code: -1,
      msg: 'telegram save rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Telegram)
    await flushPromises()

    await wrapper.vm.saveBotConfig()

    expect(toastMessages('error')).toEqual([expect.stringContaining('telegram save rejected')])
    expect(toastMessages('success')).not.toContain('Telegram bot config saved')

    wrapper.unmount()
  })

  it('shows missing Telegram ID and message as inline field errors', async () => {
    const wrapper = mount(Telegram)
    await flushPromises()

    wrapper.vm.notifyForm.type = 'single'
    await wrapper.vm.sendNotification()
    await flushPromises()

    expect(wrapper.find('#telegram-notify-id-error').text()).toBe('Please enter a Telegram ID')
    expect(wrapper.find('#telegram-notify-message-error').text()).toBe('Please enter a message')
    expect(wrapper.find('[data-test="telegram-notify-message"]').attributes('aria-invalid')).toBe('true')
    expect(adminApi.sendTelegramNotification).not.toHaveBeenCalled()
    expect(toasts()).toHaveLength(0)

    wrapper.unmount()
  })

  it('reports a failed notify toggle in a persistent error toast', async () => {
    adminApi.updateTelegramUserNotify.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mount(Telegram)
    await flushPromises()

    await wrapper.vm.toggleUserNotify(wrapper.vm.users[0])
    const [toast] = toasts('error')
    expect(toast.message).toContain('offline')
    expect(toast.duration).toBe(0)

    wrapper.unmount()
  })
})
