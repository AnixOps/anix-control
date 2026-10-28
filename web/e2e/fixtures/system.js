const CONFIGS = [
  { key: 'app.subscribe_domains', value: '["sub1.example.com","sub2.example.com"]', description: 'Alternate subscription domains', updated_at: '2026-10-01T08:00:00Z' },
  { key: 'site.name', value: 'AnixOps', description: '站点名称', updated_at: '2026-09-28T12:00:00Z' },
  { key: 'forward.runtime_backend', value: 'nftables_ansible', description: '转发运行时后端', updated_at: '2026-09-20T09:30:00Z' },
  { key: 'forward.runtime.nodex.token', value: '********', display_value: '********', sensitive: true, has_value: true, description: 'NodeX token', updated_at: '2026-09-20T09:31:00Z' },
  { key: 'smtp.timeout_seconds', value: '30', description: '', updated_at: '2026-09-02T10:00:00Z' }
]
// GET /api/v5/backups: the bundles in data_dir/backups with their
// verification (docs/reference/backups.md), at the fixed clock (2026-10-02
// 08:00 UTC).
const BACKUPS = [
  { id: 4, file: 'anix-control-20261002T034107Z-daily.tar', type: 'daily', size: 18874368, sha256: 'a'.repeat(64), created_at: '2026-10-02T03:41:07Z', verified_at: '2026-10-02T03:41:19Z', verify_status: 'passed', verify_error: '', program_version: '4.2.1', schema_version: 5 },
  { id: 3, file: 'anix-control-20261001T151204Z-manual.tar', type: 'manual', size: 18612224, sha256: 'b'.repeat(64), created_at: '2026-10-01T15:12:04Z', verified_at: '2026-10-01T15:12:15Z', verify_status: 'failed', verify_error: 'integrity: PRAGMA integrity_check: database disk image is malformed', program_version: '4.2.1', schema_version: 5 },
  { id: 2, file: 'anix-control-20261001T034107Z-daily.tar', type: 'daily', size: 18350080, sha256: 'c'.repeat(64), created_at: '2026-10-01T03:41:07Z', verified_at: '2026-10-01T03:41:18Z', verify_status: 'passed', verify_error: '', program_version: '4.2.1', schema_version: 5 },
  { id: 1, file: 'anix-control-20260930T090002Z-pre-migration.tar', type: 'pre-migration', size: 17825792, sha256: 'd'.repeat(64), created_at: '2026-09-30T09:00:02Z', verified_at: null, verify_status: 'pending', verify_error: '', program_version: '4.2.0', schema_version: 4 }
]

function backupsAnswer(scenario) {
  const unsupported = scenario === 'backupUnsupported'
  const list = scenario === 'backupEmpty' || unsupported ? [] : BACKUPS
  const stale = scenario === 'backupEmpty' || scenario === 'backupStale'
  const lastVerified = scenario === 'backupStale' ? BACKUPS[2] : list.find(backup => backup.verify_status === 'passed') || null
  return {
    data: {
      supported: !unsupported,
      reason: unsupported ? 'backups need SQLite: this Control runs on PostgreSQL, whose data Control\'s backups do not cover' : undefined,
      backups: scenario === 'backupStale' ? BACKUPS.slice(1) : list,
      summary: {
        last_verified: unsupported ? null : lastVerified,
        stale: !unsupported && stale,
        stale_after_hours: 36,
        next_daily_at: '2026-10-03T03:41:00Z',
        directory: '/var/lib/anix-control/backups'
      },
      retention: { daily: 7, weekly: 4, pre_migration_days: 30, manual: 3, verify: 1 }
    }
  }
}
const BALANCERS = [
  { id: 1, name: '香港出口', group_id: 2, group_name: 'HK', strategy: 'least-load', health_check: true, check_interval: 60, enabled: true, weights: {} },
  { id: 2, name: '日本加权', group_id: 3, group_name: 'JP', strategy: 'weight', health_check: false, check_interval: 120, enabled: false, weights: { 4: 10, 5: 5 } }
]
const AUDIT = Array.from({ length: 8 }, (_, i) => ({
  id: 120 - i,
  action: ['update', 'delete', 'create', 'login'][i % 4],
  module: ['system', 'forward', 'users', 'auth'][i % 4],
  target_type: ['config', 'forward_rule', 'user', 'session'][i % 4],
  username: i % 3 ? 'admin@example.com' : 'ops@example.com',
  content: ['updated forward.runtime_backend', 'deleted forward rule #42 hk-01 → jp-02', 'created user lin.xiao@example.com', 'signed in'][i % 4],
  ip: '203.0.113.' + (10 + i),
  status: i === 3 ? 'failed' : 'success',
  created_at: new Date(Date.UTC(2026, 9, 2, 9, 0 - i * 7)).toISOString()
}))
const SECTION = {
  general: 'general', dirty: 'general', invalid: 'general', configDialog: 'general', configError: 'general',
  backup: 'backup', backupEmpty: 'backup', backupStale: 'backup', backupUnsupported: 'backup', backupStaff: 'backup',
  balancer: 'balancer', balancerDialog: 'balancer', balancerEmpty: 'balancer',
  audit: 'audit', auditEmpty: 'audit', auditError: 'audit',
  about: 'about', aboutError: 'about',
  leave: 'general'
}
export default {
  pathFor(scenario) {
    if (scenario === 'phoneList') return '/admin/system'
    return `/admin/system/${SECTION[scenario] || 'general'}`
  },
  api(path, { scenario, query, method }) {
    if (path === '/api/v2/admin/system/configs') {
      if (scenario === 'configError') return { code: 500, msg: '数据库暂时不可用', data: null }
      return { code: 0, data: { list: CONFIGS, total: CONFIGS.length } }
    }
    if (path.startsWith('/api/v2/admin/system/configs/')) {
      const key = decodeURIComponent(path.split('/').pop())
      const found = CONFIGS.find(c => c.key === key)
      return { code: 0, data: found || { key, value: '' } }
    }
    if (path === '/api/v2/admin/system/subscription-settings') return { code: 0, data: { subscribe_path: '/s', subscribe_domains: ['sub1.example.com', 'sub2.example.com'] } }
    if (path === '/api/v5/backups' && method === 'POST') {
      return { __status: 201, body: { data: { backup: { id: 5, file: 'anix-control-20261002T080000Z-manual.tar', type: 'manual', size: 18874368, created_at: '2026-10-02T08:00:00Z', verified_at: null, verify_status: 'pending', verify_error: '' } } } }
    }
    if (path === '/api/v5/backups') return backupsAnswer(scenario)
    if (/^\/api\/v5\/backups\/\d+\/download$/.test(path)) {
      if (scenario === 'backupStaff') return { __status: 403, body: { error: { code: 'super_admin_required', message: 'only a super administrator may download a backup: it holds the root key' } } }
      return { __file: { name: 'anix-control-20261002T034107Z-daily.tar', type: 'application/x-tar', body: 'manifest.json and the bundle' } }
    }
    if (path === '/api/v2/admin/loadbalancers') return { code: 0, data: { list: scenario === 'balancerEmpty' ? [] : BALANCERS } }
    if (path === '/api/v2/admin/system/audit-logs') {
      if (scenario === 'auditError') return { code: 500, msg: '审计日志服务没有响应', data: null }
      if (scenario === 'auditEmpty') return { code: 0, data: { list: [], total: 0, page: 1, page_size: 20 } }
      return { code: 0, data: { list: AUDIT, total: 46, page: Number(query.page) || 1, page_size: 20 } }
    }
    if (path === '/api/v2/admin/system/info') {
      if (scenario === 'aboutError') return { __status: 502, body: { msg: '遥测插件没有响应' } }
      return { code: 0, data: { version: '4.1.0-rc.4', build_code: '202610020001', commit: 'a1b2c3d4e5f6', build_time: '2026-10-02T01:00:00Z' } }
    }
    return undefined
  },
  viewportOnly: ['configDialog', 'balancerDialog', 'leave'],
  scenarios: {
    general: async () => {},
    phoneList: async () => {},
    dirty: async (page) => {
      await page.locator('#settings-subscription-domains').fill('sub1.example.com\nsub2.example.com\ncdn.example.net')
      await page.locator('#settings-subscription-domains').blur()
    },
    invalid: async (page) => {
      await page.locator('#settings-subscription-domains').fill('sub1.example.com\nbad domain')
    },
    configDialog: async (page) => {
      await page.getByRole('button', { name: '添加配置项' }).click()
      await page.getByRole('dialog').getByRole('button', { name: '保存' }).click()
    },
    configError: async () => {},
    backup: async () => {},
    backupEmpty: async () => {},
    backupStale: async () => {},
    backupUnsupported: async () => {},
    backupStaff: async () => {},
    leave: async (page, { width }) => {
      await page.locator('#settings-subscription-domains').fill('sub1.example.com\ncdn.example.net')
      if (width < 834) await page.locator('[data-test="settings-back"]').click()
      else await page.locator('[data-settings-section="audit"]').click()
    },
    balancer: async () => {},
    balancerEmpty: async () => {},
    balancerDialog: async (page) => {
      await page.getByRole('button', { name: '新建负载均衡器' }).first().click()
      await page.getByRole('dialog').getByLabel('节点权重').fill('not json')
    },
    audit: async () => {},
    auditEmpty: async () => {},
    auditError: async () => {},
    about: async () => {},
    aboutError: async () => {}
  }
}
