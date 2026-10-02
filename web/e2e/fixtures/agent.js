const AGENTS = [
  { node_id: 3, version: '2.4.1', online: true, last_seen: '2026-10-02T07:59:40Z', system: { os: 'linux', arch: 'amd64' }, capabilities: ['diagnostic', 'logs'] },
  { node_id: 7, version: '2.4.1', online: true, last_seen: '2026-10-02T07:59:12Z', system: { os: 'linux', arch: 'arm64' }, capabilities: ['diagnostic'] },
  { node_id: 12, version: '2.3.0', online: false, last_seen: '2026-09-30T21:14:00Z', system: { os: 'linux', arch: 'amd64' }, capabilities: [] }
]
const TASKS = [
  { task_id: 'tsk-9f2a', node_id: 3, action: 'service_status', success: true, duration_ms: 182, timestamp: '2026-10-02T07:40:00Z' },
  { task_id: 'tsk-81bc', node_id: 7, action: 'log_tail', success: false, duration_ms: 3004, timestamp: '2026-10-02T06:12:00Z' }
]
export default {
  path: '/admin/forward/agents',
  api(path, { scenario }) {
    if (path === '/api/v2/admin/agent/list') {
      if (scenario === 'error') return { __status: 502, body: { msg: 'agent hub unavailable' } }
      if (scenario === 'loading') return { __delay: 60000, body: { code: 0, data: { agents: [] } } }
      return { code: 0, data: { agents: scenario === 'empty' ? [] : AGENTS } }
    }
    if (path === '/api/v2/admin/agent/tasks') return { code: 0, data: scenario === 'empty' ? [] : TASKS }
    if (path === '/api/v2/admin/agent/execute') return { code: 0, data: { success: true, output: 'gost.service - active (running) since Thu 2026-10-01 08:00:00 UTC' } }
    return undefined
  },
  viewportOnly: ['menu', 'task'],
  scenarios: {
    list: async () => {},
    empty: async () => {},
    error: async () => {},
    loading: async page => { await page.waitForTimeout(500) },
    menu: async page => { await page.getByRole('button', { name: /节点 #3 的操作/ }).first().click() },
    task: async page => {
      await page.getByRole('button', { name: /节点 #3 的操作/ }).first().click()
      await page.getByRole('menuitem', { name: '下发任务' }).click()
      await page.getByRole('dialog').waitFor()
    },
    terminal: async page => {
      await page.getByRole('tab', { name: '远程终端' }).click()
    },
    tasks: async page => { await page.getByRole('tab', { name: '任务历史' }).click() }
  }
}
