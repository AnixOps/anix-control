import { test, expect } from '@playwright/test'

function setSession(page, { kernel = true } = {}) {
  return page.addInitScript(({ kernel }) => {
    if (window.location.pathname === '/login') return
    localStorage.setItem('token', 'e2e-token')
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@example.com', role: 'admin' }))
    if (kernel) sessionStorage.setItem('kernel_token', 'e2e-kernel-token')
  }, { kernel })
}

function fulfillJson(route, data, headers = {}) {
  return route.fulfill({
    status: 200,
    contentType: 'application/json',
    headers,
    body: JSON.stringify({ data })
  })
}

function operationHeaders(id) {
  return {
    'X-AnixOps-Operation-ID': id,
    'X-AnixOps-Operation-Chain': id,
    'Access-Control-Expose-Headers': 'X-AnixOps-Operation-ID, X-AnixOps-Operation-Chain'
  }
}

function errorJson(route, status, message) {
  return route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify({ error: { message } })
  })
}

async function mockPluginState(page, options = {}) {
  const installation = options.installation || {
    id: 7,
    plugin_id: 'machine-telemetry',
    target: 'control',
    desired_version: '1.0.0',
    observed_version: '1.0.0',
    previous_version: '',
    state: 'disabled',
    enabled: false,
    config_revision: 2
  }
  const config = options.config || { enabled: true, interval: 30 }
  const configRevision = options.configRevision || 2

  await page.route('**/api/v3/**', async route => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname

    if (request.method() === 'GET' && pathname === '/api/v3/plugins') {
      return fulfillJson(route, [{
        id: 'machine-telemetry',
        name: 'Machine Telemetry',
        publisher: 'AnixOps',
        description: 'Collect host metrics for the Control kernel.',
        official: true
      }])
    }
    if (request.method() === 'GET' && pathname === '/api/v3/plugin-releases') {
      return fulfillJson(route, [{
        id: 11,
        plugin_id: 'machine-telemetry',
        version: '1.0.0'
      }])
    }
    if (request.method() === 'GET' && pathname === '/api/v3/plugin-installations') {
      return fulfillJson(route, [installation])
    }
    if (request.method() === 'GET' && pathname === '/api/v3/extensions') {
      return fulfillJson(route, [])
    }
    if (request.method() === 'GET' && pathname === '/api/v3/operations') {
      return fulfillJson(route, [])
    }
    if (request.method() === 'POST' && pathname === '/api/v3/plugin-installations/7/actions') {
      if (options.actionStatus) return errorJson(route, options.actionStatus, options.actionMessage)
      return fulfillJson(route, {
        installation: { ...installation, state: 'enabled', enabled: true },
        operation: { id: 'op-enable-7', kind: 'plugin.enable', state: 'pending' }
      }, operationHeaders('op-enable-7'))
    }
    if (request.method() === 'GET' && pathname === '/api/v3/plugin-installations/7/config') {
      return fulfillJson(route, { installation_id: 7, revision: configRevision, config })
    }
    if (request.method() === 'PUT' && pathname === '/api/v3/plugin-installations/7/config') {
      if (options.configStatus) return errorJson(route, options.configStatus, options.configMessage)
      return fulfillJson(route, { installation_id: 7, revision: configRevision + 1, config })
    }
    return route.fulfill({ status: 404, body: `Unhandled ${request.method()} ${pathname}` })
  })
}

test.describe('Kernel plugin lifecycle', () => {
  test('keeps the plugin connection usable on a mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await setSession(page, { kernel: false })
    await page.goto('/plugins')

    await expect(page.getByRole('heading', { name: 'Connect to Anix Control' })).toBeVisible()
    await expect(page.locator('aside')).toBeHidden()
    await expect(page.getByRole('button', { name: 'Open navigation' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    expect(await page.locator('main').evaluate(element => element.getBoundingClientRect().width)).toBe(390)

    await page.getByRole('button', { name: 'Open navigation' }).click()
    await expect(page.getByRole('link', { name: 'Kernel Plugins' })).toBeVisible()
    await page.locator('aside').last().getByRole('button', { name: 'Close navigation' }).click()
    await expect(page.getByRole('link', { name: 'Kernel Plugins' })).toBeHidden()
  })

  test('renders the catalog, installs and enables Control, then saves revisioned config', async ({ page }) => {
    await setSession(page)

    let installation = null
    let config = { enabled: true, interval: 30 }
    let configRevision = 2
    let currentOperation = null
    const installationRequests = []
    const actionRequests = []
    const configRequests = []

    await page.route('**/api/v3/**', async route => {
      const request = route.request()
      const pathname = new URL(request.url()).pathname

      if (request.method() === 'GET' && pathname === '/api/v3/plugins') {
        return fulfillJson(route, [{
          id: 'machine-telemetry',
          name: 'Machine Telemetry',
          publisher: 'AnixOps',
          description: 'Collect host metrics for the Control kernel.',
          official: true
        }])
      }

      if (request.method() === 'GET' && pathname === '/api/v3/plugin-releases') {
        return fulfillJson(route, [{
          id: 11,
          plugin_id: 'machine-telemetry',
          version: '1.0.0',
          manifest: JSON.stringify({ targets: ['control', 'agent'] })
        }, {
          id: 12,
          plugin_id: 'machine-telemetry',
          version: '2.0.0',
          manifest: JSON.stringify({ targets: ['control', 'agent'] })
        }])
      }

      if (request.method() === 'GET' && pathname === '/api/v3/plugin-installations') {
        return fulfillJson(route, installation ? [installation] : [])
      }

      if (request.method() === 'GET' && pathname === '/api/v3/extensions') {
        return fulfillJson(route, [])
      }

      if (request.method() === 'GET' && pathname === '/api/v3/operations') {
        return fulfillJson(route, currentOperation ? [currentOperation] : [])
      }

      if (request.method() === 'PUT' && pathname === '/api/v3/plugin-installations') {
        const payload = request.postDataJSON()
        installationRequests.push(payload)
        installation = {
          id: 7,
          observed_version: '',
          previous_version: '',
          state: 'disabled',
          config_revision: 2,
          ...payload
        }
        return fulfillJson(route, installation, operationHeaders('op-install-7'))
      }

      if (request.method() === 'POST' && pathname === '/api/v3/plugin-installations/7/actions') {
        const payload = request.postDataJSON()
        actionRequests.push(payload)
        const currentVersion = installation.desired_version
        let desiredVersion = currentVersion
        let previousVersion = installation.previous_version
        if (payload.action === 'update') {
          desiredVersion = payload.target_version
          previousVersion = currentVersion
        } else if (payload.action === 'rollback') {
          desiredVersion = installation.previous_version
          previousVersion = currentVersion
        }
        currentOperation = {
          id: `op-${payload.action}-7`,
          kind: `plugin.${payload.action}`,
          state: 'succeeded',
          target_version: desiredVersion
        }
        installation = {
          ...installation,
          desired_version: desiredVersion,
          observed_version: desiredVersion,
          previous_version: previousVersion,
          state: payload.action === 'disable' ? 'disabled' : 'enabled',
          enabled: payload.action !== 'disable'
        }
        return fulfillJson(route, {
          installation,
          operation: { ...currentOperation, state: 'pending' }
        }, operationHeaders(currentOperation.id))
      }

      if (request.method() === 'GET' && pathname === '/api/v3/plugin-installations/7/config') {
        return fulfillJson(route, { installation_id: 7, revision: configRevision, config })
      }

      if (request.method() === 'PUT' && pathname === '/api/v3/plugin-installations/7/config') {
        const payload = request.postDataJSON()
        configRequests.push(payload)
        expect(payload.expected_revision).toBe(2)
        config = payload.config
        configRevision = 3
        currentOperation = { id: 'op-config-7', kind: 'plugin.config', state: 'succeeded' }
        return fulfillJson(route, { installation_id: 7, revision: configRevision, config }, operationHeaders(currentOperation.id))
      }

      return route.fulfill({ status: 404, body: `Unhandled ${request.method()} ${pathname}` })
    })

    await page.goto('/plugins')
    await expect(page.getByRole('heading', { name: 'Plugins' })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Machine Telemetry' })).toBeVisible()
    await expect(page.getByText('Official', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Install control plugin' })).toBeVisible()

    await page.getByRole('button', { name: 'Install control plugin' }).click()
    await expect(page.getByRole('button', { name: 'Disable' })).toBeVisible()
    await expect.poll(() => installationRequests.length).toBe(1)
    await expect.poll(() => actionRequests.length).toBe(1)
    expect(installationRequests[0]).toMatchObject({
      plugin_id: 'machine-telemetry',
      target: 'control',
      desired_version: '1.0.0',
      enabled: false
    })
    expect(actionRequests[0]).toMatchObject({ action: 'enable' })
    expect(actionRequests[0].idempotency_key).toMatch(/^control-center:enable:7:/)
    await expect(page.getByRole('status')).toContainText('Operation op-enable-7 is succeeded')
    await expect(page.getByRole('status')).toContainText('Chain: op-enable-7')
    const operationTable = page.locator('table[aria-label="Recent plugin operations"]')
    await expect(operationTable).toContainText('op-enable-7')
    await expect(operationTable).toContainText('plugin.enable')
    await expect(operationTable).toContainText('control')
    await expect(operationTable).toContainText('1.0.0')

    await page.getByRole('button', { name: 'Configure' }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toContainText('revision 2')
    await expect(dialog.locator('textarea')).toHaveValue(JSON.stringify(config, null, 2))

    await dialog.locator('textarea').fill('{"enabled":false,"interval":60}')
    await dialog.getByRole('button', { name: 'Save configuration' }).click()
    await expect.poll(() => configRequests.length).toBe(1)
    expect(configRequests[0]).toEqual({
      config: { enabled: false, interval: 60 },
      expected_revision: 2
    })
    await expect(dialog).toBeHidden()
    await expect(page.getByRole('status')).toContainText('Operation op-config-7 is succeeded')

    await page.getByRole('button', { name: 'Update to 2.0.0' }).click()
    await expect.poll(() => actionRequests.length).toBe(2)
    expect(actionRequests[1]).toMatchObject({ action: 'update', target_version: '2.0.0' })
    await expect(page.getByRole('status')).toContainText('Operation op-update-7 is succeeded')

    await page.getByRole('button', { name: 'Rollback' }).click()
    await expect.poll(() => actionRequests.length).toBe(3)
    expect(actionRequests[2]).toMatchObject({ action: 'rollback' })
    await expect(page.getByRole('status')).toContainText('Operation op-rollback-7 is succeeded')

    await page.getByRole('button', { name: 'Disable' }).click()
    await expect.poll(() => actionRequests.length).toBe(4)
    expect(actionRequests[3]).toMatchObject({ action: 'disable' })
    await expect(page.getByRole('status')).toContainText('Operation op-disable-7 is succeeded')
  })

  test('runs the Agent target lifecycle through installation intents', async ({ page }) => {
    await setSession(page)

    let installation = null
    const upsertRequests = []

    await page.route('**/api/v3/**', async route => {
      const request = route.request()
      const pathname = new URL(request.url()).pathname

      if (request.method() === 'GET' && pathname === '/api/v3/plugins') {
        return fulfillJson(route, [{ id: 'machine-telemetry', name: 'Machine Telemetry', official: true }])
      }
      if (request.method() === 'GET' && pathname === '/api/v3/plugin-releases') {
        return fulfillJson(route, [
          { id: 11, plugin_id: 'machine-telemetry', version: '1.0.0', manifest: JSON.stringify({ targets: ['agent'] }) },
          { id: 12, plugin_id: 'machine-telemetry', version: '2.0.0', manifest: JSON.stringify({ targets: ['agent'] }) }
        ])
      }
      if (request.method() === 'GET' && pathname === '/api/v3/plugin-installations') {
        return fulfillJson(route, installation ? [installation] : [])
      }
      if (request.method() === 'GET' && pathname === '/api/v3/extensions') {
        return fulfillJson(route, [])
      }
      if (request.method() === 'GET' && pathname === '/api/v3/operations') {
        return fulfillJson(route, [])
      }
      if (request.method() === 'PUT' && pathname === '/api/v3/plugin-installations') {
        const payload = request.postDataJSON()
        upsertRequests.push(payload)
        const previousVersion = installation?.desired_version && installation.desired_version !== payload.desired_version
          ? installation.desired_version
          : installation?.previous_version || ''
        installation = {
          id: 8,
          plugin_id: payload.plugin_id,
          target: 'agent',
          desired_version: payload.desired_version,
          observed_version: payload.enabled ? payload.desired_version : installation?.observed_version || '',
          previous_version: previousVersion,
          state: payload.enabled ? 'pending' : 'disabled',
          enabled: payload.enabled,
          config_revision: 1
        }
        return fulfillJson(route, installation)
      }
      return route.fulfill({ status: 404, body: `Unhandled ${request.method()} ${pathname}` })
    })

    await page.goto('/plugins')
    await page.getByRole('button', { name: 'Install agent plugin' }).click()
    await expect.poll(() => upsertRequests.length).toBe(1)
    expect(upsertRequests[0]).toMatchObject({
      plugin_id: 'machine-telemetry', target: 'agent', desired_version: '1.0.0', enabled: true
    })

    await page.getByRole('button', { name: 'Update to 2.0.0' }).click()
    await expect.poll(() => upsertRequests.length).toBe(2)
    expect(upsertRequests[1]).toMatchObject({
      plugin_id: 'machine-telemetry', target: 'agent', desired_version: '2.0.0', enabled: true
    })

    await page.getByRole('button', { name: 'Rollback' }).click()
    await expect.poll(() => upsertRequests.length).toBe(3)
    expect(upsertRequests[2]).toMatchObject({
      plugin_id: 'machine-telemetry', target: 'agent', desired_version: '1.0.0', enabled: true
    })

    await page.getByRole('button', { name: 'Disable' }).click()
    await expect.poll(() => upsertRequests.length).toBe(4)
    expect(upsertRequests[3]).toMatchObject({
      plugin_id: 'machine-telemetry', target: 'agent', desired_version: '1.0.0', enabled: false
    })
  })
})

test.describe('Kernel plugin lifecycle errors', () => {
  for (const [status, message] of [
    [403, 'plugin execution is forbidden'],
    [409, 'plugin execution is disabled'],
    [501, 'plugin execution is not implemented']
  ]) {
    test(`shows a ${status} lifecycle error`, async ({ page }) => {
      await setSession(page)
      await mockPluginState(page, {
        actionStatus: status,
        actionMessage: message
      })

      await page.goto('/plugins')
      await page.getByRole('button', { name: 'Enable' }).click()
      await expect(page.getByText(message, { exact: true })).toBeVisible()
    })
  }

  test('reconnects Control without logging out the Workers session on 401', async ({ page }) => {
    await setSession(page)
    await page.route('**/api/v3/**', route => {
      const pathname = new URL(route.request().url()).pathname
      if (pathname === '/api/v3/plugins') return errorJson(route, 401, 'authentication required')
      return fulfillJson(route, [])
    })

    await page.goto('/plugins')
    await expect(page).toHaveURL(/\/plugins/, { timeout: 5000 })
    await expect(page.getByRole('heading', { name: 'Connect to Anix Control' })).toBeVisible()
    await expect(page.getByRole('alert')).toContainText('Control session expired')
    expect(await page.evaluate(() => localStorage.getItem('token'))).toBe('e2e-token')
    expect(await page.evaluate(() => sessionStorage.getItem('kernel_token'))).toBeNull()
  })

  test('connects to Control with MFA before loading the plugin catalog', async ({ page }) => {
    await setSession(page, { kernel: false })
    await mockPluginState(page)
    const loginRequests = []
    const pluginAuth = []
    await page.route('**/api/v2/login', async route => {
      const body = route.request().postDataJSON()
      loginRequests.push(body)
      const data = body.mfa_code
        ? { token: 'control-admin-token', is_admin: true, user_id: 1, email: body.email }
        : { mfa_required: true, methods: ['totp'] }
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, data }) })
    })
    await page.route('**/api/v3/plugins', async route => {
      pluginAuth.push(route.request().headers().authorization)
      return fulfillJson(route, [{ id: 'machine-telemetry', name: 'Machine Telemetry', official: true }])
    })

    await page.goto('/plugins')
    await expect(page.getByRole('heading', { name: 'Connect to Anix Control' })).toBeVisible()
    expect(pluginAuth).toHaveLength(0)
    await page.getByLabel('Control email').fill('admin@example.com')
    await page.getByLabel('Control password').fill('secret')
    await page.getByRole('button', { name: 'Connect' }).click()
    await expect(page.getByLabel('Verification code')).toBeVisible()
    await page.getByLabel('Verification code').fill('123456')
    await page.getByRole('button', { name: 'Connect' }).click()
    await expect(page.getByRole('heading', { name: 'Machine Telemetry' })).toBeVisible()
    expect(loginRequests).toEqual([
      { email: 'admin@example.com', password: 'secret' },
      { email: 'admin@example.com', password: 'secret', mfa_code: '123456', mfa_method: 'totp' }
    ])
    expect(pluginAuth).toContain('Bearer control-admin-token')
    expect(await page.evaluate(() => localStorage.getItem('token'))).toBe('e2e-token')
  })

  test('keeps Control disconnected when MFA enrollment is required', async ({ page }) => {
    await setSession(page, { kernel: false })
    let pluginRequests = 0
    await page.route('**/api/v2/login', route => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: { mfa_enrollment_required: true, methods: ['totp'] } })
    }))
    await page.route('**/api/v3/plugins', route => {
      pluginRequests += 1
      return fulfillJson(route, [])
    })

    await page.goto('/plugins')
    await page.getByLabel('Control email').fill('admin@example.com')
    await page.getByLabel('Control password').fill('secret')
    await page.getByRole('button', { name: 'Connect' }).click()

    await expect(page.getByRole('status')).toContainText('MFA enrollment is required')
    await expect(page.getByRole('heading', { name: 'Connect to Anix Control' })).toBeVisible()
    expect(await page.evaluate(() => sessionStorage.getItem('kernel_token'))).toBeNull()
    expect(pluginRequests).toBe(0)
  })

  test('keeps the configuration dialog open on a 409 revision conflict', async ({ page }) => {
    await setSession(page)
    await mockPluginState(page, {
      configStatus: 409,
      configMessage: 'configuration revision is stale'
    })

    await page.goto('/plugins')
    await page.getByRole('button', { name: 'Configure' }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await dialog.locator('textarea').fill('{"enabled":false,"interval":60}')
    await dialog.getByRole('button', { name: 'Save configuration' }).click()

    await expect(dialog).toBeVisible()
    await expect(dialog.getByText('configuration revision is stale', { exact: true })).toBeVisible()
  })
})
