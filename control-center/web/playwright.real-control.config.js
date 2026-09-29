import { defineConfig, devices } from '@playwright/test'

import { realControlAPIURL, realControlWebURL } from './e2e/support/real-control.mjs'

export default defineConfig({
  testDir: './e2e',
  testMatch: 'real-control.spec.js',
  globalSetup: './e2e/support/real-control.mjs',
  globalTimeout: 180_000,
  timeout: 45_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  forbidOnly: true,
  reporter: process.env.CI ? [['line'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: realControlWebURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${new URL(realControlWebURL).port}`,
    url: realControlWebURL,
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      ...process.env,
      VITE_KERNEL_API_URL: realControlAPIURL,
    },
  },
})
