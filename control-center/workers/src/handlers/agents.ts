/**
 * Agent Management API Handler
 * Handles agent registration, heartbeat, and command execution
 */

import type { Context } from 'hono'
import type {
  AgentCommandResultResponse,
  AgentHeartbeatResponse,
  AgentMetricsResponse,
  AgentRegisterResponse,
  AgentSendCommandResponse,
  AgentStoreMetricsResponse,
  ApiErrorResponse,
  Env,
  SchemaValidationErrorResponse,
} from '../types'
import { z } from 'zod'
import { logAudit } from '../utils/audit'
import { buildNodeChannels, makeRealtimeEvent, publishRealtimeEvent } from '../services/realtime'

// Agent registration schema
const registerAgentSchema = z.object({
  agent_id: z.string().min(1),
  secret: z.string().min(1),
  hostname: z.string().min(1),
  os: z.string().optional(),
  arch: z.string().optional(),
  version: z.string().optional(),
  labels: z.record(z.string(), z.string()).optional(),
  cpu_count: z.number().optional(),
  memory_gb: z.number().optional(),
  disk_gb: z.number().optional(),
})

// Heartbeat schema
const heartbeatSchema = z.object({
  agent_id: z.string().min(1),
  status: z.string().optional(),
  timestamp: z.number().optional(),
})

// Metrics schema
const metricsSchema = z.object({
  agent_id: z.string().min(1),
  cpu_usage: z.number().optional(),
  memory_usage: z.number().optional(),
  memory_total: z.number().optional(),
  memory_used: z.number().optional(),
  disk_usage: z.number().optional(),
  disk_total: z.number().optional(),
  disk_used: z.number().optional(),
  load_avg_1: z.number().optional(),
  load_avg_5: z.number().optional(),
  load_avg_15: z.number().optional(),
  uptime: z.number().optional(),
  process_count: z.number().optional(),
  timestamp: z.number(),
})

/**
 * Register a new agent
 */
export async function registerAgentHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const data = registerAgentSchema.parse(body)

    // Verify agent secret (should match a node's agent_secret)
    const node = await c.env.DB
      .prepare(`
        SELECT id, name, agent_secret FROM nodes
        WHERE agent_id = ? AND agent_secret = ?
      `)
      .bind(data.agent_id, data.secret)
      .first<{ id: number; name: string; agent_secret: string }>()

    if (!node) {
      return c.json({ success: false, error: 'Invalid agent credentials' } as ApiErrorResponse, 401)
    }

    // Update node with agent info
    await c.env.DB
      .prepare(`
        UPDATE nodes SET
          status = 'online',
          last_seen = datetime('now'),
          os = ?,
          arch = ?,
          agent_version = ?,
          cpu_count = ?,
          memory_gb = ?,
          disk_gb = ?,
          updated_at = datetime('now')
        WHERE id = ?
      `)
      .bind(
        data.os || null,
        data.arch || null,
        data.version || null,
        data.cpu_count || null,
        data.memory_gb || null,
        data.disk_gb || null,
        node.id
      )
      .run()

    // Store labels in KV if provided
    if (data.labels) {
      await c.env.KV.put(`agent:labels:${data.agent_id}`, JSON.stringify(data.labels))
    }

    await logAudit(c, user?.sub, 'agent_register', 'agent', {
      agent_id: data.agent_id,
      hostname: data.hostname,
      node_id: node.id,
    })

    publishRealtimeEvent(makeRealtimeEvent(
      'agent.registered',
      'node',
      buildNodeChannels(node.id),
      {
        agent_id: data.agent_id,
        node_id: node.id,
        hostname: data.hostname,
        status: 'online',
      },
      {
        user_id: user?.sub,
        resource: {
          kind: 'agent',
          id: data.agent_id,
          name: data.hostname,
        },
      }
    ))

    return c.json({
      success: true,
      data: {
        agent_id: data.agent_id,
        node_id: node.id,
        heartbeat_interval: 30,
        metrics_interval: 60,
      },
    } as AgentRegisterResponse)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

/**
 * Agent heartbeat
 */
export async function agentHeartbeatHandler(c: Context<{ Bindings: Env }>) {
  try {
    const body = await c.req.json()
    const data = heartbeatSchema.parse(body)

    // Verify agent
    const agentId = c.req.header('X-Agent-ID')
    const agentSecret = c.req.header('X-Agent-Secret')

    if (!agentId || !agentSecret) {
      return c.json({ success: false, error: 'Missing agent credentials' } as ApiErrorResponse, 401)
    }

    // Verify agent credentials
    const node = await c.env.DB
      .prepare(`
        SELECT id, agent_secret FROM nodes
        WHERE agent_id = ?
      `)
      .bind(agentId)
      .first<{ id: number; agent_secret: string }>()

    if (!node || node.agent_secret !== agentSecret) {
      return c.json({ success: false, error: 'Invalid agent credentials' } as ApiErrorResponse, 401)
    }

    // Update node status
    await c.env.DB
      .prepare(`
        UPDATE nodes SET
          status = 'online',
          last_seen = datetime('now'),
          updated_at = datetime('now')
        WHERE id = ?
      `)
      .bind(node.id)
      .run()

    publishRealtimeEvent(makeRealtimeEvent(
      'agent.heartbeat',
      'node',
      buildNodeChannels(node.id),
      {
        agent_id: agentId,
        node_id: node.id,
        status: data.status || 'online',
        timestamp: data.timestamp,
      },
      {
        resource: {
          kind: 'agent',
          id: agentId,
        },
      }
    ))

    // Check for pending commands
    const pendingCommands = await c.env.KV.get(`agent:commands:${agentId}`, 'json') as Array<{
      id: string
      type: string
      payload: Record<string, unknown>
    }> | null

    // Clear pending commands after retrieval
    if (pendingCommands && pendingCommands.length > 0) {
      await c.env.KV.delete(`agent:commands:${agentId}`)
    }

    return c.json({
      success: true,
      data: {
        received: true,
        commands: pendingCommands || [],
      },
    } as AgentHeartbeatResponse)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

/**
 * Submit agent metrics
 */
export async function agentMetricsHandler(c: Context<{ Bindings: Env }>) {
  try {
    const body = await c.req.json()
    const data = metricsSchema.parse(body)

    // Verify agent
    const agentId = c.req.header('X-Agent-ID')
    const agentSecret = c.req.header('X-Agent-Secret')

    if (!agentId || !agentSecret) {
      return c.json({ success: false, error: 'Missing agent credentials' } as ApiErrorResponse, 401)
    }

    // Verify agent credentials
    const node = await c.env.DB
      .prepare(`SELECT id, agent_secret FROM nodes WHERE agent_id = ?`)
      .bind(agentId)
      .first<{ id: number; agent_secret: string }>()

    if (!node || node.agent_secret !== agentSecret) {
      return c.json({ success: false, error: 'Invalid agent credentials' } as ApiErrorResponse, 401)
    }

    // Store metrics in KV (keep last 24 hours)
    const metricsKey = `agent:metrics:${agentId}`
    const existingMetrics = await c.env.KV.get(metricsKey, 'json') as Array<unknown> | null

    const metricsArray = existingMetrics || []
    metricsArray.push({
      ...data,
      received_at: new Date().toISOString(),
    })

    // Keep only last 1440 entries (24 hours at 1-minute intervals)
    if (metricsArray.length > 1440) {
      metricsArray.shift()
    }

    await c.env.KV.put(metricsKey, JSON.stringify(metricsArray), {
      expirationTtl: 86400, // 24 hours
    })

    // Update latest metrics on node
    await c.env.KV.put(`agent:latest:${agentId}`, JSON.stringify(data), {
      expirationTtl: 3600,
    })

    publishRealtimeEvent(makeRealtimeEvent(
      'agent.metrics',
      'node',
      buildNodeChannels(node.id),
      {
        agent_id: agentId,
        node_id: node.id,
        cpu_usage: data.cpu_usage,
        memory_usage: data.memory_usage,
        disk_usage: data.disk_usage,
        timestamp: data.timestamp,
      },
      {
        resource: {
          kind: 'agent',
          id: agentId,
        },
      }
    ))

    return c.json({ success: true, data: { stored: true } } as AgentStoreMetricsResponse)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

/**
 * Submit command result
 */
export async function agentCommandResultHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const agentId = c.req.header('X-Agent-ID')
    const agentSecret = c.req.header('X-Agent-Secret')

    if (!agentId || !agentSecret) {
      return c.json({ success: false, error: 'Missing agent credentials' } as ApiErrorResponse, 401)
    }

    // Verify agent credentials
    const node = await c.env.DB
      .prepare(`SELECT id, agent_secret FROM nodes WHERE agent_id = ?`)
      .bind(agentId)
      .first<{ id: number; agent_secret: string }>()

    if (!node || node.agent_secret !== agentSecret) {
      return c.json({ success: false, error: 'Invalid agent credentials' } as ApiErrorResponse, 401)
    }

    const body = await c.req.json<{
      command_id: string
      success: boolean
      output?: string
      error?: string
      duration?: number
    }>()

    // Store result in KV
    await c.env.KV.put(
      `agent:result:${body.command_id}`,
      JSON.stringify({
        ...body,
        agent_id: agentId,
        node_id: node.id,
        completed_at: new Date().toISOString(),
      }),
      { expirationTtl: 86400 }
    )

    await logAudit(c, user?.sub, 'agent_command_result', 'agent', {
      agent_id: agentId,
      command_id: body.command_id,
      success: body.success,
    })

    publishRealtimeEvent(makeRealtimeEvent(
      'agent.command_result',
      'node',
      buildNodeChannels(node.id),
      {
        agent_id: agentId,
        node_id: node.id,
        command_id: body.command_id,
        success: body.success,
        output: body.output,
        error: body.error,
        duration: body.duration,
      },
      {
        user_id: user?.sub,
        resource: {
          kind: 'agent',
          id: agentId,
        },
      }
    ))

    return c.json({ success: true, data: { received: true } } as AgentCommandResultResponse)
  } catch (err) {
    throw err
  }
}

/**
 * Send command to agent
 */
export async function sendAgentCommandHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin' && user.role !== 'operator') {
    return c.json({ success: false, error: 'Forbidden' } as ApiErrorResponse, 403)
  }

  try {
    const agentId = c.req.param('agentId') as string
    const body = await c.req.json<{
      type: string
      payload?: Record<string, unknown>
      timeout?: number
    }>()

    if (!agentId) {
      return c.json({ success: false, error: 'Agent ID is required' } as ApiErrorResponse, 400)
    }

    // Check if agent exists
    const node = await c.env.DB
      .prepare(`SELECT id, status FROM nodes WHERE agent_id = ?`)
      .bind(agentId)
      .first<{ id: number; status: string }>()

    if (!node) {
      return c.json({ success: false, error: 'Agent not found' } as ApiErrorResponse, 404)
    }

    // Generate command ID
    const commandId = `cmd-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`

    // Queue command in KV
    const pendingCommands = (await c.env.KV.get(`agent:commands:${agentId}`, 'json')) as Array<unknown> | null
    const commands = pendingCommands || []

    commands.push({
      id: commandId,
      type: body.type,
      payload: body.payload || {},
      timeout: body.timeout || 300,
      created_at: new Date().toISOString(),
    })

    await c.env.KV.put(`agent:commands:${agentId}`, JSON.stringify(commands), {
      expirationTtl: 3600,
    })

    await logAudit(c, user.sub, 'send_agent_command', 'agent', {
      agent_id: agentId,
      command_id: commandId,
      command_type: body.type,
    })

    publishRealtimeEvent(makeRealtimeEvent(
      'agent.command_queued',
      'node',
      buildNodeChannels(node.id),
      {
        agent_id: agentId,
        node_id: node.id,
        command_id: commandId,
        type: body.type,
        status: 'queued',
      },
      {
        user_id: user.sub,
        resource: {
          kind: 'agent',
          id: agentId,
        },
      }
    ))

    return c.json({
      success: true,
      data: {
        command_id: commandId,
        status: 'queued',
      },
    } as AgentSendCommandResponse)
  } catch (err) {
    throw err
  }
}

/**
 * Get agent metrics
 */
export async function getAgentMetricsHandler(c: Context<{ Bindings: Env }>) {
  const agentId = c.req.param('agentId') as string

  if (!agentId) {
    return c.json({ success: false, error: 'Agent ID is required' } as ApiErrorResponse, 400)
  }

  const range = c.req.query('range') || '1h'

  // Get metrics from KV
  const metrics = await c.env.KV.get(`agent:metrics:${agentId}`, 'json')

  if (!metrics) {
    return c.json({
      success: true,
      data: {
        agent_id: agentId,
        metrics: [],
      },
    } as AgentMetricsResponse)
  }

  // Filter by time range
  const metricsArray = metrics as Array<{ timestamp: number }>
  const now = Date.now()
  let cutoff: number

  switch (range) {
    case '15m':
      cutoff = now - 15 * 60 * 1000
      break
    case '1h':
      cutoff = now - 60 * 60 * 1000
      break
    case '6h':
      cutoff = now - 6 * 60 * 60 * 1000
      break
    case '24h':
      cutoff = now - 24 * 60 * 60 * 1000
      break
    default:
      cutoff = now - 60 * 60 * 1000
  }

  const filteredMetrics = metricsArray.filter(m => m.timestamp * 1000 >= cutoff)

  return c.json({
    success: true,
    data: {
      agent_id: agentId,
      range,
      metrics: filteredMetrics,
    },
  })
}

/**
 * Generate agent installation script
 */
export async function generateInstallScriptHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  if (user.role !== 'admin' && user.role !== 'operator') {
    return c.json({ success: false, error: 'Forbidden' } as ApiErrorResponse, 403)
  }

  const nodeId = c.req.param('nodeId') as string

  if (!nodeId) {
    return c.json({ success: false, error: 'Node ID is required' } as ApiErrorResponse, 400)
  }

  // Get node info
  const node = await c.env.DB
    .prepare('SELECT id, name, agent_id, agent_secret FROM nodes WHERE id = ?')
    .bind(nodeId)
    .first<{ id: number; name: string; agent_id: string | null; agent_secret: string | null }>()

  if (!node) {
    return c.json({ success: false, error: 'Node not found' } as ApiErrorResponse, 404)
  }

  // Generate agent ID and secret if not exists
  let agentId = node.agent_id
  let agentSecret = node.agent_secret

  if (!agentId) {
    agentId = `agent-${node.id}-${Math.random().toString(36).substr(2, 8)}`
  }
  if (!agentSecret) {
    agentSecret = Array.from({ length: 32 }, () =>
      Math.random().toString(36).charAt(2)
    ).join('')
  }

  // Update node with agent credentials
  await c.env.DB
    .prepare(`
      UPDATE nodes SET
        agent_id = ?,
        agent_secret = ?,
        updated_at = datetime('now')
      WHERE id = ?
    `)
    .bind(agentId, agentSecret, node.id)
    .run()

  const apiURL = 'https://anixops-api-v2.kalijerry.workers.dev'

  const script = `#!/bin/bash
# AnixOps Agent Installation Script
# Node: ${node.name}

set -e

AGENT_VERSION="1.0.0"
AGENT_ID="${agentId}"
AGENT_SECRET="${agentSecret}"
API_URL="${apiURL}"

echo "Installing AnixOps Agent..."

# Download agent binary
curl -sL "$API_URL/downloads/agent/linux/amd64/$AGENT_VERSION" -o /usr/local/bin/anixops-agent
chmod +x /usr/local/bin/anixops-agent

# Create config directory
mkdir -p /etc/anixops

# Create config file
cat > /etc/anixops/config.json <<EOF
{
  "server_url": "$API_URL",
  "agent_id": "$AGENT_ID",
  "secret_key": "$AGENT_SECRET",
  "hostname": "$(hostname)",
  "heartbeat_interval": 30,
  "metrics_interval": 60,
  "log_level": "info"
}
EOF

# Create systemd service
cat > /etc/systemd/system/anixops-agent.service <<EOF
[Unit]
Description=AnixOps Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/anixops-agent -config /etc/anixops/config.json
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Enable and start service
systemctl daemon-reload
systemctl enable anixops-agent
systemctl start anixops-agent

echo "AnixOps Agent installed successfully!"
echo "Agent ID: $AGENT_ID"
echo "Check status: systemctl status anixops-agent"
`

  return new Response(script, {
    headers: {
      'Content-Type': 'text/plain',
      'Content-Disposition': `attachment; filename="install-agent-${node.id}.sh"`,
    },
  })
}