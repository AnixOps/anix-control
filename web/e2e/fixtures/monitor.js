import { nodeTrafficAnswer } from './nodeTraffic.js'

const NODES = [
  { id: 1, name: 'hk-01', host: 'hk-01.example.net', status: 'online', cpu_usage: 23.4, memory_usage: 61.2, disk_usage: 40.1, online_users: 128, uptime: 1296000 },
  { id: 2, name: 'tokyo-02', host: 'tokyo-02.example.net', status: 'online', cpu_usage: 81.7, memory_usage: 77.5, disk_usage: 92.3, online_users: 96, uptime: 86400 * 3 + 7200 },
  { id: 3, name: 'sg-edge-03', host: '203.0.113.30', status: 'offline', cpu_usage: 0, memory_usage: 0, disk_usage: 55, online_users: 0, uptime: 0 },
  { id: 4, name: 'fra-04', host: 'fra-04.example.net', status: 'pending', online_users: 0 },
  { id: 5, name: 'la-05', host: 'la-05.example.net', status: 'online', cpu_usage: 12, memory_usage: 34, disk_usage: 20, online_users: 118, uptime: 3600 * 20 }
]
export default {
  path: '/admin/monitor',
  // A node's traffic history (the sheet a row opens).
  api(path, { query }) {
    const m = path.match(/^\/api\/v4\/kernel\/nodes\/(\d+)\/traffic$/)
    return m ? nodeTrafficAnswer(m[1], query) : undefined
  },
  async after(page, { scenario }) {
    if (scenario !== 'traffic') return
    await page.getByText('hk-01', { exact: true }).first().click()
    await page.getByTestId('monitor-node-traffic').waitFor()
    await page.getByTestId('node-traffic-chart').waitFor()
  },
  async setup(page, { scenario }) {
    await page.routeWebSocket(/\/ws\/monitor/, ws => {
      if (scenario === 'offline') { ws.close(); return }
      const nodes = scenario === 'empty' ? [] : NODES
      ws.send(JSON.stringify({ type: 'snapshot', data: { overview: { total_nodes: nodes.length, online_nodes: 3, offline_nodes: 1, pending_nodes: 1, total_upload: 12.4e12, total_download: 48.9e12 }, nodes } }))
    })
  },
  scenarios: { live: async () => {}, traffic: async () => {}, empty: async () => {}, offline: async page => { await page.waitForTimeout(300) } }
}
