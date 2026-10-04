import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiDialog from '../UiDialog.vue'
import UiMenu from '../UiMenu.vue'

// An open menu is portalled out of the page. It sits in its own labelled
// `region` (composables/useMenuLayer.js) so axe finds no content outside a
// landmark, and the layer exists only while the menu is open.

const layers = () => [...document.body.querySelectorAll(':scope > [role="region"].ui-menu-layer')]
const items = onSelect => [{ key: 'edit', label: 'Edit', onSelect }, { key: 'delete', label: 'Delete', danger: true }]

describe('UiMenu layer', () => {
  it('puts the open menu in a region named after the trigger and removes it on close', async () => {
    const user = userEvent.setup()
    render(UiMenu, { props: { label: 'Actions for Alice', items: items() } })
    expect(layers()).toHaveLength(0)

    const trigger = screen.getByRole('button', { name: 'Actions for Alice' })
    await user.click(trigger)
    const region = await screen.findByRole('region', { name: 'Actions for Alice' })
    expect(layers()).toEqual([region])
    const menu = within(region).getByRole('menu')
    expect(within(menu).getAllByRole('menuitem').map(item => item.textContent.trim())).toEqual(['Edit', 'Delete'])

    await user.keyboard('{Escape}')
    await waitFor(() => expect(layers()).toHaveLength(0))
    expect(screen.queryByRole('menu')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('names the region "Menu" for a trigger without a label', async () => {
    const user = userEvent.setup()
    render(UiMenu, { props: { items: items() }, slots: { trigger: '<button type="button">More</button>' } })
    await user.click(screen.getByRole('button', { name: 'More' }))
    expect(await screen.findByRole('region', { name: 'Menu' })).toBeTruthy()
  })

  it('runs the chosen item, drops the layer, and opens again in a fresh one', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()
    render(UiMenu, { props: { label: 'Row actions', items: items(onSelect) } })
    const trigger = screen.getByRole('button', { name: 'Row actions' })

    await user.click(trigger)
    await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
    await waitFor(() => expect(onSelect).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(layers()).toHaveLength(0))

    await user.click(trigger)
    expect(await screen.findByRole('region', { name: 'Row actions' })).toBeTruthy()
    expect(layers()).toHaveLength(1)
  })

  it('removes the layer when the menu is unmounted while open', async () => {
    const user = userEvent.setup()
    const { unmount } = render(UiMenu, { props: { label: 'Row actions', items: items() } })
    await user.click(screen.getByRole('button', { name: 'Row actions' }))
    await screen.findByRole('region', { name: 'Row actions' })
    unmount()
    expect(layers()).toHaveLength(0)
  })

  it('stays readable in a modal dialog: the layer is made after the dialog hid the rest of the page', async () => {
    const user = userEvent.setup()
    const Harness = {
      components: { UiDialog, UiMenu },
      setup: () => ({ open: ref(true), rows: items() }),
      template: `<div>
        <UiDialog v-model:open="open" title="Edit user">
          <UiMenu label="Actions in the dialog" :items="rows" />
        </UiDialog>
      </div>`
    }
    render(Harness)
    const dialog = await screen.findByRole('dialog')
    // Reka hid what was on the page when the dialog opened.
    expect(document.body.querySelector('[data-aria-hidden="true"]')).toBeTruthy()

    await user.click(within(dialog).getByRole('button', { name: 'Actions in the dialog' }))
    const region = await screen.findByRole('region', { name: 'Actions in the dialog' })
    expect(region.closest('[aria-hidden="true"]')).toBeNull()
    expect(screen.getByRole('menu')).toBeTruthy()
  })
})
