import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { Server } from '@lucide/vue'
import UiToastRegion from '../UiToastRegion.vue'
import UiCopyField from '../UiCopyField.vue'
import UiSkeleton from '../UiSkeleton.vue'
import UiBadge from '../UiBadge.vue'
import UiStatusDot from '../UiStatusDot.vue'
import UiEmptyState from '../UiEmptyState.vue'
import UiPageHeader from '../UiPageHeader.vue'
import UiGroupedList from '../UiGroupedList.vue'
import UiGroupedListRow from '../UiGroupedListRow.vue'
import UiSwitch from '../UiSwitch.vue'
import UiCard from '../UiCard.vue'
import { resetToasts, toastState, useToast } from '../composables/useToast'
import { useDelayedLoading } from '../composables/useDelayedLoading'
import { STATUS_TONES } from '../status'

describe('useToast() and UiToastRegion', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetToasts()
  })

  afterEach(() => {
    resetToasts()
    vi.useRealTimers()
  })

  it('auto-dismisses success after 3 s and keeps errors until dismissed', async () => {
    render(UiToastRegion)
    const toast = useToast()
    toast.success('Link copied')
    toast.error('Could not save settings.')
    await nextTick()
    const list = within(screen.getByRole('region', { name: 'Notifications' }))
    expect(list.getByText('Link copied')).toBeTruthy()

    await vi.advanceTimersByTimeAsync(2900)
    expect(list.queryByText('Link copied')).toBeTruthy()
    await vi.advanceTimersByTimeAsync(200)
    expect(list.queryByText('Link copied')).toBeNull()

    await vi.advanceTimersByTimeAsync(60000)
    const error = list.getByText('Could not save settings.').closest('li')
    await fireEvent.click(within(error).getByRole('button', { name: 'Dismiss notification' }))
    expect(list.queryByText('Could not save settings.')).toBeNull()
  })

  it('offers Undo for 5 s and runs the callback', async () => {
    render(UiToastRegion)
    const undo = vi.fn()
    useToast().success('Rule deleted', { undo })
    await nextTick()
    const button = screen.getByRole('button', { name: 'Undo' })
    await vi.advanceTimersByTimeAsync(4000)
    expect(screen.queryByText('Rule deleted')).toBeTruthy()
    await fireEvent.click(button)
    expect(undo).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('Rule deleted')).toBeNull()

    useToast().success('Rule deleted again', { undo })
    await vi.advanceTimersByTimeAsync(5100)
    expect(screen.queryByText('Rule deleted again')).toBeNull()
  })

  it('announces each toast once through a single live region, with the F8 hint for actions', async () => {
    const { container } = render(UiToastRegion)
    const live = container.querySelectorAll('[aria-live]')
    expect(live).toHaveLength(1)
    expect(live[0].getAttribute('role')).toBe('status')

    useToast().success('Link copied')
    await vi.advanceTimersByTimeAsync(150)
    expect(live[0].textContent).toBe('Link copied')
    useToast().success('Users banned', { undo: () => {} })
    await vi.advanceTimersByTimeAsync(150)
    expect(live[0].textContent).toBe('Users banned Press F8 to go to notifications.')
  })

  it('pauses timers while hovered and resumes after', async () => {
    render(UiToastRegion)
    useToast().success('Saved')
    await nextTick()
    const region = screen.getByRole('region', { name: 'Notifications' })
    const list = region
    await vi.advanceTimersByTimeAsync(2000)
    await fireEvent.mouseEnter(region)
    await vi.advanceTimersByTimeAsync(10000)
    expect(within(list).queryByText('Saved')).toBeTruthy()
    await fireEvent.mouseLeave(region)
    await vi.advanceTimersByTimeAsync(900)
    expect(within(list).queryByText('Saved')).toBeTruthy()
    await vi.advanceTimersByTimeAsync(200)
    expect(within(list).queryByText('Saved')).toBeNull()
  })

  it('F8 moves focus to the toasts and Esc returns it', async () => {
    render({
      components: { UiToastRegion },
      template: '<div><button>Save</button><UiToastRegion /></div>'
    })
    const save = screen.getByRole('button', { name: 'Save' })
    save.focus()
    useToast().error('Could not save settings.')
    await nextTick()
    await fireEvent.keyDown(window, { key: 'F8' })
    expect(document.activeElement.getAttribute('aria-label')).toBe('Dismiss notification')
    await fireEvent.keyDown(document.activeElement, { key: 'Escape' })
    expect(toastState.toasts).toHaveLength(0)
    expect(document.activeElement).toBe(save)
  })

  it('keeps at most three toasts, dropping the oldest non-error', () => {
    const toast = useToast()
    toast.error('E1')
    toast.success('S1')
    toast.success('S2')
    toast.success('S3')
    expect(toastState.toasts.map(item => item.message)).toEqual(['E1', 'S2', 'S3'])
  })
})

function stubClipboard(value) {
  Object.defineProperty(window.navigator, 'clipboard', { value, configurable: true })
}

// These tests click with fireEvent: user-event installs its own clipboard.
describe('UiCopyField', () => {
  afterEach(() => {
    stubClipboard(undefined)
    delete document.execCommand
  })

  it('copies, says "Copied" in the button and a status region, then resets', async () => {
    const writeText = vi.fn().mockResolvedValue()
    stubClipboard({ writeText })
    const onCopy = vi.fn()
    render(UiCopyField, { props: { label: 'Subscription link', value: 'https://panel.example/s/abc', onCopy } })
    const input = screen.getByLabelText('Subscription link')
    expect(input.readOnly).toBe(true)
    expect(input.value).toBe('https://panel.example/s/abc')
    const button = screen.getByRole('button', { name: 'Copy' })
    expect(button.getAttribute('aria-describedby')).toBeTruthy()
    await fireEvent.click(button)
    await nextTick()
    expect(writeText).toHaveBeenCalledWith('https://panel.example/s/abc')
    expect(onCopy).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: 'Copied' })).toBe(button)
    expect(screen.getByRole('status').textContent).toBe('Copied')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copy' })).toBeTruthy(), { timeout: 3000 })
  })

  it('masks a secret as a password field with a pressed reveal toggle', async () => {
    const user = userEvent.setup()
    render(UiCopyField, { props: { label: 'Token', value: 'anx_reg_123', secret: true } })
    const input = screen.getByLabelText('Token')
    expect(input.getAttribute('type')).toBe('password')
    const reveal = screen.getByRole('button', { name: 'Show value' })
    expect(reveal.getAttribute('aria-pressed')).toBe('false')
    await user.click(reveal)
    expect(input.getAttribute('type')).toBe('text')
    expect(reveal.getAttribute('aria-pressed')).toBe('true')
  })

  it('falls back to execCommand without the clipboard API', async () => {
    const execCommand = vi.fn(() => true)
    document.execCommand = execCommand
    render(UiCopyField, { props: { label: 'Link', value: 'http://panel/s/1' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    await nextTick()
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(screen.getByRole('button', { name: 'Copied' })).toBeTruthy()
  })

  it('selects the text and explains when copying is not possible', async () => {
    const execCommand = vi.fn(() => false)
    document.execCommand = execCommand
    render(UiCopyField, { props: { label: 'Token', value: 'anx_reg_123', secret: true } })
    await fireEvent.click(screen.getByRole('button', { name: 'Copy' }))
    await nextTick()
    expect(execCommand).toHaveBeenCalledWith('copy')
    const input = screen.getByLabelText('Token')
    expect(input.getAttribute('type')).toBe('text')
    expect(document.activeElement).toBe(input)
    expect(input.getAttribute('aria-invalid')).toBeNull()
    expect(document.getElementById(input.getAttribute('aria-describedby').split(' ')[0]).textContent).toContain('press Ctrl+C')
  })
})

describe('Skeleton and useDelayedLoading', () => {
  it('announces loading once and hides the bones', () => {
    const { container } = render(UiSkeleton, { props: { variant: 'table-row', rows: 2, columns: 3 } })
    expect(screen.getByRole('status').textContent.trim()).toBe('Loading…')
    const rows = container.querySelectorAll('.ui-skeleton__row')
    expect(rows).toHaveLength(2)
    rows.forEach(row => expect(row.getAttribute('aria-hidden')).toBe('true'))
  })

  it('shows the loading state only after 300 ms', async () => {
    vi.useFakeTimers()
    const loading = ref(false)
    const scope = effectScope()
    const visible = scope.run(() => useDelayedLoading(loading))
    loading.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(299)
    expect(visible.value).toBe(false)
    await vi.advanceTimersByTimeAsync(2)
    expect(visible.value).toBe(true)
    loading.value = false
    await nextTick()
    expect(visible.value).toBe(false)

    // A fast response never shows it.
    loading.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(200)
    loading.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(500)
    expect(visible.value).toBe(false)
    scope.stop()
    vi.useRealTimers()
  })
})

describe('Badge and StatusDot', () => {
  it('always shows the word for every status', () => {
    for (const status of Object.keys(STATUS_TONES)) {
      const { unmount } = render(UiBadge, { props: { status } })
      const word = { online: 'Online', offline: 'Offline', disabled: 'Disabled', pending: 'Pending', error: 'Error' }[status]
      expect(screen.getByText(word).classList.contains(`ui-badge--${STATUS_TONES[status]}`)).toBe(true)
      unmount()
      render(UiStatusDot, { props: { status } })
      expect(screen.getByText(word)).toBeTruthy()
    }
  })

  it('takes a free tone and label', () => {
    const { container } = render(UiBadge, { props: { tone: 'warning', label: 'Expiring' } })
    expect(container.textContent.trim()).toBe('Expiring')
    expect(container.querySelector('.ui-badge__dot').getAttribute('aria-hidden')).toBe('true')
  })
})

describe('Layout components', () => {
  it('EmptyState renders a heading, text and actions', () => {
    render(UiEmptyState, {
      props: { title: 'No nodes yet', description: 'Add the first node.', icon: Server, headingTag: 'h2' },
      slots: { actions: '<button>Add node</button>' }
    })
    expect(screen.getByRole('heading', { level: 2, name: 'No nodes yet' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Add node' })).toBeTruthy()
  })

  it('PageHeader renders the single h1 with actions', () => {
    render(UiPageHeader, { props: { title: 'Nodes', description: 'Proxy nodes.' }, slots: { actions: '<button>Add node</button>' } })
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(screen.getByRole('heading', { level: 1, name: 'Nodes' })).toBeTruthy()
  })

  it('Card titled as a section is a named region', () => {
    render(UiCard, { props: { title: 'Announcements', as: 'section' }, slots: { default: '<p>Body</p>' } })
    expect(screen.getByRole('region', { name: 'Announcements' })).toBeTruthy()
  })

  it('GroupedList is a named list; rows label their control or become buttons', async () => {
    const user = userEvent.setup()
    const onOpen = vi.fn()
    const value = ref(false)
    render({
      components: { UiGroupedList, UiGroupedListRow, UiSwitch },
      setup: () => ({ onOpen, value }),
      template: `<UiGroupedList title="General" footer="Only admins can create users.">
        <UiGroupedListRow label="Allow sign-ups" label-for="gl-sw"><UiSwitch id="gl-sw" v-model="value" /></UiGroupedListRow>
        <UiGroupedListRow label="Version" value="4.1.0" />
        <UiGroupedListRow label="Mail" @click="onOpen" />
      </UiGroupedList>`
    })
    const list = screen.getByRole('list', { name: 'General' })
    expect(within(list).getAllByRole('listitem')).toHaveLength(3)
    await user.click(screen.getByRole('switch', { name: 'Allow sign-ups' }))
    expect(value.value).toBe(true)
    await user.click(screen.getByRole('button', { name: 'Mail' }))
    expect(onOpen).toHaveBeenCalledTimes(1)
  })
})
