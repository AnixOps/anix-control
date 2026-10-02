import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RouteModes from '@/views/admin/RouteModes.vue'
import UiSelect from '@/ui/UiSelect.vue'
import { setLocale } from '@/i18n'
import { inBody, toastMessages } from './helpers/feedback'

const kernelApi = vi.hoisted(() => ({
  getKernelRouteModes: vi.fn(),
  getKernelRouteModeRevisions: vi.fn(),
  setKernelRouteModes: vi.fn(),
  rollbackKernelRouteModes: vi.fn(),
}))

const router = vi.hoisted(() => ({ push: vi.fn() }))

vi.mock('@/api/kernel', () => kernelApi)
vi.mock('vue-router', () => ({ useRouter: () => router }))

function routeModes(canSwitch = true) {
  return {
    can_switch: canSwitch,
    packages: [
      {
        package_id: 'legacy-tickets',
        installation_id: 4,
        version: '2.1.0',
        enabled: true,
        config_revision: 7,
        routes: [
          {
            route_id: 'tickets.list',
            method: 'GET',
            path: '/api/v2/user/tickets',
            transport: 'http',
            catalog: 'native-flagged',
            configured: 'shadow',
            effective: 'legacy',
            allowed_modes: ['legacy', 'shadow', 'native'],
            locked: '',
            locked_reason: '',
            host: { mode: 'shadow', effective: 'shadow', native_total: 0, native_errors: 0, shadow_total: 120, shadow_mismatch: 3, shadow_errors: 1, shadow_skipped: 0 },
          },
          {
            route_id: 'auth.login',
            method: 'POST',
            path: '/api/v2/login',
            transport: 'http',
            catalog: 'kernel-owned',
            configured: 'legacy',
            effective: 'legacy',
            allowed_modes: [],
            locked: 'identity_group_a',
            locked_reason: 'Identity group A must use the identity cutover',
          },
        ],
      },
      { package_id: 'legacy-shop', installation_id: 5, version: '1.0.0', enabled: false, config_revision: 2, routes: [] },
    ],
  }
}

const revisions = [
  {
    id: 1, group_id: 'g1', package_id: 'legacy-tickets', route_id: 'tickets.list', action: 'set',
    from_mode: 'legacy', to_mode: 'shadow', actor_user_id: 1, actor: 'root@example.com', reason: 'canary', config_revision: 7,
    created_at: '2026-10-01T08:00:00Z',
  },
]

const mounted = []

async function mountPage() {
  const wrapper = mount(RouteModes, { attachTo: document.body })
  mounted.push(wrapper)
  await flushPromises()
  return wrapper
}

function selectBy(wrapper, attribute, value) {
  return wrapper.findAllComponents(UiSelect).find(select => select.vm.$attrs[attribute] === value)
}

function body(selector) {
  const found = inBody(selector)
  if (!found.exists()) throw new Error(`Unable to find ${selector} in document.body`)
  return found
}

beforeEach(async () => {
  vi.resetAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  await setLocale('en')
  kernelApi.getKernelRouteModes.mockResolvedValue(routeModes())
  kernelApi.getKernelRouteModeRevisions.mockResolvedValue({ revisions })
  kernelApi.setKernelRouteModes.mockResolvedValue({ group_id: 'g2', package_id: 'legacy-tickets', mode: 'native', config_revision: 8, changes: [{ route_id: 'tickets.list', from: 'shadow', to: 'native' }], skipped: [] })
  kernelApi.rollbackKernelRouteModes.mockResolvedValue({ group_id: 'g3', package_id: 'legacy-tickets', mode: 'legacy', config_revision: 9, changes: [{ route_id: 'tickets.list', from: 'shadow', to: 'legacy' }], skipped: [{ route_id: 'auth.login', reason: 'already legacy' }] })
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
  vi.restoreAllMocks()
})

describe('Route modes page', () => {
  it('renders the packages, their routes and the revision history', async () => {
    const wrapper = await mountPage()

    expect(kernelApi.getKernelRouteModeRevisions).toHaveBeenCalledWith('legacy-tickets', 100)
    const table = wrapper.get('[data-testid="route-modes-table"]')
    expect(table.text()).toContain('tickets.list')
    expect(table.text()).toContain('/api/v2/user/tickets')
    expect(table.text()).toContain('Native-flagged')
    expect(table.text()).toContain('Kernel-owned')
    // effective differs from configured, and the host reports another mode
    expect(wrapper.get('[data-route-effective="tickets.list"]').text()).toContain('Differs from configured')
    expect(wrapper.get('[data-route-effective="tickets.list"] [data-host-drift]').text()).toContain('Host: Shadow')
    expect(table.text()).toContain('120')

    const packageSelect = selectBy(wrapper, 'data-testid', 'route-modes-package')
    expect(packageSelect.props('options').map(option => option.value)).toEqual(['legacy-tickets', 'legacy-shop'])
    expect(packageSelect.props('modelValue')).toBe('legacy-tickets')

    // locked rows cannot be switched and say why
    expect(selectBy(wrapper, 'data-route-mode', 'auth.login').props('disabled')).toBe(true)
    expect(wrapper.get('[data-route-locked="auth.login"]').text()).toContain('identity cutover')
    expect(selectBy(wrapper, 'data-route-mode', 'tickets.list').props('disabled')).toBe(false)

    const history = wrapper.get('[data-testid="route-mode-revisions"]')
    expect(history.text()).toContain('root@example.com')
    expect(history.text()).toContain('canary')
    expect(history.text()).toContain('Legacy → Shadow')
    expect(wrapper.find('[data-testid="route-modes-readonly"]').exists()).toBe(false)
  })

  it('requires a reason before switching a route to native and sends confirm: true', async () => {
    const wrapper = await mountPage()

    selectBy(wrapper, 'data-route-mode', 'tickets.list').vm.$emit('update:modelValue', 'native')
    await flushPromises()

    const submit = body('[data-testid="route-mode-submit"]')
    expect(body('[data-testid="route-mode-dialog"]').text()).toContain('Switch tickets.list to Native?')
    expect(submit.attributes('disabled')).toBeDefined()

    await body('[data-testid="route-mode-reason"]').setValue('   ')
    expect(body('[data-testid="route-mode-submit"]').attributes('disabled')).toBeDefined()
    await body('[data-testid="route-mode-reason"]').setValue('shadow clean for a week')
    expect(body('[data-testid="route-mode-submit"]').attributes('disabled')).toBeUndefined()

    await body('[data-testid="route-mode-submit"]').trigger('click')
    await flushPromises()

    expect(kernelApi.setKernelRouteModes).toHaveBeenCalledWith({
      package_id: 'legacy-tickets',
      routes: ['tickets.list'],
      mode: 'native',
      reason: 'shadow clean for a week',
      confirm: true,
    })
    expect(toastMessages('success')[0]).toContain('1 routes changed')
    expect(kernelApi.getKernelRouteModes).toHaveBeenCalledTimes(2)
    expect(inBody('[data-testid="route-mode-dialog"]').exists()).toBe(false)
  })

  it('switches the whole package to shadow without a reason or confirmation flag', async () => {
    const wrapper = await mountPage()

    await wrapper.get('[data-testid="route-modes-set-package"]').trigger('click')
    await flushPromises()
    expect(body('[data-testid="route-mode-submit"]').attributes('disabled')).toBeUndefined()
    await body('[data-testid="route-mode-submit"]').trigger('click')
    await flushPromises()

    expect(kernelApi.setKernelRouteModes).toHaveBeenCalledWith({ package_id: 'legacy-tickets', mode: 'shadow' })
  })

  it('shows the server message when a switch is rejected', async () => {
    kernelApi.setKernelRouteModes.mockRejectedValueOnce({
      response: { status: 400, data: { error: { code: 'route_mode_rejected', message: 'route is not native-flagged' } } },
    })
    const wrapper = await mountPage()

    await wrapper.get('[data-testid="route-modes-set-package"]').trigger('click')
    await flushPromises()
    await body('[data-testid="route-mode-submit"]').trigger('click')
    await flushPromises()

    expect(body('[data-testid="route-mode-error"]').text()).toBe('route is not native-flagged')
    expect(inBody('[data-testid="route-mode-dialog"]').exists()).toBe(true)
  })

  it('rolls the package back to legacy after a confirmation', async () => {
    const wrapper = await mountPage()

    await wrapper.get('[data-testid="route-modes-rollback"]').trigger('click')
    await flushPromises()
    const dialog = body('[data-testid="route-mode-dialog"]')
    expect(dialog.text()).toContain('Roll legacy-tickets back to legacy?')
    expect(kernelApi.rollbackKernelRouteModes).not.toHaveBeenCalled()

    await body('[data-testid="route-mode-reason"]').setValue('mismatches')
    await body('[data-testid="route-mode-submit"]').trigger('click')
    await flushPromises()

    expect(kernelApi.rollbackKernelRouteModes).toHaveBeenCalledWith('legacy-tickets', 'mismatches')
    expect(toastMessages('success')[0]).toContain('1 skipped')
  })

  it('disables every switch for admins who are not super admins', async () => {
    kernelApi.getKernelRouteModes.mockResolvedValue(routeModes(false))
    const wrapper = await mountPage()

    expect(wrapper.get('[data-testid="route-modes-readonly"]').text()).toContain('Only super admins can switch route modes.')
    expect(wrapper.get('[data-testid="route-modes-set-package"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="route-modes-rollback"]').attributes('disabled')).toBeDefined()
    expect(selectBy(wrapper, 'data-testid', 'route-modes-package-mode').props('disabled')).toBe(true)
    expect(selectBy(wrapper, 'data-route-mode', 'tickets.list').props('disabled')).toBe(true)

    selectBy(wrapper, 'data-route-mode', 'tickets.list').vm.$emit('update:modelValue', 'native')
    await flushPromises()
    expect(inBody('[data-testid="route-mode-dialog"]').exists()).toBe(false)
  })
})
