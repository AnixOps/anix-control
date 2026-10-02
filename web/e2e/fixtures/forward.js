import { systemConfig, TUNNELS } from './forwardCommon.js'
const G = 1024 ** 3
const FORWARDS = [
  { id: 11, userId: 1, userName: 'admin@example.com', name: 'HK-Web-01', tunnelId: 1, tunnelName: 'hk-port-01', inIp: '203.0.113.10', inPort: 10443, remoteAddr: 'web.example.com:443', strategy: 'fifo', status: 1, runtimeBackend: 'nftables_ansible', runtimeStatus: 2, inFlow: 12.4 * G, outFlow: 81.2 * G, inx: 1 },
  { id: 12, userId: 1, userName: 'admin@example.com', name: 'HK-API-pool', tunnelId: 1, tunnelName: 'hk-port-01', inIp: '203.0.113.10', inPort: 10444, remoteAddr: '10.0.0.11:8443,10.0.0.12:8443,10.0.0.13:8443', strategy: 'round', status: 1, runtimeBackend: 'nftables_ansible', runtimeStatus: 2, inFlow: 2.1 * G, outFlow: 9.6 * G, inx: 2 },
  { id: 13, userId: 2, userName: 'lin@example.test', name: 'JP-Game', tunnelId: 2, tunnelName: 'jp-port-02', inIp: '198.51.100.24,198.51.100.25', inPort: 21000, remoteAddr: 'game.example.net:7000', strategy: 'fifo', status: 0, inFlow: 300 * 1024 ** 2, outFlow: 1.2 * G, inx: 3 },
  { id: 14, userId: 2, userName: 'lin@example.test', name: 'JP-Stream-backup', tunnelId: 2, tunnelName: 'jp-port-02', inIp: '198.51.100.24', inPort: 21001, remoteAddr: '[2001:db8::10]:443', strategy: 'fifo', status: 1, runtimeBackend: 'nftables_ansible', runtimeStatus: 3, runtimeMessage: 'ansible apply failed: host unreachable', inFlow: 0, outFlow: 0, inx: 4 },
  { id: 15, userId: 3, userName: 'ops@example.org', name: 'SG-Mail-relay', tunnelId: 1, tunnelName: 'hk-port-01', inIp: '203.0.113.10', inPort: 10587, remoteAddr: 'mail.example.org:587', strategy: 'fifo', status: 1, runtimeBackend: 'nftables_ansible', runtimeStatus: 0, inFlow: 5 * 1024 ** 2, outFlow: 9 * 1024 ** 2, inx: 5 }
]
const DIAG = {
  forwardName: 'HK-Web-01', timestamp: Date.UTC(2026, 9, 2, 6, 30),
  results: [
    { success: true, description: '面板 → 入口 203.0.113.10:10443', nodeName: 'relay-exec-hk-01', nodeId: 11, targetIp: '203.0.113.10', targetPort: 10443, averageTime: 18.4, packetLoss: 0, message: '' },
    { success: true, description: '入口 → 目标 web.example.com:443', nodeName: 'relay-exec-hk-01', nodeId: 11, targetIp: 'web.example.com', targetPort: 443, averageTime: 72.1, packetLoss: 0.5, message: '' },
    { success: false, description: '入口 → 备用目标', nodeName: 'relay-exec-hk-01', nodeId: 11, targetIp: '10.0.0.13', targetPort: 8443, message: 'connection refused' }
  ]
}
export default {
  path: '/admin/forward',
  edition: 'community',
  api(path, { scenario }) {
    const cfg = systemConfig(path); if (cfg) return cfg
    if (path === '/api/v2/forward/list') {
      if (scenario === 'error') return { __status: 502, body: { msg: 'upstream unavailable' } }
      if (scenario === 'loading') return { __delay: 60000, body: { code: 0, data: [] } }
      return { code: 0, data: scenario === 'empty' || scenario === 'grouped-empty' ? [] : FORWARDS }
    }
    if (path === '/api/v2/tunnel/user/tunnel') return { code: 0, data: TUNNELS }
    if (path === '/api/v2/forward/diagnose') return { code: 0, data: DIAG }
    return undefined
  },
  viewportOnly: ['tooltip', 'menu', 'editor', 'diagnosis', 'bulk', 'import', 'export', 'address', 'pagemenu'],
  scenarios: {
    list: async () => {},
    tooltip: async (page, { width }) => { if (width < 640) return; const b = page.locator('[data-test="forward-status-detail"]').nth(2); await b.hover(); await page.locator('.rule-status__tooltip').waitFor(); console.log('tooltip:', await page.locator('.rule-status__tooltip').textContent()) },
    grouped: async page => { await page.getByRole('button', { name: '分组' }).first().click(); await page.waitForTimeout(300) },
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    filtered: async page => { await page.getByRole('search').locator('input').fill('nothing-matches'); await page.waitForTimeout(200) },
    menu: async page => { await page.getByRole('button', { name: /HK-Web-01 的操作/ }).first().click() },
    pagemenu: async page => { await page.getByRole('button', { name: '更多操作' }).first().click() },
    bulk: async page => { await page.getByRole('checkbox', { name: '选择 HK-Web-01' }).first().click(); await page.getByRole('checkbox', { name: '选择 JP-Game' }).first().click() },
    editor: async page => { await page.getByRole('button', { name: /HK-API-pool 的操作/ }).first().click(); await page.getByRole('menuitem', { name: '编辑' }).click(); await page.getByRole('dialog').waitFor() },
    'editor-errors': async page => { await page.getByRole('button', { name: '新增' }).first().click(); await page.getByRole('dialog').waitFor(); await page.getByRole('button', { name: '创建转发' }).click() },
    diagnosis: async page => { await page.getByRole('button', { name: /HK-Web-01 的操作/ }).first().click(); await page.getByRole('menuitem', { name: '诊断' }).click(); await page.getByRole('dialog').waitFor(); await page.waitForTimeout(300) },
    import: async page => { await page.getByRole('button', { name: '更多操作' }).first().click(); await page.getByRole('menuitem', { name: '导入' }).click(); await page.getByRole('dialog').waitFor() },
    export: async page => { await page.getByRole('checkbox', { name: '选择 HK-Web-01' }).first().click(); await page.locator('[data-bulk-bar] [data-test="forward-bulk-export"]').click(); await page.getByRole('dialog').waitFor() },
    address: async (page, { width }) => { if (width < 640) { await page.getByRole('button', { name: /HK-API-pool/ }).first().click(); await page.getByRole('dialog').waitFor(); return } await page.getByRole('button', { name: /复制目标地址：10.0.0.11/ }).first().click(); await page.getByRole('dialog').waitFor() }
  }
}
