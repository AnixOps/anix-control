import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import Notifications from '@/views/admin/Notifications.vue'
import { setLocale } from '@/i18n'
import { inBody, toastMessages, toasts } from './helpers/feedback'

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

enableAutoUnmount(afterEach)

describe('Admin Notifications', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
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

  it('starts the SMTP password empty when the API masks it, and saves it empty to keep it', async () => {
    adminApi.getEmailConfig.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        host: 'smtp.masked.test',
        port: 465,
        username: 'mailer',
        password: '********',
        from_name: 'Mailer',
        from_address: 'mailer@example.com',
        encryption: true
      },
      ts: 1783526400000
    })
    const wrapper = mount(Notifications)
    await flushPromises()

    expect(wrapper.vm.emailConfig.password).toBe('')
    expect(wrapper.vm.emailPasswordStored).toBe(true)
    const input = wrapper.find('input[type="password"]')
    expect(input.attributes('placeholder')).toBe('Password stored; leave blank to keep it')

    await wrapper.vm.saveEmailSettings()
    expect(adminApi.updateEmailConfig).toHaveBeenCalledWith(expect.objectContaining({
      host: 'smtp.masked.test',
      password: ''
    }))

    adminApi.getEmailConfig.mockResolvedValueOnce({ code: 0, msg: 'ok', data: { host: 'smtp.masked.test', password: '' }, ts: 1 })
    await wrapper.vm.fetchEmailSettings()
    expect(wrapper.vm.emailPasswordStored).toBe(false)
    expect(wrapper.find('input[type="password"]').attributes('placeholder')).toBe('Enter SMTP password')

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
    expect(toastMessages('success')).toContain('Template saved successfully')
    expect(wrapper.vm.showTemplateModal).toBe(false)

    wrapper.vm.emailConfig.host = 'smtp.save.test'
    await wrapper.vm.saveEmailSettings()
    expect(adminApi.updateEmailConfig).toHaveBeenCalledWith(wrapper.vm.emailConfig)
    expect(toastMessages('success')).toContain('Email configuration saved')

    wrapper.vm.testEmail = 'ops@example.com'
    await wrapper.vm.sendTestEmail()
    expect(adminApi.sendTestNotification).toHaveBeenCalledWith({
      type: 'email',
      recipient: 'ops@example.com',
      subject: 'Test Email',
      content: 'This is a test email. If you received it, the email configuration is working correctly.'
    })
    expect(toastMessages('success')).toContain('Test email sent successfully')

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

    const wrapper = mount(Notifications, { attachTo: document.body })
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

    await flushPromises()
    // The failure stays in the open dialog, not in a toast.
    expect(wrapper.vm.showTemplateModal).toBe(true)
    expect(inBody('[data-test="notification-template-error"]').text()).toContain('server rejected template')
    expect(toasts()).toHaveLength(0)

    wrapper.unmount()
  })

  describe('dialogs', () => {
    const Harness = {
      components: { Notifications, UiHost },
      template: '<div><Notifications /><UiHost /></div>'
    }

    it('confirms deleting a template; Cancel and Esc keep it, a failure stays inline', async () => {
      const user = userEvent.setup()
      render(Harness)
      await screen.findByText('Legacy Template')
      const opener = screen.getByRole('button', { name: 'Delete' })

      await user.click(opener)
      let dialog = await screen.findByRole('alertdialog', { name: 'Delete template Legacy Template?' })
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

      await user.click(opener)
      await screen.findByRole('alertdialog')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))
      expect(adminApi.deleteNotificationTemplate).not.toHaveBeenCalled()

      adminApi.deleteNotificationTemplate.mockResolvedValueOnce({ code: -1, msg: 'template in use' })
      await user.click(opener)
      dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete template' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('template in use')

      await user.click(within(dialog).getByRole('button', { name: 'Delete template' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deleteNotificationTemplate).toHaveBeenLastCalledWith(1)
      expect(toastMessages('success')).toEqual(['Template Legacy Template deleted'])
    })

    it('sends a test email from a dialog that validates the recipient', async () => {
      const user = userEvent.setup()
      adminApi.sendTestNotification.mockResolvedValueOnce({ code: -1, msg: 'smtp refused' })
      render(Harness)
      await screen.findByText('Legacy Template')
      await user.click(screen.getByRole('button', { name: 'Email' }))
      await user.click(screen.getByRole('button', { name: 'Send test email' }))
      const dialog = await screen.findByRole('dialog', { name: 'Send Test Email' })

      await user.click(within(dialog).getByRole('button', { name: 'Send' }))
      const field = within(dialog).getByRole('textbox')
      expect(field.getAttribute('aria-invalid')).toBe('true')
      expect(dialog.textContent).toContain('Please enter a recipient email address')
      expect(adminApi.sendTestNotification).not.toHaveBeenCalled()

      await user.type(field, 'ops@example.com')
      await user.click(within(dialog).getByRole('button', { name: 'Send' }))
      await waitFor(() => expect(dialog.textContent).toContain('smtp refused'))
      expect(toasts()).toHaveLength(0)

      await user.click(within(dialog).getByRole('button', { name: 'Send' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(toastMessages('success')).toEqual(['Test email sent successfully'])
    })

    it('edits a template in a dialog that closes with Esc', async () => {
      const user = userEvent.setup()
      render(Harness)
      await screen.findByText('Legacy Template')
      await user.click(screen.getByRole('button', { name: 'Edit' }))
      const dialog = await screen.findByRole('dialog')
      expect(dialog.getAttribute('aria-modal')).toBe('true')
      expect(within(dialog).getByLabelText(/Template name|Name/).value).toBe('Legacy Template')
      // The variable hints render literally (no i18n interpolation).
      expect(within(dialog).getByLabelText('Content Template').getAttribute('placeholder')).toBe('Supports variables: {username}, {email}, {expire_time}')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(adminApi.updateNotificationTemplate).not.toHaveBeenCalled()
    })
  })
})
