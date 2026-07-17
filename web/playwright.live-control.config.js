import { defineConfig, devices } from '@playwright/test'

import { liveControlWebUIURL } from './e2e/support/live-control-options.mjs'

// This intentionally does not use the normal Vite webServer or any page.route
// fixtures. globalSetup builds a temporary production frontend and starts a
// real Control process with an isolated SQLite database and signing key.
export default defineConfig({
  testDir: './e2e',
  testMatch: 'live-control-machine-telemetry.spec.js',
  globalSetup: './e2e/support/live-control-machine-telemetry.mjs',
  globalTimeout: 300_000,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  forbidOnly: true,
  reporter: process.env.CI ? [['line'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: liveControlWebUIURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    ...devices['Desktop Chrome'],
  },
})
