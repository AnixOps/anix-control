import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import Forward from '@/views/admin/Forward.vue'
import i18n, { setLocale } from '@/i18n'

const adminApi = vi.hoisted(() => ({
  createForward: vi.fn(),
  getForwardList: vi.fn(),
  updateForward: vi.fn(),
  deleteForward: vi.fn(),
  forceDeleteForward: vi.fn(),
  pauseForwardService: vi.fn(),
  resumeForwardService: vi.fn(),
  diagnoseForward: vi.fn(),
  updateForwardOrder: vi.fn(),
  getForwardTunnels: vi.fn(),
  getSystemConfig: vi.fn()
}))

vi.mock('@/api/admin', () => adminApi)

function mountForward() {
  return mount(Forward, {
    global: {
      stubs: {
        'router-link': {
          template: '<a><slot /></a>'
        }
      }
    }
  })
}

describe('Forward.vue', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.resetAllMocks()
    localStorage.clear()
    await setLocale('en')

    adminApi.createForward.mockResolvedValue({ code: 0 })
    adminApi.updateForward.mockResolvedValue({ code: 0 })
    adminApi.deleteForward.mockResolvedValue({ code: 0 })
    adminApi.forceDeleteForward.mockResolvedValue({ code: 0 })
    adminApi.pauseForwardService.mockResolvedValue({ code: 0 })
    adminApi.resumeForwardService.mockResolvedValue({ code: 0 })
    adminApi.diagnoseForward.mockResolvedValue({ code: 0, data: { results: [] } })
    adminApi.updateForwardOrder.mockResolvedValue({ code: 0 })
    adminApi.getForwardList.mockResolvedValue({ code: 0, data: [] })
    adminApi.getForwardTunnels.mockResolvedValue({
      code: 0,
      data: [
        { id: 1, name: 'Port Tunnel', type: 1, inNodePortSta: 1000, inNodePortEnd: 2000 },
        { id: 2, name: 'NodeX Tunnel', type: 2, inNodePortSta: 3000, inNodePortEnd: 4000 }
      ]
    })
    adminApi.getSystemConfig.mockImplementation((key) => {
      if (key === 'forward.runtime.nodex_mode') {
        return Promise.resolve({ data: { value: 'false' } })
      }
      if (key === 'forward.runtime_backend') {
        return Promise.resolve({ data: { value: 'nftables_ansible' } })
      }
      return Promise.resolve({ data: { value: '' } })
    })
  })

  it('filters tunnel options for local ansible runtime', async () => {
    const wrapper = mountForward()
    await flushPromises()

    wrapper.vm.openCreateModal()
    await wrapper.vm.$nextTick()

    const options = wrapper.findAll('[data-test="forward-tunnel-select"] option').map(option => option.text())

    expect(wrapper.text()).toContain(i18n.global.t('runtime.forward.modeCompatibilityHint'))
    expect(options.some(text => text.includes('Port Tunnel'))).toBe(true)
    expect(options.some(text => text.includes('NodeX Tunnel'))).toBe(false)
  })

  it('blocks editing a NodeX-only tunnel while local ansible runtime is active', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 9,
          userId: 1,
          name: 'Legacy NodeX Forward',
          tunnelId: 2,
          tunnelName: 'NodeX Tunnel',
          inPort: 3100,
          remoteAddr: 'example.com:443',
          status: 1,
          inx: 1
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    wrapper.vm.openEditModal(wrapper.vm.forwards[0])
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain(
      i18n.global.t('runtime.forward.tunnelHintLocalIncompatible', { name: 'NodeX Tunnel' })
    )

    await wrapper.vm.handleSubmit()
    await flushPromises()

    expect(adminApi.updateForward).not.toHaveBeenCalled()
    expect(wrapper.vm.errors.tunnelId).toBe(
      i18n.global.t('runtime.forward.messages.localRuntimeTunnelForwardUnsupported')
    )
  })
})
