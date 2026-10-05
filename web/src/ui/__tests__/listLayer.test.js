import { afterEach, describe, expect, it } from 'vitest'
import { ref, nextTick } from 'vue'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiCombobox from '../UiCombobox.vue'
import UiDialog from '../UiDialog.vue'
import UiSelect from '../UiSelect.vue'
import { inertOthers, useInertBehind } from '../composables/useInertBehind'
import { useMenuLayer } from '../composables/useMenuLayer'

// The lists of UiSelect and UiCombobox are portalled out of the page like
// the menus: each open list sits in its own labelled `region`
// (composables/useMenuLayer.js) that is gone once the list closes. A Select
// is modal in Reka (it hides the rest of the page), so the page behind it is
// made inert while it is open (composables/useInertBehind.js): a hidden
// control must not stay focusable (axe "aria-hidden-focus").

const layers = () => [...document.body.querySelectorAll(':scope > [role="region"].ui-menu-layer')]
const TEMPLATES = [{ value: 'basic', label: 'Basic' }, { value: 'standard', label: 'Standard' }, { value: 'premium', label: 'Premium' }]

// Reka moves focus into the list once it is positioned; a layout engine
// decides that, so the tests wait for it like a user would.
async function focusInside(region) {
  await waitFor(() => expect(region.contains(document.activeElement)).toBe(true))
}

describe('UiSelect list layer', () => {
  it('opens in a region named after the field and removes it on close, every time', async () => {
    const user = userEvent.setup()
    const value = ref('basic')
    render({
      components: { UiSelect },
      setup: () => ({ value, options: TEMPLATES }),
      template: '<UiSelect v-model="value" label="Template" :options="options" />'
    })
    expect(layers()).toHaveLength(0)
    const trigger = screen.getByRole('combobox', { name: 'Template' })

    // A closed Select keeps an empty placeholder in its portal; it must not
    // keep the layer alive (the layer would stay on <body> as an empty landmark).
    for (let round = 0; round < 3; round += 1) {
      trigger.focus()
      await user.keyboard('{ArrowDown}')
      const region = await screen.findByRole('region', { name: 'Template options' })
      expect(layers()).toEqual([region])
      expect(within(region).getAllByRole('option').map(option => option.textContent.trim())).toEqual(['Basic', 'Standard', 'Premium'])
      await focusInside(region)

      await user.keyboard('{Escape}')
      await waitFor(() => expect(layers()).toHaveLength(0))
      expect(screen.queryByRole('listbox')).toBeNull()
      expect(document.activeElement).toBe(trigger)
    }
    expect(value.value).toBe('basic')
  })

  it('names the region after the aria-label of a toolbar select, and "Options" without a name', async () => {
    const user = userEvent.setup()
    render({
      components: { UiSelect },
      template: '<div><UiSelect aria-label="Engine" :options="[\'nftables\', \'gost\']" /><UiSelect :options="[\'a\', \'b\']" /></div>'
    })
    const [engine, bare] = screen.getAllByRole('combobox')
    await user.click(engine)
    expect(await screen.findByRole('region', { name: 'Engine options' })).toBeTruthy()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(layers()).toHaveLength(0))

    await user.click(bare)
    expect(await screen.findByRole('region', { name: 'Options' })).toBeTruthy()
  })

  it('chooses with the keyboard through the layer and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    const value = ref('basic')
    render({
      components: { UiSelect },
      setup: () => ({ value, options: TEMPLATES }),
      template: '<UiSelect v-model="value" label="Template" :options="options" />'
    })
    const trigger = screen.getByRole('combobox', { name: 'Template' })
    trigger.focus()
    await user.keyboard('{Enter}')
    await focusInside(await screen.findByRole('region', { name: 'Template options' }))
    await user.keyboard('{ArrowDown}{ArrowDown}{Enter}')
    await waitFor(() => expect(value.value).toBe('premium'))
    await waitFor(() => expect(layers()).toHaveLength(0))
    expect(document.activeElement).toBe(trigger)
  })

  it('removes the layer when the Select is unmounted while open', async () => {
    const user = userEvent.setup()
    const { unmount } = render(UiSelect, { props: { label: 'Template', options: TEMPLATES } })
    await user.click(screen.getByRole('combobox', { name: 'Template' }))
    await screen.findByRole('region', { name: 'Template options' })
    unmount()
    expect(layers()).toHaveLength(0)
    expect(document.querySelectorAll('[inert]')).toHaveLength(0)
  })

  it('makes the page behind the open list inert, and only while it is open', async () => {
    const user = userEvent.setup()
    render({
      components: { UiSelect },
      template: `<div>
        <a href="#top" data-testid="link">Skip</a>
        <main><UiSelect label="Template" :options="['Basic', 'Standard']" /><button type="button">Other</button></main>
        <section aria-live="polite" data-testid="live"><button type="button">Undo</button></section>
      </div>`
    })
    const trigger = screen.getByRole('combobox', { name: 'Template' })
    expect(document.querySelectorAll('[inert]')).toHaveLength(0)

    trigger.focus()
    await user.keyboard('{ArrowDown}')
    const region = await screen.findByRole('region', { name: 'Template options' })
    await focusInside(region)
    await waitFor(() => expect(document.querySelector('[inert]')).toBeTruthy())
    // The control that opened the list, and the controls next to it, cannot
    // take focus while the list holds it ...
    expect(trigger.closest('[inert]')).toBeTruthy()
    expect(screen.getByTestId('link').closest('[inert]')).toBeTruthy()
    expect(screen.getByText('Other').closest('[inert]')).toBeTruthy()
    // ... the list and what carries an aria-live attribute (Reka's own
    // exemption in its hide-others pass) stay available.
    expect(region.closest('[inert]')).toBeNull()
    expect(screen.getByTestId('live').closest('[inert]')).toBeNull()

    await user.keyboard('{Escape}')
    expect(document.querySelectorAll('[inert]')).toHaveLength(0)
    await waitFor(() => expect(document.activeElement).toBe(trigger))
  })

  it('stays readable and usable in a modal dialog, and leaves the dialog usable', async () => {
    const user = userEvent.setup()
    const value = ref('basic')
    const open = ref(true)
    render({
      components: { UiDialog, UiSelect },
      setup: () => ({ value, open, options: TEMPLATES }),
      template: `<div>
        <UiDialog v-model:open="open" title="New plan">
          <UiSelect v-model="value" label="Template" :options="options" />
        </UiDialog>
      </div>`
    })
    const dialog = await screen.findByRole('dialog', { name: 'New plan' })
    // Reka hid what was on the page when the dialog opened.
    expect(document.body.querySelector('[data-aria-hidden="true"]')).toBeTruthy()

    const trigger = within(dialog).getByRole('combobox', { name: 'Template' })
    trigger.focus()
    await user.keyboard('{Enter}')
    const region = await screen.findByRole('region', { name: 'Template options' })
    // The layer was made after the dialog hid the page, so it is not hidden.
    expect(region.closest('[aria-hidden="true"]')).toBeNull()
    expect(region.closest('[inert]')).toBeNull()
    expect(within(region).getAllByRole('option')).toHaveLength(3)
    await focusInside(region)

    // Escape closes the list only; the dialog stays, with focus on the trigger.
    await user.keyboard('{Escape}')
    await waitFor(() => expect(layers()).toHaveLength(0))
    expect(screen.getByRole('dialog', { name: 'New plan' })).toBeTruthy()
    expect(document.querySelectorAll('[inert]')).toHaveLength(0)
    await waitFor(() => expect(document.activeElement).toBe(trigger))
    expect(open.value).toBe(true)

    // And a choice made in it lands in the field.
    await user.keyboard('{Enter}')
    await focusInside(await screen.findByRole('region', { name: 'Template options' }))
    await user.keyboard('{ArrowDown}{Enter}')
    await waitFor(() => expect(value.value).toBe('standard'))
    await waitFor(() => expect(layers()).toHaveLength(0))
    expect(screen.getByRole('dialog', { name: 'New plan' })).toBeTruthy()
  })
})

describe('UiCombobox list layer', () => {
  it('opens in a region named after the field and removes it on close', async () => {
    const user = userEvent.setup()
    render(UiCombobox, { props: { label: 'Entry node', options: ['hk-hkg-01', 'jp-tyo-01'] } })
    const input = screen.getByRole('combobox', { name: 'Entry node' })
    expect(layers()).toHaveLength(0)

    await user.click(input)
    const region = await screen.findByRole('region', { name: 'Entry node options' })
    expect(layers()).toEqual([region])
    expect(within(region).getAllByRole('option')).toHaveLength(2)

    await user.keyboard('{Escape}')
    await waitFor(() => expect(layers()).toHaveLength(0))
    // The page is not made inert: focus stays in the input while the list is open.
    expect(document.querySelectorAll('[inert]')).toHaveLength(0)
  })

  it('keeps typing, filtering and choosing in the input', async () => {
    const user = userEvent.setup()
    const value = ref(undefined)
    render({
      components: { UiCombobox },
      setup: () => ({ value }),
      template: '<UiCombobox v-model="value" aria-label="Node" :options="[\'hk-hkg-01\', \'jp-tyo-01\']" />'
    })
    const input = screen.getByRole('combobox', { name: 'Node' })
    await user.click(input)
    await user.type(input, 'tyo')
    const region = await screen.findByRole('region', { name: 'Node options' })
    await waitFor(() => expect(within(region).getAllByRole('option')).toHaveLength(1))
    await user.keyboard('{ArrowDown}{Enter}')
    expect(value.value).toBe('jp-tyo-01')
    await waitFor(() => expect(layers()).toHaveLength(0))
    expect(document.activeElement).toBe(input)
  })
})

describe('useMenuLayer', () => {
  // What Reka's Select does: on close it takes the list out and, in the same
  // patch, puts an empty placeholder <div> (no role) into its portal.
  function mount(open) {
    let layer
    const view = render({ setup() { layer = useMenuLayer(open, () => 'Things'); return () => null } })
    return { layer, view }
  }
  afterEach(() => { document.body.innerHTML = '' })

  it('goes when the list is replaced by a placeholder without a role', async () => {
    const open = ref(false)
    const { layer } = mount(open)
    open.value = true
    expect(layers()).toEqual([layer])
    const list = document.createElement('div')
    list.setAttribute('role', 'listbox')
    layer.append(list)
    await nextTick()

    open.value = false
    // Still there: the list has not been taken out yet.
    expect(layers()).toEqual([layer])
    list.remove()
    layer.append(document.createElement('div'))
    await waitFor(() => expect(layers()).toHaveLength(0))
  })

  it('goes at once when it closes before anything was rendered into it', () => {
    const open = ref(false)
    const { layer } = mount(open)
    layer.append(document.createElement('div'))
    open.value = true
    expect(layers()).toEqual([layer])
    open.value = false
    expect(layers()).toHaveLength(0)
  })
})

describe('inertOthers', () => {
  function page() {
    document.body.innerHTML = `
      <script id="script"></script>
      <div id="app">
        <header id="top"><a href="#x">Skip</a></header>
        <main id="main"><button id="inner">Inner</button><section id="live" aria-live="polite"><button>Undo</button></section></main>
        <footer id="foot"></footer>
      </div>
      <span id="guard" data-reka-focus-guard tabindex="0"></span>
      <div id="done" inert></div>
      <div id="layer"><div id="list"></div></div>`
    return document.getElementById('layer')
  }
  afterEach(() => { document.body.innerHTML = '' })
  const inertIds = () => [...document.querySelectorAll('[inert]')].map(element => element.id).sort()

  it('inerts what is outside the layer and the live regions, nothing else', () => {
    const layer = page()
    const release = inertOthers(layer)
    // #app holds a live region, so it stays and its other children go inert,
    // the way Reka's hide-others pass treats them. Scripts, Reka's focus
    // guards and what was inert already are left alone.
    expect(inertIds()).toEqual(['done', 'foot', 'inner', 'top'])
    expect(layer.hasAttribute('inert')).toBe(false)
    expect(document.getElementById('live').hasAttribute('inert')).toBe(false)
    expect(document.getElementById('main').hasAttribute('inert')).toBe(false)
    expect(document.getElementById('script').hasAttribute('inert')).toBe(false)
    expect(document.getElementById('guard').hasAttribute('inert')).toBe(false)

    release()
    // Only what it set is taken off: the element that was inert before stays.
    expect(inertIds()).toEqual(['done'])
  })

  it('inerts whole top-level siblings when there is no live region', () => {
    const layer = page()
    document.getElementById('live').remove()
    inertOthers(layer)
    expect(inertIds()).toEqual(['app', 'done'])
  })
})

describe('useInertBehind', () => {
  function mount(open) {
    document.body.innerHTML = '<div id="page"><button>Page</button></div>'
    const layer = document.createElement('div')
    const inner = document.createElement('button')
    layer.append(inner)
    document.body.append(layer)
    const view = render({ setup() { useInertBehind(open, layer); return () => null } })
    return { layer, inner, view }
  }
  afterEach(() => { document.body.innerHTML = '' })

  it('waits for focus to enter the layer, then releases when it closes', async () => {
    const open = ref(false)
    const { inner } = mount(open)
    const page = document.getElementById('page')
    open.value = true
    await nextTick()
    // The trigger's own subtree is not touched while it may still hold focus.
    expect(page.hasAttribute('inert')).toBe(false)

    inner.focus()
    expect(page.hasAttribute('inert')).toBe(true)

    open.value = false
    // Synchronously: before Reka hands focus back to the trigger.
    expect(page.hasAttribute('inert')).toBe(false)
  })

  it('applies at once when focus is already inside, and not after it closed before focus arrived', async () => {
    const open = ref(false)
    const { layer, inner } = mount(open)
    const page = document.getElementById('page')
    inner.focus()
    open.value = true
    expect(page.hasAttribute('inert')).toBe(true)
    open.value = false
    expect(page.hasAttribute('inert')).toBe(false)

    inner.blur()
    open.value = true
    open.value = false
    layer.dispatchEvent(new FocusEvent('focusin', { bubbles: true }))
    expect(page.hasAttribute('inert')).toBe(false)
  })

  it('releases when the component is unmounted while open', async () => {
    const open = ref(false)
    const { inner, view } = mount(open)
    open.value = true
    inner.focus()
    expect(document.getElementById('page').hasAttribute('inert')).toBe(true)
    view.unmount()
    expect(document.getElementById('page').hasAttribute('inert')).toBe(false)
  })
})
