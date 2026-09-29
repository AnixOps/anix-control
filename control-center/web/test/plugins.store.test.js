import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const apiMock = vi.hoisted(() => ({
  list: vi.fn(),
  releases: vi.fn(),
  installations: vi.fn(),
  extensions: vi.fn(),
  upsertInstallation: vi.fn(),
  action: vi.fn(),
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  operations: vi.fn()
}))

vi.mock('@/api', () => ({ kernelPluginsApi: apiMock }))

import { usePluginsStore } from '@/stores/plugins'

describe('plugins store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(apiMock).forEach((mock) => mock.mockReset())
    apiMock.list.mockResolvedValue([{ id: 'machine-telemetry', name: 'Machine Telemetry', official: true }])
    apiMock.releases.mockResolvedValue([{ id: 1, plugin_id: 'machine-telemetry', version: '1.0.0' }])
    apiMock.installations.mockResolvedValue([{
      id: 7,
      plugin_id: 'machine-telemetry',
      target: 'control',
      desired_version: '1.0.0',
      state: 'disabled',
      enabled: false,
      config_revision: 2
    }])
    apiMock.extensions.mockResolvedValue([])
    apiMock.operations.mockResolvedValue([])
  })

  it('loads catalog, releases and installations together', async () => {
    const store = usePluginsStore()
    await expect(store.refresh()).resolves.toBe(true)
    expect(store.plugins).toHaveLength(1)
    expect(store.installation('machine-telemetry')).toMatchObject({ id: 7 })
    expect(store.releasesFor('machine-telemetry')).toHaveLength(1)
  })

  it('retains recent operation rows from Control history', async () => {
    apiMock.operations.mockResolvedValue([
      { id: 'op-2', kind: 'plugin.enable', state: 'succeeded', node_id: null, target_version: '1.0.0' },
      { id: 'op-1', kind: 'plugin.install', state: 'failed', node_id: 12, target_version: '1.0.0' }
    ])
    const store = usePluginsStore()

    await expect(store.refresh()).resolves.toBe(true)

    expect(store.operations).toHaveLength(2)
    expect(store.recentOperations).toEqual([
      expect.objectContaining({ id: 'op-2', state: 'succeeded' }),
      expect.objectContaining({ id: 'op-1', state: 'failed' })
    ])
  })

  it('sends an idempotency key for lifecycle actions and rotates it after success', async () => {
    apiMock.action.mockResolvedValue({ operation: { id: 'op-1', state: 'pending' } })
    apiMock.operations.mockResolvedValue([{ id: 'op-1', state: 'succeeded' }])
    const store = usePluginsStore()
    await store.refresh()
    await store.runAction(store.installation('machine-telemetry'), 'enable')
    expect(apiMock.action).toHaveBeenCalledWith(7, 'enable', expect.objectContaining({
      idempotencyKey: expect.stringMatching(/^control-center:enable:7:/)
    }))
    expect(store.lastOperation).toMatchObject({ id: 'op-1', state: 'succeeded' })

    await store.runAction(store.installation('machine-telemetry'), 'enable')
    expect(apiMock.action.mock.calls[1][2].idempotencyKey).not.toBe(apiMock.action.mock.calls[0][2].idempotencyKey)
  })

  it('reuses a lifecycle key after a transient failure so a retry can replay safely', async () => {
    apiMock.action
      .mockRejectedValueOnce({ request: {}, message: 'network timeout' })
      .mockResolvedValueOnce({ operation: { id: 'op-replayed', state: 'succeeded' } })
    const store = usePluginsStore()
    await store.refresh()
    await expect(store.runAction(store.installation('machine-telemetry'), 'enable')).rejects.toBeTruthy()
    await store.runAction(store.installation('machine-telemetry'), 'enable')
    expect(apiMock.action.mock.calls[1][2].idempotencyKey).toBe(apiMock.action.mock.calls[0][2].idempotencyKey)
  })

  it('keeps server error summaries available to the UI', async () => {
    apiMock.list.mockRejectedValue({ response: { data: { error: { message: 'trust root required' } } } })
    const store = usePluginsStore()
    await expect(store.refresh()).resolves.toBe(false)
    expect(store.error).toBe('trust root required')
  })

  it('ignores a catalog response from a disconnected Control session', async () => {
    let completeCatalog
    apiMock.list.mockImplementation(() => new Promise((resolve) => { completeCatalog = resolve }))
    const store = usePluginsStore()

    const pending = store.refresh()
    store.clearState()
    completeCatalog([{ id: 'old-session-plugin' }])

    await expect(pending).resolves.toBe(false)
    expect(store.plugins).toEqual([])
    expect(store.loading).toBe(false)
  })

  it('ignores a lifecycle response from a disconnected Control session', async () => {
    let completeAction
    apiMock.action.mockImplementation(() => new Promise((resolve) => { completeAction = resolve }))
    const store = usePluginsStore()
    await store.refresh()

    const pending = store.runAction(store.installation('machine-telemetry'), 'enable')
    store.clearState()
    completeAction({ operation: { id: 'old-session-operation', state: 'pending' } })

    await pending
    expect(store.lastOperation).toBeNull()
    expect(store.actionLoading).toBe(false)
  })

  it('refreshes a pending operation until the server reports a terminal state', async () => {
    apiMock.action.mockResolvedValue({ operation: { id: 'op-pending', state: 'pending' } })
    apiMock.operations
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([{ id: 'op-pending', state: 'running' }])
      .mockResolvedValueOnce([{ id: 'op-pending', state: 'succeeded' }])
    const store = usePluginsStore()
    await store.refresh()
    await store.runAction(store.installation('machine-telemetry'), 'enable')
    expect(store.lastOperation).toMatchObject({ id: 'op-pending', state: 'succeeded' })
    expect(apiMock.operations).toHaveBeenCalledTimes(3)
  })
})
