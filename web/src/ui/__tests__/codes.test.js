import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { encode } from 'uqr'
import UiOtpField from '../UiOtpField.vue'
import UiQrCode from '../UiQrCode.vue'

function boxes() {
  return screen.getAllByLabelText(/^Digit \d of 6$/)
}

function value() {
  return boxes().map(box => box.value).join('')
}

function renderOtp(props = {}) {
  const model = ref(props.modelValue || '')
  const completed = []
  const result = render({
    components: { UiOtpField },
    setup: () => ({ model, completed, props }),
    template: '<UiOtpField v-model="model" label="Code" v-bind="props" @complete="v => completed.push(v)" />'
  })
  return { ...result, model, completed }
}

describe('UiOtpField', () => {
  it('is a fieldset named by its legend, one labelled box per digit', () => {
    renderOtp()
    expect(screen.getByRole('group', { name: 'Code' })).toBeTruthy()
    expect(boxes()).toHaveLength(6)
    expect(boxes()[0].getAttribute('autocomplete')).toBe('one-time-code')
    expect(boxes()[1].getAttribute('autocomplete')).toBe('off')
    expect(boxes().every(box => box.getAttribute('inputmode') === 'numeric')).toBe(true)
  })

  it('moves forward as digits are typed, ignores other keys and fires complete once full', async () => {
    const user = userEvent.setup()
    const { model, completed } = renderOtp()
    await user.click(boxes()[0])
    await user.keyboard('1a2-3')
    expect(value()).toBe('123')
    expect(document.activeElement).toBe(boxes()[3])
    expect(model.value).toBe('123')
    expect(completed).toEqual([])
    await user.keyboard('456')
    expect(model.value).toBe('123456')
    expect(completed).toEqual(['123456'])
  })

  it('clears with Backspace, then steps back to the previous box', async () => {
    const user = userEvent.setup()
    const { model } = renderOtp({ modelValue: '123' })
    await user.click(boxes()[2])
    await user.keyboard('{Backspace}')
    expect(value()).toBe('12')
    expect(document.activeElement).toBe(boxes()[2])
    await user.keyboard('{Backspace}')
    expect(value()).toBe('1')
    expect(document.activeElement).toBe(boxes()[1])
    expect(model.value).toBe('1')
  })

  it('moves with the arrow keys, Home and End', async () => {
    const user = userEvent.setup()
    renderOtp()
    await user.click(boxes()[2])
    await user.keyboard('{ArrowRight}')
    expect(document.activeElement).toBe(boxes()[3])
    await user.keyboard('{ArrowLeft}{ArrowLeft}')
    expect(document.activeElement).toBe(boxes()[1])
    await user.keyboard('{End}')
    expect(document.activeElement).toBe(boxes()[5])
    await user.keyboard('{Home}')
    expect(document.activeElement).toBe(boxes()[0])
  })

  it('spreads a pasted code from the first box, whichever box it lands in', async () => {
    const user = userEvent.setup()
    const { completed } = renderOtp()
    await user.click(boxes()[3])
    await user.paste('Code: 482 913')
    expect(value()).toBe('482913')
    expect(completed).toEqual(['482913'])
  })

  it('spreads a partial paste from the box it lands in', async () => {
    const user = userEvent.setup()
    renderOtp({ modelValue: '1' })
    await user.click(boxes()[1])
    await user.paste('23')
    expect(value()).toBe('123')
    expect(document.activeElement).toBe(boxes()[3])
  })

  it('ties the error to every box', () => {
    renderOtp({ error: 'Wrong code.' })
    for (const box of boxes()) {
      expect(box.getAttribute('aria-invalid')).toBe('true')
      const ids = box.getAttribute('aria-describedby').split(' ')
      expect(ids.map(id => document.getElementById(id)?.textContent.trim())).toContain('Wrong code.')
    }
  })
})

describe('UiQrCode', () => {
  it('draws a scannable code of the value as one labelled SVG image', async () => {
    render(UiQrCode, { props: { value: 'https://panel.example.com/s/abc', label: 'Subscription QR code', caption: 'Scan me' } })
    const image = await screen.findByRole('img', { name: 'Subscription QR code' })
    const expected = encode('https://panel.example.com/s/abc', { ecc: 'M', border: 4 })
    expect(image.getAttribute('viewBox')).toBe(`0 0 ${expected.size} ${expected.size}`)
    const dark = expected.data.flat().filter(Boolean).length
    expect((image.querySelector('path').getAttribute('d').match(/M/g) || []).length).toBe(dark)
    expect(screen.getByText('Scan me')).toBeTruthy()
  })

  it('redraws when the value changes', async () => {
    const { rerender } = render(UiQrCode, { props: { value: 'a', label: 'QR' } })
    const first = (await screen.findByRole('img', { name: 'QR' })).querySelector('path').getAttribute('d')
    await rerender({ value: 'https://panel.example.com/s/a-much-longer-token-value', label: 'QR' })
    await waitFor(() => expect(screen.getByRole('img', { name: 'QR' }).querySelector('path').getAttribute('d')).not.toBe(first))
  })
})
