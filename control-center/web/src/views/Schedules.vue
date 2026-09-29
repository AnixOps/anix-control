<template>
  <div class="schedules-page">
    <div class="page-header">
      <h1>Schedules</h1>
      <div class="header-actions">
        <button class="btn btn-primary" @click="showCreateDialog = true">
          <span class="icon">➕</span> New Schedule
        </button>
        <button class="btn btn-secondary" @click="schedulesStore.fetchSchedules()">
          <span class="icon">🔄</span> Refresh
        </button>
      </div>
    </div>

    <!-- Stats Cards -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon total">📅</div>
        <div class="stat-info">
          <span class="stat-value">{{ schedulesStore.schedules.length }}</span>
          <span class="stat-label">Total Schedules</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon enabled">✅</div>
        <div class="stat-info">
          <span class="stat-value">{{ schedulesStore.enabledCount }}</span>
          <span class="stat-label">Enabled</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon disabled">⏸️</div>
        <div class="stat-info">
          <span class="stat-value">{{ schedulesStore.disabledCount }}</span>
          <span class="stat-label">Disabled</span>
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="schedulesStore.loading" class="loading-container">
      <div class="spinner"></div>
      <span>Loading schedules...</span>
    </div>

    <!-- Error State -->
    <div v-else-if="schedulesStore.error" class="error-message">
      {{ schedulesStore.error }}
    </div>

    <!-- Empty State -->
    <div v-else-if="schedulesStore.schedules.length === 0" class="empty-state">
      No schedules found. Create one to get started.
    </div>

    <!-- Schedules Grid -->
    <div v-else class="schedules-grid">
      <div class="schedule-cards">
        <div v-for="schedule in schedulesStore.schedules" :key="schedule.id" class="schedule-card">
          <div class="card-header">
            <div class="schedule-name">
              <h3>{{ schedule.name }}</h3>
              <span :class="['status-badge', schedule.enabled ? 'enabled' : 'disabled']">
                {{ schedule.enabled ? 'Enabled' : 'Disabled' }}
              </span>
            </div>
            <div class="card-actions">
              <button class="btn-icon" @click="toggleSchedule(schedule)" :title="schedule.enabled ? 'Disable' : 'Enable'">
                {{ schedule.enabled ? '⏸️' : '▶️' }}
              </button>
              <button class="btn-icon" @click="runSchedule(schedule)" title="Run Now">
                🚀
              </button>
              <button class="btn-icon" @click="editSchedule(schedule)" title="Edit">
                ✏️
              </button>
              <button class="btn-icon danger" @click="deleteSchedule(schedule)" title="Delete">
                🗑️
              </button>
            </div>
          </div>
          <div class="card-body">
            <div class="info-row">
              <span class="label">Playbook:</span>
              <span class="value">{{ schedule.playbookName || schedule.playbook_name || '-' }}</span>
            </div>
            <div class="info-row">
              <span class="label">Schedule:</span>
              <span class="value">
                <code>{{ schedule.cron }}</code>
                <span class="cron-desc">{{ getCronDescription(schedule.cron) }}</span>
              </span>
            </div>
            <div class="info-row">
              <span class="label">Timezone:</span>
              <span class="value">{{ schedule.timezone }}</span>
            </div>
            <div class="info-row">
              <span class="label">Target Nodes:</span>
              <span class="value">{{ schedule.targetNodes?.length || schedule.target_nodes?.length || 0 }} nodes</span>
            </div>
            <div v-if="schedule.nextRun || schedule.next_run" class="info-row">
              <span class="label">Next Run:</span>
              <span class="value">{{ formatDate(schedule.nextRun || schedule.next_run) }}</span>
            </div>
            <div v-if="schedule.lastRun || schedule.last_run" class="info-row">
              <span class="label">Last Run:</span>
              <span class="value">{{ formatDate(schedule.lastRun || schedule.last_run) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Create/Edit Schedule Dialog -->
    <div v-if="showCreateDialog || editingSchedule" class="modal-overlay" @click.self="closeDialog">
      <div class="modal">
        <div class="modal-header">
          <h2>{{ editingSchedule ? 'Edit Schedule' : 'Create Schedule' }}</h2>
          <button class="close-btn" @click="closeDialog">✕</button>
        </div>
        <div class="modal-body">
          <form @submit.prevent="saveSchedule">
            <div class="form-group">
              <label>Name</label>
              <input v-model="formData.name" type="text" required placeholder="Schedule name" />
            </div>
            <div class="form-group">
              <label>Playbook</label>
              <select v-model="formData.playbookId" required>
                <option value="">Select playbook</option>
                <option v-for="p in playbooksStore.playbooks" :key="p.id || p.name" :value="p.id || p.name">{{ p.name }}</option>
              </select>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>Cron Expression</label>
                <input v-model="formData.cron" type="text" required placeholder="0 * * * *" />
              </div>
              <div class="form-group">
                <label>Timezone</label>
                <select v-model="formData.timezone">
                  <option>UTC</option>
                  <option>America/New_York</option>
                  <option>America/Los_Angeles</option>
                  <option>Europe/London</option>
                  <option>Asia/Shanghai</option>
                  <option>Asia/Tokyo</option>
                </select>
              </div>
            </div>
            <div class="form-group">
              <label>Target Nodes</label>
              <div class="node-selector">
                <label v-for="node in nodesStore.nodes" :key="node.id" class="checkbox-label">
                  <input type="checkbox" :value="node.id" v-model="formData.targetNodes" />
                  {{ node.name }}
                </label>
              </div>
            </div>
            <div class="form-group">
              <label class="checkbox-label">
                <input type="checkbox" v-model="formData.enabled" />
                Enabled
              </label>
            </div>
          </form>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeDialog">Cancel</button>
          <button class="btn btn-primary" @click="saveSchedule" :disabled="schedulesStore.loading">
            {{ editingSchedule ? 'Save' : 'Create' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useSchedulesStore } from '@/stores/schedules'
import { useNodesStore } from '@/stores/nodes'
import { usePlaybooksStore } from '@/stores/playbooks'

const schedulesStore = useSchedulesStore()
const nodesStore = useNodesStore()
const playbooksStore = usePlaybooksStore()

const showCreateDialog = ref(false)
const editingSchedule = ref(null)

const formData = ref({
  name: '',
  playbookId: '',
  cron: '0 * * * *',
  timezone: 'UTC',
  targetNodes: [],
  enabled: true
})

async function saveSchedule() {
  const payload = {
    name: formData.value.name,
    playbookId: formData.value.playbookId,
    cron: formData.value.cron,
    timezone: formData.value.timezone,
    targetNodes: formData.value.targetNodes,
    enabled: formData.value.enabled
  }

  let result
  if (editingSchedule.value) {
    result = await schedulesStore.updateSchedule(editingSchedule.value.id, payload)
  } else {
    result = await schedulesStore.createSchedule(payload)
  }

  if (result.success) {
    closeDialog()
  } else {
    alert(result.error || 'Failed to save schedule')
  }
}

async function toggleSchedule(schedule) {
  const result = await schedulesStore.toggleSchedule(schedule.id)
  if (!result.success) {
    alert(result.error || 'Failed to toggle schedule')
  }
}

async function runSchedule(schedule) {
  if (!confirm(`Run "${schedule.name}" now?`)) return
  const result = await schedulesStore.runScheduleNow(schedule.id)
  if (result.success) {
    alert(`Task created: ${result.data?.taskId || 'OK'}`)
  } else {
    alert(result.error || 'Failed to run schedule')
  }
}

async function deleteSchedule(schedule) {
  if (!confirm(`Delete "${schedule.name}"?`)) return
  const result = await schedulesStore.deleteSchedule(schedule.id)
  if (!result.success) {
    alert(result.error || 'Failed to delete schedule')
  }
}

function editSchedule(schedule) {
  editingSchedule.value = schedule
  formData.value = {
    name: schedule.name,
    playbookId: schedule.playbookId || schedule.playbook_id,
    cron: schedule.cron,
    timezone: schedule.timezone || 'UTC',
    targetNodes: schedule.targetNodes || schedule.target_nodes || [],
    enabled: schedule.enabled
  }
}

function closeDialog() {
  showCreateDialog.value = false
  editingSchedule.value = null
  formData.value = {
    name: '',
    playbookId: '',
    cron: '0 * * * *',
    timezone: 'UTC',
    targetNodes: [],
    enabled: true
  }
}

function formatDate(date) {
  if (!date) return '-'
  return new Date(date).toLocaleString()
}

function getCronDescription(cron) {
  if (!cron) return ''
  const parts = cron.split(' ')
  if (parts.length !== 5) return cron

  if (parts[0].startsWith('*/')) {
    return `Every ${parts[0].substring(2)} minutes`
  }
  if (parts[0] === '0' && parts[1] === '*') {
    return 'Hourly'
  }
  if (parts[1] !== '*' && !parts[1].includes('/')) {
    const hour = parseInt(parts[1])
    const minute = parseInt(parts[0])
    return `Daily at ${hour}:${minute.toString().padStart(2, '0')}`
  }
  return cron
}

onMounted(async () => {
  await Promise.all([
    schedulesStore.fetchSchedules(),
    nodesStore.fetchNodes(),
    playbooksStore.fetchPlaybooks()
  ])
})
</script>

<style scoped>
.schedules-page {
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 0;
  font-size: 24px;
  color: white;
}

.header-actions {
  display: flex;
  gap: 12px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  background: #1f2937;
  border-radius: 8px;
  padding: 16px;
  display: flex;
  align-items: center;
  gap: 12px;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
}

.stat-icon.total { background: rgba(33, 150, 243, 0.2); }
.stat-icon.enabled { background: rgba(76, 175, 80, 0.2); }
.stat-icon.disabled { background: rgba(158, 158, 158, 0.2); }

.stat-info {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
  color: white;
}

.stat-label {
  color: #9ca3af;
  font-size: 14px;
}

.loading-container {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 40px;
  color: #9ca3af;
}

.spinner {
  width: 24px;
  height: 24px;
  border: 2px solid #374151;
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.error-message {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  color: #f87171;
  padding: 16px;
  border-radius: 8px;
}

.empty-state {
  text-align: center;
  padding: 40px;
  color: #9ca3af;
  background: #1f2937;
  border-radius: 8px;
}

.schedules-grid {
  background: #1f2937;
  border-radius: 8px;
  padding: 16px;
}

.schedule-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));
  gap: 16px;
}

.schedule-card {
  background: #111827;
  border: 1px solid #374151;
  border-radius: 8px;
  overflow: hidden;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: #1f2937;
  border-bottom: 1px solid #374151;
}

.schedule-name {
  display: flex;
  align-items: center;
  gap: 12px;
}

.schedule-name h3 {
  margin: 0;
  font-size: 16px;
  color: white;
}

.status-badge {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 500;
  text-transform: uppercase;
}

.status-badge.enabled {
  background: rgba(76, 175, 80, 0.2);
  color: #4caf50;
}

.status-badge.disabled {
  background: rgba(158, 158, 158, 0.2);
  color: #9e9e9e;
}

.card-actions {
  display: flex;
  gap: 4px;
}

.btn-icon {
  background: none;
  border: none;
  cursor: pointer;
  padding: 4px 8px;
  font-size: 14px;
  border-radius: 4px;
}

.btn-icon:hover {
  background: #374151;
}

.btn-icon.danger:hover {
  background: rgba(244, 67, 54, 0.2);
}

.card-body {
  padding: 12px 16px;
}

.info-row {
  display: flex;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
}

.info-row .label {
  color: #9ca3af;
  min-width: 100px;
}

.info-row .value {
  color: #d1d5db;
}

.info-row code {
  background: #374151;
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 12px;
  color: #d1d5db;
}

.cron-desc {
  color: #9ca3af;
  font-size: 12px;
  margin-left: 8px;
}

.btn {
  padding: 8px 16px;
  border-radius: 6px;
  font-weight: 500;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.btn-primary {
  background: #3b82f6;
  color: white;
  border: none;
}

.btn-primary:hover {
  background: #2563eb;
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-secondary {
  background: transparent;
  color: white;
  border: 1px solid #374151;
}

.btn-secondary:hover {
  background: #374151;
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background: #1f2937;
  border-radius: 12px;
  width: 500px;
  max-height: 80vh;
  overflow: auto;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 24px;
  border-bottom: 1px solid #374151;
}

.modal-header h2 {
  margin: 0;
  font-size: 18px;
  color: white;
}

.close-btn {
  background: none;
  border: none;
  font-size: 20px;
  cursor: pointer;
  color: #9ca3af;
}

.modal-body {
  padding: 24px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 24px;
  border-top: 1px solid #374151;
}

.form-group {
  margin-bottom: 16px;
}

.form-group label {
  display: block;
  margin-bottom: 6px;
  font-weight: 500;
  color: #d1d5db;
}

.form-group input,
.form-group select {
  width: 100%;
  padding: 8px 12px;
  border-radius: 4px;
  border: 1px solid #374151;
  background: #111827;
  color: white;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.node-selector {
  max-height: 150px;
  overflow-y: auto;
  border: 1px solid #374151;
  border-radius: 4px;
  padding: 8px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  cursor: pointer;
  color: #d1d5db;
}
</style>