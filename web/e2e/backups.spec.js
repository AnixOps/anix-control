import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { openScreen } from './support/screens.js'
import { expectToast } from './support/toasts.js'

// 系统设置 → 备份 (docs/reference/backups.md): the bundles Control keeps with
// their verification, the last verified backup, 立即备份, the download (a
// super administrator only) and the restore commands; there is no restore
// in the console. The API is mocked (e2e/fixtures/system.js).
test.use({ timezoneId: 'UTC', locale: 'en-US' })

function recordRequests(page) {
  const calls = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (url.pathname.startsWith('/api/')) calls.push({ method: request.method(), path: url.pathname })
  })
  return { to: (path, method = 'GET') => calls.filter(call => call.path === path && call.method === method) }
}

async function axeFindings(page) {
  await page.mouse.move(0, 0)
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']).analyze()
  return results.violations.map(violation => `${violation.id} (${violation.impact}): ${violation.nodes.slice(0, 3).map(node => node.target.join(' ')).join(' | ')}`)
}

test('lists the bundles with their verification, the last verified backup and the restore commands', async ({ page }) => {
  const requests = recordRequests(page)
  await openScreen(page, 'admin-system-backup', { clock: true })
  const panel = page.locator('[data-settings-panel="backup"]')
  await expect(panel.getByRole('table').getByText('anix-control-20261002T034107Z-daily.tar')).toBeVisible()
  await expect(panel.locator('[data-verify="passed"]').first()).toHaveText('Verified')
  await expect(panel.locator('[data-verify="failed"]')).toHaveText('Verification failed')
  await expect(panel.locator('[data-verify="pending"]')).toHaveText('Not verified yet')
  await expect(panel.getByRole('table').getByText('Before a migration')).toBeVisible()
  await expect(page.locator('[data-test="backup-last-verified"]')).toContainText('4 hours ago')
  await expect(page.locator('[data-test="backup-freshness"]')).toHaveText('Up to date')
  await expect(panel.getByText('/var/lib/anix-control/backups', { exact: true })).toBeVisible()
  const commands = page.locator('[data-test="backup-restore-commands"]')
  await expect(commands).toContainText('sudo systemctl stop anix-control')
  await expect(commands).toContainText('sudo anix-control backup restore /var/lib/anix-control/backups/anix-control-20261002T034107Z-daily.tar --replace')
  await expect(page.getByRole('button', { name: /Restore/ })).toHaveCount(0)
  expect(requests.to('/api/v5/backups')).toHaveLength(1)
  // The removed v2 backup routes are never called.
  expect(requests.to('/api/v2/admin/system/backups')).toHaveLength(0)
  expect(requests.to('/api/v2/admin/system/backup/config')).toHaveLength(0)
})

test('backs up now and reloads the list', async ({ page }) => {
  const requests = recordRequests(page)
  await openScreen(page, 'admin-system-backup', { clock: true })
  await page.locator('[data-test="backup-now"]').click()
  await expectToast(page, 'Backed up to anix-control-20261002T080000Z-manual.tar. It’s being verified.')
  expect(requests.to('/api/v5/backups', 'POST')).toHaveLength(1)
  await expect.poll(() => requests.to('/api/v5/backups').length).toBe(2)
})

test('downloads a bundle as a file', async ({ page }) => {
  await openScreen(page, 'admin-system-backup', { clock: true })
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Actions for anix-control-20261002T034107Z-daily.tar' }).click()
  await page.getByRole('menuitem', { name: 'Download' }).click()
  expect((await download).suggestedFilename()).toBe('anix-control-20261002T034107Z-daily.tar')
})

test('tells an administrator who is not a super administrator why the download is refused', async ({ page }) => {
  await openScreen(page, 'admin-system-backup', { clock: true, scenario: 'backupStaff' })
  await page.getByRole('button', { name: 'Actions for anix-control-20261002T034107Z-daily.tar' }).click()
  await page.getByRole('menuitem', { name: 'Download' }).click()
  await expectToast(page, 'Only a super administrator can download a backup: it holds the panel’s root key.')
})

test('warns when the last verified backup is too old, and when there is none', async ({ page }) => {
  await openScreen(page, 'admin-system-backup-stale', { clock: true })
  await expect(page.locator('[data-test="backup-freshness"]')).toHaveText('Older than 36 hours')
  await openScreen(page, 'admin-system-backup-empty', { clock: true })
  await expect(page.locator('[data-test="backup-last-verified"]')).toContainText('None yet')
  await expect(page.getByText('No backups yet')).toBeVisible()
})

test('says why there are no backups on PostgreSQL', async ({ page }) => {
  await openScreen(page, 'admin-system-backup-unsupported', { clock: true })
  await expect(page.locator('[data-test="backup-unsupported"]')).toContainText('Backups need SQLite')
  await expect(page.locator('[data-test="backup-now"]')).toBeDisabled()
})

test('fits a phone with no sideways scrolling', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await openScreen(page, 'admin-system-backup', { clock: true })
  await expect(page.getByText('anix-control-20261002T034107Z-daily.tar').first()).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow).toBeLessThanOrEqual(0)
})

test('has no axe finding, light and dark', async ({ page }) => {
  for (const theme of ['light', 'dark']) {
    await page.emulateMedia({ colorScheme: theme })
    await openScreen(page, 'admin-system-backup', { theme, clock: true })
    await page.locator('[data-verify="failed"]').waitFor()
    expect(await axeFindings(page), theme).toEqual([])
  }
})

test('the dashboard shows the last verified backup and links to the backups', async ({ page }) => {
  await openScreen(page, 'admin-dashboard', { clock: true })
  const card = page.locator('[data-metric="backup"]')
  await expect(card).toContainText('Last verified backup')
  await expect(card.locator('[data-metric-value]')).toHaveText('4h ago')
  await expect(card).toHaveAttribute('href', '/admin/system/backup')
})
