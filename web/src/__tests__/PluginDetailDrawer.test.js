import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import PluginDetailDrawer from '@/components/admin/PluginDetailDrawer.vue'
import PluginInstallationDialog from '@/components/admin/PluginInstallationDialog.vue'
import PluginReleaseImportDialog from '@/components/admin/PluginReleaseImportDialog.vue'

const row = {
  key: 'protocol-runtime',
  plugin: { id: 'protocol-runtime', name: 'Protocol Runtime', description: 'Signed runtime package' },
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
  const input = wrapper.get('#plugin-release-artifact')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

async function selectTextFile(wrapper, field, file) {
  const input = wrapper.get(`#plugin-release-${field}-file`)
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

async function completeReleaseFields(wrapper) {
  await wrapper.get('#plugin-release-manifest').setValue('{"id":"protocol-runtime"}')
  await wrapper.get('#plugin-release-signature').setValue('signed-release')
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

    const dialog = wrapper.get('[data-testid="plugin-detail-drawer"]')
    expect(dialog.attributes('role')).toBe('dialog')
    expect(dialog.attributes('aria-modal')).toBe('true')

    await wrapper.get('[data-target="control"]').trigger('click')
    expect(dialog.text()).toContain('1.1.0')
    expect(dialog.text()).toContain('1.0.0')
    await wrapper.get('[data-action="configure"]').trigger('click')
    expect(wrapper.emitted('configure')[0]).toEqual([row.targets[1]])

    await wrapper.get('[data-target="agent"]').trigger('click')
    await wrapper.get('[data-action="install"]').trigger('click')
    expect(wrapper.emitted('install')[0]).toEqual([row.targets[0]])
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
    await trigger.trigger('click')
    await nextTick()
    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('[data-testid="plugin-detail-close"]').element)

    await wrapper.get('[data-testid="plugin-detail-close"]').trigger('click')
    await nextTick()
    await nextTick()
    expect(document.activeElement).toBe(trigger.element)
  })

  it('closes on Escape or click-outside only while no target action is busy', async () => {
    const ready = mount(PluginDetailDrawer, { attachTo: document.body, props: { row, open: true } })
    mounted.push(ready)
    await ready.get('[data-testid="plugin-detail-drawer"]').trigger('keydown', { key: 'Escape' })
    await ready.find('.plugin-detail-backdrop').trigger('click')
    expect(ready.emitted('close')).toHaveLength(2)

    const busy = mount(PluginDetailDrawer, { attachTo: document.body, props: { row, open: true, busyTarget: 'control' } })
    mounted.push(busy)
    await busy.get('[data-testid="plugin-detail-drawer"]').trigger('keydown', { key: 'Escape' })
    await busy.get('[data-testid="plugin-detail-drawer"]').trigger('click')
    expect(busy.emitted('close')).toBeUndefined()
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
    const wrapper = mountModalHost(spec.component, spec.props)
    const trigger = wrapper.get('[data-testid="modal-trigger"]')
    trigger.element.focus()
    await trigger.trigger('click')
    await nextTick()
    await nextTick()

    const dialog = wrapper.get(spec.dialog)
    const focusableButtons = dialog.findAll('button').filter(button => !button.element.disabled)
    const first = focusableButtons[0]
    const last = focusableButtons.at(-1)
    expect(document.activeElement).toBe(first.element)

    await first.trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(last.element)
    await last.trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(first.element)

    await first.trigger('keydown', { key: 'Escape' })
    await nextTick()
    await nextTick()
    expect(document.activeElement).toBe(trigger.element)
  })
}

describe('plugin catalog dialogs', () => {
  it('emits the selected installation intent', async () => {
    const wrapper = mount(PluginInstallationDialog, {
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
    mounted.push(wrapper)

    await wrapper.get('#plugin-install-target').setValue('agent')
    await nextTick()
    await wrapper.get('[data-action="save-installation"]').trigger('click')

    expect(wrapper.emitted('save')[0]).toEqual([{ target: 'agent', version: '1.0.0', enabled: true }])
  })

  it('resets the installation version when the dialog reopens', async () => {
    const wrapper = mount(PluginInstallationDialog, {
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
    mounted.push(wrapper)

    const version = wrapper.get('#plugin-install-version')
    expect(version.element.value).toBe('1.1.0')
    await version.setValue('1.0.0')
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await nextTick()
    expect(wrapper.get('#plugin-install-version').element.value).toBe('1.1.0')
  })

  it('allows Escape and backdrop dismissal for an idle installation dialog', async () => {
    const wrapper = mount(PluginInstallationDialog, {
      props: { open: true, plugin: row.plugin, targets: ['control'], releases: [{ version: '1.1.0', manifest: JSON.stringify({ targets: ['control'] }) }] },
    })
    mounted.push(wrapper)

    await wrapper.get('[data-testid="plugin-installation-dialog"]').trigger('keydown', { key: 'Escape' })
    await wrapper.find('.plugin-dialog-backdrop').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(2)
  })

  it('refuses Escape dismissal while the installation dialog is saving', async () => {
    const wrapper = mount(PluginInstallationDialog, {
      props: { open: true, saving: true, plugin: row.plugin, targets: ['control'], releases: [{ version: '1.1.0', manifest: JSON.stringify({ targets: ['control'] }) }] },
    })
    mounted.push(wrapper)

    await wrapper.get('[data-testid="plugin-installation-dialog"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('emits release-import text values and refuses an outside close while saving', async () => {
    const wrapper = mount(PluginReleaseImportDialog, { attachTo: document.body, props: { open: true } })
    mounted.push(wrapper)

    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('.icon-button').element)

    await completeReleaseFields(wrapper)
    await wrapper.get('[data-action="save-release"]').trigger('click')
    expect(wrapper.emitted('save')[0]).toEqual([{
      manifest: '{"id":"protocol-runtime"}',
      signature: 'signed-release',
      artifactBase64: '',
    }])

    await wrapper.get('[data-testid="plugin-release-import-dialog"]').trigger('keydown', { key: 'Escape' })
    await wrapper.find('.plugin-dialog-backdrop').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(2)

    await wrapper.setProps({ saving: true })
    await wrapper.get('[data-testid="plugin-release-import-dialog"]').trigger('keydown', { key: 'Escape' })
    await wrapper.find('.plugin-dialog-backdrop').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(2)
  })

  it('gives release file inputs distinct accessible names', () => {
    const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
    mounted.push(wrapper)

    const manifestFile = wrapper.get('#plugin-release-manifest-file')
    const signatureFile = wrapper.get('#plugin-release-signature-file')
    expect(manifestFile.attributes('aria-label')).toBe('Manifest JSON')
    expect(signatureFile.attributes('aria-label')).toBe('Signature')
    expect(manifestFile.attributes('aria-label')).not.toBe(signatureFile.attributes('aria-label'))
  })

  it('disables release submission until artifact base64 conversion completes', async () => {
    const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
    mounted.push(wrapper)
    await completeReleaseFields(wrapper)
    const pending = deferred()

    await selectArtifact(wrapper, artifactFile('pending.tar.gz', pending.promise))
    expect(wrapper.get('[data-action="save-release"]').attributes('disabled')).toBeDefined()

    pending.resolve(new Uint8Array([1]).buffer)
    await flushPromises()
    expect(wrapper.get('[data-action="save-release"]').attributes('disabled')).toBeUndefined()
  })

  it('keeps only the most recent artifact conversion', async () => {
    const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
    mounted.push(wrapper)
    await completeReleaseFields(wrapper)
    const first = deferred()
    const second = deferred()

    await selectArtifact(wrapper, artifactFile('first.tar.gz', first.promise))
    await selectArtifact(wrapper, artifactFile('second.tar.gz', second.promise))
    second.resolve(new Uint8Array([2]).buffer)
    await flushPromises()
    first.resolve(new Uint8Array([1]).buffer)
    await flushPromises()

    await wrapper.get('[data-action="save-release"]').trigger('click')
    expect(wrapper.emitted('save')[0][0].artifactBase64).toBe('Ag==')
  })

  it('ignores an artifact conversion that finishes after close and reopen', async () => {
    const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
    mounted.push(wrapper)
    const pending = deferred()

    await selectArtifact(wrapper, artifactFile('late.tar.gz', pending.promise))
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    pending.resolve(new Uint8Array([1]).buffer)
    await flushPromises()
    await completeReleaseFields(wrapper)
    await wrapper.get('[data-action="save-release"]').trigger('click')

    expect(wrapper.emitted('save')[0][0].artifactBase64).toBe('')
  })

  it('invalidates an artifact conversion as soon as close is requested', async () => {
    const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
    mounted.push(wrapper)
    const pending = deferred()

    await selectArtifact(wrapper, artifactFile('closing.tar.gz', pending.promise))
    await wrapper.get('[data-testid="plugin-release-import-dialog"]').trigger('keydown', { key: 'Escape' })
    pending.resolve(new Uint8Array([1]).buffer)
    await flushPromises()
    await completeReleaseFields(wrapper)
    await wrapper.get('[data-action="save-release"]').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(wrapper.emitted('save')[0][0].artifactBase64).toBe('')
  })

  const textFileSpecs = [
    { field: 'manifest', value: '{"id":"from-file"}' },
    { field: 'signature', value: 'from-file-signature' },
  ]

  for (const spec of textFileSpecs) {
    it(`disables release submission while ${spec.field} file text is reading`, async () => {
      const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
      mounted.push(wrapper)
      await completeReleaseFields(wrapper)
      const pending = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`${spec.field}.txt`, pending.promise))
      expect(wrapper.get('[data-action="save-release"]').attributes('disabled')).toBeDefined()

      pending.resolve(spec.value)
      await flushPromises()
      expect(wrapper.get('[data-action="save-release"]').attributes('disabled')).toBeUndefined()
    })

    it(`keeps only the most recent ${spec.field} file text`, async () => {
      const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
      mounted.push(wrapper)
      const first = deferred()
      const second = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`first-${spec.field}.txt`, first.promise))
      await selectTextFile(wrapper, spec.field, textFile(`second-${spec.field}.txt`, second.promise))
      second.resolve(`second-${spec.value}`)
      await flushPromises()
      first.resolve(`first-${spec.value}`)
      await flushPromises()

      expect(wrapper.get(`#plugin-release-${spec.field}`).element.value).toBe(`second-${spec.value}`)
    })

    it(`ignores ${spec.field} file text that finishes after close and reopen`, async () => {
      const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
      mounted.push(wrapper)
      const pending = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`late-${spec.field}.txt`, pending.promise))
      await wrapper.setProps({ open: false })
      await wrapper.setProps({ open: true })
      pending.resolve(spec.value)
      await flushPromises()

      expect(wrapper.get(`#plugin-release-${spec.field}`).element.value).toBe('')
    })

    it(`invalidates ${spec.field} file text as soon as close is requested`, async () => {
      const wrapper = mount(PluginReleaseImportDialog, { props: { open: true } })
      mounted.push(wrapper)
      const pending = deferred()

      await selectTextFile(wrapper, spec.field, textFile(`closing-${spec.field}.txt`, pending.promise))
      await wrapper.get('[data-testid="plugin-release-import-dialog"]').trigger('keydown', { key: 'Escape' })
      pending.resolve(spec.value)
      await flushPromises()

      expect(wrapper.emitted('close')).toHaveLength(1)
      expect(wrapper.get(`#plugin-release-${spec.field}`).element.value).toBe('')
    })
  }
})
