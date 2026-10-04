// Captures the F5b forwarding mockups (docs/design/forward-ui/README.md).
//
//   cd web && npx vite --port 4190 &
//   node ../docs/design/forward-ui/capture.mjs <out-dir> [--webp <dir>] [--only name]
//
// Runs on the host's Chromium (CJK fonts needed for the zh-CN copy). The API
// is mocked like e2e/support/screens.js: an administrator signed in, no
// extensions; the mockup screens themselves call nothing.
import { createRequire } from 'node:module'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const webDir = process.env.WEB_DIR || path.resolve(process.cwd())
const require = createRequire(path.join(webDir, 'package.json'))
const { chromium } = require('@playwright/test')

const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:4190'
const MOCK_NOW = Date.UTC(2026, 9, 4, 6, 0, 0)
const args = process.argv.slice(2)
const outDir = args[0]
const webpDir = args.includes('--webp') ? args[args.indexOf('--webp') + 1] : ''
const only = args.includes('--only') ? args[args.indexOf('--only') + 1] : ''

const SCREENS = [
  ['01-routes', '/admin/__mockups/forward/routes'],
  ['01b-routes-bulk', '/admin/__mockups/forward/routes?select=1'],
  ['01c-routes-empty', '/admin/__mockups/forward/routes?state=empty'],
  ['02-editor-violations', '/admin/__mockups/forward/editor'],
  ['02b-editor-preview', '/admin/__mockups/forward/editor?state=valid'],
  ['03-route-detail', '/admin/__mockups/forward/route'],
  ['04-nodes', '/admin/__mockups/forward/nodes'],
  ['04b-node-detail', '/admin/__mockups/forward/node'],
  ['05-overview', '/admin/__mockups/forward/overview']
]
const VIEWPORTS = [['desktop', { width: 1440, height: 900 }], ['phone', { width: 390, height: 844 }]]
const THEMES = ['light', 'dark']

const ADMIN = { id: 1, email: 'admin@example.com', is_admin: true }
const API = {
  '/api/v4/public/config': { edition: 'community', registration: { enabled: true } },
  '/api/v2/user/profile': { code: 0, data: ADMIN },
  '/api/v2/user/info': { code: 0, data: ADMIN },
  '/api/v2/admin/system/info': { code: 0, data: { version: '4.2.0' } },
  '/api/v3/extensions': [],
  '/api/v2/admin/notifications/unread': { code: 0, data: { count: 0 } }
}

mkdirSync(outDir, { recursive: true })
if (webpDir) mkdirSync(webpDir, { recursive: true })
const browser = await chromium.launch()
const files = []
for (const [name, url] of SCREENS) {
  if (only && !name.includes(only)) continue
  for (const [device, viewport] of VIEWPORTS) {
    for (const theme of THEMES) {
      const context = await browser.newContext({ viewport, deviceScaleFactor: 1, locale: 'zh-CN', timezoneId: 'Asia/Shanghai', colorScheme: theme, reducedMotion: 'reduce' })
      const page = await context.newPage()
      await page.addInitScript(({ theme, profile }) => {
        localStorage.setItem('app.locale', 'zh-CN')
        localStorage.setItem('v2board-theme', theme)
        localStorage.setItem('token', 'mockup-token')
        localStorage.setItem('userInfo', JSON.stringify(profile))
      }, { theme, profile: ADMIN })
      await page.clock.setFixedTime(MOCK_NOW)
      await page.route(u => u.pathname.startsWith('/api/'), route => {
        const pathname = new URL(route.request().url()).pathname
        const body = pathname in API ? API[pathname] : { code: 0, data: null }
        return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) })
      })
      await page.goto(BASE_URL + url, { waitUntil: 'networkidle' })
      await page.locator('.fwd-screen').first().waitFor()
      // The screen switcher is review chrome, not part of the design.
      await page.addStyleTag({ content: '[data-mockup-chrome]{display:none!important}' })
      await page.evaluate(() => document.fonts.ready)
      await page.waitForTimeout(900)
      const file = path.join(outDir, `${name}-${device}-${theme}.png`)
      await page.screenshot({ path: file, fullPage: true, animations: 'disabled', caret: 'hide' })
      files.push(file)
      if (webpDir) {
        const png = readFileSync(file).toString('base64')
        const webp = await page.evaluate(async src => {
          const img = new Image()
          img.src = `data:image/png;base64,${src}`
          await img.decode()
          const canvas = document.createElement('canvas')
          canvas.width = img.naturalWidth
          canvas.height = img.naturalHeight
          canvas.getContext('2d').drawImage(img, 0, 0)
          return canvas.toDataURL('image/webp', 0.82).split(',')[1]
        }, png)
        writeFileSync(path.join(webpDir, `${name}-${device}-${theme}.webp`), Buffer.from(webp, 'base64'))
      }
      await context.close()
    }
  }
}
await browser.close()
console.log(files.join('\n'))
