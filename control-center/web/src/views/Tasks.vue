<template>
  <div class="tasks-page">
    <div class="page-header">
      <h1>Tasks</h1>
      <div class="header-actions">
        <button class="btn btn-primary" @click="showCreateDialog = true">
          <span class="icon">➕</span> New Task
        </button>
        <button class="btn btn-secondary" @click="tasksStore.fetchTasks()">
          <span class="icon">🔄</span> Refresh
        </button>
      </div>
    </div>

    <!-- Stats Cards -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon pending">⏳</div>
        <div class="stat-info">
          <span class="stat-value">{{ tasksStore.pendingCount }}</span>
          <span class="stat-label">Pending</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon running">🏃</div>
        <div class="stat-info">
          <span class="stat-value">{{ tasksStore.runningCount }}</span>
          <span class="stat-label">Running</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon completed">✅</div>
        <div class="stat-info">
          <span class="stat-value">{{ tasksStore.completedCount }}</span>
          <span class="stat-label">Completed</span>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon failed">❌</div>
        <div class="stat-info">
          <span class="stat-value">{{ tasksStore.failedCount }}</span>
          <span class="stat-label">Failed</span>
        </div>
      </div>
    </div>

    <!-- Filters -->
    <div class="filters-bar">
      <div class="filter-group">
        <label>Status:</label>
        <select v-model="tasksStore.statusFilter">
          <option value="">All</option>
          <option value="pending">Pending</option>
          <option value="running">Running</option>
          <option value="completed">Completed</option>
          <option value="failed">Failed</option>
          <option value="cancelled">Cancelled</option>
        </select>
      </div>
      <div class="filter-group">
        <label>Playbook:</label>
        <select v-model="playbookFilter">
          <option value="">All Playbooks</option>
          <option v-for="p in playbooksStore.playbooks" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="tasksStore.loading" class="loading-container">
      <div class="spinner"></div>
      <span>Loading tasks...</span>
    </div>

    <!-- Error State -->
    <div v-else-if="tasksStore.error" class="error-message">
      {{ tasksStore.error }}
    </div>

    <!-- Tasks Table -->
    <div v-else class="table-container">
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>Playbook</th>
            <th>Status</th>
            <th>Target Nodes</th>
            <th>Created</th>
            <th>Duration</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="filteredTasks.length === 0">
            <td colspan="7" class="empty-row">No tasks found</td>
          </tr>
          <tr v-else v-for="task in filteredTasks" :key="task.id" @click="viewTask(task)">
            <td><code>{{ task.taskId || task.id }}</code></td>
            <td>{{ task.playbookName || task.playbook_name }}</td>
            <td>
              <span :class="['status-badge', task.status]">
                {{ task.status }}
              </span>
            </td>
            <td>{{ task.targetNodes?.length || task.target_nodes?.length || 0 }} nodes</td>
            <td>{{ formatDate(task.createdAt || task.created_at) }}</td>
            <td>{{ formatDuration(task) }}</td>
            <td class="actions">
              <button v-if="task.status === 'running'" class="btn-icon danger" @click.stop="cancelTask(task)" title="Cancel">
                ⏹️
              </button>
              <button v-if="task.status === 'failed'" class="btn-icon warning" @click.stop="retryTask(task)" title="Retry">
                🔄
              </button>
              <button class="btn-icon" @click.stop="viewTask(task)" title="View Details">
                👁️
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Create Task Dialog -->
    <div v-if="showCreateDialog" class="modal-overlay" @click.self="showCreateDialog = false">
      <div class="modal">
        <div class="modal-header">
          <h2>Create Task</h2>
          <button class="close-btn" @click="showCreateDialog = false">✕</button>
        </div>
        <div class="modal-body">
          <form @submit.prevent="createTask">
            <div class="form-group">
              <label>Playbook</label>
              <select v-model="newTask.playbookId" required>
                <option value="">Select playbook</option>
                <option v-for="p in playbooksStore.playbooks" :key="p.id || p.name" :value="p.id || p.name">{{ p.name }}</option>
              </select>
            </div>
            <div class="form-group">
              <label>Target Nodes</label>
              <div class="node-selector">
                <label v-for="node in nodesStore.nodes" :key="node.id" class="checkbox-label">
                  <input type="checkbox" :value="node.id" v-model="newTask.targetNodes" />
                  {{ node.name }}
                </label>
              </div>
            </div>
            <div class="form-group">
              <label>Variables (JSON)</label>
              <textarea v-model="newTask.variables" placeholder='{"key": "value"}' rows="4"></textarea>
            </div>
          </form>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="showCreateDialog = false">Cancel</button>
          <button class="btn btn-primary" @click="createTask" :disabled="tasksStore.loading">Create Task</button>
        </div>
      </div>
    </div>

    <!-- Task Detail Modal -->
    <div v-if="selectedTask" class="modal-overlay" @click.self="selectedTask = null">
      <div class="modal modal-lg">
        <div class="modal-header">
          <h2>Task #{{ selectedTask.taskId || selectedTask.id }}</h2>
          <button class="close-btn" @click="selectedTask = null">✕</button>
        </div>
        <div class="modal-body">
          <div class="detail-grid">
            <div class="detail-item">
              <label>Playbook:</label>
              <span>{{ selectedTask.playbookName || selectedTask.playbook_name }}</span>
            </div>
            <div class="detail-item">
              <label>Status:</label>
              <span :class="['status-badge', selectedTask.status]">{{ selectedTask.status }}</span>
            </div>
            <div class="detail-item">
              <label>Created:</label>
              <span>{{ formatDate(selectedTask.createdAt || selectedTask.created_at) }}</span>
            </div>
            <div class="detail-item">
              <label>Duration:</label>
              <span>{{ formatDuration(selectedTask) }}</span>
            </div>
          </div>
          <div class="logs-section">
            <h3>Execution Logs</h3>
            <div class="logs-container">
              <pre>{{ logsText }}</pre>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useTasksStore } from '@/stores/tasks'
import { useNodesStore } from '@/stores/nodes'
import { usePlaybooksStore } from '@/stores/playbooks'
import { useAuthStore } from '@/stores/auth'

const tasksStore = useTasksStore()
const nodesStore = useNodesStore()
const playbooksStore = usePlaybooksStore()
const authStore = useAuthStore()

const showCreateDialog = ref(false)
const selectedTask = ref(null)
const playbookFilter = ref('')

const newTask = ref({
  playbookId: '',
  targetNodes: [],
  variables: ''
})

const filteredTasks = computed(() => {
  let result = tasksStore.filteredTasks
  if (playbookFilter.value) {
    result = result.filter(t => t.playbookId === playbookFilter.value || t.playbook_id === playbookFilter.value)
  }
  return result
})

const logsText = computed(() => {
  if (!selectedTask.value) return ''
  if (tasksStore.taskLogs.length > 0) {
    return tasksStore.taskLogs.map(l => l.message || l).join('\n')
  }
  return selectedTask.value.logs || 'No logs available'
})

async function createTask() {
  const payload = {
    playbookId: newTask.value.playbookId,
    targetNodes: newTask.value.targetNodes,
  }
  if (newTask.value.variables) {
    try {
      payload.variables = JSON.parse(newTask.value.variables)
    } catch (e) {
      alert('Invalid JSON in variables')
      return
    }
  }

  const result = await tasksStore.createTask(payload)
  if (result.success) {
    showCreateDialog.value = false
    newTask.value = { playbookId: '', targetNodes: [], variables: '' }
  } else {
    alert(result.error || 'Failed to create task')
  }
}

async function cancelTask(task) {
  if (!confirm('Cancel this task?')) return
  await tasksStore.cancelTask(task.id || task.taskId)
}

async function retryTask(task) {
  await tasksStore.retryTask(task.id || task.taskId)
}

async function viewTask(task) {
  selectedTask.value = task
  await tasksStore.fetchTask(task.id || task.taskId)
  await tasksStore.fetchTaskLogs(task.id || task.taskId)
}

function formatDate(date) {
  if (!date) return '-'
  return new Date(date).toLocaleString()
}

function formatDuration(task) {
  const startedAt = task.startedAt || task.started_at
  const completedAt = task.completedAt || task.completed_at
  if (!startedAt) return '-'
  const start = new Date(startedAt)
  const end = completedAt ? new Date(completedAt) : new Date()
  const diff = Math.floor((end - start) / 1000)
  if (diff < 60) return `${diff}s`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ${diff % 60}s`
  return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`
}

onMounted(async () => {
  await Promise.all([
    tasksStore.fetchTasks(),
    nodesStore.fetchNodes(),
    playbooksStore.fetchPlaybooks()
  ])

  if (authStore.token) {
    await tasksStore.subscribeToUpdates(authStore.token)
  }
})

onUnmounted(() => {
  tasksStore.unsubscribeFromUpdates()
})
</script>

<style scoped>
.tasks-page {
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
  grid-template-columns: repeat(4, 1fr);
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

.stat-icon.pending { background: rgba(255, 193, 7, 0.2); }
.stat-icon.running { background: rgba(33, 150, 243, 0.2); }
.stat-icon.completed { background: rgba(76, 175, 80, 0.2); }
.stat-icon.failed { background: rgba(244, 67, 54, 0.2); }

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

.filters-bar {
  display: flex;
  gap: 16px;
  margin-bottom: 24px;
  padding: 16px;
  background: #1f2937;
  border-radius: 8px;
}

.filter-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.filter-group label {
  color: #9ca3af;
  font-size: 14px;
}

.filter-group select {
  padding: 8px 12px;
  border-radius: 4px;
  border: 1px solid #374151;
  background: #111827;
  color: white;
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

.table-container {
  background: #1f2937;
  border-radius: 8px;
  overflow: hidden;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 12px 16px;
  text-align: left;
  border-bottom: 1px solid #374151;
}

.data-table th {
  background: #111827;
  font-weight: 600;
  color: #9ca3af;
}

.data-table tbody tr:hover {
  background: rgba(59, 130, 246, 0.1);
  cursor: pointer;
}

.status-badge {
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  text-transform: uppercase;
}

.status-badge.pending { background: rgba(255, 193, 7, 0.2); color: #ffc107; }
.status-badge.running { background: rgba(33, 150, 243, 0.2); color: #2196f3; }
.status-badge.completed { background: rgba(76, 175, 80, 0.2); color: #4caf50; }
.status-badge.failed { background: rgba(244, 67, 54, 0.2); color: #f44336; }
.status-badge.cancelled { background: rgba(158, 158, 158, 0.2); color: #9e9e9e; }

.actions {
  display: flex;
  gap: 8px;
}

.btn-icon {
  background: none;
  border: none;
  cursor: pointer;
  padding: 4px;
  font-size: 16px;
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

.empty-row {
  text-align: center;
  color: #9ca3af;
  padding: 40px;
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

.modal-lg {
  width: 700px;
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

.form-group select,
.form-group textarea {
  width: 100%;
  padding: 8px 12px;
  border-radius: 4px;
  border: 1px solid #374151;
  background: #111827;
  color: white;
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

.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.detail-item label {
  color: #9ca3af;
  font-size: 12px;
}

.detail-item span {
  color: white;
}

.logs-section h3 {
  margin: 0 0 12px;
  font-size: 14px;
  color: #9ca3af;
}

.logs-container {
  background: #111827;
  border-radius: 8px;
  padding: 16px;
  max-height: 300px;
  overflow: auto;
}

.logs-container pre {
  margin: 0;
  color: #d4d4d4;
  font-family: monospace;
  font-size: 12px;
  white-space: pre-wrap;
}
</style>