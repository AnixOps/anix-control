import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/api'
import { useSSE, SSEEventTypes, SSEChannels } from '@/composables/useSSE'

const SSE_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1'

export const useTasksStore = defineStore('tasks', () => {
  const tasks = ref([])
  const currentTask = ref(null)
  const taskLogs = ref([])
  const loading = ref(false)
  const error = ref(null)
  const statusFilter = ref('')

  // Computed
  const filteredTasks = computed(() => {
    if (!statusFilter.value || statusFilter.value === 'all') {
      return tasks.value
    }
    return tasks.value.filter(t => t.status === statusFilter.value)
  })

  const pendingCount = computed(() =>
    tasks.value.filter(t => t.status === 'pending').length
  )

  const runningCount = computed(() =>
    tasks.value.filter(t => t.status === 'running').length
  )

  const completedCount = computed(() =>
    tasks.value.filter(t => t.status === 'completed').length
  )

  const failedCount = computed(() =>
    tasks.value.filter(t => t.status === 'failed').length
  )

  // SSE connection
  const {
    connected: sseConnected,
    connect: connectSSE,
    disconnect: disconnectSSE,
    on: onSSE,
    off: offSSE,
    subscribe
  } = useSSE()

  const taskUpdateHandler = (data) => {
    updateTaskFromSSE(data)
  }

  const logHandler = (data) => {
    addLogFromSSE(data)
  }

  let subscribedToken = null

  // Fetch all tasks
  async function fetchTasks(params = {}) {
    loading.value = true
    error.value = null

    try {
      const response = await api.get('/tasks', { params })
      if (response.data?.success) {
        tasks.value = response.data.data || []
      } else {
        tasks.value = response.data?.data || []
      }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch tasks'
    } finally {
      loading.value = false
    }
  }

  // Fetch single task
  async function fetchTask(id) {
    loading.value = true
    error.value = null

    try {
      const response = await api.get(`/tasks/${id}`)
      if (response.data?.success) {
        currentTask.value = response.data.data
      }
      return currentTask.value
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch task'
      return null
    } finally {
      loading.value = false
    }
  }

  // Create task (run playbook)
  async function createTask(taskData) {
    loading.value = true
    error.value = null

    try {
      const response = await api.post('/tasks', taskData)
      if (response.data?.success) {
        const newTask = response.data.data
        tasks.value.unshift(newTask)
        return { success: true, data: newTask }
      }
      return { success: false, error: response.data?.error || 'Failed to create task' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to create task'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Cancel task
  async function cancelTask(id) {
    try {
      const response = await api.post(`/tasks/${id}/cancel`)
      if (response.data?.success) {
        const index = tasks.value.findIndex(t => t.id === id)
        if (index > -1) {
          tasks.value[index].status = 'cancelled'
        }
        return { success: true }
      }
      return { success: false, error: response.data?.error }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  // Retry task
  async function retryTask(id) {
    try {
      const response = await api.post(`/tasks/${id}/retry`)
      if (response.data?.success) {
        const newTask = response.data.data
        tasks.value.unshift(newTask)
        return { success: true, data: newTask }
      }
      return { success: false, error: response.data?.error }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  // Fetch task logs
  async function fetchTaskLogs(id, params = {}) {
    try {
      const response = await api.get(`/tasks/${id}/logs`, { params })
      if (response.data?.success) {
        taskLogs.value = response.data.data || []
      }
      return taskLogs.value
    } catch (e) {
      return []
    }
  }

  // Update task from SSE event
  function updateTaskFromSSE(data) {
    const index = tasks.value.findIndex(t => t.id === data.task_id)
    if (index > -1) {
      tasks.value[index] = { ...tasks.value[index], ...data }
    }

    if (currentTask.value?.id === data.task_id) {
      currentTask.value = { ...currentTask.value, ...data }
    }
  }

  // Add log from SSE
  function addLogFromSSE(data) {
    if (currentTask.value?.id === data.task_id) {
      taskLogs.value.push(data)
    }
  }

  // Subscribe to real-time updates
  async function subscribeToUpdates(token) {
    if (!token) return

    if (subscribedToken === token && sseConnected.value) {
      return
    }

    connectSSE(`${SSE_BASE_URL}/sse`, token)

    offSSE(SSEEventTypes.TASK_UPDATE, taskUpdateHandler)
    offSSE(SSEEventTypes.LOG, logHandler)
    onSSE(SSEEventTypes.TASK_UPDATE, taskUpdateHandler)
    onSSE(SSEEventTypes.LOG, logHandler)

    await subscribe(SSEChannels.TASKS, token, SSE_BASE_URL)
    await subscribe(SSEChannels.LOGS, token, SSE_BASE_URL)
    subscribedToken = token
  }

  function unsubscribeFromUpdates() {
    offSSE(SSEEventTypes.TASK_UPDATE, taskUpdateHandler)
    offSSE(SSEEventTypes.LOG, logHandler)
    subscribedToken = null
  }

  return {
    // State
    tasks,
    currentTask,
    taskLogs,
    loading,
    error,
    statusFilter,
    sseConnected,

    // Computed
    filteredTasks,
    pendingCount,
    runningCount,
    completedCount,
    failedCount,

    // Actions
    fetchTasks,
    fetchTask,
    createTask,
    cancelTask,
    retryTask,
    fetchTaskLogs,
    subscribeToUpdates,
    unsubscribeFromUpdates,
    updateTaskFromSSE,
    addLogFromSSE
  }
})