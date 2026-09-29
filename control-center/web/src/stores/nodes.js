import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/api'
import { useSSE, SSEEventTypes, SSEChannels } from '@/composables/useSSE'

const SSE_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1'

export const useNodesStore = defineStore('nodes', () => {
  const nodes = ref([])
  const loading = ref(false)
  const error = ref(null)
  const selectedNode = ref(null)
  const searchQuery = ref('')
  const statusFilter = ref('')

  // Computed
  const filteredNodes = computed(() => {
    return nodes.value.filter(node => {
      const matchesSearch = !searchQuery.value ||
        node.name?.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
        node.host?.includes(searchQuery.value)
      const matchesStatus = !statusFilter.value || node.status === statusFilter.value
      return matchesSearch && matchesStatus
    })
  })

  const onlineCount = computed(() =>
    nodes.value.filter(n => n.status === 'online').length
  )

  const offlineCount = computed(() =>
    nodes.value.filter(n => n.status === 'offline').length
  )

  // SSE connection for real-time updates
  const {
    connected: sseConnected,
    connect: connectSSE,
    disconnect: disconnectSSE,
    on: onSSE,
    off: offSSE,
    subscribe
  } = useSSE()

  const nodeUpdateHandler = (data) => {
    updateNodeFromSSE(data)
  }

  let subscribedToken = null

  // Fetch all nodes
  async function fetchNodes(params = {}) {
    loading.value = true
    error.value = null

    try {
      const response = await api.get('/nodes', { params })
      if (response.data?.success) {
        nodes.value = response.data.data || []
      } else {
        nodes.value = response.data?.data || []
      }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch nodes'
    } finally {
      loading.value = false
    }
  }

  // Fetch single node
  async function fetchNode(id) {
    loading.value = true
    error.value = null

    try {
      const response = await api.get(`/nodes/${id}`)
      if (response.data?.success) {
        selectedNode.value = response.data.data
      }
      return selectedNode.value
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch node'
      return null
    } finally {
      loading.value = false
    }
  }

  // Create node
  async function createNode(nodeData) {
    loading.value = true
    error.value = null

    try {
      const response = await api.post('/nodes', nodeData)
      if (response.data?.success) {
        const newNode = response.data.data
        nodes.value.push(newNode)
        return { success: true, data: newNode }
      }
      return { success: false, error: response.data?.error || 'Failed to create node' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to create node'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Update node
  async function updateNode(id, nodeData) {
    loading.value = true
    error.value = null

    try {
      const response = await api.put(`/nodes/${id}`, nodeData)
      if (response.data?.success) {
        const updated = response.data.data
        const index = nodes.value.findIndex(n => n.id === id)
        if (index > -1) {
          nodes.value[index] = updated
        }
        if (selectedNode.value?.id === id) {
          selectedNode.value = updated
        }
        return { success: true, data: updated }
      }
      return { success: false, error: response.data?.error || 'Failed to update node' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to update node'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Delete node
  async function deleteNode(id) {
    loading.value = true
    error.value = null

    try {
      const response = await api.delete(`/nodes/${id}`)
      if (response.data?.success) {
        nodes.value = nodes.value.filter(n => n.id !== id)
        if (selectedNode.value?.id === id) {
          selectedNode.value = null
        }
        return { success: true }
      }
      return { success: false, error: response.data?.error || 'Failed to delete node' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to delete node'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Node actions
  async function startNode(id) {
    try {
      const response = await api.post(`/nodes/${id}/start`)
      return { success: response.data?.success, data: response.data?.data }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  async function stopNode(id) {
    try {
      const response = await api.post(`/nodes/${id}/stop`)
      return { success: response.data?.success, data: response.data?.data }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  async function restartNode(id) {
    try {
      const response = await api.post(`/nodes/${id}/restart`)
      return { success: response.data?.success, data: response.data?.data }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  async function testNodeConnection(id) {
    try {
      const response = await api.post(`/nodes/${id}/test`)
      return { success: response.data?.success, data: response.data?.data }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  // Update node from SSE event
  function updateNodeFromSSE(data) {
    const index = nodes.value.findIndex(n => n.id === data.node_id)
    if (index > -1) {
      nodes.value[index] = { ...nodes.value[index], ...data }
    }
  }

  // Subscribe to real-time updates
  async function subscribeToUpdates(token) {
    if (!token) return

    if (subscribedToken === token && sseConnected.value) {
      return
    }

    connectSSE(`${SSE_BASE_URL}/sse`, token)

    offSSE(SSEEventTypes.NODE_UPDATE, nodeUpdateHandler)
    onSSE(SSEEventTypes.NODE_UPDATE, nodeUpdateHandler)

    await subscribe(SSEChannels.NODES, token, SSE_BASE_URL)
    subscribedToken = token
  }

  function unsubscribeFromUpdates() {
    offSSE(SSEEventTypes.NODE_UPDATE, nodeUpdateHandler)
    subscribedToken = null
  }

  return {
    // State
    nodes,
    loading,
    error,
    selectedNode,
    searchQuery,
    statusFilter,
    sseConnected,

    // Computed
    filteredNodes,
    onlineCount,
    offlineCount,

    // Actions
    fetchNodes,
    fetchNode,
    createNode,
    updateNode,
    deleteNode,
    startNode,
    stopNode,
    restartNode,
    testNodeConnection,
    subscribeToUpdates,
    unsubscribeFromUpdates,
    updateNodeFromSSE
  }
})