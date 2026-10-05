import { test, expect } from '@playwright/test'
import { openScreen } from './support/screens.js'

// The admin console calls the kernel's admin routes (docs/guide/api-reference.md):
// server-side sort of the user, order and node lists, last online of the
// users in view, one bulk request for the users and invite-code tables, the
// members of a subscription group, and the Agent connection and certificate
// of the nodes in view. The API is mocked (e2e/fixtures); each test records
// the requests the page makes and checks what it asked for.
function recordRequests(page) {
  const calls = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/')) return
    calls.push({ method: request.method(), path: url.pathname, query: Object.fromEntries(url.searchParams), headers: request.headers(), body: request.postData() })
  })
  return {
    calls,
    to: (path, method = 'GET') => calls.filter(call => call.path === path && call.method === method)
  }
}

const firstRowText = page => page.locator('tbody tr').first().innerText()

test.describe('users', () => {
  test('a sort header sorts the whole list on the server, from page 1, and survives a reload', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-users')
    // Only the columns the API sorts by are sortable: the derived status and the plan are plain headers.
    await expect(page.getByRole('columnheader', { name: 'Status' }).getByRole('button')).toHaveCount(0)
    await expect(page.getByRole('columnheader', { name: 'Subscription template' }).getByRole('button')).toHaveCount(0)
    const traffic = page.getByRole('columnheader', { name: /^Used \/ total/ })

    await traffic.getByRole('button').click()
    await expect(traffic).toHaveAttribute('aria-sort', 'descending')
    expect(requests.to('/api/v2/admin/users').at(-1).query).toMatchObject({ sort: 'traffic', order: 'desc', page: '1' })
    await expect(page).toHaveURL(/sort=traffic&order=desc/)
    // The heaviest user comes first now; lin.xiao (no traffic) was first before.
    expect(await firstRowText(page)).not.toContain('lin.xiao@example.com')

    await traffic.getByRole('button').click()
    await expect(traffic).toHaveAttribute('aria-sort', 'ascending')
    expect(requests.to('/api/v2/admin/users').at(-1).query).toMatchObject({ sort: 'traffic', order: 'asc' })
    await expect.poll(() => firstRowText(page)).toContain('lin.xiao@example.com')

    await page.reload({ waitUntil: 'networkidle' })
    await expect(page.getByRole('columnheader', { name: /^Used \/ total/ })).toHaveAttribute('aria-sort', 'ascending')
    expect(requests.to('/api/v2/admin/users').at(-1).query).toMatchObject({ sort: 'traffic', order: 'asc' })

    // The third click is no sort: neither parameter is sent and the URL is clean.
    await page.getByRole('columnheader', { name: /^Used \/ total/ }).getByRole('button').click()
    await expect(page.getByRole('columnheader', { name: /^Used \/ total/ })).toHaveAttribute('aria-sort', 'none')
    const last = requests.to('/api/v2/admin/users').at(-1).query
    expect(last).not.toHaveProperty('sort')
    expect(last).not.toHaveProperty('order')
    await expect(page).not.toHaveURL(/sort=/)
  })

  test('last online comes in one call for the page after the list: a relative time, or Never', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-users', { clock: true })
    await expect(page.getByRole('columnheader', { name: 'Last online' })).toBeVisible()
    await expect(page.getByTestId('last-online-never').first()).toHaveText('Never')
    await expect(page.getByRole('row', { name: /lin\.xiao@example\.com/ })).toContainText('2 minutes ago')
    await expect(page.getByRole('row', { name: /chen\.jie@example\.com/ })).toContainText('3 hours ago')
    const activity = requests.to('/api/v4/admin/users/activity')
    expect(activity).toHaveLength(1)
    // The ids of the 14 users on the page, no more.
    expect(activity[0].query.ids).toBe(Array.from({ length: 14 }, (_, index) => 100 + index).join(','))
    // The user detail shows it too.
    await page.getByRole('row', { name: /wang\.fang/ }).click()
    await expect(page.getByTestId('user-last-online')).toContainText('7 minutes ago')
  })

  test('the bulk bar bans the selection with one request and resets traffic with one key', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-users')
    // The row checkboxes (the header's is "Select all rows on this page").
    const boxes = page.getByRole('checkbox', { name: /^Select (?!all)/ })
    await boxes.nth(0).click()
    await boxes.nth(1).click()
    const bar = page.getByRole('region', { name: 'Actions for the selected rows' })
    await expect(bar).toContainText('2 selected')

    await bar.getByRole('button', { name: 'Ban', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: '2 users banned' }).first()).toBeVisible()
    const bans = requests.to('/api/v4/admin/users/bulk', 'POST')
    expect(bans).toHaveLength(1)
    expect(JSON.parse(bans[0].body)).toEqual({ action: 'ban', ids: [100, 101] })
    // One request, not one per user.
    expect(requests.calls.filter(call => /\/admin\/users\/\d+\/ban$/.test(call.path))).toHaveLength(0)

    await boxes.nth(0).click()
    await bar.getByRole('button', { name: 'Reset traffic' }).click()
    const dialog = page.getByRole('alertdialog', { name: 'Reset the traffic of 1 users?' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Reset traffic' }).click()
    await expect(dialog).toBeHidden()
    const resets = requests.to('/api/v4/admin/users/bulk', 'POST').filter(call => JSON.parse(call.body).action === 'reset_traffic')
    expect(resets).toHaveLength(1)
    expect(resets[0].headers['idempotency-key']).toMatch(/^ub-/)
  })
})

test.describe('invite codes', () => {
  test('revoking the selection is one bulk request after one confirmation', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-invite-codes')
    const boxes = page.getByRole('checkbox', { name: /^Select (?!all)/ })
    await boxes.nth(0).click()
    await boxes.nth(2).click()
    await page.getByRole('region', { name: 'Actions for the selected rows' }).getByRole('button', { name: 'Revoke' }).click()
    const dialog = page.getByRole('alertdialog', { name: 'Revoke 2 invite codes?' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Revoke' }).click()
    await expect(page.getByRole('status').filter({ hasText: '2 codes revoked' }).first()).toBeVisible()
    const bulk = requests.to('/api/v4/admin/invite-codes/bulk', 'POST')
    expect(bulk).toHaveLength(1)
    expect(JSON.parse(bulk[0].body)).toEqual({ action: 'revoke', ids: [200, 198] })
    expect(requests.calls.filter(call => call.method === 'DELETE')).toHaveLength(0)
  })
})

test.describe('nodes', () => {
  test('the Agent connection and certificate of the page come in one call after the list', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-nodes', { clock: true })
    const chips = page.getByTestId('node-connection')
    await expect(chips.first()).toBeVisible()
    const transports = requests.to('/api/v4/kernel/agents/transports')
    expect(transports).toHaveLength(1)
    expect(transports[0].query.node).toBe(Array.from({ length: 12 }, (_, index) => `proxy-${101 + index}`).join(','))
    const row = name => page.getByRole('row', { name: new RegExp(name) })
    await expect(row('hk-01').getByTestId('node-connection')).toHaveText('mTLS stream')
    await expect(row('hk-01').getByTestId('node-certificate')).toContainText('Valid')
    await expect(row('hk-02').getByTestId('node-certificate')).toContainText('Renewal overdue')
    await expect(row('jp-tokyo-01').getByTestId('node-connection')).toHaveText('API key stream')
    await expect(row('sg-01').getByTestId('node-connection')).toHaveText('Legacy')
    await expect(row('us-lax-01').getByTestId('node-certificate')).toContainText('Expired')
    await expect(row('de-fra-01').getByTestId('node-connection')).toHaveText('Third-party')
    await expect(row('kr-sel-01').getByTestId('node-certificate')).toContainText('Revoked')
    await expect(row('au-syd-01').getByTestId('node-connection')).toHaveText('Offline')
  })

  test('a sort header sorts on the server; the status header stays plain', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-nodes')
    await expect(page.getByRole('columnheader', { name: 'Status' }).getByRole('button')).toHaveCount(0)
    await expect(page.getByRole('columnheader', { name: 'Connection' }).getByRole('button')).toHaveCount(0)
    await page.getByRole('columnheader', { name: /^Last heartbeat/ }).getByRole('button').click()
    await expect(page).toHaveURL(/sort=last_check_at&order=desc/)
    expect(requests.to('/api/v2/admin/nodes').at(-1).query).toMatchObject({ sort: 'last_check_at', order: 'desc', page: '1' })
  })

  test('the node page shows the connection, the certificate dates and the renewal', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-node-detail', { clock: true })
    const group = page.getByTestId('node-agent')
    await expect(group.getByTestId('node-connection')).toHaveText('mTLS stream')
    await expect(group.getByTestId('node-certificate-state')).toHaveText('Valid')
    await expect(group.getByTestId('node-certificate-not-after')).toBeVisible()
    await expect(group.getByTestId('node-certificate-renew-after')).toBeVisible()
    const transports = requests.to('/api/v4/kernel/agents/transports')
    expect(transports).toHaveLength(1)
    expect(transports[0].query.node).toBe('proxy-108')
  })
})

test.describe('subscription group members', () => {
  test('a paged, searchable table of the users granted the group directly', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-subscription-members', { clock: true })
    const table = page.getByRole('table', { name: 'Group members' })
    await expect(table.getByRole('row', { name: /lin\.xiao@example\.com/ })).toBeVisible()
    expect(requests.to('/api/v4/admin/subscription-groups/1/members')[0].query).toMatchObject({ page: '1', page_size: '20' })
    // No credential column.
    await expect(table).not.toContainText(/token|uuid/i)

    await page.getByRole('button', { name: 'Next page' }).click()
    await expect(table.getByRole('row', { name: /fan\.hui@example\.com/ })).toBeVisible()
    expect(requests.to('/api/v4/admin/subscription-groups/1/members').at(-1).query).toMatchObject({ page: '2' })

    await page.getByRole('searchbox', { name: 'Search by email' }).fill('chen')
    await page.keyboard.press('Enter')
    await expect(table.getByRole('row', { name: /chen\.jie@example\.com/ })).toBeVisible()
    await expect(table.locator('tbody tr')).toHaveCount(1)
    expect(requests.to('/api/v4/admin/subscription-groups/1/members').at(-1).query).toMatchObject({ q: 'chen', page: '1' })

    await page.getByRole('searchbox', { name: 'Search by email' }).fill('')
    await page.keyboard.press('Enter')
    await page.getByRole('group', { name: 'Filter by membership' }).getByRole('button', { name: 'Expired' }).click()
    await expect.poll(() => requests.to('/api/v4/admin/subscription-groups/1/members').at(-1).query.status).toBe('expired')
    await expect(table.getByText('Expired').first()).toBeVisible()
  })

  test('shows member cards on a phone, with no sideways scrolling', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openScreen(page, 'admin-subscription-members', { clock: true })
    await expect(page.getByText('lin.xiao@example.com').first()).toBeVisible()
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(overflow).toBeLessThanOrEqual(0)
  })
})
