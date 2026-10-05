import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { openScreen } from './support/screens.js'
import { expectToast } from './support/toasts.js'
import { FIXTURE_CODE, FIXTURE_PASSWORD, FIXTURE_RECOVERY, FIXTURE_TOKEN } from './fixtures/apiTokens.js'

// Security → API tokens (docs/guide/admin-api-tokens.md): the table of the
// administrator's tokens, the create form with its re-authentication, the one
// time token, and revoking. The API is mocked (e2e/fixtures/apiTokens.js); each
// test records the requests the page makes and checks what it sent.
//
// Flows that work a Select inside a dialog (the expiry) run on the real clock:
// the screens' fixed one freezes Date.now(), under which Vue skips a mouse press
// on a Select trigger in a dialog (docs/reference/frontend-design.md).
test.use({ timezoneId: 'UTC', locale: 'en-US' })

const TOKENS = '/api/v4/kernel/api-tokens'

function recordRequests(page) {
  const calls = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/')) return
    calls.push({ method: request.method(), path: url.pathname, url: request.url(), query: Object.fromEntries(url.searchParams), headers: request.headers(), body: request.postData() })
  })
  return {
    calls,
    to: (path, method = 'GET') => calls.filter(call => call.path === path && call.method === method)
  }
}

// Where a token could end up in the browser: the URL, storage, the DOM, the
// console and every request it makes (a URL, a header, a body).
function watchSecret(page) {
  const logged = []
  page.on('console', message => logged.push(message.text()))
  return {
    logged,
    where: async () => page.evaluate(secret => ({
      url: location.href.includes(secret),
      local: JSON.stringify({ ...localStorage }).includes(secret),
      session: JSON.stringify({ ...sessionStorage }).includes(secret),
      text: document.body.innerText.includes(secret),
      fields: [...document.querySelectorAll('input, textarea')].some(field => field.value.includes(secret)),
      markup: document.documentElement.outerHTML.includes(secret)
    }), FIXTURE_TOKEN)
  }
}

const NOWHERE = { url: false, local: false, session: false, text: false, fields: false, markup: false }

async function openForm(page) {
  await page.getByTestId('api-token-create-open').click()
  const form = page.getByRole('dialog', { name: 'Create an API token' })
  await expect(form).toBeVisible()
  return form
}

// Axe reads colours as they are at that moment: a chip or a button still
// fading to its new state (a transition, or the hover left by the click that
// switched the list) reads as a contrast failure. Settle first.
async function settled(page) {
  await page.mouse.move(0, 0)
  await page.evaluate(() => Promise.all(document.getAnimations()
    .filter(animation => animation.effect?.getTiming().iterations !== Infinity)
    .map(animation => animation.finished.catch(() => {}))))
}

async function axeFindings(page, include) {
  await settled(page)
  const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'])
  if (include) builder.include(include)
  const results = await builder.analyze()
  return results.violations.map(violation => `${violation.id} (${violation.impact}): ${violation.nodes.slice(0, 3).map(node => node.target.join(' ')).join(' | ')}`)
}

test.describe('the table of tokens', () => {
  test('lists the active tokens with scope, status, expiry both ways, last use and address', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true })
    await expect(page.getByRole('heading', { level: 1, name: 'Security' })).toBeVisible()
    await expect(page.getByRole('navigation', { name: 'Security sections' }).getByRole('link', { name: 'API tokens' })).toHaveAttribute('aria-current', 'page')

    const calls = requests.to(TOKENS)
    expect(calls).toHaveLength(1)
    expect(calls[0].query).toEqual({})
    // The console's own session, never a token of this kind.
    expect(calls[0].headers.authorization).toBe('Bearer e2e-screen-token')

    await expect(page.getByTestId('api-tokens-quota')).toHaveText('4 of 25 active tokens in use.')
    const rows = page.getByRole('row')
    await expect(rows).toHaveCount(5)
    const runner = page.getByRole('row', { name: /ci runner/ })
    await expect(runner).toContainText('anixadm_…x9Rt')
    await expect(runner).toContainText('Admin')
    await expect(runner).toContainText('Active')
    await expect(runner).toContainText('in 6 months')
    await expect(runner).toContainText('2027-03-22 08:00')
    await expect(runner).toContainText('5 minutes ago')
    await expect(runner).toContainText('198.51.100.24')
    await expect(runner).toContainText('Created 2026-09-23')

    const bot = page.getByRole('row', { name: /deploy bot/ })
    await expect(bot).toContainText('Expires soon')
    await expect(bot).toContainText('in 3 days')
    await expect(bot).toContainText('2001:db8::7')
    await expect(page.getByRole('row', { name: /grafana probe/ })).toContainText('No expiry')
    await expect(page.getByRole('row', { name: /grafana probe/ })).toContainText('Never used')
    // Only the caller's tokens: no Owner column.
    await expect(page.getByRole('columnheader', { name: 'Owner' })).toHaveCount(0)
    // No token in the page, whatever the list holds.
    expect(await page.evaluate(() => /anixadm_[A-Za-z0-9_-]{20}/.test(document.body.innerText))).toBe(false)
  })

  test('shows revoked and expired tokens, and every administrator\'s, when asked', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true })
    await page.getByRole('button', { name: 'Revoked and expired' }).click()
    await expect(page.getByRole('button', { name: 'Revoked and expired' })).toHaveAttribute('aria-pressed', 'true')
    await expect.poll(() => requests.to(TOKENS).at(-1).query).toEqual({ include_inactive: 'true' })
    const expired = page.getByRole('row', { name: /last year’s export/ })
    await expect(expired).toContainText('Expired')
    await expect(expired).toContainText('Expired last month')
    const revoked = page.getByRole('row', { name: /leaked laptop/ })
    await expect(revoked).toContainText('Revoked 20 days ago')
    await expect(revoked).toContainText('Revoked by its owner')
    // An ended token has nothing to revoke.
    await revoked.getByRole('button', { name: /^Actions for / }).click()
    await expect(page.getByRole('menuitem')).toHaveText(['Copy token ID'])
    await page.keyboard.press('Escape')

    await page.getByRole('button', { name: 'All administrators' }).click()
    await expect.poll(() => requests.to(TOKENS).at(-1).query).toEqual({ include_inactive: 'true', all: 'true' })
    await expect(page.getByRole('columnheader', { name: 'Owner' })).toBeVisible()
    await expect(page.getByRole('row', { name: /ops dashboard/ })).toContainText('Administrator #3')
    await expect(page.getByRole('row', { name: /departed admin’s script/ })).toContainText('Administrator #9')
    await expect(page.getByRole('row', { name: /departed admin’s script/ })).toContainText('Revoked: its owner was banned, demoted or deleted')
    await expect(page.getByRole('row', { name: /nightly export/ })).toContainText('You')
    // Only the caller's own active tokens count against the limit of 25.
    await expect(page.getByTestId('api-tokens-quota')).toHaveText('4 of 25 active tokens in use.')
  })

  test('replaces the all-administrators chip with a sentence when the administrator is not a super administrator', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'notSuper' })
    await page.getByRole('button', { name: 'All administrators' }).click()
    await expect(page.getByTestId('api-tokens-super-only')).toHaveText('Only super administrators can list other administrators’ tokens.')
    await expect(page.getByRole('button', { name: 'All administrators' })).toHaveCount(0)
    // The refused request, then the caller's own list.
    await expect.poll(() => requests.to(TOKENS).map(call => call.query.all)).toEqual([undefined, 'true', undefined])
    await expect(page.getByRole('row', { name: /nightly export/ })).toBeVisible()
  })

  test('says what will appear, with a button to create one, when there are none', async ({ page }) => {
    await openScreen(page, 'admin-api-tokens-empty', { clock: true })
    await expect(page.getByRole('heading', { level: 3, name: 'No API tokens' })).toBeVisible()
    await page.getByTestId('api-tokens-empty-create').click()
    await expect(page.getByRole('dialog', { name: 'Create an API token' })).toBeVisible()
  })

  test('shows the error of a failed load and loads again on Try again', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens-error', { clock: true })
    await expect(page.getByText('Couldn’t load the API tokens')).toBeVisible()
    // The route's message, not its JSON.
    await expect(page.getByRole('alert').getByText('the database is unavailable', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Try again' }).click()
    await expect.poll(() => requests.to(TOKENS).length).toBe(2)
  })

  test('turns Create off and says why at the limit of 25 active tokens', async ({ page }) => {
    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'full' })
    await expect(page.getByTestId('api-tokens-quota')).toHaveText('You hold the maximum of 25 active tokens. Revoke one to create another.')
    await expect(page.getByTestId('api-token-create-open')).toBeDisabled()
  })

  test('fits a phone with no sideways scrolling, as cards', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openScreen(page, 'admin-api-tokens', { clock: true })
    await expect(page.getByRole('list', { name: 'API tokens' }).getByRole('listitem')).toHaveCount(4)
    await expect(page.getByRole('table')).toHaveCount(0)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(overflow).toBeLessThanOrEqual(0)
  })

  test('has no axe finding of any impact, light and dark, desktop and phone', async ({ page }) => {
    for (const [theme, viewport] of [['light', { width: 1440, height: 900 }], ['dark', { width: 1440, height: 900 }], ['light', { width: 390, height: 844 }], ['dark', { width: 390, height: 844 }]]) {
      await page.setViewportSize(viewport)
      await page.emulateMedia({ colorScheme: theme, reducedMotion: 'reduce' })
      await openScreen(page, 'admin-api-tokens-all', { theme })
      const findings = await axeFindings(page)
      expect(findings, `${theme} ${viewport.width}: ${findings.join('\n')}`).toEqual([])
    }
  })
})

test.describe('creating a token', () => {
  test('sends the choices, shows the token once, and keeps it nowhere afterwards', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    const requests = recordRequests(page)
    const secret = watchSecret(page)
    await openScreen(page, 'admin-api-tokens')

    const form = await openForm(page)
    // Nothing is sent before the button, and the explanations are on the form.
    expect(requests.to(TOKENS, 'POST')).toHaveLength(0)
    await expect(form.getByRole('radio', { name: 'Read' })).toBeChecked()
    await expect(form).toContainText('GET and HEAD requests on the administrator APIs')
    await expect(form).toContainText('can’t read a node’s API key or the Telegram bot token')
    await expect(form).toContainText('It still can’t manage API tokens')
    await expect(form.getByRole('combobox', { name: 'Expires' })).toContainText('90 days (recommended)')
    await expect(form.getByRole('textbox', { name: /^Name/ })).toBeFocused()

    await form.getByRole('textbox', { name: /^Name/ }).fill('weekly report')
    await form.getByRole('radio', { name: 'Admin' }).click()
    await form.getByRole('combobox', { name: 'Expires' }).click()
    await page.getByRole('option', { name: '30 days', exact: true }).click()
    await expect(form.getByRole('combobox', { name: 'Expires' })).toContainText('30 days')
    await form.getByLabel(/Current password/).fill(FIXTURE_PASSWORD)
    await form.getByRole('button', { name: 'Create token' }).click()

    const result = page.getByRole('dialog', { name: 'Your new API token' })
    await expect(result).toBeVisible()
    const posts = requests.to(TOKENS, 'POST')
    expect(posts).toHaveLength(1)
    expect(JSON.parse(posts[0].body)).toEqual({ name: 'weekly report', scope: 'admin', expires_in_days: 30, password: FIXTURE_PASSWORD })
    // The session, not a token: tokens cannot create tokens.
    expect(posts[0].headers.authorization).toBe('Bearer e2e-screen-token')

    // The token, masked; the warning; what it was made with.
    const field = result.getByLabel('API token')
    await expect(field).toHaveValue(FIXTURE_TOKEN)
    await expect(field).toHaveAttribute('type', 'password')
    await expect(result.getByTestId('api-token-warning')).toContainText('can’t show it again')
    await expect(result.getByTestId('api-token-facts')).toContainText('weekly report')
    await expect(result.getByTestId('api-token-facts')).toContainText('Admin')
    // The usage hint has a header and a placeholder, never the token or a URL with it.
    const usage = result.getByTestId('api-token-usage')
    await expect(usage).toContainText('Authorization: Bearer $ANIXOPS_TOKEN')
    await expect(usage).not.toContainText('anixadm_')
    await expect(result.getByTestId('api-token-no-url')).toContainText('Never put it in a URL')
    // Revealing it shows it in that field; copying puts it on the clipboard.
    await result.getByRole('button', { name: 'Show value' }).click()
    await expect(field).toHaveAttribute('type', 'text')
    await result.getByRole('button', { name: 'Copy token' }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(FIXTURE_TOKEN)

    // Not in the URL, storage, the console or a later request while it is shown...
    expect(await secret.where()).toMatchObject({ url: false, local: false, session: false })
    // ...and the focus starts on the copy button.
    // Done closes it and clears it, with the focus back on the Create button.
    await result.getByRole('button', { name: 'Done' }).click()
    await expect(result).toBeHidden()
    await expect(page.getByTestId('api-token-create-open')).toBeFocused()
    expect(await secret.where()).toEqual(NOWHERE)
    expect(secret.logged.some(line => line.includes(FIXTURE_TOKEN))).toBe(false)
    expect(requests.calls.some(call => call.url.includes('anixadm_') || (call.body || '').includes(FIXTURE_TOKEN) || JSON.stringify(call.headers).includes(FIXTURE_TOKEN))).toBe(false)

    // The list shows the new token (by its last four characters), not its value.
    await expect(page.getByRole('row', { name: /weekly report/ })).toContainText(`anixadm_…${FIXTURE_TOKEN.slice(-4)}`)
    expect(requests.to(TOKENS)).toHaveLength(2)

    // Asking again starts from a clean form.
    const again = await openForm(page)
    await expect(again.getByRole('textbox', { name: /^Name/ })).toHaveValue('')
    await expect(again.getByLabel(/Current password/)).toHaveValue('')
    await expect(again.getByRole('radio', { name: 'Read' })).toBeChecked()
  })

  test('Esc and the close button clear the token too, and the scrim does not close it', async ({ page }) => {
    const secret = watchSecret(page)
    await openScreen(page, 'admin-api-token-created', { clock: true })
    const result = page.getByTestId('api-token-result')
    await expect(result.getByLabel('API token')).toHaveValue(FIXTURE_TOKEN)
    // A click outside leaves it open.
    await page.mouse.click(5, 5)
    await expect(result).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(result).toBeHidden()
    expect(await secret.where()).toEqual(NOWHERE)

    await page.getByTestId('api-token-create-open').click()
    await page.getByRole('textbox', { name: /^Name/ }).fill('again')
    await page.getByLabel(/Current password/).fill(FIXTURE_PASSWORD)
    await page.getByTestId('api-token-submit').click()
    await expect(result).toBeVisible()
    await result.getByRole('button', { name: 'Close' }).click()
    await expect(result).toBeHidden()
    expect(await secret.where()).toEqual(NOWHERE)
  })

  test('leaving the page while the token is shown clears it', async ({ page }) => {
    const secret = watchSecret(page)
    await openScreen(page, 'admin-api-token-created', { clock: true })
    await expect(page.getByTestId('api-token-result')).toBeVisible()
    await page.goBack().catch(() => {})
    await page.goto('/admin/dashboard')
    expect(await secret.where()).toEqual(NOWHERE)
  })

  test('asks for what the form needs before it sends anything', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true })
    const form = await openForm(page)
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByText('Enter a name.')).toBeVisible()
    await expect(form.getByRole('textbox', { name: /^Name/ })).toBeFocused()
    await form.getByRole('textbox', { name: /^Name/ }).fill('x'.repeat(101))
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByText('Use at most 100 characters.')).toBeVisible()
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByText('Enter your current password.')).toBeVisible()
    await expect(form.getByLabel(/Current password/)).toBeFocused()
    expect(requests.to(TOKENS, 'POST')).toHaveLength(0)
  })

  test('takes a custom number of days from 1 to 730, and warns about no expiry', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens')
    const form = await openForm(page)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByRole('combobox', { name: 'Expires' }).click()
    await page.getByRole('option', { name: 'Custom…' }).click()
    await form.getByLabel('Days until it expires').fill('731')
    await form.getByLabel(/Current password/).fill(FIXTURE_PASSWORD)
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByText('Enter a whole number of days from 1 to 730.')).toBeVisible()
    expect(requests.to(TOKENS, 'POST')).toHaveLength(0)
    await form.getByLabel('Days until it expires').fill('45')
    await form.getByRole('combobox', { name: 'Expires' }).click()
    await page.getByRole('option', { name: 'No expiry (not recommended)' }).click()
    await expect(form.getByTestId('api-token-never-notice')).toContainText('works until someone revokes it')
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(page.getByTestId('api-token-result')).toBeVisible()
    // No expiry: the field is left out of the request.
    expect(JSON.parse(requests.to(TOKENS, 'POST')[0].body)).toEqual({ name: 'probe', scope: 'read', password: FIXTURE_PASSWORD })
    await expect(page.getByTestId('api-token-facts')).toContainText('Never')
  })

  test('says so on the field for a wrong password, and works once it is right', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true })
    const form = await openForm(page)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByLabel(/Current password/).fill('not the password')
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByText('That password isn’t right.')).toBeVisible()
    await expect(form.getByLabel(/Current password/)).toHaveValue('')
    await expect(form.getByLabel(/Current password/)).toBeFocused()
    await expect(page.getByTestId('api-token-result')).toHaveCount(0)
    await form.getByLabel(/Current password/).fill(FIXTURE_PASSWORD)
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(page.getByTestId('api-token-result')).toBeVisible()
    expect(requests.to(TOKENS, 'POST')).toHaveLength(2)
  })

  test('asks for the authenticator code when two-step verification is on, with a recovery code as the way out', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'mfa' })
    const form = await openForm(page)
    await expect(form.getByRole('group', { name: 'Authenticator code' })).toBeVisible()
    await expect(form.getByLabel(/Current password/)).toHaveCount(0)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    // A wrong code, typed to the end, is sent at once and refused.
    await form.getByLabel('Digit 1 of 6').click()
    await page.keyboard.type('000000')
    await expect(form.getByText('That code isn’t right. Try the next one.')).toBeVisible()
    expect(JSON.parse(requests.to(TOKENS, 'POST')[0].body)).toMatchObject({ code: '000000', method: 'totp' })
    // A recovery code instead.
    await form.getByRole('button', { name: 'Use a recovery code instead' }).click()
    await form.getByRole('textbox', { name: 'Recovery code' }).fill(FIXTURE_RECOVERY.toLowerCase().replace('-', ''))
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(page.getByTestId('api-token-result')).toBeVisible()
    expect(JSON.parse(requests.to(TOKENS, 'POST')[1].body)).toEqual({ name: 'probe', scope: 'read', expires_in_days: 90, code: FIXTURE_RECOVERY, method: 'backup' })
  })

  test('sends the six digits of the authenticator code as totp', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'mfa' })
    const form = await openForm(page)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByLabel('Digit 1 of 6').click()
    await page.keyboard.type(FIXTURE_CODE)
    await expect(page.getByTestId('api-token-result')).toBeVisible()
    expect(JSON.parse(requests.to(TOKENS, 'POST')[0].body)).toMatchObject({ code: FIXTURE_CODE, method: 'totp' })
    expect(JSON.parse(requests.to(TOKENS, 'POST')[0].body)).not.toHaveProperty('password')
  })

  test('says to sign in again when identity holds the credentials and the sign-in is too old', async ({ page }) => {
    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'cutover' })
    const form = await openForm(page)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByLabel(/Current password/).fill(FIXTURE_PASSWORD)
    await form.getByRole('button', { name: 'Create token' }).click()
    const alert = form.getByRole('alert')
    await expect(alert).toContainText('Your sign-in is too old for this. Sign in again, then create the token within 10 minutes.')
    await alert.getByRole('button', { name: 'Sign in again' }).click()
    await expect(page).toHaveURL(/\/login$/)
  })

  test('says so for 25 active tokens and for too many failed attempts, with the wait', async ({ page }) => {
    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'tooMany' })
    let form = await openForm(page)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByLabel(/Current password/).fill(FIXTURE_PASSWORD)
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByRole('alert')).toContainText('You already hold 25 active API tokens. Revoke one first.')
    await expect(form).toBeVisible()

    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'rateLimited' })
    form = await openForm(page)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByLabel(/Current password/).fill('whatever')
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByText('Too many failed attempts. Try again in 15 min.')).toBeVisible()
    await expect(form.getByLabel(/Current password/)).toHaveValue('')
  })

  test('puts nothing of the credential in the console when the re-authentication fails', async ({ page }) => {
    const logged = []
    page.on('console', message => logged.push(message.text()))
    await openScreen(page, 'admin-api-tokens', { clock: true })
    const form = await openForm(page)
    await form.getByRole('textbox', { name: /^Name/ }).fill('probe')
    await form.getByLabel(/Current password/).fill('a-wrong-password-9')
    await form.getByRole('button', { name: 'Create token' }).click()
    await expect(form.getByText('That password isn’t right.')).toBeVisible()
    expect(logged.join('\n')).not.toContain('a-wrong-password-9')
  })

  test('the form and the token dialog fit a phone, and have no axe finding', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    for (const name of ['admin-api-token-create', 'admin-api-token-created']) {
      await openScreen(page, name, { clock: true })
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
      expect(overflow).toBeLessThanOrEqual(0)
      const findings = await axeFindings(page)
      expect(findings, `${name}: ${findings.join('\n')}`).toEqual([])
    }
  })
})

test.describe('revoking a token', () => {
  async function openRevoke(page, name) {
    await page.getByRole('button', { name: `Actions for ${name}` }).click()
    await page.getByRole('menuitem', { name: 'Revoke token…' }).click()
    const dialog = page.getByRole('alertdialog', { name: `Revoke “${name}”?` })
    await expect(dialog).toBeVisible()
    return dialog
  }

  test('asks first, deletes by id, and the token leaves the list', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true })
    const dialog = await openRevoke(page, 'deploy bot')
    await expect(dialog).toContainText('stops working at once')
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused()
    // Nothing is sent before the danger button.
    expect(requests.to(`${TOKENS}/6b0e1c52-0002`, 'DELETE')).toHaveLength(0)
    await dialog.getByRole('button', { name: 'Revoke token' }).click()
    await expect(dialog).toBeHidden()
    expect(requests.to(`${TOKENS}/6b0e1c52-0002`, 'DELETE')).toHaveLength(1)
    await expect(page.getByRole('row', { name: /deploy bot/ })).toHaveCount(0)
    await expect(page.getByTestId('api-tokens-quota')).toHaveText('3 of 25 active tokens in use.')
    await expectToast(page, 'Revoked “deploy bot”.')
    // It is in the list of ended tokens, revoked by its owner.
    await page.getByRole('button', { name: 'Revoked and expired' }).click()
    await expect(page.getByRole('row', { name: /deploy bot/ })).toContainText('Revoked by its owner')
  })

  test('cancelling sends nothing', async ({ page }) => {
    const requests = recordRequests(page)
    await openScreen(page, 'admin-api-tokens', { clock: true })
    const dialog = await openRevoke(page, 'nightly export')
    await dialog.getByRole('button', { name: 'Cancel' }).click()
    await expect(dialog).toBeHidden()
    expect(requests.calls.filter(call => call.method === 'DELETE')).toHaveLength(0)
    await expect(page.getByRole('row', { name: /nightly export/ })).toBeVisible()
  })

  test('shows a failure inside the confirmation, and says when the token is already gone', async ({ page }) => {
    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'revokeFails' })
    let dialog = await openRevoke(page, 'nightly export')
    await dialog.getByRole('button', { name: 'Revoke token' }).click()
    await expect(dialog.getByRole('alert')).toContainText('Couldn’t revoke the token: database operation failed')
    await expect(dialog).toBeVisible()

    await openScreen(page, 'admin-api-tokens', { clock: true, scenario: 'revokeGone' })
    dialog = await openRevoke(page, 'nightly export')
    await dialog.getByRole('button', { name: 'Revoke token' }).click()
    await expect(dialog.getByRole('alert')).toContainText('This token no longer exists, or it isn’t yours.')
  })

  test('copies the token\'s ID for the audit log', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await openScreen(page, 'admin-api-tokens', { clock: true })
    await page.getByRole('button', { name: 'Actions for nightly export' }).click()
    await page.getByRole('menuitem', { name: 'Copy token ID' }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('6b0e1c52-0001')
  })

  test('is a menu on a phone card too', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openScreen(page, 'admin-api-tokens', { clock: true })
    const dialog = await openRevoke(page, 'grafana probe')
    await expect(dialog).toBeVisible()
  })
})
