import nodes from './nodes.js'
import { FIXED_NOW_MS } from './clock.js'
import { agentUpgradePlanAnswer } from './nodeDiagnostics.js'

// 升级节点: the sheet over the node list, opened with the header button. `upgrade` is the
// start form with no campaign yet; `upgradeRunning` the form disabled above
// a running campaign.
const RUNNING = {
  id: 'c-2026-10-02',
  target_version: 'v4.2.0',
  status: 'running',
  current_batch: 1,
  batch_ends_at: new Date(FIXED_NOW_MS + 24 * 60_000).toISOString(),
  finished_at: null,
  status_reason: '',
  error_code: '',
  total: 12,
  batches: [
    { index: 0, percent: 5, min_duration_seconds: 1800, nodes: 1, states: { succeeded: 1 }, offered: 1, failed: 0 },
    { index: 1, percent: 25, min_duration_seconds: 1800, nodes: 2, states: { succeeded: 1, upgrading: 1 }, offered: 2, failed: 0 },
    { index: 2, percent: 100, min_duration_seconds: 1800, nodes: 9, states: { pending: 9 }, offered: 0, failed: 0 }
  ]
}

export default {
  ...nodes,
  api(path, ctx) {
    const running = ctx.scenario === 'upgradeRunning'
    if (path === '/api/v4/kernel/agents/upgrade-plan') {
      return agentUpgradePlanAnswer(running ? { active_campaign_id: RUNNING.id } : {})
    }
    if (path === '/api/v4/kernel/agents/upgrades' && ctx.method === 'GET') {
      return { data: { campaigns: running ? [RUNNING] : [] } }
    }
    return nodes.api(path, ctx)
  },
  async after(page) {
    await page.getByTestId('upgrade-nodes').click()
    await page.getByTestId('agent-upgrade-form').waitFor()
    // The campaign card (loaded apart from the plan).
    await page.locator('[data-testid="agent-upgrade-empty"], [data-testid="agent-upgrade-campaign"]').first().waitFor()
  }
}
