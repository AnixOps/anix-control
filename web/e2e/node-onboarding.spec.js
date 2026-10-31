import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { openScreen } from './support/screens.js'

// 接入节点 (docs/design/node-onboarding, "The Administrator's Flow"), against
// the mocked routes of e2e/fixtures/nodes.js: one entry point on the node
// page, the command of a node group with its mirrors, the machines that
// joined, the node groups tab and the approval of nodes that wait.
test.use({ timezoneId: 'UTC', locale: 'en-US', reducedMotion: 'reduce' })

function recordRequests(page) {
  const calls = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/')) return
    calls.push({ method: request.method(), path: url.pathname, query: Object.fromEntries(url.searchParams), body: request.postData() })
  })
  return { calls, to: (path, method = 'GET') => calls.filter(call => call.path === path && call.method === method) }
}

async function axeFindings(page, include) {
  // A sheet that is still sliding in reads as a different colour.
  await page.waitForTimeout(600)
  const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'])
  if (include) builder.include(include)
  const results = await builder.analyze()
  return results.violations
    .filter(violation => violation.impact === 'serious' || violation.impact === 'critical')
    .map(violation => `${violation.id} (${violation.impact}): ${violation.nodes.slice(0, 3).map(node => node.target.join(' ')).join(' | ')}`)
}

test.describe('the node page has one entry point', () => {
  test('Join a node replaces the registration key, the parent deployment and Add node', async ({ page }) => {
    await openScreen(page, 'admin-nodes', { clock: true })
    const header = page.getByRole('main')
    await expect(header.getByRole('button', { name: 'Join a node' })).toBeVisible()
    for (const gone of ['Registration key', 'Deploy parent nodes', 'Add node']) {
      await expect(header.getByRole('button', { name: gone })).toHaveCount(0)
    }
    await expect(page.getByTestId('approval-banner')).toContainText('1 machine(s) wait for your approval')
  })

  test('shows the default group’s command, asking for nothing, and the readiness check came first', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-join', { clock: true })
    const dialog = page.getByRole('dialog', { name: 'Join a node' })
    await expect(dialog.getByTestId('join-command-text')).toContainText('-u https://panel.example.com -t anixjoin_')
    await expect(dialog.getByTestId('join-waiting')).toContainText('Waiting for a machine to join…')
    await expect(dialog.getByTestId('join-uninstall-command')).toContainText('sudo anix-agent uninstall')

    const order = requests.calls.map(call => call.path)
    expect(order.indexOf('/api/v4/kernel/agents/install-readiness')).toBeGreaterThanOrEqual(0)
    expect(order.indexOf('/api/v4/kernel/agents/install-readiness')).toBeLessThan(order.indexOf('/api/v4/kernel/node-groups/1/command'))

    await dialog.getByRole('button', { name: 'GitHub' }).click()
    await expect(dialog.getByTestId('join-command-text')).toContainText('--mirror github')
    expect(await axeFindings(page, '[data-testid="node-join-sheet"]')).toEqual([])
  })

  test('lists the machines that joined, and approves the one that waits', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-joined', { clock: true })
    const dialog = page.getByRole('dialog', { name: 'Join a node' })
    const pending = dialog.locator('[data-node="proxy-112"]')
    await expect(pending).toContainText('au-syd-01 joined')
    await expect(pending).toContainText('Awaiting approval')
    await expect(dialog.locator('[data-node="proxy-101"]')).toContainText('Approved')

    await pending.getByRole('button', { name: 'Approve' }).click()
    await expect.poll(() => requests.to('/api/v4/kernel/nodes/proxy-112/approve', 'POST')).toHaveLength(1)
  })

  test('shows what to fix instead of a command when the Control cannot take nodes', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-join-blocked', { clock: true })
    const dialog = page.getByRole('dialog', { name: 'Join a node' })
    const blocked = dialog.getByTestId('join-blocked')
    await expect(blocked.getByRole('listitem')).toHaveCount(2)
    await expect(blocked).toContainText('Control’s address')
    await expect(blocked).toContainText('Key that signs node certificates')
    await expect(dialog.getByTestId('join-command-text')).toHaveCount(0)
    // Nothing is created or read for a command that would fail.
    expect(requests.to('/api/v4/kernel/node-groups/default', 'POST')).toHaveLength(0)
    expect(requests.calls.filter(call => call.path.endsWith('/command'))).toHaveLength(0)
    expect(await axeFindings(page, '[data-testid="node-join-sheet"]')).toEqual([])
  })
})

test.describe('node groups', () => {
  test('lists the groups with their nodes and approval, and edits the defaults', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-groups', { clock: true })
    const panel = page.getByTestId('node-groups-panel')
    await expect(panel.getByRole('row', { name: /HK/ })).toContainText('1 awaiting')
    await expect(panel.getByRole('row', { name: /edge-relays/ })).toContainText('Expired')
    await expect(panel.getByRole('row', { name: /JP/ })).toContainText('Ends 2026-10-23')

    await page.getByRole('button', { name: 'Actions for JP' }).click()
    await page.getByRole('menuitem', { name: 'Edit' }).click()
    const sheet = page.getByRole('dialog', { name: 'Edit node group' })
    await expect(sheet.getByTestId('template-editor')).toHaveCount(0)
    await sheet.getByTestId('group-name').fill('JP 2')
    await sheet.getByTestId('save-group').click()
    await expect.poll(() => requests.to('/api/v4/kernel/node-groups/2', 'PUT')).toHaveLength(1)
    const body = JSON.parse(requests.to('/api/v4/kernel/node-groups/2', 'PUT')[0].body)
    expect(body.name).toBe('JP 2')
    expect(body).not.toHaveProperty('protocol_template')
    expect(body).not.toHaveProperty('subscription_group_ids')
  })

  test('the edit sheet has no serious accessibility findings', async ({ page }) => {
    await openScreen(page, 'admin-node-group-edit', { clock: true })
    expect(await axeFindings(page, '[data-testid="node-group-sheet"]')).toEqual([])
  })
})

test.describe('nodes that wait for approval', () => {
  test('are one chip away and carry Approve and Reject on their row', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-nodes', { clock: true })
    await page.getByTestId('approval-banner-view').click()
    await expect(page).toHaveURL(/status=awaiting/)
    await expect.poll(() => requests.to('/api/v4/admin/nodes').at(-1)?.query.approval).toBe('pending')
    const row = page.getByRole('row', { name: /au-syd-01/ })
    await expect(row.getByTestId('node-approval-pending')).toHaveText('Awaiting approval')

    await page.getByRole('button', { name: 'Actions for au-syd-01' }).click()
    await expect(page.getByRole('menuitem', { name: 'Approve' })).toBeVisible()
    await expect(page.getByRole('menuitem', { name: 'Reject' })).toBeVisible()
  })
})
