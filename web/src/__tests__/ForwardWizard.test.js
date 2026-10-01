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
    expect(notice.text()).toContain('generated-relay-token')
    expect(wrapper.vm.stepIndex).toBe(2)
  })
})
