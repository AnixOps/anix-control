import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { inBody } from './helpers/feedback'
import PluginDetailDrawer from '@/components/admin/PluginDetailDrawer.vue'
import PluginInstallationDialog from '@/components/admin/PluginInstallationDialog.vue'
import PluginReleaseImportDialog from '@/components/admin/PluginReleaseImportDialog.vue'

const row = {
  key: 'protocol-runtime',
  plugin: { id: 'protocol-runtime', name: 'Protocol Runtime', description: 'Signed runtime package', official: true },
  health: { state: 'attention', error: 'control needs attention' },
  targets: [
    {
      target: 'agent',
      releases: [{ id: 1, version: '1.0.0' }],
      latestRelease: { id: 1, version: '1.0.0' },
      installation: null,
    },
    {
      target: 'control',
      releases: [{ id: 2, version: '1.1.0' }],
      latestRelease: { id: 2, version: '1.1.0' },
      installation: {
        id: 10,
        target: 'control',
        desired_version: '1.1.0',
        observed_version: '1.0.0',
        state: 'healthy',
        enabled: true,
      },
    },
  ],
}

const mounted = []

// The dialogs render into document.body (UiDialog / UiSheet portal).
function bodyGet(selector) {
  const found = inBody(selector)
  if (!found.exists()) throw new Error(`Unable to find ${selector} in document.body`)
  return found
}

function closeButton(dialogSelector) {
  return bodyGet(`${dialogSelector} [aria-label="Close"]`)
}

// Mount a dialog component open and wait for its portal to render.
async function mountDialog(component, options = {}) {
  const wrapper = mount(component, { attachTo: document.body, ...options })
  mounted.push(wrapper)
  await nextTick()
  await nextTick()
  return wrapper
}

async function pressEscape(selector) {
  await bodyGet(selector).trigger('keydown', { key: 'Escape' })
  await nextTick()
}

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
})

function deferred() {
  let resolve
  const promise = new Promise((nextResolve) => {
    resolve = nextResolve
  })
  return { promise, resolve }
}

function artifactFile(name, read) {
  return {
    name,
    size: 1,
    arrayBuffer: vi.fn(() => read),
  }
}

function textFile(name, read) {
  return {
    name,
    size: 1,
    text: vi.fn(() => read),
  }
}

async function selectArtifact(wrapper, file) {
  const input = bodyGet('#plugin-release-artifact')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

async function selectTextFile(wrapper, field, file) {
  const input = bodyGet(`#plugin-release-${field}-file`)
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

async function completeReleaseFields(wrapper) {
  await bodyGet('#plugin-release-manifest').setValue('{"id":"protocol-runtime"}')
  await bodyGet('#plugin-release-signature').setValue('signed-release')
}

function mountModalHost(component, props = {}) {
  const Host = defineComponent({
    setup() {
      const open = ref(false)
      const trigger = ref(null)
      return () => h('div', [
        h('button', {
          ref: trigger,
          'data-testid': 'modal-trigger',
          type: 'button',
          onClick: () => { open.value = true },
        }, 'Open dialog'),
        h(component, { ...props, open: open.value, onClose: () => { open.value = false } }),
      ])
    },
  })
  const wrapper = mount(Host, { attachTo: document.body })
  mounted.push(wrapper)
  return wrapper
}

describe('PluginDetailDrawer', () => {
  it('shows one target at a time and emits target-scoped actions', async () => {
    const wrapper = mount(PluginDetailDrawer, {
      attachTo: document.body,
      props: { row, open: true },
    })
    mounted.push(wrapper)
    await nextTick()

    const dialog = bodyGet('[data-testid="plugin-detail-drawer"]')
    expect(dialog.attributes('role')).toBe('dialog')
    expect(dialog.attributes('aria-modal')).toBe('true')
    expect(dialog.classes()).toContain('ui-sheet')

    const user = userEvent.setup()
    await user.click(screen.getByRole('tab', { name: 'control' }))
    expect(dialog.text()).toContain('1.1.0')
    expect(dialog.text()).toContain('1.0.0')
    await bodyGet('[data-action="configure"]').trigger('click')
    expect(wrapper.emitted('configure')[0]).toEqual([row.targets[1]])

    await user.click(screen.getByRole('tab', { name: 'agent' }))
    await bodyGet('[data-action="install"]').trigger('click')
    expect(wrapper.emitted('install')[0]).toEqual([row.targets[0]])
  })

  it('is a WAI-ARIA tablist: one tab stop, Left/Right/Home/End move and select', async () => {
    const user = userEvent.setup()
    const wrapper = mount(PluginDetailDrawer, { attachTo: document.body, props: { row, open: true } })
    mounted.push(wrapper)
    await nextTick()

    const list = screen.getByRole('tablist', { name: 'Runtime target' })
    const [agent, control] = within(list).getAllByRole('tab')
    const panelText = () => screen.getByRole('tabpanel').textContent

    expect(agent.getAttribute('aria-selected')).toBe('true')
    expect(control.getAttribute('aria-selected')).toBe('false')
    expect(screen.getByRole('tabpanel').getAttribute('aria-labelledby')).toBe(agent.id)
    expect(panelText()).toContain('Install')

    // Tab enters the tablist once, on the selected tab: roving tabindex.
    for (let step = 0; step < 8 && document.activeElement?.getAttribute('role') !== 'tab'; step += 1) await user.tab()
    expect(document.activeElement).toBe(agent)
    expect(agent.tabIndex).toBe(0)
    expect(control.tabIndex).toBe(-1)

    await user.keyboard('{ArrowRight}')
    expect(document.activeElement).toBe(control)
    expect(control.getAttribute('aria-selected')).toBe('true')
    expect(control.tabIndex).toBe(0)
    expect(agent.tabIndex).toBe(-1)
    expect(screen.getByRole('tabpanel').getAttribute('aria-labelledby')).toBe(control.id)
    expect(panelText()).toContain('Configure')

    // The row wraps around.
    await user.keyboard('{ArrowRight}')
    expect(document.activeElement).toBe(agent)
    await user.keyboard('{ArrowLeft}')
    expect(document.activeElement).toBe(control)
    await user.keyboard('{Home}')
    expect(document.activeElement).toBe(agent)
    expect(agent.getAttribute('aria-selected')).toBe('true')
    await user.keyboard('{End}')
    expect(document.activeElement).toBe(control)
    expect(control.getAttribute('aria-selected')).toBe('true')

    // Tab leaves the tablist (the panel is next), it does not step to another tab.
    await user.tab()
    expect(list.contains(document.activeElement)).toBe(false)
  })

  it('shows no tablist for a plugin without targets', async () => {
    const wrapper = mount(PluginDetailDrawer, { attachTo: document.body, props: { row: { ...row, targets: [] }, open: true } })
    mounted.push(wrapper)
    await nextTick()
    expect(screen.queryByRole('tablist')).toBeNull()
    expect(screen.queryByRole('tabpanel')).toBeNull()
  })

  it('moves focus to its close control and lets the route restore the trigger after close', async () => {
    const Host = defineComponent({
      components: { PluginDetailDrawer },
      setup() {
        const open = ref(false)
        const trigger = ref(null)
        function close() {
          open.value = false
          nextTick(() => trigger.value?.focus())
        }
        return { close, open, row, trigger }
      },
      template: `
        <button ref="trigger" type="button" @click="open = true">Open plugin</button>
        <PluginDetailDrawer :open="open" :row="row" @close="close" />
      `,
    })
    const wrapper = mount(Host, { attachTo: document.body })
    mounted.push(wrapper)

    const trigger = wrapper.get('button')
    trigger.element.focus()
    await trigger.trigger('click')
    const close = closeButton('[data-testid="plugin-detail-drawer"]')
    await waitFor(() => expect(document.activeElement).toBe(close.element))

    await close.trigger('click')
    await waitFor(() => expect(inBody('[data-testid="plugin-detail-drawer"]').exists()).toBe(false))
    await waitFor(() => expect(document.activeElement).toBe(trigger.element))
  })

  it('closes on Escape or its close button only while no target action is busy', async () => {
    const ready = mount(PluginDetailDrawer, { attachTo: document.body, props: { row, open: true } })
    mounted.push(ready)
    await nextTick()
    await pressEscape('[data-testid="plugin-detail-drawer"]')
    await closeButton('[data-testid="plugin-detail-drawer"]').trigger('click')
    expect(ready.emitted('close')).toHaveLength(2)
    ready.unmount()
    mounted.pop()

    const busy = mount(PluginDetailDrawer, { attachTo: document.body, props: { row, open: true, busyTarget: 'control' } })
    mounted.push(busy)
    await nextTick()
    await pressEscape('[data-testid="plugin-detail-drawer"]')
    await closeButton('[data-testid="plugin-detail-drawer"]').trigger('click')
    expect(busy.emitted('close')).toBeUndefined()
    expect(inBody('[data-testid="plugin-detail-drawer"]').exists()).toBe(true)
  })
})

const modalFocusSpecs = [
  {
    name: 'plugin detail drawer',
    component: PluginDetailDrawer,
    dialog: '[data-testid="plugin-detail-drawer"]',
    props: { row },
  },
  {
    name: 'plugin installation dialog',
    component: PluginInstallationDialog,
    dialog: '[data-testid="plugin-installation-dialog"]',
    props: {
      plugin: row.plugin,
      targets: ['control'],
      releases: [{ version: '1.1.0', manifest: JSON.stringify({ targets: ['control'] }) }],
    },
  },
  {
    name: 'plugin release import dialog',
    component: PluginReleaseImportDialog,
    dialog: '[data-testid="plugin-release-import-dialog"]',
    props: {},
  },
]

for (const spec of modalFocusSpecs) {
  it(`${spec.name} traps Tab focus and restores its trigger after Escape`, async () => {
    const user = userEvent.setup()
    const wrapper = mountModalHost(spec.component, spec.props)
    const trigger = wrapper.get('[data-testid="modal-trigger"]')
    await user.click(trigger.element)

    const dialog = await screen.findByRole('dialog')
    expect(dialog.matches(spec.dialog)).toBe(true)
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true))

    // Tab and Shift+Tab never leave the dialog.
    for (let step = 0; step < 12; step += 1) {
      await user.tab()
      expect(dialog.contains(document.activeElement)).toBe(true)
    }
    await user.tab({ shift: true })
    expect(dialog.contains(document.activeElement)).toBe(true)
    // The close button is named and reachable.
    expect(within(dialog).getByRole('button', { name: 'Close' })).toBeTruthy()

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(trigger.element))
  })
}

describe('plugin catalog dialogs', () => {
  it('emits the selected installation intent', async () => {
    const wrapper = await mountDialog(PluginInstallationDialog, {
      props: {
        open: true,
        plugin: row.plugin,
        targets: ['control', 'agent'],
        releases: [
          { id: 1, version: '1.1.0', manifest: JSON.stringify({ targets: ['control'] }) },
          { id: 2, version: '1.0.0', manifest: JSON.stringify({ targets: ['agent'] }) },
        ],
      },
    })

    await bodyGet('#plugin-install-target').setValue('agent')
    await nextTick()
    await bodyGet('[data-action="save-installation"]').trigger('click')

    expect(wrapper.emitted('save')[0]).toEqual([{ target: 'agent', version: '1.0.0', enabled: true }])
  })

  it('resets the installation version when the dialog reopens', async () => {
    const wrapper = await mountDialog(PluginInstallationDialog, {
      props: {
        open: true,
        plugin: row.plugin,
        targets: ['control'],
        releases: [
          { id: 1, version: '1.1.0', manifest: JSON.stringify({ targets: ['control'] }) },
          { id: 2, version: '1.0.0', manifest: JSON.stringify({ targets: ['control'] }) },
        ],
      },
    })

    const version = bodyGet('#plugin-install-version')
    expect(version.element.value).toBe('1.1.0')
    await version.setValue('1.0.0')
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await nextTick()
    expect(bodyGet('#plugin-install-version').element.value).toBe('1.1.0')
  })

  it('allows Escape and backdrop dismissal for an idle installation dialog', async () => {
    const wrapper = await mountDialog(PluginInstallationDialog, {
      props: { open: true, plugin: row.plugin, targets: ['control'], releases: [{ version: '1.1.0', manifest: JSON.stringify({ targets: ['control'] }) }] },
    })

    await pressEscape('[data-testid="plugin-installation-dialog"]')
    await closeButton('[data-testid="plugin-installation-dialog"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(2)
  })

  it('refuses Escape dismissal while the installation dialog is saving', async () => {
    const wrapper = await mountDialog(PluginInstallationDialog, {
      props: { open: true, saving: true, plugin: row.plugin, targets: ['control'], releases: [{ version: '1.1.0', manifest: JSON.stringify({ targets: ['control'] }) }] },
    })

    await pressEscape('[data-testid="plugin-installation-dialog"]')
    await closeButton('[data-testid="plugin-installation-dialog"]').trigger('click')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('emits release-import text values and refuses an outside close while saving', async () => {
    const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })

    // Focus starts in the manifest field.
    await waitFor(() => expect(document.activeElement).toBe(bodyGet('#plugin-release-manifest').element))

    await completeReleaseFields(wrapper)
    await bodyGet('[data-action="save-release"]').trigger('click')
    expect(wrapper.emitted('save')[0]).toEqual([{
      manifest: '{"id":"protocol-runtime"}',
      signature: 'signed-release',
      artifactBase64: '',
    }])

    await pressEscape('[data-testid="plugin-release-import-dialog"]')
    await closeButton('[data-testid="plugin-release-import-dialog"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(2)

    await wrapper.setProps({ saving: true })
    await pressEscape('[data-testid="plugin-release-import-dialog"]')
    await closeButton('[data-testid="plugin-release-import-dialog"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(2)
  })

  it('gives release file inputs distinct accessible names', async () => {
    const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })

    const manifestFile = bodyGet('#plugin-release-manifest-file')
    const signatureFile = bodyGet('#plugin-release-signature-file')
    expect(manifestFile.attributes('aria-label')).toBe('Manifest JSON')
    expect(signatureFile.attributes('aria-label')).toBe('Signature')
    expect(manifestFile.attributes('aria-label')).not.toBe(signatureFile.attributes('aria-label'))
  })

  it('disables release submission until artifact base64 conversion completes', async () => {
    const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
    await completeReleaseFields(wrapper)
    const pending = deferred()

    await selectArtifact(wrapper, artifactFile('pending.tar.gz', pending.promise))
    expect(bodyGet('[data-action="save-release"]').attributes('disabled')).toBeDefined()

    pending.resolve(new Uint8Array([1]).buffer)
    await flushPromises()
    expect(bodyGet('[data-action="save-release"]').attributes('disabled')).toBeUndefined()
  })

  it('keeps only the most recent artifact conversion', async () => {
    const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
    await completeReleaseFields(wrapper)
    const first = deferred()
    const second = deferred()

    await selectArtifact(wrapper, artifactFile('first.tar.gz', first.promise))
    await selectArtifact(wrapper, artifactFile('second.tar.gz', second.promise))
    second.resolve(new Uint8Array([2]).buffer)
    await flushPromises()
    first.resolve(new Uint8Array([1]).buffer)
    await flushPromises()

    await bodyGet('[data-action="save-release"]').trigger('click')
    expect(wrapper.emitted('save')[0][0].artifactBase64).toBe('Ag==')
  })

  it('ignores an artifact conversion that finishes after close and reopen', async () => {
    const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
    const pending = deferred()

    await selectArtifact(wrapper, artifactFile('late.tar.gz', pending.promise))
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    pending.resolve(new Uint8Array([1]).buffer)
    await flushPromises()
    await completeReleaseFields(wrapper)
    await bodyGet('[data-action="save-release"]').trigger('click')

    expect(wrapper.emitted('save')[0][0].artifactBase64).toBe('')
  })

  it('invalidates an artifact conversion as soon as close is requested', async () => {
    const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
    const pending = deferred()

    await selectArtifact(wrapper, artifactFile('closing.tar.gz', pending.promise))
    await pressEscape('[data-testid="plugin-release-import-dialog"]')
    pending.resolve(new Uint8Array([1]).buffer)
    await flushPromises()
    await completeReleaseFields(wrapper)
    await bodyGet('[data-action="save-release"]').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(wrapper.emitted('save')[0][0].artifactBase64).toBe('')
  })

  const textFileSpecs = [
    { field: 'manifest', value: '{"id":"from-file"}' },
    { field: 'signature', value: 'from-file-signature' },
  ]

  for (const spec of textFileSpecs) {
    it(`disables release submission while ${spec.field} file text is reading`, async () => {
      const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
      await completeReleaseFields(wrapper)
      const pending = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`${spec.field}.txt`, pending.promise))
      expect(bodyGet('[data-action="save-release"]').attributes('disabled')).toBeDefined()

      pending.resolve(spec.value)
      await flushPromises()
      expect(bodyGet('[data-action="save-release"]').attributes('disabled')).toBeUndefined()
    })

    it(`keeps only the most recent ${spec.field} file text`, async () => {
      const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
      const first = deferred()
      const second = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`first-${spec.field}.txt`, first.promise))
      await selectTextFile(wrapper, spec.field, textFile(`second-${spec.field}.txt`, second.promise))
      second.resolve(`second-${spec.value}`)
      await flushPromises()
      first.resolve(`first-${spec.value}`)
      await flushPromises()

      expect(bodyGet(`#plugin-release-${spec.field}`).element.value).toBe(`second-${spec.value}`)
    })

    it(`ignores ${spec.field} file text that finishes after close and reopen`, async () => {
      const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
      const pending = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`late-${spec.field}.txt`, pending.promise))
      await wrapper.setProps({ open: false })
      await wrapper.setProps({ open: true })
      pending.resolve(spec.value)
      await flushPromises()

      expect(bodyGet(`#plugin-release-${spec.field}`).element.value).toBe('')
    })

    it(`invalidates ${spec.field} file text as soon as close is requested`, async () => {
      const wrapper = await mountDialog(PluginReleaseImportDialog, { props: { open: true } })
      const pending = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`closing-${spec.field}.txt`, pending.promise))
      await pressEscape('[data-testid="plugin-release-import-dialog"]')
      pending.resolve(spec.value)
      await flushPromises()

      expect(wrapper.emitted('close')).toHaveLength(1)
      expect(bodyGet(`#plugin-release-${spec.field}`).element.value).toBe('')
    })
  }
})
