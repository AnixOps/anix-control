// 诊断 on the node page (5.0, D7): a section before 日志, in the URL as
// ?section=diagnostics, asking for the node as proxy-<id>.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRouter } from 'vue-router'
import NodeDetail from '@/views/admin/NodeDetail.vue'
import UiHost from '@/ui/UiHost.vue'
import { setLocale } from '@/i18n'

const api = vi.hoisted(() => ({
  getNode: vi.fn(),
  getNodes: vi.fn()
}))
vi.mock('@/api/admin', () => api)

const telemetry = vi.hoisted(() => ({ nodeHasServicesTable: vi.fn() }))
vi.mock('@/api/machineTelemetry', async importOriginal => ({ ...(await importOriginal()), ...telemetry }))

const kernelApi = vi.hoisted(() => ({
  getKernelAgentTransports: vi.fn(),
  getKernelNodeDiagnostics: vi.fn()
}))
vi.mock('@/api/kernel', async importOriginal => ({ ...(await importOriginal()), ...kernelApi }))

const Harness = {
  components: { NodeDetail, UiHost },
  template: '<div><NodeDetail /><UiHost /></div>'
}

async function renderPage(path) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/nodes', component: { template: '<div>list</div>' } },
      { path: '/admin/nodes/:id', component: Harness }
    ]
  })
  await router.push(path)
  await router.isReady()
  const result = render({ template: '<router-view />' }, { global: { plugins: [router] } })
  return { ...result, router }
}

describe('NodeDetail diagnostics section', () => {
  beforeEach(async () => {
    for (const fn of [...Object.values(api), ...Object.values(telemetry), ...Object.values(kernelApi)]) fn.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    api.getNode.mockResolvedValue({ code: 0, data: { id: 5, name: 'hk-01', host: 'hk.example.test', status: 1, protocols: [] } })
    telemetry.nodeHasServicesTable.mockResolvedValue(false)
    kernelApi.getKernelAgentTransports.mockResolvedValue({ nodes: [] })
    kernelApi.getKernelNodeDiagnostics.mockResolvedValue({
      node: 'proxy-5', connected: false, diagnostics: false, forward_checks: false, actions: [], services: ['gost'], can_restart: true, history: true, tasks: []
    })
    await setLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('puts 诊断 before 日志 and loads it only when it opens', async () => {
    const { router } = await renderPage('/admin/nodes/5')
    await screen.findByRole('heading', { level: 1, name: 'hk-01' })
    const tabs = screen.getAllByRole('tab').map(tab => tab.textContent.trim())
    expect(tabs.indexOf('Diagnostics')).toBeGreaterThan(-1)
    expect(tabs.indexOf('Diagnostics')).toBe(tabs.indexOf('Logs') - 1)
    expect(kernelApi.getKernelNodeDiagnostics).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('tab', { name: 'Diagnostics' }))
    expect(await screen.findByTestId('diagnostics-offline')).toBeTruthy()
    expect(kernelApi.getKernelNodeDiagnostics).toHaveBeenCalledWith('proxy-5')
    expect(router.currentRoute.value.query.section).toBe('diagnostics')
  })

  it('opens from ?section=diagnostics', async () => {
    await renderPage('/admin/nodes/5?section=diagnostics')
    expect(await screen.findByTestId('node-diagnostics')).toBeTruthy()
    expect(screen.getByRole('tab', { name: 'Diagnostics', selected: true })).toBeTruthy()
  })
})
