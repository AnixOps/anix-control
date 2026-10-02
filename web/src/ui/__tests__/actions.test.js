import { describe, expect, it, vi } from 'vitest'
import { h } from 'vue'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { Plus } from '@lucide/vue'
import UiButton from '../UiButton.vue'
import UiIconButton from '../UiIconButton.vue'

describe('UiButton', () => {
  it('is a type="button" by default and activates with Enter and Space', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(UiButton, { props: { variant: 'primary', icon: Plus, onClick }, slots: { default: 'Add node' } })
    const button = screen.getByRole('button', { name: 'Add node' })
    expect(button.getAttribute('type')).toBe('button')
    expect(button.classList.contains('ui-button--primary')).toBe(true)
    // The icon is decorative.
    expect(button.querySelector('svg').getAttribute('aria-hidden')).toBe('true')
    button.focus()
    await user.keyboard('{Enter}')
    await user.keyboard(' ')
    expect(onClick).toHaveBeenCalledTimes(2)
  })

  it('keeps the name, stays focusable and swallows clicks while loading', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(UiButton, { props: { loading: true, type: 'submit', onClick }, slots: { default: 'Save' } })
    const button = screen.getByRole('button', { name: 'Save' })
    expect(button.getAttribute('aria-busy')).toBe('true')
    expect(button.getAttribute('aria-disabled')).toBe('true')
    expect(button.hasAttribute('disabled')).toBe(false)
    await user.click(button)
    expect(onClick).not.toHaveBeenCalled()
    button.focus()
    expect(document.activeElement).toBe(button)
  })

  it('does not submit a form twice while loading', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn(event => event.preventDefault())
    render({
      render: () => h('form', { onSubmit }, [h(UiButton, { type: 'submit', loading: true }, () => 'Save')])
    })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('renders a link for href and a disabled link is not focusable', () => {
    render(UiButton, { props: { href: '/docs' }, slots: { default: 'Docs' } })
    expect(screen.getByRole('link', { name: 'Docs' }).getAttribute('href')).toBe('/docs')
    render(UiButton, { props: { href: '/off', disabled: true }, slots: { default: 'Off' } })
    const off = screen.getByText('Off').closest('a')
    expect(off.getAttribute('aria-disabled')).toBe('true')
    expect(off.getAttribute('tabindex')).toBe('-1')
    expect(off.hasAttribute('href')).toBe(false)
  })

  it('uses the native disabled state on buttons', () => {
    render(UiButton, { props: { disabled: true }, slots: { default: 'Nope' } })
    expect(screen.getByRole('button', { name: 'Nope' }).hasAttribute('disabled')).toBe(true)
  })
})

describe('UiIconButton', () => {
  it('takes its accessible name and tooltip from label', () => {
    render(UiIconButton, { props: { label: 'More actions', icon: Plus } })
    const button = screen.getByRole('button', { name: 'More actions' })
    expect(button.getAttribute('title')).toBe('More actions')
    expect(button.querySelector('svg').getAttribute('aria-hidden')).toBe('true')
  })

  it('reports a toggle with aria-pressed', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    const { rerender } = render(UiIconButton, { props: { label: 'Auto refresh', icon: Plus, pressed: false, onClick } })
    const button = screen.getByRole('button', { name: 'Auto refresh' })
    expect(button.getAttribute('aria-pressed')).toBe('false')
    await user.click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
    await rerender({ label: 'Auto refresh', icon: Plus, pressed: true, onClick })
    expect(button.getAttribute('aria-pressed')).toBe('true')
  })

  it('warns when the label is missing', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    render(UiIconButton, { props: { icon: Plus } })
    expect(warn.mock.calls.some(call => String(call[0]).includes('label'))).toBe(true)
    warn.mockRestore()
  })
})
