import { test, expect } from '@playwright/test'
import { openScreen } from '../support/screens.js'

// Visual baselines of key screens (plan §15). Deterministic by design: the
// API is mocked (e2e/fixtures), Date.now() is fixed, animations are off
// (reduced motion + toHaveScreenshot's animations: 'disabled'), the locale
// is English so text renders in the bundled Inter, the time zone is UTC and
// the run happens in the official Playwright image (fonts and rasterizer).
// Regions that still change between runs are masked.
const DESKTOP = { width: 1440, height: 900 }
const PHONE = { width: 390, height: 844 }

// [screen, viewports]: every screen light and dark on desktop; a few on a phone.
const SHOTS = [
  ['login', [DESKTOP, PHONE]],
  ['user-home', [DESKTOP, PHONE]],
  ['user-subscribe', [DESKTOP]],
  ['admin-dashboard', [DESKTOP, PHONE]],
  ['admin-users', [DESKTOP, PHONE]],
  ['admin-node-detail', [DESKTOP]],
  ['admin-node-services', [DESKTOP]],
  ['admin-forward', [DESKTOP]],
  // The v4.2 forwarding pages (F5b).
  ['admin-forward-overview', [DESKTOP]],
  ['admin-forward-routes', [DESKTOP, PHONE]],
  ['admin-forward-routes-bulk', [DESKTOP]],
  ['admin-forward-editor', [DESKTOP, PHONE]],
  ['admin-forward-editor-preview', [DESKTOP]],
  ['admin-forward-route', [DESKTOP]],
  ['admin-forward-nodes', [DESKTOP, PHONE]],
  ['admin-forward-node', [DESKTOP]],
  // Entry HA through DNS (L2).
  ['admin-forward-dns', [DESKTOP, PHONE]],
  ['admin-forward-route-ha', [DESKTOP, PHONE]],
  ['admin-system', [DESKTOP]],
  ['admin-monitor', [DESKTOP]],
  ['admin-plugins', [DESKTOP]],
  ['admin-users-empty', [DESKTOP]],
  ['admin-dashboard-error', [DESKTOP]]
]

// Text caret, the QR code's random-looking modules stay (same token), but
// anything marked data-visual-mask is hidden behind a solid box.
const MASKS = page => [page.locator('[data-visual-mask]')]

for (const [name, viewports] of SHOTS) {
  for (const viewport of viewports) {
    for (const theme of ['light', 'dark']) {
      if (viewport === PHONE && theme === 'dark') continue
      const device = viewport === PHONE ? 'phone' : 'desktop'
      test(`${name} ${theme} ${device}`, async ({ page }) => {
        await page.setViewportSize(viewport)
        await page.emulateMedia({ colorScheme: theme, reducedMotion: 'reduce' })
        await openScreen(page, name, { theme, clock: true })
        await page.evaluate(() => document.fonts.ready)
        // Let charts and lazy panels draw their final frame.
        await page.waitForTimeout(600)
        await expect(page).toHaveScreenshot(`${name}-${theme}-${device}.png`, { fullPage: true, mask: MASKS(page) })
      })
    }
  }
}
