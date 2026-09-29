import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/api'

export const usePlaybooksStore = defineStore('playbooks', () => {
  const playbooks = ref([])
  const builtInPlaybooks = ref([])
  const categories = ref([])
  const selectedPlaybook = ref(null)
  const loading = ref(false)
  const error = ref(null)
  const searchQuery = ref('')
  const categoryFilter = ref('')

  // Computed
  const filteredPlaybooks = computed(() => {
    return playbooks.value.filter(p => {
      const matchesSearch = !searchQuery.value ||
        p.name?.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
        p.description?.toLowerCase().includes(searchQuery.value.toLowerCase())
      const matchesCategory = !categoryFilter.value || p.category === categoryFilter.value
      return matchesSearch && matchesCategory
    })
  })

  // Fetch all playbooks
  async function fetchPlaybooks(params = {}) {
    loading.value = true
    error.value = null

    try {
      const response = await api.get('/playbooks', { params })
      if (response.data?.success) {
        playbooks.value = response.data.data || []
      } else {
        playbooks.value = response.data?.data || []
      }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch playbooks'
    } finally {
      loading.value = false
    }
  }

  // Fetch built-in playbooks
  async function fetchBuiltInPlaybooks() {
    loading.value = true
    error.value = null

    try {
      const response = await api.get('/playbooks/built-in')
      if (response.data?.success) {
        builtInPlaybooks.value = response.data.data || []
      }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch built-in playbooks'
    } finally {
      loading.value = false
    }
  }

  // Fetch playbook categories
  async function fetchCategories() {
    try {
      const response = await api.get('/playbooks/categories')
      if (response.data?.success) {
        categories.value = response.data.data || []
      }
    } catch (e) {
      // Ignore error
    }
  }

  // Fetch single playbook
  async function fetchPlaybook(name) {
    loading.value = true
    error.value = null

    try {
      const response = await api.get(`/playbooks/${encodeURIComponent(name)}`)
      if (response.data?.success) {
        selectedPlaybook.value = response.data.data
      }
      return selectedPlaybook.value
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch playbook'
      return null
    } finally {
      loading.value = false
    }
  }

  // Create playbook
  async function createPlaybook(playbookData) {
    loading.value = true
    error.value = null

    try {
      const response = await api.post('/playbooks', playbookData)
      if (response.data?.success) {
        const newPlaybook = response.data.data
        playbooks.value.push(newPlaybook)
        return { success: true, data: newPlaybook }
      }
      return { success: false, error: response.data?.error || 'Failed to create playbook' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to create playbook'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Delete playbook
  async function deletePlaybook(name) {
    loading.value = true
    error.value = null

    try {
      const response = await api.delete(`/playbooks/${encodeURIComponent(name)}`)
      if (response.data?.success) {
        playbooks.value = playbooks.value.filter(p => p.name !== name)
        if (selectedPlaybook.value?.name === name) {
          selectedPlaybook.value = null
        }
        return { success: true }
      }
      return { success: false, error: response.data?.error || 'Failed to delete playbook' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to delete playbook'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Sync built-in playbooks
  async function syncBuiltInPlaybooks() {
    loading.value = true
    error.value = null

    try {
      const response = await api.post('/playbooks/sync-builtin')
      return { success: response.data?.success, data: response.data?.data }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to sync built-in playbooks'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  return {
    // State
    playbooks,
    builtInPlaybooks,
    categories,
    selectedPlaybook,
    loading,
    error,
    searchQuery,
    categoryFilter,

    // Computed
    filteredPlaybooks,

    // Actions
    fetchPlaybooks,
    fetchBuiltInPlaybooks,
    fetchCategories,
    fetchPlaybook,
    createPlaybook,
    deletePlaybook,
    syncBuiltInPlaybooks
  }
})