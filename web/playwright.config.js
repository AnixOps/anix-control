import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  // e2e/visual runs in the Playwright image only (playwright.visual.config.js).
  testIgnore: ['live-control-machine-telemetry.spec.js', 'visual/**'],
  timeout: 30_000,
  expect: { timeout: 5_000 },
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  // The CI runners have 4 vCPUs; Playwright's default would use 2.
  workers: process.env.CI ? 4 : undefined,
  reporter: process.env.CI ? [['line'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173',
    // retain-on-failure records every test and throws the passing ones away,
    // which cost CI about 40% of the run; a failing test is retried in CI, and
    // the retry is recorded.
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'on-first-retry',
    ...devices['Desktop Chrome'],
  },
  // CI tests the bundle `npm run build` wrote to public/ (the Frontend E2E job
  // builds it first): the dev server compiles every module on demand, which
  // made the run about 20% slower. Locally the dev server needs no build.
  webServer: {
    command: process.env.CI
      ? 'npm run preview -- --host 127.0.0.1 --port 4173 --strictPort'
      : 'npm run dev -- --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173/login',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
