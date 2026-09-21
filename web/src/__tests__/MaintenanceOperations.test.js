import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Operations from '@/views/maintenance/Operations.vue'
const api = vi.hoisted(() => Object.fromEntries(['getMaintenanceCatalog', 'getMaintenanceRole', 'getMaintenanceSettings', 'saveMaintenanceSettings', 'verifyMaintenanceChannel', 'confirmMaintenanceChannel', 'getMaintenanceTickets', 'getMaintenanceTicket', 'claimMaintenanceTicket', 'addMaintenanceNote', 'closeMaintenanceTicket', 'getMaintenanceChanges', 'createMaintenanceChange', 'approveMaintenanceChange', 'executeMaintenanceChange'].map(key => [key, vi.fn()])))
vi.mock('@/api/maintenance', () => api)
const ticket = { id: 7, title: '机器监控连续健康检查失败', node_id: 3, plugin_id: 'machine-telemetry', instance_id: 'machine-telemetry', plugin_version: '1.0.0', status: 'open', severity: 'normal', first_failed_at: '2026-09-20T01:00:00Z', claimed_by: 0 }
const settings = { id: 1, revision: 1, enabled: false, owner_id: 1, technician_ids: [2], contacts: [{ user_id: 1, email: 'owner@example.test', telegram_chat_id: '1' }] }
const options = { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } }
beforeEach(() => {
 vi.resetAllMocks()
 api.getMaintenanceRole.mockResolvedValue({ role: 'technician', user_id: 2 })
 api.getMaintenanceSettings.mockResolvedValue(structuredClone(settings))
 api.getMaintenanceTickets.mockResolvedValue([structuredClone(ticket)])
 api.getMaintenanceChanges.mockResolvedValue([])
 api.getMaintenanceCatalog.mockResolvedValue({ nodes: [{ id: 3, name: '测试节点', active_version: '1.0.0', desired_version: '1.0.0', restore_versions: ['1.0.0'] }], releases: [{ version: '1.0.0' }, { version: '2.0.0' }], next_node_id: 0 })
 api.getMaintenanceTicket.mockResolvedValue({ ticket: structuredClone(ticket), events: [{ event_id: 'a', error_code: 'HEALTH_CHECK_FAILED', status: 'open', occurred_at: '2026-09-20T01:02:00Z', consecutive_failures: 3, redacted_summary: '<script>secret payload</script>' }], records: [], deliveries: [] })
})
async function mounted() { const wrapper = mount(Operations, options); await flushPromises(); return wrapper }
function button(wrapper, text) { return wrapper.findAll('button').find(node => node.text() === text) }
describe('Maintenance operations workbench', () => {
 it('lets technicians claim and write records but blocks closing before recovery', async () => {
  const wrapper = await mounted(); await wrapper.find('.ticket-row').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('HEALTH_CHECK_FAILED'); expect(wrapper.text()).not.toContain('secret payload'); expect(wrapper.find('script').exists()).toBe(false)
  await button(wrapper, '认领工单').trigger('click'); await flushPromises(); expect(api.claimMaintenanceTicket).toHaveBeenCalledWith(7)
  await wrapper.find('textarea').setValue('已检查运行日志'); expect(button(wrapper, '填写记录并关闭').attributes('disabled')).toBeDefined()
  await wrapper.find('.detail form').trigger('submit'); await flushPromises(); expect(api.addMaintenanceNote).toHaveBeenCalledWith(7, '已检查运行日志'); wrapper.unmount()
 })
 it('creates a repair for the node current version even when the incident version is stale', async () => {
  api.getMaintenanceCatalog.mockResolvedValue({ nodes: [{ id: 3, name: '测试节点', active_version: '2.0.0', desired_version: '2.0.0', restore_versions: ['1.0.0'] }], releases: [{ version: '1.0.0' }, { version: '2.0.0' }] })
  const wrapper = await mounted(); await wrapper.find('.ticket-row').trigger('click'); await flushPromises()
  await button(wrapper, '重启此节点插件').trigger('click'); await flushPromises(); await wrapper.find('.change-form').trigger('submit'); await flushPromises()
  expect(api.createMaintenanceChange).toHaveBeenCalledWith(expect.objectContaining({ node_ids: [3], target_version: '2.0.0', kind: 'restart', plugin_id: 'machine-telemetry', config: {} }))
  expect(api.executeMaintenanceChange).not.toHaveBeenCalled(); wrapper.unmount()
 })
 it('binds owner approval to the reviewed request and only executes approved changes', async () => {
  api.getMaintenanceRole.mockResolvedValue({ role: 'owner' })
  const change = { id: 11, kind: 'update', target_version: '2.0.0', node_ids_json: '[3]', binding_hash: 'b'.repeat(64), status: 'pending' }
  api.getMaintenanceChanges.mockResolvedValue([{ change, config: { interval: 30 }, audit: [] }])
  const wrapper = await mounted(); await button(wrapper, '插件操作与审批').trigger('click'); expect(button(wrapper, '执行已批准操作')).toBeUndefined()
  api.getMaintenanceChanges.mockResolvedValue([{ change: { ...change, status: 'approved' }, config: { interval: 30 }, audit: [] }])
  await button(wrapper, '批准此具体操作').trigger('click'); await flushPromises(); expect(api.approveMaintenanceChange).toHaveBeenCalledWith(11, 'b'.repeat(64))
  await button(wrapper, '执行已批准操作').trigger('click'); await flushPromises(); expect(api.executeMaintenanceChange).toHaveBeenCalledWith(11); wrapper.unmount()
 })
 it('supports owner bootstrap without incident or change access', async () => {
  api.getMaintenanceRole.mockResolvedValue({ role: 'bootstrap' }); api.getMaintenanceSettings.mockResolvedValue({ id: 1, owner_id: 0, technician_ids: [], contacts: [], enabled: false, revision: 0 })
  const wrapper = await mounted(); expect(wrapper.text()).toContain('负责人用户 ID'); expect(api.getMaintenanceTickets).not.toHaveBeenCalled(); expect(api.getMaintenanceChanges).not.toHaveBeenCalled(); expect(wrapper.find('fieldset').attributes('disabled')).toBeUndefined(); wrapper.unmount()
 })
 it('keeps email and Telegram challenges independent', async () => {
  api.getMaintenanceRole.mockResolvedValue({ role: 'owner' }); const wrapper = await mounted(); await button(wrapper, '运维设置').trigger('click')
  const email = wrapper.findAll('.verification')[0]; await email.findAll('button')[0].trigger('click'); await flushPromises(); expect(api.verifyMaintenanceChannel).toHaveBeenCalledWith('email'); expect(api.confirmMaintenanceChannel).not.toHaveBeenCalled()
  await email.find('input').setValue('123456'); await email.findAll('button')[1].trigger('click'); await flushPromises(); expect(api.confirmMaintenanceChannel).toHaveBeenCalledWith('email', '123456'); wrapper.unmount()
 })
 it('surfaces API denial without mutation', async () => {
  api.getMaintenanceRole.mockRejectedValue({ response: { data: { error: 'maintenance_access_denied' } } }); const wrapper = await mounted(); expect(wrapper.find('[role="alert"]').text()).toContain('maintenance_access_denied'); expect(api.createMaintenanceChange).not.toHaveBeenCalled(); wrapper.unmount()
 })
})
