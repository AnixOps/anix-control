import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import Forward from '@/views/admin/Forward.vue'
import i18n, { setLocale } from '@/i18n'
import { useUserStore } from '@/stores/user'

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

let wrapper = null

function deferred() {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function mountForward() {
  wrapper = mount(Forward, {
    global: {
      stubs: {
        'router-link': {
          template: '<a><slot /></a>'
        }
      }
    }
  })
  return wrapper
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

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    vi.unstubAllGlobals()
    vi.useRealTimers()
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

  it('renders direct forwards in the compact table without duplicate suite nav', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 11,
          userId: 1,
          userName: 'operator@example.com',
          name: 'Web Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10001,
          remoteAddr: 'example.com:443',
          strategy: 'round',
          status: 1,
          runtimeBackend: 'nftables_ansible',
          runtimeStatus: 2,
          inFlow: 1024,
          outFlow: 2048,
          inx: 1
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    expect(wrapper.find('.forward-table').exists()).toBe(true)
    expect(wrapper.find('.forward-suite-nav').exists()).toBe(false)
    expect(wrapper.text()).toContain(i18n.global.t('runtime.forward.table.rule'))
    expect(wrapper.text()).toContain('Web Entry')
    expect(wrapper.text()).toContain('Port Tunnel')
    expect(wrapper.text()).toContain('203.0.113.10:10001')
    expect(wrapper.text()).toContain('example.com:443')
  })

  it('filters direct forwards and selects only the visible rules', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 51,
          userId: 1,
          userName: 'alice@example.com',
          name: 'Alpha Web',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 1501,
          remoteAddr: 'alpha.example.com:443',
          status: 1,
          runtimeBackend: 'nftables_ansible',
          runtimeStatus: 2,
          inx: 1
        },
        {
          id: 52,
          userId: 2,
          userName: 'bob@example.com',
          name: 'Beta API',
          tunnelId: 2,
          tunnelName: 'NodeX Tunnel',
          inIp: '203.0.113.11',
          inPort: 1502,
          remoteAddr: 'beta.example.com:8443',
          status: 0,
          inx: 2
        },
        {
          id: 53,
          userId: 3,
          userName: 'carol@example.com',
          name: 'Gamma Fault',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.12',
          inPort: 1503,
          remoteAddr: 'gamma.example.com:9443',
          status: 1,
          runtimeBackend: 'nftables_ansible',
          runtimeStatus: 3,
          runtimeMessage: 'apply failed',
          inx: 3
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    await wrapper.find('[data-test="forward-filter-keyword"]').setValue('alpha')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Alpha Web')
    expect(wrapper.text()).not.toContain('Beta API')
    expect(wrapper.text()).not.toContain('Gamma Fault')

    await wrapper.find('[data-test="forward-select-all"]').setValue(true)
    expect(wrapper.vm.selectedDirectForwards.map(item => item.id)).toEqual([51])

    await wrapper.find('[data-test="forward-filter-clear"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.find('[data-test="forward-filter-tunnel"]').setValue('2')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).not.toContain('Alpha Web')
    expect(wrapper.text()).toContain('Beta API')
    expect(wrapper.text()).not.toContain('Gamma Fault')

    await wrapper.find('[data-test="forward-select-all"]').setValue(true)
    expect(wrapper.vm.selectedDirectForwards.map(item => item.id)).toEqual([52])

    await wrapper.find('[data-test="forward-filter-clear"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.find('[data-test="forward-filter-status"]').setValue('error')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).not.toContain('Alpha Web')
    expect(wrapper.text()).not.toContain('Beta API')
    expect(wrapper.text()).toContain('Gamma Fault')

    await wrapper.find('[data-test="forward-select-all"]').setValue(true)
    expect(wrapper.vm.selectedDirectForwards.map(item => item.id)).toEqual([53])
  })

  it('shows every forward in admin direct view instead of filtering to the admin user id', async () => {
    const userStore = useUserStore()
    userStore.login('admin-token', { id: 99, is_admin: true })

    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 21,
          userId: 1,
          userName: 'alice@example.com',
          name: 'Alice Web',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 1101,
          remoteAddr: 'alice.example.com:443',
          status: 1,
          inx: 1
        },
        {
          id: 22,
          userId: 2,
          userName: 'bob@example.com',
          name: 'Bob API',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 1102,
          remoteAddr: 'bob.example.com:443',
          status: 1,
          inx: 2
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    expect(wrapper.text()).toContain('Alice Web')
    expect(wrapper.text()).toContain('Bob API')

    await wrapper.find('[data-test="forward-select-all"]').setValue(true)
    expect(wrapper.vm.selectedDirectForwards.map(item => item.id)).toEqual([21, 22])
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

  it('runs batch pause only for selected direct forwards', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 11,
          userId: 1,
          name: 'Web Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10001,
          remoteAddr: 'example.com:443',
          status: 1,
          inx: 1
        },
        {
          id: 12,
          userId: 1,
          name: 'API Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10002,
          remoteAddr: 'api.example.com:443',
          status: 1,
          inx: 2
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    await wrapper.findAll('[data-test="forward-row-select"]')[0].setValue(true)
    await wrapper.find('[data-test="forward-bulk-pause"]').trigger('click')
    await flushPromises()

    expect(adminApi.pauseForwardService).toHaveBeenCalledTimes(1)
    expect(adminApi.pauseForwardService).toHaveBeenCalledWith(11)
    expect(adminApi.pauseForwardService).not.toHaveBeenCalledWith(12)
    expect(wrapper.find('[data-test="forward-bulk-toolbar"]').exists()).toBe(false)
  })

  it('runs batch pause only for forwards that are currently running', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 21,
          userId: 1,
          name: 'Running Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10011,
          remoteAddr: 'running.example.com:443',
          status: 1,
          inx: 1
        },
        {
          id: 22,
          userId: 1,
          name: 'Paused Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10012,
          remoteAddr: 'paused.example.com:443',
          status: 0,
          inx: 2
        },
        {
          id: 23,
          userId: 1,
          name: 'Pending Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10013,
          remoteAddr: 'pending.example.com:443',
          status: 1,
          runtimeBackend: 'nftables_ansible',
          runtimeStatus: 0,
          inx: 3
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    const selectors = wrapper.findAll('[data-test="forward-row-select"]')
    await selectors[0].setValue(true)
    await selectors[1].setValue(true)
    await selectors[2].setValue(true)
    await wrapper.find('[data-test="forward-bulk-pause"]').trigger('click')
    await flushPromises()

    expect(adminApi.pauseForwardService).toHaveBeenCalledTimes(1)
    expect(adminApi.pauseForwardService).toHaveBeenCalledWith(21)
    expect(adminApi.pauseForwardService).not.toHaveBeenCalledWith(22)
    expect(adminApi.pauseForwardService).not.toHaveBeenCalledWith(23)
    expect(adminApi.resumeForwardService).not.toHaveBeenCalled()
  })

  it('runs batch resume only for forwards that are currently paused', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 31,
          userId: 1,
          name: 'Running Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10021,
          remoteAddr: 'running.example.com:443',
          status: 1,
          inx: 1
        },
        {
          id: 32,
          userId: 1,
          name: 'Paused Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10022,
          remoteAddr: 'paused.example.com:443',
          status: 0,
          inx: 2
        },
        {
          id: 33,
          userId: 1,
          name: 'Pending Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10023,
          remoteAddr: 'pending.example.com:443',
          status: 0,
          runtimeBackend: 'nftables_ansible',
          runtimeStatus: 1,
          inx: 3
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    const selectors = wrapper.findAll('[data-test="forward-row-select"]')
    await selectors[0].setValue(true)
    await selectors[1].setValue(true)
    await selectors[2].setValue(true)
    await wrapper.find('[data-test="forward-bulk-resume"]').trigger('click')
    await flushPromises()

    expect(adminApi.resumeForwardService).toHaveBeenCalledTimes(1)
    expect(adminApi.resumeForwardService).not.toHaveBeenCalledWith(31)
    expect(adminApi.resumeForwardService).toHaveBeenCalledWith(32)
    expect(adminApi.resumeForwardService).not.toHaveBeenCalledWith(33)
    expect(adminApi.pauseForwardService).not.toHaveBeenCalled()
  })

  it('exports selected forwards as relay-panel compatible JSON', async () => {
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 41,
          userId: 1,
          name: 'Relay JSON Rule',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 1401,
          remoteAddr: '1.1.1.1:443,2.2.2.2:8443',
          status: 1,
          inx: 1
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    await wrapper.find('[data-test="forward-row-select"]').setValue(true)
    await wrapper.find('[data-test="forward-bulk-export"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(JSON.parse(wrapper.vm.exportData)).toEqual([
      {
        dest: ['1.1.1.1:443', '2.2.2.2:8443'],
        listen_port: 1401,
        name: 'Relay JSON Rule'
      }
    ])
  })

  it('imports relay-panel JSON forward rules', async () => {
    const wrapper = mountForward()
    await flushPromises()

    wrapper.vm.selectedTunnelForImport = 1
    wrapper.vm.importData = JSON.stringify([
      {
        dest: ['1.1.1.1:443', '2.2.2.2:8443'],
        listen_port: 1501,
        name: 'Imported Relay Rule'
      }
    ])

    await wrapper.vm.executeImport()
    await flushPromises()

    expect(adminApi.createForward).toHaveBeenCalledWith({
      name: 'Imported Relay Rule',
      tunnelId: 1,
      inPort: 1501,
      remoteAddr: '1.1.1.1:443,2.2.2.2:8443',
      strategy: 'fifo'
    })
  })

  it('keeps legacy pipe-line import compatibility', async () => {
    const wrapper = mountForward()
    await flushPromises()

    wrapper.vm.selectedTunnelForImport = 1
    wrapper.vm.importData = 'legacy.example.com:443|Legacy Rule|1601'

    await wrapper.vm.executeImport()
    await flushPromises()

    expect(adminApi.createForward).toHaveBeenCalledWith({
      name: 'Legacy Rule',
      tunnelId: 1,
      inPort: 1601,
      remoteAddr: 'legacy.example.com:443',
      strategy: 'fifo'
    })
  })

  it('refreshes direct forwards automatically and pauses while a modal is open', async () => {
    vi.useFakeTimers()
    adminApi.getForwardList.mockResolvedValue({
      code: 0,
      data: [
        {
          id: 11,
          userId: 1,
          name: 'Web Entry',
          tunnelId: 1,
          tunnelName: 'Port Tunnel',
          inIp: '203.0.113.10',
          inPort: 10001,
          remoteAddr: 'example.com:443',
          status: 1,
          inx: 1
        }
      ]
    })

    const wrapper = mountForward()
    await flushPromises()

    expect(adminApi.getForwardList).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(10000)
    await flushPromises()
    expect(adminApi.getForwardList).toHaveBeenCalledTimes(2)

    wrapper.vm.openCreateModal()
    await wrapper.vm.$nextTick()
    await vi.advanceTimersByTimeAsync(10000)
    await flushPromises()

    expect(adminApi.getForwardList).toHaveBeenCalledTimes(2)
  })

  it('forces a fresh list reload after create when a silent refresh is already in flight', async () => {
    const staleRefresh = deferred()
    adminApi.getForwardList
      .mockResolvedValueOnce({ code: 0, data: [] })
      .mockReturnValueOnce(staleRefresh.promise)
      .mockResolvedValueOnce({
        code: 0,
        data: [
          {
            id: 31,
            userId: 1,
            name: 'Fresh Forward',
            tunnelId: 1,
            tunnelName: 'Port Tunnel',
            inIp: '203.0.113.10',
            inPort: 1031,
            remoteAddr: 'fresh.example.com:443',
            status: 1,
            inx: 1
          }
        ]
      })

    const wrapper = mountForward()
    await flushPromises()

    const silentRefreshPromise = wrapper.vm.loadData(false, { silent: true })
    await wrapper.vm.$nextTick()
    expect(adminApi.getForwardList).toHaveBeenCalledTimes(2)

    wrapper.vm.openCreateModal()
    Object.assign(wrapper.vm.form, {
      name: 'Fresh Forward',
      tunnelId: 1,
      inPort: 1031,
      remoteAddr: 'fresh.example.com:443',
      interfaceName: '',
      strategy: 'fifo'
    })
    await wrapper.vm.$nextTick()

    const submitPromise = wrapper.vm.handleSubmit()
    await flushPromises()
    expect(adminApi.createForward).toHaveBeenCalledTimes(1)
    expect(adminApi.getForwardList).toHaveBeenCalledTimes(2)

    staleRefresh.resolve({ code: 0, data: [] })
    await silentRefreshPromise
    await submitPromise
    await flushPromises()

    expect(adminApi.getForwardList).toHaveBeenCalledTimes(3)
    expect(wrapper.text()).toContain('Fresh Forward')
  })
})
