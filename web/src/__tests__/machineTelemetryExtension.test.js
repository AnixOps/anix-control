import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, h, onMounted, readonly, ref } from 'vue'
import createMachineTelemetryExtension, { anixopsExtension } from '@/extensions/modules/machine-telemetry/index.js'

function host(request) {
  return {
    pluginID: 'machine-telemetry',
    version: '1.1.0',
    computed,
    defineComponent,
    h,
    onMounted,
    readonly,
    ref,
    request,
  }
}

describe('machine-telemetry WebUI package', () => {
  it('binds its import-free factory to the versioned host contract', () => {
    expect(anixopsExtension).toEqual({
      pluginId: 'machine-telemetry',
      version: '1.1.0',
      webuiApiVersion: 'anixops.webui/v1',
      bundle: { path: 'webui/index.mjs' },
    })
  })

  it('loads and renders live data only through its namespaced package API', async () => {
    const request = vi.fn(async () => ({
      summary: { total: 2, online: 1, offline: 1, unhealthy: 0 },
      nodes: [
        { id: 1, name: 'edge-one', host: '10.0.0.1', online: true, runtime_healthy: true, cpu_usage: 12.5, memory_usage: 40, disk_usage: 70, online_users: 3 },
        { id: 2, name: 'edge-two', host: '10.0.0.2', online: false, runtime_healthy: true, cpu_usage: 0, memory_usage: 0, disk_usage: 20, online_users: 0 },
      ],
    }))
    const wrapper = mount(createMachineTelemetryExtension(host(request)))
    await flushPromises()

    expect(request).toHaveBeenCalledWith('/api/v3/plugins/machine-telemetry/status?limit=200')
    expect(wrapper.text()).toContain('edge-one')
    expect(wrapper.text()).toContain('12.5%')
    expect(wrapper.text()).toContain('Online')
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)

    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('isolates backend failures inside the extension page', async () => {
    const wrapper = mount(createMachineTelemetryExtension(host(vi.fn(async () => {
      throw new Error('executor unavailable')
    }))))
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toBe('executor unavailable')
    expect(wrapper.text()).toContain('No telemetry')
  })
})
