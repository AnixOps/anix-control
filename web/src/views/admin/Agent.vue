<template>
  <div class="list-page agent-page">
    <UiPageHeader :title="t('runtime.nodeXAgents.title')" :description="t('runtime.nodeXAgents.subtitle')">
      <template #actions>
        <UiButton :icon="RefreshCw" :loading="agentsLoading" data-test="agent-refresh" @click="refreshAll">{{ t('runtime.nodeXAgents.actions.refresh') }}</UiButton>
      </template>
    </UiPageHeader>

    <UiTabs v-model="activeTab" variant="segmented" :aria-label="t('runtime.nodeXAgents.title')" :items="tabItems" :unmount-on-hide="false">
      <template #agents>
        <UiDataTable
          :columns="agentColumns"
          :rows="agents"
          row-key="node_id"
          :label="t('runtime.nodeXAgents.tabs.agents')"
          :row-label="agent => t('runtime.nodeXAgents.terminal.nodeLabel', { id: agent.node_id })"
          storage-key="admin.agents"
          :loading="agentsLoading"
          :error="agentsError"
          :error-title="t('runtime.nodeXAgents.messages.fetchFailed')"
          :empty-icon="Server"
          :empty-title="t('runtime.nodeXAgents.empty.agents')"
          :empty-description="t('runtime.nodeXAgents.empty.agentsDescription')"
          :row-actions="agentActions"
          @retry="fetchAgents"
        >
          <template #cell-status="{ row }">
            <UiBadge :status="row.online ? 'online' : 'offline'" :label="row.online ? t('runtime.nodeXAgents.status.online') : t('runtime.nodeXAgents.status.offline')" />
          </template>
          <template #cell-capabilities="{ row }">
            <span v-if="(row.capabilities || []).length" class="capability-tags">
              <span v-for="cap in row.capabilities" :key="cap" class="cap-tag">{{ cap }}</span>
            </span>
            <span v-else>—</span>
          </template>
        </UiDataTable>
      </template>

      <template #terminal>
        <section class="terminal" :aria-label="t('runtime.nodeXAgents.tabs.terminal')">
          <div class="terminal__bar">
            <UiSelect
              v-model="selectedNodeId"
              class="terminal__node"
              size="md"
              :aria-label="t('runtime.nodeXAgents.terminal.chooseNode')"
              :placeholder="t('runtime.nodeXAgents.terminal.chooseNode')"
              :options="nodeOptions"
            />
            <UiBadge :tone="wsConnected ? 'success' : 'neutral'" :label="wsConnected ? t('runtime.nodeXAgents.status.connected') : t('runtime.nodeXAgents.status.disconnected')" />
          </div>
          <div
            ref="terminalOutput"
            class="terminal__output"
            role="log"
            tabindex="0"
            :aria-label="t('runtime.nodeXAgents.terminal.output')"
          >
            <p v-if="!terminalLines.length" class="terminal__hint">{{ t('runtime.nodeXAgents.terminal.hint') }}</p>
            <div v-for="(line, index) in terminalLines" :key="index" class="terminal__line">
              <span class="terminal__prompt">{{ line.prompt }}</span>
              <span class="terminal__content" :class="`is-${line.type}`">{{ line.content }}</span>
            </div>
          </div>
          <div class="terminal__input">
            <UiSelect
              v-model="selectedAction"
              size="md"
              :aria-label="t('runtime.nodeXAgents.taskModal.action')"
              :placeholder="t('runtime.nodeXAgents.terminal.chooseAction')"
              :options="actionOptions"
              :disabled="!selectedNodeId"
            />
            <UiSelect
              v-if="selectedActionSpec?.params.includes('service')"
              v-model="selectedService"
              size="md"
              :aria-label="t('runtime.nodeXAgents.fields.service')"
              :options="serviceOptions"
            />
            <UiTextField
              v-if="selectedActionSpec?.params.includes('lines')"
              v-model.number="logLines"
              class="terminal__lines"
              size="md"
              type="number"
              min="1"
              max="1000"
              :aria-label="t('runtime.nodeXAgents.fields.lines')"
              :suffix="t('runtime.nodeXAgents.fields.lines')"
            />
            <UiButton variant="primary" :icon="Play" :disabled="!selectedNodeId || !selectedAction" @click="executeCommand">
              {{ t('runtime.nodeXAgents.actions.execute') }}
            </UiButton>
          </div>
        </section>
      </template>

      <template #tasks>
        <UiDataTable
          :columns="taskColumns"
          :rows="taskHistory"
          row-key="task_id"
          :label="t('runtime.nodeXAgents.tabs.tasks')"
          storage-key="admin.agent-tasks"
          :page-size="20"
          :page="taskPage"
          :loading="tasksLoading"
          :error="tasksError"
          :error-title="t('runtime.nodeXAgents.messages.tasksFetchFailed')"
          :empty-icon="ListChecks"
          :empty-title="t('runtime.nodeXAgents.empty.tasks')"
          :empty-description="t('runtime.nodeXAgents.empty.tasksDescription')"
          @retry="fetchTaskHistory"
          @update:page="taskPage = $event"
        >
          <template #cell-command="{ row }">
            <code>{{ diagnosticActionMap[row.action] ? t(`runtime.nodeXAgents.diagnosticActions.${row.action}`) : row.action }}</code>
          </template>
          <template #cell-status="{ row }">
            <UiBadge :tone="row.success ? 'success' : 'danger'" :label="row.success ? t('runtime.nodeXAgents.status.success') : t('runtime.nodeXAgents.status.failed')" />
          </template>
        </UiDataTable>
      </template>
    </UiTabs>

    <UiDialog v-model:open="showTaskModal" :title="t('runtime.nodeXAgents.taskModal.title')" :description="taskTargetNode ? t('runtime.nodeXAgents.terminal.nodeLabel', { id: taskTargetNode.node_id }) : ''" :dismissible="!taskSending">
      <div class="form-grid">
        <UiSelect
          v-model="taskForm.action"
          class="form-grid__full"
          required
          :label="t('runtime.nodeXAgents.taskModal.action')"
          :placeholder="t('runtime.nodeXAgents.terminal.chooseAction')"
          :options="actionOptions"
        />
        <UiSelect
          v-if="taskActionSpec?.params.includes('service')"
          v-model="taskForm.service"
          :label="t('runtime.nodeXAgents.fields.service')"
          :options="serviceOptions"
        />
        <UiTextField
          v-if="taskActionSpec?.params.includes('lines')"
          v-model.number="taskForm.lines"
          type="number"
          min="1"
          max="1000"
          :label="t('runtime.nodeXAgents.fields.lines')"
        />
        <UiTextField v-model.number="taskForm.timeout" type="number" min="1" :label="t('runtime.nodeXAgents.taskModal.timeoutSeconds')" />
      </div>
      <p v-if="taskError" class="form-error" role="alert" data-test="agent-task-error">{{ taskError }}</p>
      <template #footer="{ close }">
        <UiButton :disabled="taskSending" @click="close">{{ t('runtime.nodeXAgents.actions.cancel') }}</UiButton>
        <UiButton variant="primary" data-test="agent-task-send" :loading="taskSending" @click="sendTask">{{ t('runtime.nodeXAgents.actions.send') }}</UiButton>
      </template>
    </UiDialog>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Activity, ListChecks, Play, RefreshCw, Send, Server, SquareTerminal } from '@lucide/vue'
import { useAppI18n } from '@/composables/useAppI18n'
import { useListQuery } from '@/composables/useListQuery'
import UiBadge from '@/ui/UiBadge.vue'
import UiButton from '@/ui/UiButton.vue'
import UiDataTable from '@/ui/UiDataTable.vue'
import UiDialog from '@/ui/UiDialog.vue'
import UiPageHeader from '@/ui/UiPageHeader.vue'
import UiSelect from '@/ui/UiSelect.vue'
import UiTabs from '@/ui/UiTabs.vue'
import UiTextField from '@/ui/UiTextField.vue'
import { useFormat } from '@/ui/composables/useFormat'
import { useToast } from '@/ui/composables/useToast'
import { createAgentTask, executeAgentCommand, getAgents, listAgentDiagnosticTasks } from '@/api/admin'

const { t } = useAppI18n()
const format = useFormat()

// 与后端 internal/service/agent_diagnostic_actions.go 的白名单保持一致，
// 前端不接受任意字符串动作，只能从这份列表里选。
const diagnosticActions = [
  { value: 'service_status', params: ['service'] },
  { value: 'service_restart', params: ['service'] },
  { value: 'log_tail', params: ['service', 'lines'] }
]
const diagnosticServices = ['gost']
const diagnosticActionMap = Object.fromEntries(diagnosticActions.map(action => [action.value, action]))

// The open tab and the task history page live in the URL query (plan §9).
const listQuery = useListQuery()
const activeTab = ref(listQuery.read('tab', { values: ['agents', 'terminal', 'tasks'] }) || 'agents')
const taskPage = ref(listQuery.readPage())
watch([activeTab, taskPage], () => listQuery.write({
  tab: activeTab.value === 'agents' ? '' : activeTab.value,
  page: activeTab.value === 'tasks' ? taskPage.value : 1
}))
const agents = ref([])
const selectedNodeId = ref('')
const wsConnected = ref(false)
const selectedAction = ref('')
const selectedService = ref(diagnosticServices[0])
const logLines = ref(100)
const terminalLines = ref([])
const terminalOutput = ref(null)
const taskHistory = ref([])
const showTaskModal = ref(false)
const taskTargetNode = ref(null)
const taskForm = ref({
  action: '',
  service: diagnosticServices[0],
  lines: 100,
  timeout: 30
})

const onlineAgents = computed(() => agents.value.filter(agent => agent.online))
const selectedActionSpec = computed(() => diagnosticActionMap[selectedAction.value] || null)
const taskActionSpec = computed(() => diagnosticActionMap[taskForm.value.action] || null)
const toast = useToast()
const agentsLoading = ref(false)
const agentsError = ref(null)
const tasksLoading = ref(false)
const tasksError = ref(null)

const tabItems = computed(() => [
  { value: 'agents', label: t('runtime.nodeXAgents.tabs.agents'), count: agents.value.length },
  { value: 'terminal', label: t('runtime.nodeXAgents.tabs.terminal') },
  { value: 'tasks', label: t('runtime.nodeXAgents.tabs.tasks') }
])
const nodeOptions = computed(() => onlineAgents.value.map(agent => ({
  value: agent.node_id,
  label: t('runtime.nodeXAgents.terminal.nodeLabel', { id: agent.node_id })
})))
const actionOptions = computed(() => diagnosticActions.map(action => ({
  value: action.value,
  label: t(`runtime.nodeXAgents.diagnosticActions.${action.value}`)
})))
const serviceOptions = computed(() => diagnosticServices.map(service => ({
  value: service,
  label: t(`runtime.nodeXAgents.services.${service}`)
})))
// The list API answers node, version, system, last seen, capabilities and
// online state; certificate expiry and the connection type (mTLS or the
// legacy key) are not in it yet.
const agentColumns = computed(() => [
  { key: 'node_id', label: t('runtime.nodeXAgents.table.nodeId'), primary: true, sortable: true, numeric: true, format: value => t('runtime.nodeXAgents.terminal.nodeLabel', { id: value }) },
  { key: 'status', label: t('runtime.nodeXAgents.table.status'), secondary: true, sortable: true, sortValue: agent => (agent.online ? 0 : 1) },
  { key: 'version', label: t('runtime.nodeXAgents.table.version'), sortable: true },
  { key: 'system', label: t('runtime.nodeXAgents.table.system'), value: agent => (agent.system ? [agent.system.os, agent.system.arch].filter(Boolean).join(' ') : '') },
  { key: 'last_seen', label: t('runtime.nodeXAgents.table.lastSeen'), sortable: true, firstDirection: 'desc', nowrap: true, numeric: true, format: value => formatTime(value), sortValue: agent => new Date(agent.last_seen || 0).getTime() },
  { key: 'capabilities', label: t('runtime.nodeXAgents.table.capabilities'), breakpoint: 'lg' }
])
const taskColumns = computed(() => [
  { key: 'task_id', label: t('runtime.nodeXAgents.table.taskId'), primary: true },
  { key: 'command', label: t('runtime.nodeXAgents.table.command'), secondary: true },
  { key: 'node_id', label: t('runtime.nodeXAgents.table.node'), sortable: true, format: value => t('runtime.nodeXAgents.terminal.nodeLabel', { id: value }) },
  { key: 'status', label: t('runtime.nodeXAgents.table.status'), sortable: true, sortValue: task => (task.success ? 0 : 1) },
  { key: 'duration_ms', label: t('runtime.nodeXAgents.table.duration'), numeric: true, align: 'end', sortable: true, format: value => (value === undefined || value === null ? '—' : `${value} ms`) },
  { key: 'timestamp', label: t('runtime.nodeXAgents.table.time'), sortable: true, firstDirection: 'desc', nowrap: true, numeric: true, format: value => formatTime(value), sortValue: task => new Date(task.timestamp || 0).getTime() }
])
const agentActions = agent => [
  { key: 'terminal', label: t('runtime.nodeXAgents.actions.openTerminal'), icon: SquareTerminal, disabled: !agent.online, onSelect: () => openTerminal(agent) },
  { key: 'task', label: t('runtime.nodeXAgents.taskModal.title'), icon: Send, onSelect: () => openTaskModal(agent) },
  { key: 'monitor', label: t('runtime.nodeXAgents.actions.monitor'), icon: Activity, onSelect: () => viewMonitor(agent) }
]
const refreshAll = () => Promise.all([fetchAgents(), fetchTaskHistory()])
const taskSending = ref(false)
const taskError = ref('')

const buildActionParams = (actionValue, service, lines) => {
  const spec = diagnosticActionMap[actionValue]
  if (!spec) {
    return {}
  }
  const params = {}
  if (spec.params.includes('service')) {
    params.service = service
  }
  if (spec.params.includes('lines')) {
    params.lines = lines
  }
  return params
}

const fetchAgents = async () => {
  agentsLoading.value = true
  try {
    const res = await getAgents()
    const payload = readAgentObject(res)
    agents.value = payload.agents || payload.list || []
    agentsError.value = null
  } catch (err) {
    agentsError.value = err
  } finally {
    agentsLoading.value = false
  }
}

const fetchTaskHistory = async () => {
  tasksLoading.value = true
  try {
    const res = await listAgentDiagnosticTasks({ limit: 50 })
    taskHistory.value = readAgentList(res)
    tasksError.value = null
  } catch (err) {
    tasksError.value = err
  } finally {
    tasksLoading.value = false
  }
}

const readAgentPayload = (res) => {
  if (!res || typeof res !== 'object') {
    return null
  }
  if (Object.prototype.hasOwnProperty.call(res, 'code')) {
    return res.data ?? null
  }
  if (
    res.data &&
    typeof res.data === 'object' &&
    Object.prototype.hasOwnProperty.call(res.data, 'data')
  ) {
    return res.data.data ?? null
  }
  return res.data ?? res
}

const readAgentObject = (res) => {
  const payload = readAgentPayload(res)
  return payload && typeof payload === 'object' && !Array.isArray(payload) ? payload : {}
}

const readAgentList = (res) => {
  const payload = readAgentPayload(res)
  if (Array.isArray(payload)) {
    return payload
  }
  if (payload && typeof payload === 'object') {
    return payload.list || payload.tasks || payload.data || []
  }
  return []
}

const formatTime = time => format.dateTime(time)

const openTerminal = (agent) => {
  selectedNodeId.value = agent.node_id
  activeTab.value = 'terminal'
}

const openTaskModal = (agent) => {
  taskTargetNode.value = agent
  taskForm.value = {
    action: '',
    service: diagnosticServices[0],
    lines: 100,
    timeout: 30
  }
  taskError.value = ''
  showTaskModal.value = true
}

const viewMonitor = (agent) => {
  toast.info(t('runtime.nodeXAgents.hints.monitor', { id: agent.node_id }))
}

const executeCommand = async () => {
  if (!selectedNodeId.value || !selectedAction.value) {
    toast.error(t('runtime.nodeXAgents.messages.selectActionFirst'))
    return
  }

  const actionValue = selectedAction.value
  const params = buildActionParams(actionValue, selectedService.value, logLines.value)

  terminalLines.value.push({
    prompt: '$ ',
    content: t(`runtime.nodeXAgents.diagnosticActions.${actionValue}`),
    type: 'input'
  })

  try {
    const res = await executeAgentCommand({
      node_id: selectedNodeId.value,
      action: actionValue,
      params,
      timeout: 30
    })

    const result = readAgentObject(res)
    terminalLines.value.push({
      prompt: '',
      content: result.output || JSON.stringify(result, null, 2),
      type: result.success ? 'output' : 'error'
    })

    await fetchTaskHistory()
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
  if (taskSending.value) return
  taskError.value = ''
  if (!taskTargetNode.value || !taskForm.value.action) {
    taskError.value = t('runtime.nodeXAgents.messages.taskIncomplete')
    return
  }

  const params = buildActionParams(taskForm.value.action, taskForm.value.service, taskForm.value.lines)

  taskSending.value = true
  try {
    await createAgentTask({
      node_id: taskTargetNode.value.node_id,
      type: 'diagnostic',
      action: taskForm.value.action,
      params,
      timeout: taskForm.value.timeout
    })

    taskSending.value = false
    showTaskModal.value = false
    toast.success(t('runtime.nodeXAgents.messages.taskSent'))
    await fetchTaskHistory()
  } catch (err) {
    taskError.value = t('runtime.nodeXAgents.messages.taskSendFailed', {
      message: err.response?.data?.error || err.message
    })
  } finally {
    taskSending.value = false
  }
}

let refreshTimer
onMounted(() => {
  fetchAgents()
  fetchTaskHistory()
  refreshTimer = setInterval(fetchAgents, 30000)
})

onUnmounted(() => {
  clearInterval(refreshTimer)
})
</script>

<style scoped>
.capability-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.cap-tag {
  padding: var(--space-0-5) var(--space-2);
  border-radius: var(--radius-xs);
  background: var(--fill-1);
  /* Stays at 4.5:1 on a hovered or selected row too. */
  color: color-mix(in srgb, var(--label-2) 70%, var(--label-1));
  font-family: var(--font-mono);
  font-size: var(--type-caption-size);
}

/* Remote terminal: a console in the page's own colours (light and dark). */
.terminal {
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  box-shadow: var(--shadow-1), 0 0 0 0.5px var(--separator);
}

.terminal__bar,
.terminal__input {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  align-items: center;
  padding: var(--space-3) var(--space-4);
}

.terminal__bar {
  justify-content: space-between;
  border-bottom: 1px solid var(--separator);
}

.terminal__node {
  width: min(280px, 100%);
}

.terminal__input {
  border-top: 1px solid var(--separator);
}

.terminal__input > :deep(*) {
  flex: 0 1 220px;
}

.terminal__input > .terminal__lines {
  flex-basis: 140px;
}

.terminal__output {
  height: 420px;
  padding: var(--space-4);
  overflow-y: auto;
  background: var(--bg-grouped);
  font-family: var(--font-mono);
  font-size: var(--type-callout-size);
  line-height: var(--type-callout-line);
}

.terminal__output:focus-visible {
  outline: var(--focus-ring);
  outline-offset: calc(-1 * var(--focus-ring-offset));
}

.terminal__hint {
  color: var(--label-2);
  font-family: var(--font-sans);
}

.terminal__line {
  margin-bottom: var(--space-1);
}

.terminal__prompt {
  color: var(--success);
}

.terminal__content {
  color: var(--label-1);
  white-space: pre-wrap;
  word-break: break-all;
}

.terminal__content.is-input {
  color: var(--accent);
}

.terminal__content.is-error {
  color: var(--danger);
}

@media (max-width: 639.98px) {
  .terminal__output {
    height: 320px;
  }

  .terminal__input > :deep(*) {
    flex: 1 1 100%;
  }
}
</style>
