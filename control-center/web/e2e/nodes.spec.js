import { test, expect } from '@playwright/test'

// Helper to login
async function login(page, email = 'admin@example.com', password = 'admin123456') {
  await page.goto('/login')
  await page.locator('input[type="email"]').fill(email)
  await page.locator('input[type="password"]').fill(password)
  await page.locator('button[type="submit"]').click()

  // Wait for redirect to dashboard or error
  try {
    await page.waitForURL('/', { timeout: 10000 })
    return true
  } catch {
    return false
  }
}

test.describe('Nodes Page (requires auth)', () => {
  // Note: These tests require a valid test account in the API

  test.skip('displays nodes list after login', async ({ page }) => {
    const loggedIn = await login(page)
    test.skip(!loggedIn, 'Could not login with test credentials')

    await page.goto('/nodes')

    // Check for page header
    await expect(page.locator('h1')).toContainText('Nodes')

    // Check for add button
    await expect(page.locator('button:has-text("Add Node")')).toBeVisible()
  })

  test.skip('can open create node modal', async ({ page }) => {
    const loggedIn = await login(page)
    test.skip(!loggedIn, 'Could not login with test credentials')

    await page.goto('/nodes')
    await page.locator('button:has-text("Add Node")').click()

    // Check modal appears
    await expect(page.locator('text=Add Node')).toBeVisible()
    await expect(page.locator('input[name="name"]')).toBeVisible()
    await expect(page.locator('input[name="host"]')).toBeVisible()
  })

  test.skip('filters nodes by status', async ({ page }) => {
    const loggedIn = await login(page)
    test.skip(!loggedIn, 'Could not login with test credentials')

    await page.goto('/nodes')

    // Select online filter
    await page.locator('select').selectOption('online')

    // URL might update or list might filter
    // Implementation depends on actual behavior
  })
})

test.describe('Tasks Page (requires auth)', () => {
  test.skip('displays tasks list after login', async ({ page }) => {
    const loggedIn = await login(page)
    test.skip(!loggedIn, 'Could not login with test credentials')

    await page.goto('/tasks')

    // Check for page header
    await expect(page.locator('h1')).toContainText('Tasks')
  })

  test.skip('shows task statistics', async ({ page }) => {
    const loggedIn = await login(page)
    test.skip(!loggedIn, 'Could not login with test credentials')

    await page.goto('/tasks')

    // Check for stat cards
    await expect(page.locator('.stat-card')).toHaveCount(4)
  })
})

test.describe('Schedules Page (requires auth)', () => {
  test.skip('displays schedules after login', async ({ page }) => {
    const loggedIn = await login(page)
    test.skip(!loggedIn, 'Could not login with test credentials')

    await page.goto('/schedules')

    // Check for page header
    await expect(page.locator('h1')).toContainText('Schedules')
  })

  test.skip('can open create schedule modal', async ({ page }) => {
    const loggedIn = await login(page)
    test.skip(!loggedIn, 'Could not login with test credentials')

    await page.goto('/schedules')
    await page.locator('button:has-text("New Schedule")').click()

    // Check modal appears
    await expect(page.locator('text=Create Schedule')).toBeVisible()
  })
})