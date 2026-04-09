<template>
  <div class="agent-page">
    <div class="page-header">
      <h1>{{ t('runtime.nodeXAgents.title') }}</h1>
      <p class="text-secondary">{{ t('runtime.nodeXAgents.subtitle') }}</p>
    </div>

    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'agents' }]" @click="activeTab = 'agents'">
        {{ t('runtime.nodeXAgents.tabs.agents') }}
      </button>
      <button :class="['tab', { active: activeTab === 'terminal' }]" @click="activeTab = 'terminal'">
        {{ t('runtime.nodeXAgents.tabs.terminal') }}
      </button>
      <button :class="['tab', { active: activeTab === 'tasks' }]" @click="activeTab = 'tasks'">
        {{ t('runtime.nodeXAgents.tabs.tasks') }}
      </button>
    </div>

    <div v-show="activeTab === 'agents'">
      <div class="toolbar">
        <button class="btn-secondary" @click="fetchAgents">{{ t('runtime.nodeXAgents.actions.refresh') }}</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('runtime.nodeXAgents.table.nodeId') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.version') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.system') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.lastSeen') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.status') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.capabilities') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.action') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="agent in agents" :key="agent.node_id">
              <td>{{ agent.node_id }}</td>
              <td>{{ agent.version || '-' }}</td>
              <td>
                <span v-if="agent.system">
                  {{ agent.system.os || '-' }} {{ agent.system.arch || '' }}
                </span>
                <span v-else>-</span>
              </td>
              <td>{{ formatTime(agent.last_seen) }}</td>
              <td>
                <span :class="['status-badge', agent.online ? 'status-active' : 'status-offline']">
                  {{ agent.online ? t('runtime.nodeXAgents.status.online') : t('runtime.nodeXAgents.status.offline') }}
                </span>
              </td>
              <td>
                <div class="capability-tags">
                  <span v-for="cap in (agent.capabilities || [])" :key="cap" class="cap-tag">
                    {{ cap }}
                  </span>
                </div>
              </td>
              <td>
                <div class="action-buttons">
                  <button
                    class="btn-sm btn-ghost"
                    :title="t('runtime.nodeXAgents.tabs.terminal')"
                    @click="openTerminal(agent)"
                  >
                    {{ t('runtime.nodeXAgents.actions.terminalShort') }}
                  </button>
                  <button
                    class="btn-sm btn-ghost"
                    :title="t('runtime.nodeXAgents.taskModal.title')"
                    @click="openTaskModal(agent)"
                  >
                    {{ t('runtime.nodeXAgents.actions.taskShort') }}
                  </button>
                  <button
                    class="btn-sm btn-ghost"
                    :title="t('runtime.nodeXAgents.actions.monitor')"
                    @click="viewMonitor(agent)"
                  >
                    {{ t('runtime.nodeXAgents.actions.monitorShort') }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="agents.length === 0">
              <td colspan="7" class="empty-row">{{ t('runtime.nodeXAgents.empty.agents') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-show="activeTab === 'terminal'">
      <div class="terminal-container">
        <div class="terminal-header">
          <select v-model="selectedNodeId" class="node-select">
            <option value="">{{ t('runtime.nodeXAgents.terminal.chooseNode') }}</option>
            <option v-for="agent in onlineAgents" :key="agent.node_id" :value="agent.node_id">
              {{ t('runtime.nodeXAgents.terminal.nodeLabel', { id: agent.node_id }) }}
            </option>
          </select>
          <span class="terminal-status" :class="{ connected: wsConnected }">
            {{ wsConnected ? t('runtime.nodeXAgents.status.connected') : t('runtime.nodeXAgents.status.disconnected') }}
          </span>
        </div>
        <div ref="terminalOutput" class="terminal-output">
          <div v-for="(line, index) in terminalLines" :key="index" class="terminal-line">
            <span class="line-prompt">{{ line.prompt }}</span>
            <span class="line-content" :class="line.type">{{ line.content }}</span>
          </div>
        </div>
        <div class="terminal-input">
          <span class="prompt">$</span>
          <input
            v-model="commandInput"
            :disabled="!selectedNodeId"
            :placeholder="t('runtime.nodeXAgents.terminal.promptPlaceholder')"
            @keyup.enter="executeCommand"
          />
          <button :disabled="!selectedNodeId || !commandInput" @click="executeCommand">
            {{ t('runtime.nodeXAgents.actions.execute') }}
          </button>
        </div>
      </div>
    </div>

    <div v-show="activeTab === 'tasks'">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('runtime.nodeXAgents.table.taskId') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.node') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.type') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.command') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.status') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.duration') }}</th>
              <th>{{ t('runtime.nodeXAgents.table.time') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="task in taskHistory" :key="task.task_id">
              <td>{{ task.task_id }}</td>
              <td>{{ t('runtime.nodeXAgents.terminal.nodeLabel', { id: task.node_id }) }}</td>
              <td>{{ task.type }}</td>
              <td><code>{{ task.action }}</code></td>
              <td>
                <span :class="['status-badge', task.success ? 'status-active' : 'status-error']">
                  {{ task.success ? t('runtime.nodeXAgents.status.success') : t('runtime.nodeXAgents.status.failed') }}
                </span>
              </td>
              <td>{{ task.duration_ms }} ms</td>
              <td>{{ formatTime(task.timestamp) }}</td>
            </tr>
            <tr v-if="taskHistory.length === 0">
              <td colspan="7" class="empty-row">{{ t('runtime.nodeXAgents.empty.tasks') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="showTaskModal" class="modal-overlay" @click.self="showTaskModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>{{ t('runtime.nodeXAgents.taskModal.title') }}</h3>
          <button
            class="close-btn"
            :aria-label="t('common.actions.close')"
            :title="t('common.actions.close')"
            @click="showTaskModal = false"
          >
            x
          </button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>{{ t('runtime.nodeXAgents.taskModal.targetNode') }}</label>
            <input :value="taskTargetNode?.node_id" disabled />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.nodeXAgents.taskModal.taskType') }}</label>
            <select v-model="taskForm.type">
              <option value="command">{{ t('runtime.nodeXAgents.taskTypes.command') }}</option>
              <option value="file">{{ t('runtime.nodeXAgents.taskTypes.file') }}</option>
              <option value="service">{{ t('runtime.nodeXAgents.taskTypes.service') }}</option>
              <option value="gost">{{ t('runtime.nodeXAgents.taskTypes.gost') }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>{{ t('runtime.nodeXAgents.taskModal.action') }}</label>
            <input v-model="taskForm.action" :placeholder="t('runtime.nodeXAgents.table.command')" />
          </div>
          <div class="form-group">
            <label>{{ t('runtime.nodeXAgents.taskModal.paramsJson') }}</label>
            <textarea
              v-model="taskForm.paramsJson"
              :placeholder="t('runtime.nodeXAgents.taskModal.paramsPlaceholder')"
              rows="3"
            ></textarea>
          </div>
          <div class="form-group">
            <label>{{ t('runtime.nodeXAgents.taskModal.timeoutSeconds') }}</label>
            <input v-model.number="taskForm.timeout" type="number" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showTaskModal = false">{{ t('runtime.nodeXAgents.actions.cancel') }}</button>
          <button @click="sendTask">{{ t('runtime.nodeXAgents.actions.send') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { createAgentTask, executeAgentCommand, getAgents } from '@/api/admin'

const { t, formatDateTime } = useAppI18n()

const activeTab = ref('agents')
const agents = ref([])
const selectedNodeId = ref('')
const wsConnected = ref(false)
const commandInput = ref('')
const terminalLines = ref([])
const terminalOutput = ref(null)
const taskHistory = ref([])
const showTaskModal = ref(false)
const taskTargetNode = ref(null)
const taskForm = ref({
  type: 'command',
  action: '',
  paramsJson: '{}',
  timeout: 30
})

const onlineAgents = computed(() => agents.value.filter(agent => agent.online))
const notify = message => window.alert(message)

const fetchAgents = async () => {
  try {
    const res = await getAgents()
    agents.value = res.data?.agents || []
  } catch (err) {
    console.error(t('runtime.nodeXAgents.messages.fetchFailed'), err)
  }
}

const formatTime = (time) => {
  if (!time) {
    return '-'
  }

  return formatDateTime(time, {
    second: '2-digit'
  }) || String(time)
}

const openTerminal = (agent) => {
  selectedNodeId.value = agent.node_id
  activeTab.value = 'terminal'
}

const openTaskModal = (agent) => {
  taskTargetNode.value = agent
  taskForm.value = {
    type: 'command',
    action: '',
    paramsJson: '{}',
    timeout: 30
  }
  showTaskModal.value = true
}

const viewMonitor = (agent) => {
  notify(t('runtime.nodeXAgents.hints.monitor', { id: agent.node_id }))
}

const executeCommand = async () => {
  if (!selectedNodeId.value || !commandInput.value) {
    return
  }

  const cmd = commandInput.value
  commandInput.value = ''

  terminalLines.value.push({
    prompt: '$ ',
    content: cmd,
    type: 'input'
  })

  try {
    const res = await executeAgentCommand({
      node_id: selectedNodeId.value,
      command: cmd.split(' ')[0],
      args: cmd.split(' ').slice(1),
      timeout: 30
    })

    const result = res.data
    terminalLines.value.push({
      prompt: '',
      content: result.output || JSON.stringify(result, null, 2),
      type: result.success ? 'output' : 'error'
    })

    taskHistory.value.unshift({
      task_id: result.task_id || Date.now().toString(),
      node_id: selectedNodeId.value,
      type: 'command',
      action: cmd,
      success: result.success,
      duration_ms: result.duration_ms || 0,
      timestamp: new Date()
    })
  } catch (err) {
    terminalLines.value.push({
      prompt: '',
      content: t('runtime.nodeXAgents.messages.commandError', {
        message: err.response?.data?.error || err.message
      }),
      type: 'error'
    })
  }

  await nextTick()
  if (terminalOutput.value) {
    terminalOutput.value.scrollTop = terminalOutput.value.scrollHeight
  }
}

const sendTask = async () => {
  if (!taskTargetNode.value || !taskForm.value.action) {
    notify(t('runtime.nodeXAgents.messages.taskIncomplete'))
    return
  }

  let params = {}
  try {
    params = JSON.parse(taskForm.value.paramsJson || '{}')
  } catch (error) {
    notify(t('runtime.nodeXAgents.messages.invalidParamsJson'))
    return
  }

  try {
    await createAgentTask({
      node_id: taskTargetNode.value.node_id,
      type: taskForm.value.type,
      action: taskForm.value.action,
      params,
      timeout: taskForm.value.timeout
    })

    showTaskModal.value = false
    notify(t('runtime.nodeXAgents.messages.taskSent'))
  } catch (err) {
    notify(t('runtime.nodeXAgents.messages.taskSendFailed', {
      message: err.response?.data?.error || err.message
    }))
  }
}

let refreshTimer
onMounted(() => {
  fetchAgents()
  refreshTimer = setInterval(fetchAgents, 30000)
})

onUnmounted(() => {
  clearInterval(refreshTimer)
})
</script>

<style scoped>
.tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
}

.tab {
  padding: 10px 20px;
  background: var(--surface-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: var(--transition);
}

.tab.active {
  background: var(--primary-color);
  color: white;
  border-color: var(--primary-color);
}

.toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 16px;
}

.table-container {
  overflow-x: auto;
}

.action-buttons {
  display: flex;
  gap: 8px;
}

.btn-sm {
  min-width: 32px;
}

.btn-ghost {
  padding: 6px 10px;
  background: transparent;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  color: var(--text-color);
  cursor: pointer;
}

.btn-ghost:hover {
  border-color: var(--primary-color);
  color: var(--primary-color);
}

.capability-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.cap-tag {
  font-size: 11px;
  padding: 2px 6px;
  background: rgba(59, 130, 246, 0.15);
  color: #3b82f6;
  border-radius: 4px;
}

.terminal-container {
  background: #1e1e1e;
  border-radius: var(--radius-lg);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  height: 500px;
}

.terminal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: #2d2d2d;
  border-bottom: 1px solid #3d3d3d;
}

.node-select {
  background: #3d3d3d;
  border: 1px solid #4d4d4d;
  color: #fff;
  padding: 8px 12px;
  border-radius: var(--radius-md);
}

.terminal-status {
  font-size: 12px;
  padding: 4px 12px;
  border-radius: 12px;
  background: rgba(239, 68, 68, 0.2);
  color: #ef4444;
}

.terminal-status.connected {
  background: rgba(34, 197, 94, 0.2);
  color: #22c55e;
}

.terminal-output {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  font-family: Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.5;
}

.terminal-line {
  margin-bottom: 4px;
}

.line-prompt {
  color: #22c55e;
}

.line-content {
  color: #d4d4d4;
  white-space: pre-wrap;
  word-break: break-all;
}

.line-content.input {
  color: #4fc3f7;
}

.line-content.error {
  color: #ef4444;
}

.line-content.output {
  color: #d4d4d4;
}

.terminal-input {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: #2d2d2d;
  border-top: 1px solid #3d3d3d;
  gap: 8px;
}

.terminal-input .prompt {
  color: #22c55e;
  font-family: Consolas, Monaco, monospace;
}

.terminal-input input {
  flex: 1;
  background: transparent;
  border: none;
  color: #fff;
  font-family: Consolas, Monaco, monospace;
  font-size: 13px;
  outline: none;
}

.terminal-input input::placeholder {
  color: #666;
}

.terminal-input button {
  padding: 8px 16px;
  background: var(--primary-color);
  border: none;
  border-radius: var(--radius-md);
  color: white;
  cursor: pointer;
}

.terminal-input button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.status-error {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error-color);
}

.close-btn {
  background: transparent;
  border: none;
  color: var(--text-secondary);
  font-size: 20px;
  cursor: pointer;
}

code {
  background: var(--bg-color);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
}
</style>
