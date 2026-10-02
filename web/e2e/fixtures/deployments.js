import { FIXED_NOW_MS } from './clock.js'
const nodes = [{ id: 11, name: 'Shanghai entry', host: '10.0.0.11', status: 1 }, { id: 12, name: 'Hong Kong relay', host: '10.0.0.12', status: 1 }]
const plugins = [{ id: 'gost-mesh', name: 'GOST Mesh', publisher: 'AnixOps' }, { id: 'machine-telemetry', name: 'Machine Telemetry', publisher: 'AnixOps' }]
const manifest = id => JSON.stringify({ id, version: '1.0.0', targets: ['agent'] })
const topologies = [
  { id: 21, name: 'Shanghai mesh', service_scope: 'forward', active_revision_id: 5 },
  { id: 22, name: 'Tokyo mesh', service_scope: 'forward', active_revision_id: 0 }
]
const deployments = [{ id: 31, topology_id: 21, revision_id: 5, state: 'planned' }]
const operations = [
  { id: 'op-3', kind: 'deployment.apply', plugin_id: 'gost-mesh', node_id: 11, target_version: '1.0.0', revision: 6, state: 'running', operation_chain: 'op-2,op-3', created_at: new Date(FIXED_NOW_MS - 120000).toISOString() },
  { id: 'op-2', kind: 'plugin.enable', plugin_id: 'gost-mesh', target: 'control', target_version: '1.0.0', revision: 5, state: 'succeeded', created_at: new Date(FIXED_NOW_MS - 3600000).toISOString() },
  { id: 'op-1', kind: 'plugin.install', plugin_id: 'machine-telemetry', node_id: 12, target_version: '4.0.0', revision: 1, state: 'failed', last_error: 'agent package artifact is missing', created_at: new Date(FIXED_NOW_MS - 86400000).toISOString() }
]
const assignments = [
  { id: 7, node_id: 11, service_scope: 'forward', plugin_id: 'gost-mesh', role: 'relay', desired_version: '1.0.0', desired_config_revision: 6, rollout_group: 'canary-a', enabled: true },
  { id: 8, node_id: 11, service_scope: 'monitoring', plugin_id: 'machine-telemetry', role: 'telemetry', desired_version: '4.0.0', desired_config_revision: 1, rollout_group: '', enabled: false }
]
const revisionDetail = {
  revision: { id: 5, revision: 3, message: 'add exit' },
  vertices: [
    { key: 'entry', kind: 'agent', node_id: 11, plugin_id: 'gost-mesh', role: 'tunnel_entry', config: '{}' },
    { key: 'relay', kind: 'agent', node_id: 12, plugin_id: 'gost-mesh', role: 'relay', config: '{}' },
    { key: 'exit', kind: 'external', role: 'overseas_exit', config: '{}' }
  ],
  edges: [
    { source_key: 'entry', target_key: 'relay', protocol: 'tls', config: '{}' },
    { source_key: 'relay', target_key: 'exit', protocol: 'ws', config: '{}' }
  ]
}
export default {
  path: '/admin/deployments',
  settle: 800,
  api(path, { scenario, method }) {
    if (path === '/api/v3/topologies') {
      if (scenario === 'error') return { __status: 502, body: { error: { message: 'kernel unavailable' } } }
      if (scenario === 'loading') return { __delay: 60000, body: [] }
      return scenario === 'empty' ? [] : topologies
    }
    if (path === '/api/v3/deployments') return scenario === 'empty' ? [] : deployments
    if (path === '/api/v3/operations') return scenario === 'empty' ? [] : operations
    if (path === '/api/v3/service-scopes') return [{ id: 'forward', name: 'Forward' }, { id: 'monitoring', name: 'Monitoring' }]
    if (path === '/api/v2/admin/nodes') return { code: 0, data: { list: scenario === 'empty' ? [] : nodes, total: 2 } }
    if (path === '/api/v3/plugins') return plugins
    if (path === '/api/v3/plugin-releases') return [{ id: 3, plugin_id: 'gost-mesh', version: '1.0.0', manifest: manifest('gost-mesh') }, { id: 4, plugin_id: 'machine-telemetry', version: '4.0.0', manifest: manifest('machine-telemetry') }]
    if (path === '/api/v3/plugin-installations') return [{ id: 5, plugin_id: 'gost-mesh', target: 'agent', desired_version: '1.0.0', config_revision: 6, state: 'healthy', enabled: true }, { id: 6, plugin_id: 'machine-telemetry', target: 'agent', desired_version: '4.0.0', config_revision: 1, state: 'healthy', enabled: true }]
    if (/^\/api\/v3\/nodes\/\d+\/assignments$/.test(path) && method === 'GET') return assignments
    if (path === '/api/v3/topologies/21/revisions') return [{ id: 5, revision: 3 }, { id: 4, revision: 2 }]
    if (/^\/api\/v3\/topologies\/21\/revisions\/\d+$/.test(path)) return revisionDetail
    if (path === '/api/v3/deployments/31') return { deployment: { id: 31, state: 'planned' }, operations: [] }
    return undefined
  },
  viewportOnly: ['sheet', 'workspace', 'graph'],
  scenarios: {
    topologies: async () => {},
    targets: async page => { await page.getByTestId('deployment-targets').click(); await page.waitForTimeout(300) },
    timeline: async page => { await page.getByTestId('show-all-activity').click() },
    sheet: async page => { await page.getByTestId('deployment-targets').click(); await page.waitForTimeout(300); await page.getByTestId('new-assignment').click(); await page.getByTestId('assignment-drawer').waitFor() },
    workspace: async page => { await page.getByTestId('edit-topology-21').click(); await page.getByTestId('topology-workspace').waitFor(); await page.waitForTimeout(300) },
    graph: async page => {
      await page.getByTestId('edit-topology-21').click()
      await page.getByTestId('topology-workspace').waitFor()
      await page.waitForTimeout(300)
      await page.getByTestId('topology-view-switch').getByRole('button').nth(1).click()
      await page.waitForTimeout(1500)
    },
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) }
  }
}
