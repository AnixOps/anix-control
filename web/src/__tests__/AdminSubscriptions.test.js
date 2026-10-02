// 订阅分组 (UI U7): the group list, one group's page with its sections in
// the URL, and the section components (node templates, node protocols,
// subscription output). Endpoints and payloads unchanged.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { answerConfirms, toastMessages } from './helpers/feedback'
import UiHost from '@/ui/UiHost.vue'
import Subscriptions from '@/views/admin/Subscriptions.vue'
import SubscriptionGroup from '@/views/admin/SubscriptionGroup.vue'
import GroupOutput from '@/views/admin/subscriptions/GroupOutput.vue'
import GroupProtocols from '@/views/admin/subscriptions/GroupProtocols.vue'
import GroupTemplates from '@/views/admin/subscriptions/GroupTemplates.vue'
import { useUserStore } from '@/stores/user'
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
  updateGroupProtocols: vi.fn(),
  updateSubscriptionTemplate: vi.fn()
}))

const getSubscriptionStatsMock = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  default: adminApiMock,
  getSubscriptionStats: (...args) => getSubscriptionStatsMock(...args)
}))

enableAutoUnmount(afterEach)

const GROUP = { id: 1, name: 'Group', enable: 1, priority: 1 }

function makeRouter() {
  const page = { template: '<div />' }
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/subscriptions', component: Subscriptions },
      { path: '/admin/subscriptions/:id(\\d+)/:section?', component: SubscriptionGroup },
      { path: '/admin/nodes', component: page },
      { path: '/admin/plans', component: page },
      { path: '/admin/users', component: page }
    ]
  })
}

async function mountRoute(path) {
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount({ template: '<router-view />' }, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

function mountSection(component, props = { group: GROUP }) {
  return mount(component, { props, global: { plugins: [makeRouter()] } })
}

describe('Admin subscription groups', () => {
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
    adminApiMock.updateGroupProtocols.mockResolvedValue({})
    getSubscriptionStatsMock.mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  describe('group list', () => {
    it('merges the stats into the rows from legacy and panel envelope payloads', async () => {
      adminApiMock.getSubscriptionGroups.mockResolvedValueOnce({ data: [{ id: 1, name: 'Default', enable: 1 }] })
      getSubscriptionStatsMock.mockResolvedValueOnce({
        data: [{ enabled_users: 10, group_id: 1, group_name: 'Default', online_nodes: 1, plan_count: 4, protocol_count: 2, template_count: 3, total_traffic: 2147483648, user_count: 12 }]
      })
      const { wrapper } = await mountRoute('/admin/subscriptions')
      const list = wrapper.findComponent(Subscriptions)

      let metrics = wrapper.findAll('.overview-stats .stat-value').map(node => node.text())
      expect(metrics).toEqual(['1', '12', '3', '2.00 GB'])
      expect(list.vm.rows[0]).toMatchObject({ name: 'Default', user_count: 12, template_count: 3, online_nodes: 1 })

      adminApiMock.getSubscriptionGroups.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: [{ id: 2, name: 'VIP', enable: 1 }], ts: 1783526400000 })
      getSubscriptionStatsMock.mockResolvedValueOnce({
        code: 0,
        msg: '操作成功',
        data: [{ enabled_users: 18, group_id: 2, group_name: 'VIP', online_nodes: 2, plan_count: 6, protocol_count: 4, template_count: 5, total_traffic: 3221225472, user_count: 21 }],
        ts: 1783526400000
      })
      await list.vm.loadAll()
      await flushPromises()

      metrics = wrapper.findAll('.overview-stats .stat-value').map(node => node.text())
      expect(metrics).toEqual(['1', '21', '5', '3.00 GB'])
      expect(wrapper.text()).toContain('VIP')
    })

    it('shows the error state when the group list returns code -1', async () => {
      adminApiMock.getSubscriptionGroups.mockResolvedValueOnce({ code: -1, msg: 'groups rejected', data: null })
      const { wrapper } = await mountRoute('/admin/subscriptions')
      const list = wrapper.findComponent(Subscriptions)
      expect(list.vm.groups).toEqual([])
      expect(list.vm.loadError).toBe('groups rejected')
      expect(wrapper.text()).toContain('groups rejected')
    })

    it('opens a group page from its row', async () => {
      adminApiMock.getSubscriptionGroups.mockResolvedValue({ data: [{ id: 3, name: 'Asia', enable: 1 }] })
      const { wrapper, router } = await mountRoute('/admin/subscriptions')
      await wrapper.find('tbody tr').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.path).toBe('/admin/subscriptions/3')
      expect(wrapper.find('h1').text()).toBe('Asia')
    })
  })

  describe('group page', () => {
    it('keeps the section in the URL and loads the group from the list', async () => {
      adminApiMock.getSubscriptionGroups.mockResolvedValue({ data: [{ id: 3, name: 'Asia', enable: 1, priority: 2 }] })
      getSubscriptionStatsMock.mockResolvedValue({ data: [{ group_id: 3, user_count: 7, enabled_users: 5, plan_count: 2 }] })
      const { wrapper, router } = await mountRoute('/admin/subscriptions/3/members')
      expect(wrapper.find('h1').text()).toBe('Asia')
      expect(wrapper.get('[role="tab"][aria-selected="true"]').text()).toBe('Members')
      expect(wrapper.text()).toContain('7')

      await wrapper.findAll('[role="tab"]').find(tab => tab.text() === 'Node templates').trigger('mousedown', { button: 0 })
      await flushPromises()
      expect(router.currentRoute.value.path).toBe('/admin/subscriptions/3/templates')
      expect(adminApiMock.getSubscriptionTemplates).toHaveBeenCalledWith(3)
    })

    it('says when the group does not exist', async () => {
      adminApiMock.getSubscriptionGroups.mockResolvedValue({ data: [{ id: 3, name: 'Asia', enable: 1 }] })
      const { wrapper } = await mountRoute('/admin/subscriptions/9')
      expect(wrapper.text()).toContain('This group doesn’t exist')
    })

    it('sends an unknown section to the overview', async () => {
      adminApiMock.getSubscriptionGroups.mockResolvedValue({ data: [{ id: 3, name: 'Asia', enable: 1 }] })
      const { router } = await mountRoute('/admin/subscriptions/3/nope')
      await flushPromises()
      expect(router.currentRoute.value.path).toBe('/admin/subscriptions/3')
    })
  })

  describe('node templates', () => {
    it('loads templates from legacy and panel envelope payloads', async () => {
      adminApiMock.getSubscriptionTemplates.mockResolvedValueOnce({ data: [{ id: 11, group_id: 1, name: 'Legacy Template', type: 'vless', enable: 1 }] })
      const wrapper = mountSection(GroupTemplates)
      await flushPromises()
      expect(wrapper.vm.templates[0].name).toBe('Legacy Template')

      adminApiMock.getSubscriptionTemplates.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: [{ id: 12, group_id: 1, name: 'Envelope Template', type: 'vless', enable: 1 }], ts: 1783526400000 })
      await wrapper.vm.loadTemplates()
      await flushPromises()
      expect(wrapper.vm.templates[0].name).toBe('Envelope Template')
    })

    it('shows the error state when the template list returns code -1', async () => {
      adminApiMock.getSubscriptionTemplates.mockResolvedValueOnce({ code: -1, msg: 'templates rejected', data: null })
      const wrapper = mountSection(GroupTemplates)
      await flushPromises()
      expect(wrapper.vm.templates).toEqual([])
      expect(wrapper.vm.templatesError).toBe('templates rejected')
    })

    it('validates the template form before saving', async () => {
      const wrapper = mountSection(GroupTemplates)
      await flushPromises()
      wrapper.vm.openTemplateDialog()
      await wrapper.vm.saveTemplate()
      expect(adminApiMock.createSubscriptionTemplate).not.toHaveBeenCalled()
      expect(wrapper.vm.errors).toMatchObject({ name: 'Enter a node name', server: 'Enter the server address' })
    })

    it('keeps the template dialog open and does not refresh when the save fails', async () => {
      adminApiMock.createSubscriptionTemplate.mockResolvedValueOnce({ code: -1, msg: 'save rejected', data: null })
      const wrapper = mountSection(GroupTemplates)
      await flushPromises()
      adminApiMock.getSubscriptionTemplates.mockClear()
      wrapper.vm.openTemplateDialog()
      Object.assign(wrapper.vm.templateForm, { name: 'Edge', type: 'vless', server: 'edge.example.com', port: 443, tls: 1, transport: 'ws' })
      await wrapper.vm.saveTemplate()
      await flushPromises()

      expect(adminApiMock.createSubscriptionTemplate).toHaveBeenCalledWith(1, expect.objectContaining({
        name: 'Edge',
        server: 'edge.example.com',
        enable: 1,
        transport_settings: JSON.stringify({ path: '/ws', host: 'edge.example.com' }),
        protocol_settings: JSON.stringify({ flow: 'xtls-rprx-vision' })
      }))
      expect(wrapper.vm.templateFormError).toBe('Couldn’t save')
      expect(wrapper.vm.templateDialogOpen).toBe(true)
      expect(adminApiMock.getSubscriptionTemplates).not.toHaveBeenCalled()
    })

    it('does not refresh when the template delete fails', async () => {
      adminApiMock.deleteSubscriptionTemplate.mockResolvedValueOnce({ code: -1, msg: 'delete rejected', data: null })
      const wrapper = mountSection(GroupTemplates)
      await flushPromises()
      adminApiMock.getSubscriptionTemplates.mockClear()
      const confirms = answerConfirms(true)
      await wrapper.vm.deleteTemplate({ id: 10, name: 'Rejected Template' })
      await flushPromises()
      expect(confirms.last()).toMatchObject({ tone: 'danger', title: 'Delete node template Rejected Template?' })
      expect(confirms.errors.map(error => error.message)).toEqual(['delete rejected'])
      expect(adminApiMock.getSubscriptionTemplates).not.toHaveBeenCalled()
    })

    it('does not refresh when the template toggle fails', async () => {
      adminApiMock.updateSubscriptionTemplate.mockResolvedValueOnce({ code: -1, msg: 'toggle rejected', data: null })
      const wrapper = mountSection(GroupTemplates)
      await flushPromises()
      adminApiMock.getSubscriptionTemplates.mockClear()
      await wrapper.vm.toggleTemplate({ id: 10, name: 'Rejected Template', enable: 1 })
      await flushPromises()
      expect(adminApiMock.updateSubscriptionTemplate).toHaveBeenCalledWith(10, { enable: 0 })
      expect(toastMessages('error')).toEqual(['Couldn’t update'])
      expect(adminApiMock.getSubscriptionTemplates).not.toHaveBeenCalled()
    })
  })

  describe('node protocols', () => {
    it('loads linked protocols and the nested available list', async () => {
      adminApiMock.getSubscriptionProtocols.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: [{ id: 22, name: 'Envelope Protocol', type: 'vless' }], ts: 1783526400000 })
      adminApiMock.getAvailableProtocols.mockResolvedValueOnce({ data: { data: [{ id: 31, name: 'Nested Available Protocol', type: 'vless' }] } })
      const wrapper = mountSection(GroupProtocols)
      await flushPromises()
      expect(wrapper.vm.protocols[0].name).toBe('Envelope Protocol')

      await wrapper.vm.openManageProtocolsModal()
      await flushPromises()
      expect(wrapper.vm.availableProtocols[0].name).toBe('Nested Available Protocol')
      expect(wrapper.vm.selectedProtocolIds).toEqual([22])
      expect(wrapper.vm.showManageProtocolsModal).toBe(true)
    })

    it('shows an error when the available protocols return code -1', async () => {
      adminApiMock.getAvailableProtocols.mockResolvedValueOnce({ code: -1, msg: 'available rejected', data: null })
      const wrapper = mountSection(GroupProtocols)
      await flushPromises()
      await wrapper.vm.openManageProtocolsModal()
      await flushPromises()
      expect(wrapper.vm.availableProtocols).toEqual([])
      expect(toastMessages('error')).toEqual(['Couldn’t load the node protocols that can be linked'])
    })

    it('keeps the protocol dialog open when the save fails', async () => {
      adminApiMock.updateGroupProtocols.mockResolvedValueOnce({ code: -1, msg: 'links rejected', data: null })
      const wrapper = mountSection(GroupProtocols)
      await flushPromises()
      adminApiMock.getSubscriptionProtocols.mockClear()
      wrapper.vm.showManageProtocolsModal = true
      wrapper.vm.selectedProtocolIds = [10]
      await wrapper.vm.saveGroupProtocols()
      await flushPromises()
      expect(adminApiMock.updateGroupProtocols).toHaveBeenCalledWith(1, [10])
      expect(wrapper.vm.protocolsError).toBe('Couldn’t update the linked node protocols')
      expect(wrapper.vm.showManageProtocolsModal).toBe(true)
      expect(adminApiMock.getSubscriptionProtocols).not.toHaveBeenCalled()
    })
  })

  describe('subscription output', () => {
    it('renders the server preview in a code block', async () => {
      adminApiMock.previewSubscription.mockResolvedValueOnce({ code: 0, msg: '操作成功', data: { content: 'enveloped-preview-content' }, ts: 1783526400000 })
      const wrapper = mountSection(GroupOutput)
      await flushPromises()
      expect(adminApiMock.previewSubscription).toHaveBeenCalledWith({ group_ids: [1], format: 'v2ray' })
      expect(wrapper.vm.previewContent).toBe('enveloped-preview-content')
      expect(wrapper.get('[data-test="subscription-preview"] pre').text()).toBe('enveloped-preview-content')
    })

    it('shows the error state when the preview fails', async () => {
      adminApiMock.previewSubscription.mockResolvedValueOnce({ code: -1, msg: 'preview rejected', data: null, ts: 1783526400000 })
      const wrapper = mountSection(GroupOutput)
      await flushPromises()
      expect(wrapper.vm.previewError).toBe('preview rejected')
      expect(wrapper.find('[data-test="subscription-preview"]').exists()).toBe(false)
      expect(wrapper.text()).toContain('preview rejected')
    })
  })

  describe('dialogs and feedback', () => {
    async function renderPage() {
      adminApiMock.getSubscriptionGroups.mockResolvedValue({ data: [{ id: 3, name: 'Asia', enable: 1, priority: 1 }] })
      const router = makeRouter()
      await router.push('/admin/subscriptions')
      await router.isReady()
      render({ components: { UiHost }, template: '<div><router-view /><UiHost /></div>' }, { global: { plugins: [router] } })
      await screen.findByText('Asia')
      return router
    }

    async function rowAction(user, item) {
      await user.click(screen.getByRole('button', { name: 'Actions for Asia' }))
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: item }))
    }

    it('creates a group in a dialog that closes with Esc and returns focus', async () => {
      const user = userEvent.setup()
      adminApiMock.createSubscriptionGroup.mockResolvedValueOnce({ code: -1, msg: 'dup' }).mockResolvedValueOnce({ code: 0 })
      await renderPage()
      const opener = screen.getByRole('button', { name: 'New group' })
      await user.click(opener)
      let dialog = await screen.findByRole('dialog', { name: 'New subscription group' })
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))

      await user.click(opener)
      dialog = await screen.findByRole('dialog')
      await user.click(within(dialog).getByRole('button', { name: 'Save' }))
      expect(within(dialog).getByText('Enter a group name')).toBeTruthy()
      expect(adminApiMock.createSubscriptionGroup).not.toHaveBeenCalled()

      await user.type(within(dialog).getByLabelText(/Group name/), 'Europe')
      await user.click(within(dialog).getByRole('button', { name: 'Save' }))
      expect((await within(dialog).findByRole('alert')).textContent).toBe('Couldn’t save')
      await user.click(within(dialog).getByRole('button', { name: 'Save' }))
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      expect(adminApiMock.createSubscriptionGroup).toHaveBeenLastCalledWith({ name: 'Europe', description: '', priority: 0, enable: 1 })
      expect(toastMessages('success')).toEqual(['Subscription group saved'])
    })

    it('asks before deleting a group; Cancel keeps it', async () => {
      const user = userEvent.setup()
      await renderPage()
      await rowAction(user, 'Delete group…')
      const confirm = await screen.findByRole('alertdialog', { name: 'Delete subscription group Asia?' })
      await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(adminApiMock.deleteSubscriptionGroup).not.toHaveBeenCalled()

      await rowAction(user, 'Delete group…')
      await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Delete group' }))
      await waitFor(() => expect(adminApiMock.deleteSubscriptionGroup).toHaveBeenCalledWith(3))
      await waitFor(() => expect(toastMessages('success')).toEqual(['Subscription group deleted']))
    })

    it('lists the subscription links as copy fields, signed with the admin token', async () => {
      const user = userEvent.setup()
      useUserStore().userInfo = { id: 1, token: 'admin-sub-token' }
      await renderPage()
      await rowAction(user, 'Subscription links')
      const dialog = await screen.findByRole('dialog', { name: 'Subscription links of Asia' })
      expect(within(dialog).getAllByRole('button', { name: /Copy/ })).toHaveLength(11)
      expect(within(dialog).getByLabelText('Clash (YAML)').value).toBe(`${window.location.origin}/s/admin-sub-token?type=clash&groups=3`)
    })
  })
})
