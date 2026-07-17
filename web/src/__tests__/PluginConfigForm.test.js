import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import PluginConfigForm from '@/components/admin/PluginConfigForm.vue'

describe('PluginConfigForm', () => {
  it('edits primitive, enum, nested object, and array schema fields', async () => {
    const schema = {
          type: 'object',
          properties: {
            name: { type: 'string', title: 'Name' },
            port: { type: 'integer', title: 'Port' },
            enabled: { type: 'boolean', title: 'Enabled' },
            mode: { type: 'string', title: 'Mode', enum: ['quic', 'wss'] },
            remote: { type: 'object', title: 'Remote', properties: { host: { type: 'string', title: 'Host' } } },
            peers: { type: 'array', title: 'Peers', items: { type: 'string' } },
          },
          required: ['name', 'port'],
        }
    const Host = defineComponent({
      components: { PluginConfigForm },
      setup() {
        const config = ref({ name: 'entry', port: 443, enabled: true, mode: 'quic', remote: { host: 'exit.example' }, peers: ['10.0.0.2'] })
        return { config, schema }
      },
      template: '<PluginConfigForm v-model="config" :schema="schema" />',
    })
    const wrapper = mount(Host)

    await wrapper.get('#plugin-config-name').setValue('updated')
    await wrapper.get('#plugin-config-port').setValue('8443')
    await wrapper.get('#plugin-config-enabled').setValue(false)
    await wrapper.get('#plugin-config-mode').setValue('1')
    await wrapper.get('#plugin-config-remote-host').setValue('new.example')
    await wrapper.find('.array-heading button').trigger('click')
    await nextTick()

    expect(wrapper.vm.config).toEqual({
      name: 'updated', port: 8443, enabled: false, mode: 'wss', remote: { host: 'new.example' }, peers: ['10.0.0.2', ''],
    })
  })

  it('falls back to JSON for complex schemas and reports invalid JSON', async () => {
    const wrapper = mount(PluginConfigForm, {
      props: {
        schema: { oneOf: [{ type: 'object' }, { type: 'array' }] },
        modelValue: { enabled: true },
      },
    })

    const editor = wrapper.get('.full-json-editor')
    expect(editor.element.value).toContain('"enabled": true')
    await editor.setValue('{invalid')
    expect(wrapper.get('[role="alert"]').text()).toBeTruthy()
    expect(wrapper.emitted('validity').at(-1)[0]).toBe(false)
    await editor.setValue('{"enabled":false}')
    expect(wrapper.emitted('update:modelValue').at(-1)[0]).toEqual({ enabled: false })
    expect(wrapper.emitted('validity').at(-1)[0]).toBe(true)
  })

  it('materializes signed schema defaults for a new empty configuration', async () => {
    const wrapper = mount(PluginConfigForm, {
      props: {
        schema: {
          type: 'object',
          properties: {
            interval_seconds: { type: 'integer', default: 30 },
            enabled: { type: 'boolean', default: true },
            targets: { type: 'array', default: [] },
          },
          required: ['interval_seconds', 'enabled', 'targets'],
        },
        modelValue: {},
      },
    })
    await nextTick()
    expect(wrapper.emitted('update:modelValue').at(-1)[0]).toEqual({ interval_seconds: 30, enabled: true, targets: [] })
  })
})
