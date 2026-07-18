import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import OperationTimeline from '@/components/admin/OperationTimeline.vue'
import TopologyWorkspace from '@/components/admin/TopologyWorkspace.vue'

const revisionDetail = {
  revision: { id: 7, message: 'Published revision' },
  vertices: [{ key: 'entry', kind: 'agent', node_id: 11, plugin_id: 'gost-mesh', role: 'relay', config: '{"port":443}' }],
  edges: [],
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
  it('requires named confirmations before applying or rolling back a clean immutable revision', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await wrapper.get('#topology-diagnose').trigger('click')
    expect(wrapper.emitted('diagnose')).toEqual([[
      expect.objectContaining({
        topologyID: 3,
        revisionID: 7,
        options: expect.objectContaining({ rolloutGroup: '', failurePolicy: 'stop_and_rollback' }),
      }),
    ]])

    await wrapper.get('#topology-preview').trigger('click')
    await wrapper.get('#topology-plan').trigger('click')
    expect(wrapper.emitted('preview')).toHaveLength(1)
    expect(wrapper.emitted('plan')).toHaveLength(1)

    await wrapper.get('#topology-apply').trigger('click')
    expect(wrapper.emitted('apply')).toBeUndefined()
    expect(wrapper.get('[data-testid="apply-confirmation"]').text()).toContain('Regional mesh')
    await wrapper.get('[data-testid="confirm-apply"]').trigger('click')
    expect(wrapper.emitted('apply')).toEqual([[{ deploymentID: 13 }]])

    await wrapper.get('#topology-rollback').trigger('click')
    expect(wrapper.emitted('rollback')).toBeUndefined()
    expect(wrapper.get('[data-testid="rollback-confirmation"]').text()).toContain('Regional mesh')
    await wrapper.get('[data-testid="confirm-rollback"]').trigger('click')
    expect(wrapper.emitted('rollback')).toEqual([[{ deploymentID: 13 }]])
    wrapper.unmount()
  })

  it('does not roll back when a confirmed deployment becomes dirty', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await wrapper.get('#topology-rollback').trigger('click')
    const confirm = wrapper.get('[data-testid="confirm-rollback"]')
    await wrapper.get('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')

    expect(confirm.attributes('disabled')).toBeDefined()
    await confirm.trigger('click')
    expect(wrapper.emitted('rollback')).toBeUndefined()
    wrapper.unmount()
  })

  it('binds confirmations to one deployment and invalidates them when its identity or state changes', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await wrapper.get('#topology-apply').trigger('click')
    expect(wrapper.get('[data-testid="apply-confirmation"]').text()).toContain('#13')
    expect(wrapper.get('#topology-plan').attributes('disabled')).toBeDefined()

    await wrapper.setProps({
      deploymentID: 14,
      deploymentStatus: { deployment: { id: 14, state: 'planned' }, operations: [] },
    })
    expect(wrapper.find('[data-testid="apply-confirmation"]').exists()).toBe(false)

    await wrapper.get('#topology-rollback').trigger('click')
    expect(wrapper.find('[data-testid="rollback-confirmation"]').exists()).toBe(true)
    await wrapper.setProps({ deploymentStatus: { deployment: { id: 14, state: 'applying' }, operations: [] } })
    expect(wrapper.find('[data-testid="rollback-confirmation"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps plan, apply, and rollback unavailable while the revision differs from its baseline', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await wrapper.get('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')
    expect(wrapper.get('.topology-dirty').text()).toContain('unsaved changes')
    for (const selector of ['#topology-plan', '#topology-apply', '#topology-rollback']) {
      expect(wrapper.get(selector).attributes('disabled')).toBeDefined()
    }

    await wrapper.get('#topology-save-revision').trigger('click')
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

    await wrapper.get('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')
    await wrapper.setProps({ topology: { id: 3, name: 'Regional mesh refreshed' } })

    expect(wrapper.get('#topology-editor-json').element.value).toBe('{"vertices":[],"edges":[]}')
    expect(wrapper.get('.topology-dirty').exists()).toBe(true)
    wrapper.unmount()
  })

  it('retains a dirty draft when the same selected revision detail is replaced asynchronously', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await wrapper.get('#topology-editor-json').setValue('{"vertices":[],"edges":[]}')
    await wrapper.get('#topology-revision-message').setValue('Local draft')
    await wrapper.setProps({
      revisionDetail: {
        revision: { id: 7, message: 'Server refresh' },
        vertices: [{ key: 'server-entry', kind: 'agent', config: '{"port":8443}' }],
        edges: [],
      },
    })

    expect(wrapper.get('#topology-editor-json').element.value).toBe('{"vertices":[],"edges":[]}')
    expect(wrapper.get('#topology-revision-message').element.value).toBe('Local draft')
    expect(wrapper.get('.topology-dirty').exists()).toBe(true)
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

    expect(wrapper.get('#topology-editor-json').element.value).toContain('new-entry')
    expect(wrapper.get('#topology-revision-message').element.value).toBe('New revision')
    expect(wrapper.find('.topology-dirty').exists()).toBe(false)
    wrapper.unmount()
  })

  it('emits preview-compatible UI option names without route-layer request fields', async () => {
    const wrapper = mountWorkspace()
    await nextTick()
    await nextTick()

    await wrapper.get('#topology-rollout-group').setValue('canary-a')
    await wrapper.get('#topology-plan').trigger('click')

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

    await wrapper.get('#new-topology-name').setValue('China egress')
    await wrapper.get('#new-topology-scope').setValue('forward')
    await wrapper.get('#new-topology-description').setValue('Regional egress rollout')
    await wrapper.get('#create-topology').trigger('click')

    expect(wrapper.emitted('create-topology')).toEqual([[
      { name: 'China egress', service_scope: 'forward', description: 'Regional egress rollout' },
    ]])
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
    wrapper.unmount()
  })
})
