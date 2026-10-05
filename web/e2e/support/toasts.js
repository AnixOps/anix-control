// Toasts in the e2e specs. A toast shows its text twice by design: in the
// toast list (a named "Notifications" region) and, 100 ms later, in the one
// polite live region that announces it (role="status", visually hidden). A bare
// `page.getByText(message)` therefore resolves to two elements (a Playwright
// strict-mode violation) once the announcement arrives. Assert the toast once,
// scoped to its region, and the announcement separately.
import { expect } from '@playwright/test'

// The toast element carrying `message` (a string or a RegExp).
export function toastLocator(page, message) {
  return page.getByRole('region', { name: 'Notifications' }).getByText(message)
}

// The live region's announcement of `message`.
export function announcementLocator(page, message) {
  return page.getByRole('status').filter({ hasText: message })
}

// Expects the toast, then its announcement.
export async function expectToast(page, message) {
  await expect(toastLocator(page, message)).toBeVisible()
  await expect(announcementLocator(page, message)).toHaveCount(1)
}
