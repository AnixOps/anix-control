const CONFIGS = [
  { key: 'app.subscribe_domains', value: '["sub1.example.com","sub2.example.com"]', description: 'Alternate subscription domains', updated_at: '2026-10-01T08:00:00Z' },
  { key: 'site.name', value: 'AnixOps', description: '站点名称', updated_at: '2026-09-28T12:00:00Z' },
  { key: 'forward.runtime_backend', value: 'nftables_ansible', description: '转发运行时后端', updated_at: '2026-09-20T09:30:00Z' },
  { key: 'forward.runtime.nodex.token', value: '********', display_value: '********', sensitive: true, has_value: true, description: 'NodeX token', updated_at: '2026-09-20T09:31:00Z' },
  { key: 'smtp.timeout_seconds', value: '30', description: '', updated_at: '2026-09-02T10:00:00Z' }
]
const BACKUPS = [
  { id: 3, filename: 'backup-20261002-0300.tar.gz', size: 18874368, status: 'completed', created_at: '2026-10-02T03:00:00Z' },
  { id: 2, filename: 'backup-20261001-0300.tar.gz', size: 18350080, status: 'completed', created_at: '2026-10-01T03:00:00Z' },
  { id: 1, filename: 'backup-20260930-0300.tar.gz', size: 0, status: 'failed', created_at: '2026-09-30T03:00:00Z' }
]
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
const STATUS = {
  config: { backend: 'nftables_ansible', nodeXMode: false },
  attachment: { model: 'stateless', description: 'Local ansible executor on the panel host' },
  runtimeReady: { ready: true, reason: 'ansible-playbook found, inventory present' },
  reachability: { ready: true, reason: 'executor reachable' },
  localAnsible: { command: 'ansible-playbook', commandFound: true, inventoryExists: true, applyPlaybookExists: true, removePlaybookExists: true, workingDirExists: true },
  warnings: ['inventory has 1 host without ansible_user']
}
const SECTION = {
  general: 'general', dirty: 'general', invalid: 'general', configDialog: 'general', configError: 'general',
  runtime: 'runtime',
  backup: 'backup', backupS3: 'backup', backupEmpty: 'backup', restoreConfirm: 'backup',
  balancer: 'balancer', balancerDialog: 'balancer', balancerEmpty: 'balancer',
  audit: 'audit', auditEmpty: 'audit', auditError: 'audit',
  about: 'about', aboutError: 'about',
  leave: 'backup'
}
export default {
  pathFor(scenario) {
    if (scenario === 'phoneList') return '/admin/system'
    return `/admin/system/${SECTION[scenario] || 'general'}`
  },
  api(path, { scenario, query }) {
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
    if (path === '/api/v2/admin/system/backup/config') {
      if (scenario === 'backupS3') return { code: 0, data: { enabled: true, interval: 24, keep_count: 7, backup_database: true, backup_files: true, storage_type: 's3', s3_bucket: 'panel-backups', s3_region: 'ap-east-1', s3_endpoint: '', s3_access_key: '********', s3_access_key_sensitive: true, s3_access_key_has_value: true, s3_secret_key_sensitive: true, s3_secret_key_has_value: true } }
      return { code: 0, data: { enabled: true, interval: 24, keep_count: 7, backup_database: true, backup_files: false, storage_type: 'local', storage_path: 'backups' } }
    }
    if (path === '/api/v2/admin/system/backups') return { code: 0, data: { list: scenario === 'backupEmpty' ? [] : BACKUPS } }
    if (path === '/api/v2/admin/system/backup/stats') return { code: 0, data: scenario === 'backupEmpty' ? { total_count: 0, total_size: 0 } : { total_count: 3, total_size: 37224448, last_backup: '2026-10-02T03:00:00Z' } }
    if (path === '/api/v2/admin/loadbalancers') return { code: 0, data: { list: scenario === 'balancerEmpty' ? [] : BALANCERS } }
    if (path === '/api/v2/admin/system/audit-logs') {
      if (scenario === 'auditError') return { code: 500, msg: '审计日志服务没有响应', data: null }
      if (scenario === 'auditEmpty') return { code: 0, data: { list: [], total: 0, page: 1, page_size: 20 } }
      return { code: 0, data: { list: AUDIT, total: 46, page: Number(query.page) || 1, page_size: 20 } }
    }
    if (path === '/api/v2/admin/forward/runtime/status') return { code: 0, data: STATUS }
    if (path === '/api/v2/admin/system/info') {
      if (scenario === 'aboutError') return { __status: 502, body: { msg: '遥测插件没有响应' } }
      return { code: 0, data: { version: '4.1.0-rc.4', build_code: '202610020001', commit: 'a1b2c3d4e5f6', build_time: '2026-10-02T01:00:00Z' } }
    }
    return undefined
  },
  viewportOnly: ['configDialog', 'balancerDialog', 'restoreConfirm', 'leave'],
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
    runtime: async () => {},
    backup: async () => {},
    backupS3: async () => {},
    backupEmpty: async () => {},
    restoreConfirm: async (page, { width }) => {
      await page.getByRole('button', { name: /backup-20261002-0300.tar.gz 的操作/ }).first().click()
      await page.getByRole('menuitem', { name: '恢复…' }).click()
    },
    leave: async (page, { width }) => {
      await page.locator('#backup-enabled').click()
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
