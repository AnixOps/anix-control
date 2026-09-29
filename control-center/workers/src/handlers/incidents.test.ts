import { describe, it, expect } from 'vitest'
import app from '../index'
import type { AuthLoginResponse, Env } from '../types'
import { createMockD1, createMockKV, createMockR2 } from '../../test/setup'

function createEnv(): Env {
  return {
    ENVIRONMENT: 'development',
    JWT_SECRET: 'incident-test-secret-key-min-32-characters!',
    JWT_EXPIRE: '3600',
    API_KEY_SALT: 'incident-test-salt',
    DB: createMockD1(),
    KV: createMockKV(),
    R2: createMockR2(),
    AI: {
      run: async () => ({
        response: JSON.stringify({
          summary: 'Investigated incident',
          severity: 'medium',
          likely_cause: 'Synthetic test condition',
          recommended_actions: ['Review bounded remediation'],
        }),
      }),
    } as Env['AI'],
  }
}

async function registerAndLogin(env: Env, email: string, password: string, role: 'admin' | 'operator' | 'viewer' = 'admin') {
  await app.request('/api/v1/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password, role }),
  }, env)

  const loginRes = await app.request('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  }, env)

  const loginData = await loginRes.json() as AuthLoginResponse
  return loginData.data?.access_token || ''
}

describe('incident handlers', () => {
  it('creates and executes a restart_deployment incident', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-restart@example.com', 'IncidentRestart123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Restart deployment incident',
        source: 'test-suite',
        severity: 'high',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
        evidence: [{ type: 'manual', source: 'test', content: 'Restart deployment' }],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }

    await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    const executeRes = await app.request(`/api/v1/incidents/${createData.data?.id}/execute`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(executeRes.status).toBe(200)
    const executeData = await executeRes.json() as { success: boolean; data?: { status: string; execution_result?: { restarted?: boolean } } }
    expect(executeData.success).toBe(true)
    expect(executeData.data?.status).toBe('resolved')
    expect(executeData.data?.execution_result?.success).toBe(true)
    expect(executeData.data?.execution_result?.backend).toBe('kubernetes')
  })

  it('automatically enriches evidence for restart_deployment incidents', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-evidence@example.com', 'IncidentEvidence123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Evidence enrichment incident',
        source: 'test-suite',
        severity: 'medium',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
        evidence: [{ type: 'manual', source: 'test', content: 'Manual evidence' }],
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { data?: { evidence?: Array<{ source: string }> } }
    const evidence = createData.data?.evidence || []

    expect(evidence.length).toBeGreaterThan(1)
    expect(evidence.some(item => item.source === 'kubernetes.deployment')).toBe(true)
  })

  it('automatically enriches evidence for task references', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-task-evidence@example.com', 'IncidentTaskEvidence123!', 'admin')

    await env.DB
      .prepare(`
        INSERT INTO tasks (task_id, playbook_id, playbook_name, status, trigger_type, triggered_by, target_nodes, variables)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      `)
      .bind('task-evidence-1', 1, 'deploy-app', 'failed', 'manual', 1, '[]', '{}')
      .run()

    await env.DB
      .prepare(`
        INSERT INTO task_logs (task_id, node_id, node_name, level, message, metadata)
        VALUES (?, ?, ?, ?, ?, ?)
      `)
      .bind('task-evidence-1', null, 'node-a', 'error', 'Deployment failed', JSON.stringify({ reason: 'timeout' }))
      .run()

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Task evidence incident',
        source: 'task',
        severity: 'medium',
        action_ref: 'task:task-evidence-1',
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { data?: { evidence?: Array<{ source: string }> } }
    const evidence = createData.data?.evidence || []
    expect(evidence.some(item => item.source === 'tasks.record')).toBe(true)
    expect(evidence.some(item => item.source === 'tasks.log')).toBe(true)
  })

  it('automatically enriches evidence for node references', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-node-evidence@example.com', 'IncidentNodeEvidence123!', 'admin')

    const node = await env.DB
      .prepare(`
        INSERT INTO nodes (name, host, port, status, config)
        VALUES (?, ?, ?, 'offline', ?)
        RETURNING *
      `)
      .bind('node-evidence', '10.0.0.10', 22, null)
      .first<{ id: number }>()

    await env.KV.put(`agent:latest:${node?.id}`, JSON.stringify({ cpu_usage: 91, memory_usage: 84 }))

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Node evidence incident',
        source: 'node',
        severity: 'medium',
        action_ref: `node:${node?.id}`,
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { data?: { evidence?: Array<{ source: string }> } }
    const evidence = createData.data?.evidence || []
    expect(evidence.some(item => item.source === 'nodes.record')).toBe(true)
    expect(evidence.some(item => item.source === 'nodes.latest_metrics')).toBe(true)
  })

  it('returns standardized execution result for restart incidents', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-exec-standard@example.com', 'IncidentExec123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Standardized execution incident',
        source: 'alerts.rollout',
        severity: 'medium',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    const executeRes = await app.request(`/api/v1/incidents/${createData.data?.id}/execute`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(executeRes.status).toBe(200)
    const executeData = await executeRes.json() as {
      data?: { execution_result?: { backend: string; success: boolean; operation: string; target?: { kind: string } } }
    }
    expect(executeData.data?.execution_result?.backend).toBe('kubernetes')
    expect(executeData.data?.execution_result?.success).toBe(true)
    expect(executeData.data?.execution_result?.operation).toBe('restart_deployment')
    expect(executeData.data?.execution_result?.target?.kind).toBe('deployment')
  })

  it('adds structured links for incident resources', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-links@example.com', 'IncidentLinks123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Linked incident',
        source: 'task',
        severity: 'medium',
        action_ref: 'task:task-link-1',
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { data?: { links?: Array<{ kind: string; href?: string }> } }
    expect((createData.data?.links || []).some((link) => link.kind === 'task')).toBe(true)
    expect((createData.data?.links || []).some((link) => link.href?.includes('/api/v1/tasks/'))).toBe(true)
  })

  it('blocks operator approval for critical incidents', async () => {
    const env = createEnv()
    const adminToken = await registerAndLogin(env, 'incident-admin@example.com', 'IncidentAdmin123!', 'admin')
    const operatorToken = await registerAndLogin(env, 'incident-operator@example.com', 'IncidentOperator123!', 'operator')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        title: 'Critical restart incident',
        source: 'test-suite',
        severity: 'critical',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(approveRes.status).toBe(403)
  })

  it('blocks operator approval for scale_policy incidents', async () => {
    const env = createEnv()
    const adminToken = await registerAndLogin(env, 'incident-admin2@example.com', 'IncidentAdmin123!', 'admin')
    const operatorToken = await registerAndLogin(env, 'incident-operator2@example.com', 'IncidentOperator123!', 'operator')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${adminToken}`,
      },
      body: JSON.stringify({
        title: 'Scale policy incident',
        source: 'test-suite',
        severity: 'medium',
        action_type: 'scale_policy',
        action_ref: 'policy-1',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const approveRes = await app.request(`/api/v1/incidents/${createData.data?.id}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${operatorToken}` },
    }, env)

    expect(approveRes.status).toBe(403)
  })

  it('returns incident timeline with all events', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-timeline@example.com', 'IncidentTimeline123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Timeline test incident',
        source: 'test-suite',
        severity: 'high',
        action_type: 'restart_deployment',
        action_ref: 'default/anixops-api',
        evidence: [{ type: 'manual', source: 'test', content: 'Initial evidence' }],
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const incidentId = createData.data?.id || ''

    // Get initial timeline (should have created event)
    const initialTimelineRes = await app.request(`/api/v1/incidents/${incidentId}/timeline`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(initialTimelineRes.status).toBe(200)
    const initialTimeline = await initialTimelineRes.json() as { success: boolean; data?: { events?: Array<{ type: string }> } }
    expect(initialTimeline.success).toBe(true)
    expect(initialTimeline.data?.events?.some(e => e.type === 'created')).toBe(true)

    // Approve and execute
    await app.request(`/api/v1/incidents/${incidentId}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    await app.request(`/api/v1/incidents/${incidentId}/execute`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    // Get final timeline (should have all events)
    const finalTimelineRes = await app.request(`/api/v1/incidents/${incidentId}/timeline`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(finalTimelineRes.status).toBe(200)
    const finalTimeline = await finalTimelineRes.json() as {
      success: boolean
      data?: {
        incident_id: string
        events?: Array<{ type: string; summary: string }>
        total_events: number
      }
    }
    expect(finalTimeline.success).toBe(true)
    expect(finalTimeline.data?.incident_id).toBe(incidentId)
    expect(finalTimeline.data?.total_events).toBeGreaterThan(1)

    const eventTypes = finalTimeline.data?.events?.map(e => e.type) || []
    expect(eventTypes).toContain('created')
    expect(eventTypes).toContain('approved')
    expect(eventTypes).toContain('resolved')
  })

  it('adds, lists, updates, and deletes incident comments', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-comment@example.com', 'IncidentComment123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Comment test incident',
        source: 'test-suite',
        severity: 'medium',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const incidentId = createData.data?.id || ''

    // Add a comment
    const addCommentRes = await app.request(`/api/v1/incidents/${incidentId}/comments`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        content: 'This is a test comment',
        visibility: 'public',
      }),
    }, env)

    expect(addCommentRes.status).toBe(201)
    const addCommentData = await addCommentRes.json() as { success: boolean; data?: { id: string; content: string } }
    expect(addCommentData.success).toBe(true)
    expect(addCommentData.data?.content).toBe('This is a test comment')
    const commentId = addCommentData.data?.id || ''

    // List comments
    const listCommentsRes = await app.request(`/api/v1/incidents/${incidentId}/comments`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(listCommentsRes.status).toBe(200)
    const listCommentsData = await listCommentsRes.json() as { success: boolean; data?: Array<{ id: string }> }
    expect(listCommentsData.success).toBe(true)
    expect(listCommentsData.data?.length).toBe(1)
    expect(listCommentsData.data?.[0]?.id).toBe(commentId)

    // Update comment
    const updateCommentRes = await app.request(`/api/v1/incidents/${incidentId}/comments/${commentId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        content: 'Updated comment content',
      }),
    }, env)

    expect(updateCommentRes.status).toBe(200)
    const updateCommentData = await updateCommentRes.json() as { success: boolean; data?: { content: string } }
    expect(updateCommentData.success).toBe(true)
    expect(updateCommentData.data?.content).toBe('Updated comment content')

    // Delete comment
    const deleteCommentRes = await app.request(`/api/v1/incidents/${incidentId}/comments/${commentId}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(deleteCommentRes.status).toBe(200)

    // Verify deleted
    const listAfterDeleteRes = await app.request(`/api/v1/incidents/${incidentId}/comments`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    const listAfterDeleteData = await listAfterDeleteRes.json() as { success: boolean; data?: Array<{ id: string }> }
    expect(listAfterDeleteData.data?.length).toBe(0)
  })

  it('denies comment update from non-author', async () => {
    const env = createEnv()
    const authorToken = await registerAndLogin(env, 'comment-author@example.com', 'CommentAuthor123!', 'admin')
    const otherToken = await registerAndLogin(env, 'comment-other@example.com', 'CommentOther123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authorToken}`,
      },
      body: JSON.stringify({
        title: 'Comment permission test',
        source: 'test',
        severity: 'medium',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const incidentId = createData.data?.id || ''

    const addCommentRes = await app.request(`/api/v1/incidents/${incidentId}/comments`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authorToken}`,
      },
      body: JSON.stringify({
        content: 'Comment by author',
      }),
    }, env)

    const addCommentData = await addCommentRes.json() as { data?: { id: string } }
    const commentId = addCommentData.data?.id || ''

    // Try to update with different user
    const updateRes = await app.request(`/api/v1/incidents/${incidentId}/comments/${commentId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${otherToken}`,
      },
      body: JSON.stringify({
        content: 'Attempted update',
      }),
    }, env)

    expect(updateRes.status).toBe(404)
  })

  it('returns incident statistics', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'incident-stats@example.com', 'IncidentStats123!', 'admin')

    // Create a few incidents with different severities and statuses
    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Low severity incident',
        source: 'test-1',
        severity: 'low',
      }),
    }, env)

    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Critical incident',
        source: 'test-2',
        severity: 'critical',
        action_type: 'restart_deployment',
        action_ref: 'default/app',
      }),
    }, env)

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Medium incident to resolve',
        source: 'test-3',
        severity: 'medium',
        action_type: 'restart_deployment',
        action_ref: 'default/app',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const incidentId = createData.data?.id || ''

    // Approve and execute to get a resolved incident
    await app.request(`/api/v1/incidents/${incidentId}/approve`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    await app.request(`/api/v1/incidents/${incidentId}/execute`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    // Get statistics
    const statsRes = await app.request('/api/v1/incidents/statistics', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(statsRes.status).toBe(200)
    const statsData = await statsRes.json() as {
      success: boolean
      data?: {
        total: number
        by_status: Record<string, number>
        by_severity: Record<string, number>
        by_action_type: Record<string, number>
        trends: { daily: Array<{ date: string }>; weekly: Array<{ week: string }> }
        action_success_rate: { total_executed: number; success_rate: number }
      }
    }
    expect(statsData.success).toBe(true)
    expect(statsData.data?.total).toBeGreaterThanOrEqual(3)
    expect(statsData.data?.by_status?.open).toBeGreaterThanOrEqual(2)
    expect(statsData.data?.by_status?.resolved).toBeGreaterThanOrEqual(1)
    expect(statsData.data?.by_severity?.low).toBeGreaterThanOrEqual(1)
    expect(statsData.data?.by_severity?.critical).toBeGreaterThanOrEqual(1)
    expect(statsData.data?.by_action_type?.restart_deployment).toBeGreaterThanOrEqual(2)
    expect(statsData.data?.trends?.daily?.length).toBe(30)
    expect(statsData.data?.trends?.weekly?.length).toBe(12)
    expect(statsData.data?.action_success_rate?.total_executed).toBeGreaterThanOrEqual(1)
    expect(statsData.data?.action_success_rate?.success_rate).toBeGreaterThanOrEqual(0)
  })

  it('performs bulk approve operations', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'bulk-approve@example.com', 'BulkApprove123!', 'admin')

    // Create multiple incidents
    const incidentIds: string[] = []
    for (let i = 0; i < 3; i++) {
      const createRes = await app.request('/api/v1/incidents', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          title: `Bulk test incident ${i}`,
          source: 'test',
          severity: 'medium',
          action_type: 'restart_deployment',
          action_ref: 'default/app',
        }),
      }, env)
      const data = await createRes.json() as { data?: { id: string } }
      incidentIds.push(data.data?.id || '')
    }

    // Bulk approve
    const bulkRes = await app.request('/api/v1/incidents/bulk/approve', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ incident_ids: incidentIds }),
    }, env)

    expect(bulkRes.status).toBe(200)
    const bulkData = await bulkRes.json() as {
      success: boolean
      data?: { total: number; successful: string[]; failed: Array<{ id: string }> }
    }
    expect(bulkData.success).toBe(true)
    expect(bulkData.data?.total).toBe(3)
    expect(bulkData.data?.successful.length).toBe(3)
    expect(bulkData.data?.failed.length).toBe(0)
  })

  it('performs bulk execute operations', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'bulk-execute@example.com', 'BulkExecute123!', 'admin')

    // Create and approve multiple incidents
    const incidentIds: string[] = []
    for (let i = 0; i < 2; i++) {
      const createRes = await app.request('/api/v1/incidents', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${authToken}`,
        },
        body: JSON.stringify({
          title: `Bulk execute incident ${i}`,
          source: 'test',
          severity: 'medium',
          action_type: 'restart_deployment',
          action_ref: 'default/app',
        }),
      }, env)
      const data = await createRes.json() as { data?: { id: string } }
      const id = data.data?.id || ''
      incidentIds.push(id)

      // Approve each
      await app.request(`/api/v1/incidents/${id}/approve`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${authToken}` },
      }, env)
    }

    // Bulk execute
    const bulkRes = await app.request('/api/v1/incidents/bulk/execute', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ incident_ids: incidentIds }),
    }, env)

    expect(bulkRes.status).toBe(200)
    const bulkData = await bulkRes.json() as {
      success: boolean
      data?: { total: number; successful: string[]; failed: Array<{ id: string }> }
    }
    expect(bulkData.success).toBe(true)
    expect(bulkData.data?.total).toBe(2)
  })

  it('handles partial failures in bulk operations', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'bulk-partial@example.com', 'BulkPartial123!', 'admin')

    // Create one incident
    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Partial failure test',
        source: 'test',
        severity: 'medium',
        action_type: 'restart_deployment',
        action_ref: 'default/app',
      }),
    }, env)
    const data = await createRes.json() as { data?: { id: string } }
    const validId = data.data?.id || ''

    // Try to approve valid + invalid IDs
    const bulkRes = await app.request('/api/v1/incidents/bulk/approve', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        incident_ids: [validId, '00000000-0000-0000-0000-000000000000'],
      }),
    }, env)

    expect(bulkRes.status).toBe(200)
    const bulkData = await bulkRes.json() as {
      success: boolean
      data?: { total: number; successful: string[]; failed: Array<{ id: string; error: string }> }
    }
    expect(bulkData.success).toBe(true)
    expect(bulkData.data?.total).toBe(2)
    expect(bulkData.data?.successful.length).toBe(1)
    expect(bulkData.data?.failed.length).toBe(1)
    expect(bulkData.data?.failed[0]?.error).toContain('not found')
  })

  it('searches incidents by title and summary', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'search-test@example.com', 'SearchTest123!', 'admin')

    // Create incidents with different titles
    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Database connection timeout',
        source: 'monitoring',
        severity: 'high',
        summary: 'Production database is not responding to connection requests',
      }),
    }, env)

    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'API latency spike',
        source: 'alerts',
        severity: 'medium',
        summary: 'API response times exceeded threshold',
      }),
    }, env)

    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Memory leak in worker',
        source: 'monitoring',
        severity: 'low',
      }),
    }, env)

    // Search for "database"
    const dbSearchRes = await app.request('/api/v1/incidents/search?q=database', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(dbSearchRes.status).toBe(200)
    const dbSearchData = await dbSearchRes.json() as {
      success: boolean
      data?: {
        items: Array<{ title: string; search_score: number; search_highlights: string[] }>
        total: number
        query: string
      }
    }
    expect(dbSearchData.success).toBe(true)
    expect(dbSearchData.data?.query).toBe('database')
    expect(dbSearchData.data?.total).toBeGreaterThanOrEqual(1)
    expect(dbSearchData.data?.items.some(i => i.title.includes('Database'))).toBe(true)

    // Search for "monitoring"
    const monSearchRes = await app.request('/api/v1/incidents/search?q=monitoring', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(monSearchRes.status).toBe(200)
    const monSearchData = await monSearchRes.json() as {
      success: boolean
      data?: { total: number }
    }
    expect(monSearchData.data?.total).toBeGreaterThanOrEqual(2)
  })

  it('searches incidents with filters', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'search-filter@example.com', 'SearchFilter123!', 'admin')

    // Create incidents with different severities
    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Critical system failure',
        source: 'alerts',
        severity: 'critical',
      }),
    }, env)

    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'System maintenance reminder',
        source: 'scheduler',
        severity: 'low',
      }),
    }, env)

    // Search for "system" with severity filter
    const searchRes = await app.request('/api/v1/incidents/search?q=system&severity=critical', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(searchRes.status).toBe(200)
    const searchData = await searchRes.json() as {
      success: boolean
      data?: { total: number; items: Array<{ severity: string }> }
    }
    expect(searchData.success).toBe(true)
    expect(searchData.data?.items.every(i => i.severity === 'critical')).toBe(true)
  })

  it('returns empty results for non-matching query', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'search-empty@example.com', 'SearchEmpty123!', 'admin')

    const searchRes = await app.request('/api/v1/incidents/search?q=nonexistentqueryterm', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(searchRes.status).toBe(200)
    const searchData = await searchRes.json() as { success: boolean; data?: { total: number } }
    expect(searchData.success).toBe(true)
    expect(searchData.data?.total).toBe(0)
  })

  // Dashboard Metrics Tests
  it('returns dashboard metrics', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'dashboard@example.com', 'Dashboard123!', 'admin')

    // Create some incidents
    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Critical incident',
        source: 'monitoring',
        severity: 'critical',
      }),
    }, env)

    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'High incident',
        source: 'monitoring',
        severity: 'high',
      }),
    }, env)

    const metricsRes = await app.request('/api/v1/incidents/dashboard/metrics', {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(metricsRes.status).toBe(200)
    const metricsData = await metricsRes.json() as {
      success: boolean
      data?: {
        current: { open_incidents: number; critical_incidents: number }
        last_24h: { created: number }
        last_7d: { created: number }
        last_30d: { created: number }
      }
    }
    expect(metricsData.success).toBe(true)
    expect(metricsData.data?.current.open_incidents).toBeGreaterThanOrEqual(2)
    expect(metricsData.data?.current.critical_incidents).toBeGreaterThanOrEqual(1)
    expect(metricsData.data?.last_24h.created).toBeGreaterThanOrEqual(2)
  })

  // Incident Correlation Tests
  it('finds related incidents with correlation', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'correlation@example.com', 'Correlation123!', 'admin')

    // Create incidents with same correlation_id
    const createRes1 = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'First related incident',
        source: 'monitoring',
        severity: 'high',
        correlation_id: 'related-test-001',
      }),
    }, env)

    const data1 = await createRes1.json() as { data?: { id: string } }

    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Second related incident',
        source: 'monitoring',
        severity: 'high',
        correlation_id: 'related-test-001',
      }),
    }, env)

    const correlationRes = await app.request(`/api/v1/incidents/${data1.data?.id}/correlation`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(correlationRes.status).toBe(200)
    const correlationData = await correlationRes.json() as {
      success: boolean
      data?: {
        incident_id: string
        related_incidents: Array<{ id: string; correlation_score: number }>
      }
    }
    expect(correlationData.success).toBe(true)
    expect(correlationData.data?.incident_id).toBe(data1.data?.id)
  })

  // Incident Watch Tests
  it('allows user to watch and unwatch an incident', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'watch@example.com', 'Watch123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Watchable incident',
        source: 'test',
        severity: 'medium',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const incidentId = createData.data?.id

    // Watch the incident
    const watchRes = await app.request(`/api/v1/incidents/${incidentId}/watch`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        notify_on: ['resolved', 'escalated', 'comment'],
      }),
    }, env)

    expect(watchRes.status).toBe(201)
    const watchData = await watchRes.json() as { success: boolean; data?: { incident_id: string; notify_on: string[] } }
    expect(watchData.success).toBe(true)
    expect(watchData.data?.incident_id).toBe(incidentId)
    expect(watchData.data?.notify_on).toContain('resolved')

    // Get watchers
    const watchersRes = await app.request(`/api/v1/incidents/${incidentId}/watchers`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(watchersRes.status).toBe(200)
    const watchersData = await watchersRes.json() as { success: boolean; data?: Array<{ incident_id: string }> }
    expect(watchersData.success).toBe(true)
    expect(watchersData.data?.length).toBeGreaterThanOrEqual(1)

    // Unwatch the incident
    const unwatchRes = await app.request(`/api/v1/incidents/${incidentId}/watch`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(unwatchRes.status).toBe(200)
  })

  // External Ticket Tests
  it('creates and updates external tickets', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'ticket@example.com', 'Ticket123!', 'admin')

    const createRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Incident with external ticket',
        source: 'test',
        severity: 'high',
      }),
    }, env)

    const createData = await createRes.json() as { data?: { id: string } }
    const incidentId = createData.data?.id

    // Create external ticket
    const ticketRes = await app.request(`/api/v1/incidents/${incidentId}/tickets`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        system: 'jira',
        ticket_id: 'OPS-12345',
        ticket_url: 'https://jira.example.com/browse/OPS-12345',
        status: 'Open',
      }),
    }, env)

    expect(ticketRes.status).toBe(201)
    const ticketData = await ticketRes.json() as { success: boolean; data?: { system: string; ticket_id: string; status: string } }
    expect(ticketData.success).toBe(true)
    expect(ticketData.data?.system).toBe('jira')
    expect(ticketData.data?.ticket_id).toBe('OPS-12345')

    // List tickets
    const listRes = await app.request(`/api/v1/incidents/${incidentId}/tickets`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(listRes.status).toBe(200)
    const listData = await listRes.json() as { success: boolean; data?: Array<{ ticket_id: string }> }
    expect(listData.success).toBe(true)
    expect(listData.data?.some(t => t.ticket_id === 'OPS-12345')).toBe(true)

    // Update ticket status
    const updateRes = await app.request(`/api/v1/incidents/${incidentId}/tickets/OPS-12345`, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        status: 'In Progress',
      }),
    }, env)

    expect(updateRes.status).toBe(200)
    const updateData = await updateRes.json() as { success: boolean; data?: { status: string } }
    expect(updateData.success).toBe(true)
    expect(updateData.data?.status).toBe('In Progress')
  })

  // Response Playbook Tests
  it('creates and executes a response playbook', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'playbook@example.com', 'Playbook123!', 'admin')

    // Create playbook
    const createRes = await app.request('/api/v1/incidents/response-playbooks', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'High Severity Response',
        description: 'Response playbook for high severity incidents',
        trigger_conditions: {
          severity: ['high', 'critical'],
        },
        steps: [
          { title: 'Assess impact', action: 'manual', estimated_duration_minutes: 10 },
          { title: 'Notify stakeholders', action: 'automated', automated_action: { type: 'notify', ref: 'slack-alerts' } },
          { title: 'Execute remediation', action: 'approval', required_role: 'admin' },
        ],
        auto_trigger: true,
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { id: string; steps: Array<{ id: string }> } }
    expect(createData.success).toBe(true)
    expect(createData.data?.steps.length).toBe(3)

    const playbookId = createData.data?.id

    // Create incident to match
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Critical system failure',
        source: 'monitoring',
        severity: 'critical',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Get matching playbooks
    const matchRes = await app.request(`/api/v1/incidents/${incidentId}/matching-playbooks`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(matchRes.status).toBe(200)
    const matchData = await matchRes.json() as { success: boolean; data?: Array<{ id: string }> }
    expect(matchData.success).toBe(true)
    expect(matchData.data?.some(p => p.id === playbookId)).toBe(true)

    // Start playbook execution
    const execRes = await app.request(`/api/v1/incidents/${incidentId}/execute-playbook`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ playbook_id: playbookId }),
    }, env)

    expect(execRes.status).toBe(201)
    const execData = await execRes.json() as { success: boolean; data?: { id: string; status: string; current_step: number } }
    expect(execData.success).toBe(true)
    expect(execData.data?.status).toBe('running')

    const executionId = execData.data?.id
    const stepId = createData.data?.steps[0].id

    // Complete first step
    const completeRes = await app.request(`/api/v1/incidents/playbook-executions/${executionId}/steps/${stepId}/complete`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ result: { notes: 'Impact assessed' } }),
    }, env)

    expect(completeRes.status).toBe(200)
    const completeData = await completeRes.json() as { success: boolean; data?: { current_step: number } }
    expect(completeData.data?.current_step).toBe(2)
  })

  // Custom Fields Tests
  it('creates and uses custom fields for incidents', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'customfield@example.com', 'CustomField123!', 'admin')

    // Create custom field
    const fieldRes = await app.request('/api/v1/incidents/custom-fields', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'Customer Impact',
        key: 'customer_impact',
        type: 'select',
        required: false,
        options: ['Low', 'Medium', 'High', 'Critical'],
        description: 'Impact level on customers',
      }),
    }, env)

    expect(fieldRes.status).toBe(201)
    const fieldData = await fieldRes.json() as { success: boolean; data?: { id: string; key: string } }
    expect(fieldData.success).toBe(true)
    expect(fieldData.data?.key).toBe('customer_impact')

    const fieldId = fieldData.data?.id

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Service degradation',
        source: 'monitoring',
        severity: 'high',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Set custom field value
    const setValueRes = await app.request(`/api/v1/incidents/${incidentId}/custom-fields/${fieldId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ value: 'High' }),
    }, env)

    expect(setValueRes.status).toBe(200)
    const setValueData = await setValueRes.json() as { success: boolean; data?: { value: string } }
    expect(setValueData.success).toBe(true)
    expect(setValueData.data?.value).toBe('High')

    // Get custom fields
    const getFieldsRes = await app.request(`/api/v1/incidents/${incidentId}/custom-fields`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(getFieldsRes.status).toBe(200)
    const getFieldsData = await getFieldsRes.json() as { success: boolean; data?: Array<{ field_id: string; value: string }> }
    expect(getFieldsData.success).toBe(true)
    expect(getFieldsData.data?.some(f => f.field_id === fieldId && f.value === 'High')).toBe(true)
  })

  // War Room Tests
  it('creates and manages a war room', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'warroom@example.com', 'WarRoom123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Major outage',
        source: 'monitoring',
        severity: 'critical',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Create war room
    const createRes = await app.request(`/api/v1/incidents/${incidentId}/war-room`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { id: string; status: string; participants: Array<{ role: string }> } }
    expect(createData.success).toBe(true)
    expect(createData.data?.status).toBe('active')
    expect(createData.data?.participants.length).toBe(1)
    expect(createData.data?.participants[0].role).toBe('commander')

    // Join war room
    const joinRes = await app.request(`/api/v1/incidents/${incidentId}/war-room/join`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ role: 'responder' }),
    }, env)

    expect(joinRes.status).toBe(200)

    // Add message
    const msgRes = await app.request(`/api/v1/incidents/${incidentId}/war-room/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ message: 'Investigating the root cause' }),
    }, env)

    expect(msgRes.status).toBe(200)
    const msgData = await msgRes.json() as { success: boolean; data?: { chat_messages: Array<{ message: string }> } }
    expect(msgData.data?.chat_messages.some(m => m.message === 'Investigating the root cause')).toBe(true)

    // Add resource
    const resourceRes = await app.request(`/api/v1/incidents/${incidentId}/war-room/resources`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        type: 'dashboard',
        title: 'System Dashboard',
        url: 'https://grafana.example.com/d/system',
      }),
    }, env)

    expect(resourceRes.status).toBe(200)

    // Close war room
    const closeRes = await app.request(`/api/v1/incidents/${incidentId}/war-room/close`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(closeRes.status).toBe(200)
    const closeData = await closeRes.json() as { success: boolean; data?: { status: string } }
    expect(closeData.data?.status).toBe('closed')
  })

  // AI Analysis Tests
  it('generates AI root cause analysis', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'aianalysis@example.com', 'AIAnalysis123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Database connection timeout',
        source: 'monitoring',
        severity: 'high',
        evidence: [
          { type: 'log', source: 'app-logs', content: 'Connection timeout to database after 30s' },
        ],
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Generate AI analysis
    const analysisRes = await app.request(`/api/v1/incidents/${incidentId}/ai-analysis`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(analysisRes.status).toBe(200)
    const analysisData = await analysisRes.json() as {
      success: boolean
      data?: {
        incident_id: string
        summary: string
        root_causes: Array<{ category: string }>
        impact_analysis: { business_impact: string }
      }
    }
    expect(analysisData.success).toBe(true)
    expect(analysisData.data?.incident_id).toBe(incidentId)
  })

  // Export Tests
  it('exports incidents to JSON', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'export@example.com', 'Export123!', 'admin')

    // Create some incidents
    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Export test incident 1',
        source: 'test',
        severity: 'medium',
      }),
    }, env)

    await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Export test incident 2',
        source: 'test',
        severity: 'low',
      }),
    }, env)

    // Request export
    const exportRes = await app.request('/api/v1/incidents/export', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        format: 'json',
        include_evidence: true,
        filters: {
          source: ['test'],
        },
      }),
    }, env)

    expect(exportRes.status).toBe(201)
    const exportData = await exportRes.json() as {
      success: boolean
      data?: {
        id: string
        format: string
        status: string
        total_incidents: number
        download_url?: string
      }
    }
    expect(exportData.success).toBe(true)
    expect(exportData.data?.format).toBe('json')
    expect(exportData.data?.total_incidents).toBeGreaterThanOrEqual(1)
    expect(exportData.data?.download_url).toBeDefined()
  })

  // Incident Review Tests
  it('creates and completes incident reviews', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'review@example.com', 'Review123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Review test incident',
        source: 'test',
        severity: 'high',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Create review
    const createRes = await app.request(`/api/v1/incidents/${incidentId}/reviews`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        scheduled_at: new Date(Date.now() + 86400000).toISOString(),
        review_type: 'post_resolution',
        agenda: ['Review timeline', 'Identify improvements'],
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { id: string; agenda: string[] } }
    expect(createData.success).toBe(true)
    expect(createData.data?.agenda.length).toBe(2)

    const reviewId = createData.data?.id

    // Complete review
    const completeRes = await app.request(`/api/v1/incidents/${incidentId}/reviews/${reviewId}/complete`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        notes: 'Review completed successfully',
        action_items: [
          { description: 'Update runbook for this scenario', owner_id: 1 },
        ],
      }),
    }, env)

    expect(completeRes.status).toBe(200)
    const completeData = await completeRes.json() as { success: boolean; data?: { status: string; action_items: Array<{ description: string }> } }
    expect(completeData.success).toBe(true)
    expect(completeData.data?.status).toBe('completed')
    expect(completeData.data?.action_items.length).toBe(1)
  })

  // Incident Feedback Tests
  it('submits and retrieves incident feedback', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'feedback@example.com', 'Feedback123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Feedback test incident',
        source: 'test',
        severity: 'medium',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Submit feedback
    const submitRes = await app.request(`/api/v1/incidents/${incidentId}/feedback`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        ratings: {
          overall_satisfaction: 4,
          response_speed: 5,
          communication: 4,
          resolution_quality: 4,
        },
        strengths: ['Fast response', 'Clear communication'],
        improvements: ['More frequent updates'],
        would_recommend: true,
      }),
    }, env)

    expect(submitRes.status).toBe(201)
    const submitData = await submitRes.json() as { success: boolean; data?: { ratings: { overall_satisfaction: number } } }
    expect(submitData.success).toBe(true)
    expect(submitData.data?.ratings.overall_satisfaction).toBe(4)

    // Get feedback
    const getRes = await app.request(`/api/v1/incidents/${incidentId}/feedback`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(getRes.status).toBe(200)
    const getData = await getRes.json() as { success: boolean; data?: { would_recommend: boolean } }
    expect(getData.success).toBe(true)
    expect(getData.data?.would_recommend).toBe(true)
  })

  // Incident Cost Tests
  it('calculates and retrieves incident costs', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'cost@example.com', 'Cost123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Cost test incident',
        source: 'test',
        severity: 'high',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Calculate cost
    const calcRes = await app.request(`/api/v1/incidents/${incidentId}/cost`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        labor_hours: 8,
        labor_rate_usd: 150,
        infrastructure_cost_usd: 200,
        revenue_impact_usd: 1000,
      }),
    }, env)

    expect(calcRes.status).toBe(201)
    const calcData = await calcRes.json() as { success: boolean; data?: { estimated_cost_usd: number; cost_breakdown: { labor_cost_usd: number } } }
    expect(calcData.success).toBe(true)
    expect(calcData.data?.estimated_cost_usd).toBe(2400) // 1200 labor + 200 infra + 1000 revenue
    expect(calcData.data?.cost_breakdown.labor_cost_usd).toBe(1200)

    // Get cost
    const getRes = await app.request(`/api/v1/incidents/${incidentId}/cost`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(getRes.status).toBe(200)
  })

  // On-Call Schedule Tests
  it('creates and retrieves on-call schedules', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'oncall@example.com', 'OnCall123!', 'admin')

    // Create schedule
    const createRes = await app.request('/api/v1/incidents/oncall/schedules', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'Primary On-Call',
        description: 'Primary on-call rotation',
        rotation_type: 'weekly',
        rotation_config: {
          start_date: new Date().toISOString(),
          members: [
            { user_id: 1, email: 'oncall1@example.com', order: 1 },
            { user_id: 2, email: 'oncall2@example.com', order: 2 },
          ],
          handoff_time: '09:00',
        },
        timezone: 'UTC',
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { id: string; rotation_type: string } }
    expect(createData.success).toBe(true)
    expect(createData.data?.rotation_type).toBe('weekly')

    const scheduleId = createData.data?.id

    // Get schedule
    const getRes = await app.request(`/api/v1/incidents/oncall/schedules/${scheduleId}`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(getRes.status).toBe(200)

    // Get current on-call
    const currentRes = await app.request(`/api/v1/incidents/oncall/schedules/${scheduleId}/current`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(currentRes.status).toBe(200)
    const currentData = await currentRes.json() as { success: boolean; data?: { is_override: boolean } }
    expect(currentData.success).toBe(true)
  })

  // Checklist Tests
  it('creates and updates incident checklists', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'checklist@example.com', 'Checklist123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Checklist test incident',
        source: 'test',
        severity: 'high',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Create checklist
    const createRes = await app.request(`/api/v1/incidents/${incidentId}/checklists`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        name: 'Response Checklist',
        items: ['Acknowledge incident', 'Assess impact', 'Notify stakeholders', 'Begin remediation'],
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { id: string; items: Array<{ id: string; checked: boolean }> } }
    expect(createData.success).toBe(true)
    expect(createData.data?.items.length).toBe(4)

    const checklistId = createData.data?.id
    const itemId = createData.data?.items[0].id

    // Update checklist item
    const updateRes = await app.request(`/api/v1/incidents/${incidentId}/checklists/${checklistId}/items/${itemId}`, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({ checked: true }),
    }, env)

    expect(updateRes.status).toBe(200)
    const updateData = await updateRes.json() as { success: boolean; data?: { items: Array<{ id: string; checked: boolean }> } }
    expect(updateData.success).toBe(true)
    expect(updateData.data?.items.find(i => i.id === itemId)?.checked).toBe(true)
  })

  // Change Link Tests
  it('links incidents to changes', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'changelink@example.com', 'ChangeLink123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Change link test incident',
        source: 'deployment',
        severity: 'high',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Link to change
    const linkRes = await app.request(`/api/v1/incidents/${incidentId}/changes`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        change_id: 'deploy-2024-001',
        change_type: 'deployment',
        change_description: 'Deployed version 2.1.0',
        change_url: 'https://github.com/example/repo/deployments/1',
        change_timestamp: new Date().toISOString(),
        relationship: 'caused',
      }),
    }, env)

    expect(linkRes.status).toBe(201)
    const linkData = await linkRes.json() as { success: boolean; data?: { change_id: string; relationship: string } }
    expect(linkData.success).toBe(true)
    expect(linkData.data?.change_id).toBe('deploy-2024-001')

    // Get changes
    const listRes = await app.request(`/api/v1/incidents/${incidentId}/changes`, {
      method: 'GET',
      headers: { Authorization: `Bearer ${authToken}` },
    }, env)

    expect(listRes.status).toBe(200)
    const listData = await listRes.json() as { success: boolean; data?: Array<{ change_id: string }> }
    expect(listData.success).toBe(true)
    expect(listData.data?.length).toBe(1)
  })

  // Compliance Tests
  it('creates and updates compliance records', async () => {
    const env = createEnv()
    const authToken = await registerAndLogin(env, 'compliance@example.com', 'Compliance123!', 'admin')

    // Create incident
    const incidentRes = await app.request('/api/v1/incidents', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        title: 'Compliance test incident',
        source: 'security',
        severity: 'critical',
      }),
    }, env)

    const incidentData = await incidentRes.json() as { data?: { id: string } }
    const incidentId = incidentData.data?.id

    // Create compliance record
    const createRes = await app.request(`/api/v1/incidents/${incidentId}/compliance`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        framework: 'soc2',
        requirements: [
          { requirement_id: 'CC6.1', description: 'Incident response procedures' },
          { requirement_id: 'CC6.6', description: 'Security incident logging' },
        ],
      }),
    }, env)

    expect(createRes.status).toBe(201)
    const createData = await createRes.json() as { success: boolean; data?: { framework: string; requirements: Array<{ requirement_id: string }> } }
    expect(createData.success).toBe(true)
    expect(createData.data?.framework).toBe('soc2')

    // Update requirement
    const updateRes = await app.request(`/api/v1/incidents/${incidentId}/compliance/CC6.1`, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authToken}`,
      },
      body: JSON.stringify({
        status: 'compliant',
        evidence: 'Runbook followed, timeline documented',
      }),
    }, env)

    expect(updateRes.status).toBe(200)
    const updateData = await updateRes.json() as { success: boolean; data?: { requirements: Array<{ requirement_id: string; status: string }> } }
    expect(updateData.success).toBe(true)
    expect(updateData.data?.requirements.find(r => r.requirement_id === 'CC6.1')?.status).toBe('compliant')
  })
})
