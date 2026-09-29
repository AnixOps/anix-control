import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const mockConnect = vi.fn()
const mockDisconnect = vi.fn()
const mockOn = vi.fn()
const mockOff = vi.fn()
const mockSubscribe = vi.fn()
const mockConnected = { value: false }

vi.mock('@/api', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  }
}))

vi.mock('@/composables/useSSE', () => ({
  useSSE: () => ({
    connected: mockConnected,
    connect: mockConnect,
    disconnect: mockDisconnect,
    on: mockOn,
    off: mockOff,
    subscribe: mockSubscribe,
  }),
  SSEEventTypes: {
    NODE_UPDATE: 'node_update',
    TASK_UPDATE: 'task_update',
    LOG: 'log',
  },
  SSEChannels: {
    NODES: 'nodes',
    TASKS: 'tasks',
    LOGS: 'logs',
  },
}))

describe('Nodes Store realtime', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockConnect.mockReset()
    mockDisconnect.mockReset()
    mockOn.mockReset()
    mockOff.mockReset()
    mockSubscribe.mockReset()
    mockSubscribe.mockResolvedValue(true)
    mockConnected.value = false
    vi.resetModules()
  })

  it('connects SSE and subscribes once for nodes updates', async () => {
    const { useNodesStore } = await import('@/stores/nodes')
    const store = useNodesStore()

    await store.subscribeToUpdates('token-1')

    expect(mockConnect).toHaveBeenCalledWith('/api/v1/sse', 'token-1')
    expect(mockOff).toHaveBeenCalledWith('node_update', expect.any(Function))
    expect(mockOn).toHaveBeenCalledWith('node_update', expect.any(Function))
    expect(mockSubscribe).toHaveBeenCalledWith('nodes', 'token-1', '/api/v1')
  })

  it('skips duplicate subscribe when already connected with same token', async () => {
    const { useNodesStore } = await import('@/stores/nodes')
    const store = useNodesStore()

    mockConnected.value = false
    await store.subscribeToUpdates('token-1')

    mockConnect.mockClear()
    mockOff.mockClear()
    mockOn.mockClear()
    mockSubscribe.mockClear()
    mockConnected.value = true

    await store.subscribeToUpdates('token-1')

    expect(mockConnect).not.toHaveBeenCalled()
    expect(mockOn).not.toHaveBeenCalled()
    expect(mockSubscribe).not.toHaveBeenCalled()
  })

  it('updates node state from SSE payload', async () => {
    const { useNodesStore } = await import('@/stores/nodes')
    const store = useNodesStore()

    store.nodes = [
      { id: 1, name: 'node-a', status: 'offline', host: '10.0.0.1' },
    ]

    store.updateNodeFromSSE({ node_id: 1, status: 'online', users: 12 })

    expect(store.nodes[0]).toEqual({
      id: 1,
      name: 'node-a',
      status: 'online',
      host: '10.0.0.1',
      users: 12,
      node_id: 1,
    })
  })

  it('removes node handlers on unsubscribe without dropping shared connection', async () => {
    const { useNodesStore } = await import('@/stores/nodes')
    const store = useNodesStore()

    store.unsubscribeFromUpdates()

    expect(mockOff).toHaveBeenCalledWith('node_update', expect.any(Function))
    expect(mockDisconnect).not.toHaveBeenCalled()
  })
})

describe('Tasks Store realtime', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockConnect.mockReset()
    mockDisconnect.mockReset()
    mockOn.mockReset()
    mockOff.mockReset()
    mockSubscribe.mockReset()
    mockSubscribe.mockResolvedValue(true)
    mockConnected.value = false
    vi.resetModules()
  })

  it('connects SSE and subscribes to task and log channels', async () => {
    const { useTasksStore } = await import('@/stores/tasks')
    const store = useTasksStore()

    await store.subscribeToUpdates('token-2')

    expect(mockConnect).toHaveBeenCalledWith('/api/v1/sse', 'token-2')
    expect(mockOff).toHaveBeenCalledWith('task_update', expect.any(Function))
    expect(mockOff).toHaveBeenCalledWith('log', expect.any(Function))
    expect(mockOn).toHaveBeenCalledWith('task_update', expect.any(Function))
    expect(mockOn).toHaveBeenCalledWith('log', expect.any(Function))
    expect(mockSubscribe).toHaveBeenNthCalledWith(1, 'tasks', 'token-2', '/api/v1')
    expect(mockSubscribe).toHaveBeenNthCalledWith(2, 'logs', 'token-2', '/api/v1')
  })

  it('updates current task and list item from SSE payload', async () => {
    const { useTasksStore } = await import('@/stores/tasks')
    const store = useTasksStore()

    store.tasks = [{ id: 7, status: 'pending', name: 'deploy' }]
    store.currentTask = { id: 7, status: 'pending', name: 'deploy' }

    store.updateTaskFromSSE({ task_id: 7, status: 'running', progress: 50 })

    expect(store.tasks[0]).toEqual({
      id: 7,
      status: 'running',
      name: 'deploy',
      task_id: 7,
      progress: 50,
    })
    expect(store.currentTask).toEqual({
      id: 7,
      status: 'running',
      name: 'deploy',
      task_id: 7,
      progress: 50,
    })
  })

  it('appends logs only for selected task', async () => {
    const { useTasksStore } = await import('@/stores/tasks')
    const store = useTasksStore()

    store.currentTask = { id: 9 }
    store.addLogFromSSE({ task_id: 9, message: 'step 1' })
    store.addLogFromSSE({ task_id: 10, message: 'ignore me' })

    expect(store.taskLogs).toEqual([{ task_id: 9, message: 'step 1' }])
  })

  it('removes task handlers on unsubscribe without dropping shared connection', async () => {
    const { useTasksStore } = await import('@/stores/tasks')
    const store = useTasksStore()

    store.unsubscribeFromUpdates()

    expect(mockOff).toHaveBeenCalledWith('task_update', expect.any(Function))
    expect(mockOff).toHaveBeenCalledWith('log', expect.any(Function))
    expect(mockDisconnect).not.toHaveBeenCalled()
  })
})
