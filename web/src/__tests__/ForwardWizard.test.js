import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ForwardWizard from '@/views/admin/ForwardWizard.vue'

describe('ForwardWizard.vue', () => {
  it('renders the lightweight local stepper without Arco components', () => {
    const wrapper = mount(ForwardWizard, {
      global: {
        stubs: {
          StepForwardModeForm: { template: '<section data-test="mode-step" />' },
          StepMachineForm: true,
          StepNodeForm: true,
          StepTunnelForm: true,
          StepForwardForm: true,
          RouterLink: true
        }
      }
    })

    const steps = wrapper.findAll('.wizard-step')
    expect(steps).toHaveLength(5)
    expect(steps[0].attributes('aria-current')).toBe('step')
    expect(steps[0].classes()).toContain('active')
    expect(wrapper.findComponent({ name: 'ASteps' }).exists()).toBe(false)
    expect(wrapper.find('[data-test="mode-step"]').exists()).toBe(true)
  })

  it('shows the generated API token of the node it created once, after the node step', async () => {
    const wrapper = mount(ForwardWizard, {
      global: {
        stubs: {
          StepForwardModeForm: true,
          StepMachineForm: true,
          StepNodeForm: true,
          StepTunnelForm: true,
          StepForwardForm: true,
          RouterLink: true
        }
      }
    })
    expect(wrapper.find('[data-testid="wizard-node-token"]').exists()).toBe(false)

    wrapper.vm.handleModeSelected({ nodeXMode: true, tunnelType: 1 })
    wrapper.vm.handleNodeCreated({ id: 7, name: 'relay-hk', apiToken: 'generated-relay-token' })
    await wrapper.vm.$nextTick()

    const notice = wrapper.get('[data-testid="wizard-node-token"]')
    expect(notice.text()).toContain('relay-hk')
    expect(notice.get('input').element.value).toBe('generated-relay-token')
    expect(wrapper.vm.stepIndex).toBe(2)
  })

  it('puts 上一步 / 下一步 in the footer: the primary button submits the step form', async () => {
    const step = {
      template: '<form id="wizard-step-form" @submit.prevent="submit"><span data-test="step" /></form>',
      setup(_, { expose }) {
        const calls = []
        const submit = () => calls.push('submit')
        expose({ submit, busy: false, canSubmit: true, primaryLabel: 'Create and continue', calls })
        return { submit }
      }
    }
    const wrapper = mount(ForwardWizard, {
      attachTo: document.body,
      global: {
        stubs: { StepForwardModeForm: step, StepMachineForm: step, StepNodeForm: step, StepTunnelForm: step, StepForwardForm: step, RouterLink: true }
      }
    })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-test="wizard-back"]').exists()).toBe(false)
    const next = wrapper.get('[data-test="wizard-next"]')
    expect(next.attributes('type')).toBe('submit')
    expect(next.attributes('form')).toBe('wizard-step-form')
    expect(next.text()).toBe('Create and continue')

    wrapper.vm.handleModeSelected({ nodeXMode: false, tunnelType: 1 })
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.stepIndex).toBe(1)
    expect(wrapper.findAll('.wizard-step')[0].classes()).toContain('complete')
    await wrapper.get('[data-test="wizard-back"]').trigger('click')
    expect(wrapper.vm.stepIndex).toBe(0)

    wrapper.vm.handleModeSelected({ nodeXMode: false, tunnelType: 1 })
    wrapper.vm.handleNodeCreated({ id: 3, name: 'relay-1' })
    wrapper.vm.handleTunnelCreated({ id: 4, name: 'tunnel-1' })
    wrapper.vm.handleForwardCreated({ id: 5, name: 'forward-1' })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-test="wizard-next"]').exists()).toBe(false)
    await wrapper.get('[data-test="wizard-create-another"]').trigger('click')
    expect(wrapper.vm.stepIndex).toBe(3)
    wrapper.unmount()
  })
})

