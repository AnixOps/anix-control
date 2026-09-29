import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '@/api'

export const useSchedulesStore = defineStore('schedules', () => {
  const schedules = ref([])
  const loading = ref(false)
  const error = ref(null)

  // Computed
  const enabledCount = computed(() =>
    schedules.value.filter(s => s.enabled).length
  )

  const disabledCount = computed(() =>
    schedules.value.filter(s => !s.enabled).length
  )

  // Fetch all schedules
  async function fetchSchedules(params = {}) {
    loading.value = true
    error.value = null

    try {
      const response = await api.get('/schedules', { params })
      if (response.data?.success) {
        schedules.value = response.data.data || []
      } else {
        schedules.value = response.data?.data || []
      }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to fetch schedules'
    } finally {
      loading.value = false
    }
  }

  // Create schedule
  async function createSchedule(scheduleData) {
    loading.value = true
    error.value = null

    try {
      const response = await api.post('/schedules', scheduleData)
      if (response.data?.success) {
        const newSchedule = response.data.data
        schedules.value.push(newSchedule)
        return { success: true, data: newSchedule }
      }
      return { success: false, error: response.data?.error || 'Failed to create schedule' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to create schedule'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Update schedule
  async function updateSchedule(id, scheduleData) {
    loading.value = true
    error.value = null

    try {
      const response = await api.put(`/schedules/${id}`, scheduleData)
      if (response.data?.success) {
        const updated = response.data.data
        const index = schedules.value.findIndex(s => s.id === id)
        if (index > -1) {
          schedules.value[index] = updated
        }
        return { success: true, data: updated }
      }
      return { success: false, error: response.data?.error || 'Failed to update schedule' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to update schedule'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Delete schedule
  async function deleteSchedule(id) {
    loading.value = true
    error.value = null

    try {
      const response = await api.delete(`/schedules/${id}`)
      if (response.data?.success) {
        schedules.value = schedules.value.filter(s => s.id !== id)
        return { success: true }
      }
      return { success: false, error: response.data?.error || 'Failed to delete schedule' }
    } catch (e) {
      error.value = e.response?.data?.error || 'Failed to delete schedule'
      return { success: false, error: error.value }
    } finally {
      loading.value = false
    }
  }

  // Toggle schedule enabled status
  async function toggleSchedule(id) {
    try {
      const response = await api.post(`/schedules/${id}/toggle`)
      if (response.data?.success) {
        const updated = response.data.data
        const index = schedules.value.findIndex(s => s.id === id)
        if (index > -1) {
          schedules.value[index] = updated
        }
        return { success: true, data: updated }
      }
      return { success: false, error: response.data?.error }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  // Run schedule immediately
  async function runScheduleNow(id) {
    try {
      const response = await api.post(`/schedules/${id}/run`)
      return { success: response.data?.success, data: response.data?.data }
    } catch (e) {
      return { success: false, error: e.response?.data?.error }
    }
  }

  return {
    // State
    schedules,
    loading,
    error,

    // Computed
    enabledCount,
    disabledCount,

    // Actions
    fetchSchedules,
    createSchedule,
    updateSchedule,
    deleteSchedule,
    toggleSchedule,
    runScheduleNow
  }
})