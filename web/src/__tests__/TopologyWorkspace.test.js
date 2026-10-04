import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { inBody } from './helpers/feedback'
import { nextTick } from 'vue'
import { screen } from '@testing-library/vue'
import OperationTimeline from '@/components/admin/OperationTimeline.vue'
import TopologyWorkspace from '@/components/admin/TopologyWorkspace.vue'
import UiSegmentedControl from '@/ui/UiSegmentedControl.vue'
import UiSelect from '@/ui/UiSelect.vue'

// G6 draws on canvas; record what the preview hands it instead.
const g6 = vi.hoisted(() => ({ graphs: [] }))
vi.mock('@antv/g6', () => ({
  Graph: class {
    constructor(options) {
      this.options = options
      this.destroyed = false
      g6.graphs.push(this)
    }
    render() { return Promise.resolve() }
    resize() {}
    fitView() { return Promise.resolve() }
    destroy() { this.destroyed = true }
  }
}))

const revisionDetail = {
  revision: { id: 7, message: 'Published revision' },
  vertices: [{ key: 'entry', kind: 'agent', node_id: 11, plugin_id: 'gost-mesh', role: 'relay', config: '{"port":443}' }],
  edges: [],
}

// The workspace is a UiDialog: it renders into document.body.
function bodyGet(selector) {
  const found = inBody(selector)
  if (!found.exists()) throw new Error(`Unable to find ${selector} in document.body`)
  return found
}

function mountWorkspace(props = {}) {
  return mount(TopologyWorkspace, {
    attachTo: document.body,
    props: {
      open: true,
      topology: { id: 3, name: 'Regional mesh' },
      revisions: [{ id: 7, revision: 2 }],
      revisionID: 7,
      revisionDetail,
      deploymentID: 13,
      deploymentStatus: { deployment: { id: 13, state: 'planned' }, operations: [] },
      scopes: [{ id: 'forward', name: 'Forward' }],
      validation: { valid: true, issues: [], checks: [{ name: 'release', status: 'passed' }] },
      preview: { valid: true, steps: [{ order: 1, vertex_key: 'entry', apply_action: 'configure', rollback_mode: 'restore' }] },
      ...props,
    },
  })
}

describe('TopologyWorkspace', () => {
  it('is a labelled dialog that closes on Escape only while idle', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()
    const dialog = bodyGet('[data-testid="topology-workspace"]')
    expect(dialog.attributes('role')).toBe('dialog')
    expect(document.getElementById(dialog.attributes('aria-labelledby')).textContent).toContain('Topology')
    expect(dialog.element.contains(document.activeElement)).toBe(true)
    await dialog.trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()

    const busy = mountWorkspace({ saving: true })
    await nextTick()
    await nextTick()
    await bodyGet('[data-testid="topology-workspace"]').trigger('keydown', { key: 'Escape' })
    await bodyGet('[data-testid="topology-workspace"] [aria-label="Close"]').trigger('click')
    expect(busy.emitted('close')).toBeUndefined()
    busy.unmount()
  })

  it('requires named confirmations before applying or rolling back a clean immutable revision', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await bodyGet('#topology-diagnose').trigger('click')
    expect(wrapper.emitted('diagnose')).toEqual([[
      expect.objectContaining({
        topologyID: 3,
        revisionID: 7,
        options: expect.objectContaining({ rolloutGroup: '', failurePolicy: 'stop_and_rollback' }),
      }),
    ]])

    await bodyGet('#topology-preview').trigger('click')
    await bodyGet('#topology-plan').trigger('click')
    expect(wrapper.emitted('preview')).toHaveLength(1)
    expect(wrapper.emitted('plan')).toHaveLength(1)

    await bodyGet('#topology-apply').trigger('click')
    expect(wrapper.emitted('apply')).toBeUndefined()
    expect(bodyGet('[data-testid="apply-confirmation"]').text()).toContain('Regional mesh')
    await bodyGet('[data-testid="confirm-apply"]').trigger('click')
    expect(wrapper.emitted('apply')).toEqual([[{ deploymentID: 13 }]])

    await bodyGet('#topology-rollback').trigger('click')
    expect(wrapper.emitted('rollback')).toBeUndefined()
    expect(bodyGet('[data-testid="rollback-confirmation"]').text()).toContain('Regional mesh')
    await bodyGet('[data-testid="confirm-rollback"]').trigger('click')
    expect(wrapper.emitted('rollback')).toEqual([[{ deploymentID: 13 }]])
    wrapper.unmount()
  })

  it('does not roll back when a confirmed deployment becomes dirty', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await bodyGet('#topology-rollback').trigger('click')
    const confirm = bodyGet('[data-testid="confirm-rollback"]')
    await bodyGet('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')

    expect(confirm.attributes('disabled')).toBeDefined()
    await confirm.trigger('click')
    expect(wrapper.emitted('rollback')).toBeUndefined()
    wrapper.unmount()
  })

  it('binds confirmations to one deployment and invalidates them when its identity or state changes', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await bodyGet('#topology-apply').trigger('click')
    expect(bodyGet('[data-testid="apply-confirmation"]').text()).toContain('#13')
    expect(bodyGet('#topology-plan').attributes('disabled')).toBeDefined()

    await wrapper.setProps({
      deploymentID: 14,
      deploymentStatus: { deployment: { id: 14, state: 'planned' }, operations: [] },
    })
    expect(inBody('[data-testid="apply-confirmation"]').exists()).toBe(false)

    await bodyGet('#topology-rollback').trigger('click')
    expect(inBody('[data-testid="rollback-confirmation"]').exists()).toBe(true)
    await wrapper.setProps({ deploymentStatus: { deployment: { id: 14, state: 'applying' }, operations: [] } })
    expect(inBody('[data-testid="rollback-confirmation"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps plan, apply, and rollback unavailable while the revision differs from its baseline', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await bodyGet('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')
    expect(bodyGet('.topology-dirty').text()).toContain('unsaved changes')
    for (const selector of ['#topology-plan', '#topology-apply', '#topology-rollback']) {
      expect(bodyGet(selector).attributes('disabled')).toBeDefined()
    }

    await bodyGet('#topology-save-revision').trigger('click')
    expect(wrapper.emitted('save-revision')).toEqual([[
      {
        topologyID: 3,
        input: {
          message: 'Published revision',
          vertices: [],
          edges: [],
        },
      },
    ]])
    wrapper.unmount()
  })

  it('retains a dirty revision when its parent refreshes the same topology record', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await bodyGet('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')
    await wrapper.setProps({ topology: { id: 3, name: 'Regional mesh refreshed' } })

    expect(bodyGet('#topology-editor-json').element.value).toBe('{"vertices":[],"edges":[]}')
    expect(bodyGet('.topology-dirty').exists()).toBe(true)
    wrapper.unmount()
  })

  it('retains a dirty draft when the same selected revision detail is replaced asynchronously', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await bodyGet('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')
    await bodyGet('#topology-revision-message').setValue('Local draft')
    await wrapper.setProps({
      revisionDetail: {
        revision: { id: 7, message: 'Server refresh' },
        vertices: [{ key: 'server-entry', kind: 'agent', config: '{"port":8443}' }],
        edges: [],
      },
    })

    expect(bodyGet('#topology-editor-json').element.value).toBe('{"vertices":[],"edges":[]}')
    expect(bodyGet('#topology-revision-message').element.value).toBe('Local draft')
    expect(bodyGet('.topology-dirty').exists()).toBe(true)
    wrapper.unmount()
  })

  it('loads a replacement detail when the selected topology and revision change', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await wrapper.setProps({
      topology: { id: 4, name: 'New regional mesh' },
      revisionID: 8,
      revisionDetail: {
        revision: { id: 8, message: 'New revision' },
        vertices: [{ key: 'new-entry', kind: 'agent', config: '{"port":8443}' }],
        edges: [],
      },
    })

    expect(bodyGet('#topology-editor-json').element.value).toContain('new-entry')
    expect(bodyGet('#topology-revision-message').element.value).toBe('New revision')
    expect(inBody('.topology-dirty').exists()).toBe(false)
    wrapper.unmount()
  })

  it('emits preview-compatible UI option names without route-layer request fields', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await bodyGet('#topology-rollout-group').setValue('canary-a')
    await bodyGet('#topology-plan').trigger('click')

    expect(wrapper.emitted('plan')).toEqual([[
      {
        topologyID: 3,
        revisionID: 7,
        options: {
          rolloutGroup: 'canary-a',
          failurePolicy: 'stop_and_rollback',
        },
      },
    ]])
    wrapper.unmount()
  })

  it('collects the kernel topology creation shape when opened without a topology', async () => {
    const wrapper = mountWorkspace({ topology: null, revisionID: 0, revisionDetail: null })
    await nextTick()

    await bodyGet('#new-topology-name').setValue('China egress')
    // UiSelect (Reka) has no native value: choose through the component.
    const scope = wrapper.findAllComponents(UiSelect).find(select => select.props('id') === 'new-topology-scope')
    expect(scope.props('options').map(option => option.value)).toContain('forward')
    scope.vm.$emit('update:modelValue', 'forward')
    await nextTick()
    await bodyGet('#new-topology-description').setValue('Regional egress rollout')
    await bodyGet('#create-topology').trigger('click')

    expect(wrapper.emitted('create-topology')).toEqual([[
      { name: 'China egress', service_scope: 'forward', description: 'Regional egress rollout' },
    ]])
    wrapper.unmount()
  })
})

describe('TopologyWorkspace field labels', () => {
  // Every text control has a real <label> (UiTextField / UiTextarea), so it
  // has an accessible name and a click on the label focuses it.
  it('names every text field of the revision editor', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    for (const [name, id] of [['Rollout group', 'topology-rollout-group'], ['Revision message', 'topology-revision-message'], ['Graph JSON', 'topology-editor-json']]) {
      const field = screen.getByRole('textbox', { name })
      expect(field.id).toBe(id)
      const label = document.querySelector(`label[for="${id}"]`)
      expect(label.textContent.trim()).toBe(name)
    }
    // The JSON help is the field's description, not a loose paragraph.
    const json = screen.getByRole('textbox', { name: 'Graph JSON' })
    expect(document.getElementById(json.getAttribute('aria-describedby')).textContent).toContain('Secrets must be referenced by secret_id')
    // The selects keep their labels too.
    expect(screen.getByRole('combobox', { name: 'Revision' })).toBeTruthy()
    expect(screen.getByRole('combobox', { name: 'Failure policy' })).toBeTruthy()
    wrapper.unmount()
  })

  it('names the fields of the create form and keeps them locked while saving', async () => {
    const wrapper = mountWorkspace({ topology: null, revisionID: 0, revisionDetail: null })
    await nextTick()

    expect(screen.getByRole('textbox', { name: 'Name' }).id).toBe('new-topology-name')
    expect(screen.getByRole('textbox', { name: 'Description' }).id).toBe('new-topology-description')
    expect(screen.getByRole('combobox', { name: 'Scope' })).toBeTruthy()

    await wrapper.setProps({ saving: true })
    expect(screen.getByRole('textbox', { name: 'Name' }).disabled).toBe(true)
    expect(screen.getByRole('textbox', { name: 'Description' }).disabled).toBe(true)
    wrapper.unmount()
  })
})

describe('TopologyWorkspace graph preview', () => {
  it('loads G6 only for the 图示 view and draws the JSON being edited', async () => {
    g6.graphs.length = 0
    const wrapper = mountWorkspace({
      revisionDetail: {
        revision: { id: 7, message: 'Published revision' },
        vertices: [
          { key: 'entry', kind: 'agent', node_id: 11, plugin_id: 'gost-mesh', role: 'relay', config: '{}' },
          { key: 'exit', kind: 'agent', node_id: 12, plugin_id: 'nat-egress', role: 'nat_egress', config: '{}' },
        ],
        edges: [{ source_key: 'entry', target_key: 'exit', protocol: 'tls', config: '{}' }],
      },
    })
    await nextTick()
    await nextTick()
    expect(inBody('[data-testid="topology-graph-preview"]').exists()).toBe(false)
    await flushPromises()
    expect(g6.graphs).toHaveLength(0)

    wrapper.findComponent(UiSegmentedControl).vm.$emit('update:modelValue', 'graph')
    await flushPromises()
    await flushPromises()
    const preview = bodyGet('[data-testid="topology-graph-preview"]')
    expect(preview.get('[role="img"]').attributes('aria-label')).toContain('2')
    expect(preview.text()).toContain('entry → exit')
    expect(g6.graphs).toHaveLength(1)
    const data = g6.graphs[0].options.data
    expect(data.nodes.map(node => node.id)).toEqual(['entry', 'exit'])
    expect(data.edges).toEqual([expect.objectContaining({ source: 'entry', target: 'exit', style: { labelText: 'tls' } })])

    await bodyGet('#topology-editor-json').setValue('{ not json')
    await flushPromises()
    expect(bodyGet('[data-testid="topology-graph-preview"]').text()).toContain('invalid')
    wrapper.unmount()
  })
})

describe('OperationTimeline', () => {
  it('shows scoped activity first, exposes global history explicitly, and only cancels cancellable operations', async () => {
    const wrapper = mount(OperationTimeline, {
      props: {
        operations: [
          { id: 'running', kind: 'deployment.apply', state: 'running', created_at: '2026-07-18T01:00:00Z' },
          { id: 'completed', kind: 'deployment.plan', state: 'completed', last_error: 'none', created_at: '2026-07-18T02:00:00Z' },
        ],
        scopedOperationIDs: ['running'],
      },
    })

    expect(wrapper.get('[data-testid="operation-row-running"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="operation-row-completed"]').exists()).toBe(false)
    await wrapper.get('[data-testid="cancel-operation-running"]').trigger('click')
    expect(wrapper.emitted('cancel')).toEqual([['running']])

    await wrapper.get('[data-testid="show-all-activity"]').trigger('click')
    const rows = wrapper.findAll('[data-testid^="operation-row-"]')
    expect(rows.map(row => row.attributes('data-testid'))).toEqual(['operation-row-completed', 'operation-row-running'])
    expect(wrapper.find('[data-testid="cancel-operation-completed"]').exists()).toBe(false)
    // The state is a word in a badge, not only the marker colour.
    expect(wrapper.get('[data-testid="operation-row-completed"]').text()).toContain('completed')
    expect(wrapper.get('[data-testid="operation-row-completed"]').text()).toContain('none')
    expect(wrapper.get('ol').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows an empty state and keeps a heading for its region', () => {
    const wrapper = mount(OperationTimeline, {
      props: { operations: [], heading: 'Recent plugin operations', emptyLabel: 'No recent plugin operations', showToggle: false },
    })
    const region = wrapper.get('[data-testid="operation-timeline"]')
    expect(region.attributes('aria-labelledby')).toBe(wrapper.get('h2').attributes('id'))
    expect(wrapper.get('h2').text()).toBe('Recent plugin operations')
    expect(wrapper.text()).toContain('No recent plugin operations')
    expect(wrapper.find('[data-testid="show-all-activity"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('switches revisions through the revision UiSelect', async () => {
    const wrapper = mountWorkspace({ revisions: [{ id: 7, revision: 2 }, { id: 9, revision: 3 }] })
    await nextTick()
    const selects = wrapper.findAllComponents(UiSelect)
    const revision = selects.find(select => select.props('id') === 'topology-revision-selector')
    expect(revision.props('options')).toEqual([{ value: 7, label: 'r2 (#7)' }, { value: 9, label: 'r3 (#9)' }])
    expect(revision.props('modelValue')).toBe(7)
    revision.vm.$emit('update:modelValue', 9)
    await nextTick()
    expect(wrapper.emitted('select-revision')).toEqual([[{ topologyID: 3, revisionID: 9 }]])
    expect(selects.find(select => select.props('id') === 'topology-failure-policy').props('modelValue')).toBe('stop_and_rollback')
    wrapper.unmount()
  })
})
