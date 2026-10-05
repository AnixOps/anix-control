import { test, expect } from '@playwright/test'
import { openScreen } from './support/screens.js'

// Stacking of the layers that open out of other layers. A menu or an option
// list is portalled to <body> (ui/composables/useMenuLayer.js), so what covers
// it is decided by z-index alone: a layer opened from inside a dialog, a sheet
// or the navigation drawer has to stack above them (--z-popover, styles/
// base.css) while a toast stays above all of them. Each check asks the
// browser what is at the centre of the thing a person would click
// (`elementFromPoint`): the thing itself, or a scrim over it.
test.use({ reducedMotion: 'reduce' })

// null when `locator`'s element is what the pointer reaches at its centre,
// else a description of what covers it (the failure message names it).
function coveredBy(locator) {
  return locator.evaluate(element => {
    const box = element.getBoundingClientRect()
    const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2)
    if (hit && element.contains(hit)) return null
    return hit ? `${hit.localName}.${String(hit.className).split(' ')[0]}` : 'nothing'
  })
}

// The list itself and every row are on top, each at its own centre. A long
// list scrolls inside its layer, so a row is brought into view first.
async function expectOnTop(list, rowRole) {
  await expect(list).toBeVisible()
  expect(await coveredBy(list), 'the layer is covered at its centre').toBeNull()
  const rows = list.getByRole(rowRole)
  for (let index = 0; index < await rows.count(); index++) {
    await rows.nth(index).scrollIntoViewIfNeeded()
    expect(await coveredBy(rows.nth(index)), `${rowRole} ${index} is covered`).toBeNull()
  }
}

async function openGroupSheet(page) {
  await openScreen(page, 'admin-access-groups')
  await page.getByRole('row').nth(1).click()
  const sheet = page.getByRole('dialog').filter({ hasText: 'machine-telemetry' })
  await expect(sheet).toBeVisible()
  return sheet
}

test('a row menu opened in a sheet is above its scrim and clickable', async ({ page }) => {
  const sheet = await openGroupSheet(page)
  await sheet.getByRole('button', { name: 'Actions for plugin_api/machine-telemetry' }).click()
  const menu = page.getByRole('menu')
  await expectOnTop(menu, 'menuitem')

  // The click lands on the item: the confirmation it asks for opens over the
  // sheet, and the sheet is still there under it.
  await menu.getByRole('menuitem', { name: 'Remove' }).click()
  const confirm = page.getByRole('alertdialog')
  await expect(confirm).toBeVisible()
  expect(await coveredBy(confirm), 'the confirmation is under the sheet').toBeNull()
  await page.keyboard.press('Escape')
  await expect(confirm).toBeHidden()
  await expect(sheet).toBeVisible()
})

// A toast outranks every layer a person can have open, and is reachable
// while a modal sheet's scrim is up (it would otherwise be unseen).
test('a toast raised from a sheet stays above the sheet and its scrim', async ({ page }) => {
  const sheet = await openGroupSheet(page)
  await sheet.getByRole('button', { name: 'Actions for plugin_api/machine-telemetry' }).click()
  await page.getByRole('menu').getByRole('menuitem', { name: 'Remove' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Remove' }).click()
  const toast = page.locator('.ui-toast')
  await expect(toast).toBeVisible()
  expect(await coveredBy(toast), 'the toast is under another layer').toBeNull()
  await expect(sheet).toBeVisible()
})

test('a Select list opened in a dialog is above its scrim', async ({ page }) => {
  await openScreen(page, 'admin-users')
  await page.getByRole('button', { name: 'New user' }).first().click()
  const dialog = page.getByRole('dialog', { name: 'New user' })
  await dialog.getByRole('combobox', { name: 'User type' }).click()
  await expectOnTop(page.getByRole('listbox'), 'option')
})

test('the account menu opened from the navigation drawer is above the drawer', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await openScreen(page, 'admin-users')
  await page.locator('[data-sidebar-toggle]').click()
  const drawer = page.getByRole('dialog', { name: 'Admin navigation' })
  await expect(drawer).toBeVisible()
  await drawer.locator('[data-account-menu-trigger]').click()
  await expectOnTop(page.getByRole('menu'), 'menuitem')
})
