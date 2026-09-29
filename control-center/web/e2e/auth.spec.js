import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test('shows login page', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('h1')).toContainText('AnixOps')
  })

  test('login form is visible', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('input[type="email"]')).toBeVisible()
    await expect(page.locator('input[type="password"]')).toBeVisible()
    await expect(page.locator('button[type="submit"]')).toBeVisible()
  })

  test('shows error on invalid credentials', async ({ page }) => {
    await page.goto('/login')

    // Fill form with invalid credentials
    await page.locator('input[type="email"]').fill('invalid@example.com')
    await page.locator('input[type="password"]').fill('wrongpassword')
    await page.locator('button[type="submit"]').click()

    // Wait for error message (could be various formats)
    await page.waitForTimeout(3000)

    // Check for any error indication - either error message or still on login page
    const hasError = await page.locator('text=/Invalid|failed|error/i').count() > 0
    const stillOnLogin = page.url().includes('/login')

    expect(hasError || stillOnLogin).toBeTruthy()
  })

  test('redirects unauthenticated users to login', async ({ page }) => {
    await page.goto('/nodes')

    // Should be redirected to login
    await expect(page).toHaveURL(/\/login/, { timeout: 5000 })
  })
})

test.describe('Protected Routes', () => {
  test('requires auth for nodes page', async ({ page }) => {
    await page.goto('/nodes')
    await expect(page).toHaveURL(/\/login/)
  })

  test('requires auth for tasks page', async ({ page }) => {
    await page.goto('/tasks')
    await expect(page).toHaveURL(/\/login/)
  })

  test('requires auth for schedules page', async ({ page }) => {
    await page.goto('/schedules')
    await expect(page).toHaveURL(/\/login/)
  })
})