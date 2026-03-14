<template>
  <div class="agent-page">
    <div class="page-header">
      <h1>Agent 管理</h1>
      <p class="text-secondary">管理远程节点 Agent，执行命令和监控状态</p>
    </div>

    <!-- 标签切换 -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 'agents' }]" @click="activeTab = 'agents'">
        在线 Agent
      </button>
      <button :class="['tab', { active: activeTab === 'terminal' }]" @click="activeTab = 'terminal'">
        远程终端
      </button>
      <button :class="['tab', { active: activeTab === 'tasks' }]" @click="activeTab = 'tasks'">
        任务历史
      </button>
    </div>

    <!-- Agent 列表 -->
    <div v-show="activeTab === 'agents'">
      <div class="toolbar">
        <button class="btn-secondary" @click="fetchAgents">刷新</button>
      </div>

      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>节点 ID</th>
              <th>版本</th>
              <th>系统</th>
              <th>最后在线</th>
              <th>状态</th>
              <th>能力</th>
              <th>操作</th>
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
                  {{ agent.online ? '在线' : '离线' }}
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
                  <button class="btn-sm btn-ghost" @click="openTerminal(agent)" title="终端">
                    💻
                  </button>
                  <button class="btn-sm btn-ghost" @click="openTaskModal(agent)" title="下发任务">
                    📤
                  </button>
                  <button class="btn-sm btn-ghost" @click="viewMonitor(agent)" title="监控">
                    📊
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="agents.length === 0">
              <td colspan="7" class="empty-row">暂无在线 Agent</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 远程终端 -->
    <div v-show="activeTab === 'terminal'">
      <div class="terminal-container">
        <div class="terminal-header">
          <select v-model="selectedNodeId" class="node-select">
            <option value="">选择节点</option>
            <option v-for="agent in onlineAgents" :key="agent.node_id" :value="agent.node_id">
              Node #{{ agent.node_id }}
            </option>
          </select>
          <span class="terminal-status" :class="{ connected: wsConnected }">
            {{ wsConnected ? '已连接' : '未连接' }}
          </span>
        </div>
        <div class="terminal-output" ref="terminalOutput">
          <div v-for="(line, index) in terminalLines" :key="index" class="terminal-line">
            <span class="line-prompt">{{ line.prompt }}</span>
            <span class="line-content" :class="line.type">{{ line.content }}</span>
          </div>
        </div>
        <div class="terminal-input">
          <span class="prompt">$</span>
          <input
            v-model="commandInput"
            @keyup.enter="executeCommand"
            placeholder="输入命令..."
            :disabled="!selectedNodeId"
          />
          <button @click="executeCommand" :disabled="!selectedNodeId || !commandInput">执行</button>
        </div>
      </div>
    </div>

    <!-- 任务历史 -->
    <div v-show="activeTab === 'tasks'">
      <div class="table-container">
        <table class="data-table">
          <thead>
            <tr>
              <th>任务 ID</th>
              <th>节点</th>
              <th>类型</th>
              <th>命令/动作</th>
              <th>状态</th>
              <th>耗时</th>
              <th>时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="task in taskHistory" :key="task.task_id">
              <td>{{ task.task_id }}</td>
              <td>Node #{{ task.node_id }}</td>
              <td>{{ task.type }}</td>
              <td><code>{{ task.action }}</code></td>
              <td>
                <span :class="['status-badge', task.success ? 'status-active' : 'status-error']">
                  {{ task.success ? '成功' : '失败' }}
                </span>
              </td>
              <td>{{ task.duration_ms }}ms</td>
              <td>{{ formatTime(task.timestamp) }}</td>
            </tr>
            <tr v-if="taskHistory.length === 0">
              <td colspan="7" class="empty-row">暂无任务记录</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 任务下发弹窗 -->
    <div v-if="showTaskModal" class="modal-overlay" @click.self="showTaskModal = false">
      <div class="modal">
        <div class="modal-header">
          <h3>下发任务</h3>
          <button class="close-btn" @click="showTaskModal = false">x</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>目标节点</label>
            <input :value="taskTargetNode?.node_id" disabled />
          </div>
          <div class="form-group">
            <label>任务类型</label>
            <select v-model="taskForm.type">
              <option value="command">执行命令</option>
              <option value="file">文件操作</option>
              <option value="service">服务管理</option>
              <option value="gost">GOST 管理</option>
            </select>
          </div>
          <div class="form-group">
            <label>动作</label>
            <input v-model="taskForm.action" placeholder="命令或动作" />
          </div>
          <div class="form-group">
            <label>参数 (JSON)</label>
            <textarea v-model="taskForm.paramsJson" placeholder='{"key": "value"}' rows="3"></textarea>
          </div>
          <div class="form-group">
            <label>超时 (秒)</label>
            <input v-model.number="taskForm.timeout" type="number" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn-secondary" @click="showTaskModal = false">取消</button>
          <button @click="sendTask">发送</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { getAgents, createAgentTask, executeAgentCommand } from '@/api/admin'

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

const onlineAgents = computed(() => agents.value.filter(a => a.online))

const fetchAgents = async () => {
  try {
    const res = await getAgents()
    agents.value = res.data?.agents || []
  } catch (err) {
    console.error('获取 Agent 列表失败:', err)
  }
}

const formatTime = (time) => {
  if (!time) return '-'
  const date = new Date(time)
  return date.toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
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
  // TODO: 打开监控面板
  alert(`查看节点 #${agent.node_id} 的监控数据`)
}

const executeCommand = async () => {
  if (!selectedNodeId.value || !commandInput.value) return

  const cmd = commandInput.value
  commandInput.value = ''

  // 添加到终端输出
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

    // 添加结果
    const result = res.data
    terminalLines.value.push({
      prompt: '',
      content: result.output || JSON.stringify(result, null, 2),
      type: result.success ? 'output' : 'error'
    })

    // 记录到任务历史
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
      content: 'Error: ' + (err.response?.data?.error || err.message),
      type: 'error'
    })
  }

  // 滚动到底部
  await nextTick()
  if (terminalOutput.value) {
    terminalOutput.value.scrollTop = terminalOutput.value.scrollHeight
  }
}

const sendTask = async () => {
  if (!taskTargetNode.value || !taskForm.value.action) {
    alert('请填写完整信息')
    return
  }

  try {
    let params = {}
    try {
      params = JSON.parse(taskForm.value.paramsJson || '{}')
    } catch (e) {
      // ignore
    }

    await createAgentTask({
      node_id: taskTargetNode.value.node_id,
      type: taskForm.value.type,
      action: taskForm.value.action,
      params,
      timeout: taskForm.value.timeout
    })

    showTaskModal.value = false
    alert('任务已发送')
  } catch (err) {
    alert('发送失败: ' + (err.response?.data?.error || err.message))
  }
}

let refreshTimer
onMounted(() => {
  fetchAgents()
  // 每 30 秒刷新一次
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
  font-family: 'Consolas', 'Monaco', monospace;
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
  font-family: 'Consolas', 'Monaco', monospace;
}

.terminal-input input {
  flex: 1;
  background: transparent;
  border: none;
  color: #fff;
  font-family: 'Consolas', 'Monaco', monospace;
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

code {
  background: var(--bg-color);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12px;
}
</style>