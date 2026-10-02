const TEMPLATES = [
  { id: 1, name: '注册欢迎', type: 'email', event: 'user.register', title: '欢迎加入', content: '你好 {username}', enabled: true },
  { id: 2, name: '到期提醒', type: 'telegram', event: 'user.expire', enabled: true },
  { id: 3, name: '节点离线告警', type: 'webhook', event: 'node.offline', enabled: false }
]
const LOGS = [
  { id: 31, type: 'email', recipient: 'lin.xiao@example.com', title: '欢迎加入', status: 'success', created_at: '2026-10-02T08:00:00Z' },
  { id: 30, type: 'telegram', recipient: '123456789', title: '订阅即将到期', status: 'failed', created_at: '2026-10-02T07:00:00Z' },
  { id: 29, type: 'email', recipient: 'ops@example.com', title: '测试邮件', status: 'pending', created_at: '2026-10-02T06:00:00Z' }
]
const PATHS = { email: 'email', emailDirty: 'email', testDialog: 'email', emailError: 'email', telegram: 'telegram', telegramNoToken: 'telegram', templates: 'templates', templatesEmpty: 'templates', templateDialog: 'templates', logs: 'logs', logsError: 'logs', legacy: null }
export default {
  pathFor: s => (s === 'legacy' ? '/admin/telegram' : `/admin/notifications/${PATHS[s]}`),
  api(path, { scenario }) {
    if (path === '/api/v2/admin/notification/email/config') {
      if (scenario === 'emailError') return { __status: 502, body: { msg: '通知服务没有响应' } }
      return { code: 0, data: { host: 'smtp.example.com', port: 465, username: 'noreply@example.com', password: '********', from_name: 'AnixOps', from_address: 'noreply@example.com', encryption: true } }
    }
    if (path === '/api/v2/admin/notification/templates') return { code: 0, data: { list: scenario === 'templatesEmpty' ? [] : TEMPLATES } }
    if (path === '/api/v2/admin/notification/logs') {
      if (scenario === 'logsError') return { code: 500, msg: '日志服务没有响应', data: null }
      return { code: 0, data: { list: LOGS } }
    }
    if (path === '/api/v2/admin/telegram/bot') return { code: 0, data: scenario === 'telegramNoToken' ? {} : { token: '123456:ABC-DEF', admin_ids: [10001, 10002], welcome_message: '欢迎使用 AnixOps 机器人' } }
    if (path === '/api/v2/admin/telegram/users') return { code: 0, data: { list: [
      { id: 1, telegram_id: 123456789, user_id: 7, user_email: 'lin.xiao@example.com', notify_enabled: true, created_at: '2026-09-01T00:00:00Z' },
      { id: 2, telegram_id: 987654321, user_id: 9, user_email: 'ops@example.com', notify_enabled: false, created_at: '2026-09-12T00:00:00Z' }
    ] } }
    return undefined
  },
  viewportOnly: ['testDialog', 'templateDialog'],
  scenarios: {
    email: async () => {},
    emailDirty: async (page) => { await page.locator('#smtp-port').fill('70000'); await page.locator('#smtp-from-address').fill('bad') },
    testDialog: async (page) => { await page.getByRole('button', { name: '测试发送' }).click(); await page.getByRole('dialog').getByRole('button', { name: '发送测试邮件' }).click() },
    emailError: async () => {},
    telegram: async () => {},
    telegramNoToken: async () => {},
    templates: async () => {},
    templatesEmpty: async () => {},
    templateDialog: async (page) => { await page.getByRole('button', { name: '新建模板' }).first().click() },
    logs: async () => {},
    logsError: async () => {},
    legacy: async () => {}
  }
}
