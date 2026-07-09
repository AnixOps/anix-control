import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import Subscriptions from '@/views/admin/Subscriptions.vue'
import { setLocale } from '@/i18n'

const adminApiMock = vi.hoisted(() => ({
  createSubscriptionGroup: vi.fn(),
  createSubscriptionTemplate: vi.fn(),
  deleteSubscriptionGroup: vi.fn(),
  deleteSubscriptionTemplate: vi.fn(),
  getAvailableProtocols: vi.fn(),
  getSubscriptionGroups: vi.fn(),
  getSubscriptionProtocols: vi.fn(),
  getSubscriptionTemplates: vi.fn(),
  previewSubscription: vi.fn(),
  updateSubscriptionGroup: vi.fn(),
  updateSubscriptionTemplate: vi.fn()
}))

const getSubscriptionStatsMock = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  default: adminApiMock,
  getSubscriptionStats: (...args) => getSubscriptionStatsMock(...args)
}))

describe('Admin Subscriptions', () => {
  beforeEach(async () => {
    vi.resetAllMocks()
    setActivePinia(createPinia())
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await setLocale('en')

    adminApiMock.getSubscriptionGroups.mockResolvedValue({ data: [] })
    adminApiMock.getSubscriptionTemplates.mockResolvedValue({ data: [] })
    adminApiMock.getSubscriptionProtocols.mockResolvedValue({ data: [] })
    adminApiMock.getAvailableProtocols.mockResolvedValue({ data: [] })
    adminApiMock.previewSubscription.mockResolvedValue({ data: { content: '' } })
    adminApiMock.createSubscriptionGroup.mockResolvedValue({})
    adminApiMock.updateSubscriptionGroup.mockResolvedValue({})
    adminApiMock.deleteSubscriptionGroup.mockResolvedValue({})
    adminApiMock.createSubscriptionTemplate.mockResolvedValue({})
    adminApiMock.updateSubscriptionTemplate.mockResolvedValue({})
    adminApiMock.deleteSubscriptionTemplate.mockResolvedValue({})
    getSubscriptionStatsMock.mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders subscription stats from legacy and panel envelope payloads', async () => {
    getSubscriptionStatsMock
      .mockResolvedValueOnce({
        data: [{
          enabled_users: 10,
          group_id: 1,
          group_name: 'Default',
          online_nodes: 1,
          plan_count: 4,
          protocol_count: 2,
          template_count: 3,
          total_traffic: 2147483648,
          user_count: 12
        }]
      })
      .mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [{
          enabled_users: 18,
          group_id: 2,
          group_name: 'VIP',
          online_nodes: 2,
          plan_count: 6,
          protocol_count: 4,
          template_count: 5,
          total_traffic: 3221225472,
          user_count: 21
        }],
        ts: 1783526400000
      })

    const wrapper = mount(Subscriptions)
    await flushPromises()

    let metricValues = wrapper.findAll('.overview-stats .stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['0', '12', '3', '2.00 GB'])
    expect(wrapper.text()).toContain('Default')

    await wrapper.vm.loadStats()
    await flushPromises()

    metricValues = wrapper.findAll('.overview-stats .stat-card .stat-value').map(node => node.text())
    expect(metricValues).toEqual(['0', '21', '5', '3.00 GB'])
    expect(wrapper.text()).toContain('VIP')

    wrapper.unmount()
  })

  it('loads subscription resources from legacy and panel envelope payloads', async () => {
    adminApiMock.getSubscriptionGroups.mockResolvedValueOnce({
      data: [{ id: 1, name: 'Legacy Group', enable: 1 }]
    })
    adminApiMock.getSubscriptionTemplates.mockResolvedValueOnce({
      data: [{ id: 11, group_id: 1, name: 'Legacy Template', type: 'vless', enable: 1 }]
    })
    adminApiMock.getSubscriptionProtocols.mockResolvedValueOnce({
      data: [{ id: 21, name: 'Legacy Protocol', type: 'vless' }]
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()

    expect(wrapper.vm.groups[0].name).toBe('Legacy Group')
    expect(wrapper.vm.templates[0].name).toBe('Legacy Template')
    expect(wrapper.vm.protocols[0].name).toBe('Legacy Protocol')

    adminApiMock.getSubscriptionGroups.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ id: 2, name: 'Envelope Group', enable: 1 }],
      ts: 1783526400000
    })
    adminApiMock.getSubscriptionTemplates.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ id: 12, group_id: 2, name: 'Envelope Template', type: 'vless', enable: 1 }],
      ts: 1783526400000
    })
    adminApiMock.getSubscriptionProtocols.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: [{ id: 22, name: 'Envelope Protocol', type: 'vless' }],
      ts: 1783526400000
    })

    wrapper.vm.selectedGroup = null
    await wrapper.vm.loadGroups()
    await flushPromises()

    expect(wrapper.vm.groups[0].name).toBe('Envelope Group')
    expect(wrapper.vm.templates[0].name).toBe('Envelope Template')
    expect(wrapper.vm.protocols[0].name).toBe('Envelope Protocol')

    adminApiMock.getAvailableProtocols.mockResolvedValueOnce({
      data: {
        data: [{ id: 31, name: 'Nested Available Protocol', type: 'vless' }]
      }
    })

    await wrapper.vm.openManageProtocolsModal()
    await flushPromises()

    expect(wrapper.vm.availableProtocols[0].name).toBe('Nested Available Protocol')
    expect(wrapper.vm.showManageProtocolsModal).toBe(true)

    adminApiMock.previewSubscription.mockResolvedValueOnce({
      code: 0,
      msg: '操作成功',
      data: { content: 'enveloped-preview-content' },
      ts: 1783526400000
    })

    await wrapper.vm.loadPreview()
    await flushPromises()

    expect(wrapper.vm.previewContent).toBe('enveloped-preview-content')

    wrapper.unmount()
  })

  it('shows an error when subscription group list returns code -1', async () => {
    adminApiMock.getSubscriptionGroups.mockResolvedValueOnce({
      code: -1,
      msg: 'group list rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()

    expect(wrapper.vm.groups).toEqual([])
    expect(wrapper.vm.toastType).toBe('error')
    expect(wrapper.vm.toastMessage).toBe('Failed to load subscription data')

    wrapper.unmount()
  })

  it('does not close group modal or refresh groups when enveloped save fails', async () => {
    adminApiMock.createSubscriptionGroup.mockResolvedValueOnce({
      code: -1,
      msg: 'group save rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()
    adminApiMock.getSubscriptionGroups.mockClear()
    wrapper.vm.showCreateGroupModal = true
    Object.assign(wrapper.vm.groupForm, {
      id: null,
      name: 'Rejected Group',
      description: '',
      priority: 0,
      enable: true
    })

    await wrapper.vm.saveGroup()
    await flushPromises()

    expect(wrapper.vm.toastType).toBe('error')
    expect(wrapper.vm.toastMessage).toBe('Save failed')
    expect(wrapper.vm.showCreateGroupModal).toBe(true)
    expect(adminApiMock.getSubscriptionGroups).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('does not refresh groups when enveloped delete fails', async () => {
    vi.spyOn(window, 'confirm').mockImplementation(() => true)
    adminApiMock.deleteSubscriptionGroup.mockResolvedValueOnce({
      code: -1,
      msg: 'group delete rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()
    adminApiMock.getSubscriptionGroups.mockClear()

    await wrapper.vm.deleteGroup({ id: 10, name: 'Rejected Group' })
    await flushPromises()

    expect(wrapper.vm.toastType).toBe('error')
    expect(wrapper.vm.toastMessage).toBe('Delete failed')
    expect(adminApiMock.getSubscriptionGroups).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('shows an error when subscription template list returns code -1', async () => {
    adminApiMock.getSubscriptionGroups.mockResolvedValueOnce({
      data: [{ id: 1, name: 'Group', enable: 1 }]
    })
    adminApiMock.getSubscriptionTemplates.mockResolvedValueOnce({
      code: -1,
      msg: 'template list rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()

    expect(wrapper.vm.templates).toEqual([])
    expect(wrapper.vm.toastType).toBe('error')
    expect(wrapper.vm.toastMessage).toBe('Failed to load subscription data')

    wrapper.unmount()
  })

  it('does not close template modal or refresh templates when enveloped save fails', async () => {
    adminApiMock.createSubscriptionTemplate.mockResolvedValueOnce({
      code: -1,
      msg: 'template save rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()
    adminApiMock.getSubscriptionTemplates.mockClear()
    wrapper.vm.selectedGroup = { id: 1, name: 'Group' }
    wrapper.vm.showCreateTemplateModal = true
    Object.assign(wrapper.vm.templateForm, {
      id: null,
      name: 'Rejected Template',
      type: 'vless',
      server: 'example.com',
      port: 443,
      server_name: '',
      tls: 1,
      tls_fingerprint: 'chrome',
      transport: 'tcp',
      reality_public_key: '',
      reality_short_id: '',
      enable: true
    })

    await wrapper.vm.saveTemplate()
    await flushPromises()

    expect(wrapper.vm.toastType).toBe('error')
    expect(wrapper.vm.toastMessage).toBe('Save failed')
    expect(wrapper.vm.showCreateTemplateModal).toBe(true)
    expect(adminApiMock.getSubscriptionTemplates).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('does not refresh templates when enveloped template delete fails', async () => {
    vi.spyOn(window, 'confirm').mockImplementation(() => true)
    adminApiMock.deleteSubscriptionTemplate.mockResolvedValueOnce({
      code: -1,
      msg: 'template delete rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()
    adminApiMock.getSubscriptionTemplates.mockClear()
    wrapper.vm.selectedGroup = { id: 1, name: 'Group' }

    await wrapper.vm.deleteTemplate({ id: 10, name: 'Rejected Template' })
    await flushPromises()

    expect(wrapper.vm.toastType).toBe('error')
    expect(wrapper.vm.toastMessage).toBe('Delete failed')
    expect(adminApiMock.getSubscriptionTemplates).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('does not refresh templates when enveloped template toggle fails', async () => {
    adminApiMock.updateSubscriptionTemplate.mockResolvedValueOnce({
      code: -1,
      msg: 'template update rejected',
      data: null,
      ts: 1783526400000
    })

    const wrapper = mount(Subscriptions)
    await flushPromises()
    adminApiMock.getSubscriptionTemplates.mockClear()
    wrapper.vm.selectedGroup = { id: 1, name: 'Group' }

    await wrapper.vm.toggleTemplate({ id: 10, name: 'Rejected Template', enable: 1 })
    await flushPromises()

    expect(wrapper.vm.toastType).toBe('error')
    expect(wrapper.vm.toastMessage).toBe('Update failed')
    expect(adminApiMock.getSubscriptionTemplates).not.toHaveBeenCalled()

    wrapper.unmount()
  })
})
