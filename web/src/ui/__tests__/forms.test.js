import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { render, screen, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiTextField from '../UiTextField.vue'
import UiTextarea from '../UiTextarea.vue'
import UiPasswordField from '../UiPasswordField.vue'
import UiNumberField from '../UiNumberField.vue'
import UiField from '../UiField.vue'
import UiSwitch from '../UiSwitch.vue'
import UiCheckbox from '../UiCheckbox.vue'
import UiRadioGroup from '../UiRadioGroup.vue'

function describedText(element) {
  return (element.getAttribute('aria-describedby') || '')
    .split(' ')
    .filter(Boolean)
    .map(id => document.getElementById(id)?.textContent.trim())
}

describe('UiTextField', () => {
  it('ties the top label, help and error to the input', async () => {
    const { rerender } = render(UiTextField, {
      props: { label: 'Node name', help: 'Shown to users.', modelValue: '' }
    })
    const input = screen.getByLabelText('Node name')
    expect(input.tagName).toBe('INPUT')
    expect(input.hasAttribute('aria-invalid')).toBe(false)
    expect(describedText(input)).toEqual(['Shown to users.'])

    await rerender({ label: 'Node name', help: 'Shown to users.', error: 'Enter a node name.', modelValue: '' })
    expect(input.getAttribute('aria-invalid')).toBe('true')
    // The error comes first, then the help.
    expect(describedText(input)).toEqual(['Enter a node name.', 'Shown to users.'])
    // The error sits in a polite live region so it is announced.
    const errorId = input.getAttribute('aria-describedby').split(' ')[0]
    expect(document.getElementById(errorId).getAttribute('aria-live')).toBe('polite')
  })

  it('emits update:modelValue, marks required and sends attributes to the input', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn()
    const { container } = render(UiTextField, {
      props: { label: 'Email', required: true, modelValue: '', 'onUpdate:modelValue': onUpdate },
      attrs: { autocomplete: 'username', class: 'outer', name: 'email' }
    })
    const input = screen.getByLabelText(/Email/)
    expect(input.required).toBe(true)
    expect(input.getAttribute('autocomplete')).toBe('username')
    expect(input.getAttribute('name')).toBe('email')
    expect(input.classList.contains('outer')).toBe(false)
    expect(container.querySelector('.ui-field').classList.contains('outer')).toBe(true)
    // The asterisk is visual only; the native required attribute carries it.
    expect(container.querySelector('.ui-field__required').getAttribute('aria-hidden')).toBe('true')
    await user.type(input, 'a@b.c')
    expect(onUpdate).toHaveBeenLastCalledWith('a@b.c')
  })

  it('describes the input with its unit', () => {
    render(UiTextField, { props: { label: 'Speed limit', suffix: 'Mbps', modelValue: '10' } })
    expect(describedText(screen.getByLabelText('Speed limit'))).toEqual(['Mbps'])
  })
})

describe('UiTextarea', () => {
  it('labels the textarea and wires the error', () => {
    render(UiTextarea, { props: { label: 'Public key', error: 'Invalid key.', modelValue: '' } })
    const area = screen.getByLabelText('Public key')
    expect(area.tagName).toBe('TEXTAREA')
    expect(area.getAttribute('aria-invalid')).toBe('true')
    expect(describedText(area)).toEqual(['Invalid key.'])
  })
})

describe('UiPasswordField', () => {
  it('reveals with a pressed toggle that keeps its name', async () => {
    const user = userEvent.setup()
    render(UiPasswordField, { props: { label: 'Password', modelValue: 'hunter2' } })
    const input = screen.getByLabelText('Password')
    expect(input.getAttribute('type')).toBe('password')
    expect(input.getAttribute('autocomplete')).toBe('current-password')
    const toggle = screen.getByRole('button', { name: 'Show password' })
    expect(toggle.getAttribute('aria-pressed')).toBe('false')
    expect(toggle.getAttribute('aria-controls')).toBe(input.id)
    await user.click(toggle)
    expect(input.getAttribute('type')).toBe('text')
    expect(toggle.getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: 'Show password' })).toBe(toggle)
  })
})

describe('UiNumberField', () => {
  it('is a labelled spinbutton that steps with the arrow keys', async () => {
    const user = userEvent.setup()
    const value = ref(30001)
    render({
      components: { UiNumberField },
      setup: () => ({ value }),
      template: '<UiNumberField v-model="value" label="Port" :min="1" :max="65535" help="1–65535" />'
    })
    const input = screen.getByRole('spinbutton', { name: 'Port' })
    // No digit grouping: ports read 30001.
    expect(input.value).toBe('30001')
    expect(input.getAttribute('aria-valuemin')).toBe('1')
    expect(input.getAttribute('aria-valuemax')).toBe('65535')
    expect(input.hasAttribute('aria-roledescription')).toBe(false)
    expect(describedText(input)).toEqual(['1–65535'])
    input.focus()
    await user.keyboard('{ArrowUp}')
    expect(value.value).toBe(30002)
    await user.keyboard('{ArrowDown}{ArrowDown}')
    expect(value.value).toBe(30000)
  })

  it('reads the unit as part of the description', () => {
    render(UiNumberField, { props: { label: 'Speed limit', unit: 'Mbps', modelValue: 5 } })
    expect(describedText(screen.getByRole('spinbutton', { name: 'Speed limit' }))).toEqual(['Mbps'])
  })
})

describe('UiField', () => {
  it('gives a custom control its id and description', () => {
    render({
      components: { UiField },
      template: `<UiField v-slot="field" label="Colour" help="Pick one." error="Required.">
        <input :id="field.id" :aria-describedby="field.describedBy" :aria-invalid="field.invalid">
      </UiField>`
    })
    const input = screen.getByLabelText('Colour')
    expect(describedText(input)).toEqual(['Required.', 'Pick one.'])
    expect(input.getAttribute('aria-invalid')).toBe('true')
  })
})

describe('UiSwitch', () => {
  it('is a labelled switch toggled by click and Space', async () => {
    const user = userEvent.setup()
    const value = ref(false)
    render({
      components: { UiSwitch },
      setup: () => ({ value }),
      template: '<UiSwitch v-model="value" label="Allow sign-ups" description="Opens the sign-up page." />'
    })
    const control = screen.getByRole('switch', { name: 'Allow sign-ups' })
    expect(control.getAttribute('aria-checked')).toBe('false')
    expect(describedText(control)).toEqual(['Opens the sign-up page.'])
    await user.click(control)
    expect(value.value).toBe(true)
    expect(control.getAttribute('aria-checked')).toBe('true')
    control.focus()
    await user.keyboard(' ')
    expect(value.value).toBe(false)
  })

  it('accepts aria-label when there is no visible label', () => {
    render(UiSwitch, { props: { modelValue: true }, attrs: { 'aria-label': 'Auto renew' } })
    expect(screen.getByRole('switch', { name: 'Auto renew' }).getAttribute('aria-checked')).toBe('true')
  })
})

describe('UiCheckbox', () => {
  it('toggles with Space and reports the mixed state', async () => {
    const user = userEvent.setup()
    const value = ref(false)
    render({
      components: { UiCheckbox },
      setup: () => ({ value }),
      template: '<UiCheckbox v-model="value" label="I agree" />'
    })
    const box = screen.getByRole('checkbox', { name: 'I agree' })
    expect(box.getAttribute('aria-checked')).toBe('false')
    box.focus()
    await user.keyboard(' ')
    expect(value.value).toBe(true)
    expect(box.getAttribute('aria-checked')).toBe('true')
    // Clicking the label toggles it too.
    await user.click(screen.getByText('I agree'))
    expect(value.value).toBe(false)
  })

  it('reports indeterminate as aria-checked="mixed"', () => {
    render(UiCheckbox, { props: { modelValue: 'indeterminate', label: 'Select all' } })
    expect(screen.getByRole('checkbox', { name: 'Select all' }).getAttribute('aria-checked')).toBe('mixed')
  })
})

describe('UiRadioGroup', () => {
  it('is a named radiogroup moved with the arrow keys', async () => {
    const user = userEvent.setup()
    const value = ref('agent')
    render({
      components: { UiRadioGroup },
      setup: () => ({ value }),
      template: `<UiRadioGroup v-model="value" label="Run mode" help="Cannot change later."
        :options="[{ value: 'agent', label: 'Agent' }, { value: 'ansible', label: 'Ansible' }, { value: 'local', label: 'Local', disabled: true }]" />`
    })
    const group = screen.getByRole('radiogroup', { name: 'Run mode' })
    expect(describedText(group)).toEqual(['Cannot change later.'])
    const radios = within(group).getAllByRole('radio')
    expect(radios.map(radio => radio.getAttribute('aria-checked'))).toEqual(['true', 'false', 'false'])
    expect(screen.getByRole('radio', { name: 'Ansible' })).toBe(radios[1])
    radios[0].focus()
    // Hold the key: the radio checks itself on focus while an arrow key is down.
    await user.keyboard('{ArrowDown>}')
    await new Promise(resolve => setTimeout(resolve, 10))
    await user.keyboard('{/ArrowDown}')
    expect(value.value).toBe('ansible')
    expect(document.activeElement).toBe(radios[1])
  })
})
