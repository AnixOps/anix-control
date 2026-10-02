import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiDialog from '../UiDialog.vue'
import UiConfirmDialog from '../UiConfirmDialog.vue'
import UiSheet from '../UiSheet.vue'
import UiHost from '../UiHost.vue'
import UiButton from '../UiButton.vue'
import UiTextField from '../UiTextField.vue'
import { resetConfirms, useConfirm } from '../composables/useConfirm'

afterEach(() => {
  resetConfirms()
})

function labelOf(element, attribute) {
  return (element.getAttribute(attribute) || '').split(' ').filter(Boolean).map(id => document.getElementById(id)?.textContent.trim()).join(' ')
}

const DialogHarness = {
  components: { UiDialog, UiButton, UiTextField },
  props: { description: { type: String, default: 'It appears in every subscription of its group.' } },
  setup() {
    return { open: ref(false), name: ref('') }
  },
  template: `<div>
    <UiButton @click="open = true">Add node…</UiButton>
    <UiDialog v-model:open="open" title="Add node" :description="description">
      <UiTextField v-model="name" label="Name" />
      <template #footer="{ close }">
        <UiButton @click="close">Cancel</UiButton>
        <UiButton variant="primary" @click="close">Add node</UiButton>
      </template>
    </UiDialog>
  </div>`
}

describe('UiDialog', () => {
  it('is a labelled modal dialog that traps focus, closes with Esc and returns focus', async () => {
    const user = userEvent.setup()
    render(DialogHarness)
    const opener = screen.getByRole('button', { name: 'Add node…' })
    await user.click(opener)

    const dialog = await screen.findByRole('dialog')
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(labelOf(dialog, 'aria-labelledby')).toBe('Add node')
    expect(labelOf(dialog, 'aria-describedby')).toBe('It appears in every subscription of its group.')
    // Focus starts on the first field, not on the close button.
    await waitFor(() => expect(document.activeElement).toBe(within(dialog).getByLabelText('Name')))

    // Tab order: field, Cancel, Add node, then the close button, then back.
    await user.tab()
    expect(document.activeElement.textContent.trim()).toBe('Cancel')
    await user.tab()
    await user.tab()
    expect(document.activeElement.getAttribute('aria-label')).toBe('Close')
    await user.tab()
    expect(document.activeElement).toBe(within(dialog).getByLabelText('Name'))

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(opener))
  })

  it('drops aria-describedby when there is no description', async () => {
    const user = userEvent.setup()
    render(DialogHarness, { props: { description: '' } })
    await user.click(screen.getByRole('button', { name: 'Add node…' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog.hasAttribute('aria-describedby')).toBe(false)
  })

  it('closes from the footer through the slot close()', async () => {
    const user = userEvent.setup()
    render(DialogHarness)
    await user.click(screen.getByRole('button', { name: 'Add node…' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('ignores Esc when not dismissible', async () => {
    const user = userEvent.setup()
    const onUpdate = vi.fn()
    render(UiDialog, { props: { open: true, title: 'Saving', dismissible: false, 'onUpdate:open': onUpdate }, slots: { default: '<p>Working</p>' } })
    await screen.findByRole('dialog')
    await user.keyboard('{Escape}')
    expect(onUpdate).not.toHaveBeenCalled()
  })
})

describe('UiConfirmDialog', () => {
  it('is an alertdialog that needs the typed name before the danger button enables', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    render(UiConfirmDialog, {
      props: {
        open: true,
        title: 'Delete node hk-01?',
        message: 'This cannot be undone.',
        confirmLabel: 'Delete node',
        tone: 'danger',
        requireText: 'hk-01',
        onConfirm
      }
    })
    const dialog = await screen.findByRole('alertdialog')
    expect(labelOf(dialog, 'aria-labelledby')).toBe('Delete node hk-01?')
    expect(labelOf(dialog, 'aria-describedby')).toBe('This cannot be undone.')
    const field = within(dialog).getByLabelText('Type hk-01 to confirm')
    await waitFor(() => expect(document.activeElement).toBe(field))
    const confirm = within(dialog).getByRole('button', { name: 'Delete node' })
    expect(confirm.disabled).toBe(true)
    expect(confirm.classList.contains('ui-button--danger')).toBe(true)

    await user.type(field, 'hk-0')
    await user.keyboard('{Enter}')
    expect(onConfirm).not.toHaveBeenCalled()
    await user.type(field, '1')
    expect(confirm.disabled).toBe(false)
    await user.keyboard('{Enter}')
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('focuses Cancel for a danger action without typed text, and Esc cancels', async () => {
    const user = userEvent.setup()
    const onCancel = vi.fn()
    render(UiConfirmDialog, { props: { open: true, title: 'Reset link?', tone: 'danger', confirmLabel: 'Reset link', onCancel } })
    const dialog = await screen.findByRole('alertdialog')
    await waitFor(() => expect(document.activeElement).toBe(within(dialog).getByRole('button', { name: 'Cancel' })))
    await user.keyboard('{Escape}')
    expect(onCancel).toHaveBeenCalledTimes(1)
  })
})

describe('useConfirm()', () => {
  const Harness = {
    components: { UiHost, UiButton },
    props: { options: { type: Object, required: true } },
    emits: ['result'],
    setup(props, { emit }) {
      const confirm = useConfirm()
      return { ask: async () => emit('result', await confirm(props.options)) }
    },
    template: '<div><UiButton @click="ask">Ask</UiButton><UiHost /></div>'
  }

  it('resolves true on confirm and false on cancel', async () => {
    const user = userEvent.setup()
    const onResult = vi.fn()
    render(Harness, { props: { options: { title: 'Disable node hk-01?', confirmLabel: 'Disable node' }, onResult } })
    await user.click(screen.getByRole('button', { name: 'Ask' }))
    await user.click(await screen.findByRole('button', { name: 'Disable node' }))
    await waitFor(() => expect(onResult).toHaveBeenLastCalledWith(true))

    await user.click(screen.getByRole('button', { name: 'Ask' }))
    await user.click(await screen.findByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(onResult).toHaveBeenLastCalledWith(false))
  })

  it('keeps the dialog open with the error when onConfirm fails', async () => {
    const user = userEvent.setup()
    const onResult = vi.fn()
    const onConfirm = vi.fn().mockRejectedValueOnce(new Error('Connection timed out.')).mockResolvedValueOnce()
    render(Harness, { props: { options: { title: 'Disable node?', confirmLabel: 'Disable node', onConfirm }, onResult } })
    await user.click(screen.getByRole('button', { name: 'Ask' }))
    const button = await screen.findByRole('button', { name: 'Disable node' })
    await user.click(button)
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toBe('The action did not complete: Connection timed out.')
    expect(screen.getByRole('alertdialog')).toBeTruthy()
    expect(onResult).not.toHaveBeenCalled()
    await user.click(button)
    await waitFor(() => expect(onResult).toHaveBeenCalledWith(true))
    expect(onConfirm).toHaveBeenCalledTimes(2)
  })
})

describe('UiSheet', () => {
  it('is a labelled modal dialog that focuses the first control and closes with ×', async () => {
    const user = userEvent.setup()
    const open = ref(true)
    render({
      components: { UiSheet, UiTextField },
      setup: () => ({ open }),
      template: '<UiSheet v-model:open="open" title="Edit rule"><UiTextField model-value="" label="Remote address" /></UiSheet>'
    })
    const sheet = await screen.findByRole('dialog')
    expect(sheet.getAttribute('aria-modal')).toBe('true')
    expect(labelOf(sheet, 'aria-labelledby')).toBe('Edit rule')
    await waitFor(() => expect(document.activeElement).toBe(within(sheet).getByLabelText('Remote address')))
    await user.click(within(sheet).getByRole('button', { name: 'Close' }))
    expect(open.value).toBe(false)
  })
})

describe('useScrollableFocus', () => {
  it('makes an overflowing body without focusable content reachable with Tab', async () => {
    const { useScrollableFocus } = await import('../internal/useScrollableFocus')
    const { mount } = await import('@vue/test-utils')
    const { nextTick } = await import('vue')
    const Probe = {
      props: { withButton: Boolean },
      setup() {
        return { scrollable: useScrollableFocus() }
      },
      template: `<div :ref="scrollable.setElement" :tabindex="scrollable.tabindex.value"><p>Long read-only text</p><button v-if="withButton">Copy</button></div>`
    }
    const overflow = (el) => {
      Object.defineProperty(el, 'scrollHeight', { value: 900, configurable: true })
      Object.defineProperty(el, 'clientHeight', { value: 300, configurable: true })
    }
    const plain = mount(Probe)
    expect(plain.attributes('tabindex')).toBeUndefined()
    overflow(plain.element)
    await plain.setProps({ withButton: false })
    plain.element.appendChild(document.createElement('p'))
    await new Promise(resolve => setTimeout(resolve))
    await nextTick()
    expect(plain.attributes('tabindex')).toBe('0')

    // Something focusable inside already lets the keyboard scroll it.
    await plain.setProps({ withButton: true })
    await new Promise(resolve => setTimeout(resolve))
    await nextTick()
    expect(plain.attributes('tabindex')).toBeUndefined()
    plain.unmount()
  })
})
