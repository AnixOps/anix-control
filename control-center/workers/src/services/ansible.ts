/**
 * Ansible Execution Service
 *
 * This service handles playbook execution through the Agent system.
 * Since Cloudflare Workers cannot directly execute Ansible, we delegate
 * execution to the Go-based Agent running on target nodes.
 */

import type { Env } from '../types'
import { buildTaskChannels, makeRealtimeEvent, publishRealtimeEvent } from '../services/realtime'

// Execution status
export type ExecutionStatus = 'pending' | 'running' | 'success' | 'failed' | 'cancelled' | 'timeout'

// Execution result for a single node
export interface NodeExecutionResult {
  node_id: number
  node_name: string
  status: ExecutionStatus
  started_at?: string
  completed_at?: string
  exit_code?: number
  stdout?: string
  stderr?: string
  error?: string
}

// Full execution result
export interface ExecutionResult {
  task_id: string
  playbook_name: string
  status: ExecutionStatus
  total_nodes: number
  successful_nodes: number
  failed_nodes: number
  started_at: string
  completed_at?: string
  duration_ms?: number
  node_results: NodeExecutionResult[]
  summary?: string
}

// Execution options
export interface ExecutionOptions {
  timeout_seconds?: number
  retry_count?: number
  retry_delay_seconds?: number
  verbose?: boolean
  check_mode?: boolean // Ansible --check (dry-run)
  diff_mode?: boolean // Ansible --diff
  extra_vars?: Record<string, unknown>
}

// Task queue item
export interface TaskQueueItem {
  task_id: string
  playbook_id: number
  playbook_name: string
  storage_key: string
  nodes: Array<{ id: number; name: string; host: string }>
  variables: Record<string, unknown>
  triggered_by: number
  options?: ExecutionOptions
  created_at: string
}

// Playbook parsed structure
export interface ParsedPlaybook {
  name: string
  hosts: string
  become: boolean
  vars?: Record<string, unknown>
  tasks: Array<{
    name?: string
    [key: string]: unknown
  }>
}

/**
 * Parse YAML playbook content (simplified parser)
 * Note: Full YAML parsing would require a library, this is a basic implementation
 */
export function parsePlaybook(content: string): ParsedPlaybook | null {
  try {
    // Basic YAML parsing for Ansible playbooks
    const lines = content.split('\n')
    const playbook: ParsedPlaybook = {
      name: '',
      hosts: 'all',
      become: false,
      tasks: [],
    }

    let currentSection = ''
    let currentTask: Record<string, unknown> | null = null
    let inTasks = false
    let playNameSet = false

    for (const line of lines) {
      // Skip comments and empty lines
      if (line.trim().startsWith('#') || line.trim() === '') continue

      const trimmedLine = line.trim()

      // Detect play-level properties (before tasks section)
      if (!inTasks) {
        if (trimmedLine.startsWith('- name:') && !playNameSet) {
          playbook.name = trimmedLine.substring(7).trim().replace(/"/g, '').replace(/'/g, '')
          playNameSet = true
        } else if (trimmedLine.startsWith('hosts:')) {
          playbook.hosts = trimmedLine.substring(6).trim()
        } else if (trimmedLine.startsWith('become:')) {
          const value = trimmedLine.substring(7).trim()
          playbook.become = value === 'yes' || value === 'true'
        } else if (trimmedLine === 'vars:') {
          currentSection = 'vars'
          playbook.vars = playbook.vars || {}
        }
      }

      // Detect tasks section
      if (trimmedLine === 'tasks:') {
        inTasks = true
        currentSection = 'tasks'
      } else if (inTasks) {
        if (trimmedLine.startsWith('- name:')) {
          // New task
          if (currentTask) {
            playbook.tasks.push(currentTask)
          }
          currentTask = {
            name: trimmedLine.substring(7).trim().replace(/"/g, '').replace(/'/g, ''),
          }
        } else if (currentTask && line.includes(':') && !trimmedLine.startsWith('-')) {
          // Task property
          const colonIndex = line.indexOf(':')
          if (colonIndex > 0) {
            const key = line.substring(0, colonIndex).trim()
            const value = line.substring(colonIndex + 1).trim()
            currentTask[key] = parseValue(value)
          }
        }
      }

      // Variables section
      if (currentSection === 'vars' && !inTasks && !trimmedLine.startsWith('-') && line.includes(':')) {
        const colonIndex = line.indexOf(':')
        if (colonIndex > 0) {
          const key = line.substring(0, colonIndex).trim()
          const value = line.substring(colonIndex + 1).trim()
          playbook.vars![key] = parseValue(value)
        }
      }
    }

    // Add last task
    if (currentTask) {
      playbook.tasks.push(currentTask)
    }

    return playbook
  } catch (error) {
    console.error('Failed to parse playbook:', error)
    return null
  }
}

/**
 * Parse a YAML value
 */
export function parseValue(value: string): unknown {
  // Remove quotes
  value = value.replace(/^["']|["']$/g, '')

  // Boolean
  if (value === 'yes' || value === 'true') return true
  if (value === 'no' || value === 'false') return false

  // Number
  const num = Number(value)
  if (!isNaN(num)) return num

  // Array (simple)
  if (value.startsWith('[') && value.endsWith(']')) {
    return value
      .slice(1, -1)
      .split(',')
      .map(v => parseValue(v.trim()))
  }

  // String
  return value
}

/**
 * Generate shell commands to execute a playbook
 */
export function generateExecutionCommands(
  playbookContent: string,
  playbookName: string,
  options: ExecutionOptions = {}
): string[] {
  const commands: string[] = []

  // Create temporary playbook file
  const playbookPath = `/tmp/anixops-${playbookName}-${Date.now()}.yml`
  commands.push(`cat > ${playbookPath} << 'EOFPLAYBOOK'\n${playbookContent}\nEOFPLAYBOOK`)

  // Build ansible-playbook command
  const ansibleCmd = ['ansible-playbook', playbookPath]

  // Add options
  if (options.check_mode) {
    ansibleCmd.push('--check')
  }
  if (options.diff_mode) {
    ansibleCmd.push('--diff')
  }
  if (options.verbose) {
    ansibleCmd.push('-v')
  }

  // Add extra variables
  if (options.extra_vars && Object.keys(options.extra_vars).length > 0) {
    const varsJson = JSON.stringify(options.extra_vars)
    ansibleCmd.push(`--extra-vars '${varsJson}'`)
  }

  // Add local connection for agent execution
  ansibleCmd.push('-i', 'localhost,')

  commands.push(ansibleCmd.join(' '))

  // Cleanup
  commands.push(`rm -f ${playbookPath}`)

  return commands
}

/**
 * Execute playbook on a single node via Agent
 */
export async function executePlaybookOnNode(
  env: Env,
  taskId: string,
  nodeId: number,
  nodeName: string,
  playbookContent: string,
  playbookName: string,
  variables: Record<string, unknown>,
  options: ExecutionOptions = {}
): Promise<NodeExecutionResult> {
  const result: NodeExecutionResult = {
    node_id: nodeId,
    node_name: nodeName,
    status: 'pending',
  }

  try {
    // Get node's agent info
    const node = await env.DB
      .prepare('SELECT agent_id, agent_secret, host FROM nodes WHERE id = ?')
      .bind(nodeId)
      .first<{ agent_id: string; agent_secret: string; host: string }>()

    if (!node || !node.agent_id) {
      result.status = 'failed'
      result.error = 'Node has no registered agent'
      return result
    }

    // Generate execution commands
    const commands = generateExecutionCommands(playbookContent, playbookName, {
      ...options,
      extra_vars: { ...variables, ...options.extra_vars },
    })

    // Send command to agent via KV (agent will pick it up)
    const commandId = crypto.randomUUID()
    const commandPayload = {
      command_id: commandId,
      task_id: taskId,
      type: 'execute_playbook',
      playbook_name: playbookName,
      commands,
      timeout: options.timeout_seconds || 3600,
      created_at: new Date().toISOString(),
    }

    // Store command for agent to fetch
    await env.KV.put(
      `agent:command:${node.agent_id}:${commandId}`,
      JSON.stringify(commandPayload),
      { expirationTtl: 3600 }
    )

    // Notify agent via heartbeat channel
    await env.KV.put(
      `agent:pending:${node.agent_id}`,
      JSON.stringify({ command_id: commandId, task_id: taskId }),
      { expirationTtl: 3600 }
    )

    result.status = 'running'
    result.started_at = new Date().toISOString()

    // Log execution start
    await logExecutionEvent(env, taskId, nodeId, nodeName, 'info',
      `Starting playbook execution: ${playbookName}`)

    return result
  } catch (error) {
    result.status = 'failed'
    result.error = error instanceof Error ? error.message : 'Unknown error'
    return result
  }
}

/**
 * Process task from queue
 */
export async function processTaskQueue(env: Env): Promise<void> {
  // Get pending tasks from KV
  const list = await env.KV.list({ prefix: 'task:queue:' })

  for (const key of list.keys) {
    const taskData = await env.KV.get(key.name, 'json') as TaskQueueItem | null
    if (!taskData) continue

    try {
      // Get playbook content from R2
      const object = await env.R2.get(taskData.storage_key)
      if (!object) {
        await updateTaskStatus(env, taskData.task_id, 'failed', undefined,
          'Playbook content not found')
        await env.KV.delete(key.name)
        continue
      }

      const playbookContent = await object.text()

      // Update task status to running
      await updateTaskStatus(env, taskData.task_id, 'running')
      publishRealtimeEvent(makeRealtimeEvent(
        'task.running',
        'task',
        buildTaskChannels(taskData.task_id, taskData.triggered_by),
        {
          task_id: taskData.task_id,
          status: 'running',
          playbook_name: taskData.playbook_name,
        },
        {
          user_id: taskData.triggered_by,
        }
      ))

      // Execute on all target nodes
      const nodeResults: NodeExecutionResult[] = []
      const options: ExecutionOptions = taskData.options || {}

      for (const node of taskData.nodes) {
        const result = await executePlaybookOnNode(
          env,
          taskData.task_id,
          node.id,
          node.name,
          playbookContent,
          taskData.playbook_name,
          taskData.variables,
          options
        )
        nodeResults.push(result)
      }

      // Determine overall status
      const failedCount = nodeResults.filter(r => r.status === 'failed').length
      const successCount = nodeResults.filter(r => r.status === 'success').length

      let overallStatus: ExecutionStatus = 'running'
      if (failedCount === nodeResults.length) {
        overallStatus = 'failed'
      } else if (successCount === nodeResults.length) {
        overallStatus = 'success'
      } else if (successCount > 0) {
        overallStatus = 'success' // Partial success
      }

      await updateTaskStatus(env, taskData.task_id, overallStatus, {
        total_nodes: nodeResults.length,
        successful_nodes: successCount,
        failed_nodes: failedCount,
        node_results: nodeResults,
        summary: `${successCount}/${nodeResults.length} nodes completed successfully`,
      })
      publishRealtimeEvent(makeRealtimeEvent(
        `task.${overallStatus}`,
        'task',
        buildTaskChannels(taskData.task_id, taskData.triggered_by),
        {
          task_id: taskData.task_id,
          status: overallStatus,
          playbook_name: taskData.playbook_name,
          total_nodes: nodeResults.length,
          successful_nodes: successCount,
          failed_nodes: failedCount,
        },
        {
          user_id: taskData.triggered_by,
        }
      ))

      // Remove from queue
      await env.KV.delete(key.name)

    } catch (error) {
      console.error(`Failed to process task ${taskData.task_id}:`, error)
      await updateTaskStatus(
        env,
        taskData.task_id,
        'failed',
        undefined,
        error instanceof Error ? error.message : 'Unknown error'
      )
      await env.KV.delete(key.name)
    }
  }
}

/**
 * Get execution result
 */
export async function getExecutionResult(
  env: Env,
  taskId: string
): Promise<ExecutionResult | null> {
  const result = await env.KV.get(`task:result:${taskId}`, 'json')
  return result as ExecutionResult | null
}

/**
 * Log execution event
 */
export async function logExecutionEvent(
  env: Env,
  taskId: string,
  nodeId: number | undefined,
  nodeName: string | undefined,
  level: 'debug' | 'info' | 'warning' | 'error',
  message: string,
  metadata?: Record<string, unknown>
): Promise<void> {
  await env.DB
    .prepare(`
      INSERT INTO task_logs (task_id, node_id, node_name, level, message, metadata)
      VALUES (?, ?, ?, ?, ?, ?)
    `)
    .bind(
      taskId,
      nodeId || null,
      nodeName || null,
      level,
      message,
      metadata ? JSON.stringify(metadata) : null
    )
    .run()
}

/**
 * Update task status
 */
export async function updateTaskStatus(
  env: Env,
  taskId: string,
  status: ExecutionStatus,
  result?: Record<string, unknown>,
  error?: string
): Promise<void> {
  const updates: string[] = ['status = ?']
  const values: (string | null)[] = [status]

  if (status === 'running') {
    updates.push("started_at = datetime('now')")
  } else if (['success', 'failed', 'cancelled', 'timeout'].includes(status)) {
    updates.push("completed_at = datetime('now')")
  }

  if (result) {
    updates.push('result = ?')
    values.push(JSON.stringify(result))
  }

  if (error) {
    updates.push('error = ?')
    values.push(error)
  }

  values.push(taskId)

  await env.DB
    .prepare(`UPDATE tasks SET ${updates.join(', ')} WHERE task_id = ?`)
    .bind(...values)
    .run()
}

/**
 * Retry failed task
 */
export async function retryFailedTask(
  env: Env,
  taskId: string,
  maxRetries: number = 3
): Promise<boolean> {
  const task = await env.DB
    .prepare(`
      SELECT t.*, p.storage_key
      FROM tasks t
      LEFT JOIN playbooks p ON t.playbook_id = p.id
      WHERE t.task_id = ?
    `)
    .bind(taskId)
    .first<{
      task_id: string
      playbook_id: number
      playbook_name: string
      target_nodes: string
      variables: string
      storage_key: string
    }>()

  if (!task || !task.storage_key) return false

  const targetNodes = JSON.parse(task.target_nodes || '[]')
  const variables = JSON.parse(task.variables || '{}')

  // Create new queue item
  const queueItem: TaskQueueItem = {
    task_id: taskId,
    playbook_id: task.playbook_id,
    playbook_name: task.playbook_name,
    storage_key: task.storage_key,
    nodes: targetNodes,
    variables,
    triggered_by: 0, // System retry
    options: {
      retry_count: maxRetries,
    },
    created_at: new Date().toISOString(),
  }

  await env.KV.put(`task:queue:${taskId}`, JSON.stringify(queueItem), {
    expirationTtl: 86400,
  })

  // Reset task status
  await updateTaskStatus(env, taskId, 'pending')

  return true
}

/**
 * Cancel running task
 */
export async function cancelTask(
  env: Env,
  taskId: string
): Promise<boolean> {
  // Remove from queue
  await env.KV.delete(`task:queue:${taskId}`)

  // Update status
  await updateTaskStatus(env, taskId, 'cancelled')

  // Notify agents to cancel
  const task = await env.DB
    .prepare(`
      SELECT t.target_nodes
      FROM tasks t
      WHERE t.task_id = ?
    `)
    .bind(taskId)
    .first<{ target_nodes: string }>()

  if (task?.target_nodes) {
    const nodes = JSON.parse(task.target_nodes) as Array<{ id: number; name: string }>
    for (const node of nodes) {
      // Get node's agent
      const nodeData = await env.DB
        .prepare('SELECT agent_id FROM nodes WHERE id = ?')
        .bind(node.id)
        .first<{ agent_id: string }>()

      if (nodeData?.agent_id) {
        await env.KV.put(
          `agent:cancel:${nodeData.agent_id}`,
          JSON.stringify({ task_id: taskId, reason: 'user_cancelled' }),
          { expirationTtl: 3600 }
        )
      }
    }
  }

  return true
}