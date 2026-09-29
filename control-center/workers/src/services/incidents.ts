import type {
  AuthPrincipal,
  Env,
  IncidentActionType,
  IncidentComment,
  IncidentCommentInput,
  IncidentDetail,
  IncidentEvidence,
  IncidentExecutionResult,
  IncidentLink,
  IncidentRecord,
  IncidentRecommendation,
  IncidentSeverity,
  IncidentStatus,
  IncidentSummary,
  WebhookEventType,
} from '../types'
import { evaluateScalingPolicy, executeScalingAction, getScalingPolicy, type ScalingDecision } from './autoscaling'
import { listDeployments, listEvents, restartDeployment, scaleDeployment } from './kubernetes'
import { generateText, AI_MODELS } from './ai'
import { buildIncidentChannels, makeRealtimeEvent, publishRealtimeEvent } from './realtime'
import { getActivePolicy, evaluatePolicy } from './governance'
import { triggerWebhooks as triggerGlobalWebhooks } from './webhooks'
import { nanoid } from 'nanoid'
import { writeAnalyticsEvent } from './analytics'

const INCIDENT_PREFIX = 'incident:'
const INCIDENT_INDEX_KEY = `${INCIDENT_PREFIX}index`
const SUPPRESSION_RULES_KEY = `${INCIDENT_PREFIX}suppression_rules`
const DEDUP_WINDOW_KEY = `${INCIDENT_PREFIX}dedup:`

export interface CreateIncidentInput {
  title: string
  severity?: IncidentSeverity
  source: string
  summary?: string
  correlation_id?: string
  evidence?: IncidentEvidence[]
  action_type?: IncidentActionType
  action_ref?: string
  tags?: string[]
}

export interface IncidentListFilters {
  status?: IncidentStatus
  severity?: IncidentSeverity
  action_type?: IncidentActionType
  source?: string
  requested_via?: 'jwt' | 'api_key'
  approved_by?: number
  correlation_id?: string
  has_action?: boolean
  tags?: string[]
  page?: number
  per_page?: number
  sort?: 'created_at' | 'updated_at' | 'severity' | 'status'
  order?: 'asc' | 'desc'
}

export interface IncidentListResult {
  items: IncidentRecord[]
  total: number
  page: number
  per_page: number
  total_pages: number
}

export interface IncidentSearchParams {
  query: string
  status?: IncidentStatus[]
  severity?: IncidentSeverity[]
  action_type?: IncidentActionType[]
  source?: string[]
  created_after?: string
  created_before?: string
  page?: number
  per_page?: number
}

export interface IncidentSearchResult {
  items: Array<IncidentRecord & { search_score: number; search_highlights: string[] }>
  total: number
  page: number
  per_page: number
  total_pages: number
  query: string
}

function nowIso(): string {
  return new Date().toISOString()
}

async function getIncidentIndex(env: Env): Promise<string[]> {
  return (await env.KV.get(INCIDENT_INDEX_KEY, 'json') as string[] | null) || []
}

async function setIncidentIndex(env: Env, ids: string[]): Promise<void> {
  await env.KV.put(INCIDENT_INDEX_KEY, JSON.stringify(ids.slice(0, 200)), { expirationTtl: 86400 * 30 })
}

function incidentKey(id: string): string {
  return `${INCIDENT_PREFIX}${id}`
}

function serializeIncident(incident: IncidentRecord) {
  return {
    ...incident,
    evidence: JSON.stringify(incident.evidence),
    recommendations: JSON.stringify(incident.recommendations),
    links: incident.links ? JSON.stringify(incident.links) : null,
    analysis: incident.analysis ? JSON.stringify(incident.analysis) : null,
    execution_result: incident.execution_result ? JSON.stringify(incident.execution_result) : null,
    tags: JSON.stringify(incident.tags || []),
  }
}

function deserializeIncident(record: Record<string, unknown>): IncidentRecord {
  return {
    ...(record as unknown as IncidentRecord),
    evidence: typeof record.evidence === 'string' ? JSON.parse(record.evidence) : ((record.evidence as IncidentRecord['evidence']) || []),
    recommendations: typeof record.recommendations === 'string' ? JSON.parse(record.recommendations) : ((record.recommendations as IncidentRecord['recommendations']) || []),
    links: typeof record.links === 'string' ? JSON.parse(record.links) : (record.links as IncidentRecord['links']),
    analysis: typeof record.analysis === 'string' ? JSON.parse(record.analysis) : (record.analysis as IncidentRecord['analysis']),
    execution_result: typeof record.execution_result === 'string' ? JSON.parse(record.execution_result) : (record.execution_result as IncidentRecord['execution_result']),
    tags: typeof record.tags === 'string' ? JSON.parse(record.tags) : ((record.tags as IncidentRecord['tags']) || []),
  }
}

export async function storeIncident(env: Env, incident: IncidentRecord): Promise<void> {
  const serialized = serializeIncident(incident)

  await env.DB
    .prepare(`
      INSERT OR REPLACE INTO incidents (
        id, title, summary, status, severity, source, correlation_id,
        requested_by, requested_by_email, requested_via, approved_by, approved_at,
        execution_id, action_type, action_ref, evidence, recommendations,
        links, analysis, execution_result, created_at, updated_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    .bind(
      serialized.id,
      serialized.title,
      serialized.summary || null,
      serialized.status,
      serialized.severity,
      serialized.source,
      serialized.correlation_id,
      serialized.requested_by,
      serialized.requested_by_email || null,
      serialized.requested_via,
      serialized.approved_by || null,
      serialized.approved_at || null,
      serialized.execution_id || null,
      serialized.action_type || null,
      serialized.action_ref || null,
      serialized.evidence,
      serialized.recommendations,
      serialized.links,
      serialized.analysis,
      serialized.execution_result,
      serialized.created_at,
      serialized.updated_at,
    )
    .run()

  await env.KV.put(incidentKey(incident.id), JSON.stringify(incident), { expirationTtl: 86400 * 30 })
  const ids = await getIncidentIndex(env)
  if (!ids.includes(incident.id)) {
    ids.unshift(incident.id)
    await setIncidentIndex(env, ids)
  }
}

export async function getIncident(env: Env, id: string): Promise<IncidentRecord | null> {
  const record = await env.DB
    .prepare('SELECT * FROM incidents WHERE id = ?')
    .bind(id)
    .first<Record<string, unknown>>()

  if (record) {
    return deserializeIncident(record)
  }

  return await env.KV.get(incidentKey(id), 'json') as IncidentRecord | null
}

export async function updateIncident(
  env: Env,
  id: string,
  updates: Partial<Omit<IncidentRecord, 'id' | 'created_at' | 'requested_by'>>
): Promise<IncidentRecord | null> {
  const incident = await getIncident(env, id)
  if (!incident) return null

  const updated: IncidentRecord = {
    ...incident,
    ...updates,
    updated_at: nowIso(),
  }

  await storeIncident(env, updated)
  return updated
}

export async function listIncidents(env: Env, filters: IncidentListFilters = {}): Promise<IncidentListResult> {
  const page = Math.max(1, filters.page || 1)
  const perPage = Math.max(1, Math.min(filters.per_page || 20, 100))
  const sort = filters.sort || 'created_at'
  const order = filters.order || 'desc'

  let sql = 'SELECT * FROM incidents WHERE 1=1'
  let countSql = 'SELECT COUNT(*) as total FROM incidents WHERE 1=1'
  const params: Array<string | number> = []

  if (filters.status) {
    sql += ' AND status = ?'
    countSql += ' AND status = ?'
    params.push(filters.status)
  }
  if (filters.severity) {
    sql += ' AND severity = ?'
    countSql += ' AND severity = ?'
    params.push(filters.severity)
  }
  if (filters.action_type) {
    sql += ' AND action_type = ?'
    countSql += ' AND action_type = ?'
    params.push(filters.action_type)
  }
  if (filters.source) {
    sql += ' AND source = ?'
    countSql += ' AND source = ?'
    params.push(filters.source)
  }
  if (filters.requested_via) {
    sql += ' AND requested_via = ?'
    countSql += ' AND requested_via = ?'
    params.push(filters.requested_via)
  }
  if (filters.approved_by) {
    sql += ' AND approved_by = ?'
    countSql += ' AND approved_by = ?'
    params.push(filters.approved_by)
  }
  if (filters.correlation_id) {
    sql += ' AND correlation_id = ?'
    countSql += ' AND correlation_id = ?'
    params.push(filters.correlation_id)
  }
  if (typeof filters.has_action === 'boolean') {
    const clause = filters.has_action
      ? ' AND action_type IS NOT NULL AND action_ref IS NOT NULL'
      : ' AND (action_type IS NULL OR action_ref IS NULL)'
    sql += clause
    countSql += clause
  }

  sql += ` ORDER BY ${sort} ${order.toUpperCase()} LIMIT ? OFFSET ?`
  const pagedParams = [...params, perPage, (page - 1) * perPage]

  const countResult = await env.DB
    .prepare(countSql)
    .bind(...params)
    .first<{ total: number }>()

  const result = await env.DB
    .prepare(sql)
    .bind(...pagedParams)
    .all<Record<string, unknown>>()

  if (result.results.length > 0 || (countResult?.total || 0) > 0) {
    const total = countResult?.total || 0
    return {
      items: result.results.map(deserializeIncident),
      total,
      page,
      per_page: perPage,
      total_pages: Math.max(1, Math.ceil(total / perPage)),
    }
  }

  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const filtered = incidents
    .filter((incident): incident is IncidentRecord => !!incident)
    .filter((incident) => {
      if (filters.status && incident.status !== filters.status) return false
      if (filters.severity && incident.severity !== filters.severity) return false
      if (filters.action_type && incident.action_type !== filters.action_type) return false
      if (filters.source && incident.source !== filters.source) return false
      if (filters.requested_via && incident.requested_via !== filters.requested_via) return false
      if (filters.approved_by && incident.approved_by !== filters.approved_by) return false
      if (filters.correlation_id && incident.correlation_id !== filters.correlation_id) return false
      if (typeof filters.has_action === 'boolean') {
        const hasAction = !!incident.action_type && !!incident.action_ref
        if (hasAction !== filters.has_action) return false
      }
      return true
    })

  const sorted = [...filtered].sort((a, b) => {
    const left = String((a as unknown as Record<string, unknown>)[sort] || '')
    const right = String((b as unknown as Record<string, unknown>)[sort] || '')
    return order === 'asc' ? left.localeCompare(right) : right.localeCompare(left)
  })

  const items = sorted.slice((page - 1) * perPage, page * perPage)
  return {
    items,
    total: filtered.length,
    page,
    per_page: perPage,
    total_pages: Math.max(1, Math.ceil(filtered.length / perPage)),
  }
}

function buildDefaultRecommendations(actionType?: IncidentActionType, actionRef?: string): IncidentRecommendation[] {
  if (!actionType || !actionRef) {
    return []
  }

  return [{
    id: crypto.randomUUID(),
    title: 'Execute bounded remediation',
    description: `Execute approved ${actionType} action for reference ${actionRef}`,
    action_type: actionType,
    action_ref: actionRef,
    confidence: 0.7,
  }]
}

async function collectTaskEvidence(env: Env, actionRef: string): Promise<IncidentEvidence[]> {
  const task = await env.DB
    .prepare(`
      SELECT t.*, p.name as playbook_name
      FROM tasks t
      LEFT JOIN playbooks p ON t.playbook_id = p.id
      WHERE t.task_id = ? OR t.id = ?
    `)
    .bind(actionRef, actionRef)
    .first<Record<string, unknown> & { task_id?: string }>()

  if (!task) {
    return []
  }

  const logs = await env.DB
    .prepare(`
      SELECT * FROM task_logs
      WHERE task_id = ?
      ORDER BY created_at ASC
      LIMIT 10
    `)
    .bind(task.task_id || actionRef)
    .all<Record<string, unknown>>()

  return [
    {
      type: 'task',
      source: 'tasks.record',
      content: JSON.stringify(task),
    },
    ...logs.results.map((log) => ({
      type: 'log' as const,
      source: 'tasks.log',
      content: JSON.stringify(log),
    })),
  ]
}

async function collectNodeEvidence(env: Env, actionRef: string): Promise<IncidentEvidence[]> {
  const nodeId = actionRef.startsWith('node:') ? actionRef.slice(5) : actionRef
  const node = await env.DB
    .prepare('SELECT * FROM nodes WHERE id = ?')
    .bind(nodeId)
    .first<Record<string, unknown>>()

  if (!node) {
    return []
  }

  const latestMetrics = await env.KV.get(`agent:latest:${(node as { agent_id?: string }).agent_id || nodeId}`, 'json') as Record<string, unknown> | null

  return [
    {
      type: 'node',
      source: 'nodes.record',
      content: JSON.stringify(node),
    },
    ...(latestMetrics ? [{
      type: 'metric' as const,
      source: 'nodes.latest_metrics',
      content: JSON.stringify(latestMetrics),
    }] : []),
  ]
}

async function collectIncidentEvidence(env: Env, input: CreateIncidentInput): Promise<IncidentEvidence[]> {
  const evidence: IncidentEvidence[] = [...(input.evidence || [])]

  if (input.action_ref?.startsWith('task:') || input.source === 'task') {
    const taskRef = input.action_ref?.startsWith('task:') ? input.action_ref.slice(5) : input.action_ref
    if (taskRef) {
      evidence.push(...await collectTaskEvidence(env, taskRef))
    }
  }

  if (input.action_ref?.startsWith('node:') || input.source === 'node') {
    evidence.push(...await collectNodeEvidence(env, input.action_ref || input.source))
  }

  if (input.action_type === 'scale_policy' && input.action_ref) {
    const policy = await getScalingPolicy(env, input.action_ref)
    if (policy) {
      evidence.push({
        type: 'metric',
        source: 'autoscaling.policy',
        content: JSON.stringify({
          policy_id: policy.id,
          target_type: policy.targetType,
          target_id: policy.targetId,
          namespace: policy.namespace,
          min_replicas: policy.minReplicas,
          max_replicas: policy.maxReplicas,
          metrics: policy.metrics,
        }),
      })
    }
  }

  if (input.action_type === 'restart_deployment' && input.action_ref) {
    const [namespace, name] = input.action_ref.split('/', 2)
    if (namespace && name) {
      const [deployments, events] = await Promise.all([
        listDeployments(env, namespace),
        listEvents(env, namespace),
      ])

      const deployment = deployments.find((item) => item.name === name)
      if (deployment) {
        evidence.push({
          type: 'service',
          source: 'kubernetes.deployment',
          content: JSON.stringify({
            namespace,
            name: deployment.name,
            replicas: deployment.replicas,
            ready_replicas: deployment.readyReplicas,
            available_replicas: deployment.availableReplicas,
            containers: deployment.containers,
          }),
        })
      }

      const relatedEvents = events
        .filter((event) => event.involvedObject.name === name || event.involvedObject.name.startsWith(`${name}-`))
        .slice(0, 5)
      for (const event of relatedEvents) {
        evidence.push({
          type: 'alert',
          source: 'kubernetes.event',
          content: JSON.stringify({
            namespace,
            reason: event.reason,
            message: event.message,
            type: event.type,
            last_timestamp: event.lastTimestamp,
          }),
        })
      }
    }
  }

  return evidence
}

function publishIncidentEvent(env: Env, incident: IncidentRecord, type: string): void {
  publishRealtimeEvent(makeRealtimeEvent(
    type,
    'incident',
    buildIncidentChannels(incident.id, incident.requested_by),
    {
      incident_id: incident.id,
      status: incident.status,
      severity: incident.severity,
      title: incident.title,
      action_type: incident.action_type,
      action_ref: incident.action_ref,
    },
    {
      user_id: incident.requested_by,
      correlation_id: incident.correlation_id,
      resource: {
        kind: 'incident',
        id: incident.id,
        name: incident.title,
      },
    }
  ))

  writeAnalyticsEvent(env, {
    indexes: [type, incident.id, incident.status, incident.severity],
    doubles: [1],
  })

  // Trigger webhooks (async, non-blocking)
  triggerGlobalWebhooks(env, type as import('../types').WebhookEventType, incident).catch(() => {
    // Silently fail webhook triggers
  })
}

function buildIncidentLinks(input: CreateIncidentInput): IncidentRecord['links'] {
  const links: NonNullable<IncidentRecord['links']> = []

  if (!input.action_ref) {
    return links
  }

  if (input.action_type === 'scale_policy') {
    links.push({
      kind: 'scaling_policy',
      id: input.action_ref,
      href: `/api/v1/scaling/policies/${input.action_ref}`,
    })
  }

  if (input.action_type === 'restart_deployment') {
    const [namespace, name] = input.action_ref.split('/', 2)
    if (namespace && name) {
      links.push({
        kind: 'deployment',
        id: `${namespace}/${name}`,
        name,
        href: `/api/v1/kubernetes/namespaces/${namespace}/deployments/${name}`,
      })
    }
  }

  if (input.action_ref.startsWith('task:')) {
    const id = input.action_ref.slice(5)
    links.push({
      kind: 'task',
      id,
      href: `/api/v1/tasks/${id}`,
    })
  }

  if (input.action_ref.startsWith('node:')) {
    const id = input.action_ref.slice(5)
    links.push({
      kind: 'node',
      id,
      href: `/api/v1/nodes/${id}`,
    })
  }

  return links
}

export function toIncidentSummary(incident: IncidentRecord): IncidentSummary {
  return {
    id: incident.id,
    title: incident.title,
    summary: incident.summary,
    status: incident.status,
    severity: incident.severity,
    source: incident.source,
    requested_via: incident.requested_via,
    action_type: incident.action_type,
    action_ref: incident.action_ref,
    approved_by: incident.approved_by,
    correlation_id: incident.correlation_id,
    links: incident.links,
    tags: incident.tags || [],
    created_at: incident.created_at,
    updated_at: incident.updated_at,
  }
}

export function toIncidentDetail(incident: IncidentRecord): IncidentDetail {
  return {
    ...toIncidentSummary(incident),
    requested_by: incident.requested_by,
    requested_by_email: incident.requested_by_email,
    approved_at: incident.approved_at,
    execution_id: incident.execution_id,
    evidence: incident.evidence,
    recommendations: incident.recommendations,
    analysis: incident.analysis,
    execution_result: incident.execution_result,
  }
}

export async function canApproveIncident(env: Env, principal: AuthPrincipal, incident: IncidentRecord): Promise<boolean> {
  const policy = await getActivePolicy(env)
  const evaluation = evaluatePolicy(policy, principal, incident)
  return evaluation.allowed
}

export async function canExecuteIncident(env: Env, principal: AuthPrincipal, incident: IncidentRecord): Promise<boolean> {
  // Execution uses the same policy as approval
  return canApproveIncident(env, principal, incident)
}

export async function createIncident(env: Env, principal: AuthPrincipal, input: CreateIncidentInput): Promise<IncidentRecord> {
  // Check for suppression
  const suppressionRule = await findMatchingSuppressionRule(env, input)
  if (suppressionRule) {
    const timestamp = nowIso()
    const suppressedIncident: IncidentRecord = {
      id: crypto.randomUUID(),
      title: input.title,
      summary: `[SUPPRESSED] ${input.summary || ''}`,
      status: 'resolved',
      severity: input.severity || 'medium',
      source: input.source,
      correlation_id: input.correlation_id || crypto.randomUUID(),
      requested_by: principal.sub,
      requested_by_email: principal.email,
      requested_via: principal.auth_method,
      action_type: input.action_type,
      action_ref: input.action_ref,
      evidence: [],
      recommendations: [],
      tags: ['suppressed'],
      suppressed_by: {
        rule_id: suppressionRule.id,
        rule_name: suppressionRule.name,
        matched_at: timestamp,
        expires_at: suppressionRule.expires_at || '',
      },
      created_at: timestamp,
      updated_at: timestamp,
    }

    await storeIncident(env, suppressedIncident)
    return suppressedIncident
  }

  // Check for duplicates
  const correlationId = input.correlation_id || crypto.randomUUID()
  const duplicate = await findDuplicateIncident(env, correlationId)
  if (duplicate) {
    const updated = await incrementDuplicateCount(env, duplicate)
    return updated
  }

  const timestamp = nowIso()
  const evidence = await collectIncidentEvidence(env, input)
  const incident: IncidentRecord = {
    id: crypto.randomUUID(),
    title: input.title,
    summary: input.summary,
    status: 'open',
    severity: input.severity || 'medium',
    source: input.source,
    correlation_id: correlationId,
    requested_by: principal.sub,
    requested_by_email: principal.email,
    requested_via: principal.auth_method,
    action_type: input.action_type,
    action_ref: input.action_ref,
    evidence,
    recommendations: buildDefaultRecommendations(input.action_type, input.action_ref),
    links: buildIncidentLinks(input),
    tags: input.tags || [],
    duplicate_count: 1,
    created_at: timestamp,
    updated_at: timestamp,
  }

  await storeIncident(env, incident)
  publishIncidentEvent(env, incident, 'incident.created')
  return incident
}

export async function analyzeIncident(env: Env, incident: IncidentRecord): Promise<IncidentRecord> {
  const prompt = `Analyze this infrastructure incident and return strict JSON with keys: summary, severity, likely_cause, recommended_actions.\n\nTitle: ${incident.title}\nSource: ${incident.source}\nCurrent Severity: ${incident.severity}\nSummary: ${incident.summary || 'N/A'}\nEvidence: ${JSON.stringify(incident.evidence)}`

  const result = await generateText(env, prompt, {
    model: AI_MODELS.textGenerationFast,
    maxTokens: 256,
    temperature: 0.2,
    systemPrompt: 'You are an incident response assistant for DevOps workflows. Return concise JSON only.',
  })

  let analysis: Record<string, unknown> = {
    summary: incident.summary || incident.title,
    severity: incident.severity,
    likely_cause: 'Unknown',
    recommended_actions: incident.recommendations.map(r => r.title),
  }

  if (result.success && result.data) {
    const response = result.data as { response?: string; result?: { response?: string } }
    const text = typeof response === 'string'
      ? response
      : typeof response?.response === 'string'
        ? response.response
        : typeof response?.result?.response === 'string'
          ? response.result.response
          : ''

    if (text) {
      try {
        analysis = JSON.parse(text) as Record<string, unknown>
      } catch {
        analysis = {
          ...analysis,
          raw_response: text,
        }
      }
    }
  }

  const recommendations = incident.recommendations.length > 0
    ? incident.recommendations
    : [{
        id: crypto.randomUUID(),
        title: 'Review evidence and choose bounded remediation',
        description: 'Incident was analyzed but no executable remediation reference was supplied.',
      }]

  const updated: IncidentRecord = {
    ...incident,
    status: 'analyzed',
    summary: typeof analysis.summary === 'string' ? analysis.summary : incident.summary,
    analysis,
    recommendations,
    updated_at: nowIso(),
  }

  await storeIncident(env, updated)
  publishIncidentEvent(env, updated, 'incident.analyzed')
  return updated
}

export async function approveIncident(env: Env, incident: IncidentRecord, principal: AuthPrincipal): Promise<IncidentRecord> {
  const updated: IncidentRecord = {
    ...incident,
    status: 'approved',
    approved_by: principal.sub,
    approved_at: nowIso(),
    updated_at: nowIso(),
  }

  await storeIncident(env, updated)
  publishIncidentEvent(env, updated, 'incident.approved')
  return updated
}

async function resolveScalingDecision(env: Env, incident: IncidentRecord): Promise<ScalingDecision> {
  if (!incident.action_ref) {
    throw new Error('Incident has no action reference')
  }

  const policy = await getScalingPolicy(env, incident.action_ref)
  if (!policy) {
    throw new Error('Scaling policy not found')
  }

  return evaluateScalingPolicy(env, policy)
}

async function executeRestartDeployment(env: Env, actionRef: string): Promise<{ success: boolean; execution_id: string; result: Record<string, unknown> }> {
  const [namespace, name] = actionRef.split('/', 2)
  if (!namespace || !name) {
    throw new Error('restart_deployment action_ref must be <namespace>/<deployment>')
  }

  const success = await restartDeployment(env, namespace, name)
  return {
    success,
    execution_id: crypto.randomUUID(),
    result: {
      namespace,
      deployment: name,
      restarted: success,
    },
  }
}

async function executeScaleDeployment(env: Env, actionRef: string): Promise<{ success: boolean; execution_id: string; result: Record<string, unknown> }> {
  // Format: <namespace>/<deployment>/<replicas>
  const parts = actionRef.split('/')
  if (parts.length !== 3) {
    throw new Error('scale_deployment action_ref must be <namespace>/<deployment>/<replicas>')
  }

  const [namespace, name, replicasStr] = parts
  const replicas = parseInt(replicasStr, 10)
  if (isNaN(replicas) || replicas < 0 || replicas > 1000) {
    throw new Error('Replicas must be a valid number between 0 and 1000')
  }

  const success = await scaleDeployment(env, namespace, name, replicas)
  return {
    success,
    execution_id: crypto.randomUUID(),
    result: {
      namespace,
      deployment: name,
      replicas,
      scaled: success,
    },
  }
}

export async function executeIncident(env: Env, incident: IncidentRecord): Promise<IncidentRecord> {
  if (incident.status !== 'approved') {
    throw new Error('Incident must be approved before execution')
  }

  if (!incident.action_type || !incident.action_ref) {
    throw new Error('Unsupported or missing incident action')
  }

  const executing: IncidentRecord = {
    ...incident,
    status: 'executing',
    updated_at: nowIso(),
  }

  await storeIncident(env, executing)
  publishIncidentEvent(env, executing, 'incident.executing')

  const actionRef = executing.action_ref
  if (!actionRef) {
    throw new Error('Incident has no action reference')
  }

  let updated: IncidentRecord

  if (incident.action_type === 'scale_policy') {
    const decision = await resolveScalingDecision(env, executing)
    const result = await executeScalingAction(env, actionRef, decision)

    updated = {
      ...executing,
      status: result.success ? 'resolved' : 'failed',
      execution_id: result.event?.id,
      execution_result: result.success
        ? {
            backend: 'autoscaling',
            success: true,
            operation: decision.action,
            message: decision.reason,
            target: {
              kind: 'scaling_policy',
              id: actionRef,
            },
            details: {
              decision,
              event: result.event,
            },
          }
        : {
            backend: 'autoscaling',
            success: false,
            operation: decision.action,
            message: result.error,
            target: {
              kind: 'scaling_policy',
              id: actionRef,
            },
            details: {
              decision,
            },
          },
      updated_at: nowIso(),
    }
  } else if (incident.action_type === 'restart_deployment') {
    const result = await executeRestartDeployment(env, actionRef)
    updated = {
      ...executing,
      status: result.success ? 'resolved' : 'failed',
      execution_id: result.execution_id,
      execution_result: {
        backend: 'kubernetes',
        success: result.success,
        operation: 'restart_deployment',
        message: result.success ? 'Deployment restarted' : 'Deployment restart failed',
        target: {
          kind: 'deployment',
          id: actionRef,
          name: (result.result.deployment as string | undefined),
          namespace: (result.result.namespace as string | undefined),
        },
        details: result.result,
      },
      updated_at: nowIso(),
    }
  } else if (incident.action_type === 'scale_deployment') {
    const result = await executeScaleDeployment(env, actionRef)
    updated = {
      ...executing,
      status: result.success ? 'resolved' : 'failed',
      execution_id: result.execution_id,
      execution_result: {
        backend: 'kubernetes',
        success: result.success,
        operation: 'scale_deployment',
        message: result.success ? `Deployment scaled to ${result.result.replicas} replicas` : 'Deployment scaling failed',
        target: {
          kind: 'deployment',
          id: actionRef,
          name: (result.result.deployment as string | undefined),
          namespace: (result.result.namespace as string | undefined),
        },
        details: result.result,
      },
      updated_at: nowIso(),
    }
  } else {
    throw new Error('Unsupported incident action type')
  }

  await storeIncident(env, updated)
  publishIncidentEvent(env, updated, updated.status === 'resolved' ? 'incident.resolved' : 'incident.failed')
  return updated
}

export function buildIncidentTimeline(incident: IncidentRecord): import('../types').IncidentTimeline {
  const events: import('../types').IncidentTimelineEvent[] = []

  // Creation event
  events.push({
    id: `${incident.id}:created`,
    incident_id: incident.id,
    type: 'created',
    timestamp: incident.created_at,
    actor: {
      user_id: incident.requested_by,
      email: incident.requested_by_email,
    },
    summary: `Incident created: ${incident.title}`,
    details: {
      severity: incident.severity,
      source: incident.source,
      action_type: incident.action_type,
      action_ref: incident.action_ref,
    },
    metadata: {
      correlation_id: incident.correlation_id,
      requested_via: incident.requested_via,
      initial_evidence_count: incident.evidence.length,
    },
  })

  // Evidence added events (for enriched evidence beyond the first)
  if (incident.evidence.length > 0) {
    const enrichedEvidence = incident.evidence.slice(1)
    enrichedEvidence.forEach((evidence, index) => {
      events.push({
        id: `${incident.id}:evidence:${index}`,
        incident_id: incident.id,
        type: 'evidence_added',
        timestamp: incident.created_at,
        summary: `Evidence added: ${evidence.type} from ${evidence.source}`,
        details: {
          type: evidence.type,
          source: evidence.source,
          content_preview: evidence.content.substring(0, 200),
        },
      })
    })
  }

  // Analysis event (if incident was analyzed)
  if (incident.status !== 'open' && incident.analysis) {
    events.push({
      id: `${incident.id}:analyzed`,
      incident_id: incident.id,
      type: 'analyzed',
      timestamp: incident.updated_at,
      summary: incident.analysis.summary as string || 'Incident analyzed',
      details: {
        severity: incident.analysis.severity || incident.severity,
        likely_cause: incident.analysis.likely_cause,
        recommended_actions: incident.analysis.recommended_actions,
      },
      metadata: {
        recommendations_count: incident.recommendations.length,
      },
    })
  }

  // Approval event (if incident was approved)
  if (incident.approved_by && incident.approved_at) {
    events.push({
      id: `${incident.id}:approved`,
      incident_id: incident.id,
      type: 'approved',
      timestamp: incident.approved_at,
      actor: {
        user_id: incident.approved_by,
      },
      summary: 'Incident approved for execution',
      details: {
        action_type: incident.action_type,
        action_ref: incident.action_ref,
      },
    })
  }

  // Execution events (if incident was executed)
  if (incident.status === 'executing' || incident.status === 'resolved' || incident.status === 'failed') {
    if (incident.execution_result) {
      events.push({
        id: `${incident.id}:executing`,
        incident_id: incident.id,
        type: 'executing',
        timestamp: incident.updated_at,
        summary: `Executing ${incident.action_type} action`,
        details: {
          backend: incident.execution_result.backend,
          operation: incident.execution_result.operation,
          target: incident.execution_result.target,
        },
      })

      // Resolution/Failure event
      events.push({
        id: `${incident.id}:${incident.status}`,
        incident_id: incident.id,
        type: incident.status,
        timestamp: incident.updated_at,
        summary: incident.status === 'resolved'
          ? 'Incident resolved successfully'
          : 'Incident execution failed',
        details: {
          success: incident.execution_result.success,
          message: incident.execution_result.message,
          execution_id: incident.execution_id,
        },
        metadata: incident.execution_result.details,
      })
    }
  }

  // Sort by timestamp (oldest first)
  events.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())

  return {
    incident_id: incident.id,
    correlation_id: incident.correlation_id,
    events,
    total_events: events.length,
  }
}

// Incident Comments
const COMMENT_PREFIX = 'incident:comment:'
const COMMENT_INDEX_PREFIX = 'incident:comments:'

function commentKey(commentId: string): string {
  return `${COMMENT_PREFIX}${commentId}`
}

function commentIndexKey(incidentId: string): string {
  return `${COMMENT_INDEX_PREFIX}${incidentId}`
}

export async function addIncidentComment(
  env: Env,
  incident: IncidentRecord,
  principal: AuthPrincipal,
  input: IncidentCommentInput
): Promise<IncidentComment> {
  const now = nowIso()
  const commentId = crypto.randomUUID()

  const comment: IncidentComment = {
    id: commentId,
    incident_id: incident.id,
    author_id: principal.sub,
    author_email: principal.email,
    author_role: principal.role,
    content: input.content,
    visibility: input.visibility || 'public',
    created_at: now,
    updated_at: now,
  }

  await env.KV.put(commentKey(commentId), JSON.stringify(comment), { expirationTtl: 86400 * 30 })

  const indexKey = commentIndexKey(incident.id)
  const index = (await env.KV.get(indexKey, 'json') as string[] | null) || []
  index.unshift(commentId)
  await env.KV.put(indexKey, JSON.stringify(index.slice(0, 100)), { expirationTtl: 86400 * 30 })

  return comment
}

export async function getIncidentComment(env: Env, commentId: string): Promise<IncidentComment | null> {
  return await env.KV.get(commentKey(commentId), 'json') as IncidentComment | null
}

export async function listIncidentComments(env: Env, incidentId: string): Promise<IncidentComment[]> {
  const indexKey = commentIndexKey(incidentId)
  const index = (await env.KV.get(indexKey, 'json') as string[] | null) || []

  const comments = await Promise.all(
    index.map(id => getIncidentComment(env, id))
  )

  return comments.filter((c): c is IncidentComment => c !== null)
}

export async function updateIncidentComment(
  env: Env,
  commentId: string,
  principal: AuthPrincipal,
  content: string
): Promise<IncidentComment | null> {
  const comment = await getIncidentComment(env, commentId)
  if (!comment) {
    return null
  }

  // Only the author can update the comment
  if (comment.author_id !== principal.sub) {
    return null
  }

  const updated: IncidentComment = {
    ...comment,
    content,
    updated_at: nowIso(),
  }

  await env.KV.put(commentKey(commentId), JSON.stringify(updated), { expirationTtl: 86400 * 30 })
  return updated
}

export async function deleteIncidentComment(
  env: Env,
  commentId: string,
  principal: AuthPrincipal
): Promise<boolean> {
  const comment = await getIncidentComment(env, commentId)
  if (!comment) {
    return false
  }

  // Only the author or an admin can delete the comment
  if (comment.author_id !== principal.sub && principal.role !== 'admin') {
    return false
  }

  await env.KV.delete(commentKey(commentId))

  // Remove from index
  const indexKey = commentIndexKey(comment.incident_id)
  const index = (await env.KV.get(indexKey, 'json') as string[] | null) || []
  const newIndex = index.filter(id => id !== commentId)
  await env.KV.put(indexKey, JSON.stringify(newIndex), { expirationTtl: 86400 * 30 })

  return true
}

// Incident Statistics
export interface IncidentStatistics {
  total: number
  by_status: Record<IncidentStatus, number>
  by_severity: Record<IncidentSeverity, number>
  by_action_type: Record<string, number>
  by_source: Record<string, number>
  by_auth_method: Record<string, number>
  time_metrics: {
    mean_time_to_approval_minutes: number | null
    mean_time_to_resolution_minutes: number | null
    median_time_to_resolution_minutes: number | null
  }
  trends: {
    daily: Array<{ date: string; created: number; resolved: number }>
    weekly: Array<{ week: string; created: number; resolved: number }>
  }
  action_success_rate: {
    total_executed: number
    successful: number
    failed: number
    success_rate: number
  }
}

export async function getIncidentStatistics(
  env: Env,
  options: { since?: string; until?: string } = {}
): Promise<IncidentStatistics> {
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const validIncidents = incidents.filter((i): i is IncidentRecord => i !== null)

  // Filter by date range if provided
  let filtered = validIncidents
  if (options.since) {
    const sinceDate = new Date(options.since)
    filtered = filtered.filter(i => new Date(i.created_at) >= sinceDate)
  }
  if (options.until) {
    const untilDate = new Date(options.until)
    filtered = filtered.filter(i => new Date(i.created_at) <= untilDate)
  }

  // Count by status
  const byStatus: Record<IncidentStatus, number> = {
    open: 0,
    analyzed: 0,
    approved: 0,
    executing: 0,
    resolved: 0,
    failed: 0,
  }
  for (const i of filtered) {
    byStatus[i.status]++
  }

  // Count by severity
  const bySeverity: Record<IncidentSeverity, number> = {
    low: 0,
    medium: 0,
    high: 0,
    critical: 0,
  }
  for (const i of filtered) {
    bySeverity[i.severity]++
  }

  // Count by action type
  const byActionType: Record<string, number> = {}
  for (const i of filtered) {
    if (i.action_type) {
      byActionType[i.action_type] = (byActionType[i.action_type] || 0) + 1
    }
  }

  // Count by source
  const bySource: Record<string, number> = {}
  for (const i of filtered) {
    bySource[i.source] = (bySource[i.source] || 0) + 1
  }

  // Count by auth method
  const byAuthMethod: Record<string, number> = {}
  for (const i of filtered) {
    byAuthMethod[i.requested_via] = (byAuthMethod[i.requested_via] || 0) + 1
  }

  // Time metrics
  const approvedIncidents = filtered.filter(i => i.approved_at && i.created_at)
  const resolvedIncidents = filtered.filter(i => i.status === 'resolved' && i.execution_result)

  let meanTimeToApproval: number | null = null
  if (approvedIncidents.length > 0) {
    const approvalTimes = approvedIncidents.map(i =>
      (new Date(i.approved_at!).getTime() - new Date(i.created_at).getTime()) / 60000
    )
    meanTimeToApproval = approvalTimes.reduce((a, b) => a + b, 0) / approvalTimes.length
  }

  let meanTimeToResolution: number | null = null
  let medianTimeToResolution: number | null = null
  if (resolvedIncidents.length > 0) {
    const resolutionTimes = resolvedIncidents.map(i =>
      (new Date(i.updated_at).getTime() - new Date(i.created_at).getTime()) / 60000
    )
    meanTimeToResolution = resolutionTimes.reduce((a, b) => a + b, 0) / resolutionTimes.length

    const sorted = [...resolutionTimes].sort((a, b) => a - b)
    const mid = Math.floor(sorted.length / 2)
    medianTimeToResolution = sorted.length % 2 !== 0
      ? sorted[mid]
      : (sorted[mid - 1] + sorted[mid]) / 2
  }

  // Daily trends (last 30 days)
  const daily: Array<{ date: string; created: number; resolved: number }> = []
  const now = new Date()
  for (let d = 29; d >= 0; d--) {
    const date = new Date(now)
    date.setDate(date.getDate() - d)
    const dateStr = date.toISOString().split('T')[0]

    const createdCount = validIncidents.filter(i =>
      i.created_at.startsWith(dateStr)
    ).length

    const resolvedCount = validIncidents.filter(i =>
      i.status === 'resolved' && i.updated_at.startsWith(dateStr)
    ).length

    daily.push({ date: dateStr, created: createdCount, resolved: resolvedCount })
  }

  // Weekly trends (last 12 weeks)
  const weekly: Array<{ week: string; created: number; resolved: number }> = []
  for (let w = 11; w >= 0; w--) {
    const weekStart = new Date(now)
    weekStart.setDate(weekStart.getDate() - (w * 7 + now.getDay()))
    const weekEnd = new Date(weekStart)
    weekEnd.setDate(weekEnd.getDate() + 6)
    const weekStr = weekStart.toISOString().split('T')[0]

    const createdCount = validIncidents.filter(i => {
      const created = new Date(i.created_at)
      return created >= weekStart && created <= weekEnd
    }).length

    const resolvedCount = validIncidents.filter(i => {
      if (i.status !== 'resolved') return false
      const resolved = new Date(i.updated_at)
      return resolved >= weekStart && resolved <= weekEnd
    }).length

    weekly.push({ week: weekStr, created: createdCount, resolved: resolvedCount })
  }

  // Action success rate
  const executed = validIncidents.filter(i => i.execution_result)
  const successful = executed.filter(i => i.execution_result?.success).length
  const failed = executed.filter(i => !i.execution_result?.success).length

  return {
    total: filtered.length,
    by_status: byStatus,
    by_severity: bySeverity,
    by_action_type: byActionType,
    by_source: bySource,
    by_auth_method: byAuthMethod,
    time_metrics: {
      mean_time_to_approval_minutes: meanTimeToApproval,
      mean_time_to_resolution_minutes: meanTimeToResolution,
      median_time_to_resolution_minutes: medianTimeToResolution,
    },
    trends: {
      daily,
      weekly,
    },
    action_success_rate: {
      total_executed: executed.length,
      successful,
      failed,
      success_rate: executed.length > 0 ? (successful / executed.length) * 100 : 0,
    },
  }
}

// Bulk Operations
export interface BulkOperationResult {
  total: number
  successful: string[]
  failed: Array<{ id: string; error: string }>
}

export async function bulkApproveIncidents(
  env: Env,
  principal: AuthPrincipal,
  incidentIds: string[]
): Promise<BulkOperationResult> {
  const result: BulkOperationResult = {
    total: incidentIds.length,
    successful: [],
    failed: [],
  }

  for (const id of incidentIds) {
    try {
      const incident = await getIncident(env, id)
      if (!incident) {
        result.failed.push({ id, error: 'Incident not found' })
        continue
      }

      const canApprove = await canApproveIncident(env, principal, incident)
      if (!canApprove) {
        result.failed.push({ id, error: 'Forbidden: approval policy denies this action' })
        continue
      }

      if (incident.status !== 'open' && incident.status !== 'analyzed') {
        result.failed.push({ id, error: `Cannot approve incident in ${incident.status} state` })
        continue
      }

      await approveIncident(env, incident, principal)
      result.successful.push(id)
    } catch (err) {
      result.failed.push({ id, error: err instanceof Error ? err.message : 'Unknown error' })
    }
  }

  return result
}

export async function bulkExecuteIncidents(
  env: Env,
  principal: AuthPrincipal,
  incidentIds: string[]
): Promise<BulkOperationResult> {
  const result: BulkOperationResult = {
    total: incidentIds.length,
    successful: [],
    failed: [],
  }

  for (const id of incidentIds) {
    try {
      const incident = await getIncident(env, id)
      if (!incident) {
        result.failed.push({ id, error: 'Incident not found' })
        continue
      }

      const canExecute = await canExecuteIncident(env, principal, incident)
      if (!canExecute) {
        result.failed.push({ id, error: 'Forbidden: execution policy denies this action' })
        continue
      }

      if (incident.status !== 'approved') {
        result.failed.push({ id, error: `Cannot execute incident in ${incident.status} state` })
        continue
      }

      const updated = await executeIncident(env, incident)
      if (updated.status === 'resolved') {
        result.successful.push(id)
      } else {
        result.failed.push({ id, error: 'Execution failed' })
      }
    } catch (err) {
      result.failed.push({ id, error: err instanceof Error ? err.message : 'Unknown error' })
    }
  }

  return result
}

export async function bulkAnalyzeIncidents(
  env: Env,
  incidentIds: string[]
): Promise<BulkOperationResult> {
  const result: BulkOperationResult = {
    total: incidentIds.length,
    successful: [],
    failed: [],
  }

  for (const id of incidentIds) {
    try {
      const incident = await getIncident(env, id)
      if (!incident) {
        result.failed.push({ id, error: 'Incident not found' })
        continue
      }

      if (incident.status !== 'open') {
        result.failed.push({ id, error: `Cannot analyze incident in ${incident.status} state` })
        continue
      }

      await analyzeIncident(env, incident)
      result.successful.push(id)
    } catch (err) {
      result.failed.push({ id, error: err instanceof Error ? err.message : 'Unknown error' })
    }
  }

  return result
}

export async function bulkDeleteIncidents(
  env: Env,
  incidentIds: string[]
): Promise<BulkOperationResult> {
  const result: BulkOperationResult = {
    total: incidentIds.length,
    successful: [],
    failed: [],
  }

  for (const id of incidentIds) {
    try {
      const incident = await getIncident(env, id)
      if (!incident) {
        result.failed.push({ id, error: 'Incident not found' })
        continue
      }

      // Only allow deletion of resolved or failed incidents
      if (incident.status !== 'resolved' && incident.status !== 'failed') {
        result.failed.push({ id, error: `Cannot delete incident in ${incident.status} state` })
        continue
      }

      // Delete from D1
      await env.DB.prepare('DELETE FROM incidents WHERE id = ?').bind(id).run()

      // Delete from KV
      await env.KV.delete(incidentKey(id))

      // Remove from index
      const ids = await getIncidentIndex(env)
      const newIds = ids.filter(i => i !== id)
      await setIncidentIndex(env, newIds)

      result.successful.push(id)
    } catch (err) {
      result.failed.push({ id, error: err instanceof Error ? err.message : 'Unknown error' })
    }
  }

  return result
}

// Search Functions
function normalizeText(text: string): string {
  return text.toLowerCase().trim()
}

function tokenize(query: string): string[] {
  return normalizeText(query)
    .split(/\s+/)
    .filter(token => token.length > 1)
}

function matchText(text: string, tokens: string[]): { matches: number; highlights: string[] } {
  const normalized = normalizeText(text)
  const highlights: string[] = []
  let matches = 0

  for (const token of tokens) {
    if (normalized.includes(token)) {
      matches++
      // Extract surrounding context for highlight
      const index = normalized.indexOf(token)
      const start = Math.max(0, index - 20)
      const end = Math.min(normalized.length, index + token.length + 20)
      const context = text.substring(start, end)
      if (context && !highlights.includes(context)) {
        highlights.push(context.length < text.length ? `...${context}...` : context)
      }
    }
  }

  return { matches, highlights }
}

function calculateSearchScore(incident: IncidentRecord, tokens: string[]): { score: number; highlights: string[] } {
  let totalScore = 0
  const allHighlights: string[] = []

  // Title matches are weighted highest
  if (incident.title) {
    const { matches, highlights } = matchText(incident.title, tokens)
    totalScore += matches * 10
    allHighlights.push(...highlights.slice(0, 2))
  }

  // Summary matches
  if (incident.summary) {
    const { matches, highlights } = matchText(incident.summary, tokens)
    totalScore += matches * 5
    allHighlights.push(...highlights.slice(0, 2))
  }

  // Source matches
  const sourceResult = matchText(incident.source, tokens)
  totalScore += sourceResult.matches * 3

  // Evidence content matches
  for (const evidence of incident.evidence) {
    const { matches, highlights } = matchText(evidence.content, tokens)
    totalScore += matches * 2
    allHighlights.push(...highlights.slice(0, 1))
  }

  // Analysis matches
  if (incident.analysis) {
    const analysisStr = JSON.stringify(incident.analysis)
    const { matches, highlights } = matchText(analysisStr, tokens)
    totalScore += matches * 2
    allHighlights.push(...highlights.slice(0, 1))
  }

  // Deduplicate highlights
  const uniqueHighlights = [...new Set(allHighlights)].slice(0, 5)

  return { score: totalScore, highlights: uniqueHighlights }
}

export async function searchIncidents(
  env: Env,
  params: IncidentSearchParams
): Promise<IncidentSearchResult> {
  const page = Math.max(1, params.page || 1)
  const perPage = Math.max(1, Math.min(params.per_page || 20, 100))
  const tokens = tokenize(params.query)

  if (tokens.length === 0) {
    return {
      items: [],
      total: 0,
      page,
      per_page: perPage,
      total_pages: 0,
      query: params.query,
    }
  }

  // Get all incidents
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const validIncidents = incidents.filter((i): i is IncidentRecord => i !== null)

  // Filter by status
  let filtered = validIncidents
  if (params.status?.length) {
    filtered = filtered.filter(i => params.status!.includes(i.status))
  }

  // Filter by severity
  if (params.severity?.length) {
    filtered = filtered.filter(i => params.severity!.includes(i.severity))
  }

  // Filter by action_type
  if (params.action_type?.length) {
    filtered = filtered.filter(i => i.action_type && params.action_type!.includes(i.action_type))
  }

  // Filter by source
  if (params.source?.length) {
    filtered = filtered.filter(i => params.source!.some(s =>
      normalizeText(i.source).includes(normalizeText(s))
    ))
  }

  // Filter by created_after
  if (params.created_after) {
    const afterDate = new Date(params.created_after)
    filtered = filtered.filter(i => new Date(i.created_at) >= afterDate)
  }

  // Filter by created_before
  if (params.created_before) {
    const beforeDate = new Date(params.created_before)
    filtered = filtered.filter(i => new Date(i.created_at) <= beforeDate)
  }

  // Search and score
  const scored = filtered
    .map(incident => {
      const { score, highlights } = calculateSearchScore(incident, tokens)
      return { incident, score, highlights }
    })
    .filter(item => item.score > 0)
    .sort((a, b) => b.score - a.score)

  const paginated = scored.slice((page - 1) * perPage, page * perPage)

  return {
    items: paginated.map(item => ({
      ...item.incident,
      search_score: item.score,
      search_highlights: item.highlights,
    })),
    total: scored.length,
    page,
    per_page: perPage,
    total_pages: Math.max(1, Math.ceil(scored.length / perPage)),
    query: params.query,
  }
}

// Tag Management Functions
export async function addIncidentTags(
  env: Env,
  incident: IncidentRecord,
  tags: string[]
): Promise<IncidentRecord> {
  const uniqueTags = [...new Set([...(incident.tags || []), ...tags])]
    .map(t => t.toLowerCase().trim())
    .filter(t => t.length > 0)

  const updated: IncidentRecord = {
    ...incident,
    tags: uniqueTags,
    updated_at: nowIso(),
  }

  await storeIncident(env, updated)
  return updated
}

export async function removeIncidentTags(
  env: Env,
  incident: IncidentRecord,
  tags: string[]
): Promise<IncidentRecord> {
  const tagsToRemove = tags.map(t => t.toLowerCase().trim())
  const remainingTags = (incident.tags || []).filter(t => !tagsToRemove.includes(t.toLowerCase()))

  const updated: IncidentRecord = {
    ...incident,
    tags: remainingTags,
    updated_at: nowIso(),
  }

  await storeIncident(env, updated)
  return updated
}

export async function setIncidentTags(
  env: Env,
  incident: IncidentRecord,
  tags: string[]
): Promise<IncidentRecord> {
  const uniqueTags = [...new Set(tags.map(t => t.toLowerCase().trim()).filter(t => t.length > 0))]

  const updated: IncidentRecord = {
    ...incident,
    tags: uniqueTags,
    updated_at: nowIso(),
  }

  await storeIncident(env, updated)
  return updated
}

export async function listAllTags(env: Env): Promise<Array<{ tag: string; count: number }>> {
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const validIncidents = incidents.filter((i): i is IncidentRecord => i !== null)

  const tagCounts: Record<string, number> = {}
  for (const incident of validIncidents) {
    for (const tag of incident.tags || []) {
      const normalizedTag = tag.toLowerCase().trim()
      tagCounts[normalizedTag] = (tagCounts[normalizedTag] || 0) + 1
    }
  }

  return Object.entries(tagCounts)
    .map(([tag, count]) => ({ tag, count }))
    .sort((a, b) => b.count - a.count)
}

// SLA thresholds in minutes for each severity
const SLA_THRESHOLDS: Record<IncidentSeverity, { acknowledge: number; resolve: number }> = {
  critical: { acknowledge: 15, resolve: 60 },
  high: { acknowledge: 30, resolve: 240 },
  medium: { acknowledge: 60, resolve: 480 },
  low: { acknowledge: 240, resolve: 1440 },
}

export function calculateSlaDeadline(
  severity: IncidentSeverity,
  createdAt: string
): string {
  const thresholds = SLA_THRESHOLDS[severity]
  const created = new Date(createdAt)
  const deadline = new Date(created.getTime() + thresholds.resolve * 60 * 1000)
  return deadline.toISOString()
}

export async function acknowledgeIncident(
  env: Env,
  incident: IncidentRecord,
  principal: AuthPrincipal
): Promise<IncidentRecord> {
  if (incident.status !== 'open') {
    throw new Error('Can only acknowledge open incidents')
  }

  const now = nowIso()
  const updated: IncidentRecord = {
    ...incident,
    acknowledged_by: principal.sub,
    acknowledged_at: now,
    status: 'analyzed', // Move to analyzed status after acknowledgment
    updated_at: now,
  }

  await storeIncident(env, updated)

  await publishIncidentEvent(env, updated, 'incident.acknowledged')
  await triggerGlobalWebhooks(env, 'incident.analyzed', updated)

  return updated
}

export async function escalateIncident(
  env: Env,
  incident: IncidentRecord,
  escalateToSeverity: IncidentSeverity
): Promise<IncidentRecord> {
  const severityOrder: IncidentSeverity[] = ['low', 'medium', 'high', 'critical']
  const currentIndex = severityOrder.indexOf(incident.severity)
  const escalateToIndex = severityOrder.indexOf(escalateToSeverity)

  if (escalateToIndex <= currentIndex) {
    throw new Error('Can only escalate to a higher severity')
  }

  const now = nowIso()
  const updated: IncidentRecord = {
    ...incident,
    severity: escalateToSeverity,
    escalated_from: incident.severity,
    escalated_at: now,
    sla_deadline: calculateSlaDeadline(escalateToSeverity, incident.created_at),
    updated_at: now,
  }

  await storeIncident(env, updated)

  await publishIncidentEvent(env, updated, 'incident.escalated')

  return updated
}

export async function checkSlaBreaches(env: Env): Promise<IncidentRecord[]> {
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const openIncidents = incidents.filter((i): i is IncidentRecord =>
    i !== null && ['open', 'analyzed', 'approved'].includes(i.status)
  )

  const now = new Date()
  const breached: IncidentRecord[] = []

  for (const incident of openIncidents) {
    const deadline = incident.sla_deadline || calculateSlaDeadline(incident.severity, incident.created_at)
    if (new Date(deadline) < now) {
      breached.push(incident)
    }
  }

  return breached
}

export async function autoEscalateBreachedIncidents(env: Env): Promise<IncidentRecord[]> {
  const breached = await checkSlaBreaches(env)
  const escalated: IncidentRecord[] = []

  const severityOrder: IncidentSeverity[] = ['low', 'medium', 'high', 'critical']

  for (const incident of breached) {
    const currentIndex = severityOrder.indexOf(incident.severity)
    if (currentIndex < severityOrder.length - 1) {
      const newSeverity = severityOrder[currentIndex + 1]
      const escalatedIncident = await escalateIncident(env, incident, newSeverity)
      escalated.push(escalatedIncident)
    }
  }

  return escalated
}

export async function getIncidentSlaStatus(
  env: Env,
  incident: IncidentRecord
): Promise<{
  acknowledged: boolean
  acknowledgedWithinSla: boolean
  resolvedWithinSla: boolean
  timeToAcknowledge?: number
  timeToResolve?: number
  slaDeadline: string
  isBreached: boolean
}> {
  const deadline = incident.sla_deadline || calculateSlaDeadline(incident.severity, incident.created_at)
  const thresholds = SLA_THRESHOLDS[incident.severity]
  const created = new Date(incident.created_at)
  const now = new Date()

  const result = {
    acknowledged: !!incident.acknowledged_at,
    acknowledgedWithinSla: true,
    resolvedWithinSla: true,
    timeToAcknowledge: undefined as number | undefined,
    timeToResolve: undefined as number | undefined,
    slaDeadline: deadline,
    isBreached: false,
  }

  if (incident.acknowledged_at) {
    const acknowledged = new Date(incident.acknowledged_at)
    result.timeToAcknowledge = (acknowledged.getTime() - created.getTime()) / 1000 / 60 // minutes
    result.acknowledgedWithinSla = result.timeToAcknowledge <= thresholds.acknowledge
  }

  if (incident.status === 'resolved' && incident.updated_at) {
    const resolved = new Date(incident.updated_at)
    result.timeToResolve = (resolved.getTime() - created.getTime()) / 1000 / 60 // minutes
    result.resolvedWithinSla = result.timeToResolve <= thresholds.resolve
  }

  result.isBreached = new Date(deadline) < now && !['resolved', 'failed'].includes(incident.status)

  return result
}

export async function assignIncident(
  env: Env,
  incident: IncidentRecord,
  assigneeId: number,
  assigneeEmail?: string
): Promise<IncidentRecord> {
  const now = nowIso()
  const updated: IncidentRecord = {
    ...incident,
    assignee_id: assigneeId,
    assignee_email: assigneeEmail,
    assigned_at: now,
    updated_at: now,
  }

  await storeIncident(env, updated)

  await publishIncidentEvent(env, updated, 'incident.assigned')

  return updated
}

export async function unassignIncident(
  env: Env,
  incident: IncidentRecord
): Promise<IncidentRecord> {
  const now = nowIso()
  const updated: IncidentRecord = {
    ...incident,
    assignee_id: undefined,
    assignee_email: undefined,
    assigned_at: undefined,
    updated_at: now,
  }

  await storeIncident(env, updated)

  return updated
}

export async function listAssignedIncidents(
  env: Env,
  assigneeId: number
): Promise<IncidentRecord[]> {
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  return incidents.filter((i): i is IncidentRecord =>
    i !== null && i.assignee_id === assigneeId && !['resolved', 'failed'].includes(i.status)
  )
}

// ==================== Deduplication ====================

const DEFAULT_DEDUP_WINDOW_MINUTES = 60

export async function findDuplicateIncident(
  env: Env,
  correlationId: string
): Promise<IncidentRecord | null> {
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.slice(0, 100).map(id => getIncident(env, id)))

  const cutoff = new Date(Date.now() - DEFAULT_DEDUP_WINDOW_MINUTES * 60 * 1000)

  for (const incident of incidents) {
    if (!incident) continue
    if (incident.correlation_id !== correlationId) continue
    if (['resolved', 'failed'].includes(incident.status)) continue
    if (new Date(incident.created_at) < cutoff) continue

    return incident
  }

  return null
}

export async function incrementDuplicateCount(
  env: Env,
  incident: IncidentRecord
): Promise<IncidentRecord> {
  const now = nowIso()
  const updated: IncidentRecord = {
    ...incident,
    duplicate_count: (incident.duplicate_count || 1) + 1,
    updated_at: now,
  }

  await storeIncident(env, updated)
  return updated
}

// ==================== Suppression Rules ====================

async function getSuppressionRules(env: Env): Promise<import('../types').SuppressionRule[]> {
  const rules = await env.KV.get(SUPPRESSION_RULES_KEY, 'json')
  return (rules as import('../types').SuppressionRule[] | null) || []
}

async function setSuppressionRules(env: Env, rules: import('../types').SuppressionRule[]): Promise<void> {
  await env.KV.put(SUPPRESSION_RULES_KEY, JSON.stringify(rules), { expirationTtl: 86400 * 30 })
}

export async function createSuppressionRule(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    conditions: import('../types').SuppressionRule['conditions']
    duration_minutes: number
  }
): Promise<import('../types').SuppressionRule> {
  const now = nowIso()
  const rule: import('../types').SuppressionRule = {
    id: crypto.randomUUID(),
    name: input.name,
    description: input.description,
    enabled: true,
    conditions: input.conditions,
    duration_minutes: input.duration_minutes,
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
    expires_at: new Date(Date.now() + input.duration_minutes * 60 * 1000).toISOString(),
  }

  const rules = await getSuppressionRules(env)
  rules.push(rule)
  await setSuppressionRules(env, rules)

  return rule
}

export async function listSuppressionRules(env: Env): Promise<import('../types').SuppressionRule[]> {
  return getSuppressionRules(env)
}

export async function deleteSuppressionRule(env: Env, ruleId: string): Promise<boolean> {
  const rules = await getSuppressionRules(env)
  const index = rules.findIndex(r => r.id === ruleId)
  if (index === -1) return false

  rules.splice(index, 1)
  await setSuppressionRules(env, rules)
  return true
}

export async function toggleSuppressionRule(env: Env, ruleId: string, enabled: boolean): Promise<import('../types').SuppressionRule | null> {
  const rules = await getSuppressionRules(env)
  const rule = rules.find(r => r.id === ruleId)
  if (!rule) return null

  rule.enabled = enabled
  rule.updated_at = nowIso()
  if (enabled) {
    rule.expires_at = new Date(Date.now() + rule.duration_minutes * 60 * 1000).toISOString()
  }

  await setSuppressionRules(env, rules)
  return rule
}

export function matchesSuppressionRule(
  incident: CreateIncidentInput,
  rule: import('../types').SuppressionRule
): boolean {
  if (!rule.enabled) return false
  if (rule.expires_at && new Date(rule.expires_at) < new Date()) return false

  const conditions = rule.conditions

  if (conditions.severity?.length && incident.severity) {
    if (!conditions.severity.includes(incident.severity)) return false
  }

  if (conditions.source?.length) {
    if (!conditions.source.includes(incident.source)) return false
  }

  if (conditions.action_type?.length && incident.action_type) {
    if (!conditions.action_type.includes(incident.action_type)) return false
  }

  if (conditions.title_pattern && incident.title) {
    try {
      const regex = new RegExp(conditions.title_pattern, 'i')
      if (!regex.test(incident.title)) return false
    } catch {
      // Invalid regex, skip this condition
    }
  }

  if (conditions.correlation_id_pattern && incident.correlation_id) {
    try {
      const regex = new RegExp(conditions.correlation_id_pattern, 'i')
      if (!regex.test(incident.correlation_id)) return false
    } catch {
      // Invalid regex, skip this condition
    }
  }

  return true
}

export async function findMatchingSuppressionRule(
  env: Env,
  input: CreateIncidentInput
): Promise<import('../types').SuppressionRule | null> {
  const rules = await getSuppressionRules(env)

  for (const rule of rules) {
    if (matchesSuppressionRule(input, rule)) {
      return rule
    }
  }

  return null
}

// ==================== Incident Merging ====================

export interface MergeResult {
  primary: IncidentRecord
  merged: IncidentRecord[]
  merged_count: number
}

export async function mergeIncidents(
  env: Env,
  primaryId: string,
  incidentIds: string[]
): Promise<MergeResult> {
  // Get the primary incident
  const primary = await getIncident(env, primaryId)
  if (!primary) {
    throw new Error('Primary incident not found')
  }

  if (['resolved', 'failed'].includes(primary.status)) {
    throw new Error('Cannot merge into a resolved or failed incident')
  }

  // Get incidents to merge
  const toMerge: IncidentRecord[] = []
  for (const id of incidentIds) {
    if (id === primaryId) continue // Skip primary

    const incident = await getIncident(env, id)
    if (!incident) continue
    if (['resolved', 'failed'].includes(incident.status)) continue

    toMerge.push(incident)
  }

  if (toMerge.length === 0) {
    return { primary, merged: [], merged_count: 0 }
  }

  const now = nowIso()

  // Merge evidence
  const allEvidence = [...primary.evidence]
  for (const incident of toMerge) {
    for (const evidence of incident.evidence) {
      // Avoid duplicate evidence
      const exists = allEvidence.some(e =>
        e.type === evidence.type && e.source === evidence.source && e.content === evidence.content
      )
      if (!exists) {
        allEvidence.push(evidence)
      }
    }
  }

  // Merge tags
  const allTags = new Set(primary.tags || [])
  for (const incident of toMerge) {
    for (const tag of incident.tags || []) {
      allTags.add(tag)
    }
  }

  // Merge recommendations
  const allRecommendations = [...primary.recommendations]
  for (const incident of toMerge) {
    for (const rec of incident.recommendations) {
      const exists = allRecommendations.some(r => r.id === rec.id)
      if (!exists) {
        allRecommendations.push(rec)
      }
    }
  }

  // Calculate total duplicate count
  const totalDuplicates = (primary.duplicate_count || 1) +
    toMerge.reduce((sum, i) => sum + (i.duplicate_count || 1), 0)

  // Determine highest severity
  const severityOrder: IncidentSeverity[] = ['low', 'medium', 'high', 'critical']
  let highestSeverity = primary.severity
  for (const incident of toMerge) {
    if (severityOrder.indexOf(incident.severity) > severityOrder.indexOf(highestSeverity)) {
      highestSeverity = incident.severity
    }
  }

  // Update primary incident
  const updatedPrimary: IncidentRecord = {
    ...primary,
    severity: highestSeverity,
    evidence: allEvidence,
    recommendations: allRecommendations,
    tags: Array.from(allTags),
    duplicate_count: totalDuplicates,
    updated_at: now,
  }

  await storeIncident(env, updatedPrimary)

  // Mark merged incidents as resolved
  const merged: IncidentRecord[] = []
  for (const incident of toMerge) {
    const mergedIncident: IncidentRecord = {
      ...incident,
      status: 'resolved',
      duplicate_of: primaryId,
      updated_at: now,
    }
    await storeIncident(env, mergedIncident)
    merged.push(mergedIncident)

    // Publish merge event for each merged incident
    publishIncidentEvent(env, mergedIncident, 'incident.merged')
  }

  // Publish update for primary
  publishIncidentEvent(env, updatedPrimary, 'incident.updated')

  return {
    primary: updatedPrimary,
    merged,
    merged_count: merged.length,
  }
}

// ==================== Notification Rules ====================

const NOTIFICATION_RULES_KEY = `${INCIDENT_PREFIX}notification_rules`

async function getNotificationRules(env: Env): Promise<import('../types').NotificationRule[]> {
  const rules = await env.KV.get(NOTIFICATION_RULES_KEY, 'json')
  return (rules as import('../types').NotificationRule[] | null) || []
}

async function setNotificationRules(env: Env, rules: import('../types').NotificationRule[]): Promise<void> {
  await env.KV.put(NOTIFICATION_RULES_KEY, JSON.stringify(rules), { expirationTtl: 86400 * 30 })
}

export async function createNotificationRule(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    conditions: import('../types').NotificationRule['conditions']
    channels: import('../types').NotificationRule['channels']
    recipients: string[]
    template?: string
  }
): Promise<import('../types').NotificationRule> {
  const now = nowIso()
  const rule: import('../types').NotificationRule = {
    id: crypto.randomUUID(),
    name: input.name,
    description: input.description,
    enabled: true,
    conditions: input.conditions,
    channels: input.channels,
    recipients: input.recipients,
    template: input.template,
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  const rules = await getNotificationRules(env)
  rules.push(rule)
  await setNotificationRules(env, rules)

  return rule
}

export async function listNotificationRules(env: Env): Promise<import('../types').NotificationRule[]> {
  return getNotificationRules(env)
}

export async function deleteNotificationRule(env: Env, ruleId: string): Promise<boolean> {
  const rules = await getNotificationRules(env)
  const index = rules.findIndex(r => r.id === ruleId)
  if (index === -1) return false

  rules.splice(index, 1)
  await setNotificationRules(env, rules)
  return true
}

export async function toggleNotificationRule(
  env: Env,
  ruleId: string,
  enabled: boolean
): Promise<import('../types').NotificationRule | null> {
  const rules = await getNotificationRules(env)
  const rule = rules.find(r => r.id === ruleId)
  if (!rule) return null

  rule.enabled = enabled
  rule.updated_at = nowIso()

  await setNotificationRules(env, rules)
  return rule
}

export function matchesNotificationRule(
  incident: IncidentRecord,
  rule: import('../types').NotificationRule
): boolean {
  if (!rule.enabled) return false

  const conditions = rule.conditions

  if (conditions.severity?.length && incident.severity) {
    if (!conditions.severity.includes(incident.severity)) return false
  }

  if (conditions.source?.length) {
    if (!conditions.source.includes(incident.source)) return false
  }

  if (conditions.action_type?.length && incident.action_type) {
    if (!conditions.action_type.includes(incident.action_type)) return false
  }

  if (conditions.status?.length) {
    if (!conditions.status.includes(incident.status)) return false
  }

  if (conditions.tags?.length && incident.tags?.length) {
    const hasMatchingTag = conditions.tags.some(tag =>
      incident.tags.includes(tag)
    )
    if (!hasMatchingTag) return false
  }

  return true
}

export async function findMatchingNotificationRules(
  env: Env,
  incident: IncidentRecord
): Promise<import('../types').NotificationRule[]> {
  const rules = await getNotificationRules(env)
  return rules.filter(rule => matchesNotificationRule(incident, rule))
}

// ==================== Reports ====================

export async function generateIncidentReport(
  env: Env,
  startDate: string,
  endDate: string
): Promise<import('../types').IncidentReport> {
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const validIncidents = incidents.filter((i): i is IncidentRecord => i !== null)

  // Filter by date range
  const start = new Date(startDate)
  const end = new Date(endDate)
  const filteredIncidents = validIncidents.filter(i => {
    const created = new Date(i.created_at)
    return created >= start && created <= end
  })

  const totalIncidents = filteredIncidents.length
  const openIncidents = filteredIncidents.filter(i => !['resolved', 'failed'].includes(i.status)).length
  const resolvedIncidents = filteredIncidents.filter(i => i.status === 'resolved').length

  // Calculate MTTR (Mean Time To Resolve)
  const resolvedWithTime = filteredIncidents.filter(i =>
    i.status === 'resolved' && i.created_at && i.updated_at
  )
  const avgResolutionTime = resolvedWithTime.length > 0
    ? resolvedWithTime.reduce((sum, i) => {
        const created = new Date(i.created_at)
        const resolved = new Date(i.updated_at)
        return sum + (resolved.getTime() - created.getTime()) / 1000 / 60
      }, 0) / resolvedWithTime.length
    : 0

  // SLA calculations
  const breachedIncidents = await checkSlaBreaches(env)
  const slaBreachCount = breachedIncidents.length
  const slaComplianceRate = totalIncidents > 0
    ? ((totalIncidents - slaBreachCount) / totalIncidents) * 100
    : 100

  // By severity
  const bySeverity: Record<IncidentSeverity, { count: number; percentage: number }> = {
    low: { count: 0, percentage: 0 },
    medium: { count: 0, percentage: 0 },
    high: { count: 0, percentage: 0 },
    critical: { count: 0, percentage: 0 },
  }
  for (const incident of filteredIncidents) {
    bySeverity[incident.severity].count++
  }
  for (const severity of Object.keys(bySeverity) as IncidentSeverity[]) {
    bySeverity[severity].percentage = totalIncidents > 0
      ? (bySeverity[severity].count / totalIncidents) * 100
      : 0
  }

  // By status
  const byStatus: Record<IncidentStatus, { count: number; percentage: number }> = {
    open: { count: 0, percentage: 0 },
    analyzed: { count: 0, percentage: 0 },
    approved: { count: 0, percentage: 0 },
    executing: { count: 0, percentage: 0 },
    resolved: { count: 0, percentage: 0 },
    failed: { count: 0, percentage: 0 },
  }
  for (const incident of filteredIncidents) {
    byStatus[incident.status].count++
  }
  for (const status of Object.keys(byStatus) as IncidentStatus[]) {
    byStatus[status].percentage = totalIncidents > 0
      ? (byStatus[status].count / totalIncidents) * 100
      : 0
  }

  // By source
  const sourceCounts: Record<string, number> = {}
  for (const incident of filteredIncidents) {
    sourceCounts[incident.source] = (sourceCounts[incident.source] || 0) + 1
  }
  const bySource = Object.entries(sourceCounts)
    .map(([source, count]) => ({
      source,
      count,
      percentage: totalIncidents > 0 ? (count / totalIncidents) * 100 : 0,
    }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 10)

  // By action type
  const actionTypeStats: Record<string, { count: number; successCount: number }> = {}
  for (const incident of filteredIncidents) {
    if (incident.action_type) {
      if (!actionTypeStats[incident.action_type]) {
        actionTypeStats[incident.action_type] = { count: 0, successCount: 0 }
      }
      actionTypeStats[incident.action_type].count++
      if (incident.status === 'resolved') {
        actionTypeStats[incident.action_type].successCount++
      }
    }
  }
  const byActionType = Object.entries(actionTypeStats)
    .map(([action_type, stats]) => ({
      action_type,
      count: stats.count,
      success_rate: stats.count > 0 ? (stats.successCount / stats.count) * 100 : 0,
    }))
    .sort((a, b) => b.count - a.count)

  // Top tags
  const tagCounts: Record<string, number> = {}
  for (const incident of filteredIncidents) {
    for (const tag of incident.tags || []) {
      tagCounts[tag] = (tagCounts[tag] || 0) + 1
    }
  }
  const topTags = Object.entries(tagCounts)
    .map(([tag, count]) => ({ tag, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 10)

  // MTTR by severity
  const mttrBySeverity: Record<IncidentSeverity, number> = {
    low: 0,
    medium: 0,
    high: 0,
    critical: 0,
  }
  for (const severity of Object.keys(mttrBySeverity) as IncidentSeverity[]) {
    const severityIncidents = resolvedWithTime.filter(i => i.severity === severity)
    if (severityIncidents.length > 0) {
      mttrBySeverity[severity] = severityIncidents.reduce((sum, i) => {
        const created = new Date(i.created_at)
        const resolved = new Date(i.updated_at)
        return sum + (resolved.getTime() - created.getTime()) / 1000 / 60
      }, 0) / severityIncidents.length
    }
  }

  // Trends (daily for the period)
  const trends: import('../types').IncidentTrend[] = []
  const currentDate = new Date(start)
  while (currentDate <= end) {
    const dateStr = currentDate.toISOString().split('T')[0]
    const dayIncidents = filteredIncidents.filter(i =>
      i.created_at.startsWith(dateStr)
    )
    const dayResolved = dayIncidents.filter(i => i.status === 'resolved')

    const dayBySeverity: Record<IncidentSeverity, number> = {
      low: 0, medium: 0, high: 0, critical: 0,
    }
    const dayByStatus: Record<IncidentStatus, number> = {
      open: 0, analyzed: 0, approved: 0, executing: 0, resolved: 0, failed: 0,
    }

    for (const incident of dayIncidents) {
      dayBySeverity[incident.severity]++
      dayByStatus[incident.status]++
    }

    trends.push({
      date: dateStr,
      total: dayIncidents.length,
      by_severity: dayBySeverity,
      by_status: dayByStatus,
      resolved_count: dayResolved.length,
      avg_resolution_time_minutes: dayResolved.length > 0
        ? dayResolved.reduce((sum, i) => {
            const created = new Date(i.created_at)
            const resolved = new Date(i.updated_at)
            return sum + (resolved.getTime() - created.getTime()) / 1000 / 60
          }, 0) / dayResolved.length
        : 0,
    })

    currentDate.setDate(currentDate.getDate() + 1)
  }

  return {
    period: { start: startDate, end: endDate },
    summary: {
      total_incidents: totalIncidents,
      open_incidents: openIncidents,
      resolved_incidents: resolvedIncidents,
      avg_resolution_time_minutes: avgResolutionTime,
      sla_breach_count: slaBreachCount,
      sla_compliance_rate: slaComplianceRate,
    },
    by_severity: bySeverity,
    by_status: byStatus,
    by_source: bySource,
    by_action_type: byActionType,
    trends,
    top_tags: topTags,
    top_sources: bySource.slice(0, 5),
    mttr_by_severity: mttrBySeverity,
  }
}

// ==================== Link Management ====================

export async function addIncidentLink(
  env: Env,
  incident: IncidentRecord,
  link: IncidentLink
): Promise<IncidentRecord> {
  const now = nowIso()
  const existingLinks = incident.links || []

  // Check if link already exists
  const exists = existingLinks.some(l => l.kind === link.kind && l.id === link.id)
  if (exists) {
    return incident
  }

  const updated: IncidentRecord = {
    ...incident,
    links: [...existingLinks, link],
    updated_at: now,
  }

  await storeIncident(env, updated)
  publishIncidentEvent(env, updated, 'incident.link_added')

  return updated
}

export async function removeIncidentLink(
  env: Env,
  incident: IncidentRecord,
  linkKind: IncidentLink['kind'],
  linkId: string
): Promise<IncidentRecord> {
  const now = nowIso()
  const existingLinks = incident.links || []

  const updated: IncidentRecord = {
    ...incident,
    links: existingLinks.filter(l => !(l.kind === linkKind && l.id === linkId)),
    updated_at: now,
  }

  await storeIncident(env, updated)

  return updated
}

// ==================== Evidence Management ====================

export async function addIncidentEvidence(
  env: Env,
  incident: IncidentRecord,
  evidence: IncidentEvidence
): Promise<IncidentRecord> {
  const now = nowIso()

  const updated: IncidentRecord = {
    ...incident,
    evidence: [...incident.evidence, evidence],
    updated_at: now,
  }

  await storeIncident(env, updated)
  publishIncidentEvent(env, updated, 'incident.evidence_added')

  return updated
}

// ==================== Activity Log ====================

const ACTIVITY_LOG_PREFIX = 'incident:activity:'

async function getActivityLogKey(incidentId: string): Promise<string> {
  return `${ACTIVITY_LOG_PREFIX}${incidentId}`
}

export interface ActivityLogEntry {
  id: string
  incident_id: string
  timestamp: string
  action: string
  actor?: {
    user_id: number
    email?: string
  }
  details?: Record<string, unknown>
}

export async function logIncidentActivity(
  env: Env,
  incidentId: string,
  action: string,
  actor?: { user_id: number; email?: string },
  details?: Record<string, unknown>
): Promise<ActivityLogEntry> {
  const entry: ActivityLogEntry = {
    id: crypto.randomUUID(),
    incident_id: incidentId,
    timestamp: nowIso(),
    action,
    actor,
    details,
  }

  const key = await getActivityLogKey(incidentId)
  const existing = await env.KV.get(key, 'json') as ActivityLogEntry[] | null
  const logs = existing || []
  logs.unshift(entry)

  // Keep last 100 entries
  await env.KV.put(key, JSON.stringify(logs.slice(0, 100)), { expirationTtl: 86400 * 30 })

  return entry
}

export async function getIncidentActivityLog(
  env: Env,
  incidentId: string
): Promise<ActivityLogEntry[]> {
  const key = await getActivityLogKey(incidentId)
  const logs = await env.KV.get(key, 'json') as ActivityLogEntry[] | null
  return logs || []
}

// ==================== Runbook Integration ====================

export async function suggestRunbooks(
  env: Env,
  incident: IncidentRecord
): Promise<Array<{ id: string; name: string; relevance: number }>> {
  // Get all playbooks
  const playbooksKey = 'playbook:index'
  const playbookIds = (await env.KV.get(playbooksKey, 'json') as string[] | null) || []

  const suggestions: Array<{ id: string; name: string; relevance: number }> = []

  // Simple relevance scoring based on tags and keywords
  const keywords = [
    incident.title.toLowerCase(),
    incident.summary?.toLowerCase() || '',
    incident.source.toLowerCase(),
    ...incident.tags.map(t => t.toLowerCase()),
  ].join(' ')

  for (const playbookId of playbookIds.slice(0, 20)) {
    const playbook = await env.KV.get(`playbook:${playbookId}`, 'json') as {
      id: number
      name: string
      category?: string
      tags?: string
    } | null

    if (!playbook) continue

    let relevance = 0

    // Check category match
    if (incident.action_type === 'restart_deployment' && playbook.category?.includes('kubernetes')) {
      relevance += 50
    }
    if (incident.action_type === 'scale_deployment' && playbook.category?.includes('scaling')) {
      relevance += 50
    }

    // Check tag matches
    const playbookTags = (playbook.tags || '').toLowerCase()
    for (const tag of incident.tags) {
      if (playbookTags.includes(tag.toLowerCase())) {
        relevance += 20
      }
    }

    // Check keyword matches
    const playbookName = playbook.name.toLowerCase()
    for (const keyword of incident.tags) {
      if (playbookName.includes(keyword.toLowerCase())) {
        relevance += 10
      }
    }

    if (relevance > 0) {
      suggestions.push({
        id: String(playbook.id),
        name: playbook.name,
        relevance,
      })
    }
  }

  return suggestions.sort((a, b) => b.relevance - a.relevance).slice(0, 5)
}

export async function executeRunbookForIncident(
  env: Env,
  incident: IncidentRecord,
  playbookId: number,
  principal: AuthPrincipal
): Promise<{ task_id: string; status: string }> {
  // This would typically create a task to execute the playbook
  // For now, we log the activity and return a mock task ID

  const taskId = crypto.randomUUID()

  await logIncidentActivity(env, incident.id, 'runbook_executed', {
    user_id: principal.sub,
    email: principal.email,
  }, {
    playbook_id: playbookId,
    task_id: taskId,
  })

  return {
    task_id: taskId,
    status: 'pending',
  }
}

// ==================== Incident Templates ====================

const TEMPLATES_KEY = `${INCIDENT_PREFIX}templates`

async function getTemplates(env: Env): Promise<import('../types').IncidentTemplate[]> {
  const templates = await env.KV.get(TEMPLATES_KEY, 'json')
  return (templates as import('../types').IncidentTemplate[] | null) || []
}

async function setTemplates(env: Env, templates: import('../types').IncidentTemplate[]): Promise<void> {
  await env.KV.put(TEMPLATES_KEY, JSON.stringify(templates), { expirationTtl: 86400 * 30 })
}

export async function createIncidentTemplate(
  env: Env,
  principal: AuthPrincipal,
  input: Omit<import('../types').IncidentTemplate, 'id' | 'created_by' | 'created_at' | 'updated_at'>
): Promise<import('../types').IncidentTemplate> {
  const now = nowIso()
  const template: import('../types').IncidentTemplate = {
    ...input,
    id: crypto.randomUUID(),
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  const templates = await getTemplates(env)
  templates.push(template)
  await setTemplates(env, templates)

  return template
}

export async function listIncidentTemplates(env: Env): Promise<import('../types').IncidentTemplate[]> {
  return getTemplates(env)
}

export async function getIncidentTemplate(env: Env, id: string): Promise<import('../types').IncidentTemplate | null> {
  const templates = await getTemplates(env)
  return templates.find(t => t.id === id) || null
}

export async function updateIncidentTemplate(
  env: Env,
  id: string,
  updates: Partial<Omit<import('../types').IncidentTemplate, 'id' | 'created_by' | 'created_at'>>
): Promise<import('../types').IncidentTemplate | null> {
  const templates = await getTemplates(env)
  const index = templates.findIndex(t => t.id === id)
  if (index === -1) return null

  templates[index] = {
    ...templates[index],
    ...updates,
    updated_at: nowIso(),
  }

  await setTemplates(env, templates)
  return templates[index]
}

export async function deleteIncidentTemplate(env: Env, id: string): Promise<boolean> {
  const templates = await getTemplates(env)
  const index = templates.findIndex(t => t.id === id)
  if (index === -1) return false

  templates.splice(index, 1)
  await setTemplates(env, templates)
  return true
}

export async function createIncidentFromTemplate(
  env: Env,
  principal: AuthPrincipal,
  templateId: string,
  variables: Record<string, string> = {}
): Promise<IncidentRecord> {
  const template = await getIncidentTemplate(env, templateId)
  if (!template) {
    throw new Error('Template not found')
  }

  // Replace variables in templates
  let title = template.title_template
  let summary = template.summary_template || ''

  for (const [key, value] of Object.entries(variables)) {
    const regex = new RegExp(`\\{\\{${key}\\}\\}`, 'g')
    title = title.replace(regex, value)
    summary = summary.replace(regex, value)
  }

  const input: CreateIncidentInput = {
    title,
    summary,
    severity: template.default_severity,
    source: template.default_source,
    action_type: template.default_action_type,
    action_ref: template.default_action_ref,
    tags: template.default_tags,
    evidence: [],
  }

  return createIncident(env, principal, input)
}

// ==================== Automation Rules ====================

const AUTOMATION_RULES_KEY = `${INCIDENT_PREFIX}automation_rules`

async function getAutomationRules(env: Env): Promise<import('../types').AutomationRule[]> {
  const rules = await env.KV.get(AUTOMATION_RULES_KEY, 'json')
  return (rules as import('../types').AutomationRule[] | null) || []
}

async function setAutomationRules(env: Env, rules: import('../types').AutomationRule[]): Promise<void> {
  await env.KV.put(AUTOMATION_RULES_KEY, JSON.stringify(rules), { expirationTtl: 86400 * 30 })
}

export async function createAutomationRule(
  env: Env,
  principal: AuthPrincipal,
  input: Omit<import('../types').AutomationRule, 'id' | 'created_by' | 'created_at' | 'updated_at'>
): Promise<import('../types').AutomationRule> {
  const now = nowIso()
  const rule: import('../types').AutomationRule = {
    ...input,
    id: crypto.randomUUID(),
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  const rules = await getAutomationRules(env)
  rules.push(rule)
  rules.sort((a, b) => b.priority - a.priority)
  await setAutomationRules(env, rules)

  return rule
}

export async function listAutomationRules(env: Env): Promise<import('../types').AutomationRule[]> {
  return getAutomationRules(env)
}

export async function deleteAutomationRule(env: Env, id: string): Promise<boolean> {
  const rules = await getAutomationRules(env)
  const index = rules.findIndex(r => r.id === id)
  if (index === -1) return false

  rules.splice(index, 1)
  await setAutomationRules(env, rules)
  return true
}

export async function toggleAutomationRule(env: Env, id: string, enabled: boolean): Promise<import('../types').AutomationRule | null> {
  const rules = await getAutomationRules(env)
  const rule = rules.find(r => r.id === id)
  if (!rule) return null

  rule.enabled = enabled
  rule.updated_at = nowIso()

  await setAutomationRules(env, rules)
  return rule
}

function matchesAutomationConditions(
  incident: IncidentRecord,
  conditions: import('../types').AutomationRule['conditions']
): boolean {
  if (conditions.severity?.length && !conditions.severity.includes(incident.severity)) {
    return false
  }

  if (conditions.source?.length && !conditions.source.includes(incident.source)) {
    return false
  }

  if (conditions.action_type?.length && incident.action_type && !conditions.action_type.includes(incident.action_type)) {
    return false
  }

  if (conditions.tags?.length && incident.tags?.length) {
    const hasMatchingTag = conditions.tags.some(tag => incident.tags.includes(tag))
    if (!hasMatchingTag) return false
  }

  if (conditions.time_range) {
    const now = new Date()
    const hour = now.getHours()
    if (hour < conditions.time_range.start_hour || hour >= conditions.time_range.end_hour) {
      return false
    }
  }

  return true
}

export async function executeAutomationRules(
  env: Env,
  trigger: import('../types').AutomationTrigger,
  incident: IncidentRecord,
  principal: AuthPrincipal
): Promise<Array<{ rule_id: string; rule_name: string; actions_executed: string[] }>> {
  const rules = await getAutomationRules(env)
  const results: Array<{ rule_id: string; rule_name: string; actions_executed: string[] }> = []

  for (const rule of rules) {
    if (!rule.enabled) continue
    if (rule.trigger !== trigger) continue
    if (!matchesAutomationConditions(incident, rule.conditions)) continue

    const actionsExecuted: string[] = []

    for (const action of rule.actions) {
      try {
        switch (action.type) {
          case 'assign':
            await assignIncident(env, incident, action.assignee_id)
            actionsExecuted.push(`assign:${action.assignee_id}`)
            break

          case 'escalate':
            await escalateIncident(env, incident, action.target_severity)
            actionsExecuted.push(`escalate:${action.target_severity}`)
            break

          case 'add_tags':
            await addIncidentTags(env, incident, action.tags)
            actionsExecuted.push(`add_tags:${action.tags.join(',')}`)
            break

          case 'notify':
            // Log notification activity
            await logIncidentActivity(env, incident.id, 'notification_sent', {
              user_id: principal.sub,
            }, {
              channels: action.channels,
              recipients: action.recipients,
            })
            actionsExecuted.push(`notify:${action.channels.join(',')}`)
            break

          case 'execute_runbook':
            await executeRunbookForIncident(env, incident, action.playbook_id, principal)
            actionsExecuted.push(`execute_runbook:${action.playbook_id}`)
            break

          case 'set_sla':
            const newDeadline = new Date(Date.now() + action.minutes * 60 * 1000).toISOString()
            const updated: IncidentRecord = {
              ...incident,
              sla_deadline: newDeadline,
              updated_at: nowIso(),
            }
            await storeIncident(env, updated)
            actionsExecuted.push(`set_sla:${action.minutes}`)
            break

          case 'add_comment':
            await addIncidentComment(env, incident, principal, { content: action.content })
            actionsExecuted.push('add_comment')
            break
        }
      } catch (err) {
        console.error(`Failed to execute automation action: ${action.type}`, err)
      }
    }

    if (actionsExecuted.length > 0) {
      results.push({
        rule_id: rule.id,
        rule_name: rule.name,
        actions_executed: actionsExecuted,
      })
    }
  }

  return results
}

// ==================== Post-Mortems ====================

const POSTMORTEM_PREFIX = `${INCIDENT_PREFIX}postmortem:`

function postMortemKey(incidentId: string): string {
  return `${POSTMORTEM_PREFIX}${incidentId}`
}

export async function createPostMortem(
  env: Env,
  principal: AuthPrincipal,
  incidentId: string,
  input: Omit<import('../types').IncidentPostMortem, 'id' | 'incident_id' | 'created_by' | 'created_at' | 'updated_at'>
): Promise<import('../types').IncidentPostMortem> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  const now = nowIso()
  const postMortem: import('../types').IncidentPostMortem = {
    ...input,
    id: crypto.randomUUID(),
    incident_id: incidentId,
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  await env.KV.put(postMortemKey(incidentId), JSON.stringify(postMortem), { expirationTtl: 86400 * 90 })

  // Log activity
  await logIncidentActivity(env, incidentId, 'postmortem_created', {
    user_id: principal.sub,
    email: principal.email,
  }, {
    postmortem_id: postMortem.id,
    title: postMortem.title,
  })

  return postMortem
}

export async function getPostMortem(env: Env, incidentId: string): Promise<import('../types').IncidentPostMortem | null> {
  return await env.KV.get(postMortemKey(incidentId), 'json') as import('../types').IncidentPostMortem | null
}

export async function updatePostMortem(
  env: Env,
  incidentId: string,
  updates: Partial<Omit<import('../types').IncidentPostMortem, 'id' | 'incident_id' | 'created_by' | 'created_at'>>
): Promise<import('../types').IncidentPostMortem | null> {
  const existing = await getPostMortem(env, incidentId)
  if (!existing) return null

  const updated: import('../types').IncidentPostMortem = {
    ...existing,
    ...updates,
    updated_at: nowIso(),
  }

  await env.KV.put(postMortemKey(incidentId), JSON.stringify(updated), { expirationTtl: 86400 * 90 })

  return updated
}

export async function updateActionItemStatus(
  env: Env,
  incidentId: string,
  actionItemId: string,
  status: 'open' | 'in_progress' | 'completed'
): Promise<import('../types').IncidentPostMortem | null> {
  const postMortem = await getPostMortem(env, incidentId)
  if (!postMortem) return null

  const actionItem = postMortem.action_items.find(item => item.id === actionItemId)
  if (!actionItem) return null

  actionItem.status = status
  postMortem.updated_at = nowIso()

  await env.KV.put(postMortemKey(incidentId), JSON.stringify(postMortem), { expirationTtl: 86400 * 90 })

  return postMortem
}

// ==================== Dashboard Metrics ====================

export async function getIncidentDashboardMetrics(env: Env): Promise<import('../types').IncidentDashboardMetrics> {
  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const validIncidents = incidents.filter((i): i is IncidentRecord => i !== null)

  const now = new Date()
  const last24h = new Date(now.getTime() - 24 * 60 * 60 * 1000)
  const last7d = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
  const last30d = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)

  // Current state
  const openIncidents = validIncidents.filter(i => !['resolved', 'failed'].includes(i.status))
  const criticalIncidents = openIncidents.filter(i => i.severity === 'critical')
  const unassignedIncidents = openIncidents.filter(i => !i.assignee_id)

  const avgAgeMinutes = openIncidents.length > 0
    ? openIncidents.reduce((sum, i) => {
        const created = new Date(i.created_at)
        return sum + (now.getTime() - created.getTime()) / 1000 / 60
      }, 0) / openIncidents.length
    : 0

  const breachedIncidents = await checkSlaBreaches(env)

  // Last 24 hours
  const last24hIncidents = validIncidents.filter(i => new Date(i.created_at) >= last24h)
  const last24hResolved = validIncidents.filter(i =>
    i.status === 'resolved' && i.updated_at && new Date(i.updated_at) >= last24h
  )

  const last24hBySeverity: Record<IncidentSeverity, number> = {
    low: 0, medium: 0, high: 0, critical: 0,
  }
  for (const incident of last24hIncidents) {
    last24hBySeverity[incident.severity]++
  }

  const last24hSourceCounts: Record<string, number> = {}
  for (const incident of last24hIncidents) {
    last24hSourceCounts[incident.source] = (last24hSourceCounts[incident.source] || 0) + 1
  }

  // Last 7 days
  const last7dIncidents = validIncidents.filter(i => new Date(i.created_at) >= last7d)
  const last7dResolved = validIncidents.filter(i =>
    i.status === 'resolved' && new Date(i.created_at) >= last7d
  )

  const last7dTrend: Array<{ date: string; created: number; resolved: number }> = []
  for (let d = 6; d >= 0; d--) {
    const date = new Date(now.getTime() - d * 24 * 60 * 60 * 1000)
    const dateStr = date.toISOString().split('T')[0]
    const dayCreated = validIncidents.filter(i => i.created_at.startsWith(dateStr))
    const dayResolved = validIncidents.filter(i =>
      i.status === 'resolved' && i.updated_at?.startsWith(dateStr)
    )
    last7dTrend.push({
      date: dateStr,
      created: dayCreated.length,
      resolved: dayResolved.length,
    })
  }

  // Last 30 days
  const last30dIncidents = validIncidents.filter(i => new Date(i.created_at) >= last30d)
  const last30dResolved = last30dIncidents.filter(i => i.status === 'resolved')

  const last30dSourceCounts: Record<string, number> = {}
  for (const incident of last30dIncidents) {
    last30dSourceCounts[incident.source] = (last30dSourceCounts[incident.source] || 0) + 1
  }

  const last30dAssigneeCounts: Record<number, { email: string; count: number }> = {}
  for (const incident of last30dIncidents) {
    if (incident.assignee_id && incident.assignee_email) {
      if (!last30dAssigneeCounts[incident.assignee_id]) {
        last30dAssigneeCounts[incident.assignee_id] = {
          email: incident.assignee_email,
          count: 0,
        }
      }
      last30dAssigneeCounts[incident.assignee_id].count++
    }
  }

  // Calculate MTTRs
  const calculateMttr = (incidents: IncidentRecord[]): number => {
    const resolved = incidents.filter(i => i.status === 'resolved' && i.created_at && i.updated_at)
    if (resolved.length === 0) return 0
    return resolved.reduce((sum, i) => {
      const created = new Date(i.created_at)
      const updated = new Date(i.updated_at!)
      return sum + (updated.getTime() - created.getTime()) / 1000 / 60
    }, 0) / resolved.length
  }

  // SLA compliance (last 7 days)
  const slaBreachedLast7d = last7dIncidents.filter(async i => {
    const status = await getIncidentSlaStatus(env, i)
    return status.isBreached
  }).length
  const slaComplianceRate = last7dIncidents.length > 0
    ? ((last7dIncidents.length - slaBreachedLast7d) / last7dIncidents.length) * 100
    : 100

  return {
    current: {
      open_incidents: openIncidents.length,
      critical_incidents: criticalIncidents.length,
      unassigned_incidents: unassignedIncidents.length,
      sla_breach_count: breachedIncidents.length,
      avg_age_minutes: avgAgeMinutes,
    },
    last_24h: {
      created: last24hIncidents.length,
      resolved: last24hResolved.length,
      mttr_minutes: calculateMttr(last24hIncidents),
      by_severity: last24hBySeverity,
      by_source: Object.entries(last24hSourceCounts)
        .map(([source, count]) => ({ source, count }))
        .sort((a, b) => b.count - a.count)
        .slice(0, 5),
    },
    last_7d: {
      created: last7dIncidents.length,
      resolved: last7dResolved.length,
      mttr_minutes: calculateMttr(last7dIncidents),
      sla_compliance_rate: slaComplianceRate,
      trend: last7dTrend,
    },
    last_30d: {
      created: last30dIncidents.length,
      resolved: last30dResolved.length,
      mttr_minutes: calculateMttr(last30dIncidents),
      top_sources: Object.entries(last30dSourceCounts)
        .map(([source, count]) => ({
          source,
          count,
          percentage: last30dIncidents.length > 0 ? (count / last30dIncidents.length) * 100 : 0,
        }))
        .sort((a, b) => b.count - a.count)
        .slice(0, 5),
      top_assignees: Object.entries(last30dAssigneeCounts)
        .map(([userId, data]) => ({ user_id: Number(userId), email: data.email, count: data.count }))
        .sort((a, b) => b.count - a.count)
        .slice(0, 5),
    },
  }
}

// ==================== Incident Correlation ====================

export async function findRelatedIncidents(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentCorrelation> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  const ids = await getIncidentIndex(env)
  const incidents = await Promise.all(ids.map(id => getIncident(env, id)))
  const validIncidents = incidents.filter((i): i is IncidentRecord =>
    i !== null && i.id !== incidentId
  )

  const related: import('../types').IncidentCorrelation['related_incidents'] = []

  for (const other of validIncidents) {
    let score = 0
    const sharedAttributes: string[] = []

    // Same source
    if (other.source === incident.source) {
      score += 30
      sharedAttributes.push('source')
    }

    // Same action type
    if (other.action_type && other.action_type === incident.action_type) {
      score += 25
      sharedAttributes.push('action_type')
    }

    // Same action ref
    if (other.action_ref && other.action_ref === incident.action_ref) {
      score += 40
      sharedAttributes.push('action_ref')
    }

    // Overlapping tags
    const sharedTags = (incident.tags || []).filter(t => (other.tags || []).includes(t))
    if (sharedTags.length > 0) {
      score += 10 * sharedTags.length
      sharedAttributes.push(`tags:${sharedTags.join(',')}`)
    }

    // Same severity
    if (other.severity === incident.severity) {
      score += 10
    }

    // Time proximity
    const timeDiff = Math.abs(
      new Date(other.created_at).getTime() - new Date(incident.created_at).getTime()
    ) / 1000 / 60

    if (timeDiff < 60) {
      score += 20
    } else if (timeDiff < 240) {
      score += 10
    }

    if (score >= 30) {
      related.push({
        id: other.id,
        title: other.title,
        correlation_score: score,
        shared_attributes: sharedAttributes,
        time_proximity_minutes: timeDiff,
      })
    }
  }

  // Sort by score
  related.sort((a, b) => b.correlation_score - a.correlation_score)

  // Detect patterns
  let patternMatch: import('../types').IncidentCorrelation['pattern_match']

  const sameActionRef = related.filter(r =>
    r.shared_attributes.includes('action_ref')
  )

  if (sameActionRef.length >= 3) {
    patternMatch = {
      pattern_type: 'recurring',
      description: `Recurring incident with same action reference: ${incident.action_ref}`,
      confidence: 85,
    }
  } else if (related.filter(r => r.time_proximity_minutes < 30).length >= 3) {
    patternMatch = {
      pattern_type: 'cascading',
      description: 'Multiple incidents occurred within 30 minutes, possible cascading failure',
      confidence: 70,
    }
  } else if (related.filter(r => r.shared_attributes.includes('source')).length >= 3) {
    patternMatch = {
      pattern_type: 'related_service',
      description: `Multiple incidents from same source: ${incident.source}`,
      confidence: 75,
    }
  }

  return {
    incident_id: incidentId,
    related_incidents: related.slice(0, 10),
    pattern_match: patternMatch,
  }
}

// ==================== Incident Watch ====================

const WATCH_PREFIX = `${INCIDENT_PREFIX}watch:`

function watchKey(incidentId: string): string {
  return `${WATCH_PREFIX}${incidentId}`
}

export async function watchIncident(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  notifyOn: import('../types').IncidentTimelineEventType[]
): Promise<import('../types').IncidentWatch> {
  const now = nowIso()
  const watch: import('../types').IncidentWatch = {
    id: crypto.randomUUID(),
    incident_id: incidentId,
    user_id: principal.sub,
    user_email: principal.email,
    notify_on: notifyOn,
    created_at: now,
  }

  const key = watchKey(incidentId)
  const existing = await env.KV.get(key, 'json') as import('../types').IncidentWatch[] | null
  const watches = existing || []

  // Check if already watching
  const alreadyWatching = watches.find(w => w.user_id === principal.sub)
  if (alreadyWatching) {
    alreadyWatching.notify_on = notifyOn
    await env.KV.put(key, JSON.stringify(watches), { expirationTtl: 86400 * 30 })
    return alreadyWatching
  }

  watches.push(watch)
  await env.KV.put(key, JSON.stringify(watches), { expirationTtl: 86400 * 30 })

  return watch
}

export async function unwatchIncident(
  env: Env,
  incidentId: string,
  userId: number
): Promise<boolean> {
  const key = watchKey(incidentId)
  const watches = await env.KV.get(key, 'json') as import('../types').IncidentWatch[] | null

  if (!watches) return false

  const index = watches.findIndex(w => w.user_id === userId)
  if (index === -1) return false

  watches.splice(index, 1)
  await env.KV.put(key, JSON.stringify(watches), { expirationTtl: 86400 * 30 })

  return true
}

export async function getIncidentWatchers(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentWatch[]> {
  const key = watchKey(incidentId)
  return (await env.KV.get(key, 'json') as import('../types').IncidentWatch[] | null) || []
}

// ==================== External Tickets ====================

const EXTERNAL_TICKET_PREFIX = `${INCIDENT_PREFIX}ticket:`

function externalTicketKey(incidentId: string): string {
  return `${EXTERNAL_TICKET_PREFIX}${incidentId}`
}

export async function createExternalTicket(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  system: import('../types').ExternalTicket['system'],
  ticketId: string,
  ticketUrl: string,
  status: string
): Promise<import('../types').ExternalTicket> {
  const now = nowIso()
  const ticket: import('../types').ExternalTicket = {
    id: crypto.randomUUID(),
    incident_id: incidentId,
    system,
    ticket_id: ticketId,
    ticket_url: ticketUrl,
    status,
    synced_at: now,
    created_by: principal.sub,
    created_at: now,
  }

  const key = externalTicketKey(incidentId)
  const existing = await env.KV.get(key, 'json') as import('../types').ExternalTicket[] | null
  const tickets = existing || []
  tickets.push(ticket)

  await env.KV.put(key, JSON.stringify(tickets), { expirationTtl: 86400 * 90 })

  // Add link to incident
  const incident = await getIncident(env, incidentId)
  if (incident) {
    await addIncidentLink(env, incident, {
      kind: 'runbook', // Using runbook as proxy for external reference
      id: ticketId,
      name: `${system}: ${ticketId}`,
      href: ticketUrl,
      relationship: 'investigates',
    })
  }

  return ticket
}

export async function getExternalTickets(
  env: Env,
  incidentId: string
): Promise<import('../types').ExternalTicket[]> {
  const key = externalTicketKey(incidentId)
  return (await env.KV.get(key, 'json') as import('../types').ExternalTicket[] | null) || []
}

export async function updateExternalTicketStatus(
  env: Env,
  incidentId: string,
  ticketId: string,
  status: string
): Promise<import('../types').ExternalTicket | null> {
  const key = externalTicketKey(incidentId)
  const tickets = await env.KV.get(key, 'json') as import('../types').ExternalTicket[] | null

  if (!tickets) return null

  const ticket = tickets.find(t => t.ticket_id === ticketId)
  if (!ticket) return null

  ticket.status = status
  ticket.synced_at = nowIso()

  await env.KV.put(key, JSON.stringify(tickets), { expirationTtl: 86400 * 90 })

  return ticket
}

// ==================== Response Playbooks ====================

const RESPONSE_PLAYBOOK_KEY = 'incident:response_playbooks'
const PLAYBOOK_EXECUTION_KEY = 'incident:playbook_execution:'

function generatePlaybookId(): string {
  return `rpb-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}

function generateExecutionId(): string {
  return `pbe-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}

export async function listResponsePlaybooks(
  env: Env,
  enabled?: boolean
): Promise<import('../types').ResponsePlaybook[]> {
  const playbooks = (await env.KV.get(RESPONSE_PLAYBOOK_KEY, 'json') as import('../types').ResponsePlaybook[] | null) || []
  if (enabled === undefined) return playbooks
  return playbooks.filter(p => p.enabled === enabled)
}

export async function getResponsePlaybook(
  env: Env,
  playbookId: string
): Promise<import('../types').ResponsePlaybook | null> {
  const playbooks = await listResponsePlaybooks(env)
  return playbooks.find(p => p.id === playbookId) || null
}

export async function createResponsePlaybook(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    category?: string
    trigger_conditions: import('../types').ResponsePlaybook['trigger_conditions']
    steps: Omit<import('../types').ResponsePlaybookStep, 'id' | 'order'>[]
    auto_trigger?: boolean
    estimated_total_duration_minutes?: number
  }
): Promise<import('../types').ResponsePlaybook> {
  const playbooks = await listResponsePlaybooks(env)

  const now = nowIso()
  const playbook: import('../types').ResponsePlaybook = {
    id: generatePlaybookId(),
    name: input.name,
    description: input.description,
    category: input.category,
    trigger_conditions: input.trigger_conditions,
    steps: input.steps.map((step, index) => ({
      ...step,
      id: `step-${index + 1}`,
      order: index + 1,
    })),
    auto_trigger: input.auto_trigger ?? false,
    estimated_total_duration_minutes: input.estimated_total_duration_minutes,
    version: 1,
    enabled: true,
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  playbooks.push(playbook)
  await env.KV.put(RESPONSE_PLAYBOOK_KEY, JSON.stringify(playbooks), { expirationTtl: 86400 * 365 })

  return playbook
}

export async function updateResponsePlaybook(
  env: Env,
  playbookId: string,
  updates: Partial<Omit<import('../types').ResponsePlaybook, 'id' | 'created_by' | 'created_at' | 'version'>>
): Promise<import('../types').ResponsePlaybook | null> {
  const playbooks = await listResponsePlaybooks(env)
  const index = playbooks.findIndex(p => p.id === playbookId)

  if (index === -1) return null

  playbooks[index] = {
    ...playbooks[index],
    ...updates,
    version: playbooks[index].version + 1,
    updated_at: nowIso(),
  }

  // Reindex steps if provided
  if (updates.steps) {
    playbooks[index].steps = updates.steps.map((step, idx) => ({
      ...step,
      order: idx + 1,
    })) as import('../types').ResponsePlaybookStep[]
  }

  await env.KV.put(RESPONSE_PLAYBOOK_KEY, JSON.stringify(playbooks), { expirationTtl: 86400 * 365 })

  return playbooks[index]
}

export async function deleteResponsePlaybook(
  env: Env,
  playbookId: string
): Promise<boolean> {
  const playbooks = await listResponsePlaybooks(env)
  const index = playbooks.findIndex(p => p.id === playbookId)

  if (index === -1) return false

  playbooks.splice(index, 1)
  await env.KV.put(RESPONSE_PLAYBOOK_KEY, JSON.stringify(playbooks), { expirationTtl: 86400 * 365 })

  return true
}

export async function matchResponsePlaybooks(
  env: Env,
  incident: IncidentRecord
): Promise<import('../types').ResponsePlaybook[]> {
  const playbooks = await listResponsePlaybooks(env, true)
  const matches: import('../types').ResponsePlaybook[] = []

  for (const playbook of playbooks) {
    const conditions = playbook.trigger_conditions
    let matched = true

    if (conditions.severity?.length && !conditions.severity.includes(incident.severity)) {
      matched = false
    }
    if (conditions.source?.length && !conditions.source.includes(incident.source)) {
      matched = false
    }
    if (conditions.action_type?.length && incident.action_type && !conditions.action_type.includes(incident.action_type)) {
      matched = false
    }
    if (conditions.tags?.length && !incident.tags.some(t => conditions.tags!.includes(t))) {
      matched = false
    }
    if (conditions.title_pattern) {
      try {
        const regex = new RegExp(conditions.title_pattern, 'i')
        if (!regex.test(incident.title)) {
          matched = false
        }
      } catch {
        matched = false
      }
    }

    if (matched) {
      matches.push(playbook)
    }
  }

  // Sort by specificity (more conditions = higher priority)
  return matches.sort((a, b) => {
    const aScore = Object.values(a.trigger_conditions).filter(v => v && v.length > 0).length
    const bScore = Object.values(b.trigger_conditions).filter(v => v && v.length > 0).length
    return bScore - aScore
  })
}

export async function startPlaybookExecution(
  env: Env,
  incidentId: string,
  playbookId: string,
  principal: AuthPrincipal
): Promise<import('../types').ResponsePlaybookExecution> {
  const playbook = await getResponsePlaybook(env, playbookId)
  if (!playbook) {
    throw new Error('Playbook not found')
  }

  const execution: import('../types').ResponsePlaybookExecution = {
    id: generateExecutionId(),
    incident_id: incidentId,
    playbook_id: playbookId,
    playbook_version: playbook.version,
    status: 'running',
    current_step: 1,
    completed_steps: [],
    started_at: nowIso(),
    step_results: {},
  }

  // Initialize step results
  for (const step of playbook.steps) {
    execution.step_results[step.id] = { status: 'pending' }
  }

  await env.KV.put(
    `${PLAYBOOK_EXECUTION_KEY}${execution.id}`,
    JSON.stringify(execution),
    { expirationTtl: 86400 * 30 }
  )

  // Store execution reference on incident
  const incident = await getIncident(env, incidentId)
  if (incident) {
    const executions = (await env.KV.get(`${PLAYBOOK_EXECUTION_KEY}incident:${incidentId}`, 'json') as string[] | null) || []
    executions.push(execution.id)
    await env.KV.put(`${PLAYBOOK_EXECUTION_KEY}incident:${incidentId}`, JSON.stringify(executions), { expirationTtl: 86400 * 30 })
  }

  return execution
}

export async function getPlaybookExecution(
  env: Env,
  executionId: string
): Promise<import('../types').ResponsePlaybookExecution | null> {
  return (await env.KV.get(`${PLAYBOOK_EXECUTION_KEY}${executionId}`, 'json') as import('../types').ResponsePlaybookExecution | null) || null
}

export async function completePlaybookStep(
  env: Env,
  executionId: string,
  stepId: string,
  principal: AuthPrincipal,
  result?: Record<string, unknown>
): Promise<import('../types').ResponsePlaybookExecution | null> {
  const execution = await getPlaybookExecution(env, executionId)
  if (!execution || execution.status !== 'running') return null

  const playbook = await getResponsePlaybook(env, execution.playbook_id)
  if (!playbook) return null

  const step = playbook.steps.find(s => s.id === stepId)
  if (!step) return null

  const now = nowIso()
  execution.step_results[stepId] = {
    status: 'completed',
    started_at: now,
    completed_at: now,
    result: result,
    actor: { user_id: principal.sub, email: principal.email },
  }
  execution.completed_steps.push(stepId)

  // Move to next step
  const stepIndex = playbook.steps.findIndex(s => s.id === stepId)
  if (stepIndex < playbook.steps.length - 1) {
    execution.current_step = stepIndex + 2
  } else {
    // All steps completed
    execution.status = 'completed'
    execution.completed_at = now
  }

  await env.KV.put(
    `${PLAYBOOK_EXECUTION_KEY}${executionId}`,
    JSON.stringify(execution),
    { expirationTtl: 86400 * 30 }
  )

  return execution
}

export async function skipPlaybookStep(
  env: Env,
  executionId: string,
  stepId: string,
  principal: AuthPrincipal,
  reason: string
): Promise<import('../types').ResponsePlaybookExecution | null> {
  const execution = await getPlaybookExecution(env, executionId)
  if (!execution || execution.status !== 'running') return null

  const playbook = await getResponsePlaybook(env, execution.playbook_id)
  if (!playbook) return null

  execution.step_results[stepId] = {
    status: 'skipped',
    result: { reason },
    actor: { user_id: principal.sub, email: principal.email },
  }

  // Move to next step
  const stepIndex = playbook.steps.findIndex(s => s.id === stepId)
  if (stepIndex < playbook.steps.length - 1) {
    execution.current_step = stepIndex + 2
  } else {
    execution.status = 'completed'
    execution.completed_at = nowIso()
  }

  await env.KV.put(
    `${PLAYBOOK_EXECUTION_KEY}${executionId}`,
    JSON.stringify(execution),
    { expirationTtl: 86400 * 30 }
  )

  return execution
}

// ==================== Custom Fields ====================

const CUSTOM_FIELDS_KEY = 'incident:custom_fields'
const CUSTOM_FIELD_VALUES_KEY = 'incident:custom_field_values:'

export async function listCustomFieldDefinitions(
  env: Env,
  category?: string
): Promise<import('../types').CustomFieldDefinition[]> {
  const fields = (await env.KV.get(CUSTOM_FIELDS_KEY, 'json') as import('../types').CustomFieldDefinition[] | null) || []
  if (category) {
    return fields.filter(f => f.category === category)
  }
  return fields.sort((a, b) => a.order - b.order)
}

export async function getCustomFieldDefinition(
  env: Env,
  fieldId: string
): Promise<import('../types').CustomFieldDefinition | null> {
  const fields = await listCustomFieldDefinitions(env)
  return fields.find(f => f.id === fieldId) || null
}

export async function createCustomFieldDefinition(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    key: string
    type: import('../types').CustomFieldDefinition['type']
    required?: boolean
    default_value?: string | number | boolean | string[]
    options?: string[]
    validation?: import('../types').CustomFieldDefinition['validation']
    description?: string
    category?: string
  }
): Promise<import('../types').CustomFieldDefinition> {
  const fields = await listCustomFieldDefinitions(env)

  // Check for duplicate key
  if (fields.some(f => f.key === input.key)) {
    throw new Error(`Custom field with key '${input.key}' already exists`)
  }

  const now = nowIso()
  const field: import('../types').CustomFieldDefinition = {
    id: `cf-${Date.now().toString(36)}`,
    name: input.name,
    key: input.key,
    type: input.type,
    required: input.required ?? false,
    default_value: input.default_value,
    options: input.options,
    validation: input.validation,
    description: input.description,
    category: input.category,
    order: fields.length + 1,
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  fields.push(field)
  await env.KV.put(CUSTOM_FIELDS_KEY, JSON.stringify(fields), { expirationTtl: 86400 * 365 })

  return field
}

export async function updateCustomFieldDefinition(
  env: Env,
  fieldId: string,
  updates: Partial<Omit<import('../types').CustomFieldDefinition, 'id' | 'key' | 'created_by' | 'created_at'>>
): Promise<import('../types').CustomFieldDefinition | null> {
  const fields = await listCustomFieldDefinitions(env)
  const index = fields.findIndex(f => f.id === fieldId)

  if (index === -1) return null

  fields[index] = {
    ...fields[index],
    ...updates,
    updated_at: nowIso(),
  }

  await env.KV.put(CUSTOM_FIELDS_KEY, JSON.stringify(fields), { expirationTtl: 86400 * 365 })

  return fields[index]
}

export async function deleteCustomFieldDefinition(
  env: Env,
  fieldId: string
): Promise<boolean> {
  const fields = await listCustomFieldDefinitions(env)
  const index = fields.findIndex(f => f.id === fieldId)

  if (index === -1) return false

  fields.splice(index, 1)

  // Reorder remaining fields
  fields.forEach((f, i) => { f.order = i + 1 })

  await env.KV.put(CUSTOM_FIELDS_KEY, JSON.stringify(fields), { expirationTtl: 86400 * 365 })

  return true
}

export async function setIncidentCustomField(
  env: Env,
  incidentId: string,
  fieldId: string,
  value: string | number | boolean | string[] | null,
  principal: AuthPrincipal
): Promise<import('../types').IncidentCustomFieldValue> {
  const fieldDef = await getCustomFieldDefinition(env, fieldId)
  if (!fieldDef) {
    throw new Error('Field definition not found')
  }

  const fieldValue: import('../types').IncidentCustomFieldValue = {
    incident_id: incidentId,
    field_id: fieldId,
    value,
    updated_by: principal.sub,
    updated_at: nowIso(),
  }

  const valuesKey = `${CUSTOM_FIELD_VALUES_KEY}${incidentId}`
  const values = (await env.KV.get(valuesKey, 'json') as Record<string, import('../types').IncidentCustomFieldValue> | null) || {}
  values[fieldId] = fieldValue

  await env.KV.put(valuesKey, JSON.stringify(values), { expirationTtl: 86400 * 90 })

  return fieldValue
}

export async function getIncidentCustomFields(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentCustomFieldValue[]> {
  const valuesKey = `${CUSTOM_FIELD_VALUES_KEY}${incidentId}`
  const values = (await env.KV.get(valuesKey, 'json') as Record<string, import('../types').IncidentCustomFieldValue> | null) || {}
  return Object.values(values)
}

// ==================== AI Root Cause Analysis ====================

export async function generateAIRootCauseAnalysis(
  env: Env,
  incidentId: string
): Promise<import('../types').AIRootCauseAnalysis> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  // Get related data for context
  const timeline = buildIncidentTimeline(incident)
  const comments = await listIncidentComments(env, incidentId)

  // Build context for AI
  const context = `
Incident: ${incident.title}
Severity: ${incident.severity}
Source: ${incident.source}
Status: ${incident.status}
Summary: ${incident.summary || 'N/A'}
Evidence: ${JSON.stringify(incident.evidence, null, 2)}
Timeline Events: ${timeline.events.slice(0, 10).map(e => `${e.type}: ${e.summary}`).join('\n')}
Comments: ${comments.slice(0, 5).map(c => c.content).join('\n')}
`

  const prompt = `Analyze this incident and provide a root cause analysis:

${context}

Provide your analysis in the following JSON format:
{
  "summary": "Brief overall analysis summary",
  "root_causes": [
    {
      "category": "infrastructure|application|network|security|configuration|external",
      "description": "Description of the root cause",
      "confidence": 0.0-1.0,
      "evidence": ["Evidence point 1", "Evidence point 2"],
      "suggested_actions": ["Action 1", "Action 2"]
    }
  ],
  "impact_analysis": {
    "affected_services": ["service1", "service2"],
    "affected_users_estimate": 0,
    "business_impact": "low|medium|high|critical",
    "blast_radius": "Description of scope"
  },
  "predictions": [
    {
      "type": "escalation|cascading|recovery_time",
      "prediction": "Specific prediction",
      "confidence": 0.0-1.0,
      "factors": ["Factor 1", "Factor 2"]
    }
  ],
  "recommendations": [
    {
      "priority": "immediate|short_term|long_term",
      "action": "Specific action",
      "rationale": "Why this action",
      "automated": true/false
    }
  ]
}

Respond only with valid JSON.`

  const aiResult = await generateText(env, prompt, { maxTokens: 2000 })

  let analysis: Omit<import('../types').AIRootCauseAnalysis, 'incident_id' | 'generated_at' | 'model' | 'analysis_type' | 'similar_incidents'>

  try {
    const responseText = aiResult.success && aiResult.data
      ? (aiResult.data as { response?: string })?.response || JSON.stringify(aiResult.data)
      : ''
    analysis = JSON.parse(responseText)
  } catch {
    // Parse the response as best we can
    const fallbackText = aiResult.success && aiResult.data
      ? JSON.stringify(aiResult.data).slice(0, 500)
      : 'Analysis unavailable'
    analysis = {
      summary: fallbackText,
      root_causes: [],
      impact_analysis: {
        affected_services: [],
        business_impact: 'medium',
        blast_radius: 'Unknown',
      },
      predictions: [],
      recommendations: [],
    }
  }

  // Find similar incidents
  const similarIncidents = await findSimilarIncidents(env, incident)

  return {
    incident_id: incidentId,
    analysis_type: 'root_cause',
    generated_at: nowIso(),
    model: 'workers-ai',
    ...analysis,
    similar_incidents: similarIncidents,
  }
}

async function findSimilarIncidents(
  env: Env,
  incident: IncidentRecord
): Promise<import('../types').AIRootCauseAnalysis['similar_incidents']> {
  const allIncidents = await listIncidents(env, { per_page: 100 })
  const similar: import('../types').AIRootCauseAnalysis['similar_incidents'] = []

  for (const other of allIncidents.items) {
    if (other.id === incident.id) continue

    let score = 0
    const sharedPatterns: string[] = []

    // Same severity
    if (other.severity === incident.severity) {
      score += 0.2
      sharedPatterns.push(`same severity: ${incident.severity}`)
    }

    // Same source
    if (other.source === incident.source) {
      score += 0.2
      sharedPatterns.push(`same source: ${incident.source}`)
    }

    // Same action type
    if (other.action_type && other.action_type === incident.action_type) {
      score += 0.3
      sharedPatterns.push(`same action: ${incident.action_type}`)
    }

    // Overlapping tags
    const sharedTags = other.tags.filter(t => incident.tags.includes(t))
    if (sharedTags.length > 0) {
      score += 0.1 * Math.min(sharedTags.length, 3)
      sharedPatterns.push(`shared tags: ${sharedTags.join(', ')}`)
    }

    // Similar title (simple word overlap)
    const incidentWords = new Set(incident.title.toLowerCase().split(/\s+/))
    const otherWords = new Set(other.title.toLowerCase().split(/\s+/))
    const commonWords = [...incidentWords].filter(w => otherWords.has(w) && w.length > 3)
    if (commonWords.length > 1) {
      score += 0.1 * Math.min(commonWords.length, 3)
    }

    if (score >= 0.3) {
      similar.push({
        incident_id: other.id,
        title: other.title,
        similarity_score: Math.min(score, 1),
        shared_patterns: sharedPatterns,
      })
    }
  }

  return similar.sort((a, b) => b.similarity_score - a.similarity_score).slice(0, 5)
}

// ==================== War Room ====================

const WAR_ROOM_KEY = 'incident:war_room:'

export async function createWarRoom(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal
): Promise<import('../types').IncidentWarRoom> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  const warRoom: import('../types').IncidentWarRoom = {
    id: `wr-${Date.now().toString(36)}`,
    incident_id: incidentId,
    status: 'active',
    commander_id: principal.sub,
    commander_email: principal.email,
    participants: [
      {
        user_id: principal.sub,
        email: principal.email,
        role: 'commander',
        joined_at: nowIso(),
      }
    ],
    chat_messages: [],
    shared_resources: [],
    created_at: nowIso(),
  }

  await env.KV.put(`${WAR_ROOM_KEY}${incidentId}`, JSON.stringify(warRoom), { expirationTtl: 86400 * 7 })

  return warRoom
}

export async function getWarRoom(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentWarRoom | null> {
  return (await env.KV.get(`${WAR_ROOM_KEY}${incidentId}`, 'json') as import('../types').IncidentWarRoom | null) || null
}

export async function joinWarRoom(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  role: 'commander' | 'responder' | 'observer' = 'responder'
): Promise<import('../types').IncidentWarRoom | null> {
  const warRoom = await getWarRoom(env, incidentId)
  if (!warRoom || warRoom.status === 'closed') return null

  // Check if already participant
  if (warRoom.participants.some(p => p.user_id === principal.sub)) {
    return warRoom
  }

  warRoom.participants.push({
    user_id: principal.sub,
    email: principal.email,
    role,
    joined_at: nowIso(),
  })

  // Add system message
  warRoom.chat_messages.push({
    id: `msg-${Date.now().toString(36)}`,
    user_id: 0,
    user_email: 'system',
    message: `${principal.email} joined as ${role}`,
    timestamp: nowIso(),
    system: true,
  })

  await env.KV.put(`${WAR_ROOM_KEY}${incidentId}`, JSON.stringify(warRoom), { expirationTtl: 86400 * 7 })

  return warRoom
}

export async function leaveWarRoom(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal
): Promise<import('../types').IncidentWarRoom | null> {
  const warRoom = await getWarRoom(env, incidentId)
  if (!warRoom) return null

  warRoom.participants = warRoom.participants.filter(p => p.user_id !== principal.sub)

  // Add system message
  warRoom.chat_messages.push({
    id: `msg-${Date.now().toString(36)}`,
    user_id: 0,
    user_email: 'system',
    message: `${principal.email} left the war room`,
    timestamp: nowIso(),
    system: true,
  })

  await env.KV.put(`${WAR_ROOM_KEY}${incidentId}`, JSON.stringify(warRoom), { expirationTtl: 86400 * 7 })

  return warRoom
}

export async function addWarRoomMessage(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  message: string
): Promise<import('../types').IncidentWarRoom | null> {
  const warRoom = await getWarRoom(env, incidentId)
  if (!warRoom || warRoom.status === 'closed') return null

  // Check if participant
  if (!warRoom.participants.some(p => p.user_id === principal.sub)) {
    return null
  }

  warRoom.chat_messages.push({
    id: `msg-${Date.now().toString(36)}`,
    user_id: principal.sub,
    user_email: principal.email,
    message,
    timestamp: nowIso(),
  })

  await env.KV.put(`${WAR_ROOM_KEY}${incidentId}`, JSON.stringify(warRoom), { expirationTtl: 86400 * 7 })

  return warRoom
}

export async function addWarRoomResource(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  resource: {
    type: 'link' | 'document' | 'dashboard' | 'log'
    title: string
    url: string
  }
): Promise<import('../types').IncidentWarRoom | null> {
  const warRoom = await getWarRoom(env, incidentId)
  if (!warRoom || warRoom.status === 'closed') return null

  warRoom.shared_resources.push({
    ...resource,
    added_by: principal.sub,
    added_at: nowIso(),
  })

  await env.KV.put(`${WAR_ROOM_KEY}${incidentId}`, JSON.stringify(warRoom), { expirationTtl: 86400 * 7 })

  return warRoom
}

export async function closeWarRoom(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal
): Promise<import('../types').IncidentWarRoom | null> {
  const warRoom = await getWarRoom(env, incidentId)
  if (!warRoom || warRoom.status === 'closed') return null

  warRoom.status = 'closed'
  warRoom.closed_at = nowIso()

  warRoom.chat_messages.push({
    id: `msg-${Date.now().toString(36)}`,
    user_id: 0,
    user_email: 'system',
    message: `War room closed by ${principal.email}`,
    timestamp: nowIso(),
    system: true,
  })

  await env.KV.put(`${WAR_ROOM_KEY}${incidentId}`, JSON.stringify(warRoom), { expirationTtl: 86400 * 7 })

  return warRoom
}

// ==================== Incident Export ====================

export async function exportIncidents(
  env: Env,
  options: import('../types').IncidentExportOptions,
  principal: AuthPrincipal
): Promise<import('../types').IncidentExportResult> {
  const exportId = `export-${Date.now().toString(36)}`

  const result: import('../types').IncidentExportResult = {
    id: exportId,
    format: options.format,
    status: 'pending',
    total_incidents: 0,
    created_at: nowIso(),
  }

  // Get incidents based on filters
  const filters: IncidentListFilters = {}
  if (options.filters?.status) {
    filters.status = options.filters.status[0]
  }
  if (options.filters?.severity) {
    filters.severity = options.filters.severity[0]
  }
  if (options.filters?.source) {
    filters.source = options.filters.source[0]
  }

  const incidents = await listIncidents(env, { ...filters, per_page: 1000 })
  result.total_incidents = incidents.total
  result.status = 'processing'

  // Build export data
  const exportData: Array<Record<string, unknown>> = []

  for (const incident of incidents.items) {
    const item: Record<string, unknown> = {
      id: incident.id,
      title: incident.title,
      summary: incident.summary,
      status: incident.status,
      severity: incident.severity,
      source: incident.source,
      correlation_id: incident.correlation_id,
      action_type: incident.action_type,
      action_ref: incident.action_ref,
      tags: incident.tags,
      created_at: incident.created_at,
      updated_at: incident.updated_at,
    }

    if (options.include_evidence) {
      item.evidence = incident.evidence
    }

    if (options.include_timeline) {
      const timeline = buildIncidentTimeline(incident)
      item.timeline = timeline.events
    }

    if (options.include_comments) {
      const comments = await listIncidentComments(env, incident.id)
      item.comments = comments
    }

    if (options.include_recommendations) {
      item.recommendations = incident.recommendations
    }

    if (options.include_links) {
      item.links = incident.links
    }

    if (options.include_postmortem) {
      const postMortem = await getPostMortem(env, incident.id)
      item.postmortem = postMortem
    }

    exportData.push(item)
  }

  // Generate output
  let output: string
  let storageKey: string

  if (options.format === 'json') {
    output = JSON.stringify(exportData, null, 2)
    storageKey = `incident:export:${exportId}.json`
  } else {
    // CSV format
    const headers = Object.keys(exportData[0] || {})
    const rows = exportData.map(item =>
      headers.map(h => {
        const val = item[h]
        if (typeof val === 'string') return `"${val.replace(/"/g, '""')}"`
        if (Array.isArray(val)) return `"${JSON.stringify(val).replace(/"/g, '""')}"`
        return String(val ?? '')
      }).join(',')
    )
    output = [headers.join(','), ...rows].join('\n')
    storageKey = `incident:export:${exportId}.csv`
  }

  // Store in R2
  await env.R2.put(storageKey, output, {
    httpMetadata: {
      contentType: options.format === 'json' ? 'application/json' : 'text/csv',
    },
    customMetadata: {
      exported_by: String(principal.sub),
      exported_at: nowIso(),
    },
  })

  result.status = 'completed'
  result.download_url = `/api/v1/incidents/export/${exportId}/download`
  result.completed_at = nowIso()
  result.expires_at = new Date(Date.now() + 86400000).toISOString() // 24 hours

  // Store export metadata
  await env.KV.put(`incident:export:meta:${exportId}`, JSON.stringify(result), { expirationTtl: 86400 })

  return result
}

export async function getExportResult(
  env: Env,
  exportId: string
): Promise<import('../types').IncidentExportResult | null> {
  return (await env.KV.get(`incident:export:meta:${exportId}`, 'json') as import('../types').IncidentExportResult | null) || null
}

export async function getExportDownload(
  env: Env,
  exportId: string
): Promise<{ body: ReadableStream; contentType: string } | null> {
  const meta = await getExportResult(env, exportId)
  if (!meta || meta.status !== 'completed') return null

  const extension = meta.format === 'json' ? 'json' : 'csv'
  const object = await env.R2.get(`incident:export:${exportId}.${extension}`)
  if (!object) return null

  return {
    body: object.body,
    contentType: meta.format === 'json' ? 'application/json' : 'text/csv',
  }
}

// ==================== Incident Reviews ====================

const INCIDENT_REVIEW_KEY = 'incident:reviews:'

export async function createIncidentReview(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  input: {
    scheduled_at: string
    review_type: import('../types').IncidentReview['review_type']
    attendees?: Array<{ user_id: number; email: string; required?: boolean }>
    agenda?: string[]
  }
): Promise<import('../types').IncidentReview> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  const review: import('../types').IncidentReview = {
    id: `review-${Date.now().toString(36)}`,
    incident_id: incidentId,
    scheduled_at: input.scheduled_at,
    status: 'scheduled',
    review_type: input.review_type,
    attendees: (input.attendees || [{ user_id: principal.sub, email: principal.email, required: true }]).map(a => ({
      user_id: a.user_id,
      email: a.email,
      required: a.required ?? true,
    })),
    agenda: input.agenda || [],
    action_items: [],
    created_by: principal.sub,
    created_at: nowIso(),
  }

  // Store review
  const reviewsKey = `${INCIDENT_REVIEW_KEY}${incidentId}`
  const reviews = (await env.KV.get(reviewsKey, 'json') as import('../types').IncidentReview[] | null) || []
  reviews.push(review)
  await env.KV.put(reviewsKey, JSON.stringify(reviews), { expirationTtl: 86400 * 90 })

  return review
}

export async function getIncidentReviews(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentReview[]> {
  const key = `${INCIDENT_REVIEW_KEY}${incidentId}`
  return (await env.KV.get(key, 'json') as import('../types').IncidentReview[] | null) || []
}

export async function completeIncidentReview(
  env: Env,
  incidentId: string,
  reviewId: string,
  principal: AuthPrincipal,
  notes?: string,
  actionItems?: Array<{ description: string; owner_id?: number; due_date?: string }>
): Promise<import('../types').IncidentReview | null> {
  const reviews = await getIncidentReviews(env, incidentId)
  const review = reviews.find(r => r.id === reviewId)

  if (!review) return null

  review.status = 'completed'
  review.notes = notes
  review.completed_at = nowIso()
  review.completed_by = principal.sub

  if (actionItems) {
    review.action_items = actionItems.map((item, i) => ({
      id: `action-${Date.now().toString(36)}-${i}`,
      description: item.description,
      owner_id: item.owner_id,
      due_date: item.due_date,
      status: 'open',
    }))
  }

  await env.KV.put(`${INCIDENT_REVIEW_KEY}${incidentId}`, JSON.stringify(reviews), { expirationTtl: 86400 * 90 })

  return review
}

// ==================== Response Analytics ====================

export async function calculateResponseAnalytics(
  env: Env,
  startDate: string,
  endDate: string
): Promise<import('../types').IncidentResponseAnalytics> {
  const incidents = await listIncidents(env, { per_page: 1000 })

  // Filter by date range
  const filtered = incidents.items.filter(i => {
    const createdAt = new Date(i.created_at)
    return createdAt >= new Date(startDate) && createdAt <= new Date(endDate)
  })

  const analytics: import('../types').IncidentResponseAnalytics = {
    period: { start: startDate, end: endDate },
    response_metrics: {
      avg_time_to_acknowledge_minutes: 0,
      avg_time_to_assign_minutes: 0,
      avg_time_to_analyze_minutes: 0,
      avg_time_to_approve_minutes: 0,
      avg_time_to_resolve_minutes: 0,
      avg_time_to_first_response_minutes: 0,
    },
    by_severity: {
      low: { count: 0, avg_resolution_time_minutes: 0, sla_compliance_rate: 0, avg_time_to_acknowledge_minutes: 0 },
      medium: { count: 0, avg_resolution_time_minutes: 0, sla_compliance_rate: 0, avg_time_to_acknowledge_minutes: 0 },
      high: { count: 0, avg_resolution_time_minutes: 0, sla_compliance_rate: 0, avg_time_to_acknowledge_minutes: 0 },
      critical: { count: 0, avg_resolution_time_minutes: 0, sla_compliance_rate: 0, avg_time_to_acknowledge_minutes: 0 },
    },
    by_source: [],
    by_assignee: [],
    hourly_distribution: Array.from({ length: 24 }, (_, i) => ({ hour: i, count: 0 })),
    daily_distribution: [],
    escalation_rate: 0,
    reopen_rate: 0,
    auto_resolution_rate: 0,
  }

  if (filtered.length === 0) return analytics

  // Calculate metrics
  const ackTimes: number[] = []
  const assignTimes: number[] = []
  const analyzeTimes: number[] = []
  const approveTimes: number[] = []
  const resolveTimes: number[] = []
  const firstResponseTimes: number[] = []

  const severityStats: Record<string, { times: number[]; ackTimes: number[]; slaMet: number }> = {
    low: { times: [], ackTimes: [], slaMet: 0 },
    medium: { times: [], ackTimes: [], slaMet: 0 },
    high: { times: [], ackTimes: [], slaMet: 0 },
    critical: { times: [], ackTimes: [], slaMet: 0 },
  }

  const sourceStats: Record<string, { times: number[]; firstResponse: number[] }> = {}
  const assigneeStats: Record<string, { times: number[]; slaMet: number; email: string }> = {}

  let escalated = 0
  let autoResolved = 0

  for (const incident of filtered) {
    const created = new Date(incident.created_at).getTime()

    // Acknowledgment time
    if (incident.acknowledged_at) {
      const ackTime = (new Date(incident.acknowledged_at).getTime() - created) / 60000
      ackTimes.push(ackTime)
      severityStats[incident.severity].ackTimes.push(ackTime)
    }

    // Assignment time
    if (incident.assigned_at) {
      assignTimes.push((new Date(incident.assigned_at).getTime() - created) / 60000)
    }

    // Resolution time
    if (incident.status === 'resolved' && incident.updated_at) {
      const resolveTime = (new Date(incident.updated_at).getTime() - created) / 60000
      resolveTimes.push(resolveTime)
      severityStats[incident.severity].times.push(resolveTime)

      if (!incident.approved_by) {
        autoResolved++
      }
    }

    // Escalation check
    if (incident.escalated_from) {
      escalated++
    }

    // Source stats
    if (!sourceStats[incident.source]) {
      sourceStats[incident.source] = { times: [], firstResponse: [] }
    }
    if (incident.status === 'resolved' && incident.updated_at) {
      sourceStats[incident.source].times.push((new Date(incident.updated_at).getTime() - created) / 60000)
    }

    // Assignee stats
    if (incident.assignee_id) {
      const key = String(incident.assignee_id)
      if (!assigneeStats[key]) {
        assigneeStats[key] = { times: [], slaMet: 0, email: incident.assignee_email || '' }
      }
      if (incident.status === 'resolved' && incident.updated_at) {
        assigneeStats[key].times.push((new Date(incident.updated_at).getTime() - created) / 60000)
      }
    }

    // Hourly distribution
    const hour = new Date(incident.created_at).getHours()
    analytics.hourly_distribution[hour].count++

    // Count by severity
    analytics.by_severity[incident.severity].count++
  }

  // Calculate averages
  const avg = (arr: number[]) => arr.length > 0 ? arr.reduce((a, b) => a + b, 0) / arr.length : 0

  analytics.response_metrics.avg_time_to_acknowledge_minutes = avg(ackTimes)
  analytics.response_metrics.avg_time_to_assign_minutes = avg(assignTimes)
  analytics.response_metrics.avg_time_to_analyze_minutes = avg(analyzeTimes)
  analytics.response_metrics.avg_time_to_approve_minutes = avg(approveTimes)
  analytics.response_metrics.avg_time_to_resolve_minutes = avg(resolveTimes)
  analytics.response_metrics.avg_time_to_first_response_minutes = avg(firstResponseTimes)

  // Severity breakdown
  for (const [severity, stats] of Object.entries(severityStats)) {
    analytics.by_severity[severity as IncidentSeverity].avg_resolution_time_minutes = avg(stats.times)
    analytics.by_severity[severity as IncidentSeverity].avg_time_to_acknowledge_minutes = avg(stats.ackTimes)
    analytics.by_severity[severity as IncidentSeverity].sla_compliance_rate =
      stats.times.length > 0 ? (stats.slaMet / stats.times.length) * 100 : 0
  }

  // Source breakdown
  analytics.by_source = Object.entries(sourceStats).map(([source, stats]) => ({
    source,
    count: stats.times.length,
    avg_resolution_time_minutes: avg(stats.times),
    avg_time_to_first_response_minutes: avg(stats.firstResponse),
  }))

  // Assignee breakdown
  analytics.by_assignee = Object.entries(assigneeStats).map(([userId, stats]) => ({
    user_id: Number(userId),
    email: stats.email,
    incidents_handled: stats.times.length,
    avg_resolution_time_minutes: avg(stats.times),
    sla_compliance_rate: stats.times.length > 0 ? (stats.slaMet / stats.times.length) * 100 : 0,
  }))

  // Rates
  analytics.escalation_rate = filtered.length > 0 ? (escalated / filtered.length) * 100 : 0
  analytics.auto_resolution_rate = resolveTimes.length > 0 ? (autoResolved / resolveTimes.length) * 100 : 0

  // Daily distribution
  const dailyCounts: Record<string, { created: number; resolved: number }> = {}
  for (const incident of filtered) {
    const day = incident.created_at.split('T')[0]
    if (!dailyCounts[day]) dailyCounts[day] = { created: 0, resolved: 0 }
    dailyCounts[day].created++
    if (incident.status === 'resolved') {
      const resolvedDay = incident.updated_at?.split('T')[0]
      if (resolvedDay) {
        if (!dailyCounts[resolvedDay]) dailyCounts[resolvedDay] = { created: 0, resolved: 0 }
        dailyCounts[resolvedDay].resolved++
      }
    }
  }
  analytics.daily_distribution = Object.entries(dailyCounts).map(([day, counts]) => ({
    day,
    ...counts,
  }))

  return analytics
}

// ==================== Incident Feedback ====================

const INCIDENT_FEEDBACK_KEY = 'incident:feedback:'

export async function submitIncidentFeedback(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  input: {
    ratings: import('../types').IncidentFeedback['ratings']
    strengths?: string[]
    improvements?: string[]
    additional_comments?: string
    would_recommend: boolean
  }
): Promise<import('../types').IncidentFeedback> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  const feedback: import('../types').IncidentFeedback = {
    id: `feedback-${Date.now().toString(36)}`,
    incident_id: incidentId,
    submitted_by: principal.sub,
    submitted_at: nowIso(),
    ratings: input.ratings,
    strengths: input.strengths,
    improvements: input.improvements,
    additional_comments: input.additional_comments,
    would_recommend: input.would_recommend,
  }

  await env.KV.put(`${INCIDENT_FEEDBACK_KEY}${incidentId}`, JSON.stringify(feedback), { expirationTtl: 86400 * 365 })

  return feedback
}

export async function getIncidentFeedback(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentFeedback | null> {
  return (await env.KV.get(`${INCIDENT_FEEDBACK_KEY}${incidentId}`, 'json') as import('../types').IncidentFeedback | null) || null
}

// ==================== Incident Cost Tracking ====================

export async function calculateIncidentCost(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  input: {
    labor_hours: number
    labor_rate_usd?: number
    infrastructure_cost_usd?: number
    revenue_impact_usd?: number
    third_party_cost_usd?: number
    notes?: string
  }
): Promise<import('../types').IncidentCost> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  const laborRate = input.labor_rate_usd || 150 // Default $150/hr
  const laborCost = input.labor_hours * laborRate

  const cost: import('../types').IncidentCost = {
    incident_id: incidentId,
    estimated_cost_usd: laborCost + (input.infrastructure_cost_usd || 0) + (input.revenue_impact_usd || 0) + (input.third_party_cost_usd || 0),
    cost_breakdown: {
      labor_hours: input.labor_hours,
      labor_cost_usd: laborCost,
      infrastructure_cost_usd: input.infrastructure_cost_usd || 0,
      revenue_impact_usd: input.revenue_impact_usd || 0,
      third_party_cost_usd: input.third_party_cost_usd || 0,
    },
    calculated_at: nowIso(),
    calculated_by: principal.sub,
    notes: input.notes,
  }

  await env.KV.put(`incident:cost:${incidentId}`, JSON.stringify(cost), { expirationTtl: 86400 * 365 })

  return cost
}

export async function getIncidentCost(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentCost | null> {
  return (await env.KV.get(`incident:cost:${incidentId}`, 'json') as import('../types').IncidentCost | null) || null
}

// ==================== Incident Compliance ====================

const COMPLIANCE_KEY = 'incident:compliance:'

export async function createComplianceRecord(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  framework: import('../types').IncidentComplianceRecord['framework'],
  requirements: Array<{ requirement_id: string; description: string }>
): Promise<import('../types').IncidentComplianceRecord> {
  const incident = await getIncident(env, incidentId)
  if (!incident) {
    throw new Error('Incident not found')
  }

  const record: import('../types').IncidentComplianceRecord = {
    id: `comp-${Date.now().toString(36)}`,
    incident_id: incidentId,
    framework,
    requirements: requirements.map(r => ({
      ...r,
      status: 'pending' as const,
    })),
    status: 'pending',
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  await env.KV.put(`${COMPLIANCE_KEY}${incidentId}`, JSON.stringify(record), { expirationTtl: 86400 * 365 })

  return record
}

export async function getComplianceRecord(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentComplianceRecord | null> {
  return (await env.KV.get(`${COMPLIANCE_KEY}${incidentId}`, 'json') as import('../types').IncidentComplianceRecord | null) || null
}

export async function updateComplianceRequirement(
  env: Env,
  incidentId: string,
  requirementId: string,
  status: 'compliant' | 'non_compliant' | 'not_applicable',
  evidence?: string,
  notes?: string,
  principal?: AuthPrincipal
): Promise<import('../types').IncidentComplianceRecord | null> {
  const record = await getComplianceRecord(env, incidentId)
  if (!record) return null

  const req = record.requirements.find(r => r.requirement_id === requirementId)
  if (!req) return null

  req.status = status
  req.evidence = evidence
  req.notes = notes

  // Update overall status
  const allReviewed = record.requirements.every(r => r.status !== 'pending')
  if (allReviewed) {
    record.status = record.requirements.some(r => r.status === 'non_compliant') ? 'non_compliant' : 'compliant'
  }

  record.updated_at = nowIso()
  if (principal) {
    record.reviewed_at = nowIso()
    record.reviewed_by = principal.sub
  }

  await env.KV.put(`${COMPLIANCE_KEY}${incidentId}`, JSON.stringify(record), { expirationTtl: 86400 * 365 })

  return record
}

// ==================== On-Call Schedules ====================

const ONCALL_SCHEDULE_KEY = 'incident:oncall_schedules'

export async function listOnCallSchedules(
  env: Env,
  enabled?: boolean
): Promise<import('../types').OnCallSchedule[]> {
  const schedules = (await env.KV.get(ONCALL_SCHEDULE_KEY, 'json') as import('../types').OnCallSchedule[] | null) || []
  if (enabled === undefined) return schedules
  return schedules.filter(s => s.enabled === enabled)
}

export async function getOnCallSchedule(
  env: Env,
  scheduleId: string
): Promise<import('../types').OnCallSchedule | null> {
  const schedules = await listOnCallSchedules(env)
  return schedules.find(s => s.id === scheduleId) || null
}

export async function createOnCallSchedule(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    team?: string
    rotation_type: import('../types').OnCallSchedule['rotation_type']
    rotation_config: import('../types').OnCallSchedule['rotation_config']
    timezone?: string
  }
): Promise<import('../types').OnCallSchedule> {
  const schedules = await listOnCallSchedules(env)

  const schedule: import('../types').OnCallSchedule = {
    id: `oncall-${Date.now().toString(36)}`,
    name: input.name,
    description: input.description,
    team: input.team,
    rotation_type: input.rotation_type,
    rotation_config: input.rotation_config,
    overrides: [],
    timezone: input.timezone || 'UTC',
    enabled: true,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  schedules.push(schedule)
  await env.KV.put(ONCALL_SCHEDULE_KEY, JSON.stringify(schedules), { expirationTtl: 86400 * 365 })

  return schedule
}

export async function getCurrentOnCall(
  env: Env,
  scheduleId: string
): Promise<import('../types').OnCallShift | null> {
  const schedule = await getOnCallSchedule(env, scheduleId)
  if (!schedule || !schedule.enabled) return null

  const now = new Date()
  const members = schedule.rotation_config.members.sort((a, b) => a.order - b.order)

  if (members.length === 0) return null

  // Check for overrides first
  for (const override of schedule.overrides) {
    const start = new Date(override.start)
    const end = new Date(override.end)
    if (now >= start && now <= end) {
      return {
        id: `shift-${Date.now().toString(36)}`,
        schedule_id: scheduleId,
        user_id: override.user_id,
        user_email: override.email,
        start: override.start,
        end: override.end,
        is_override: true,
        status: 'active',
      }
    }
  }

  // Calculate current rotation
  const startDate = new Date(schedule.rotation_config.start_date)
  const msSinceStart = now.getTime() - startDate.getTime()

  let rotationLengthMs: number
  switch (schedule.rotation_type) {
    case 'weekly':
      rotationLengthMs = 7 * 24 * 60 * 60 * 1000
      break
    case 'biweekly':
      rotationLengthMs = 14 * 24 * 60 * 60 * 1000
      break
    case 'monthly':
      rotationLengthMs = 30 * 24 * 60 * 60 * 1000
      break
    default:
      rotationLengthMs = 7 * 24 * 60 * 60 * 1000
  }

  const rotationsSinceStart = Math.floor(msSinceStart / rotationLengthMs)
  const currentMemberIndex = rotationsSinceStart % members.length
  const currentMember = members[currentMemberIndex]

  const currentRotationStart = new Date(startDate.getTime() + rotationsSinceStart * rotationLengthMs)
  const currentRotationEnd = new Date(currentRotationStart.getTime() + rotationLengthMs)

  return {
    id: `shift-${Date.now().toString(36)}`,
    schedule_id: scheduleId,
    user_id: currentMember.user_id,
    user_email: currentMember.email,
    start: currentRotationStart.toISOString(),
    end: currentRotationEnd.toISOString(),
    is_override: false,
    status: 'active',
  }
}

// ==================== Incident Checklists ====================

const CHECKLIST_KEY = 'incident:checklists:'

export async function getIncidentChecklists(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentChecklist[]> {
  return (await env.KV.get(`${CHECKLIST_KEY}${incidentId}`, 'json') as import('../types').IncidentChecklist[] | null) || []
}

export async function createIncidentChecklist(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  name: string,
  items: string[]
): Promise<import('../types').IncidentChecklist> {
  const checklists = await getIncidentChecklists(env, incidentId)

  const checklist: import('../types').IncidentChecklist = {
    id: `checklist-${Date.now().toString(36)}`,
    incident_id: incidentId,
    name,
    items: items.map((text, i) => ({
      id: `item-${i}`,
      text,
      checked: false,
    })),
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  checklists.push(checklist)
  await env.KV.put(`${CHECKLIST_KEY}${incidentId}`, JSON.stringify(checklists), { expirationTtl: 86400 * 90 })

  return checklist
}

export async function updateChecklistItem(
  env: Env,
  incidentId: string,
  checklistId: string,
  itemId: string,
  checked: boolean,
  principal: AuthPrincipal
): Promise<import('../types').IncidentChecklist | null> {
  const checklists = await getIncidentChecklists(env, incidentId)
  const checklist = checklists.find(c => c.id === checklistId)

  if (!checklist) return null

  const item = checklist.items.find(i => i.id === itemId)
  if (!item) return null

  item.checked = checked
  if (checked) {
    item.checked_at = nowIso()
    item.checked_by = principal.sub
  } else {
    delete item.checked_at
    delete item.checked_by
  }

  checklist.updated_at = nowIso()

  await env.KV.put(`${CHECKLIST_KEY}${incidentId}`, JSON.stringify(checklists), { expirationTtl: 86400 * 90 })

  return checklist
}

// ==================== Incident Change Links ====================

const CHANGE_LINK_KEY = 'incident:changes:'

export async function linkIncidentToChange(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  change: {
    change_id: string
    change_type: import('../types').IncidentChangeLink['change_type']
    change_description?: string
    change_url?: string
    change_timestamp: string
    relationship: import('../types').IncidentChangeLink['relationship']
  }
): Promise<import('../types').IncidentChangeLink> {
  const link: import('../types').IncidentChangeLink = {
    id: `change-${Date.now().toString(36)}`,
    incident_id: incidentId,
    ...change,
    created_by: principal.sub,
    created_at: nowIso(),
  }

  const links = (await env.KV.get(`${CHANGE_LINK_KEY}${incidentId}`, 'json') as import('../types').IncidentChangeLink[] | null) || []
  links.push(link)
  await env.KV.put(`${CHANGE_LINK_KEY}${incidentId}`, JSON.stringify(links), { expirationTtl: 86400 * 90 })

  return link
}

export async function getIncidentChanges(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentChangeLink[]> {
  return (await env.KV.get(`${CHANGE_LINK_KEY}${incidentId}`, 'json') as import('../types').IncidentChangeLink[] | null) || []
}

// ==================== Incident Run History ====================

const RUN_HISTORY_KEY = 'incident:runs:'

export async function recordIncidentRun(
  env: Env,
  incidentId: string,
  actionType: string,
  actionRef: string,
  triggeredBy: number
): Promise<import('../types').IncidentRunHistory> {
  const run: import('../types').IncidentRunHistory = {
    id: `run-${Date.now().toString(36)}`,
    incident_id: incidentId,
    action_type: actionType,
    action_ref: actionRef,
    triggered_by: triggeredBy,
    triggered_at: nowIso(),
    status: 'running',
    retry_count: 0,
  }

  const runs = (await env.KV.get(`${RUN_HISTORY_KEY}${incidentId}`, 'json') as import('../types').IncidentRunHistory[] | null) || []
  runs.push(run)
  await env.KV.put(`${RUN_HISTORY_KEY}${incidentId}`, JSON.stringify(runs), { expirationTtl: 86400 * 90 })

  return run
}

export async function getIncidentRunHistory(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentRunHistory[]> {
  return (await env.KV.get(`${RUN_HISTORY_KEY}${incidentId}`, 'json') as import('../types').IncidentRunHistory[] | null) || []
}

export async function updateIncidentRun(
  env: Env,
  incidentId: string,
  runId: string,
  status: 'success' | 'failed',
  result?: Record<string, unknown>,
  error?: string
): Promise<import('../types').IncidentRunHistory | null> {
  const runs = await getIncidentRunHistory(env, incidentId)
  const run = runs.find(r => r.id === runId)

  if (!run) return null

  run.status = status
  run.result = result
  run.error = error
  run.duration_ms = Date.now() - new Date(run.triggered_at).getTime()

  await env.KV.put(`${RUN_HISTORY_KEY}${incidentId}`, JSON.stringify(runs), { expirationTtl: 86400 * 90 })

  return run
}

// ==================== Responder Teams ====================

const RESPONDER_TEAM_KEY = 'incident:responder_teams'

export async function listResponderTeams(
  env: Env
): Promise<import('../types').ResponderTeam[]> {
  return (await env.KV.get(RESPONDER_TEAM_KEY, 'json') as import('../types').ResponderTeam[] | null) || []
}

export async function getResponderTeam(
  env: Env,
  teamId: string
): Promise<import('../types').ResponderTeam | null> {
  const teams = await listResponderTeams(env)
  return teams.find(t => t.id === teamId) || null
}

export async function createResponderTeam(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    members: Array<{
      user_id: number
      email: string
      role: 'lead' | 'responder' | 'observer'
      skills?: string[]
    }>
    services?: string[]
    escalation_policy_id?: string
    on_call_schedule_id?: string
  }
): Promise<import('../types').ResponderTeam> {
  const teams = await listResponderTeams(env)

  const team: import('../types').ResponderTeam = {
    id: `team-${Date.now().toString(36)}`,
    name: input.name,
    description: input.description,
    members: input.members,
    services: input.services || [],
    escalation_policy_id: input.escalation_policy_id,
    on_call_schedule_id: input.on_call_schedule_id,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  teams.push(team)
  await env.KV.put(RESPONDER_TEAM_KEY, JSON.stringify(teams), { expirationTtl: 86400 * 365 })

  return team
}

export async function updateResponderTeam(
  env: Env,
  teamId: string,
  updates: Partial<Omit<import('../types').ResponderTeam, 'id' | 'created_by' | 'created_at'>>
): Promise<import('../types').ResponderTeam | null> {
  const teams = await listResponderTeams(env)
  const index = teams.findIndex(t => t.id === teamId)

  if (index === -1) return null

  teams[index] = {
    ...teams[index],
    ...updates,
    updated_at: nowIso(),
  }

  await env.KV.put(RESPONDER_TEAM_KEY, JSON.stringify(teams), { expirationTtl: 86400 * 365 })

  return teams[index]
}

export async function deleteResponderTeam(
  env: Env,
  teamId: string
): Promise<boolean> {
  const teams = await listResponderTeams(env)
  const index = teams.findIndex(t => t.id === teamId)

  if (index === -1) return false

  teams.splice(index, 1)
  await env.KV.put(RESPONDER_TEAM_KEY, JSON.stringify(teams), { expirationTtl: 86400 * 365 })

  return true
}

// ==================== SLA Calendars ====================

const SLA_CALENDAR_KEY = 'incident:sla_calendars'

export async function listSLACalendars(
  env: Env
): Promise<import('../types').SLACalendar[]> {
  return (await env.KV.get(SLA_CALENDAR_KEY, 'json') as import('../types').SLACalendar[] | null) || []
}

export async function getSLACalendar(
  env: Env,
  calendarId: string
): Promise<import('../types').SLACalendar | null> {
  const calendars = await listSLACalendars(env)
  return calendars.find(c => c.id === calendarId) || null
}

export async function getDefaultSLACalendar(
  env: Env
): Promise<import('../types').SLACalendar | null> {
  const calendars = await listSLACalendars(env)
  return calendars.find(c => c.is_default) || null
}

export async function createSLACalendar(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    timezone: string
    working_hours: import('../types').SLACalendar['working_hours']
    holidays?: import('../types').SLACalendar['holidays']
    is_default?: boolean
  }
): Promise<import('../types').SLACalendar> {
  const calendars = await listSLACalendars(env)

  // If this is set as default, remove default from others
  if (input.is_default) {
    for (const cal of calendars) {
      if (cal.is_default) {
        cal.is_default = false
      }
    }
  }

  const calendar: import('../types').SLACalendar = {
    id: `sla-cal-${Date.now().toString(36)}`,
    name: input.name,
    description: input.description,
    timezone: input.timezone,
    working_hours: input.working_hours,
    holidays: input.holidays || [],
    is_default: input.is_default ?? calendars.length === 0,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  calendars.push(calendar)
  await env.KV.put(SLA_CALENDAR_KEY, JSON.stringify(calendars), { expirationTtl: 86400 * 365 })

  return calendar
}

export async function calculateBusinessMinutes(
  env: Env,
  calendarId: string,
  start: Date,
  end: Date
): Promise<number> {
  const calendar = await getSLACalendar(env, calendarId)
  if (!calendar) {
    // Default to all minutes if no calendar
    return (end.getTime() - start.getTime()) / 60000
  }

  let totalMinutes = 0
  const current = new Date(start)

  while (current < end) {
    const dayOfWeek = current.getDay()

    // Check if it's a working day
    if (calendar.working_hours.days.includes(dayOfWeek)) {
      // Check if it's a holiday
      const dateStr = current.toISOString().split('T')[0]
      const isHoliday = calendar.holidays.some(h => h.date === dateStr)

      if (!isHoliday) {
        // Calculate working hours for this day
        const dayStart = new Date(current)
        const [startHour, startMin] = calendar.working_hours.start.split(':').map(Number)
        dayStart.setHours(startHour, startMin, 0, 0)

        const dayEnd = new Date(current)
        const [endHour, endMin] = calendar.working_hours.end.split(':').map(Number)
        dayEnd.setHours(endHour, endMin, 0, 0)

        // Intersect with our range
        const effectiveStart = new Date(Math.max(dayStart.getTime(), start.getTime()))
        const effectiveEnd = new Date(Math.min(dayEnd.getTime(), end.getTime()))

        if (effectiveEnd > effectiveStart) {
          totalMinutes += (effectiveEnd.getTime() - effectiveStart.getTime()) / 60000
        }
      }
    }

    // Move to next day
    current.setDate(current.getDate() + 1)
    current.setHours(0, 0, 0, 0)
  }

  return totalMinutes
}

// ==================== Notification Templates ====================

const NOTIFICATION_TEMPLATE_KEY = 'incident:notification_templates'

export async function listNotificationTemplates(
  env: Env,
  channel?: string
): Promise<import('../types').NotificationTemplate[]> {
  const templates = (await env.KV.get(NOTIFICATION_TEMPLATE_KEY, 'json') as import('../types').NotificationTemplate[] | null) || []
  if (channel) {
    return templates.filter(t => t.channel === channel)
  }
  return templates
}

export async function getNotificationTemplate(
  env: Env,
  templateId: string
): Promise<import('../types').NotificationTemplate | null> {
  const templates = await listNotificationTemplates(env)
  return templates.find(t => t.id === templateId) || null
}

export async function createNotificationTemplate(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    channel: import('../types').NotificationTemplate['channel']
    event_type: import('../types').NotificationTemplate['event_type']
    subject_template?: string
    body_template: string
    variables?: string[]
  }
): Promise<import('../types').NotificationTemplate> {
  const templates = await listNotificationTemplates(env)

  const template: import('../types').NotificationTemplate = {
    id: `notif-tpl-${Date.now().toString(36)}`,
    name: input.name,
    description: input.description,
    channel: input.channel,
    event_type: input.event_type,
    subject_template: input.subject_template,
    body_template: input.body_template,
    variables: input.variables || [],
    enabled: true,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  templates.push(template)
  await env.KV.put(NOTIFICATION_TEMPLATE_KEY, JSON.stringify(templates), { expirationTtl: 86400 * 365 })

  return template
}

export function renderNotificationTemplate(
  template: import('../types').NotificationTemplate,
  variables: Record<string, unknown>
): { subject?: string; body: string } {
  let body = template.body_template
  let subject = template.subject_template

  // Simple variable substitution
  for (const [key, value] of Object.entries(variables)) {
    const pattern = new RegExp(`\\{\\{\\s*${key}\\s*\\}\\}`, 'g')
    const strValue = String(value)
    body = body.replace(pattern, strValue)
    if (subject) {
      subject = subject.replace(pattern, strValue)
    }
  }

  return { subject, body }
}

// ==================== Severity Escalation Rules ====================

const ESCALATION_RULE_KEY = 'incident:escalation_rules'

export async function listEscalationRules(
  env: Env,
  enabled?: boolean
): Promise<import('../types').SeverityEscalationRule[]> {
  const rules = (await env.KV.get(ESCALATION_RULE_KEY, 'json') as import('../types').SeverityEscalationRule[] | null) || []
  if (enabled === undefined) return rules
  return rules.filter(r => r.enabled === enabled)
}

export async function createEscalationRule(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    description?: string
    from_severity: IncidentSeverity
    to_severity: IncidentSeverity
    trigger_conditions: import('../types').SeverityEscalationRule['trigger_conditions']
    actions: import('../types').SeverityEscalationRule['actions']
  }
): Promise<import('../types').SeverityEscalationRule> {
  const rules = await listEscalationRules(env)

  const rule: import('../types').SeverityEscalationRule = {
    id: `esc-rule-${Date.now().toString(36)}`,
    name: input.name,
    description: input.description,
    from_severity: input.from_severity,
    to_severity: input.to_severity,
    trigger_conditions: input.trigger_conditions,
    actions: input.actions,
    enabled: true,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  rules.push(rule)
  await env.KV.put(ESCALATION_RULE_KEY, JSON.stringify(rules), { expirationTtl: 86400 * 365 })

  return rule
}

export async function checkEscalationRules(
  env: Env,
  incident: IncidentRecord
): Promise<import('../types').SeverityEscalationRule | null> {
  const rules = await listEscalationRules(env, true)
  const created = new Date(incident.created_at)
  const now = new Date()
  const minutesSinceCreation = (now.getTime() - created.getTime()) / 60000

  for (const rule of rules) {
    if (rule.from_severity !== incident.severity) continue

    const conditions = rule.trigger_conditions
    let shouldEscalate = false

    if (conditions.time_without_ack_minutes && !incident.acknowledged_at) {
      if (minutesSinceCreation >= conditions.time_without_ack_minutes) {
        shouldEscalate = true
      }
    }

    if (conditions.time_without_assign_minutes && !incident.assignee_id) {
      if (minutesSinceCreation >= conditions.time_without_assign_minutes) {
        shouldEscalate = true
      }
    }

    if (shouldEscalate) {
      return rule
    }
  }

  return null
}

// ==================== Incident Attachments ====================

const ATTACHMENT_KEY = 'incident:attachments:'

export async function listIncidentAttachments(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentAttachment[]> {
  return (await env.KV.get(`${ATTACHMENT_KEY}${incidentId}`, 'json') as import('../types').IncidentAttachment[] | null) || []
}

export async function uploadIncidentAttachment(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  file: {
    filename: string
    contentType: string
    content: ArrayBuffer
  },
  description?: string
): Promise<import('../types').IncidentAttachment> {
  const attachmentId = `attach-${Date.now().toString(36)}`
  const storageKey = `incident-attachments/${incidentId}/${attachmentId}-${file.filename}`

  // Store in R2
  await env.R2.put(storageKey, file.content, {
    httpMetadata: {
      contentType: file.contentType,
    },
    customMetadata: {
      incident_id: incidentId,
      uploaded_by: String(principal.sub),
    },
  })

  const attachment: import('../types').IncidentAttachment = {
    id: attachmentId,
    incident_id: incidentId,
    filename: file.filename,
    content_type: file.contentType,
    size_bytes: file.content.byteLength,
    storage_key: storageKey,
    uploaded_by: principal.sub,
    uploaded_at: nowIso(),
    description,
  }

  const attachments = await listIncidentAttachments(env, incidentId)
  attachments.push(attachment)
  await env.KV.put(`${ATTACHMENT_KEY}${incidentId}`, JSON.stringify(attachments), { expirationTtl: 86400 * 90 })

  return attachment
}

export async function downloadIncidentAttachment(
  env: Env,
  incidentId: string,
  attachmentId: string
): Promise<{ body: ReadableStream; contentType: string; filename: string } | null> {
  const attachments = await listIncidentAttachments(env, incidentId)
  const attachment = attachments.find(a => a.id === attachmentId)

  if (!attachment) return null

  const object = await env.R2.get(attachment.storage_key)
  if (!object) return null

  return {
    body: object.body,
    contentType: attachment.content_type,
    filename: attachment.filename,
  }
}

export async function deleteIncidentAttachment(
  env: Env,
  incidentId: string,
  attachmentId: string
): Promise<boolean> {
  const attachments = await listIncidentAttachments(env, incidentId)
  const index = attachments.findIndex(a => a.id === attachmentId)

  if (index === -1) return false

  const attachment = attachments[index]

  // Delete from R2
  await env.R2.delete(attachment.storage_key)

  // Remove from list
  attachments.splice(index, 1)
  await env.KV.put(`${ATTACHMENT_KEY}${incidentId}`, JSON.stringify(attachments), { expirationTtl: 86400 * 90 })

  return true
}

// ==================== Incident Related Items ====================

const RELATED_ITEM_KEY = 'incident:related_items:'

export async function listRelatedItems(
  env: Env,
  incidentId: string,
  itemType?: string
): Promise<import('../types').IncidentRelatedItem[]> {
  const items = (await env.KV.get(`${RELATED_ITEM_KEY}${incidentId}`, 'json') as import('../types').IncidentRelatedItem[] | null) || []
  if (itemType) {
    return items.filter(i => i.item_type === itemType)
  }
  return items
}

export async function addRelatedItem(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  input: {
    item_type: import('../types').IncidentRelatedItem['item_type']
    title: string
    description?: string
    url?: string
    content?: string
    metadata?: Record<string, unknown>
  }
): Promise<import('../types').IncidentRelatedItem> {
  const item: import('../types').IncidentRelatedItem = {
    id: `item-${Date.now().toString(36)}`,
    incident_id: incidentId,
    ...input,
    added_by: principal.sub,
    added_at: nowIso(),
  }

  const items = await listRelatedItems(env, incidentId)
  items.push(item)
  await env.KV.put(`${RELATED_ITEM_KEY}${incidentId}`, JSON.stringify(items), { expirationTtl: 86400 * 90 })

  return item
}

export async function removeRelatedItem(
  env: Env,
  incidentId: string,
  itemId: string
): Promise<boolean> {
  const items = await listRelatedItems(env, incidentId)
  const index = items.findIndex(i => i.id === itemId)

  if (index === -1) return false

  items.splice(index, 1)
  await env.KV.put(`${RELATED_ITEM_KEY}${incidentId}`, JSON.stringify(items), { expirationTtl: 86400 * 90 })

  return true
}

// ==================== Response Time Targets ====================

const RESPONSE_TARGET_KEY = 'incident:response_targets'

export async function listResponseTimeTargets(
  env: Env,
  enabled?: boolean
): Promise<import('../types').ResponseTimeTarget[]> {
  const targets = (await env.KV.get(RESPONSE_TARGET_KEY, 'json') as import('../types').ResponseTimeTarget[] | null) || []
  if (enabled === undefined) return targets
  return targets.filter(t => t.enabled === enabled)
}

export async function createResponseTimeTarget(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    severity: IncidentSeverity
    target_type: import('../types').ResponseTimeTarget['target_type']
    target_minutes: number
    business_hours_only?: boolean
    sla_calendar_id?: string
  }
): Promise<import('../types').ResponseTimeTarget> {
  const targets = await listResponseTimeTargets(env)

  const target: import('../types').ResponseTimeTarget = {
    id: `resp-target-${Date.now().toString(36)}`,
    name: input.name,
    severity: input.severity,
    target_type: input.target_type,
    target_minutes: input.target_minutes,
    business_hours_only: input.business_hours_only ?? false,
    sla_calendar_id: input.sla_calendar_id,
    enabled: true,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  targets.push(target)
  await env.KV.put(RESPONSE_TARGET_KEY, JSON.stringify(targets), { expirationTtl: 86400 * 365 })

  return target
}

// ==================== Incident Integrations ====================

const INTEGRATION_KEY = 'incident:integrations'

export async function listIntegrations(
  env: Env,
  enabled?: boolean
): Promise<import('../types').IncidentIntegrationConfig[]> {
  const integrations = (await env.KV.get(INTEGRATION_KEY, 'json') as import('../types').IncidentIntegrationConfig[] | null) || []
  if (enabled === undefined) return integrations
  return integrations.filter(i => i.enabled === enabled)
}

export async function createIntegration(
  env: Env,
  principal: AuthPrincipal,
  input: {
    name: string
    type: import('../types').IncidentIntegrationConfig['type']
    config: Record<string, unknown>
    mapping_rules?: import('../types').IncidentIntegrationConfig['mapping_rules']
  }
): Promise<import('../types').IncidentIntegrationConfig> {
  const integrations = await listIntegrations(env)

  const integration: import('../types').IncidentIntegrationConfig = {
    id: `int-${Date.now().toString(36)}`,
    name: input.name,
    type: input.type,
    enabled: true,
    config: input.config,
    mapping_rules: input.mapping_rules || [],
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }

  integrations.push(integration)
  await env.KV.put(INTEGRATION_KEY, JSON.stringify(integrations), { expirationTtl: 86400 * 365 })

  return integration
}

export async function updateIntegration(
  env: Env,
  integrationId: string,
  updates: Partial<Omit<import('../types').IncidentIntegrationConfig, 'id' | 'created_by' | 'created_at'>>
): Promise<import('../types').IncidentIntegrationConfig | null> {
  const integrations = await listIntegrations(env)
  const index = integrations.findIndex(i => i.id === integrationId)

  if (index === -1) return null

  integrations[index] = {
    ...integrations[index],
    ...updates,
    updated_at: nowIso(),
  }

  await env.KV.put(INTEGRATION_KEY, JSON.stringify(integrations), { expirationTtl: 86400 * 365 })

  return integrations[index]
}

export async function deleteIntegration(
  env: Env,
  integrationId: string
): Promise<boolean> {
  const integrations = await listIntegrations(env)
  const index = integrations.findIndex(i => i.id === integrationId)

  if (index === -1) return false

  integrations.splice(index, 1)
  await env.KV.put(INTEGRATION_KEY, JSON.stringify(integrations), { expirationTtl: 86400 * 365 })

  return true
}

// ===== Timeline Events =====

const TIMELINE_KEY = 'incident:timeline:'

export async function listTimelineEvents(
  env: Env,
  incidentId: string
): Promise<import('../types').IncidentTimelineEvent[]> {
  const data = await env.KV.get(TIMELINE_KEY + incidentId)
  return data ? JSON.parse(data) : []
}

export async function addTimelineEvent(
  env: Env,
  incidentId: string,
  event: Omit<import('../types').IncidentTimelineEvent, 'id' | 'incident_id'>
): Promise<import('../types').IncidentTimelineEvent> {
  const events = await listTimelineEvents(env, incidentId)
  const newEvent: import('../types').IncidentTimelineEvent = {
    ...event,
    id: `timeline-${nanoid(8)}`,
    incident_id: incidentId,
    timestamp: event.timestamp || nowIso(),
  }
  events.push(newEvent)
  await env.KV.put(TIMELINE_KEY + incidentId, JSON.stringify(events), { expirationTtl: 86400 * 365 })
  return newEvent
}

// ===== Runbooks =====

const RUNBOOK_KEY = 'incident:runbooks'

export async function listRunbooks(
  env: Env,
  category?: string,
  enabled?: boolean
): Promise<import('../types').IncidentRunbook[]> {
  const data = await env.KV.get(RUNBOOK_KEY)
  let runbooks: import('../types').IncidentRunbook[] = data ? JSON.parse(data) : []

  if (category) {
    runbooks = runbooks.filter(r => r.category === category)
  }
  if (enabled !== undefined) {
    runbooks = runbooks.filter(r => r.enabled === enabled)
  }

  return runbooks
}

export async function getRunbook(
  env: Env,
  runbookId: string
): Promise<import('../types').IncidentRunbook | null> {
  const runbooks = await listRunbooks(env)
  return runbooks.find(r => r.id === runbookId) || null
}

export async function createRunbook(
  env: Env,
  principal: AuthPrincipal,
  input: Omit<import('../types').IncidentRunbook, 'id' | 'version' | 'created_by' | 'created_at' | 'updated_at'>
): Promise<import('../types').IncidentRunbook> {
  const runbooks = await listRunbooks(env)
  const newRunbook: import('../types').IncidentRunbook = {
    ...input,
    id: `runbook-${nanoid(8)}`,
    version: 1,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }
  runbooks.push(newRunbook)
  await env.KV.put(RUNBOOK_KEY, JSON.stringify(runbooks), { expirationTtl: 86400 * 365 })
  return newRunbook
}

export async function updateRunbook(
  env: Env,
  runbookId: string,
  updates: Partial<Omit<import('../types').IncidentRunbook, 'id' | 'created_by' | 'created_at'>>
): Promise<import('../types').IncidentRunbook | null> {
  const runbooks = await listRunbooks(env)
  const index = runbooks.findIndex(r => r.id === runbookId)

  if (index === -1) return null

  runbooks[index] = {
    ...runbooks[index],
    ...updates,
    version: runbooks[index].version + 1,
    updated_at: nowIso(),
  }

  await env.KV.put(RUNBOOK_KEY, JSON.stringify(runbooks), { expirationTtl: 86400 * 365 })
  return runbooks[index]
}

export async function deleteRunbook(env: Env, runbookId: string): Promise<boolean> {
  const runbooks = await listRunbooks(env)
  const index = runbooks.findIndex(r => r.id === runbookId)

  if (index === -1) return false

  runbooks.splice(index, 1)
  await env.KV.put(RUNBOOK_KEY, JSON.stringify(runbooks), { expirationTtl: 86400 * 365 })
  return true
}

// ===== Auto-Remediation Rules =====

const AUTO_REMEDIATION_KEY = 'incident:auto_remediation_rules'

export async function listAutoRemediationRules(
  env: Env,
  enabled?: boolean
): Promise<import('../types').AutoRemediationRule[]> {
  const data = await env.KV.get(AUTO_REMEDIATION_KEY)
  let rules: import('../types').AutoRemediationRule[] = data ? JSON.parse(data) : []

  if (enabled !== undefined) {
    rules = rules.filter(r => r.enabled === enabled)
  }

  return rules
}

export async function createAutoRemediationRule(
  env: Env,
  principal: AuthPrincipal,
  input: Omit<import('../types').AutoRemediationRule, 'id' | 'created_by' | 'created_at' | 'updated_at'>
): Promise<import('../types').AutoRemediationRule> {
  const rules = await listAutoRemediationRules(env)
  const newRule: import('../types').AutoRemediationRule = {
    ...input,
    id: `autorem-${nanoid(8)}`,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }
  rules.push(newRule)
  await env.KV.put(AUTO_REMEDIATION_KEY, JSON.stringify(rules), { expirationTtl: 86400 * 365 })
  return newRule
}

export async function updateAutoRemediationRule(
  env: Env,
  ruleId: string,
  updates: Partial<Omit<import('../types').AutoRemediationRule, 'id' | 'created_by' | 'created_at'>>
): Promise<import('../types').AutoRemediationRule | null> {
  const rules = await listAutoRemediationRules(env)
  const index = rules.findIndex(r => r.id === ruleId)

  if (index === -1) return null

  rules[index] = {
    ...rules[index],
    ...updates,
    updated_at: nowIso(),
  }

  await env.KV.put(AUTO_REMEDIATION_KEY, JSON.stringify(rules), { expirationTtl: 86400 * 365 })
  return rules[index]
}

export async function deleteAutoRemediationRule(env: Env, ruleId: string): Promise<boolean> {
  const rules = await listAutoRemediationRules(env)
  const index = rules.findIndex(r => r.id === ruleId)

  if (index === -1) return false

  rules.splice(index, 1)
  await env.KV.put(AUTO_REMEDIATION_KEY, JSON.stringify(rules), { expirationTtl: 86400 * 365 })
  return true
}

export async function evaluateAutoRemediationRules(
  env: Env,
  incident: IncidentRecord
): Promise<import('../types').AutoRemediationRule[]> {
  const rules = await listAutoRemediationRules(env, true)
  const matchingRules: import('../types').AutoRemediationRule[] = []

  for (const rule of rules) {
    let matches = rule.logical_operator === 'and'

    for (const condition of rule.conditions) {
      const incidentValue = getIncidentFieldValue(incident, condition.field)
      const conditionMatches = evaluateCondition(incidentValue, condition.operator, condition.value)

      if (rule.logical_operator === 'and') {
        matches = matches && conditionMatches
      } else {
        matches = matches || conditionMatches
      }
    }

    if (matches) {
      matchingRules.push(rule)
    }
  }

  return matchingRules
}

function getIncidentFieldValue(incident: IncidentRecord, field: string): unknown {
  switch (field) {
    case 'severity': return incident.severity
    case 'source': return incident.source
    case 'title_pattern': return incident.title
    case 'tag': return incident.tags?.join(',')
    case 'service': return incident.source // Use source as service equivalent
    default: return null
  }
}

function evaluateCondition(value: unknown, operator: string, conditionValue: string | number): boolean {
  if (value === null || value === undefined) return false

  const strValue = String(value)

  switch (operator) {
    case 'equals': return strValue === String(conditionValue)
    case 'contains': return strValue.includes(String(conditionValue))
    case 'matches': return new RegExp(String(conditionValue)).test(strValue)
    case 'greater_than': return Number(value) > Number(conditionValue)
    case 'less_than': return Number(value) < Number(conditionValue)
    default: return false
  }
}

// ===== Maintenance Windows =====

const MAINTENANCE_KEY = 'incident:maintenance_windows'

export async function listMaintenanceWindows(
  env: Env,
  status?: string,
  service?: string
): Promise<import('../types').MaintenanceWindow[]> {
  const data = await env.KV.get(MAINTENANCE_KEY)
  let windows: import('../types').MaintenanceWindow[] = data ? JSON.parse(data) : []

  if (status) {
    windows = windows.filter(w => w.status === status)
  }
  if (service) {
    windows = windows.filter(w => w.services.includes(service))
  }

  return windows
}

export async function getMaintenanceWindow(
  env: Env,
  windowId: string
): Promise<import('../types').MaintenanceWindow | null> {
  const windows = await listMaintenanceWindows(env)
  return windows.find(w => w.id === windowId) || null
}

export async function createMaintenanceWindow(
  env: Env,
  principal: AuthPrincipal,
  input: Omit<import('../types').MaintenanceWindow, 'id' | 'status' | 'created_by' | 'created_at' | 'updated_at'>
): Promise<import('../types').MaintenanceWindow> {
  const windows = await listMaintenanceWindows(env)
  const newWindow: import('../types').MaintenanceWindow = {
    ...input,
    id: `maint-${nanoid(8)}`,
    status: 'scheduled',
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }
  windows.push(newWindow)
  await env.KV.put(MAINTENANCE_KEY, JSON.stringify(windows), { expirationTtl: 86400 * 365 })
  return newWindow
}

export async function updateMaintenanceWindow(
  env: Env,
  windowId: string,
  updates: Partial<Omit<import('../types').MaintenanceWindow, 'id' | 'created_by' | 'created_at'>>
): Promise<import('../types').MaintenanceWindow | null> {
  const windows = await listMaintenanceWindows(env)
  const index = windows.findIndex(w => w.id === windowId)

  if (index === -1) return null

  windows[index] = {
    ...windows[index],
    ...updates,
    updated_at: nowIso(),
  }

  await env.KV.put(MAINTENANCE_KEY, JSON.stringify(windows), { expirationTtl: 86400 * 365 })
  return windows[index]
}

export async function cancelMaintenanceWindow(env: Env, windowId: string): Promise<boolean> {
  return (await updateMaintenanceWindow(env, windowId, { status: 'cancelled' })) !== null
}

export async function getActiveMaintenanceWindows(
  env: Env,
  service?: string
): Promise<import('../types').MaintenanceWindow[]> {
  const now = new Date()
  const windows = await listMaintenanceWindows(env, 'active', service)

  return windows.filter(w => {
    const start = new Date(w.start_time)
    const end = new Date(w.end_time)
    return start <= now && now <= end
  })
}

// ===== Bulk Operations =====

const BULK_OP_KEY = 'incident:bulk_operations'

export async function listBulkOperations(
  env: Env,
  status?: string
): Promise<import('../types').IncidentBulkOperation[]> {
  const data = await env.KV.get(BULK_OP_KEY)
  let ops: import('../types').IncidentBulkOperation[] = data ? JSON.parse(data) : []

  if (status) {
    ops = ops.filter(o => o.status === status)
  }

  return ops
}

export async function getBulkOperation(
  env: Env,
  operationId: string
): Promise<import('../types').IncidentBulkOperation | null> {
  const ops = await listBulkOperations(env)
  return ops.find(o => o.id === operationId) || null
}

export async function createBulkOperation(
  env: Env,
  principal: AuthPrincipal,
  input: Omit<import('../types').IncidentBulkOperation, 'id' | 'status' | 'results' | 'created_at' | 'completed_at' | 'created_by'>
): Promise<import('../types').IncidentBulkOperation> {
  const ops = await listBulkOperations(env)
  const newOp: import('../types').IncidentBulkOperation = {
    ...input,
    id: `bulk-${nanoid(8)}`,
    status: 'pending',
    results: [],
    created_by: principal.sub,
    created_at: nowIso(),
  }
  ops.push(newOp)
  await env.KV.put(BULK_OP_KEY, JSON.stringify(ops), { expirationTtl: 86400 * 365 })
  return newOp
}

export async function executeBulkOperation(
  env: Env,
  operationId: string
): Promise<import('../types').IncidentBulkOperation | null> {
  const ops = await listBulkOperations(env)
  const index = ops.findIndex(o => o.id === operationId)

  if (index === -1) return null

  const op = ops[index]
  op.status = 'processing'

  for (const incidentId of op.incident_ids) {
    try {
      const incident = await getIncident(env, incidentId)
      if (!incident) {
        op.results.push({ incident_id: incidentId, success: false, error: 'Incident not found' })
        continue
      }

      // Apply the operation based on type
      switch (op.operation_type) {
        case 'status_change':
          await updateIncident(env, incidentId, { status: op.payload.status as IncidentStatus })
          break
        case 'severity_change':
          await updateIncident(env, incidentId, { severity: op.payload.severity as IncidentSeverity })
          break
        case 'assign':
          await updateIncident(env, incidentId, { assignee_id: op.payload.assigned_to as number })
          break
        case 'tag':
          const existingTags = incident.tags || []
          const newTags = [...new Set([...existingTags, ...(op.payload.tags as string[])])]
          await updateIncident(env, incidentId, { tags: newTags })
          break
        case 'close':
          await updateIncident(env, incidentId, { status: 'resolved' })
          break
        case 'escalate':
          const escalationOrder: IncidentSeverity[] = ['low', 'medium', 'high', 'critical']
          const currentIndex = escalationOrder.indexOf(incident.severity)
          if (currentIndex < escalationOrder.length - 1) {
            await updateIncident(env, incidentId, { severity: escalationOrder[currentIndex + 1] })
          }
          break
      }

      op.results.push({ incident_id: incidentId, success: true })
    } catch (error) {
      op.results.push({ incident_id: incidentId, success: false, error: String(error) })
    }
  }

  op.status = op.results.every(r => r.success) ? 'completed' : op.results.some(r => r.success) ? 'partial' : 'failed'
  op.completed_at = nowIso()

  await env.KV.put(BULK_OP_KEY, JSON.stringify(ops), { expirationTtl: 86400 * 365 })
  return op
}

// ===== SLA Breaches =====

const SLA_BREACH_KEY = 'incident:sla_breaches'

export async function listSLABreaches(
  env: Env,
  incidentId?: string,
  severity?: IncidentSeverity
): Promise<import('../types').IncidentSLABreach[]> {
  const data = await env.KV.get(SLA_BREACH_KEY)
  let breaches: import('../types').IncidentSLABreach[] = data ? JSON.parse(data) : []

  if (incidentId) {
    breaches = breaches.filter(b => b.incident_id === incidentId)
  }
  if (severity) {
    breaches = breaches.filter(b => b.severity === severity)
  }

  return breaches
}

export async function createSLABreach(
  env: Env,
  breach: Omit<import('../types').IncidentSLABreach, 'id'>
): Promise<import('../types').IncidentSLABreach> {
  const breaches = await listSLABreaches(env)
  const newBreach: import('../types').IncidentSLABreach = {
    ...breach,
    id: `breach-${nanoid(8)}`,
  }
  breaches.push(newBreach)
  await env.KV.put(SLA_BREACH_KEY, JSON.stringify(breaches), { expirationTtl: 86400 * 365 })
  return newBreach
}

export async function acknowledgeSLABreach(
  env: Env,
  breachId: string
): Promise<import('../types').IncidentSLABreach | null> {
  const breaches = await listSLABreaches(env)
  const index = breaches.findIndex(b => b.id === breachId)

  if (index === -1) return null

  breaches[index].acknowledged_at = nowIso()
  await env.KV.put(SLA_BREACH_KEY, JSON.stringify(breaches), { expirationTtl: 86400 * 365 })
  return breaches[index]
}

// ===== Analytics Snapshots =====

const ANALYTICS_KEY = 'incident:analytics'

export async function listAnalyticsSnapshots(
  env: Env,
  period?: string,
  startDate?: string,
  endDate?: string
): Promise<import('../types').IncidentAnalyticsSnapshot[]> {
  const data = await env.KV.get(ANALYTICS_KEY)
  let snapshots: import('../types').IncidentAnalyticsSnapshot[] = data ? JSON.parse(data) : []

  if (period) {
    snapshots = snapshots.filter(s => s.period === period)
  }
  if (startDate) {
    snapshots = snapshots.filter(s => s.snapshot_date >= startDate)
  }
  if (endDate) {
    snapshots = snapshots.filter(s => s.snapshot_date <= endDate)
  }

  return snapshots.sort((a, b) => b.snapshot_date.localeCompare(a.snapshot_date))
}

export async function generateAnalyticsSnapshot(
  env: Env,
  date: string,
  period: 'daily' | 'weekly' | 'monthly'
): Promise<import('../types').IncidentAnalyticsSnapshot> {
  const incidentsResult = await listIncidents(env)
  const incidents = incidentsResult.items
  const breaches = await listSLABreaches(env)

  // Filter incidents for the period
  const periodStart = new Date(date)
  let periodEnd = new Date(date)

  if (period === 'daily') {
    periodEnd.setDate(periodEnd.getDate() + 1)
  } else if (period === 'weekly') {
    periodEnd.setDate(periodEnd.getDate() + 7)
  } else {
    periodEnd.setMonth(periodEnd.getMonth() + 1)
  }

  const periodIncidents = incidents.filter(i => {
    const created = new Date(i.created_at)
    return periodStart <= created && created < periodEnd
  })

  const periodBreaches = breaches.filter(b => {
    const breached = new Date(b.breached_at)
    return periodStart <= breached && breached < periodEnd
  })

  // Calculate metrics
  const bySeverity: Record<IncidentSeverity, number> = { low: 0, medium: 0, high: 0, critical: 0 }
  const byStatus: Record<IncidentStatus, number> = { open: 0, analyzed: 0, approved: 0, executing: 0, resolved: 0, failed: 0 }
  const bySource: Record<string, number> = {}

  for (const incident of periodIncidents) {
    bySeverity[incident.severity]++
    byStatus[incident.status]++
    bySource[incident.source] = (bySource[incident.source] || 0) + 1
  }

  // Calculate MTTR - use updated_at for resolved incidents
  const resolvedIncidents = periodIncidents.filter(i => i.status === 'resolved' || i.status === 'failed')
  let totalResolutionTime = 0
  for (const incident of resolvedIncidents) {
    const created = new Date(incident.created_at)
    const resolved = new Date(incident.updated_at)
    totalResolutionTime += resolved.getTime() - created.getTime()
  }
  const mttr_minutes = resolvedIncidents.length > 0 ? totalResolutionTime / resolvedIncidents.length / 60000 : 0

  // Calculate MTTA
  const acknowledgedIncidents = periodIncidents.filter(i => i.acknowledged_at)
  let totalAckTime = 0
  for (const incident of acknowledgedIncidents) {
    const created = new Date(incident.created_at)
    const acknowledged = new Date(incident.acknowledged_at!)
    totalAckTime += acknowledged.getTime() - created.getTime()
  }
  const mtta_minutes = acknowledgedIncidents.length > 0 ? totalAckTime / acknowledgedIncidents.length / 60000 : 0

  const snapshot: import('../types').IncidentAnalyticsSnapshot = {
    id: `analytics-${nanoid(8)}`,
    snapshot_date: date,
    period,
    metrics: {
      total_incidents: periodIncidents.length,
      by_severity: bySeverity,
      by_status: byStatus,
      by_source: bySource,
      mttr_minutes,
      mtta_minutes,
      first_response_minutes: mtta_minutes, // Simplified
      sla_breach_count: periodBreaches.length,
      sla_breach_rate: periodIncidents.length > 0 ? periodBreaches.length / periodIncidents.length : 0,
      escalation_count: periodIncidents.filter(i => i.severity === 'critical' || i.severity === 'high').length,
      auto_resolved_count: 0, // Would need tracking
      avg_participants: 0, // Would need tracking
      avg_comments: 0, // Would need tracking
      avg_duration_minutes: mttr_minutes,
    },
    created_at: nowIso(),
  }

  // Save snapshot
  const snapshots = await listAnalyticsSnapshots(env)
  snapshots.push(snapshot)
  await env.KV.put(ANALYTICS_KEY, JSON.stringify(snapshots), { expirationTtl: 86400 * 365 })

  return snapshot
}

// ===== Webhook Subscriptions =====

const WEBHOOK_SUB_KEY = 'incident:webhook_subscriptions'

export async function listWebhookSubscriptions(
  env: Env,
  enabled?: boolean
): Promise<import('../types').IncidentWebhookSubscription[]> {
  const data = await env.KV.get(WEBHOOK_SUB_KEY)
  let subs: import('../types').IncidentWebhookSubscription[] = data ? JSON.parse(data) : []

  if (enabled !== undefined) {
    subs = subs.filter(s => s.enabled === enabled)
  }

  return subs
}

export async function getWebhookSubscription(
  env: Env,
  subscriptionId: string
): Promise<import('../types').IncidentWebhookSubscription | null> {
  const subs = await listWebhookSubscriptions(env)
  return subs.find(s => s.id === subscriptionId) || null
}

export async function createWebhookSubscription(
  env: Env,
  principal: AuthPrincipal,
  input: Omit<import('../types').IncidentWebhookSubscription, 'id' | 'failure_count' | 'created_by' | 'created_at' | 'updated_at'>
): Promise<import('../types').IncidentWebhookSubscription> {
  const subs = await listWebhookSubscriptions(env)
  const newSub: import('../types').IncidentWebhookSubscription = {
    ...input,
    id: `webhook-${nanoid(8)}`,
    failure_count: 0,
    created_by: principal.sub,
    created_at: nowIso(),
    updated_at: nowIso(),
  }
  subs.push(newSub)
  await env.KV.put(WEBHOOK_SUB_KEY, JSON.stringify(subs), { expirationTtl: 86400 * 365 })
  return newSub
}

export async function updateWebhookSubscription(
  env: Env,
  subscriptionId: string,
  updates: Partial<Omit<import('../types').IncidentWebhookSubscription, 'id' | 'created_by' | 'created_at'>>
): Promise<import('../types').IncidentWebhookSubscription | null> {
  const subs = await listWebhookSubscriptions(env)
  const index = subs.findIndex(s => s.id === subscriptionId)

  if (index === -1) return null

  subs[index] = {
    ...subs[index],
    ...updates,
    updated_at: nowIso(),
  }

  await env.KV.put(WEBHOOK_SUB_KEY, JSON.stringify(subs), { expirationTtl: 86400 * 365 })
  return subs[index]
}

export async function deleteWebhookSubscription(env: Env, subscriptionId: string): Promise<boolean> {
  const subs = await listWebhookSubscriptions(env)
  const index = subs.findIndex(s => s.id === subscriptionId)

  if (index === -1) return false

  subs.splice(index, 1)
  await env.KV.put(WEBHOOK_SUB_KEY, JSON.stringify(subs), { expirationTtl: 86400 * 365 })
  return true
}

export async function triggerIncidentWebhooks(
  env: Env,
  event: WebhookEventType,
  payload: Record<string, unknown>
): Promise<void> {
  const subs = await listWebhookSubscriptions(env, true)
  const matchingSubs = subs.filter(s => s.events.includes(event))

  for (const sub of matchingSubs) {
    // Check filters
    if (sub.filters) {
      if (sub.filters.severities && payload.severity && !sub.filters.severities.includes(payload.severity as IncidentSeverity)) {
        continue
      }
      if (sub.filters.services && payload.service && !sub.filters.services.includes(payload.service as string)) {
        continue
      }
    }

    try {
      const response = await fetch(sub.url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...sub.headers,
        },
        body: JSON.stringify({
          event,
          timestamp: nowIso(),
          data: payload,
        }),
      })

      if (!response.ok) {
        await updateWebhookSubscription(env, sub.id, { failure_count: sub.failure_count + 1 })
      } else {
        await updateWebhookSubscription(env, sub.id, { last_triggered: nowIso(), failure_count: 0 })
      }
    } catch {
      await updateWebhookSubscription(env, sub.id, { failure_count: sub.failure_count + 1 })
    }
  }
}

// ===== Snooze =====

const SNOOZE_KEY = 'incident:snoozes'

export async function listSnoozes(
  env: Env,
  incidentId?: string,
  status?: string
): Promise<import('../types').IncidentSnooze[]> {
  const data = await env.KV.get(SNOOZE_KEY)
  let snoozes: import('../types').IncidentSnooze[] = data ? JSON.parse(data) : []

  if (incidentId) {
    snoozes = snoozes.filter(s => s.incident_id === incidentId)
  }
  if (status) {
    snoozes = snoozes.filter(s => s.status === status)
  }

  return snoozes
}

export async function createSnooze(
  env: Env,
  incidentId: string,
  principal: AuthPrincipal,
  wakeAt: string,
  reason: string
): Promise<import('../types').IncidentSnooze> {
  const snoozes = await listSnoozes(env)

  // Cancel any existing active snooze
  for (const snooze of snoozes) {
    if (snooze.incident_id === incidentId && snooze.status === 'active') {
      snooze.status = 'cancelled'
    }
  }

  const newSnooze: import('../types').IncidentSnooze = {
    id: `snooze-${nanoid(8)}`,
    incident_id: incidentId,
    snoozed_by: principal.sub,
    snoozed_at: nowIso(),
    wake_at: wakeAt,
    reason,
    auto_wake: true,
    status: 'active',
  }

  snoozes.push(newSnooze)
  await env.KV.put(SNOOZE_KEY, JSON.stringify(snoozes), { expirationTtl: 86400 * 365 })

  return newSnooze
}

export async function wakeSnooze(
  env: Env,
  snoozeId: string,
  principal: AuthPrincipal
): Promise<import('../types').IncidentSnooze | null> {
  const snoozes = await listSnoozes(env)
  const index = snoozes.findIndex(s => s.id === snoozeId)

  if (index === -1 || snoozes[index].status !== 'active') return null

  snoozes[index].status = 'woke'
  snoozes[index].woke_at = nowIso()
  snoozes[index].woke_by = principal.sub

  await env.KV.put(SNOOZE_KEY, JSON.stringify(snoozes), { expirationTtl: 86400 * 365 })
  return snoozes[index]
}

// ===== Merge =====

const MERGE_KEY = 'incident:merges'

export async function listMerges(
  env: Env,
  primaryIncidentId?: string
): Promise<import('../types').IncidentMerge[]> {
  const data = await env.KV.get(MERGE_KEY)
  let merges: import('../types').IncidentMerge[] = data ? JSON.parse(data) : []

  if (primaryIncidentId) {
    merges = merges.filter(m => m.primary_incident_id === primaryIncidentId)
  }

  return merges
}

export async function createMerge(
  env: Env,
  primaryIncidentId: string,
  mergedIncidentIds: string[],
  principal: AuthPrincipal,
  reason: string,
  preserveSubIncidents: boolean = false
): Promise<import('../types').IncidentMerge> {
  const merges = await listMerges(env)

  const newMerge: import('../types').IncidentMerge = {
    id: `merge-${nanoid(8)}`,
    primary_incident_id: primaryIncidentId,
    merged_incident_ids: mergedIncidentIds,
    merged_by: principal.sub,
    merged_at: nowIso(),
    merge_reason: reason,
    preserve_sub_incidents: preserveSubIncidents,
    notify_subscribers: true,
  }

  // Update merged incidents to reference primary
  for (const incidentId of mergedIncidentIds) {
    await updateIncident(env, incidentId, { status: 'resolved' })
  }

  merges.push(newMerge)
  await env.KV.put(MERGE_KEY, JSON.stringify(merges), { expirationTtl: 86400 * 365 })

  return newMerge
}

// ===== Split =====

const SPLIT_KEY = 'incident:splits'

export async function listSplits(
  env: Env,
  sourceIncidentId?: string
): Promise<import('../types').IncidentSplit[]> {
  const data = await env.KV.get(SPLIT_KEY)
  let splits: import('../types').IncidentSplit[] = data ? JSON.parse(data) : []

  if (sourceIncidentId) {
    splits = splits.filter(s => s.source_incident_id === sourceIncidentId)
  }

  return splits
}

export async function createSplit(
  env: Env,
  sourceIncidentId: string,
  newIncidents: Array<{
    title: string
    description?: string
    severity: IncidentSeverity
    evidence_ids?: string[]
  }>,
  principal: AuthPrincipal,
  reason: string
): Promise<import('../types').IncidentSplit> {
  const splits = await listSplits(env)
  const sourceIncident = await getIncident(env, sourceIncidentId)

  if (!sourceIncident) {
    throw new Error('Source incident not found')
  }

  const createdIncidents: Array<{ id: string; title: string; description?: string; severity: IncidentSeverity; evidence_ids?: string[] }> = []

  for (const newIncident of newIncidents) {
    const created = await createIncident(env, principal, {
      title: newIncident.title,
      summary: newIncident.description || sourceIncident.summary,
      severity: newIncident.severity,
      source: sourceIncident.source,
    })
    createdIncidents.push({
      id: created.id,
      title: newIncident.title,
      description: newIncident.description,
      severity: newIncident.severity,
      evidence_ids: newIncident.evidence_ids,
    })
  }

  const newSplit: import('../types').IncidentSplit = {
    id: `split-${nanoid(8)}`,
    source_incident_id: sourceIncidentId,
    new_incidents: createdIncidents,
    split_by: principal.sub,
    split_at: nowIso(),
    split_reason: reason,
  }

  splits.push(newSplit)
  await env.KV.put(SPLIT_KEY, JSON.stringify(splits), { expirationTtl: 86400 * 365 })

  return newSplit
}

// ===== Recurrence =====

const RECURRENCE_KEY = 'incident:recurrences'

export async function listRecurrences(
  env: Env,
  incidentId?: string
): Promise<import('../types').IncidentRecurrence[]> {
  const data = await env.KV.get(RECURRENCE_KEY)
  let recurrences: import('../types').IncidentRecurrence[] = data ? JSON.parse(data) : []

  if (incidentId) {
    recurrences = recurrences.filter(r => r.incident_id === incidentId || r.linked_incidents.includes(incidentId))
  }

  return recurrences
}

export async function detectRecurrence(
  env: Env,
  incident: IncidentRecord
): Promise<import('../types').IncidentRecurrence | null> {
  // Look for similar incidents in the past
  const allIncidentsResult = await listIncidents(env)
  const similarIncidents = allIncidentsResult.items.filter(i =>
    i.id !== incident.id &&
    i.title === incident.title &&
    i.source === incident.source &&
    i.status === 'resolved'
  ).sort((a, b) => b.created_at.localeCompare(a.created_at))

  if (similarIncidents.length === 0) return null

  const previousIncident = similarIncidents[0]
  const recurrences = await listRecurrences(env)
  const existingRecurrence = recurrences.find(r => r.incident_id === previousIncident.id)

  if (existingRecurrence) {
    // Update existing recurrence chain
    existingRecurrence.recurrence_count++
    existingRecurrence.last_occurred_at = incident.created_at
    existingRecurrence.linked_incidents.push(incident.id)
    await env.KV.put(RECURRENCE_KEY, JSON.stringify(recurrences), { expirationTtl: 86400 * 365 })

    // Create recurrence record for new incident
    const newRecurrence: import('../types').IncidentRecurrence = {
      id: `recur-${nanoid(8)}`,
      incident_id: incident.id,
      previous_incident_id: previousIncident.id,
      recurrence_count: existingRecurrence.recurrence_count,
      first_occurred_at: existingRecurrence.first_occurred_at,
      last_occurred_at: incident.created_at,
      pattern_detected: true,
      root_cause_resolved: false,
      linked_incidents: existingRecurrence.linked_incidents,
    }
    recurrences.push(newRecurrence)
    await env.KV.put(RECURRENCE_KEY, JSON.stringify(recurrences), { expirationTtl: 86400 * 365 })
    return newRecurrence
  }

  // Create new recurrence chain
  const newRecurrence: import('../types').IncidentRecurrence = {
    id: `recur-${nanoid(8)}`,
    incident_id: incident.id,
    previous_incident_id: previousIncident.id,
    recurrence_count: 1,
    first_occurred_at: previousIncident.created_at,
    last_occurred_at: incident.created_at,
    pattern_detected: true,
    root_cause_resolved: false,
    linked_incidents: [previousIncident.id, incident.id],
  }

  recurrences.push(newRecurrence)
  await env.KV.put(RECURRENCE_KEY, JSON.stringify(recurrences), { expirationTtl: 86400 * 365 })
  return newRecurrence
}

export async function markRootCauseResolved(
  env: Env,
  recurrenceId: string
): Promise<import('../types').IncidentRecurrence | null> {
  const recurrences = await listRecurrences(env)
  const index = recurrences.findIndex(r => r.id === recurrenceId)

  if (index === -1) return null

  recurrences[index].root_cause_resolved = true
  await env.KV.put(RECURRENCE_KEY, JSON.stringify(recurrences), { expirationTtl: 86400 * 365 })
  return recurrences[index]
}
