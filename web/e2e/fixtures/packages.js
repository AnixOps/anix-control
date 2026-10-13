// 系统设置 → 内置包 (/admin/system/packages): the built-in packages Control
// installs, upgrades and enables at every start, with one that failed and
// one starting a new version.
const manifest = (id, version, extra = {}) => JSON.stringify({ id, version, targets: ['control'], ...extra })
const PLUGINS = [
  { id: 'identity-platform', name: 'Identity Platform', description: '账户、登录、两步验证与令牌签发。', publisher: 'AnixOps', official: true },
  { id: 'forward', name: 'Forward', description: '转发路由、节点清单与 DNS。', publisher: 'AnixOps', official: true },
  { id: 'knowledge', name: 'Knowledge', description: '帮助中心文章。', publisher: 'AnixOps', official: true },
  { id: 'machine-telemetry', name: 'Machine Telemetry', description: '采集主机 CPU、内存、磁盘与网络指标。', publisher: 'AnixOps', official: true },
  { id: 'ticket', name: 'Tickets', description: '工单。', publisher: 'AnixOps', official: true }
]
const SCHEMA = { type: 'object', properties: { interval_seconds: { type: 'integer', title: 'Interval (seconds)', minimum: 5 } } }
const RELEASES = [
  { id: 1, plugin_id: 'identity-platform', version: '4.2.1', manifest: manifest('identity-platform', '4.2.1') },
  { id: 2, plugin_id: 'forward', version: '4.2.1', manifest: manifest('forward', '4.2.1') },
  { id: 3, plugin_id: 'knowledge', version: '4.3.0', manifest: manifest('knowledge', '4.3.0') },
  { id: 4, plugin_id: 'machine-telemetry', version: '4.2.1', manifest: manifest('machine-telemetry', '4.2.1', { config_schema: SCHEMA }) },
  { id: 5, plugin_id: 'ticket', version: '4.2.1', manifest: manifest('ticket', '4.2.1') }
]
const INSTALLS = [
  { id: 11, plugin_id: 'identity-platform', target: 'control', desired_version: '4.2.1', observed_version: '4.2.1', state: 'healthy', health: 'healthy', enabled: true },
  { id: 12, plugin_id: 'forward', target: 'control', desired_version: '4.2.1', observed_version: '4.2.1', state: 'healthy', health: 'healthy', enabled: true },
  { id: 13, plugin_id: 'knowledge', target: 'control', desired_version: '4.3.0', observed_version: '4.2.1', state: 'pending', health: 'pending', enabled: true },
  { id: 14, plugin_id: 'machine-telemetry', target: 'control', desired_version: '4.2.1', observed_version: '4.2.1', state: 'healthy', health: 'healthy', enabled: true },
  { id: 15, plugin_id: 'ticket', target: 'control', desired_version: '4.2.1', observed_version: '4.2.0', state: 'failed', health: 'unhealthy', enabled: true, has_error: true, last_error: 'plugin installation failed' }
]

export default {
  path: '/admin/system/packages',
  api(path, { scenario, method }) {
    if (path === '/api/v3/plugins') {
      if (scenario === 'error') return { __status: 503, body: { error: { message: 'kernel package host is not ready' } } }
      return scenario === 'empty' ? [] : PLUGINS
    }
    if (path === '/api/v3/plugin-releases') return scenario === 'empty' ? [] : RELEASES
    if (path === '/api/v3/plugin-installations') return scenario === 'empty' ? [] : INSTALLS
    if (path === '/api/v3/plugin-installations/14/config') return { installation_id: 14, revision: 3, config: '{"interval_seconds":30}' }
    if (/^\/api\/v3\/plugin-installations\/\d+\/actions$/.test(path) && method === 'POST') {
      return { __status: 202, body: { data: { operation: { id: 'op-restart', kind: 'plugin.enable', state: 'pending' } } } }
    }
    return undefined
  },
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {}
  }
}
