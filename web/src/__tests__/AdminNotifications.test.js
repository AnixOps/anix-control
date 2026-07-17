import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Notifications from '@/views/admin/Notifications.vue'
import { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  createNotificationTemplate: vi.fn(),
  deleteNotificationTemplate: vi.fn(),
  getEmailConfig: vi.fn(),
  getNotificationLogs: vi.fn(),
  getNotificationTemplates: vi.fn(),
  sendTestNotification: vi.fn(),
  updateEmailConfig: vi.fn(),
  updateNotificationTemplate: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

describe('Admin Notifications', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.spyOn(window, 'alert').mockImplementation(() => {})
    vi.spyOn(window, 'confirm').mockImplementation(() => true)
    await setLocale('en')

    adminApi.getNotificationTemplates.mockResolvedValue({
      data: {
        list: [{
          id: 1,
          name: 'Legacy Template',
          type: 'email',
          event: 'user.register',
          enabled: true
        }],
        total: 1
      }
    })
    adminApi.getNotificationLogs.mockResolvedValue({
      data: {
        list: [{
          id: 9,
          type: 'email',
          recipient: 'user:1',
          title: 'Legacy Log',
          status: 'success',
          created_at: 1783526400000
        }],
        total: 1
      }
    })
    adminApi.getEmailConfig.mockResolvedValue({
      data: {
        host: 'smtp.legacy.test',
        port: 465,
        username: 'legacy-user',
        from_name: 'Legacy Mailer',
        from_address: 'legacy@example.com',
        encryption: true
      }
    })
    adminApi.createNotificationTemplate.mockResolvedValue({})
    adminApi.deleteNotificationTemplate.mockResolvedValue({})
    adminApi.sendTestNotification.mockResolvedValue({})
    adminApi.updateEmailConfig.mockResolvedValue({})
    adminApi.updateNotificationTemplate.mockResolvedValue({})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads templates, logs, and email settings from legacy and panel envelope payloads', async () => {
    const wrapper = mount(Notifications)
    await flushPromises()

    expect(wrapper.vm.templates).toHaveLength(1)
    expect(wrapper.vm.templates[0].name).toBe('Legacy Template')
    expect(wrapper.vm.logs).toHaveLength(1)
    expect(wrapper.vm.logs[0].title).toBe('Legacy Log')
    expect(wrapper.vm.emailConfig.host).toBe('smtp.legacy.test')

    adminApi.getNotificationTemplates.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        list: [{
          id: 2,
          name: 'Panel Template',
          type: 'telegram',
          event: 'ticket.reply',
          enabled: false
        }],
        total: 1
      },
      ts: 1783526400000
    })
    adminApi.getNotificationLogs.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        list: [{
          id: 10,
          type: 'webhook',
          recipient: 'user:2',
          title: 'Panel Log',
          status: 'failed',
          created_at: 1786118400000
        }],
        total: 1
      },
      ts: 1783526400000
    })
    adminApi.getEmailConfig.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: {
        host: 'smtp.panel.test',
        port: 587,
        username: 'panel-user',
        from_name: 'Panel Mailer',
        from_address: 'panel@example.com',
        encryption: false
      },
      ts: 1783526400000
    })

    await wrapper.vm.fetchTemplates()
    await wrapper.vm.fetchLogs()
    await wrapper.vm.fetchEmailSettings()
    await flushPromises()

    expect(wrapper.vm.templates).toHaveLength(1)
    expect(wrapper.vm.templates[0].name).toBe('Panel Template')
    expect(wrapper.vm.logs).toHaveLength(1)
    expect(wrapper.vm.logs[0].title).toBe('Panel Log')
    expect(wrapper.vm.emailConfig.host).toBe('smtp.panel.test')
    expect(wrapper.vm.emailConfig.encryption).toBe(false)

    wrapper.unmount()
  })

  it('keeps write flows working after response envelope normalization', async () => {
    const wrapper = mount(Notifications)
    await flushPromises()

    wrapper.vm.openTemplateModal()
    wrapper.vm.templateForm = {
      name: 'Created Template',
      type: 'email',
      event: 'user.login',
      title: 'Login',
      content: 'Body',
      enabled: true
    }
    await wrapper.vm.saveTemplate()
    expect(adminApi.createNotificationTemplate).toHaveBeenCalledWith(wrapper.vm.templateForm)
    expect(window.alert).toHaveBeenCalledWith('Template saved successfully')

    wrapper.vm.emailConfig.host = 'smtp.save.test'
    await wrapper.vm.saveEmailSettings()
    expect(adminApi.updateEmailConfig).toHaveBeenCalledWith(wrapper.vm.emailConfig)
    expect(window.alert).toHaveBeenCalledWith('Email configuration saved')

    wrapper.vm.testEmail = 'ops@example.com'
    await wrapper.vm.sendTestEmail()
    expect(adminApi.sendTestNotification).toHaveBeenCalledWith({
      type: 'email',
      recipient: 'ops@example.com',
      subject: 'Test Email',
      content: 'This is a test email. If you received it, the email configuration is working correctly.'
    })
    expect(window.alert).toHaveBeenCalledWith('Test email sent successfully')

    wrapper.unmount()
  })

  it('logs panel envelope load failures instead of treating them as empty success payloads', async () => {
    adminApi.getNotificationTemplates.mockResolvedValueOnce({
      code: -1,
      msg: 'failed to load templates',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Notifications)
    await flushPromises()

    expect(wrapper.vm.templates).toHaveLength(0)
    expect(console.error).toHaveBeenCalledWith(
      'Failed to load notification templates',
      expect.any(Error)
    )

    wrapper.unmount()
  })

  it('does not report write success when a panel envelope mutation fails', async () => {
    adminApi.createNotificationTemplate.mockResolvedValueOnce({
      code: -1,
      msg: 'server rejected template',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Notifications)
    await flushPromises()

    wrapper.vm.openTemplateModal()
    wrapper.vm.templateForm = {
      name: 'Rejected Template',
      type: 'email',
      event: 'user.login',
      title: 'Login',
      content: 'Body',
      enabled: true
    }
    await wrapper.vm.saveTemplate()

    expect(window.alert).toHaveBeenCalledWith(
      expect.stringContaining('server rejected template')
    )
    expect(window.alert).not.toHaveBeenCalledWith('Template saved successfully')

    wrapper.unmount()
  })
})
