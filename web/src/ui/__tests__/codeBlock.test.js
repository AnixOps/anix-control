import { describe, expect, it } from 'vitest'
import { render, screen, waitFor } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiCodeBlock from '../UiCodeBlock.vue'

describe('UiCodeBlock', () => {
  it('shows the code in a focusable, labelled block and copies it', async () => {
    // userEvent provides the clipboard.
    const user = userEvent.setup()
    const { emitted } = render(UiCodeBlock, { props: { label: 'Clash', code: 'proxies:\n  - name: hk' } })
    const pre = document.querySelector('pre')
    expect(pre.getAttribute('tabindex')).toBe('0')
    expect(pre.getAttribute('aria-labelledby')).toBe(screen.getByText('Clash').id)
    expect(pre.textContent).toBe('proxies:\n  - name: hk')
    await user.click(screen.getByRole('button', { name: /Copy|复制/ }))
    expect(await navigator.clipboard.readText()).toBe('proxies:\n  - name: hk')
    await waitFor(() => expect(screen.getByRole('status').textContent).toMatch(/Copied|已复制/))
    expect(emitted().copy).toHaveLength(1)
  })

  it('hides the copy button when not copyable', () => {
    render(UiCodeBlock, { props: { code: 'x', copyable: false } })
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('takes a specific copy label and can block copying', async () => {
    const { rerender } = render(UiCodeBlock, { props: { label: 'config.json', code: '{}', copyLabel: 'Copy config', copyDisabled: true } })
    const button = screen.getByRole('button', { name: /Copy config/ })
    expect(button.disabled).toBe(true)
    await rerender({ copyDisabled: false })
    expect(screen.getByRole('button', { name: /Copy config/ }).disabled).toBe(false)
  })
})
