import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
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
    expect(ready.emitted('close')).toHaveLength(1)

    const busy = mount(PluginDetailDrawer, { attachTo: document.body, props: { row, open: true, busyTarget: 'control' } })
    mounted.push(busy)
    await busy.get('[data-testid="plugin-detail-drawer"]').trigger('keydown', { key: 'Escape' })
    await busy.get('[data-testid="plugin-detail-drawer"]').trigger('click')
    expect(busy.emitted('close')).toBeUndefined()
  })
})

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

  it('emits release-import text values and refuses an outside close while saving', async () => {
    const wrapper = mount(PluginReleaseImportDialog, { attachTo: document.body, props: { open: true } })
    mounted.push(wrapper)

    await nextTick()
    expect(document.activeElement).toBe(wrapper.get('.icon-button').element)

    await wrapper.get('#plugin-release-manifest').setValue('{"id":"protocol-runtime"}')
    await wrapper.get('#plugin-release-signature').setValue('signed-release')
    await wrapper.get('[data-action="save-release"]').trigger('click')
    expect(wrapper.emitted('save')[0]).toEqual([{
      manifest: '{"id":"protocol-runtime"}',
      signature: 'signed-release',
      artifactBase64: '',
    }])

    await wrapper.setProps({ saving: true })
    await wrapper.get('[data-testid="plugin-release-import-dialog"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})
