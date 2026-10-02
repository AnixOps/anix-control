const SCOPES = [{ id: 'forward', name: '转发' }]
const GROUPS = [{ id: 7, scope_id: 'forward', name: '金丝雀运维', description: '灰度发布期间的专用访问', enabled: true }, { id: 8, scope_id: 'forward', name: '只读审计', enabled: false }]
const PATHS = { mfa: '/admin/security/mfa', mfaDirty: '/admin/security/mfa', mfaError: '/admin/security/mfa', groups: '/admin/security/access-groups', phoneList: '/admin/security', legacy: '/admin/mfa' }
export default {
  pathFor: s => PATHS[s],
  api(path, { scenario }) {
    if (path === '/api/v2/admin/mfa/config') {
      if (scenario === 'mfaError') return { code: 500, msg: '身份服务没有响应', data: null }
      return { code: 0, data: { enabled: true, required: false, methods: { totp: true, sms: false, email: true }, backup_codes_count: 10, max_attempts: 5, lockout_duration: 15 } }
    }
    if (path === '/api/v3/service-scopes') return { data: SCOPES }
    if (path === '/api/v3/access-groups') return { data: GROUPS }
    return undefined
  },
  scenarios: {
    mfa: async () => {},
    mfaDirty: async (page) => { await page.locator('#mfa-method-totp').click(); await page.locator('#mfa-method-email').click() },
    mfaError: async () => {},
    groups: async () => {},
    phoneList: async () => {},
    legacy: async () => {}
  }
}
