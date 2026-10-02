import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiSelect from '../UiSelect.vue'
import UiCombobox from '../UiCombobox.vue'
import UiSegmentedControl from '../UiSegmentedControl.vue'
import UiTabs from '../UiTabs.vue'

const TEMPLATES = [
  { value: 'basic', label: 'Basic', description: '100 GB' },
  { value: 'standard', label: 'Standard' },
  { value: 'premium', label: 'Premium' },
  { value: 'legacy', label: 'Legacy', disabled: true }
]

describe('UiSelect', () => {
  it('is a labelled combobox that opens, selects and closes with the keyboard', async () => {
    const user = userEvent.setup()
    const value = ref('standard')
    render({
      components: { UiSelect },
      setup: () => ({ value, options: TEMPLATES }),
      template: '<UiSelect v-model="value" label="Template" help="Default for new users." :options="options" />'
    })
    const trigger = screen.getByRole('combobox', { name: 'Template' })
    await waitFor(() => expect(trigger.textContent).toContain('Standard'))
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    expect(document.getElementById(trigger.getAttribute('aria-describedby')).textContent).toBe('Default for new users.')

    trigger.focus()
    await user.keyboard('{Enter}')
    const listbox = await screen.findByRole('listbox')
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    const options = within(listbox).getAllByRole('option')
    expect(options.map(option => option.getAttribute('aria-selected'))).toEqual(['false', 'true', 'false', 'false'])
    expect(options[3].getAttribute('aria-disabled')).toBe('true')

    await user.keyboard('{ArrowDown}{Enter}')
    expect(value.value).toBe('premium')
    await waitFor(() => expect(screen.queryByRole('listbox')).toBeNull())
    expect(document.activeElement).toBe(trigger)
  })

  it('closes with Escape without changing the value', async () => {
    const user = userEvent.setup()
    const value = ref('basic')
    render({
      components: { UiSelect },
      setup: () => ({ value, options: TEMPLATES }),
      template: '<UiSelect v-model="value" label="Template" :options="options" />'
    })
    const trigger = screen.getByRole('combobox', { name: 'Template' })
    trigger.focus()
    await user.keyboard('{ArrowDown}')
    await screen.findByRole('listbox')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('listbox')).toBeNull())
    expect(value.value).toBe('basic')
    expect(document.activeElement).toBe(trigger)
  })

  it('marks an error on the trigger', () => {
    render(UiSelect, { props: { label: 'Group', options: ['HK'], error: 'Choose a group.' } })
    const trigger = screen.getByRole('combobox', { name: 'Group' })
    expect(trigger.getAttribute('aria-invalid')).toBe('true')
    expect(document.getElementById(trigger.getAttribute('aria-describedby')).textContent).toContain('Choose a group.')
  })
})

describe('UiCombobox', () => {
  it('filters as you type and selects with Enter', async () => {
    const user = userEvent.setup()
    const value = ref(undefined)
    render({
      components: { UiCombobox },
      setup: () => ({ value, options: ['hk-hkg-01', 'hk-hkg-02', 'jp-tyo-01'] }),
      template: '<UiCombobox v-model="value" label="Entry node" :options="options" />'
    })
    const input = screen.getByRole('combobox', { name: 'Entry node' })
    await user.click(input)
    await user.type(input, 'tyo')
    const listbox = await screen.findByRole('listbox')
    await waitFor(() => expect(within(listbox).getAllByRole('option')).toHaveLength(1))
    await user.keyboard('{ArrowDown}{Enter}')
    expect(value.value).toBe('jp-tyo-01')
    expect(input.value).toBe('jp-tyo-01')
  })

  it('says when nothing matches', async () => {
    const user = userEvent.setup()
    render(UiCombobox, { props: { label: 'Entry node', options: ['hk-hkg-01'] } })
    const input = screen.getByRole('combobox', { name: 'Entry node' })
    await user.click(input)
    await user.type(input, 'zzz')
    expect(await screen.findByText('No matching options')).toBeTruthy()
  })
})

describe('UiSegmentedControl', () => {
  it('is a named group of pressed buttons moved with arrow keys', async () => {
    const user = userEvent.setup()
    const value = ref('24h')
    render({
      components: { UiSegmentedControl },
      setup: () => ({ value }),
      template: `<div><button>before</button><UiSegmentedControl v-model="value" aria-label="Time range"
        :options="[{ value: '1h', label: '1 h' }, { value: '24h', label: '24 h' }, { value: '7d', label: '7 d' }]" /></div>`
    })
    const group = screen.getByRole('group', { name: 'Time range' })
    const buttons = within(group).getAllByRole('button')
    expect(buttons.map(button => button.getAttribute('aria-pressed'))).toEqual(['false', 'true', 'false'])

    // Roving focus: Tab enters the group at the selected segment.
    screen.getByRole('button', { name: 'before' }).focus()
    await user.tab()
    expect(document.activeElement).toBe(buttons[1])
    await user.keyboard('{ArrowRight}')
    expect(document.activeElement).toBe(buttons[2])
    await user.keyboard(' ')
    expect(value.value).toBe('7d')

    // Clicking the active segment keeps the selection.
    await user.click(buttons[2])
    expect(value.value).toBe('7d')
    expect(buttons[2].getAttribute('aria-pressed')).toBe('true')
  })
})

describe('UiTabs', () => {
  it('wires tabs to panels and switches with arrow keys', async () => {
    const user = userEvent.setup()
    const tab = ref('overview')
    render({
      components: { UiTabs },
      setup: () => ({ tab }),
      template: `<UiTabs v-model="tab" aria-label="Node" :items="[{ value: 'overview', label: 'Overview' }, { value: 'logs', label: 'Logs', count: 3 }]">
        <template #overview><p>Health</p></template>
        <template #logs><p>Last lines</p></template>
      </UiTabs>`
    })
    const list = screen.getByRole('tablist', { name: 'Node' })
    const tabs = within(list).getAllByRole('tab')
    expect(tabs[0].getAttribute('aria-selected')).toBe('true')
    const panel = screen.getByRole('tabpanel')
    expect(panel.getAttribute('aria-labelledby')).toBe(tabs[0].id)
    await waitFor(() => expect(tabs[0].getAttribute('aria-controls')).toBe(panel.id))
    expect(panel.textContent).toBe('Health')

    tabs[0].focus()
    await user.keyboard('{ArrowRight}')
    expect(tab.value).toBe('logs')
    expect(document.activeElement).toBe(tabs[1])
    expect(screen.getByRole('tabpanel').textContent).toBe('Last lines')
  })
})
