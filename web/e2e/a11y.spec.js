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
