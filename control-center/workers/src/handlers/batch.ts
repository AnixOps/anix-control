/**
 * Batch Operations Handler
 *
 * Provides efficient batch processing for multiple operations
 */

import type { Context } from 'hono'
import { z } from 'zod'
import type {
  ApiErrorResponse,
  BatchNodeStatusResponse,
  BatchOperationResult,
  BatchOperationsResponse,
  Env,
  SchemaValidationErrorResponse,
} from '../types'
import { logAudit } from '../utils/audit'

// Batch request schema
const batchRequestSchema = z.object({
  operations: z.array(z.object({
    method: z.enum(['GET', 'POST', 'PUT', 'DELETE']),
    path: z.string(),
    body: z.unknown().optional(),
  })).max(100), // Max 100 operations per batch
  stopOnError: z.boolean().optional().default(false),
})

type BatchOperation = z.infer<typeof batchRequestSchema>['operations'][0]

type BatchResult = BatchOperationResult

/**
 * Execute batch operations
 */
export async function batchOperationsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const { operations, stopOnError } = batchRequestSchema.parse(body)

    const results: BatchResult[] = []
    let hasError = false

    for (const op of operations) {
      if (hasError && stopOnError) {
        results.push({
          path: op.path,
          status: 0,
          success: false,
          error: 'Skipped due to previous error',
        })
        continue
      }

      try {
        const result = await executeOperation(c, op)
        results.push(result)

        if (!result.success) {
          hasError = true
        }
      } catch (err) {
        results.push({
          path: op.path,
          status: 500,
          success: false,
          error: err instanceof Error ? err.message : 'Unknown error',
        })
        hasError = true
      }
    }

    // Log batch operation
    await logAudit(c, user?.sub, 'batch_operation', 'api', {
      operations_count: operations.length,
      success_count: results.filter(r => r.success).length,
      error_count: results.filter(r => !r.success).length,
    })

    return c.json({
      success: !hasError,
      results,
      summary: {
        total: operations.length,
        successful: results.filter(r => r.success).length,
        failed: results.filter(r => !r.success).length,
      },
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({
        success: false,
        error: 'Validation error',
        details: err.issues,
      }, 400)
    }

    return c.json({
      success: false,
      error: err instanceof Error ? err.message : 'Unknown error',
    }, 500)
  }
}

/**
 * Execute a single operation
 */
async function executeOperation(
  c: Context<{ Bindings: Env }>,
  op: BatchOperation
): Promise<BatchResult> {
  const { method, path, body } = op

  // Extract resource type from path
  const pathParts = path.split('/').filter(Boolean)
  const resource = pathParts[1] || 'unknown'

  switch (resource) {
    case 'nodes':
      return executeNodeOperation(c, method, path, body)
    case 'playbooks':
      return executePlaybookOperation(c, method, path, body)
    case 'tasks':
      return executeTaskOperation(c, method, path, body)
    default:
      return {
        path,
        status: 400,
        success: false,
        error: `Unknown resource: ${resource}`,
      }
  }
}

/**
 * Node operations
 */
async function executeNodeOperation(
  c: Context<{ Bindings: Env }>,
  method: string,
  path: string,
  body: unknown
): Promise<BatchResult> {
  const pathParts = path.split('/').filter(Boolean)
  const nodeId = pathParts[2] ? parseInt(pathParts[2], 10) : null

  switch (method) {
    case 'GET':
      if (nodeId) {
        const node = await c.env.DB
          .prepare('SELECT * FROM nodes WHERE id = ?')
          .bind(nodeId)
          .first()
        return {
          path,
          status: node ? 200 : 404,
          success: !!node,
          data: node,
          error: node ? undefined : 'Node not found',
        }
      }
      const nodes = await c.env.DB
        .prepare('SELECT * FROM nodes ORDER BY created_at DESC')
        .all()
      return {
        path,
        status: 200,
        success: true,
        data: nodes.results,
      }

    case 'POST':
      if (body && typeof body === 'object') {
        const { name, host, port = 22 } = body as Record<string, unknown>
        const result = await c.env.DB
          .prepare('INSERT INTO nodes (name, host, port, status) VALUES (?, ?, ?, ?) RETURNING *')
          .bind(name, host, port, 'offline')
          .first()
        return {
          path,
          status: 201,
          success: true,
          data: result,
        }
      }
      return {
        path,
        status: 400,
        success: false,
        error: 'Invalid body',
      }

    case 'DELETE':
      if (nodeId) {
        await c.env.DB
          .prepare('DELETE FROM nodes WHERE id = ?')
          .bind(nodeId)
          .run()
        return {
          path,
          status: 200,
          success: true,
        }
      }
      return {
        path,
        status: 400,
        success: false,
        error: 'Node ID required',
      }

    default:
      return {
        path,
        status: 405,
        success: false,
        error: `Method ${method} not allowed`,
      }
  }
}

/**
 * Playbook operations
 */
async function executePlaybookOperation(
  c: Context<{ Bindings: Env }>,
  method: string,
  path: string,
  body: unknown
): Promise<BatchResult> {
  const pathParts = path.split('/').filter(Boolean)

  switch (method) {
    case 'GET':
      const playbooks = await c.env.DB
        .prepare('SELECT id, name, description, category, created_at FROM playbooks ORDER BY created_at DESC')
        .all()
      return {
        path,
        status: 200,
        success: true,
        data: playbooks.results,
      }

    default:
      return {
        path,
        status: 405,
        success: false,
        error: `Method ${method} not allowed`,
      }
  }
}

/**
 * Task operations
 */
async function executeTaskOperation(
  c: Context<{ Bindings: Env }>,
  method: string,
  path: string,
  body: unknown
): Promise<BatchResult> {
  const pathParts = path.split('/').filter(Boolean)

  switch (method) {
    case 'GET':
      const tasks = await c.env.DB
        .prepare('SELECT * FROM tasks ORDER BY created_at DESC LIMIT 50')
        .all()
      return {
        path,
        status: 200,
        success: true,
        data: tasks.results,
      }

    default:
      return {
        path,
        status: 405,
        success: false,
        error: `Method ${method} not allowed`,
      }
  }
}

/**
 * Bulk node status update
 */
export async function bulkNodeStatusHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const body = await c.req.json()

  const schema = z.object({
    node_ids: z.array(z.number()).min(1).max(100),
    action: z.enum(['start', 'stop', 'restart']),
  })

  try {
    const { node_ids, action } = schema.parse(body)

    const results = []
    for (const nodeId of node_ids) {
      const node = await c.env.DB
        .prepare('SELECT * FROM nodes WHERE id = ?')
        .bind(nodeId)
        .first()

      if (node) {
        await c.env.DB
          .prepare('UPDATE nodes SET status = ? WHERE id = ?')
          .bind(action === 'stop' ? 'stopped' : 'running', nodeId)
          .run()

        results.push({ node_id: nodeId, success: true })
      } else {
        results.push({ node_id: nodeId, success: false, error: 'Not found' })
      }
    }

    await logAudit(c, user?.sub, `bulk_${action}_nodes`, 'node', {
      node_ids,
      success_count: results.filter(r => r.success).length,
    })

    return c.json({
      success: true,
      action,
      results,
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Unknown error' }, 500)
  }
}