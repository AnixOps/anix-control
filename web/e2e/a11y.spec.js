import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { SCREENS, openScreen } from './support/screens.js'

// Plan §11: axe on the main admin and user screens, light and dark, desktop
// and phone, with the API mocked. Serious and critical findings fail the
// build; WCAG 2.2 AA rules plus best practices.
const VIEWPORTS = { desktop: { width: 1440, height: 900 }, phone: { width: 390, height: 844 } }
// admin-forward-no-capability leads to 插件中心 (admin-plugins).
const SKIP = new Set(['admin-dashboard-error', 'admin-users-empty', 'admin-forward-no-capability', 'admin-forward-editor-blank'])

for (const name of Object.keys(SCREENS).filter(screen => !SKIP.has(screen))) {
  for (const theme of ['light', 'dark']) {
    for (const [device, viewport] of Object.entries(VIEWPORTS)) {
      test(`${name} has no serious axe findings (${theme}, ${device})`, async ({ page }) => {
        await page.setViewportSize(viewport)
        await page.emulateMedia({ colorScheme: theme, reducedMotion: 'reduce' })
        await openScreen(page, name, { theme })
        const results = await new AxeBuilder({ page })
          .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'])
          .analyze()
        const blocking = results.violations
          .filter(violation => violation.impact === 'serious' || violation.impact === 'critical')
          .map(violation => `${violation.id} (${violation.impact}): ${violation.nodes.slice(0, 3).map(node => node.target.join(' ')).join(' | ')}`)
        expect(blocking, blocking.join('\n')).toEqual([])
      })
    }
  }
}

// Plan §11: a skip link, landmarks, one h1, and focus back on the opener
// after a Sheet or dialog closes.
test('admin shell has a skip link, landmarks and a single h1', async ({ page }) => {
  await openScreen(page, 'admin-users')
  await page.keyboard.press('Tab')
  const skip = page.locator(':focus')
  await expect(skip).toHaveAttribute('href', /#/)
  await skip.press('Enter')
  await expect(page.locator('main').first()).toBeFocused()
  await expect(page.locator('h1')).toHaveCount(1)
  await expect(page.getByRole('navigation').first()).toBeVisible()
  await expect(page.getByRole('banner').first()).toBeAttached()
})

test('user shell has a skip link, landmarks and a single h1', async ({ page }) => {
  await openScreen(page, 'user-home')
  await page.keyboard.press('Tab')
  const skip = page.locator(':focus')
  await expect(skip).toHaveAttribute('href', /#/)
  await skip.press('Enter')
  await expect(page.locator('main').first()).toBeFocused()
  await expect(page.locator('h1')).toHaveCount(1)
})

test('closing a dialog returns focus to the button that opened it', async ({ page }) => {
  await openScreen(page, 'user-subscribe')
  const opener = page.locator('[data-reset-open]')
  await opener.focus()
  await opener.press('Enter')
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(opener).toBeFocused()
})

// Drop-down menus are portalled out of the page. Each open menu sits in its
// own labelled region (ui/composables/useMenuLayer.js), so none of its items
// is axe "region" content (moderate, which the screen sweep above does not
// fail on), and no layer is left behind once it closes.
const AXE_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']

async function axeFindings(page, options = {}) {
  const builder = new AxeBuilder({ page }).withTags(AXE_TAGS)
  if (options.include) builder.include(options.include)
  const results = await builder.analyze()
  return results.violations.map(violation => `${violation.id} (${violation.impact}): ${violation.nodes.slice(0, 3).map(node => node.target.join(' ')).join(' | ')}`)
}

test('an open row action menu is inside a labelled region and has no axe findings', async ({ page }) => {
  await openScreen(page, 'admin-users')
  await page.getByRole('row').nth(1).getByRole('button', { name: /^Actions for / }).click()
  const region = page.getByRole('region', { name: /^Actions for / })
  await expect(region.getByRole('menu')).toBeVisible()
  await expect(region.getByRole('menuitem').first()).toBeVisible()
  const findings = await axeFindings(page)
  expect(findings, findings.join('\n')).toEqual([])
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu')).toHaveCount(0)
  await expect(page.getByRole('region', { name: /^Actions for / })).toHaveCount(0)
})

test('the open account menu is inside a labelled region and has no axe findings', async ({ page }) => {
  await openScreen(page, 'admin-users')
  await page.locator('[data-account-menu-trigger]').first().click()
  const region = page.getByRole('region', { name: /^Account menu: / })
  await expect(region.getByRole('menu')).toBeVisible()
  const findings = await axeFindings(page)
  expect(findings, findings.join('\n')).toEqual([])
  await page.keyboard.press('Escape')
  await expect(page.getByRole('region', { name: /^Account menu: / })).toHaveCount(0)
})

// Select and Combobox lists are portalled out of the page as well. Each open
// list sits in its own labelled region (useMenuLayer), named "<label>
// options", and the region is gone once the list closes.
//
// A Select is modal in Reka (pointer events outside it are off, Tab is
// swallowed, every other part of the page is `aria-hidden`), and axe reports
// "aria-hidden-focus" (serious) for hidden controls that can still be
// focused. UiSelect makes the page behind its list `inert` (useInertBehind),
// so nothing hidden can take focus. What is left on the page is checked below
// the way axe does it: no tabbable control outside the list that is not
// inert, except what carries an `aria-live` attribute (the same exemption as
// Reka's hide-others pass).
async function settled(page) {
  await page.evaluate(() => Promise.all(document.getAnimations()
    .filter(animation => animation.effect?.getTiming().iterations !== Infinity)
    .map(animation => animation.finished.catch(() => {}))))
}

function tabbableOutside(page, layerSelector) {
  return page.evaluate(selector => [...document.querySelectorAll('a[href], button, input, select, textarea, [tabindex]')]
    .filter(element => element.tabIndex >= 0 && !element.disabled && !element.closest(`[inert], ${selector}, [aria-live], [data-reka-focus-guard]`))
    .map(element => `${element.localName}${element.className ? `.${String(element.className).split(' ')[0]}` : ''}`), layerSelector)
}

test('an open Select list is in a labelled region and the page behind it has no focusable hidden control', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await openScreen(page, 'admin-forward-routes')
  const trigger = page.getByRole('combobox', { name: 'Engine' })
  await trigger.click()
  const region = page.getByRole('region', { name: 'Engine options' })
  await expect(region.getByRole('listbox')).toBeVisible()
  await expect(region.getByRole('option').first()).toBeVisible()
  await expect(region.locator(':focus')).toHaveCount(1)
  expect(await tabbableOutside(page, '.ui-menu-layer')).toEqual([])
  await settled(page)
  // The page heading is hidden on purpose while the modal list is open, and
  // axe cannot tell: a listbox is not a dialog, so "page-has-heading-one"
  // would fail. Everything else must be clean, aria-hidden-focus and region
  // included.
  const findings = (await axeFindings(page)).filter(item => !item.startsWith('page-has-heading-one'))
  expect(findings, findings.join('\n')).toEqual([])

  // Keyboard behaviour is unchanged: arrows move, Enter chooses and the
  // page is back in the tab order with focus on the trigger.
  await page.keyboard.press('ArrowDown')
  await expect(region.getByRole('option').nth(1)).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('region', { name: 'Engine options' })).toHaveCount(0)
  await expect(page.locator('[inert]')).toHaveCount(0)
  await expect(trigger).toBeFocused()
  await expect(trigger).not.toHaveText(/^All/)

  await page.keyboard.press('ArrowDown')
  await expect(region.getByRole('listbox')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(region).toHaveCount(0)
  await expect(page.locator('[inert]')).toHaveCount(0)
  await expect(trigger).toBeFocused()

  // `inert` changes hit-testing: a click outside still dismisses the list
  // (Reka's outside-pointer handling) and puts the page back.
  await trigger.click()
  await expect(region.getByRole('listbox')).toBeVisible()
  await page.mouse.click(700, 120)
  await expect(region).toHaveCount(0)
  await expect(page.locator('[inert]')).toHaveCount(0)
})

test('a Select opened in a modal dialog stays readable and usable', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await openScreen(page, 'admin-users')
  await page.getByRole('button', { name: 'New user' }).first().click()
  const dialog = page.getByRole('dialog', { name: 'New user' })
  const trigger = dialog.getByRole('combobox', { name: 'User type' })
  await trigger.click()
  // Playwright's role queries skip aria-hidden content: the list is readable.
  const region = page.getByRole('region', { name: 'User type options' })
  await expect(region.getByRole('listbox')).toBeVisible()
  await settled(page)
  const findings = await axeFindings(page)
  expect(findings, findings.join('\n')).toEqual([])

  // Escape closes the list and leaves the dialog open, with focus back on
  // the trigger.
  await page.keyboard.press('Escape')
  await expect(region).toHaveCount(0)
  await expect(dialog).toBeVisible()
  await expect(trigger).toBeFocused()

  // A pointer choice works through the layer, and the dialog is usable after.
  await trigger.click()
  await region.getByRole('option', { name: 'Admin' }).click()
  await expect(trigger).toHaveText(/Admin/)
  await expect(region).toHaveCount(0)
  await expect(page.locator('[inert]')).toHaveCount(0)
  await dialog.getByLabel('Email').fill('new@example.com')
  await expect(dialog.getByLabel('Email')).toHaveValue('new@example.com')

  // Only Escape on the dialog itself closes it.
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
})

test('an open Combobox list is inside a labelled region and has no axe findings', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await openScreen(page, 'admin-forward-editor-blank')
  const input = page.locator('[data-hop-index="0"]').getByRole('combobox', { name: /Add a node to hop/ })
  await input.click()
  // sg-relay-01 is the one enabled match; a disabled option's grey text is
  // exempt from contrast by WCAG, but axe cannot tell it from enabled text.
  await input.fill('sg-relay')
  const region = page.getByRole('region', { name: /^Add a node to hop 1 options$/ })
  await expect(region.getByRole('option', { name: /sg-relay-01/ })).toBeVisible()
  await settled(page)
  const findings = await axeFindings(page)
  expect(findings, findings.join('\n')).toEqual([])
  await page.keyboard.press('Escape')
  await expect(region).toHaveCount(0)
  await expect(input).toBeFocused()
})

// WAI-ARIA tabs: the install targets of the plugin drawer are one tab stop;
// Left/Right/Home/End move between them and select (Reka Tabs via UiTabs).
test('plugin drawer target tabs follow the WAI-ARIA tabs keyboard pattern', async ({ page }) => {
  await openScreen(page, 'admin-plugins')
  await page.getByTestId('plugin-row-protocol-runtime').click()
  const drawer = page.getByTestId('plugin-detail-drawer')
  const tabs = drawer.getByRole('tablist', { name: 'Runtime target' }).getByRole('tab')
  await expect(tabs).toHaveCount(2)
  const [first, second] = [tabs.nth(0), tabs.nth(1)]
  await expect(first).toHaveAttribute('aria-selected', 'true')

  // Tab from the sheet header lands on the selected tab, once.
  await drawer.getByRole('button', { name: 'Close' }).focus()
  await page.keyboard.press('Tab')
  await expect(first).toBeFocused()
  await expect(first).toHaveAttribute('tabindex', '0')
  await expect(second).toHaveAttribute('tabindex', '-1')

  await page.keyboard.press('ArrowRight')
  await expect(second).toBeFocused()
  await expect(second).toHaveAttribute('aria-selected', 'true')
  await expect(second).toHaveAttribute('tabindex', '0')
  await expect(first).toHaveAttribute('tabindex', '-1')
  await expect(drawer.getByRole('tabpanel')).toHaveAttribute('aria-labelledby', await second.getAttribute('id'))

  await page.keyboard.press('ArrowRight')
  await expect(first).toBeFocused()
  await page.keyboard.press('End')
  await expect(second).toBeFocused()
  await page.keyboard.press('Home')
  await expect(first).toBeFocused()
  await page.keyboard.press('ArrowLeft')
  await expect(second).toBeFocused()

  // Tab leaves the tablist for the panel instead of walking the tabs.
  await page.keyboard.press('Tab')
  await expect(drawer.getByRole('tablist').locator(':focus')).toHaveCount(0)
  const findings = await axeFindings(page, { include: '[data-testid="plugin-detail-drawer"]' })
  expect(findings, findings.join('\n')).toEqual([])
})

// The topology workspace's text fields are labelled controls (UiTextField /
// UiTextarea), in the create form and in the revision editor.
test('topology workspace text fields have accessible names', async ({ page }) => {
  await openScreen(page, 'admin-deployments')
  await page.getByTestId('edit-topology-21').click()
  const workspace = page.getByTestId('topology-workspace')
  await workspace.waitFor()
  for (const name of ['Rollout group', 'Revision message', 'Graph JSON']) {
    await expect(workspace.getByRole('textbox', { name })).toBeVisible()
  }
  const findings = await axeFindings(page, { include: '[data-testid="topology-workspace"]' })
  expect(findings.filter(item => /^(label|aria-input-field-name|aria-toggle-field-name)/.test(item)), findings.join('\n')).toEqual([])
})
