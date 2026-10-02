import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { inBody } from './helpers/feedback'
import { nextTick } from 'vue'
import AssignmentDrawer from '@/components/admin/AssignmentDrawer.vue'
import UiSelect from '@/ui/UiSelect.vue'

const nodes = [{ id: 11, name: 'Shanghai entry', host: '10.0.0.11' }]
const plugins = [{ id: 'gost-mesh', name: 'GOST Mesh' }]
const releases = [{
  id: 3,
  plugin_id: 'gost-mesh',
  version: '1.0.0',
  manifest: JSON.stringify({ targets: ['agent'] }),
}]
const installations = [{
  id: 5,
  plugin_id: 'gost-mesh',
  target: 'agent',
  desired_version: '1.0.0',
  config_revision: 6,
  enabled: true,
}]
const scopes = [{ id: 'forward', name: 'Forward' }]

// UiSelect (Reka) has no native value: read it from the component.
function selectValue(wrapper, id) {
  return wrapper.findAllComponents(UiSelect).find(select => select.props('id') === id)?.props('modelValue')
}

function mountDrawer(props = {}) {
  return mount(AssignmentDrawer, {
    attachTo: document.body,
    props: {
      open: true,
      selectedNodeID: 11,
      nodes,
      plugins,
      releases,
      installations,
      scopes,
      ...props,
    },
  })
}

describe('AssignmentDrawer', () => {
  it('uses the first installed agent plugin and its legacy defaults for a new assignment', async () => {
    const wrapper = mountDrawer()
    await nextTick()

    expect(selectValue(wrapper, 'assignment-node')).toBe(11)
    expect(selectValue(wrapper, 'assignment-plugin')).toBe('gost-mesh')
    expect(selectValue(wrapper, 'assignment-scope')).toBe('forward')
    expect(inBody('#assignment-role').element.value).toBe('relay')
    expect(selectValue(wrapper, 'assignment-version')).toBe('1.0.0')
    // The labels name the controls (top labels, plan D5).
    expect(inBody('label[for="assignment-rollout-group"]').exists()).toBe(true)
    expect(inBody('#assignment-config-revision').element.value).toBe('6')
    wrapper.unmount()
  })

  it('keeps identity fields immutable in edit mode while submitting editable rollout fields', async () => {
    const wrapper = mountDrawer({
      assignment: {
        id: 7,
        node_id: 11,
        service_scope: 'forward',
        plugin_id: 'gost-mesh',
        role: 'relay',
        desired_version: '1.0.0',
        desired_config_revision: 6,
        rollout_group: 'canary-a',
        enabled: false,
      },
    })
    await nextTick()

    for (const selector of ['#assignment-node', '#assignment-plugin', '#assignment-scope', '#assignment-role']) {
      // A native input says disabled; the Reka select trigger data-disabled.
      expect(inBody(selector).attributes('disabled') ?? inBody(selector).attributes('data-disabled')).toBeDefined()
    }
    for (const selector of ['#assignment-version', '#assignment-config-revision', '#assignment-rollout-group']) {
      expect(inBody(selector).attributes('disabled') ?? inBody(selector).attributes('data-disabled')).toBeUndefined()
    }
    expect(inBody('#assignment-enabled').attributes('disabled')).toBeUndefined()

    await inBody('#assignment-config-revision').setValue('8')
    await inBody('#assignment-rollout-group').setValue('canary-b')
    expect(inBody('#assignment-enabled').attributes('aria-checked')).toBe('false')
    await inBody('#assignment-enabled').trigger('click')
    await inBody('[data-testid="save-assignment"]').trigger('click')

    expect(wrapper.emitted('save')).toEqual([[
      {
        nodeID: 11,
        payload: {
          service_scope: 'forward',
          plugin_id: 'gost-mesh',
          role: 'relay',
          desired_version: '1.0.0',
          desired_config_revision: 8,
          enabled: true,
          rollout_group: 'canary-b',
        },
      },
    ]])
    wrapper.unmount()
  })

  it('moves focus into the sheet and closes on Escape', async () => {
    const wrapper = mountDrawer()
    await nextTick()
    await nextTick()

    const drawer = inBody('[data-testid="assignment-drawer"]')
    expect(drawer.classes()).toContain('ui-sheet')
    expect(drawer.element.contains(document.activeElement)).toBe(true)
    await drawer.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('refuses Escape and its close button while saving', async () => {
    const wrapper = mountDrawer({ saving: true })
    await nextTick()
    await nextTick()

    const drawer = inBody('[data-testid="assignment-drawer"]')
    await drawer.trigger('keydown', { key: 'Escape' })
    await drawer.get('[aria-label="Close"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(inBody('[data-testid="save-assignment"]').attributes('aria-busy')).toBe('true')
    wrapper.unmount()
  })
})
