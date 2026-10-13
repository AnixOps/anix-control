import { test, expect } from '@playwright/test'
import { openScreen } from './support/screens.js'

// 系统设置 → 内置包 replaced 插件中心: the built-in packages' state, a host
// restart and a package's settings; nothing to import, install, upgrade,
// roll back or disable.

test('lists the built-in packages with their state and no lifecycle controls', async ({ page }) => {
  await openScreen(page, 'admin-system-packages')
  const list = page.getByTestId('package-list')
  await expect(list.locator('[data-package]')).toHaveCount(5)
  await expect(page.getByTestId('package-row-ticket')).toContainText('Failed')
  await expect(page.getByTestId('package-row-ticket')).toContainText('plugin installation failed')
  await expect(page.getByTestId('package-row-ticket')).toContainText('Version 4.2.0; 4.2.1 is not running')
  await expect(page.getByTestId('package-row-knowledge')).toContainText('Version 4.2.1, starting 4.3.0')
  for (const action of ['install', 'upgrade', 'rollback', 'enable', 'disable']) {
    await expect(page.locator(`[data-action="${action}"]`)).toHaveCount(0)
  }
  await expect(page.getByRole('button', { name: /import/i })).toHaveCount(0)
})

test('restarts a package host after a confirmation', async ({ page }) => {
  const restarts = []
  page.on('request', request => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v3/plugin-installations/11/actions') {
      restarts.push(request.postDataJSON())
    }
  })
  await openScreen(page, 'admin-system-packages')
  await page.getByRole('button', { name: 'Restart Identity Platform' }).click()
  const dialog = page.getByRole('alertdialog')
  await expect(dialog).toContainText('Restart Identity Platform?')
  await dialog.getByRole('button', { name: 'Restart' }).click()
  await expect(page.getByText('Identity Platform is restarting')).toBeVisible()
  expect(restarts).toHaveLength(1)
  expect(restarts[0]).toEqual(expect.objectContaining({ action: 'restart', idempotency_key: expect.stringMatching(/^webui:11:restart:/) }))
})

test('opens the settings of a package that declares them', async ({ page }) => {
  await openScreen(page, 'admin-system-packages')
  await page.getByRole('button', { name: 'Configure Machine Telemetry' }).click()
  const dialog = page.getByTestId('package-config-dialog')
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('Machine Telemetry settings')
  await expect(page.getByRole('button', { name: 'Configure Identity Platform' })).toHaveCount(0)
})

test('sends the old plugin center and route-mode pages to the built-in packages', async ({ page }) => {
  await openScreen(page, 'admin-system-packages')
  for (const path of ['/admin/plugins', '/admin/plugins/route-modes']) {
    await page.goto(path)
    await expect(page).toHaveURL(/\/admin\/system\/packages$/)
    await expect(page.getByTestId('package-list')).toBeVisible()
  }
})
