// 通知 (UI U7): one page with the e-mail, Telegram, templates and send log
// channels in the path; e-mail is a settings form with the save bar and a
// test send. The Telegram panel has its own test file (AdminTelegram).
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import UiHost from '@/ui/UiHost.vue'
import Notifications from '@/views/admin/Notifications.vue'
import NotifyEmail from '@/views/admin/notifications/NotifyEmail.vue'
import NotifyLogs from '@/views/admin/notifications/NotifyLogs.vue'
import NotifyTemplates from '@/views/admin/notifications/NotifyTemplates.vue'
import { setLocale } from '@/i18n'
import { answerConfirms, inBody, toastMessages, toasts } from './helpers/feedback'

const adminApi = vi.hoisted(() => ({
  createNotificationTemplate: vi.fn(),
  deleteNotificationTemplate: vi.fn(),
  getEmailConfig: vi.fn(),
  getNotificationLogs: vi.fn(),
  getNotificationTemplates: vi.fn(),
  sendTestNotification: vi.fn(),
  updateEmailConfig: vi.fn(),
  updateNotificationTemplate: vi.fn(),
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

enableAutoUnmount(afterEach)

async function mountPage(path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/notifications/:channel?', component: Notifications },
      { path: '/admin/telegram', redirect: '/admin/notifications/telegram' },
      { path: '/admin/users', component: { template: '<div />' } }
    ]
  })
  await router.push(path)
  await router.isReady()
  const wrapper = mount({ template: '<router-view />' }, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return { wrapper, router }
}

describe('Admin notifications', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApi.getNotificationTemplates.mockResolvedValue({
      data: { list: [{ id: 1, name: 'Legacy Template', type: 'email', event: 'user.register', enabled: true }], total: 1 }
    })
    adminApi.getNotificationLogs.mockResolvedValue({
      data: { list: [{ id: 9, type: 'email', recipient: 'user:1', title: 'Legacy Log', status: 'success', created_at: 1783526400000 }], total: 1 }
    })
    adminApi.getEmailConfig.mockResolvedValue({
      data: { host: 'smtp.legacy.test', port: 465, username: 'legacy-user', from_name: 'Legacy Mailer', from_address: 'legacy@example.com', encryption: true }
    })
    adminApi.getTelegramBot.mockResolvedValue({ data: { token: '', admin_ids: [], welcome_message: '' } })
    adminApi.getTelegramUsers.mockResolvedValue({ data: { list: [] } })
    adminApi.createNotificationTemplate.mockResolvedValue({})
    adminApi.deleteNotificationTemplate.mockResolvedValue({})
    adminApi.sendTestNotification.mockResolvedValue({})
    adminApi.updateEmailConfig.mockResolvedValue({})
    adminApi.updateNotificationTemplate.mockResolvedValue({})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('page', () => {
    it('switches channels with segmented tabs kept in the path', async () => {
      const { wrapper, router } = await mountPage('/admin/notifications')
      const tabs = wrapper.findAll('[role="tab"]')
      expect(tabs.map(tab => tab.text())).toEqual(['E-mail', 'Telegram', 'Templates', 'Send log'])
      expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('E-mail')
      expect(wrapper.find('[data-notify-panel="email"]').exists()).toBe(true)
      expect(adminApi.getNotificationTemplates).not.toHaveBeenCalled()

      await tabs[2].trigger('mousedown', { button: 0 })
      await flushPromises()
      expect(router.currentRoute.value.path).toBe('/admin/notifications/templates')
      expect(wrapper.find('[data-notify-panel="templates"]').exists()).toBe(true)
      expect(adminApi.getNotificationTemplates).toHaveBeenCalledTimes(1)
    })

    it('sends /admin/telegram to the Telegram channel', async () => {
      const { wrapper, router } = await mountPage('/admin/telegram')
      expect(router.currentRoute.value.path).toBe('/admin/notifications/telegram')
      expect(wrapper.find('[data-notify-panel="telegram"]').exists()).toBe(true)
    })

    it('asks before leaving unsaved e-mail settings', async () => {
      const { wrapper, router } = await mountPage('/admin/notifications/email')
      await wrapper.get('#smtp-host').setValue('smtp.changed.test')
      await flushPromises()
      expect(document.querySelector('[data-test="settings-save-bar"]')).not.toBeNull()

      const confirms = answerConfirms(false)
      await router.push('/admin/notifications/logs')
      await flushPromises()
      expect(confirms.last()).toMatchObject({ title: 'Discard unsaved changes?' })
      expect(router.currentRoute.value.path).toBe('/admin/notifications/email')
    })
  })

  describe('e-mail', () => {
    it('loads the e-mail settings from legacy and panel envelope payloads', async () => {
      const wrapper = mount(NotifyEmail)
      await flushPromises()
      expect(wrapper.vm.emailConfig.host).toBe('smtp.legacy.test')

      adminApi.getEmailConfig.mockResolvedValueOnce({ code: 0, msg: 'ok', data: { host: 'smtp.panel.test', port: 587, encryption: false }, ts: 1 })
      await wrapper.vm.fetchEmailSettings()
      expect(wrapper.vm.emailConfig.host).toBe('smtp.panel.test')
      expect(wrapper.vm.emailConfig.encryption).toBe(false)
      expect(wrapper.vm.emailDirty).toBe(false)
    })

    it('starts the SMTP password empty when the API masks it, and saves it empty to keep it', async () => {
      adminApi.getEmailConfig.mockResolvedValue({
        code: 0,
        msg: 'ok',
        data: { host: 'smtp.masked.test', port: 465, username: 'mailer', password: '********', from_name: 'Mailer', from_address: 'mailer@example.com', encryption: true },
        ts: 1783526400000
      })
      const wrapper = mount(NotifyEmail)
      await flushPromises()

      expect(wrapper.vm.emailConfig.password).toBe('')
      expect(wrapper.vm.emailPasswordStored).toBe(true)
      expect(wrapper.text()).toContain('Saved and hidden. Leave it empty to keep the current password.')

      wrapper.vm.emailConfig.username = 'mailer2'
      await wrapper.vm.saveEmailSettings()
      expect(adminApi.updateEmailConfig).toHaveBeenCalledWith(expect.objectContaining({ host: 'smtp.masked.test', username: 'mailer2', password: '' }))
      expect(wrapper.vm.emailDirty).toBe(false)

      adminApi.getEmailConfig.mockResolvedValueOnce({ code: 0, msg: 'ok', data: { host: 'smtp.masked.test', password: '' }, ts: 1 })
      await wrapper.vm.fetchEmailSettings()
      expect(wrapper.vm.emailPasswordStored).toBe(false)
    })

    it('validates as you type and blocks saving an invalid form', async () => {
      const wrapper = mount(NotifyEmail)
      await flushPromises()
      wrapper.vm.emailConfig.from_address = 'not-an-address'
      wrapper.vm.emailConfig.port = 70000
      await flushPromises()
      expect(wrapper.vm.fromAddressError).toBe('Enter a valid e-mail address')
      expect(wrapper.vm.portError).toBe('A port is a whole number from 1 to 65535')
      expect(wrapper.vm.emailInvalid).toBe(true)
      await wrapper.vm.saveEmailSettings()
      expect(adminApi.updateEmailConfig).not.toHaveBeenCalled()
    })

    it('reports a failed save in an error toast', async () => {
      adminApi.updateEmailConfig.mockResolvedValueOnce({ code: -1, msg: 'smtp config rejected' })
      const wrapper = mount(NotifyEmail)
      await flushPromises()
      wrapper.vm.emailConfig.host = 'smtp.save.test'
      await wrapper.vm.saveEmailSettings()
      expect(toastMessages('error')).toEqual(['Couldn’t save: smtp config rejected'])
      expect(wrapper.vm.emailDirty).toBe(true)
    })

    it('sends a test e-mail from a dialog that validates the recipient', async () => {
      const user = userEvent.setup()
      adminApi.sendTestNotification.mockResolvedValueOnce({ code: -1, msg: 'smtp refused' })
      render({ components: { NotifyEmail, UiHost }, template: '<div><NotifyEmail /><UiHost /></div>' })
      await screen.findByDisplayValue('smtp.legacy.test')
      await user.click(screen.getByRole('button', { name: 'Send test' }))
      const dialog = await screen.findByRole('dialog', { name: 'Send a test e-mail' })

      await user.click(within(dialog).getByRole('button', { name: 'Send test e-mail' }))
      const field = within(dialog).getByRole('textbox')
      expect(field.getAttribute('aria-invalid')).toBe('true')
      expect(dialog.textContent).toContain('Enter an address to send to')
      expect(adminApi.sendTestNotification).not.toHaveBeenCalled()

      await user.type(field, 'ops@example.com')
      await user.click(within(dialog).getByRole('button', { name: 'Send test e-mail' }))
      await waitFor(() => expect(dialog.textContent).toContain('smtp refused'))
      expect(toasts()).toHaveLength(0)

      await user.click(within(dialog).getByRole('button', { name: 'Send test e-mail' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(adminApi.sendTestNotification).toHaveBeenLastCalledWith({
        type: 'email',
        recipient: 'ops@example.com',
        subject: 'Test e-mail',
        content: 'This is a test e-mail. If you received it, the e-mail settings work.'
      })
      expect(toastMessages('success')).toEqual(['Test e-mail sent to ops@example.com'])
    })
  })

  describe('templates', () => {
    it('loads templates from legacy and panel envelope payloads', async () => {
      const wrapper = mount(NotifyTemplates)
      await flushPromises()
      expect(wrapper.vm.templates[0].name).toBe('Legacy Template')

      adminApi.getNotificationTemplates.mockResolvedValueOnce({ code: 0, msg: 'ok', data: { list: [{ id: 2, name: 'Panel Template', type: 'telegram', event: 'order.paid', enabled: false }] }, ts: 1 })
      await wrapper.vm.fetchTemplates()
      expect(wrapper.vm.templates[0].name).toBe('Panel Template')
      expect(wrapper.text()).toContain('Order paid')
    })

    it('shows a load failure as the table error state', async () => {
      adminApi.getNotificationTemplates.mockResolvedValueOnce({ code: -1, msg: 'failed to load templates', data: null, ts: 1 })
      const wrapper = mount(NotifyTemplates)
      await flushPromises()
      expect(wrapper.vm.templates).toHaveLength(0)
      expect(wrapper.vm.templatesError).toBe('failed to load templates')
      expect(wrapper.text()).toContain('failed to load templates')
    })

    it('creates a template and keeps a failed save inside the dialog', async () => {
      adminApi.createNotificationTemplate.mockResolvedValueOnce({ code: -1, msg: 'server rejected template', data: null, ts: 1 })
      const wrapper = mount(NotifyTemplates, { attachTo: document.body })
      await flushPromises()
      const form = { name: 'Created Template', type: 'email', event: 'user.login', title: 'Login', content: 'Body', enabled: true }

      wrapper.vm.openTemplateModal()
      wrapper.vm.templateForm = { ...form }
      await wrapper.vm.saveTemplate()
      await flushPromises()
      expect(wrapper.vm.showTemplateModal).toBe(true)
      expect(inBody('[data-test="notification-template-error"]').text()).toContain('server rejected template')
      expect(toasts()).toHaveLength(0)

      await wrapper.vm.saveTemplate()
      expect(adminApi.createNotificationTemplate).toHaveBeenLastCalledWith(form)
      expect(toastMessages('success')).toContain('Notification template saved')
      expect(wrapper.vm.showTemplateModal).toBe(false)
    })

    it('confirms deleting a template; Cancel and Esc keep it, a failure stays inline', async () => {
      const user = userEvent.setup()
      render({ components: { NotifyTemplates, UiHost }, template: '<div><NotifyTemplates /><UiHost /></div>' })
      await screen.findByText('Legacy Template')
      const opener = screen.getByRole('button', { name: 'Actions for Legacy Template' })
      const choose = async () => {
        await user.click(opener)
        await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Delete' }))
      }

      await choose()
      let dialog = await screen.findByRole('alertdialog', { name: 'Delete notification template Legacy Template?' })
      await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())

      await choose()
      await screen.findByRole('alertdialog')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))
      expect(adminApi.deleteNotificationTemplate).not.toHaveBeenCalled()

      adminApi.deleteNotificationTemplate.mockResolvedValueOnce({ code: -1, msg: 'template in use' })
      await choose()
      dialog = await screen.findByRole('alertdialog')
      await user.click(within(dialog).getByRole('button', { name: 'Delete template' }))
      expect((await within(dialog).findByRole('alert')).textContent).toContain('template in use')

      await user.click(within(dialog).getByRole('button', { name: 'Delete template' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApi.deleteNotificationTemplate).toHaveBeenLastCalledWith(1)
      expect(toastMessages('success')).toEqual(['Deleted notification template Legacy Template'])
    })

    it('shows the variable hint literally in the content field', async () => {
      const user = userEvent.setup()
      render({ components: { NotifyTemplates, UiHost }, template: '<div><NotifyTemplates /><UiHost /></div>' })
      await screen.findByText('Legacy Template')
      await user.click(screen.getByRole('button', { name: 'Actions for Legacy Template' }))
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Edit' }))
      const dialog = await screen.findByRole('dialog', { name: 'Edit notification template' })
      expect(within(dialog).getByLabelText(/Name/).value).toBe('Legacy Template')
      expect(within(dialog).getByLabelText('Content').getAttribute('placeholder')).toBe('The notification text. Variables: {username}, {email}, {expire_time}')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(adminApi.updateNotificationTemplate).not.toHaveBeenCalled()
    })
  })

  describe('send log', () => {
    it('loads the log and filters it on the server', async () => {
      const wrapper = mount(NotifyLogs)
      await flushPromises()
      expect(wrapper.vm.logs[0].title).toBe('Legacy Log')
      expect(adminApi.getNotificationLogs).toHaveBeenCalledWith({ type: '', status: '' })

      adminApi.getNotificationLogs.mockResolvedValueOnce({ code: 0, msg: 'ok', data: { list: [{ id: 10, type: 'telegram', title: 'Panel Log', status: 'failed' }] }, ts: 1 })
      wrapper.vm.logFilter.status = 'failed'
      await wrapper.vm.fetchLogs()
      expect(adminApi.getNotificationLogs).toHaveBeenLastCalledWith({ type: '', status: 'failed' })
      expect(wrapper.vm.logs[0].title).toBe('Panel Log')
    })

    it('shows a load failure as the table error state', async () => {
      adminApi.getNotificationLogs.mockResolvedValueOnce({ code: -1, msg: 'logs rejected' })
      const wrapper = mount(NotifyLogs)
      await flushPromises()
      expect(wrapper.vm.logsError).toBe('logs rejected')
    })
  })
})
