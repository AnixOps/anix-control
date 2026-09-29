import type { Context } from 'hono'
import { z } from 'zod'
import type { ApiErrorResponse, ApiMessageResponse, ApiSuccessResponse, Env, IncidentEvidence, IncidentLink, IncidentRecord, IncidentTimelineEventType, IncidentSeverity, IncidentStatus, SchemaValidationErrorResponse } from '../types'
import { logAudit } from '../utils/audit'
import {
  addIncidentComment,
  addIncidentEvidence,
  addIncidentLink,
  addIncidentTags,
  acknowledgeIncident,
  analyzeIncident,
  approveIncident,
  assignIncident,
  bulkAnalyzeIncidents,
  bulkApproveIncidents,
  bulkDeleteIncidents,
  bulkExecuteIncidents,
  buildIncidentTimeline,
  canApproveIncident,
  canExecuteIncident,
  createAutomationRule,
  createIncident,
  createIncidentFromTemplate,
  createIncidentTemplate,
  createNotificationRule,
  createPostMortem,
  createSuppressionRule,
  deleteAutomationRule,
  deleteIncidentComment,
  deleteIncidentTemplate,
  deleteNotificationRule,
  deleteSuppressionRule,
  escalateIncident,
  executeIncident,
  executeRunbookForIncident,
  generateIncidentReport,
  getIncident,
  getIncidentActivityLog,
  getIncidentComment,
  getIncidentSlaStatus,
  getIncidentStatistics,
  getIncidentTemplate,
  getPostMortem,
  listAllTags,
  listAssignedIncidents,
  listAutomationRules,
  listIncidentComments,
  listIncidentTemplates,
  listIncidents,
  listNotificationRules,
  listSuppressionRules,
  mergeIncidents,
  removeIncidentLink,
  removeIncidentTags,
  searchIncidents,
  setIncidentTags,
  suggestRunbooks,
  toggleAutomationRule,
  toggleNotificationRule,
  toggleSuppressionRule,
  toIncidentDetail,
  toIncidentSummary,
  unassignIncident,
  updateActionItemStatus,
  updateIncidentComment,
  updateIncidentTemplate,
  updatePostMortem,
  // New functions
  getIncidentDashboardMetrics,
  findRelatedIncidents,
  watchIncident,
  unwatchIncident,
  getIncidentWatchers,
  createExternalTicket,
  getExternalTickets,
  updateExternalTicketStatus,
  // Advanced features
  listResponsePlaybooks,
  getResponsePlaybook,
  createResponsePlaybook,
  updateResponsePlaybook,
  deleteResponsePlaybook,
  matchResponsePlaybooks,
  startPlaybookExecution,
  getPlaybookExecution,
  completePlaybookStep,
  skipPlaybookStep,
  listCustomFieldDefinitions,
  getCustomFieldDefinition,
  createCustomFieldDefinition,
  updateCustomFieldDefinition,
  deleteCustomFieldDefinition,
  setIncidentCustomField,
  getIncidentCustomFields,
  generateAIRootCauseAnalysis,
  createWarRoom,
  getWarRoom,
  joinWarRoom,
  leaveWarRoom,
  addWarRoomMessage,
  addWarRoomResource,
  closeWarRoom,
  exportIncidents,
  getExportResult,
  getExportDownload,
  // Reviews
  createIncidentReview,
  getIncidentReviews,
  completeIncidentReview,
  // Analytics
  calculateResponseAnalytics,
  // Feedback
  submitIncidentFeedback,
  getIncidentFeedback,
  // Cost
  calculateIncidentCost,
  getIncidentCost,
  // Compliance
  createComplianceRecord,
  getComplianceRecord,
  updateComplianceRequirement,
  // On-call
  listOnCallSchedules,
  getOnCallSchedule,
  createOnCallSchedule,
  getCurrentOnCall,
  // Checklists
  getIncidentChecklists,
  createIncidentChecklist,
  updateChecklistItem,
  // Change links
  linkIncidentToChange,
  getIncidentChanges,
  // Run history
  getIncidentRunHistory,
  // Responder teams
  listResponderTeams,
  getResponderTeam,
  createResponderTeam,
  updateResponderTeam,
  deleteResponderTeam,
  // SLA calendars
  listSLACalendars,
  getSLACalendar,
  createSLACalendar,
  // Notification templates
  listNotificationTemplates,
  getNotificationTemplate,
  createNotificationTemplate,
  // Escalation rules
  listEscalationRules,
  createEscalationRule,
  // Attachments
  listIncidentAttachments,
  uploadIncidentAttachment,
  downloadIncidentAttachment,
  deleteIncidentAttachment,
  // Related items
  listRelatedItems,
  addRelatedItem,
  removeRelatedItem,
  // Response targets
  listResponseTimeTargets,
  createResponseTimeTarget,
  // Integrations
  listIntegrations,
  createIntegration,
  updateIntegration,
  deleteIntegration,
  // Timeline events
  listTimelineEvents,
  addTimelineEvent,
  // Runbooks
  listRunbooks,
  getRunbook,
  createRunbook,
  updateRunbook,
  deleteRunbook,
  // Auto-remediation
  listAutoRemediationRules,
  createAutoRemediationRule,
  updateAutoRemediationRule,
  deleteAutoRemediationRule,
  // Maintenance windows
  listMaintenanceWindows,
  getMaintenanceWindow,
  createMaintenanceWindow,
  updateMaintenanceWindow,
  cancelMaintenanceWindow,
  // Bulk operations
  listBulkOperations,
  getBulkOperation,
  createBulkOperation,
  executeBulkOperation,
  // SLA breaches
  listSLABreaches,
  createSLABreach,
  acknowledgeSLABreach,
  // Analytics
  listAnalyticsSnapshots,
  generateAnalyticsSnapshot,
  // Webhook subscriptions
  listWebhookSubscriptions,
  getWebhookSubscription,
  createWebhookSubscription,
  updateWebhookSubscription,
  deleteWebhookSubscription,
  // Snooze
  listSnoozes,
  createSnooze,
  wakeSnooze,
  // Merge
  listMerges,
  createMerge,
  // Split
  listSplits,
  createSplit,
  // Recurrence
  listRecurrences,
  detectRecurrence,
  markRootCauseResolved,
} from '../services/incidents'

const createIncidentSchema = z.object({
  title: z.string().min(1),
  source: z.string().min(1),
  summary: z.string().optional(),
  severity: z.enum(['low', 'medium', 'high', 'critical']).optional(),
  correlation_id: z.string().optional(),
  action_type: z.enum(['scale_policy', 'restart_deployment', 'scale_deployment']).optional(),
  action_ref: z.string().optional(),
  tags: z.array(z.string().max(50)).max(10).optional(),
  evidence: z.array(z.object({
    type: z.enum(['log', 'metric', 'task', 'node', 'alert', 'service', 'manual']),
    source: z.string().min(1),
    content: z.string().min(1),
  })).optional(),
})

const listIncidentQuerySchema = z.object({
  status: z.enum(['open', 'analyzed', 'approved', 'executing', 'resolved', 'failed']).optional(),
  severity: z.enum(['low', 'medium', 'high', 'critical']).optional(),
  action_type: z.enum(['scale_policy', 'restart_deployment', 'scale_deployment']).optional(),
  source: z.string().optional(),
  requested_via: z.enum(['jwt', 'api_key']).optional(),
  approved_by: z.coerce.number().int().optional(),
  correlation_id: z.string().optional(),
  has_action: z.enum(['true', 'false']).optional(),
  page: z.coerce.number().int().min(1).default(1),
  per_page: z.coerce.number().int().min(1).max(100).default(20),
  sort: z.enum(['created_at', 'updated_at', 'severity', 'status']).default('created_at'),
  order: z.enum(['asc', 'desc']).default('desc'),
})

async function requireIncident(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const incident = await getIncident(c.env, incidentId)

  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  return incident
}

export async function listIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const rawQuery = listIncidentQuerySchema.parse(Object.fromEntries(new URL(c.req.url).searchParams))
  const query = {
    ...rawQuery,
    has_action: rawQuery.has_action === undefined ? undefined : rawQuery.has_action === 'true',
  }
  const incidents = await listIncidents(c.env, query)

  return c.json({
    success: true,
    data: {
      items: incidents.items.map(toIncidentSummary),
      total: incidents.total,
      page: incidents.page,
      per_page: incidents.per_page,
      total_pages: incidents.total_pages,
    },
  })
}

export async function getIncidentHandler(c: Context<{ Bindings: Env }>) {
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  return c.json({ success: true, data: toIncidentDetail(incident) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
}

export async function createIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createIncidentSchema.parse(await c.req.json())
    const incident = await createIncident(c.env, principal, body)

    await logAudit(c, principal.sub, 'create_incident', 'incident', {
      incident_id: incident.id,
      severity: incident.severity,
      source: incident.source,
      action_type: incident.action_type,
      action_ref: incident.action_ref,
    })

    return c.json({ success: true, data: toIncidentDetail(incident) }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function analyzeIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const updated = await analyzeIncident(c.env, incident)

  await logAudit(c, principal.sub, 'analyze_incident', 'incident', {
    incident_id: updated.id,
    status: updated.status,
  })

  return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
}

export async function approveIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  if (!await canApproveIncident(c.env, principal, incident)) {
    return c.json({ success: false, error: 'Forbidden: approval policy denies this action' } as ApiErrorResponse, 403)
  }

  const updated = await approveIncident(c.env, incident, principal)

  await logAudit(c, principal.sub, 'approve_incident', 'incident', {
    incident_id: updated.id,
    approved_at: updated.approved_at,
  })

  return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
}

export async function executeIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  if (!await canExecuteIncident(c.env, principal, incident)) {
    return c.json({ success: false, error: 'Forbidden: execution policy denies this action' } as ApiErrorResponse, 403)
  }

  try {
    const updated = await executeIncident(c.env, incident)

    await logAudit(c, principal.sub, 'execute_incident', 'incident', {
      incident_id: updated.id,
      status: updated.status,
      execution_id: updated.execution_id,
      action_type: updated.action_type,
      action_ref: updated.action_ref,
    }, updated.status === 'resolved' ? 'success' : 'failure')

    return c.json({ success: true, data: toIncidentDetail(updated) }, updated.status === 'resolved' ? 200 : 400)
  } catch (err) {
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Execution failed' }, 400)
  }
}

export async function acknowledgeIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const updated = await acknowledgeIncident(c.env, incident, principal)

    await logAudit(c, principal.sub, 'acknowledge_incident', 'incident', {
      incident_id: updated.id,
      acknowledged_at: updated.acknowledged_at,
    })

    return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
  } catch (err) {
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Acknowledgment failed' }, 400)
  }
}

const escalateSchema = z.object({
  severity: z.enum(['low', 'medium', 'high', 'critical']),
})

export async function escalateIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = escalateSchema.parse(await c.req.json())
    const updated = await escalateIncident(c.env, incident, body.severity)

    await logAudit(c, principal.sub, 'escalate_incident', 'incident', {
      incident_id: updated.id,
      escalated_from: incident.severity,
      escalated_to: updated.severity,
      escalated_at: updated.escalated_at,
    })

    return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Escalation failed' }, 400)
  }
}

const assignSchema = z.object({
  assignee_id: z.number().int().positive(),
  assignee_email: z.string().email().optional(),
})

export async function assignIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = assignSchema.parse(await c.req.json())
    const updated = await assignIncident(c.env, incident, body.assignee_id, body.assignee_email)

    await logAudit(c, principal.sub, 'assign_incident', 'incident', {
      incident_id: updated.id,
      assignee_id: body.assignee_id,
      assignee_email: body.assignee_email,
      assigned_at: updated.assigned_at,
    })

    return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Assignment failed' }, 400)
  }
}

export async function unassignIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const updated = await unassignIncident(c.env, incident)

  await logAudit(c, principal.sub, 'unassign_incident', 'incident', {
    incident_id: updated.id,
  })

  return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
}

export async function getIncidentSlaStatusHandler(c: Context<{ Bindings: Env }>) {
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const slaStatus = await getIncidentSlaStatus(c.env, incident)
  return c.json({ success: true, data: slaStatus } as ApiSuccessResponse<Awaited<ReturnType<typeof getIncidentSlaStatus>>>)
}

export async function getIncidentTimelineHandler(c: Context<{ Bindings: Env }>) {
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const timeline = buildIncidentTimeline(incident)
  return c.json({ success: true, data: timeline } as ApiSuccessResponse<ReturnType<typeof buildIncidentTimeline>>)
}

const createCommentSchema = z.object({
  content: z.string().min(1).max(5000),
  visibility: z.enum(['public', 'internal']).optional(),
})

const updateCommentSchema = z.object({
  content: z.string().min(1).max(5000),
})

export async function listIncidentCommentsHandler(c: Context<{ Bindings: Env }>) {
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const comments = await listIncidentComments(c.env, incident.id)
  return c.json({ success: true, data: comments } as ApiSuccessResponse<Awaited<ReturnType<typeof listIncidentComments>>>)
}

export async function addIncidentCommentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = createCommentSchema.parse(await c.req.json())
    const comment = await addIncidentComment(c.env, incident, principal, body)

    await logAudit(c, principal.sub, 'add_incident_comment', 'incident', {
      incident_id: incident.id,
      comment_id: comment.id,
      visibility: comment.visibility,
    })

    return c.json({ success: true, data: comment } as ApiSuccessResponse<Awaited<ReturnType<typeof addIncidentComment>>>, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function updateIncidentCommentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const commentId = c.req.param('commentId') as string

  try {
    const body = updateCommentSchema.parse(await c.req.json())
    const updated = await updateIncidentComment(c.env, commentId, principal, body.content)

    if (!updated) {
      return c.json({ success: false, error: 'Comment not found or not authorized' } as ApiErrorResponse, 404)
    }

    await logAudit(c, principal.sub, 'update_incident_comment', 'incident', {
      incident_id: updated.incident_id,
      comment_id: updated.id,
    })

    return c.json({ success: true, data: updated } as ApiSuccessResponse<Awaited<ReturnType<typeof updateIncidentComment>>>)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function deleteIncidentCommentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const commentId = c.req.param('commentId') as string

  const deleted = await deleteIncidentComment(c.env, commentId, principal)

  if (!deleted) {
    return c.json({ success: false, error: 'Comment not found or not authorized' } as ApiErrorResponse, 404)
  }

  await logAudit(c, principal.sub, 'delete_incident_comment', 'incident', {
    comment_id: commentId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// Tag Management
const tagsSchema = z.object({
  tags: z.array(z.string().max(50)).min(1).max(10),
})

export async function listTagsHandler(c: Context<{ Bindings: Env }>) {
  const tags = await listAllTags(c.env)
  return c.json({ success: true, data: tags })
}

export async function addTagsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = tagsSchema.parse(await c.req.json())
    const updated = await addIncidentTags(c.env, incident, body.tags)

    await logAudit(c, principal.sub, 'add_incident_tags', 'incident', {
      incident_id: incident.id,
      tags: body.tags,
    })

    return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function removeTagsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = tagsSchema.parse(await c.req.json())
    const updated = await removeIncidentTags(c.env, incident, body.tags)

    await logAudit(c, principal.sub, 'remove_incident_tags', 'incident', {
      incident_id: incident.id,
      tags: body.tags,
    })

    return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function setTagsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = tagsSchema.parse(await c.req.json())
    const updated = await setIncidentTags(c.env, incident, body.tags)

    await logAudit(c, principal.sub, 'set_incident_tags', 'incident', {
      incident_id: incident.id,
      tags: body.tags,
    })

    return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

const statisticsQuerySchema = z.object({
  since: z.string().optional(),
  until: z.string().optional(),
})

export async function getIncidentStatisticsHandler(c: Context<{ Bindings: Env }>) {
  const rawQuery = Object.fromEntries(new URL(c.req.url).searchParams)
  const query = statisticsQuerySchema.parse(rawQuery)

  const stats = await getIncidentStatistics(c.env, query)
  return c.json({ success: true, data: stats })
}

const searchQuerySchema = z.object({
  q: z.string().min(2),
  status: z.string().optional(),
  severity: z.string().optional(),
  action_type: z.string().optional(),
  source: z.string().optional(),
  created_after: z.string().optional(),
  created_before: z.string().optional(),
  page: z.coerce.number().int().min(1).default(1),
  per_page: z.coerce.number().int().min(1).max(100).default(20),
})

export async function searchIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const rawQuery = Object.fromEntries(new URL(c.req.url).searchParams)
  const query = searchQuerySchema.parse(rawQuery)

  const searchParams: Parameters<typeof searchIncidents>[1] = {
    query: query.q,
    page: query.page,
    per_page: query.per_page,
  }

  if (query.status) {
    searchParams.status = query.status.split(',').filter(s =>
      ['open', 'analyzed', 'approved', 'executing', 'resolved', 'failed'].includes(s)
    ) as typeof searchParams.status
  }

  if (query.severity) {
    searchParams.severity = query.severity.split(',').filter(s =>
      ['low', 'medium', 'high', 'critical'].includes(s)
    ) as typeof searchParams.severity
  }

  if (query.action_type) {
    searchParams.action_type = query.action_type.split(',').filter(s =>
      ['scale_policy', 'restart_deployment', 'scale_deployment'].includes(s)
    ) as typeof searchParams.action_type
  }

  if (query.source) {
    searchParams.source = query.source.split(',')
  }

  if (query.created_after) {
    searchParams.created_after = query.created_after
  }

  if (query.created_before) {
    searchParams.created_before = query.created_before
  }

  const results = await searchIncidents(c.env, searchParams)

  return c.json({
    success: true,
    data: {
      items: results.items.map(item => ({
        id: item.id,
        title: item.title,
        summary: item.summary,
        status: item.status,
        severity: item.severity,
        source: item.source,
        action_type: item.action_type,
        action_ref: item.action_ref,
        correlation_id: item.correlation_id,
        search_score: item.search_score,
        search_highlights: item.search_highlights,
        created_at: item.created_at,
        updated_at: item.updated_at,
      })),
      total: results.total,
      page: results.page,
      per_page: results.per_page,
      total_pages: results.total_pages,
      query: results.query,
    },
  })
}

const bulkOperationSchema = z.object({
  incident_ids: z.array(z.string().uuid()).min(1).max(50),
})

export async function bulkApproveIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = bulkOperationSchema.parse(await c.req.json())
    const result = await bulkApproveIncidents(c.env, principal, body.incident_ids)

    await logAudit(c, principal.sub, 'bulk_approve_incidents', 'incident', {
      total: result.total,
      successful: result.successful.length,
      failed: result.failed.length,
    })

    return c.json({ success: true, data: result })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function bulkExecuteIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = bulkOperationSchema.parse(await c.req.json())
    const result = await bulkExecuteIncidents(c.env, principal, body.incident_ids)

    await logAudit(c, principal.sub, 'bulk_execute_incidents', 'incident', {
      total: result.total,
      successful: result.successful.length,
      failed: result.failed.length,
    })

    return c.json({ success: true, data: result })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function bulkAnalyzeIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = bulkOperationSchema.parse(await c.req.json())
    const result = await bulkAnalyzeIncidents(c.env, body.incident_ids)

    await logAudit(c, principal.sub, 'bulk_analyze_incidents', 'incident', {
      total: result.total,
      successful: result.successful.length,
      failed: result.failed.length,
    })

    return c.json({ success: true, data: result })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function bulkDeleteIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = bulkOperationSchema.parse(await c.req.json())
    const result = await bulkDeleteIncidents(c.env, body.incident_ids)

    await logAudit(c, principal.sub, 'bulk_delete_incidents', 'incident', {
      total: result.total,
      successful: result.successful.length,
      failed: result.failed.length,
    })

    return c.json({ success: true, data: result })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// Suppression Rules
const createSuppressionRuleSchema = z.object({
  name: z.string().min(1).max(100),
  description: z.string().max(500).optional(),
  conditions: z.object({
    severity: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
    source: z.array(z.string()).optional(),
    action_type: z.array(z.enum(['scale_policy', 'restart_deployment', 'scale_deployment'])).optional(),
    title_pattern: z.string().optional(),
    correlation_id_pattern: z.string().optional(),
  }),
  duration_minutes: z.number().int().min(1).max(10080), // Max 1 week
})

export async function listSuppressionRulesHandler(c: Context<{ Bindings: Env }>) {
  const rules = await listSuppressionRules(c.env)
  return c.json({ success: true, data: rules })
}

export async function createSuppressionRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createSuppressionRuleSchema.parse(await c.req.json())
    const rule = await createSuppressionRule(c.env, principal, {
      name: body.name,
      description: body.description,
      conditions: body.conditions,
      duration_minutes: body.duration_minutes,
    })

    await logAudit(c, principal.sub, 'create_suppression_rule', 'incident', {
      rule_id: rule.id,
      rule_name: rule.name,
      duration_minutes: rule.duration_minutes,
    })

    return c.json({ success: true, data: rule }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function deleteSuppressionRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  const deleted = await deleteSuppressionRule(c.env, ruleId)

  if (!deleted) {
    return c.json({ success: false, error: 'Suppression rule not found' } as ApiErrorResponse, 404)
  }

  await logAudit(c, principal.sub, 'delete_suppression_rule', 'incident', {
    rule_id: ruleId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

export async function toggleSuppressionRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  try {
    const body = z.object({ enabled: z.boolean() }).parse(await c.req.json())
    const rule = await toggleSuppressionRule(c.env, ruleId, body.enabled)

    if (!rule) {
      return c.json({ success: false, error: 'Suppression rule not found' } as ApiErrorResponse, 404)
    }

    await logAudit(c, principal.sub, 'toggle_suppression_rule', 'incident', {
      rule_id: ruleId,
      enabled: body.enabled,
    })

    return c.json({ success: true, data: rule })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// Incident Merging
const mergeSchema = z.object({
  primary_id: z.string().uuid(),
  incident_ids: z.array(z.string().uuid()).min(1).max(10),
})

export async function mergeIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = mergeSchema.parse(await c.req.json())

    // Ensure primary is in the list
    if (!body.incident_ids.includes(body.primary_id)) {
      body.incident_ids.push(body.primary_id)
    }

    const result = await mergeIncidents(c.env, body.primary_id, body.incident_ids)

    await logAudit(c, principal.sub, 'merge_incidents', 'incident', {
      primary_id: body.primary_id,
      merged_ids: result.merged.map(i => i.id),
      merged_count: result.merged_count,
    })

    return c.json({
      success: true,
      data: {
        primary: toIncidentDetail(result.primary),
        merged_count: result.merged_count,
        merged_ids: result.merged.map(i => i.id),
      },
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Merge failed' }, 400)
  }
}

// Notification Rules
const createNotificationRuleSchema = z.object({
  name: z.string().min(1).max(100),
  description: z.string().max(500).optional(),
  conditions: z.object({
    severity: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
    source: z.array(z.string()).optional(),
    action_type: z.array(z.enum(['scale_policy', 'restart_deployment', 'scale_deployment'])).optional(),
    status: z.array(z.enum(['open', 'analyzed', 'approved', 'executing', 'resolved', 'failed'])).optional(),
    tags: z.array(z.string()).optional(),
  }),
  channels: z.array(z.enum(['email', 'webhook', 'slack'])).min(1),
  recipients: z.array(z.string()).min(1),
  template: z.string().optional(),
})

export async function listNotificationRulesHandler(c: Context<{ Bindings: Env }>) {
  const rules = await listNotificationRules(c.env)
  return c.json({ success: true, data: rules })
}

export async function createNotificationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createNotificationRuleSchema.parse(await c.req.json())
    const rule = await createNotificationRule(c.env, principal, {
      name: body.name,
      description: body.description,
      conditions: body.conditions,
      channels: body.channels,
      recipients: body.recipients,
      template: body.template,
    })

    await logAudit(c, principal.sub, 'create_notification_rule', 'incident', {
      rule_id: rule.id,
      rule_name: rule.name,
      channels: rule.channels,
    })

    return c.json({ success: true, data: rule }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function deleteNotificationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  const deleted = await deleteNotificationRule(c.env, ruleId)

  if (!deleted) {
    return c.json({ success: false, error: 'Notification rule not found' } as ApiErrorResponse, 404)
  }

  await logAudit(c, principal.sub, 'delete_notification_rule', 'incident', {
    rule_id: ruleId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

export async function toggleNotificationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  try {
    const body = z.object({ enabled: z.boolean() }).parse(await c.req.json())
    const rule = await toggleNotificationRule(c.env, ruleId, body.enabled)

    if (!rule) {
      return c.json({ success: false, error: 'Notification rule not found' } as ApiErrorResponse, 404)
    }

    await logAudit(c, principal.sub, 'toggle_notification_rule', 'incident', {
      rule_id: ruleId,
      enabled: body.enabled,
    })

    return c.json({ success: true, data: rule })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// Reports
const reportQuerySchema = z.object({
  start_date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
  end_date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
})

export async function getIncidentReportHandler(c: Context<{ Bindings: Env }>) {
  const rawQuery = Object.fromEntries(new URL(c.req.url).searchParams)
  const query = reportQuerySchema.parse(rawQuery)

  const report = await generateIncidentReport(c.env, query.start_date, query.end_date)
  return c.json({ success: true, data: report })
}

async function handleIncidentMutation<TBody>(
  c: Context<{ Bindings: Env }>,
  action: string,
  bodyParser: () => Promise<TBody>,
  mutate: (env: Env, incident: IncidentRecord, body: TBody) => Promise<IncidentRecord>,
  auditDetails: (incident: IncidentRecord, body: TBody) => Record<string, unknown>,
) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = await bodyParser()
    const updated = await mutate(c.env, incident, body)

    await logAudit(c, principal.sub, action, 'incident', auditDetails(incident, body))

    return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// Link Management
const linkSchema = z.object({
  kind: z.enum(['task', 'node', 'scaling_policy', 'deployment', 'runbook', 'alert', 'playbook']),
  id: z.string().min(1),
  name: z.string().optional(),
  href: z.string().optional(),
  relationship: z.enum(['caused_by', 'related_to', 'resolves', 'investigates']).optional(),
})

export async function addIncidentLinkHandler(c: Context<{ Bindings: Env }>) {
  return handleIncidentMutation(
    c,
    'add_incident_link',
    async () => linkSchema.parse(await c.req.json()) as IncidentLink,
    (env, incident, body) => addIncidentLink(env, incident, body),
    (incident, body) => ({
      incident_id: incident.id,
      link_kind: body.kind,
      link_id: body.id,
    }),
  )
}

export async function removeIncidentLinkHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const linkKind = c.req.param('linkKind') as IncidentLink['kind']
  const linkId = c.req.param('linkId') as string

  const updated = await removeIncidentLink(c.env, incident, linkKind, linkId)

  await logAudit(c, principal.sub, 'remove_incident_link', 'incident', {
    incident_id: incident.id,
    link_kind: linkKind,
    link_id: linkId,
  })

  return c.json({ success: true, data: toIncidentDetail(updated) } as ApiSuccessResponse<ReturnType<typeof toIncidentDetail>>)
}

// Evidence Management
const evidenceSchema = z.object({
  type: z.enum(['log', 'metric', 'task', 'node', 'alert', 'service', 'manual']),
  source: z.string().min(1),
  content: z.string().min(1),
})

export async function addIncidentEvidenceHandler(c: Context<{ Bindings: Env }>) {
  return handleIncidentMutation(
    c,
    'add_incident_evidence',
    async () => evidenceSchema.parse(await c.req.json()) as IncidentEvidence,
    (env, incident, body) => addIncidentEvidence(env, incident, body),
    (incident, body) => ({
      incident_id: incident.id,
      evidence_type: body.type,
      evidence_source: body.source,
    }),
  )
}

// Activity Log
export async function getIncidentActivityLogHandler(c: Context<{ Bindings: Env }>) {
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const logs = await getIncidentActivityLog(c.env, incident.id)
  return c.json({ success: true, data: logs })
}

// Runbook Suggestions
export async function getRunbookSuggestionsHandler(c: Context<{ Bindings: Env }>) {
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  const suggestions = await suggestRunbooks(c.env, incident)
  return c.json({ success: true, data: suggestions })
}

export async function executeRunbookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incident = await requireIncident(c)
  if (incident instanceof Response) {
    return incident
  }

  try {
    const body = z.object({ playbook_id: z.number().int().positive() }).parse(await c.req.json())
    const result = await executeRunbookForIncident(c.env, incident, body.playbook_id, principal)

    await logAudit(c, principal.sub, 'execute_runbook_for_incident', 'incident', {
      incident_id: incident.id,
      playbook_id: body.playbook_id,
      task_id: result.task_id,
    })

    return c.json({ success: true, data: result })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// ==================== Templates ====================

const createTemplateSchema = z.object({
  name: z.string().min(1).max(100),
  description: z.string().max(500).optional(),
  category: z.string().max(50).optional(),
  title_template: z.string().min(1).max(200),
  summary_template: z.string().max(1000).optional(),
  default_severity: z.enum(['low', 'medium', 'high', 'critical']),
  default_source: z.string().min(1),
  default_action_type: z.enum(['scale_policy', 'restart_deployment', 'scale_deployment']).optional(),
  default_action_ref: z.string().optional(),
  default_tags: z.array(z.string().max(50)).default([]),
  evidence_templates: z.array(z.object({
    type: z.enum(['log', 'metric', 'task', 'node', 'alert', 'service', 'manual']),
    source_template: z.string(),
    content_template: z.string().optional(),
  })).default([]),
})

export async function listTemplatesHandler(c: Context<{ Bindings: Env }>) {
  const templates = await listIncidentTemplates(c.env)
  return c.json({ success: true, data: templates })
}

export async function getTemplateHandler(c: Context<{ Bindings: Env }>) {
  const templateId = c.req.param('templateId') as string
  const template = await getIncidentTemplate(c.env, templateId)

  if (!template) {
    return c.json({ success: false, error: 'Template not found' } as ApiErrorResponse, 404)
  }

  return c.json({ success: true, data: template })
}

export async function createTemplateHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createTemplateSchema.parse(await c.req.json())
    const template = await createIncidentTemplate(c.env, principal, {
      name: body.name,
      description: body.description,
      category: body.category,
      title_template: body.title_template,
      summary_template: body.summary_template,
      default_severity: body.default_severity,
      default_source: body.default_source,
      default_action_type: body.default_action_type,
      default_action_ref: body.default_action_ref,
      default_tags: body.default_tags,
      evidence_templates: body.evidence_templates,
    })

    await logAudit(c, principal.sub, 'create_incident_template', 'incident', {
      template_id: template.id,
      template_name: template.name,
    })

    return c.json({ success: true, data: template }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function updateTemplateHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const templateId = c.req.param('templateId') as string

  try {
    const body = createTemplateSchema.partial().parse(await c.req.json())
    const template = await updateIncidentTemplate(c.env, templateId, body)

    if (!template) {
      return c.json({ success: false, error: 'Template not found' } as ApiErrorResponse, 404)
    }

    await logAudit(c, principal.sub, 'update_incident_template', 'incident', {
      template_id: templateId,
    })

    return c.json({ success: true, data: template })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function deleteTemplateHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const templateId = c.req.param('templateId') as string

  const deleted = await deleteIncidentTemplate(c.env, templateId)

  if (!deleted) {
    return c.json({ success: false, error: 'Template not found' } as ApiErrorResponse, 404)
  }

  await logAudit(c, principal.sub, 'delete_incident_template', 'incident', {
    template_id: templateId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

export async function createFromTemplateHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const templateId = c.req.param('templateId') as string

  try {
    const body = z.object({ variables: z.record(z.string(), z.string()).optional() }).parse(await c.req.json())
    const incident = await createIncidentFromTemplate(c.env, principal, templateId, body.variables || {})

    await logAudit(c, principal.sub, 'create_incident_from_template', 'incident', {
      template_id: templateId,
      incident_id: incident.id,
    })

    return c.json({ success: true, data: toIncidentDetail(incident) }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to create incident' }, 400)
  }
}

// ==================== Automation Rules ====================

const createActionSchema = z.discriminatedUnion('type', [
  z.object({ type: z.literal('assign'), assignee_id: z.number().int().positive() }),
  z.object({ type: z.literal('escalate'), target_severity: z.enum(['low', 'medium', 'high', 'critical']) }),
  z.object({ type: z.literal('add_tags'), tags: z.array(z.string()).min(1) }),
  z.object({ type: z.literal('notify'), channels: z.array(z.string()).min(1), recipients: z.array(z.string()).min(1) }),
  z.object({ type: z.literal('execute_runbook'), playbook_id: z.number().int().positive() }),
  z.object({ type: z.literal('set_sla'), minutes: z.number().int().min(1).max(10080) }),
  z.object({ type: z.literal('add_comment'), content: z.string().min(1).max(5000) }),
])

const createAutomationRuleSchema = z.object({
  name: z.string().min(1).max(100),
  description: z.string().max(500).optional(),
  enabled: z.boolean().default(true),
  trigger: z.enum(['incident.created', 'incident.acknowledged', 'incident.escalated', 'incident.analyzed', 'incident.approved', 'incident.resolved', 'incident.failed', 'sla_breach', 'duplicate_detected']),
  conditions: z.object({
    severity: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
    source: z.array(z.string()).optional(),
    action_type: z.array(z.enum(['scale_policy', 'restart_deployment', 'scale_deployment'])).optional(),
    tags: z.array(z.string()).optional(),
    time_range: z.object({ start_hour: z.number().min(0).max(23), end_hour: z.number().min(1).max(24) }).optional(),
  }),
  actions: z.array(createActionSchema).min(1),
  priority: z.number().int().min(1).max(100).default(50),
})

export async function listAutomationRulesHandler(c: Context<{ Bindings: Env }>) {
  const rules = await listAutomationRules(c.env)
  return c.json({ success: true, data: rules })
}

export async function createAutomationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createAutomationRuleSchema.parse(await c.req.json())
    const rule = await createAutomationRule(c.env, principal, {
      name: body.name,
      description: body.description,
      enabled: body.enabled,
      trigger: body.trigger,
      conditions: body.conditions,
      actions: body.actions as import('../types').AutomationAction[],
      priority: body.priority,
    })

    await logAudit(c, principal.sub, 'create_automation_rule', 'incident', {
      rule_id: rule.id,
      rule_name: rule.name,
      trigger: rule.trigger,
    })

    return c.json({ success: true, data: rule }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function deleteAutomationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  const deleted = await deleteAutomationRule(c.env, ruleId)

  if (!deleted) {
    return c.json({ success: false, error: 'Automation rule not found' } as ApiErrorResponse, 404)
  }

  await logAudit(c, principal.sub, 'delete_automation_rule', 'incident', {
    rule_id: ruleId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

export async function toggleAutomationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  try {
    const body = z.object({ enabled: z.boolean() }).parse(await c.req.json())
    const rule = await toggleAutomationRule(c.env, ruleId, body.enabled)

    if (!rule) {
      return c.json({ success: false, error: 'Automation rule not found' } as ApiErrorResponse, 404)
    }

    await logAudit(c, principal.sub, 'toggle_automation_rule', 'incident', {
      rule_id: ruleId,
      enabled: body.enabled,
    })

    return c.json({ success: true, data: rule })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// ==================== Post-Mortems ====================

const createPostMortemSchema = z.object({
  title: z.string().min(1).max(200),
  summary: z.string().min(1).max(5000),
  timeline: z.array(z.object({
    timestamp: z.string(),
    event: z.string(),
    details: z.string().optional(),
  })),
  root_cause: z.string().min(1).max(5000),
  contributing_factors: z.array(z.string()),
  impact: z.object({
    users_affected: z.number().int().optional(),
    duration_minutes: z.number().int().min(1),
    services_affected: z.array(z.string()),
  }),
  resolution: z.string().min(1).max(5000),
  lessons_learned: z.array(z.string()),
  action_items: z.array(z.object({
    id: z.string(),
    description: z.string(),
    owner: z.string().optional(),
    status: z.enum(['open', 'in_progress', 'completed']),
    due_date: z.string().optional(),
  })),
})

export async function getPostMortemHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const postMortem = await getPostMortem(c.env, incidentId)

  if (!postMortem) {
    return c.json({ success: false, error: 'Post-mortem not found' } as ApiErrorResponse, 404)
  }

  return c.json({ success: true, data: postMortem })
}

export async function createPostMortemHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const body = createPostMortemSchema.parse(await c.req.json())
    const postMortem = await createPostMortem(c.env, principal, incidentId, {
      title: body.title,
      summary: body.summary,
      timeline: body.timeline,
      root_cause: body.root_cause,
      contributing_factors: body.contributing_factors,
      impact: body.impact,
      resolution: body.resolution,
      lessons_learned: body.lessons_learned,
      action_items: body.action_items,
    })

    await logAudit(c, principal.sub, 'create_postmortem', 'incident', {
      incident_id: incidentId,
      postmortem_id: postMortem.id,
      title: postMortem.title,
    })

    return c.json({ success: true, data: postMortem }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to create post-mortem' }, 400)
  }
}

export async function updatePostMortemHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const body = createPostMortemSchema.partial().parse(await c.req.json())
    const postMortem = await updatePostMortem(c.env, incidentId, body)

    if (!postMortem) {
      return c.json({ success: false, error: 'Post-mortem not found' } as ApiErrorResponse, 404)
    }

    await logAudit(c, principal.sub, 'update_postmortem', 'incident', {
      incident_id: incidentId,
    })

    return c.json({ success: true, data: postMortem })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function updateActionItemHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const actionItemId = c.req.param('actionItemId') as string

  try {
    const body = z.object({ status: z.enum(['open', 'in_progress', 'completed']) }).parse(await c.req.json())
    const postMortem = await updateActionItemStatus(c.env, incidentId, actionItemId, body.status)

    if (!postMortem) {
      return c.json({ success: false, error: 'Post-mortem or action item not found' }, 404)
    }

    await logAudit(c, principal.sub, 'update_action_item', 'incident', {
      incident_id: incidentId,
      action_item_id: actionItemId,
      status: body.status,
    })

    return c.json({ success: true, data: postMortem })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// ==================== Dashboard Metrics ====================

export async function getIncidentDashboardMetricsHandler(c: Context<{ Bindings: Env }>) {
  const metrics = await getIncidentDashboardMetrics(c.env)
  return c.json({ success: true, data: metrics })
}

// ==================== Incident Correlation ====================

export async function getIncidentCorrelationHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string

  // Verify incident exists
  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  const correlation = await findRelatedIncidents(c.env, incidentId)
  return c.json({ success: true, data: correlation })
}

// ==================== Incident Watch ====================

const watchIncidentSchema = z.object({
  notify_on: z.array(z.enum([
    'created',
    'acknowledged',
    'escalated',
    'assigned',
    'merged',
    'analyzed',
    'approved',
    'executing',
    'resolved',
    'failed',
    'evidence_added',
    'comment',
    'severity_upgraded',
    'link_added',
    'runbook_executed',
    'status_changed',
  ])).min(1),
})

export async function watchIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  // Verify incident exists
  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  try {
    const body = watchIncidentSchema.parse(await c.req.json())
    const watch = await watchIncident(c.env, incidentId, principal, body.notify_on as IncidentTimelineEventType[])

    await logAudit(c, principal.sub, 'watch_incident', 'incident', {
      incident_id: incidentId,
      notify_on: body.notify_on,
    })

    return c.json({ success: true, data: watch }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function unwatchIncidentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const removed = await unwatchIncident(c.env, incidentId, principal.sub)

  if (!removed) {
    return c.json({ success: false, error: 'Watch not found' }, 404)
  }

  await logAudit(c, principal.sub, 'unwatch_incident', 'incident', {
    incident_id: incidentId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

export async function getIncidentWatchersHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string

  // Verify incident exists
  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  const watchers = await getIncidentWatchers(c.env, incidentId)
  return c.json({ success: true, data: watchers })
}

// ==================== External Tickets ====================

const createExternalTicketSchema = z.object({
  system: z.enum(['jira', 'servicenow', 'zendesk', 'linear']),
  ticket_id: z.string().min(1),
  ticket_url: z.string().url(),
  status: z.string().min(1),
})

export async function createExternalTicketHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  // Verify incident exists
  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  try {
    const body = createExternalTicketSchema.parse(await c.req.json())
    const ticket = await createExternalTicket(
      c.env,
      incidentId,
      principal,
      body.system,
      body.ticket_id,
      body.ticket_url,
      body.status
    )

    await logAudit(c, principal.sub, 'create_external_ticket', 'incident', {
      incident_id: incidentId,
      system: body.system,
      ticket_id: body.ticket_id,
    })

    return c.json({ success: true, data: ticket }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function listExternalTicketsHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string

  // Verify incident exists
  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  const tickets = await getExternalTickets(c.env, incidentId)
  return c.json({ success: true, data: tickets })
}

const updateExternalTicketSchema = z.object({
  status: z.string().min(1),
})

export async function updateExternalTicketHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const ticketId = c.req.param('ticketId') as string

  try {
    const body = updateExternalTicketSchema.parse(await c.req.json())
    const ticket = await updateExternalTicketStatus(c.env, incidentId, ticketId, body.status)

    if (!ticket) {
      return c.json({ success: false, error: 'Ticket not found' }, 404)
    }

    await logAudit(c, principal.sub, 'update_external_ticket', 'incident', {
      incident_id: incidentId,
      ticket_id: ticketId,
      status: body.status,
    })

    return c.json({ success: true, data: ticket })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

// ==================== Response Playbooks ====================

const responsePlaybookStepSchema = z.object({
  title: z.string().min(1),
  description: z.string().optional(),
  action: z.enum(['manual', 'automated', 'approval']),
  automated_action: z.object({
    type: z.enum(['run_playbook', 'restart_deployment', 'scale_deployment', 'execute_webhook', 'notify']),
    ref: z.string(),
    params: z.record(z.string(), z.unknown()).optional(),
  }).optional(),
  estimated_duration_minutes: z.number().int().min(1).optional(),
  required_role: z.enum(['admin', 'operator', 'viewer']).optional(),
  checklist: z.array(z.string()).optional(),
})

const createResponsePlaybookSchema = z.object({
  name: z.string().min(1).max(200),
  description: z.string().optional(),
  category: z.string().optional(),
  trigger_conditions: z.object({
    severity: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
    source: z.array(z.string()).optional(),
    action_type: z.array(z.enum(['scale_policy', 'restart_deployment', 'scale_deployment'])).optional(),
    title_pattern: z.string().optional(),
    tags: z.array(z.string()).optional(),
  }),
  steps: z.array(responsePlaybookStepSchema).min(1),
  auto_trigger: z.boolean().optional(),
  estimated_total_duration_minutes: z.number().int().min(1).optional(),
})

export async function listResponsePlaybooksHandler(c: Context<{ Bindings: Env }>) {
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const playbooks = await listResponsePlaybooks(c.env, enabled)
  return c.json({ success: true, data: playbooks })
}

export async function getResponsePlaybookHandler(c: Context<{ Bindings: Env }>) {
  const playbookId = c.req.param('playbookId') as string
  const playbook = await getResponsePlaybook(c.env, playbookId)

  if (!playbook) {
    return c.json({ success: false, error: 'Playbook not found' } as ApiErrorResponse, 404)
  }

  return c.json({ success: true, data: playbook })
}

export async function createResponsePlaybookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createResponsePlaybookSchema.parse(await c.req.json())
    const playbook = await createResponsePlaybook(c.env, principal, body)

    await logAudit(c, principal.sub, 'create_response_playbook', 'incident', {
      playbook_id: playbook.id,
      name: playbook.name,
    })

    return c.json({ success: true, data: playbook }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function updateResponsePlaybookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const playbookId = c.req.param('playbookId') as string

  try {
    const body = createResponsePlaybookSchema.partial().parse(await c.req.json())

    // Transform steps if provided - the service will add id and order
    const updates: Parameters<typeof updateResponsePlaybook>[2] = {
      ...body,
      steps: body.steps as unknown as import('../types').ResponsePlaybookStep[] | undefined,
    }

    const playbook = await updateResponsePlaybook(c.env, playbookId, updates)

    if (!playbook) {
      return c.json({ success: false, error: 'Playbook not found' } as ApiErrorResponse, 404)
    }

    await logAudit(c, principal.sub, 'update_response_playbook', 'incident', {
      playbook_id: playbookId,
    })

    return c.json({ success: true, data: playbook })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function deleteResponsePlaybookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const playbookId = c.req.param('playbookId') as string

  const deleted = await deleteResponsePlaybook(c.env, playbookId)

  if (!deleted) {
    return c.json({ success: false, error: 'Playbook not found' } as ApiErrorResponse, 404)
  }

  await logAudit(c, principal.sub, 'delete_response_playbook', 'incident', {
    playbook_id: playbookId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

export async function matchResponsePlaybooksHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string

  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  const playbooks = await matchResponsePlaybooks(c.env, incident)
  return c.json({ success: true, data: playbooks })
}

export async function startPlaybookExecutionHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({ playbook_id: z.string() }).parse(await c.req.json())

  try {
    const execution = await startPlaybookExecution(c.env, incidentId, body.playbook_id, principal)

    await logAudit(c, principal.sub, 'start_playbook_execution', 'incident', {
      incident_id: incidentId,
      playbook_id: body.playbook_id,
      execution_id: execution.id,
    })

    return c.json({ success: true, data: execution }, 201)
  } catch (err) {
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to start playbook' }, 400)
  }
}

export async function getPlaybookExecutionHandler(c: Context<{ Bindings: Env }>) {
  const executionId = c.req.param('executionId') as string
  const execution = await getPlaybookExecution(c.env, executionId)

  if (!execution) {
    return c.json({ success: false, error: 'Execution not found' } as ApiErrorResponse, 404)
  }

  return c.json({ success: true, data: execution })
}

export async function completePlaybookStepHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const executionId = c.req.param('executionId') as string
  const stepId = c.req.param('stepId') as string

  const body = z.object({ result: z.record(z.string(), z.unknown()).optional() }).parse(await c.req.json())

  const execution = await completePlaybookStep(c.env, executionId, stepId, principal, body.result)

  if (!execution) {
    return c.json({ success: false, error: 'Execution not found or not running' }, 404)
  }

  await logAudit(c, principal.sub, 'complete_playbook_step', 'incident', {
    execution_id: executionId,
    step_id: stepId,
  })

  return c.json({ success: true, data: execution })
}

export async function skipPlaybookStepHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const executionId = c.req.param('executionId') as string
  const stepId = c.req.param('stepId') as string

  const body = z.object({ reason: z.string().min(1) }).parse(await c.req.json())

  const execution = await skipPlaybookStep(c.env, executionId, stepId, principal, body.reason)

  if (!execution) {
    return c.json({ success: false, error: 'Execution not found or not running' }, 404)
  }

  await logAudit(c, principal.sub, 'skip_playbook_step', 'incident', {
    execution_id: executionId,
    step_id: stepId,
    reason: body.reason,
  })

  return c.json({ success: true, data: execution })
}

// ==================== Custom Fields ====================

const createCustomFieldSchema = z.object({
  name: z.string().min(1).max(100),
  key: z.string().min(1).max(50).regex(/^[a-z][a-z0-9_]*$/),
  type: z.enum(['text', 'number', 'boolean', 'select', 'multiselect', 'date', 'datetime', 'user', 'url']),
  required: z.boolean().optional(),
  default_value: z.union([z.string(), z.number(), z.boolean(), z.array(z.string())]).optional(),
  options: z.array(z.string()).optional(),
  validation: z.object({
    min: z.number().optional(),
    max: z.number().optional(),
    pattern: z.string().optional(),
    min_length: z.number().int().optional(),
    max_length: z.number().int().optional(),
  }).optional(),
  description: z.string().optional(),
  category: z.string().optional(),
})

export async function listCustomFieldsHandler(c: Context<{ Bindings: Env }>) {
  const category = c.req.query('category')
  const fields = await listCustomFieldDefinitions(c.env, category)
  return c.json({ success: true, data: fields })
}

export async function getCustomFieldHandler(c: Context<{ Bindings: Env }>) {
  const fieldId = c.req.param('fieldId') as string
  const field = await getCustomFieldDefinition(c.env, fieldId)

  if (!field) {
    return c.json({ success: false, error: 'Field not found' }, 404)
  }

  return c.json({ success: true, data: field })
}

export async function createCustomFieldHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createCustomFieldSchema.parse(await c.req.json())
    const field = await createCustomFieldDefinition(c.env, principal, body)

    await logAudit(c, principal.sub, 'create_custom_field', 'incident', {
      field_id: field.id,
      key: field.key,
    })

    return c.json({ success: true, data: field }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to create field' }, 400)
  }
}

export async function updateCustomFieldHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const fieldId = c.req.param('fieldId') as string

  try {
    const body = createCustomFieldSchema.partial().parse(await c.req.json())
    const field = await updateCustomFieldDefinition(c.env, fieldId, body)

    if (!field) {
      return c.json({ success: false, error: 'Field not found' }, 404)
    }

    await logAudit(c, principal.sub, 'update_custom_field', 'incident', {
      field_id: fieldId,
    })

    return c.json({ success: true, data: field })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function deleteCustomFieldHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const fieldId = c.req.param('fieldId') as string

  const deleted = await deleteCustomFieldDefinition(c.env, fieldId)

  if (!deleted) {
    return c.json({ success: false, error: 'Field not found' }, 404)
  }

  await logAudit(c, principal.sub, 'delete_custom_field', 'incident', {
    field_id: fieldId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

export async function setIncidentCustomFieldHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const fieldId = c.req.param('fieldId') as string

  const body = z.object({
    value: z.union([z.string(), z.number(), z.boolean(), z.array(z.string()), z.null()]),
  }).parse(await c.req.json())

  try {
    const fieldValue = await setIncidentCustomField(c.env, incidentId, fieldId, body.value, principal)

    await logAudit(c, principal.sub, 'set_custom_field', 'incident', {
      incident_id: incidentId,
      field_id: fieldId,
    })

    return c.json({ success: true, data: fieldValue })
  } catch (err) {
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to set field' }, 400)
  }
}

export async function getIncidentCustomFieldsHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string

  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  const fields = await getIncidentCustomFields(c.env, incidentId)
  return c.json({ success: true, data: fields })
}

// ==================== AI Root Cause Analysis ====================

export async function generateAIRootCauseAnalysisHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const incident = await getIncident(c.env, incidentId)
  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  try {
    const analysis = await generateAIRootCauseAnalysis(c.env, incidentId)

    await logAudit(c, principal.sub, 'generate_ai_analysis', 'incident', {
      incident_id: incidentId,
    })

    return c.json({ success: true, data: analysis })
  } catch (err) {
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to generate analysis' }, 500)
  }
}

// ==================== War Room ====================

export async function createWarRoomHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const warRoom = await createWarRoom(c.env, incidentId, principal)

    await logAudit(c, principal.sub, 'create_war_room', 'incident', {
      incident_id: incidentId,
      war_room_id: warRoom.id,
    })

    return c.json({ success: true, data: warRoom }, 201)
  } catch (err) {
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to create war room' }, 400)
  }
}

export async function getWarRoomHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const warRoom = await getWarRoom(c.env, incidentId)

  if (!warRoom) {
    return c.json({ success: false, error: 'War room not found' }, 404)
  }

  return c.json({ success: true, data: warRoom })
}

export async function joinWarRoomHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    role: z.enum(['commander', 'responder', 'observer']).optional(),
  }).parse(await c.req.json())

  const warRoom = await joinWarRoom(c.env, incidentId, principal, body.role)

  if (!warRoom) {
    return c.json({ success: false, error: 'War room not found or closed' }, 404)
  }

  return c.json({ success: true, data: warRoom })
}

export async function leaveWarRoomHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const warRoom = await leaveWarRoom(c.env, incidentId, principal)

  if (!warRoom) {
    return c.json({ success: false, error: 'War room not found' }, 404)
  }

  return c.json({ success: true, data: warRoom })
}

export async function addWarRoomMessageHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({ message: z.string().min(1).max(5000) }).parse(await c.req.json())

  const warRoom = await addWarRoomMessage(c.env, incidentId, principal, body.message)

  if (!warRoom) {
    return c.json({ success: false, error: 'War room not found, closed, or not a participant' }, 404)
  }

  return c.json({ success: true, data: warRoom })
}

export async function addWarRoomResourceHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    type: z.enum(['link', 'document', 'dashboard', 'log']),
    title: z.string().min(1).max(200),
    url: z.string().url(),
  }).parse(await c.req.json())

  const warRoom = await addWarRoomResource(c.env, incidentId, principal, body)

  if (!warRoom) {
    return c.json({ success: false, error: 'War room not found or closed' }, 404)
  }

  await logAudit(c, principal.sub, 'add_war_room_resource', 'incident', {
    incident_id: incidentId,
    resource_type: body.type,
    resource_title: body.title,
  })

  return c.json({ success: true, data: warRoom })
}

export async function closeWarRoomHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const warRoom = await closeWarRoom(c.env, incidentId, principal)

  if (!warRoom) {
    return c.json({ success: false, error: 'War room not found or already closed' }, 404)
  }

  await logAudit(c, principal.sub, 'close_war_room', 'incident', {
    incident_id: incidentId,
  })

  return c.json({ success: true, data: warRoom })
}

// ==================== Incident Export ====================

const incidentExportSchema = z.object({
  format: z.enum(['json', 'csv']),
  include_evidence: z.boolean().optional(),
  include_timeline: z.boolean().optional(),
  include_comments: z.boolean().optional(),
  include_recommendations: z.boolean().optional(),
  include_links: z.boolean().optional(),
  include_postmortem: z.boolean().optional(),
  date_range: z.object({
    start: z.string().datetime(),
    end: z.string().datetime(),
  }).optional(),
  filters: z.object({
    status: z.array(z.enum(['open', 'analyzed', 'approved', 'executing', 'resolved', 'failed'])).optional(),
    severity: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
    source: z.array(z.string()).optional(),
  }).optional(),
})

export async function exportIncidentsHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = incidentExportSchema.parse(await c.req.json())

    // Provide defaults for optional boolean fields
    const exportOptions: import('../types').IncidentExportOptions = {
      format: body.format,
      include_evidence: body.include_evidence ?? false,
      include_timeline: body.include_timeline ?? false,
      include_comments: body.include_comments ?? false,
      include_recommendations: body.include_recommendations ?? false,
      include_links: body.include_links ?? false,
      include_postmortem: body.include_postmortem ?? false,
      date_range: body.date_range,
      filters: body.filters,
    }

    const result = await exportIncidents(c.env, exportOptions, principal)

    await logAudit(c, principal.sub, 'export_incidents', 'incident', {
      export_id: result.id,
      format: body.format,
      total_incidents: result.total_incidents,
    })

    return c.json({ success: true, data: result }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function getExportStatusHandler(c: Context<{ Bindings: Env }>) {
  const exportId = c.req.param('exportId') as string
  const result = await getExportResult(c.env, exportId)

  if (!result) {
    return c.json({ success: false, error: 'Export not found' }, 404)
  }

  return c.json({ success: true, data: result })
}

export async function downloadExportHandler(c: Context<{ Bindings: Env }>) {
  const exportId = c.req.param('exportId') as string
  const download = await getExportDownload(c.env, exportId)

  if (!download) {
    return c.json({ success: false, error: 'Export not found or not ready' }, 404)
  }

  return new Response(download.body, {
    headers: {
      'Content-Type': download.contentType,
      'Content-Disposition': `attachment; filename="incidents-${exportId}.${download.contentType === 'application/json' ? 'json' : 'csv'}"`,
    },
  })
}

// ==================== Incident Reviews ====================

const createReviewSchema = z.object({
  scheduled_at: z.string().datetime(),
  review_type: z.enum(['post_resolution', 'weekly', 'monthly', 'custom']),
  attendees: z.array(z.object({
    user_id: z.number(),
    email: z.string().email(),
    required: z.boolean().optional(),
  })).optional(),
  agenda: z.array(z.string()).optional(),
})

export async function createReviewHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const body = createReviewSchema.parse(await c.req.json())
    const review = await createIncidentReview(c.env, incidentId, principal, body)

    await logAudit(c, principal.sub, 'create_incident_review', 'incident', {
      incident_id: incidentId,
      review_id: review.id,
    })

    return c.json({ success: true, data: review }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to create review' }, 400)
  }
}

export async function listReviewsHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const reviews = await getIncidentReviews(c.env, incidentId)
  return c.json({ success: true, data: reviews })
}

export async function completeReviewHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const reviewId = c.req.param('reviewId') as string

  const body = z.object({
    notes: z.string().optional(),
    action_items: z.array(z.object({
      description: z.string(),
      owner_id: z.number().optional(),
      due_date: z.string().optional(),
    })).optional(),
  }).parse(await c.req.json())

  const review = await completeIncidentReview(c.env, incidentId, reviewId, principal, body.notes, body.action_items)

  if (!review) {
    return c.json({ success: false, error: 'Review not found' }, 404)
  }

  await logAudit(c, principal.sub, 'complete_incident_review', 'incident', {
    incident_id: incidentId,
    review_id: reviewId,
  })

  return c.json({ success: true, data: review })
}

// ==================== Response Analytics ====================

export async function getResponseAnalyticsHandler(c: Context<{ Bindings: Env }>) {
  const startDate = c.req.query('start_date')
  const endDate = c.req.query('end_date')

  if (!startDate || !endDate) {
    return c.json({ success: false, error: 'start_date and end_date are required' }, 400)
  }

  const analytics = await calculateResponseAnalytics(c.env, startDate, endDate)
  return c.json({ success: true, data: analytics })
}

// ==================== Incident Feedback ====================

const submitFeedbackSchema = z.object({
  ratings: z.object({
    overall_satisfaction: z.union([z.literal(1), z.literal(2), z.literal(3), z.literal(4), z.literal(5)]),
    response_speed: z.union([z.literal(1), z.literal(2), z.literal(3), z.literal(4), z.literal(5)]),
    communication: z.union([z.literal(1), z.literal(2), z.literal(3), z.literal(4), z.literal(5)]),
    resolution_quality: z.union([z.literal(1), z.literal(2), z.literal(3), z.literal(4), z.literal(5)]),
  }),
  strengths: z.array(z.string()).optional(),
  improvements: z.array(z.string()).optional(),
  additional_comments: z.string().optional(),
  would_recommend: z.boolean(),
})

export async function submitFeedbackHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const body = submitFeedbackSchema.parse(await c.req.json())
    const feedback = await submitIncidentFeedback(c.env, incidentId, principal, body)

    await logAudit(c, principal.sub, 'submit_incident_feedback', 'incident', {
      incident_id: incidentId,
    })

    return c.json({ success: true, data: feedback }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to submit feedback' }, 400)
  }
}

export async function getFeedbackHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const feedback = await getIncidentFeedback(c.env, incidentId)

  if (!feedback) {
    return c.json({ success: false, error: 'Feedback not found' }, 404)
  }

  return c.json({ success: true, data: feedback })
}

// ==================== Incident Cost ====================

const calculateCostSchema = z.object({
  labor_hours: z.number().min(0),
  labor_rate_usd: z.number().min(0).optional(),
  infrastructure_cost_usd: z.number().min(0).optional(),
  revenue_impact_usd: z.number().min(0).optional(),
  third_party_cost_usd: z.number().min(0).optional(),
  notes: z.string().optional(),
})

export async function calculateCostHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const body = calculateCostSchema.parse(await c.req.json())
    const cost = await calculateIncidentCost(c.env, incidentId, principal, body)

    await logAudit(c, principal.sub, 'calculate_incident_cost', 'incident', {
      incident_id: incidentId,
      total_cost: cost.estimated_cost_usd,
    })

    return c.json({ success: true, data: cost }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to calculate cost' }, 400)
  }
}

export async function getCostHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const cost = await getIncidentCost(c.env, incidentId)

  if (!cost) {
    return c.json({ success: false, error: 'Cost not found' }, 404)
  }

  return c.json({ success: true, data: cost })
}

// ==================== Incident Compliance ====================

const createComplianceSchema = z.object({
  framework: z.enum(['soc2', 'iso27001', 'gdpr', 'hipaa', 'pci', 'custom']),
  requirements: z.array(z.object({
    requirement_id: z.string(),
    description: z.string(),
  })),
})

export async function createComplianceHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const body = createComplianceSchema.parse(await c.req.json())
    const record = await createComplianceRecord(c.env, incidentId, principal, body.framework, body.requirements)

    await logAudit(c, principal.sub, 'create_compliance_record', 'incident', {
      incident_id: incidentId,
      framework: body.framework,
    })

    return c.json({ success: true, data: record }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    return c.json({ success: false, error: err instanceof Error ? err.message : 'Failed to create compliance record' }, 400)
  }
}

export async function getComplianceHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const record = await getComplianceRecord(c.env, incidentId)

  if (!record) {
    return c.json({ success: false, error: 'Compliance record not found' }, 404)
  }

  return c.json({ success: true, data: record })
}

export async function updateComplianceHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const requirementId = c.req.param('requirementId') as string

  const body = z.object({
    status: z.enum(['compliant', 'non_compliant', 'not_applicable']),
    evidence: z.string().optional(),
    notes: z.string().optional(),
  }).parse(await c.req.json())

  const record = await updateComplianceRequirement(
    c.env,
    incidentId,
    requirementId,
    body.status,
    body.evidence,
    body.notes,
    principal
  )

  if (!record) {
    return c.json({ success: false, error: 'Record or requirement not found' }, 404)
  }

  await logAudit(c, principal.sub, 'update_compliance_requirement', 'incident', {
    incident_id: incidentId,
    requirement_id: requirementId,
    status: body.status,
  })

  return c.json({ success: true, data: record })
}

// ==================== On-Call Schedules ====================

const createOnCallSchema = z.object({
  name: z.string().min(1).max(100),
  description: z.string().optional(),
  team: z.string().optional(),
  rotation_type: z.enum(['weekly', 'biweekly', 'monthly', 'custom']),
  rotation_config: z.object({
    start_date: z.string().datetime(),
    members: z.array(z.object({
      user_id: z.number(),
      email: z.string().email(),
      order: z.number().int().min(1),
    })).min(1),
    handoff_time: z.string().regex(/^\d{2}:\d{2}$/),
    handoff_day: z.number().int().min(0).max(6).optional(),
  }),
  timezone: z.string().optional(),
})

export async function listOnCallSchedulesHandler(c: Context<{ Bindings: Env }>) {
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const schedules = await listOnCallSchedules(c.env, enabled)
  return c.json({ success: true, data: schedules })
}

export async function getOnCallScheduleHandler(c: Context<{ Bindings: Env }>) {
  const scheduleId = c.req.param('scheduleId') as string
  const schedule = await getOnCallSchedule(c.env, scheduleId)

  if (!schedule) {
    return c.json({ success: false, error: 'Schedule not found' }, 404)
  }

  return c.json({ success: true, data: schedule })
}

export async function createOnCallScheduleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createOnCallSchema.parse(await c.req.json())
    const schedule = await createOnCallSchedule(c.env, principal, body)

    await logAudit(c, principal.sub, 'create_oncall_schedule', 'incident', {
      schedule_id: schedule.id,
      name: schedule.name,
    })

    return c.json({ success: true, data: schedule }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function getCurrentOnCallHandler(c: Context<{ Bindings: Env }>) {
  const scheduleId = c.req.param('scheduleId') as string
  const shift = await getCurrentOnCall(c.env, scheduleId)

  if (!shift) {
    return c.json({ success: false, error: 'No active on-call found' }, 404)
  }

  return c.json({ success: true, data: shift })
}

// ==================== Incident Checklists ====================

export async function listChecklistsHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const checklists = await getIncidentChecklists(c.env, incidentId)
  return c.json({ success: true, data: checklists })
}

export async function createChecklistHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    name: z.string().min(1).max(100),
    items: z.array(z.string().min(1)).min(1),
  }).parse(await c.req.json())

  const checklist = await createIncidentChecklist(c.env, incidentId, principal, body.name, body.items)

  await logAudit(c, principal.sub, 'create_checklist', 'incident', {
    incident_id: incidentId,
    checklist_id: checklist.id,
  })

  return c.json({ success: true, data: checklist }, 201)
}

export async function updateChecklistItemHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const checklistId = c.req.param('checklistId') as string
  const itemId = c.req.param('itemId') as string

  const body = z.object({ checked: z.boolean() }).parse(await c.req.json())

  const checklist = await updateChecklistItem(c.env, incidentId, checklistId, itemId, body.checked, principal)

  if (!checklist) {
    return c.json({ success: false, error: 'Checklist or item not found' }, 404)
  }

  return c.json({ success: true, data: checklist })
}

// ==================== Incident Change Links ====================

const linkChangeSchema = z.object({
  change_id: z.string().min(1),
  change_type: z.enum(['deployment', 'configuration', 'infrastructure', 'schedule']),
  change_description: z.string().optional(),
  change_url: z.string().url().optional(),
  change_timestamp: z.string().datetime(),
  relationship: z.enum(['caused', 'contributed', 'resolved', 'related']),
})

export async function linkChangeHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  try {
    const body = linkChangeSchema.parse(await c.req.json())
    const link = await linkIncidentToChange(c.env, incidentId, principal, body)

    await logAudit(c, principal.sub, 'link_change_to_incident', 'incident', {
      incident_id: incidentId,
      change_id: body.change_id,
      relationship: body.relationship,
    })

    return c.json({ success: true, data: link }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues } as SchemaValidationErrorResponse, 400)
    }
    throw err
  }
}

export async function listChangesHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const changes = await getIncidentChanges(c.env, incidentId)
  return c.json({ success: true, data: changes })
}

// ==================== Incident Run History ====================

export async function listRunHistoryHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const history = await getIncidentRunHistory(c.env, incidentId)
  return c.json({ success: true, data: history })
}

// ==================== Responder Teams ====================

export async function listResponderTeamsHandler(c: Context<{ Bindings: Env }>) {
  const teams = await listResponderTeams(c.env)
  return c.json({ success: true, data: teams })
}

export async function getResponderTeamHandler(c: Context<{ Bindings: Env }>) {
  const teamId = c.req.param('teamId') as string
  const team = await getResponderTeam(c.env, teamId)

  if (!team) {
    return c.json({ success: false, error: 'Team not found' }, 404)
  }

  return c.json({ success: true, data: team })
}

export async function createResponderTeamHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().optional(),
    members: z.array(z.object({
      user_id: z.number(),
      email: z.string().email(),
      role: z.enum(['lead', 'responder', 'observer']),
      skills: z.array(z.string()).optional(),
    })).min(1),
    services: z.array(z.string()).optional(),
  }).parse(await c.req.json())

  const team = await createResponderTeam(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_responder_team', 'incident', {
    team_id: team.id,
    name: team.name,
  })

  return c.json({ success: true, data: team }, 201)
}

export async function updateResponderTeamHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const teamId = c.req.param('teamId') as string

  const body = z.object({
    name: z.string().min(1).max(100).optional(),
    description: z.string().optional(),
    members: z.array(z.object({
      user_id: z.number(),
      email: z.string().email(),
      role: z.enum(['lead', 'responder', 'observer']),
      skills: z.array(z.string()).optional(),
    })).optional(),
    services: z.array(z.string()).optional(),
  }).parse(await c.req.json())

  const team = await updateResponderTeam(c.env, teamId, body)

  if (!team) {
    return c.json({ success: false, error: 'Team not found' }, 404)
  }

  await logAudit(c, principal.sub, 'update_responder_team', 'incident', {
    team_id: teamId,
  })

  return c.json({ success: true, data: team })
}

export async function deleteResponderTeamHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const teamId = c.req.param('teamId') as string

  const deleted = await deleteResponderTeam(c.env, teamId)

  if (!deleted) {
    return c.json({ success: false, error: 'Team not found' }, 404)
  }

  await logAudit(c, principal.sub, 'delete_responder_team', 'incident', {
    team_id: teamId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== SLA Calendars ====================

export async function listSLACalendarsHandler(c: Context<{ Bindings: Env }>) {
  const calendars = await listSLACalendars(c.env)
  return c.json({ success: true, data: calendars })
}

export async function getSLACalendarHandler(c: Context<{ Bindings: Env }>) {
  const calendarId = c.req.param('calendarId') as string
  const calendar = await getSLACalendar(c.env, calendarId)

  if (!calendar) {
    return c.json({ success: false, error: 'Calendar not found' }, 404)
  }

  return c.json({ success: true, data: calendar })
}

export async function createSLACalendarHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().optional(),
    timezone: z.string(),
    working_hours: z.object({
      start: z.string().regex(/^\d{2}:\d{2}$/),
      end: z.string().regex(/^\d{2}:\d{2}$/),
      days: z.array(z.number().int().min(0).max(6)),
    }),
    holidays: z.array(z.object({
      date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
      name: z.string(),
    })).optional(),
    is_default: z.boolean().optional(),
  }).parse(await c.req.json())

  const calendar = await createSLACalendar(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_sla_calendar', 'incident', {
    calendar_id: calendar.id,
    name: calendar.name,
  })

  return c.json({ success: true, data: calendar }, 201)
}

// ==================== Notification Templates ====================

export async function listNotificationTemplatesHandler(c: Context<{ Bindings: Env }>) {
  const channel = c.req.query('channel')
  const templates = await listNotificationTemplates(c.env, channel)
  return c.json({ success: true, data: templates })
}

export async function getNotificationTemplateHandler(c: Context<{ Bindings: Env }>) {
  const templateId = c.req.param('templateId') as string
  const template = await getNotificationTemplate(c.env, templateId)

  if (!template) {
    return c.json({ success: false, error: 'Template not found' } as ApiErrorResponse, 404)
  }

  return c.json({ success: true, data: template })
}

export async function createNotificationTemplateHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().optional(),
    channel: z.enum(['email', 'slack', 'teams', 'webhook', 'sms']),
    event_type: z.enum([
      'incident.created', 'incident.acknowledged', 'incident.escalated',
      'incident.assigned', 'incident.analyzed', 'incident.approved',
      'incident.executing', 'incident.resolved', 'incident.failed', 'all',
    ]),
    subject_template: z.string().optional(),
    body_template: z.string().min(1),
    variables: z.array(z.string()).optional(),
  }).parse(await c.req.json())

  const template = await createNotificationTemplate(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_notification_template', 'incident', {
    template_id: template.id,
    name: template.name,
  })

  return c.json({ success: true, data: template }, 201)
}

// ==================== Escalation Rules ====================

export async function listEscalationRulesHandler(c: Context<{ Bindings: Env }>) {
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const rules = await listEscalationRules(c.env, enabled)
  return c.json({ success: true, data: rules })
}

export async function createEscalationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().optional(),
    from_severity: z.enum(['low', 'medium', 'high', 'critical']),
    to_severity: z.enum(['low', 'medium', 'high', 'critical']),
    trigger_conditions: z.object({
      time_without_ack_minutes: z.number().int().min(1).optional(),
      time_without_assign_minutes: z.number().int().min(1).optional(),
      time_without_resolve_minutes: z.number().int().min(1).optional(),
    }),
    actions: z.array(z.object({
      type: z.enum(['notify', 'escalate', 'auto_assign']),
      target: z.string(),
    })),
  }).parse(await c.req.json())

  const rule = await createEscalationRule(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_escalation_rule', 'incident', {
    rule_id: rule.id,
    name: rule.name,
  })

  return c.json({ success: true, data: rule }, 201)
}

// ==================== Incident Attachments ====================

export async function listAttachmentsHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const attachments = await listIncidentAttachments(c.env, incidentId)
  return c.json({ success: true, data: attachments })
}

export async function uploadAttachmentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  // Note: In a real implementation, you'd parse multipart/form-data
  // This is a simplified version
  const body = await c.req.json()
  const { filename, content_type, content_base64, description } = body

  const content = Uint8Array.from(atob(content_base64), c => c.charCodeAt(0)).buffer

  const attachment = await uploadIncidentAttachment(c.env, incidentId, principal, {
    filename,
    contentType: content_type,
    content,
  }, description)

  await logAudit(c, principal.sub, 'upload_attachment', 'incident', {
    incident_id: incidentId,
    attachment_id: attachment.id,
    filename: attachment.filename,
  })

  return c.json({ success: true, data: attachment }, 201)
}

export async function downloadAttachmentHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const attachmentId = c.req.param('attachmentId') as string

  const result = await downloadIncidentAttachment(c.env, incidentId, attachmentId)

  if (!result) {
    return c.json({ success: false, error: 'Attachment not found' }, 404)
  }

  return new Response(result.body, {
    headers: {
      'Content-Type': result.contentType,
      'Content-Disposition': `attachment; filename="${result.filename}"`,
    },
  })
}

export async function deleteAttachmentHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const attachmentId = c.req.param('attachmentId') as string

  const deleted = await deleteIncidentAttachment(c.env, incidentId, attachmentId)

  if (!deleted) {
    return c.json({ success: false, error: 'Attachment not found' }, 404)
  }

  await logAudit(c, principal.sub, 'delete_attachment', 'incident', {
    incident_id: incidentId,
    attachment_id: attachmentId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== Related Items ====================

export async function listRelatedItemsHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const itemType = c.req.query('item_type')
  const items = await listRelatedItems(c.env, incidentId, itemType)
  return c.json({ success: true, data: items })
}

export async function addRelatedItemHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    item_type: z.enum(['log', 'metric', 'trace', 'alert', 'runbook', 'documentation', 'code', 'config']),
    title: z.string().min(1).max(200),
    description: z.string().optional(),
    url: z.string().url().optional(),
    content: z.string().optional(),
    metadata: z.record(z.string(), z.unknown()).optional(),
  }).parse(await c.req.json())

  const item = await addRelatedItem(c.env, incidentId, principal, body)

  await logAudit(c, principal.sub, 'add_related_item', 'incident', {
    incident_id: incidentId,
    item_id: item.id,
    item_type: body.item_type,
  })

  return c.json({ success: true, data: item }, 201)
}

export async function removeRelatedItemHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string
  const itemId = c.req.param('itemId') as string

  const removed = await removeRelatedItem(c.env, incidentId, itemId)

  if (!removed) {
    return c.json({ success: false, error: 'Item not found' }, 404)
  }

  await logAudit(c, principal.sub, 'remove_related_item', 'incident', {
    incident_id: incidentId,
    item_id: itemId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== Response Time Targets ====================

export async function listResponseTargetsHandler(c: Context<{ Bindings: Env }>) {
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const targets = await listResponseTimeTargets(c.env, enabled)
  return c.json({ success: true, data: targets })
}

export async function createResponseTargetHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    severity: z.enum(['low', 'medium', 'high', 'critical']),
    target_type: z.enum(['acknowledge', 'assign', 'resolve', 'first_response']),
    target_minutes: z.number().int().min(1),
    business_hours_only: z.boolean().optional(),
    sla_calendar_id: z.string().optional(),
  }).parse(await c.req.json())

  const target = await createResponseTimeTarget(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_response_target', 'incident', {
    target_id: target.id,
    name: target.name,
  })

  return c.json({ success: true, data: target }, 201)
}

// ==================== Integrations ====================

export async function listIntegrationsHandler(c: Context<{ Bindings: Env }>) {
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const integrations = await listIntegrations(c.env, enabled)
  return c.json({ success: true, data: integrations })
}

export async function getIntegrationHandler(c: Context<{ Bindings: Env }>) {
  const integrationId = c.req.param('integrationId') as string
  const integrations = await listIntegrations(c.env)
  const integration = integrations.find(i => i.id === integrationId)

  if (!integration) {
    return c.json({ success: false, error: 'Integration not found' }, 404)
  }

  return c.json({ success: true, data: integration })
}

export async function createIntegrationHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    type: z.enum(['pagerduty', 'opsgenie', 'datadog', 'newrelic', 'prometheus', 'grafana', 'slack', 'teams', 'custom']),
    config: z.record(z.string(), z.unknown()),
    mapping_rules: z.array(z.object({
      source_field: z.string(),
      target_field: z.string(),
      transform: z.string().optional(),
    })).optional(),
  }).parse(await c.req.json())

  const integration = await createIntegration(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_integration', 'incident', {
    integration_id: integration.id,
    name: integration.name,
    type: integration.type,
  })

  return c.json({ success: true, data: integration }, 201)
}

export async function updateIntegrationHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const integrationId = c.req.param('integrationId') as string

  const body = z.object({
    name: z.string().min(1).max(100).optional(),
    enabled: z.boolean().optional(),
    config: z.record(z.string(), z.unknown()).optional(),
    mapping_rules: z.array(z.object({
      source_field: z.string(),
      target_field: z.string(),
      transform: z.string().optional(),
    })).optional(),
  }).parse(await c.req.json())

  const integration = await updateIntegration(c.env, integrationId, body)

  if (!integration) {
    return c.json({ success: false, error: 'Integration not found' }, 404)
  }

  await logAudit(c, principal.sub, 'update_integration', 'incident', {
    integration_id: integrationId,
  })

  return c.json({ success: true, data: integration })
}

export async function deleteIntegrationHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const integrationId = c.req.param('integrationId') as string

  const deleted = await deleteIntegration(c.env, integrationId)

  if (!deleted) {
    return c.json({ success: false, error: 'Integration not found' }, 404)
  }

  await logAudit(c, principal.sub, 'delete_integration', 'incident', {
    integration_id: integrationId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== Timeline Events ====================

export async function listTimelineEventsHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.param('id') as string
  const events = await listTimelineEvents(c.env, incidentId)
  return c.json({ success: true, data: events })
}

export async function addTimelineEventHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    type: z.enum(['created', 'acknowledged', 'escalated', 'assigned', 'merged', 'analyzed', 'approved', 'executing', 'resolved', 'failed', 'evidence_added', 'comment', 'severity_upgraded']),
    summary: z.string().min(1).max(1000),
    details: z.record(z.string(), z.unknown()).optional(),
    metadata: z.record(z.string(), z.unknown()).optional(),
  }).parse(await c.req.json())

  const event = await addTimelineEvent(c.env, incidentId, {
    ...body,
    timestamp: new Date().toISOString(),
    actor: {
      user_id: principal.sub,
      email: principal.email,
    },
  })

  await logAudit(c, principal.sub, 'add_timeline_event', 'incident', {
    incident_id: incidentId,
    type: body.type,
  })

  return c.json({ success: true, data: event }, 201)
}

// ==================== Runbooks ====================

export async function listRunbooksHandler(c: Context<{ Bindings: Env }>) {
  const category = c.req.query('category')
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const runbooks = await listRunbooks(c.env, category, enabled)
  return c.json({ success: true, data: runbooks })
}

export async function getRunbookHandler(c: Context<{ Bindings: Env }>) {
  const runbookId = c.req.param('runbookId') as string
  const runbook = await getRunbook(c.env, runbookId)

  if (!runbook) {
    return c.json({ success: false, error: 'Runbook not found' }, 404)
  }

  return c.json({ success: true, data: runbook })
}

export async function createRunbookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().max(500).optional(),
    category: z.enum(['incident_response', 'remediation', 'communication', 'escalation', 'recovery']),
    triggers: z.array(z.object({
      type: z.enum(['severity', 'source', 'tag', 'service']),
      value: z.string(),
    })),
    steps: z.array(z.object({
      id: z.string(),
      order: z.number().int().min(0),
      title: z.string().min(1).max(200),
      description: z.string().max(1000).optional(),
      action_type: z.enum(['manual', 'automated', 'approval', 'notification']),
      action_config: z.object({
        script: z.string().optional(),
        api_endpoint: z.string().optional(),
        notification_template_id: z.string().optional(),
        approver_roles: z.array(z.string()).optional(),
        timeout_minutes: z.number().int().optional(),
      }).optional(),
      required: z.boolean(),
      estimated_minutes: z.number().int().min(1).optional(),
    })),
    auto_start: z.boolean(),
    enabled: z.boolean(),
  }).parse(await c.req.json())

  const runbook = await createRunbook(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_runbook', 'incident', {
    runbook_id: runbook.id,
    name: runbook.name,
  })

  return c.json({ success: true, data: runbook }, 201)
}

export async function updateRunbookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const runbookId = c.req.param('runbookId') as string

  const body = z.object({
    name: z.string().min(1).max(100).optional(),
    description: z.string().max(500).optional(),
    category: z.enum(['incident_response', 'remediation', 'communication', 'escalation', 'recovery']).optional(),
    triggers: z.array(z.object({
      type: z.enum(['severity', 'source', 'tag', 'service']),
      value: z.string(),
    })).optional(),
    steps: z.array(z.object({
      id: z.string(),
      order: z.number().int().min(0),
      title: z.string().min(1).max(200),
      description: z.string().max(1000).optional(),
      action_type: z.enum(['manual', 'automated', 'approval', 'notification']),
      action_config: z.object({
        script: z.string().optional(),
        api_endpoint: z.string().optional(),
        notification_template_id: z.string().optional(),
        approver_roles: z.array(z.string()).optional(),
        timeout_minutes: z.number().int().optional(),
      }).optional(),
      required: z.boolean(),
      estimated_minutes: z.number().int().min(1).optional(),
    })).optional(),
    auto_start: z.boolean().optional(),
    enabled: z.boolean().optional(),
  }).parse(await c.req.json())

  const runbook = await updateRunbook(c.env, runbookId, body)

  if (!runbook) {
    return c.json({ success: false, error: 'Runbook not found' }, 404)
  }

  await logAudit(c, principal.sub, 'update_runbook', 'incident', {
    runbook_id: runbookId,
  })

  return c.json({ success: true, data: runbook })
}

export async function deleteRunbookHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const runbookId = c.req.param('runbookId') as string

  const deleted = await deleteRunbook(c.env, runbookId)

  if (!deleted) {
    return c.json({ success: false, error: 'Runbook not found' }, 404)
  }

  await logAudit(c, principal.sub, 'delete_runbook', 'incident', {
    runbook_id: runbookId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== Auto-Remediation Rules ====================

export async function listAutoRemediationRulesHandler(c: Context<{ Bindings: Env }>) {
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const rules = await listAutoRemediationRules(c.env, enabled)
  return c.json({ success: true, data: rules })
}

export async function createAutoRemediationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().max(500).optional(),
    conditions: z.array(z.object({
      field: z.enum(['severity', 'source', 'title_pattern', 'tag', 'service', 'metric_threshold']),
      operator: z.enum(['equals', 'contains', 'matches', 'greater_than', 'less_than']),
      value: z.union([z.string(), z.number()]),
    })),
    logical_operator: z.enum(['and', 'or']),
    action: z.object({
      type: z.enum(['run_playbook', 'execute_script', 'api_call', 'scale_service', 'restart_service']),
      config: z.object({
        playbook_id: z.string().optional(),
        script: z.string().optional(),
        api_endpoint: z.string().optional(),
        api_method: z.string().optional(),
        api_payload: z.record(z.string(), z.unknown()).optional(),
        service_name: z.string().optional(),
        target_replicas: z.number().int().optional(),
      }),
    }),
    requires_approval: z.boolean(),
    approver_roles: z.array(z.string()),
    cooldown_minutes: z.number().int().min(0),
    max_executions_per_hour: z.number().int().min(1),
    enabled: z.boolean(),
  }).parse(await c.req.json())

  const rule = await createAutoRemediationRule(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_auto_remediation_rule', 'incident', {
    rule_id: rule.id,
    name: rule.name,
  })

  return c.json({ success: true, data: rule }, 201)
}

export async function updateAutoRemediationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  const body = z.object({
    name: z.string().min(1).max(100).optional(),
    description: z.string().max(500).optional(),
    conditions: z.array(z.object({
      field: z.enum(['severity', 'source', 'title_pattern', 'tag', 'service', 'metric_threshold']),
      operator: z.enum(['equals', 'contains', 'matches', 'greater_than', 'less_than']),
      value: z.union([z.string(), z.number()]),
    })).optional(),
    logical_operator: z.enum(['and', 'or']).optional(),
    action: z.object({
      type: z.enum(['run_playbook', 'execute_script', 'api_call', 'scale_service', 'restart_service']),
      config: z.object({
        playbook_id: z.string().optional(),
        script: z.string().optional(),
        api_endpoint: z.string().optional(),
        api_method: z.string().optional(),
        api_payload: z.record(z.string(), z.unknown()).optional(),
        service_name: z.string().optional(),
        target_replicas: z.number().int().optional(),
      }),
    }).optional(),
    requires_approval: z.boolean().optional(),
    approver_roles: z.array(z.string()).optional(),
    cooldown_minutes: z.number().int().min(0).optional(),
    max_executions_per_hour: z.number().int().min(1).optional(),
    enabled: z.boolean().optional(),
  }).parse(await c.req.json())

  const rule = await updateAutoRemediationRule(c.env, ruleId, body)

  if (!rule) {
    return c.json({ success: false, error: 'Rule not found' }, 404)
  }

  await logAudit(c, principal.sub, 'update_auto_remediation_rule', 'incident', {
    rule_id: ruleId,
  })

  return c.json({ success: true, data: rule })
}

export async function deleteAutoRemediationRuleHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const ruleId = c.req.param('ruleId') as string

  const deleted = await deleteAutoRemediationRule(c.env, ruleId)

  if (!deleted) {
    return c.json({ success: false, error: 'Rule not found' }, 404)
  }

  await logAudit(c, principal.sub, 'delete_auto_remediation_rule', 'incident', {
    rule_id: ruleId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== Maintenance Windows ====================

export async function listMaintenanceWindowsHandler(c: Context<{ Bindings: Env }>) {
  const status = c.req.query('status')
  const service = c.req.query('service')
  const windows = await listMaintenanceWindows(c.env, status, service)
  return c.json({ success: true, data: windows })
}

export async function getMaintenanceWindowHandler(c: Context<{ Bindings: Env }>) {
  const windowId = c.req.param('windowId') as string
  const window = await getMaintenanceWindow(c.env, windowId)

  if (!window) {
    return c.json({ success: false, error: 'Maintenance window not found' }, 404)
  }

  return c.json({ success: true, data: window })
}

export async function createMaintenanceWindowHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().max(500).optional(),
    services: z.array(z.string().min(1)),
    start_time: z.string(),
    end_time: z.string(),
    timezone: z.string(),
    recurring: z.object({
      frequency: z.enum(['daily', 'weekly', 'monthly']),
      interval: z.number().int().min(1),
      end_date: z.string().optional(),
    }).optional(),
    suppress_alerts: z.boolean(),
    suppress_notifications: z.boolean(),
    auto_detect_incidents: z.boolean(),
  }).parse(await c.req.json())

  const window = await createMaintenanceWindow(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_maintenance_window', 'incident', {
    window_id: window.id,
    name: window.name,
  })

  return c.json({ success: true, data: window }, 201)
}

export async function updateMaintenanceWindowHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const windowId = c.req.param('windowId') as string

  const body = z.object({
    name: z.string().min(1).max(100).optional(),
    description: z.string().max(500).optional(),
    services: z.array(z.string().min(1)).optional(),
    start_time: z.string().optional(),
    end_time: z.string().optional(),
    timezone: z.string().optional(),
    recurring: z.object({
      frequency: z.enum(['daily', 'weekly', 'monthly']),
      interval: z.number().int().min(1),
      end_date: z.string().optional(),
    }).optional(),
    suppress_alerts: z.boolean().optional(),
    suppress_notifications: z.boolean().optional(),
    auto_detect_incidents: z.boolean().optional(),
    status: z.enum(['scheduled', 'active', 'completed', 'cancelled']).optional(),
  }).parse(await c.req.json())

  const window = await updateMaintenanceWindow(c.env, windowId, body)

  if (!window) {
    return c.json({ success: false, error: 'Maintenance window not found' }, 404)
  }

  await logAudit(c, principal.sub, 'update_maintenance_window', 'incident', {
    window_id: windowId,
  })

  return c.json({ success: true, data: window })
}

export async function cancelMaintenanceWindowHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const windowId = c.req.param('windowId') as string

  const cancelled = await cancelMaintenanceWindow(c.env, windowId)

  if (!cancelled) {
    return c.json({ success: false, error: 'Maintenance window not found' }, 404)
  }

  await logAudit(c, principal.sub, 'cancel_maintenance_window', 'incident', {
    window_id: windowId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== Bulk Operations ====================

export async function listBulkOperationsHandler(c: Context<{ Bindings: Env }>) {
  const status = c.req.query('status')
  const ops = await listBulkOperations(c.env, status)
  return c.json({ success: true, data: ops })
}

export async function getBulkOperationHandler(c: Context<{ Bindings: Env }>) {
  const operationId = c.req.param('operationId') as string
  const op = await getBulkOperation(c.env, operationId)

  if (!op) {
    return c.json({ success: false, error: 'Bulk operation not found' }, 404)
  }

  return c.json({ success: true, data: op })
}

export async function createBulkOperationHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    operation_type: z.enum(['assign', 'status_change', 'severity_change', 'tag', 'close', 'escalate']),
    incident_ids: z.array(z.string().min(1)).min(1).max(100),
    payload: z.record(z.string(), z.unknown()),
  }).parse(await c.req.json())

  const op = await createBulkOperation(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_bulk_operation', 'incident', {
    operation_id: op.id,
    operation_type: body.operation_type,
    incident_count: body.incident_ids.length,
  })

  return c.json({ success: true, data: op }, 201)
}

export async function executeBulkOperationHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const operationId = c.req.param('operationId') as string

  const op = await executeBulkOperation(c.env, operationId)

  if (!op) {
    return c.json({ success: false, error: 'Bulk operation not found' }, 404)
  }

  await logAudit(c, principal.sub, 'execute_bulk_operation', 'incident', {
    operation_id: operationId,
    status: op.status,
    success_count: op.results.filter(r => r.success).length,
    failure_count: op.results.filter(r => !r.success).length,
  })

  return c.json({ success: true, data: op })
}

// ==================== SLA Breaches ====================

export async function listSLABreachesHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.query('incident_id')
  const severity = c.req.query('severity') as IncidentSeverity | undefined
  const breaches = await listSLABreaches(c.env, incidentId, severity)
  return c.json({ success: true, data: breaches })
}

export async function acknowledgeSLABreachHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const breachId = c.req.param('breachId') as string

  const breach = await acknowledgeSLABreach(c.env, breachId)

  if (!breach) {
    return c.json({ success: false, error: 'SLA breach not found' }, 404)
  }

  await logAudit(c, principal.sub, 'acknowledge_sla_breach', 'incident', {
    breach_id: breachId,
    incident_id: breach.incident_id,
  })

  return c.json({ success: true, data: breach })
}

// ==================== Analytics ====================

export async function listAnalyticsSnapshotsHandler(c: Context<{ Bindings: Env }>) {
  const period = c.req.query('period')
  const startDate = c.req.query('start_date')
  const endDate = c.req.query('end_date')
  const snapshots = await listAnalyticsSnapshots(c.env, period, startDate, endDate)
  return c.json({ success: true, data: snapshots })
}

export async function generateAnalyticsSnapshotHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    date: z.string(),
    period: z.enum(['daily', 'weekly', 'monthly']),
  }).parse(await c.req.json())

  const snapshot = await generateAnalyticsSnapshot(c.env, body.date, body.period)

  await logAudit(c, principal.sub, 'generate_analytics_snapshot', 'incident', {
    snapshot_id: snapshot.id,
    date: body.date,
    period: body.period,
  })

  return c.json({ success: true, data: snapshot }, 201)
}

// ==================== Webhook Subscriptions ====================

export async function listWebhookSubscriptionsHandler(c: Context<{ Bindings: Env }>) {
  const enabled = c.req.query('enabled') === 'true' ? true : c.req.query('enabled') === 'false' ? false : undefined
  const subs = await listWebhookSubscriptions(c.env, enabled)
  return c.json({ success: true, data: subs })
}

export async function getWebhookSubscriptionHandler(c: Context<{ Bindings: Env }>) {
  const subscriptionId = c.req.param('subscriptionId') as string
  const sub = await getWebhookSubscription(c.env, subscriptionId)

  if (!sub) {
    return c.json({ success: false, error: 'Webhook subscription not found' }, 404)
  }

  return c.json({ success: true, data: sub })
}

export async function createWebhookSubscriptionHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  const body = z.object({
    name: z.string().min(1).max(100),
    description: z.string().max(500).optional(),
    url: z.string().url(),
    secret: z.string().optional(),
    events: z.array(z.enum(['incident.created', 'incident.acknowledged', 'incident.escalated', 'incident.assigned', 'incident.analyzed', 'incident.approved', 'incident.executing', 'incident.resolved', 'incident.failed'])),
    filters: z.object({
      severities: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
      services: z.array(z.string()).optional(),
      tags: z.array(z.string()).optional(),
    }).optional(),
    headers: z.record(z.string(), z.string()).optional(),
    retry_policy: z.object({
      max_retries: z.number().int().min(0).max(10),
      backoff_multiplier: z.number().min(1).max(10),
      initial_delay_ms: z.number().int().min(100).max(60000),
    }),
    enabled: z.boolean(),
  }).parse(await c.req.json())

  const sub = await createWebhookSubscription(c.env, principal, body)

  await logAudit(c, principal.sub, 'create_webhook_subscription', 'incident', {
    subscription_id: sub.id,
    name: sub.name,
    url: sub.url,
  })

  return c.json({ success: true, data: sub }, 201)
}

export async function updateWebhookSubscriptionHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const subscriptionId = c.req.param('subscriptionId') as string

  const body = z.object({
    name: z.string().min(1).max(100).optional(),
    description: z.string().max(500).optional(),
    url: z.string().url().optional(),
    secret: z.string().optional(),
    events: z.array(z.enum(['incident.created', 'incident.acknowledged', 'incident.escalated', 'incident.assigned', 'incident.analyzed', 'incident.approved', 'incident.executing', 'incident.resolved', 'incident.failed'])).optional(),
    filters: z.object({
      severities: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
      services: z.array(z.string()).optional(),
      tags: z.array(z.string()).optional(),
    }).optional(),
    headers: z.record(z.string(), z.string()).optional(),
    retry_policy: z.object({
      max_retries: z.number().int().min(0).max(10),
      backoff_multiplier: z.number().min(1).max(10),
      initial_delay_ms: z.number().int().min(100).max(60000),
    }).optional(),
    enabled: z.boolean().optional(),
  }).parse(await c.req.json())

  const sub = await updateWebhookSubscription(c.env, subscriptionId, body)

  if (!sub) {
    return c.json({ success: false, error: 'Webhook subscription not found' }, 404)
  }

  await logAudit(c, principal.sub, 'update_webhook_subscription', 'incident', {
    subscription_id: subscriptionId,
  })

  return c.json({ success: true, data: sub })
}

export async function deleteWebhookSubscriptionHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const subscriptionId = c.req.param('subscriptionId') as string

  const deleted = await deleteWebhookSubscription(c.env, subscriptionId)

  if (!deleted) {
    return c.json({ success: false, error: 'Webhook subscription not found' }, 404)
  }

  await logAudit(c, principal.sub, 'delete_webhook_subscription', 'incident', {
    subscription_id: subscriptionId,
  })

  return c.json({ success: true } as ApiMessageResponse)
}

// ==================== Snooze ====================

export async function listSnoozesHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.query('incident_id')
  const status = c.req.query('status')
  const snoozes = await listSnoozes(c.env, incidentId, status)
  return c.json({ success: true, data: snoozes })
}

export async function createSnoozeHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    wake_at: z.string(),
    reason: z.string().min(1).max(500),
  }).parse(await c.req.json())

  const snooze = await createSnooze(c.env, incidentId, principal, body.wake_at, body.reason)

  await logAudit(c, principal.sub, 'create_snooze', 'incident', {
    incident_id: incidentId,
    snooze_id: snooze.id,
    wake_at: body.wake_at,
  })

  return c.json({ success: true, data: snooze }, 201)
}

export async function wakeSnoozeHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const snoozeId = c.req.param('snoozeId') as string

  const snooze = await wakeSnooze(c.env, snoozeId, principal)

  if (!snooze) {
    return c.json({ success: false, error: 'Snooze not found or already woke' }, 404)
  }

  await logAudit(c, principal.sub, 'wake_snooze', 'incident', {
    snooze_id: snoozeId,
    incident_id: snooze.incident_id,
  })

  return c.json({ success: true, data: snooze })
}

// ==================== Merge ====================

export async function listMergesHandler(c: Context<{ Bindings: Env }>) {
  const primaryIncidentId = c.req.query('primary_incident_id')
  const merges = await listMerges(c.env, primaryIncidentId)
  return c.json({ success: true, data: merges })
}

export async function createMergeHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    merged_incident_ids: z.array(z.string().min(1)).min(1).max(10),
    reason: z.string().min(1).max(1000),
    preserve_sub_incidents: z.boolean().optional(),
  }).parse(await c.req.json())

  const merge = await createMerge(c.env, incidentId, body.merged_incident_ids, principal, body.reason, body.preserve_sub_incidents)

  await logAudit(c, principal.sub, 'merge_incidents', 'incident', {
    primary_incident_id: incidentId,
    merged_incident_ids: body.merged_incident_ids,
    merge_id: merge.id,
  })

  return c.json({ success: true, data: merge }, 201)
}

// ==================== Split ====================

export async function listSplitsHandler(c: Context<{ Bindings: Env }>) {
  const sourceIncidentId = c.req.query('source_incident_id')
  const splits = await listSplits(c.env, sourceIncidentId)
  return c.json({ success: true, data: splits })
}

export async function createSplitHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const body = z.object({
    new_incidents: z.array(z.object({
      title: z.string().min(1).max(200),
      description: z.string().max(2000).optional(),
      severity: z.enum(['low', 'medium', 'high', 'critical']),
      evidence_ids: z.array(z.string()).optional(),
    })).min(2).max(5),
    reason: z.string().min(1).max(1000),
  }).parse(await c.req.json())

  const split = await createSplit(c.env, incidentId, body.new_incidents, principal, body.reason)

  await logAudit(c, principal.sub, 'split_incident', 'incident', {
    source_incident_id: incidentId,
    new_incident_count: body.new_incidents.length,
    split_id: split.id,
  })

  return c.json({ success: true, data: split }, 201)
}

// ==================== Recurrence ====================

export async function listRecurrencesHandler(c: Context<{ Bindings: Env }>) {
  const incidentId = c.req.query('incident_id')
  const recurrences = await listRecurrences(c.env, incidentId)
  return c.json({ success: true, data: recurrences })
}

export async function detectRecurrenceHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const incidentId = c.req.param('id') as string

  const incident = await getIncident(c.env, incidentId)

  if (!incident) {
    return c.json({ success: false, error: 'Incident not found' } as ApiErrorResponse, 404)
  }

  const recurrence = await detectRecurrence(c.env, incident)

  await logAudit(c, principal.sub, 'detect_recurrence', 'incident', {
    incident_id: incidentId,
    recurrence_detected: !!recurrence,
  })

  return c.json({ success: true, data: recurrence })
}

export async function markRootCauseResolvedHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const recurrenceId = c.req.param('recurrenceId') as string

  const recurrence = await markRootCauseResolved(c.env, recurrenceId)

  if (!recurrence) {
    return c.json({ success: false, error: 'Recurrence not found' }, 404)
  }

  await logAudit(c, principal.sub, 'mark_root_cause_resolved', 'incident', {
    recurrence_id: recurrenceId,
    incident_id: recurrence.incident_id,
  })

  return c.json({ success: true, data: recurrence })
}
