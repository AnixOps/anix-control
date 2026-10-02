import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiHost from '@/ui/UiHost.vue'
import { runAction } from '@/ui/composables/useToast'
import { inBody, toastMessages, toasts } from './helpers/feedback'
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
  return mount(AccessGroups, { attachTo: document.body })
}

// The group opens in a sheet (UI U6), which renders in document.body.
async function openGroup(wrapper, id = 7) {
  await wrapper.vm.openDetail(id)
  await flushPromises()
}

function formFor(selector) {
  return inBody(selector).element.closest('form')
}

async function submit(selector) {
  formFor(selector).dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await flushPromises()
}

enableAutoUnmount(afterEach)

describe('AccessGroups.vue', () => {
  beforeEach(() => {
    vi.resetAllMocks()
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
    expect(wrapper.find('.access-groups-table').text()).toContain('Canary operators')
    // The detail loads when the group opens.
    expect(kernelApi.getKernelAccessGroupDetail).not.toHaveBeenCalled()
    await openGroup(wrapper)
    expect(kernelApi.getKernelAccessGroupDetail).toHaveBeenCalledWith(7)
    const sheet = inBody('[data-test="access-detail-sheet"]').text()
    expect(sheet).toContain('operator@example.com')
    expect(sheet).toContain('Canary plan')
    expect(sheet).toContain('machine-telemetry.api')
    expect(sheet).toContain('requests_per_minute')
    wrapper.unmount()
  })

  it('creates a group and preserves its scope as a server-owned identity', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()

    await wrapper.find('[data-test="access-new-group"]').trigger('click')
    await flushPromises()
    wrapper.vm.groupEditor.scopeID = 'proxy'
    await inBody('#access-group-name').setValue('Proxy viewers')
    await inBody('#access-group-description').setValue('Scoped proxy permission')
    await inBody('[data-test="access-group-editor"]').trigger('submit')
    await flushPromises()

    expect(kernelApi.createKernelAccessGroup).toHaveBeenCalledWith({
      scope_id: 'proxy', name: 'Proxy viewers', description: 'Scoped proxy permission', enabled: true,
    })
    wrapper.unmount()
  })

  it('submits user and plan memberships plus JSON policies through the Kernel API and refreshes the selected group', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()

    await openGroup(wrapper)
    await inBody('#access-member-id').setValue('42')
    await submit('#access-member-id')
    expect(kernelApi.addKernelAccessGroupUser).toHaveBeenCalledWith(7, 42)

    await inBody('#access-plan-id').setValue('5')
    await submit('#access-plan-id')
    expect(kernelApi.addKernelAccessGroupPlan).toHaveBeenCalledWith(7, 5)

    await inBody('#access-grant-id').setValue('machine-telemetry')
    await inBody('#access-grant-permissions').setValue('["machine-telemetry.api"]')
    await submit('#access-grant-id')
    expect(kernelApi.createKernelResourceGrant).toHaveBeenCalledWith({
      group_id: 7, resource_type: 'plugin_api', resource_id: 'machine-telemetry', permissions: '["machine-telemetry.api"]',
    })

    await inBody('#access-quota-key').setValue('machine-telemetry.rate')
    await inBody('#access-quota-policy').setValue('{"requests_per_minute":120}')
    await submit('#access-quota-key')
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
    // The scope defaults to the first one (forward).
    expect(wrapper.vm.resolver.scopeID).toBe('forward')
    await wrapper.get('#access-resolve-user').element.closest('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(kernelApi.resolveKernelAccess).toHaveBeenCalledWith({ userID: 11, planID: 4, scopeID: 'forward' })
    expect(wrapper.find('.resolver-result').text()).toContain('Canary operators')
    wrapper.unmount()
  })

  it('shows the kernel error without replacing a previously loaded policy view', async () => {
    const wrapper = mountAccessGroups()
    await flushPromises()
    await openGroup(wrapper)
    kernelApi.addKernelAccessGroupUser.mockRejectedValueOnce({ response: { data: { error: { message: 'user does not exist' } } } })

    await inBody('#access-member-id').setValue('999')
    await submit('#access-member-id')

    expect(inBody('.error-message').text()).toBe('user does not exist')
    expect(inBody('[data-test="access-detail-sheet"]').text()).toContain('operator@example.com')
    wrapper.unmount()
  })

  describe('dialogs and feedback', () => {
    const Harness = { components: { AccessGroups, UiHost }, template: '<div><AccessGroups /><UiHost /></div>' }

    async function renderPage() {
      const user = userEvent.setup()
      render(Harness)
      await user.click(await screen.findByText('Canary operators'))
      await screen.findByText('operator@example.com')
    }

    it('keeps editor errors inside the dialog and closes it with Esc', async () => {
      const user = userEvent.setup()
      kernelApi.createKernelAccessGroup.mockRejectedValueOnce(new Error('name taken'))
      render(Harness)
      await screen.findByText('Canary operators')
      const opener = screen.getByRole('button', { name: 'New group' })
      await user.click(opener)
      const dialog = await screen.findByRole('dialog')
      await user.type(within(dialog).getByLabelText(/name/i), 'Dup')
      await user.click(within(dialog).getByRole('button', { name: 'Save' }))
      expect((await within(dialog).findByRole('alert')).textContent).toBe('name taken')
      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
      await waitFor(() => expect(document.activeElement).toBe(opener))
    })

    it('asks before deleting a group; Cancel keeps it', async () => {
      const user = userEvent.setup()
      kernelApi.deleteKernelAccessGroup.mockResolvedValue({})
      await renderPage()
      const sheet = screen.getByRole('dialog', { name: 'Canary operators' })
      await user.click(within(sheet).getByRole('button', { name: 'Delete group' }))
      let confirm = await screen.findByRole('alertdialog', { name: 'Delete access group Canary operators?' })
      await user.click(within(confirm).getByRole('button', { name: 'Cancel' }))
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
      expect(kernelApi.deleteKernelAccessGroup).not.toHaveBeenCalled()

      await user.click(within(sheet).getByRole('button', { name: 'Delete group' }))
      confirm = await screen.findByRole('alertdialog')
      await user.click(within(confirm).getByRole('button', { name: 'Delete group' }))
      await waitFor(() => expect(kernelApi.deleteKernelAccessGroup).toHaveBeenCalledWith(7))
    })

    it('removes a member at once with undo in the toast', async () => {
      kernelApi.removeKernelAccessGroupUser.mockResolvedValue({})
      await renderPage()
      await userEvent.setup().click(screen.getByRole('button', { name: 'Remove operator@example.com' }))
      await waitFor(() => expect(kernelApi.removeKernelAccessGroupUser).toHaveBeenCalledWith(7, 11))
      expect(screen.queryByRole('alertdialog')).toBeNull()
      await waitFor(() => expect(toastMessages('success')).toEqual(['Removed user #11']))
      await runAction(toasts('success')[0].id)
      expect(kernelApi.addKernelAccessGroupUser).toHaveBeenCalledWith(7, 11)
    })

    it('shows a failed grant removal inside the confirmation', async () => {
      const user = userEvent.setup()
      kernelApi.deleteKernelResourceGrant.mockRejectedValue(new Error('grant in use'))
      await renderPage()
      await user.click(screen.getByRole('button', { name: 'Actions for plugin_api/machine-telemetry' }))
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Remove' }))
      const confirm = await screen.findByRole('alertdialog', { name: 'Remove resource grant #9?' })
      await user.click(within(confirm).getByRole('button', { name: 'Remove grant' }))
      expect((await within(confirm).findByRole('alert')).textContent).toContain('grant in use')
    })
      it('opens a group from the row with Enter and shows a list load failure with retry', async () => {
      const user = userEvent.setup()
      kernelApi.getKernelScopes.mockRejectedValueOnce(new Error('kernel offline'))
      render(Harness)
      const alert = await screen.findByRole('alert')
      expect(alert.textContent).toContain('kernel offline')
      await user.click(within(alert).getByRole('button', { name: 'Try again' }))
      const row = (await screen.findByText('Canary operators')).closest('tr')
      row.focus()
      await user.keyboard('{Enter}')
      expect(await screen.findByRole('dialog', { name: 'Canary operators' })).toBeTruthy()
      expect(kernelApi.getKernelAccessGroupDetail).toHaveBeenCalledWith(7)
    })

    it('deletes from the row menu', async () => {
      const user = userEvent.setup()
      kernelApi.deleteKernelAccessGroup.mockResolvedValue({})
      render(Harness)
      await screen.findByText('Canary operators')
      await user.click(screen.getByRole('button', { name: 'Actions for Canary operators' }))
      await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'Delete' }))
      const confirm = await screen.findByRole('alertdialog', { name: 'Delete access group Canary operators?' })
      await user.click(within(confirm).getByRole('button', { name: 'Delete group' }))
      await waitFor(() => expect(kernelApi.deleteKernelAccessGroup).toHaveBeenCalledWith(7))
      await waitFor(() => expect(toastMessages('success')).toEqual(['Deleted access group Canary operators']))
    })

    it('filters the groups by service scope on the server', async () => {
      const user = userEvent.setup()
      render(Harness)
      await screen.findByText('Canary operators')
      await user.click(screen.getByRole('combobox', { name: 'Filter by service scope' }))
      await user.click(await screen.findByRole('option', { name: 'Proxy (proxy)' }))
      await waitFor(() => expect(kernelApi.getKernelAccessGroups).toHaveBeenLastCalledWith('proxy'))
    })
  })
})
