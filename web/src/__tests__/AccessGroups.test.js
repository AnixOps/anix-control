import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccessGroups from '@/views/admin/AccessGroups.vue'

const kernelApi = vi.hoisted(() => ({
  addKernelAccessGroupPlan: vi.fn(),
  addKernelAccessGroupUser: vi.fn(),
  createKernelAccessGroup: vi.fn(),
  createKernelResourceGrant: vi.fn(),
  deleteKernelAccessGroup: vi.fn(),
  deleteKernelQuotaPolicy: vi.fn(),
  deleteKernelResourceGrant: vi.fn(),
  getKernelAccessGroupDetail: vi.fn(),
  getKernelAccessGroups: vi.fn(),
  getKernelScopes: vi.fn(),
  removeKernelAccessGroupPlan: vi.fn(),
  removeKernelAccessGroupUser: vi.fn(),
  resolveKernelAccess: vi.fn(),
  updateKernelAccessGroup: vi.fn(),
  upsertKernelQuotaPolicy: vi.fn(),
}))

vi.mock('@/api/kernel', () => kernelApi)

const group = {
  id: 7,
  scope_id: 'forward',
  name: 'Canary operators',
  description: 'Dedicated canary access',
  enabled: true,
}

function detailFixture() {
  return {
    group: { ...group },
    users: [{ id: 11, email: 'operator@example.com' }],
    plans: [{ id: 4, name: 'Canary plan' }],
    resource_grants: [{ id: 9, group_id: 7, resource_type: 'plugin_api', resource_id: 'machine-telemetry', permissions: '["machine-telemetry.api"]' }],
    quota_policies: [{ id: 12, group_id: 7, key: 'machine-telemetry.rate', policy: '{"requests_per_minute":60}' }],
  }
}

function mountAccessGroups() {
  return mount(AccessGroups)
}

function formFor(wrapper, selector) {
  return wrapper.findAll('form').find(form => form.find(selector).exists())
}

describe('AccessGroups.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(globalThis, 'confirm').mockReturnValue(true)
    kernelApi.getKernelScopes.mockResolvedValue([
      { id: 'forward', name: 'Forward', description: 'Forwarding access' },
      { id: 'proxy', name: 'Proxy', description: 'Proxy access' },
    ])
    kernelApi.getKernelAccessGroups.mockResolvedValue([{ ...group }])
    kernelApi.getKernelAccessGroupDetail.mockResolvedValue(detailFixture())
    kernelApi.createKernelAccessGroup.mockResolvedValue({ ...group, id: 8, name: 'New group' })
    kernelApi.addKernelAccessGroupUser.mockResolvedValue({ id: 20, group_id: 7, user_id: 42 })
    kernelApi.addKernelAccessGroupPlan.mockResolvedValue({ id: 21, group_id: 7, plan_id: 5 })
    kernelApi.createKernelResourceGrant.mockResolvedValue({ id: 22 })
    kernelApi.upsertKernelQuotaPolicy.mockResolvedValue({ id: 23 })
    kernelApi.resolveKernelAccess.mockResolvedValue({ scope_id: 'forward', groups: [{ ...group }], grants: [{ id: 9 }], quotas: [{ id: 12 }] })
  })

  it('loads a scoped group and shows its member, plan, grant, and quota detail from the authoritative detail endpoint', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()

    expect(kernelApi.getKernelScopes).toHaveBeenCalledTimes(1)
    expect(kernelApi.getKernelAccessGroups).toHaveBeenCalledWith(undefined)
    expect(kernelApi.getKernelAccessGroupDetail).toHaveBeenCalledWith(7)
    expect(wrapper.find('.access-groups-table').text()).toContain('Canary operators')
    expect(wrapper.text()).toContain('operator@example.com')
    expect(wrapper.text()).toContain('Canary plan')
    expect(wrapper.text()).toContain('machine-telemetry.api')
    expect(wrapper.text()).toContain('requests_per_minute')
    wrapper.unmount()
  })

  it('creates a group and preserves its scope as a server-owned identity', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()

    await wrapper.find('.page-header .btn-primary').trigger('click')
    await wrapper.get('#access-group-scope').setValue('proxy')
    await wrapper.get('#access-group-name').setValue('Proxy viewers')
    await wrapper.get('#access-group-description').setValue('Scoped proxy permission')
    await wrapper.get('[aria-labelledby="access-group-editor-title"] form').trigger('submit')
    await flushPromises()

    expect(kernelApi.createKernelAccessGroup).toHaveBeenCalledWith({
      scope_id: 'proxy', name: 'Proxy viewers', description: 'Scoped proxy permission', enabled: true,
    })
    wrapper.unmount()
  })

  it('submits user and plan memberships plus JSON policies through the Kernel API and refreshes the selected group', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()

    await wrapper.get('#access-member-id').setValue('42')
    await formFor(wrapper, '#access-member-id').trigger('submit')
    await flushPromises()
    expect(kernelApi.addKernelAccessGroupUser).toHaveBeenCalledWith(7, 42)

    await wrapper.get('#access-plan-id').setValue('5')
    await formFor(wrapper, '#access-plan-id').trigger('submit')
    await flushPromises()
    expect(kernelApi.addKernelAccessGroupPlan).toHaveBeenCalledWith(7, 5)

    await wrapper.get('#access-grant-id').setValue('machine-telemetry')
    await wrapper.get('#access-grant-permissions').setValue('["machine-telemetry.api"]')
    await formFor(wrapper, '#access-grant-id').trigger('submit')
    await flushPromises()
    expect(kernelApi.createKernelResourceGrant).toHaveBeenCalledWith({
      group_id: 7, resource_type: 'plugin_api', resource_id: 'machine-telemetry', permissions: '["machine-telemetry.api"]',
    })

    await wrapper.get('#access-quota-key').setValue('machine-telemetry.rate')
    await wrapper.get('#access-quota-policy').setValue('{"requests_per_minute":120}')
    await formFor(wrapper, '#access-quota-key').trigger('submit')
    await flushPromises()
    expect(kernelApi.upsertKernelQuotaPolicy).toHaveBeenCalledWith({
      group_id: 7, key: 'machine-telemetry.rate', policy: '{"requests_per_minute":120}',
    })
    expect(kernelApi.getKernelAccessGroupDetail.mock.calls.length).toBeGreaterThan(1)
    wrapper.unmount()
  })

  it('uses the server-side allow-union resolver instead of recomputing permissions in the browser', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()

    await wrapper.get('#access-resolve-user').setValue('11')
    await wrapper.get('#access-resolve-plan').setValue('4')
    await wrapper.get('#access-resolve-scope').setValue('forward')
    await formFor(wrapper, '#access-resolve-user').trigger('submit')
    await flushPromises()

    expect(kernelApi.resolveKernelAccess).toHaveBeenCalledWith({ userID: 11, planID: 4, scopeID: 'forward' })
    expect(wrapper.find('.resolver-result').text()).toContain('Canary operators')
    wrapper.unmount()
  })

  it('shows the kernel error without replacing a previously loaded policy view', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()
    kernelApi.addKernelAccessGroupUser.mockRejectedValueOnce({ response: { data: { error: { message: 'user does not exist' } } } })

    await wrapper.get('#access-member-id').setValue('999')
    await formFor(wrapper, '#access-member-id').trigger('submit')
    await flushPromises()

    expect(wrapper.get('.error-message').text()).toBe('user does not exist')
    expect(wrapper.text()).toContain('operator@example.com')
    wrapper.unmount()
  })
})
