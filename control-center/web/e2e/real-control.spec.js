import { test, expect } from '@playwright/test'

import { realControlAdmin, realControlAPIURL } from './support/real-control.mjs'

const lifecycleGateEnabled = process.env.ANIXOPS_REAL_CONTROL_LIFECYCLE === '1'

async function realInstallation(page, pluginID) {
  return page.evaluate(async ({ id, apiURL }) => {
    const token = sessionStorage.getItem('kernel_token')
    const response = await fetch(`${apiURL}/api/v3/plugin-installations`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const payload = await response.json()
    return (Array.isArray(payload?.data) ? payload.data : [])
      .find(row => row.plugin_id === id && row.target === 'control') || null
  }, { id: pluginID, apiURL: realControlAPIURL })
}

async function latestPluginOperation(page, pluginID, kind) {
  return page.evaluate(async ({ id, expectedKind, apiURL }) => {
    const token = sessionStorage.getItem('kernel_token')
    const response = await fetch(`${apiURL}/api/v3/operations`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const payload = await response.json()
    return (Array.isArray(payload?.data) ? payload.data : [])
      .find(operation => operation.plugin_id === id && operation.kind === expectedKind) || null
  }, { id: pluginID, expectedKind: kind, apiURL: realControlAPIURL })
}

test('logs into the real Control process and loads its authenticated plugin catalog', async ({ page }) => {
  await page.addInitScript(() => {
    // A synthetic Workers session is intentionally pre-seeded: this gate
    // covers the separate Control login and API boundary without depending
    // on a Workers service.
    localStorage.setItem('token', 'workers-session-for-real-control-gate')
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'worker-admin@anixops.test', role: 'admin' }))
  })

  const loginResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname === '/api/v2/login' && response.request().method() === 'POST'
  })
  const catalogResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname === '/api/v3/plugins' && response.request().method() === 'GET'
  })
  const installationsResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname === '/api/v3/plugin-installations' && response.request().method() === 'GET'
  })

  await page.goto('/plugins')
  await expect(page.getByRole('heading', { name: 'Connect to Anix Control' })).toBeVisible()
  await page.getByLabel('Control email').fill(realControlAdmin.email)
  await page.getByLabel('Control password').fill(realControlAdmin.password)
  await page.getByRole('button', { name: 'Connect' }).click()

  const login = await loginResponse
  expect(login.status()).toBe(200)
  const loginPayload = await login.json()
  expect(loginPayload).toEqual(expect.objectContaining({
    code: 0,
    data: expect.objectContaining({
      is_admin: true,
      email: realControlAdmin.email,
      token: expect.stringMatching(/^[^./]+\.[^./]+\.[^./]+$/),
    }),
  }))

  const catalog = await catalogResponse
  expect(catalog.status()).toBe(200)
  expect(catalog.request().headers().authorization).toMatch(/^Bearer\s+[^ ]+$/)
  const catalogPayload = await catalog.json()
  expect(catalogPayload).toEqual(expect.objectContaining({ data: expect.any(Array) }))
  expect(catalogPayload.data).toEqual(expect.arrayContaining([
    expect.objectContaining({ id: 'identity-platform' }),
  ]))

  const installations = await installationsResponse
  expect(installations.status()).toBe(200)
  const installationPayload = await installations.json()
  expect(installationPayload.data).toEqual(expect.arrayContaining([
    expect.objectContaining({
      plugin_id: 'identity-platform',
      target: 'control',
      desired_version: '4.0.0',
      observed_version: '4.0.0',
      enabled: true,
      state: 'healthy',
    }),
  ]))

  await expect(page.getByRole('heading', { name: 'Identity Platform' })).toBeVisible()
  expect(await page.evaluate(() => sessionStorage.getItem('kernel_token'))).toMatch(/\./)
  expect(await page.evaluate(() => localStorage.getItem('token'))).toBe('workers-session-for-real-control-gate')

  if (!lifecycleGateEnabled) return

  const machineCard = page.locator('article').filter({ hasText: 'Machine Telemetry' })
  await expect(machineCard).toContainText('healthy')
  await expect(machineCard.getByRole('button', { name: 'Disable' })).toBeVisible()
  await machineCard.getByRole('button', { name: 'Disable' }).click()
  await expect(machineCard.getByRole('button', { name: 'Enable' })).toBeVisible({ timeout: 60_000 })
  await expect.poll(() => realInstallation(page, 'machine-telemetry')).toMatchObject({
    enabled: false,
    state: 'disabled',
  })
  await expect.poll(() => latestPluginOperation(page, 'machine-telemetry', 'plugin.disable')).toMatchObject({
    state: 'succeeded',
  })

  await machineCard.getByRole('button', { name: 'Enable' }).click()
  await expect(machineCard.getByRole('button', { name: 'Disable' })).toBeVisible({ timeout: 60_000 })
  await expect.poll(() => realInstallation(page, 'machine-telemetry')).toMatchObject({
    enabled: true,
    state: 'healthy',
    observed_version: '4.0.0',
  })
  await expect.poll(() => latestPluginOperation(page, 'machine-telemetry', 'plugin.enable')).toMatchObject({
    state: 'succeeded',
  })
})
