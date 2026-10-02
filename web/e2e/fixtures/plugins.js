const manifest = (id, version, targets) => JSON.stringify({ id, version, targets })
const PLUGINS = [
  { id: 'machine-telemetry', name: 'Machine Telemetry', description: '采集主机 CPU、内存、磁盘与网络指标，并在控制面展示实时图表。', publisher: 'AnixOps', official: true },
  { id: 'identity-platform', name: 'Identity Platform', description: '账户、登录、两步验证与令牌签发。', publisher: 'AnixOps', official: true },
  { id: 'protocol-runtime', name: 'Protocol Runtime', description: '在 Agent 上运行代理协议，按节点分配。', publisher: 'AnixOps', official: true },
  { id: 'billing-bridge', name: 'Billing Bridge', description: '把订单与支付事件同步到外部账务系统。', publisher: 'Example Labs', official: false },
  { id: 'geo-routing', name: 'Geo Routing', description: '按地区选择最近的转发出口。', publisher: 'AnixOps', official: true }
]
const RELEASES = [
  { id: 1, plugin_id: 'machine-telemetry', version: '4.0.0', manifest: manifest('machine-telemetry', '4.0.0', ['control']) },
  { id: 2, plugin_id: 'identity-platform', version: '4.0.0', manifest: manifest('identity-platform', '4.0.0', ['control']) },
  { id: 3, plugin_id: 'protocol-runtime', version: '1.2.0', manifest: manifest('protocol-runtime', '1.2.0', ['control', 'agent']) },
  { id: 4, plugin_id: 'protocol-runtime', version: '1.1.0', manifest: manifest('protocol-runtime', '1.1.0', ['control', 'agent']) },
  { id: 5, plugin_id: 'billing-bridge', version: '0.3.0', manifest: manifest('billing-bridge', '0.3.0', ['control']) },
  { id: 6, plugin_id: 'geo-routing', version: '2.0.0', manifest: manifest('geo-routing', '2.0.0', ['agent']) }
]
const INSTALLS = [
  { id: 11, plugin_id: 'machine-telemetry', target: 'control', desired_version: '4.0.0', observed_version: '4.0.0', state: 'healthy', enabled: true },
  { id: 12, plugin_id: 'identity-platform', target: 'control', desired_version: '4.0.0', observed_version: '4.0.0', state: 'healthy', enabled: true },
  { id: 13, plugin_id: 'protocol-runtime', target: 'control', desired_version: '1.1.0', observed_version: '1.1.0', state: 'healthy', enabled: true },
  { id: 14, plugin_id: 'protocol-runtime', target: 'agent', desired_version: '1.1.0', observed_version: '1.0.0', state: 'failed', enabled: true, last_error: 'agent 3 did not report the new version' }
]
const OPS = [
  { id: 'op-2', kind: 'plugin.update', plugin_id: 'protocol-runtime', target: 'agent', target_version: '1.1.0', state: 'running', created_at: '2026-10-02T08:00:00Z' },
  { id: 'op-1', kind: 'plugin.enable', plugin_id: 'machine-telemetry', target: 'control', target_version: '4.0.0', state: 'succeeded', created_at: '2026-10-01T08:00:00Z' }
]

export default {
  path: '/admin/plugins',
  api(path, { scenario }) {
    if (path === '/api/v3/plugins') {
      if (scenario === 'error') return { __status: 503, body: { error: { message: 'kernel package host is not ready' } } }
      if (scenario === 'loading') return { __delay: 60000, body: [] }
      return scenario === 'empty' ? [] : PLUGINS
    }
    if (path === '/api/v3/plugin-releases') return scenario === 'empty' ? [] : RELEASES
    if (path === '/api/v3/plugin-installations') return scenario === 'empty' ? [] : INSTALLS
    if (path === '/api/v3/operations') return scenario === 'empty' ? [] : OPS
    return undefined
  },
  viewportOnly: ['sheet', 'install'],
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    sheet: async (page) => {
      await page.getByTestId('plugin-row-protocol-runtime').click()
      await page.getByTestId('plugin-detail-drawer').waitFor()
    },
    install: async (page) => {
      await page.getByTestId('plugin-row-geo-routing').click()
      await page.getByTestId('plugin-detail-drawer').locator('[data-action="install"]').click()
      await page.getByTestId('plugin-installation-dialog').waitFor()
    }
  }
}
