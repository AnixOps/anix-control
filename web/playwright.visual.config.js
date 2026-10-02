import { defineConfig, devices } from '@playwright/test'

// Visual regression (plan §15): screenshots of key screens with the API
// mocked, compared with the baselines in e2e/visual/__screenshots__/.
// Run only inside the official Playwright image so fonts and rendering
// match the baselines: `npm run test:visual` (docker) locally, the
// Frontend Visual job in CI. `npm run test:visual:update` rewrites them.
export default defineConfig({
  testDir: './e2e/visual',
  snapshotPathTemplate: '{testDir}/__screenshots__/{arg}{ext}',
  timeout: 60_000,
  expect: {
    timeout: 10_000,
    toHaveScreenshot: { animations: 'disabled', caret: 'hide', scale: 'css', maxDiffPixelRatio: 0.001 }
  },
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  reporter: process.env.CI ? [['line'], ['html', { open: 'never', outputFolder: 'playwright-visual-report' }]] : 'list',
  outputDir: 'test-results/visual',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:4174',
    deviceScaleFactor: 1,
    locale: 'en-US',
    timezoneId: 'UTC',
    colorScheme: 'light',
    reducedMotion: 'reduce',
    trace: 'off'
  },
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 4174 --strictPort',
    url: 'http://127.0.0.1:4174/login',
    reuseExistingServer: false,
    timeout: 120_000
  }
})
