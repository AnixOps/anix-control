import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PluginSecrets from '@/components/admin/PluginSecrets.vue'

const api = vi.hoisted(() => ({
  createKernelPluginSecret: vi.fn(),
  createKernelPluginSecretVersion: vi.fn(),
  deleteKernelPluginSecret: vi.fn(),
  deleteKernelPluginSecretVersion: vi.fn(),
  getKernelPluginSecret: vi.fn(),
  getKernelPluginSecretAudit: vi.fn(),
  getKernelPluginSecrets: vi.fn()
}))

vi.mock('@/api/kernel', () => api)

describe('PluginSecrets', () => {
  beforeEach(() => {
    for (const mock of Object.values(api)) mock.mockReset()
    api.getKernelPluginSecrets.mockResolvedValue([{ id: 'mesh-edge', name: 'Mesh edge', active_version: 1, updated_at: '2026-09-22T00:00:00Z' }])
    api.getKernelPluginSecret.mockResolvedValue({
      id: 'mesh-edge', name: 'Mesh edge', active_version: 1,
      versions: [{ version: 1, key_id: 'primary', files: [{ name: 'ca.pem', size: 4, sha256: 'a'.repeat(64) }] }]
    })
    api.getKernelPluginSecretAudit.mockResolvedValue([{ id: 1, action: 'create', outcome: 'succeeded', created_at: '2026-09-22T00:00:00Z' }])
  })

  it('loads metadata and renders canonical references without material', async () => {
    const wrapper = mount(PluginSecrets)
    await flushPromises()
    expect(api.getKernelPluginSecrets).toHaveBeenCalledOnce()
    await wrapper.findAll('button').find(button => button.text().includes('Inspect') || button.text().includes('查看')).trigger('click')
    await flushPromises()
    expect(api.getKernelPluginSecret).toHaveBeenCalledWith('mesh-edge')
    expect(wrapper.text()).toContain('ca.pem')
    expect(wrapper.text()).not.toContain('content_base64')
  })

  it('uploads selected files as base64 and clears the editor', async () => {
    api.createKernelPluginSecret.mockResolvedValue({ id: 'mesh-edge' })
    const wrapper = mount(PluginSecrets)
    await flushPromises()
    await wrapper.get('#new-plugin-secret').trigger('click')
    await wrapper.get('#secret-id').setValue('mesh-edge')
    await wrapper.get('#secret-name').setValue('Mesh edge')
    const file = new File(['cert'], 'ca.pem', { type: 'application/x-pem-file' })
    const input = wrapper.get('#secret-files')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await wrapper.get('#save-plugin-secret').trigger('click')
    await flushPromises()
    expect(api.createKernelPluginSecret).toHaveBeenCalledWith({
      id: 'mesh-edge', name: 'Mesh edge', description: '', files: [{ name: 'ca.pem', content_base64: 'Y2VydA==' }]
    })
    expect(wrapper.find('#secret-id').exists()).toBe(false)
  })
})
