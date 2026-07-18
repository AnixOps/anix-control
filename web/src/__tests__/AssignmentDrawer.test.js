import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import AssignmentDrawer from '@/components/admin/AssignmentDrawer.vue'

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

    expect(wrapper.get('#assignment-node').element.value).toBe('11')
    expect(wrapper.get('#assignment-plugin').element.value).toBe('gost-mesh')
    expect(wrapper.get('#assignment-scope').element.value).toBe('forward')
    expect(wrapper.get('#assignment-role').element.value).toBe('relay')
    expect(wrapper.get('#assignment-version').element.value).toBe('1.0.0')
    expect(wrapper.get('#assignment-config-revision').element.value).toBe('6')
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
      expect(wrapper.get(selector).attributes('disabled')).toBeDefined()
    }
    for (const selector of ['#assignment-version', '#assignment-config-revision', '#assignment-rollout-group']) {
      expect(wrapper.get(selector).attributes('disabled')).toBeUndefined()
    }
    expect(wrapper.get('#assignment-enabled').attributes('disabled')).toBeUndefined()

    await wrapper.get('#assignment-config-revision').setValue('8')
    await wrapper.get('#assignment-rollout-group').setValue('canary-b')
    await wrapper.get('#assignment-enabled').setValue(true)
    await wrapper.get('[data-testid="save-assignment"]').trigger('click')

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

  it('uses the shared modal focus behavior for Escape dismissal', async () => {
    const wrapper = mountDrawer()
    await nextTick()
    await nextTick()

    expect(document.activeElement).toBe(wrapper.get('[data-testid="assignment-close"]').element)
    await wrapper.get('[data-testid="assignment-drawer"]').trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})
