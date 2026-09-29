import type { ZodIssue } from 'zod'

// Cloudflare Workers 环境类型定义

export interface Env {
  // 环境变量
  ENVIRONMENT: 'development' | 'production'
  DEVELOPER_MODE?: 'true' | 'false'
  JWT_SECRET: string
  JWT_EXPIRE: string
  API_KEY_SALT: string
  APP_VERSION?: string
  BUILD_SHA?: string
  KUBERNETES_API_SERVER?: string
  KUBERNETES_NAMESPACE?: string

  // D1 数据库
  DB: D1Database

  // KV 命名空间
  KV: KVNamespace

  // R2 存储桶
  R2: R2Bucket

  // Workers AI
  AI: Ai

  // Analytics Engine
  ANALYTICS?: AnalyticsEngineDataset

  // Vectorize 向量数据库 (可选 - 需要先创建索引)
  VECTORIZE?: VectorizeIndex

  // Durable Objects - 暂时禁用
  // WEBSOCKET_SERVER: DurableObjectNamespace
}

// 用户类型
export interface User {
  id: number
  email: string
  password_hash?: string
  role: 'admin' | 'operator' | 'viewer'
  auth_provider: 'local' | 'github' | 'google' | 'cloudflare'
  enabled: boolean
  last_login_at?: string
  created_at: string
  updated_at: string
}

export interface ApiSuccessResponse<T> extends ApiResponse<T> {
  success: true
  data: T
}

export interface ApiErrorResponse extends ApiResponse<never> {
  success: false
  error: string
}

export interface ApiMessageResponse extends ApiResponse<never> {
  success: true
  message: string
}

export type PasswordStrength = 'weak' | 'medium' | 'strong' | 'very-strong'

export interface AuthUserSummary {
  id: number
  email: string
  role: User['role']
}

export interface AuthLockoutDetails {
  locked_until?: string
  retry_after?: number
}

export interface AuthFailedLoginDetails extends AuthLockoutDetails {
  remaining_attempts?: number
  account_locked?: boolean
}

export interface AuthLoginData {
  access_token: string
  refresh_token: string
  token_type: 'Bearer'
  expires_in: number
  user: AuthUserSummary
}

export interface AuthRefreshData {
  access_token: string
  token_type: 'Bearer'
  expires_in: number
}

export interface AuthRegisterData {
  id: number
  email: string
  role: User['role']
  created_at: string
}

export interface AuthMeData {
  id: number
  email: string
  role: User['role']
  auth_provider: User['auth_provider']
  last_login_at?: string
  created_at: string
}

export interface AuthLoginResponse extends ApiSuccessResponse<AuthLoginData> {
  data: AuthLoginData
}

export interface AuthRefreshResponse extends ApiSuccessResponse<AuthRefreshData> {
  data: AuthRefreshData
}

export interface AuthRegisterResponse extends ApiSuccessResponse<AuthRegisterData> {
  data: AuthRegisterData
}

export interface AuthMeResponse extends ApiSuccessResponse<AuthMeData> {
  data: AuthMeData
}

export interface AuthLogoutResponse extends ApiMessageResponse {
  message: string
}

export interface AuthLockoutResponse extends ApiErrorResponse, AuthLockoutDetails {}

export interface AuthInvalidCredentialsResponse extends ApiErrorResponse, AuthFailedLoginDetails {}

export interface SchemaValidationErrorResponse extends ApiErrorResponse {
  details: ZodIssue[]
}

export interface AuthSchemaValidationErrorResponse extends SchemaValidationErrorResponse {}

export interface AuthPasswordValidationErrorResponse extends ApiErrorResponse {
  details: string[]
  strength: PasswordStrength
}

export interface AgentQueuedCommand {
  id: string
  type: string
  payload: Record<string, unknown>
  timeout?: number
  created_at?: string
}

export interface AgentRegisterResponseData {
  agent_id: string
  node_id: number
  heartbeat_interval: number
  metrics_interval: number
}

export interface AgentHeartbeatResponseData {
  received: true
  commands: AgentQueuedCommand[]
}

export interface AgentStoreMetricsResponseData {
  stored: true
}

export interface AgentCommandResultResponseData {
  received: true
}

export interface AgentSendCommandResponseData {
  command_id: string
  status: 'queued'
}

export interface AgentMetricSample {
  timestamp: number
  [key: string]: unknown
}

export interface AgentMetricsResponseData {
  agent_id: string
  range?: string
  metrics: AgentMetricSample[]
}

export interface AgentRegisterResponse extends ApiSuccessResponse<AgentRegisterResponseData> {
  data: AgentRegisterResponseData
}

export interface AgentHeartbeatResponse extends ApiSuccessResponse<AgentHeartbeatResponseData> {
  data: AgentHeartbeatResponseData
}

export interface AgentStoreMetricsResponse extends ApiSuccessResponse<AgentStoreMetricsResponseData> {
  data: AgentStoreMetricsResponseData
}

export interface AgentCommandResultResponse extends ApiSuccessResponse<AgentCommandResultResponseData> {
  data: AgentCommandResultResponseData
}

export interface AgentSendCommandResponse extends ApiSuccessResponse<AgentSendCommandResponseData> {
  data: AgentSendCommandResponseData
}

export interface AgentMetricsResponse extends ApiSuccessResponse<AgentMetricsResponseData> {
  data: AgentMetricsResponseData
}

export interface BatchOperationResult {
  path: string
  status: number
  success: boolean
  data?: unknown
  error?: string
}

export interface BatchOperationsSummary {
  total: number
  successful: number
  failed: number
}

export interface BatchOperationsResponse {
  success: boolean
  results: BatchOperationResult[]
  summary: BatchOperationsSummary
}

export interface BatchNodeStatusResult {
  node_id: number
  success: boolean
  error?: string
}

export interface BatchNodeStatusResponse {
  success: boolean
  action: 'start' | 'stop' | 'restart'
  results: BatchNodeStatusResult[]
}

export interface NodeSummaryResponseData {
  items: Node[]
  total: number
  page: number
  per_page: number
  total_pages: number
}

export interface NodeActionResponseData {
  id: string
  status: 'starting' | 'stopping' | 'restarting'
}

export interface NodeStatsResponseData {
  node_id: number
  status: Node['status']
  uptime: number
  cpu_usage: number
  memory_usage: number
  disk_usage: number
  network: {
    upload: number
    download: number
  }
  connections: number
  users: number
  last_updated: string
}

export interface NodeSyncResponseData {
  node_id: number
  status: string
  synced_at: string
}

export interface NodeBulkActionResult {
  id: number
  success: boolean
  error?: string
}

export interface NodeBulkActionResponse {
  success: boolean
  data: {
    action: 'start' | 'stop' | 'restart' | 'delete'
    results: NodeBulkActionResult[]
  }
}

export interface NodeSummaryResponse extends ApiResponse<NodeSummaryResponseData> {
  data: NodeSummaryResponseData
}

export interface NodeGetResponse extends ApiResponse<Node> {
  data: Node
}

export interface NodeCreateResponse extends ApiResponse<Node> {
  data: Node
}

export interface NodeUpdateResponse extends ApiResponse<Node> {
  data: Node
}

export interface NodeDeleteResponse extends ApiMessageResponse {
  message: string
}

export interface NodeActionResponse extends ApiResponse<NodeActionResponseData> {
  message: string
  data: NodeActionResponseData
}

export interface NodeStatsResponse extends ApiResponse<NodeStatsResponseData> {
  data: NodeStatsResponseData
}

export interface NodeSyncResponse extends ApiResponse<NodeSyncResponseData> {
  message: string
  data: NodeSyncResponseData
}

export interface NodeConnectionTestResponseData {
  node_id: number
  host: string
  port: number
  reachable: boolean
  response_time: number | null
  error: string | null
  tested_at: string
}

export interface NodeConnectionTestResponse extends ApiResponse<NodeConnectionTestResponseData> {
  data: NodeConnectionTestResponseData
}

export interface NodeLogsResponseData {
  node_id: number
  node_name: string
  logs: Array<{
    timestamp: string
    level: string
    message: string
  }>
  total: number
}

export interface NodeLogsResponse extends ApiResponse<NodeLogsResponseData> {
  data: NodeLogsResponseData
}

export interface NodeBulkActionResponse extends ApiResponse<{ action: 'start' | 'stop' | 'restart' | 'delete'; results: NodeBulkActionResult[] }> {
  data: {
    action: 'start' | 'stop' | 'restart' | 'delete'
    results: NodeBulkActionResult[]
  }
}

// JWT Payload
export interface JWTPayload {
  sub: number
  email: string
  role: string
  iat: number
  exp: number
}

export interface AuthPrincipal extends JWTPayload {
  kind: 'user' | 'api_key'
  auth_method: 'jwt' | 'api_key'
  token_id?: number
  token_name?: string
}

export interface RuntimeServiceHealthCheck {
  name: string
  status: 'healthy' | 'degraded' | 'unhealthy'
  latency: number
  message?: string
  lastCheck: string
}

export interface RuntimeServiceChecks {
  database: RuntimeServiceHealthCheck
  kv: RuntimeServiceHealthCheck
  r2: RuntimeServiceHealthCheck
}

export interface HealthResponse {
  status: 'healthy'
  version: string
  build_sha: string
  timestamp: string
  environment: Env['ENVIRONMENT']
}

export interface ReadinessResponse {
  status: 'ready' | 'degraded'
  version: string
  build_sha: string
  checks: RuntimeServiceChecks
  timestamp: string
}

export interface ServiceErrorResponse {
  status: 'error'
  error: string
}

export interface DashboardPanel {
  id: string
  title: string
  type: 'line' | 'bar' | 'pie' | 'stat' | 'table'
  metrics: string[]
  width: number
  height: number
  x: number
  y: number
}

export interface DashboardConfig {
  id: string
  name: string
  panels: DashboardPanel[]
  refreshInterval: number
  timeRange: string
}

export interface DeveloperActor {
  id: number
  email: string
  role: User['role']
  auth_method: AuthPrincipal['auth_method']
}

export interface DeveloperModeStatusData {
  developer_mode: boolean
  environment: Env['ENVIRONMENT']
  version: string
  build_sha: string
  request_id: string
  actor: DeveloperActor
  capabilities: string[]
}

export interface DeveloperDiagnosticsData {
  runtime: {
    environment: Env['ENVIRONMENT']
    developer_mode: boolean
    version: string
    build_sha: string
    has_analytics: boolean
    has_vectorize: boolean
    has_kubernetes_api: boolean
  }
  routing: {
    public_health_endpoints: string[]
    internal_debug_endpoints: string[]
  }
  security: {
    admin_only: boolean
    jwt_only: boolean
    audited: boolean
    hides_when_disabled: boolean
  }
}

export interface DeveloperReadinessSummaryResponseData {
  request_id: string
  environment: Env['ENVIRONMENT']
  version: string
  build_sha: string
  actor: DeveloperActor
  readiness: {
    status: 'ready' | 'degraded'
    checks: RuntimeServiceChecks
  }
  manifest: DeveloperReadinessSummary
}

export interface DeveloperFixtureCatalogData {
  fixtures: Array<{
    key: string
    status: 'available' | 'planned'
    description: string
  }>
  notes: string[]
}

export interface DeveloperModeStatusResponse extends ApiResponse<DeveloperModeStatusData> {
  data: DeveloperModeStatusData
}

export interface DeveloperDiagnosticsResponse extends ApiResponse<DeveloperDiagnosticsData> {
  data: DeveloperDiagnosticsData
}

export interface DeveloperReadinessSummaryResponse extends ApiResponse<DeveloperReadinessSummaryResponseData> {
  data: DeveloperReadinessSummaryResponseData
}

export interface DeveloperFixtureCatalogResponse extends ApiResponse<DeveloperFixtureCatalogData> {
  data: DeveloperFixtureCatalogData
}

export interface DashboardOverviewResponseData extends ApiResponse<DashboardOverviewData> {
  cached: boolean
  data: DashboardOverviewData
}

export interface DashboardOverviewData {
  nodes: {
    total: number
    online: number
    offline: number
    maintenance: number
  }
  users: {
    total: number
  }
  activity: {
    last_24h: number
  }
  developer_readiness_summary: DeveloperReadinessSummary | null
  timestamp: string
}

export type EndpointReadinessTier = 'verified' | 'seeded' | 'diagnostic' | 'manual' | 'inventory'
export type EndpointExecutionMode = 'automated' | 'fixture-backed' | 'diagnostic' | 'manual' | 'inventory'
export type EndpointAuthRequirement = 'public' | 'user' | 'operator' | 'admin'

export interface EndpointManifestEntry {
  id: string
  family: string
  subgroup?: string
  name: string
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
  auth: EndpointAuthRequirement
  readiness: EndpointReadinessTier
  execution_mode: EndpointExecutionMode
  expected_status: number
  fixture_keys?: string[]
  manual_notes?: string
}

export interface DeveloperReadinessEndpointSummary {
  id: string
  family: string
  subgroup?: string
  name: string
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
  auth: EndpointAuthRequirement
  readiness: EndpointReadinessTier
  execution_mode: EndpointExecutionMode
  fixture_keys: string[]
}

export interface DeveloperReadinessFixtureCoverage {
  total_endpoints: number
  fixture_key_counts: Record<string, number>
  endpoints: DeveloperReadinessEndpointSummary[]
}

export interface DeveloperReadinessSummary {
  manifest_total: number
  readiness_counts: Record<EndpointReadinessTier, number>
  execution_mode_counts: Record<EndpointExecutionMode, number>
  ready_endpoints: DeveloperReadinessEndpointSummary[]
  manual_endpoints: DeveloperReadinessEndpointSummary[]
  fixture_coverage: DeveloperReadinessFixtureCoverage
}

// 节点类型
export interface Node {
  id: number
  name: string
  host: string
  port: number
  status: 'online' | 'offline' | 'maintenance'
  last_seen?: string
  config?: string
  agent_id?: string
  agent_secret?: string
  agent_version?: string
  os?: string
  arch?: string
  cpu_count?: number
  memory_gb?: number
  disk_gb?: number
  created_at: string
  updated_at: string
}

// Playbook 类型
export interface Playbook {
  id: number
  name: string
  storage_key: string
  description?: string
  category?: string
  source?: string
  github_repo?: string
  github_path?: string
  version?: string
  variables?: string
  author?: string
  tags?: string
  created_at: string
  updated_at: string
}

// 任务类型
export interface Task {
  id: number
  task_id: string
  playbook_id: number
  playbook_name: string
  status: 'pending' | 'running' | 'success' | 'failed' | 'cancelled'
  trigger_type: 'manual' | 'scheduled' | 'webhook' | 'api'
  triggered_by?: number
  target_nodes?: string
  variables?: string
  result?: string
  error?: string
  started_at?: string
  completed_at?: string
  created_at: string
}

// 任务日志类型
export interface TaskLog {
  id: number
  task_id: string
  node_id?: number
  node_name?: string
  level: 'debug' | 'info' | 'warning' | 'error'
  message: string
  metadata?: string
  created_at: string
}

export interface TaskListItem extends Task {
  category?: string
  triggered_by_email?: string
}

export interface TaskListResponseData {
  items: TaskListItem[]
  total: number
  page: number
  per_page: number
  total_pages: number
}

export interface TaskDetailResponseData extends TaskListItem {
  playbook_variables?: string
}

export interface TaskCreateResponseData {
  task_id: string
  status: 'pending'
  message: string
}

export interface TaskRetryResponseData {
  task_id: string
  status: 'pending'
  message: string
}

export interface TaskListResponse extends ApiSuccessResponse<TaskListResponseData> {
  data: TaskListResponseData
}

export interface TaskDetailResponse extends ApiSuccessResponse<TaskDetailResponseData> {
  data: TaskDetailResponseData
}

export interface TaskCreateResponse extends ApiSuccessResponse<TaskCreateResponseData> {
  data: TaskCreateResponseData
}

export interface TaskRetryResponse extends ApiSuccessResponse<TaskRetryResponseData> {
  data: TaskRetryResponseData
}

export interface TaskLogsResponse extends ApiSuccessResponse<TaskLog[]> {
  data: TaskLog[]
}

export interface TaskCancelResponse extends ApiMessageResponse {
  message: string
}

// 调度类型
export interface Schedule {
  id: number
  name: string
  playbook_id: number
  playbook_name: string
  cron: string
  timezone?: string
  target_nodes?: string
  variables?: string
  enabled: boolean
  last_run?: string
  next_run?: string
  last_task_id?: string
  created_by?: number
  created_at: string
  updated_at: string
}

// 节点组类型
export interface NodeGroup {
  id: number
  name: string
  description?: string
  parent_id?: number
  created_at: string
}

// 插件类型
export interface Plugin {
  id: number
  name: string
  display_name?: string
  version?: string
  description?: string
  author?: string
  type?: string
  enabled: boolean
  config?: string
  permissions?: string
  installed_at: string
  updated_at?: string
}

// 通知类型
export interface Notification {
  id: number
  user_id: number
  type: 'info' | 'success' | 'warning' | 'error' | 'task' | 'system'
  title: string
  message?: string
  resource_type?: string
  resource_id?: string
  read: boolean
  action_url?: string
  created_at: string
}

export interface NotificationListResponseData {
  items: Notification[]
  total: number
  page: number
  per_page: number
  total_pages: number
  unread_count: number
}

export interface NotificationCreateResponse extends ApiSuccessResponse<Notification> {
  data: Notification
}

export interface NotificationUnreadCountResponseData {
  unread_count: number
}

export interface NotificationListResponse extends ApiSuccessResponse<NotificationListResponseData> {
  data: NotificationListResponseData
}

export interface NotificationUnreadCountResponse extends ApiSuccessResponse<NotificationUnreadCountResponseData> {
  data: NotificationUnreadCountResponseData
}

// 审计日志类型
export interface AuditLog {
  id: number
  tenant_id?: number
  user_id?: number
  user_email?: string
  action: string
  resource: string
  resource_id?: string
  ip?: string
  user_agent?: string
  status: 'success' | 'failure' | 'pending'
  details?: string
  created_at: string
}

export interface AuditActionGroup {
  category: string
  actions: string[]
}

export interface AuditLogListResponseData {
  items: AuditLog[]
  total: number
  page: number
  per_page: number
  total_pages: number
}

export interface AuditStatsData {
  total: number
  byAction: Array<{ action: string; count: number }>
  byUser: Array<{ user_id: number; user_email: string; count: number }>
  byResource: Array<{ resource: string; count: number }>
  failures: number
}

export interface AuditCleanupResponseData {
  deleted: number
}

export interface AuditLogListResponse extends ApiSuccessResponse<AuditLogListResponseData> {
  data: AuditLogListResponseData
}

export interface AuditLogDetailResponse extends ApiSuccessResponse<AuditLog> {
  data: AuditLog
}

export interface AuditStatsResponse extends ApiSuccessResponse<AuditStatsData> {
  data: AuditStatsData
}

export interface AuditCleanupResponse extends ApiSuccessResponse<AuditCleanupResponseData> {
  data: AuditCleanupResponseData
}

export interface AuditActionsResponse extends ApiSuccessResponse<AuditActionGroup[]> {
  data: AuditActionGroup[]
}

export interface SIEMConfigResponseData {
  enabled: boolean
  webhook_url: string
  api_key?: string
  format: 'json' | 'cef' | 'syslog'
  filters?: string[]
}

export interface SIEMConfigResponse extends ApiSuccessResponse<SIEMConfigResponseData | null> {
  data: SIEMConfigResponseData | null
}

// Incident workflow types
export type IncidentSeverity = 'low' | 'medium' | 'high' | 'critical'
export type IncidentStatus = 'open' | 'analyzed' | 'approved' | 'executing' | 'resolved' | 'failed'
export type IncidentActionType = 'scale_policy' | 'restart_deployment' | 'scale_deployment'

export interface IncidentEvidence {
  type: 'log' | 'metric' | 'task' | 'node' | 'alert' | 'service' | 'manual'
  source: string
  content: string
}

export interface IncidentRecommendation {
  id: string
  title: string
  description: string
  action_type?: IncidentActionType
  action_ref?: string
  confidence?: number
}

export interface IncidentLink {
  kind: 'task' | 'node' | 'scaling_policy' | 'deployment' | 'runbook' | 'alert' | 'playbook'
  id: string
  name?: string
  href?: string
  relationship?: 'caused_by' | 'related_to' | 'resolves' | 'investigates'
}

export interface IncidentExecutionResult {
  backend: 'autoscaling' | 'kubernetes'
  success: boolean
  message?: string
  operation?: string
  target?: {
    kind: 'scaling_policy' | 'deployment'
    id: string
    name?: string
    namespace?: string
  }
  details?: Record<string, unknown>
}

export interface IncidentSummary {
  id: string
  title: string
  summary?: string
  status: IncidentStatus
  severity: IncidentSeverity
  source: string
  requested_via: 'jwt' | 'api_key'
  action_type?: IncidentActionType
  action_ref?: string
  approved_by?: number
  correlation_id: string
  links?: IncidentLink[]
  tags: string[]
  created_at: string
  updated_at: string
}

export interface IncidentDetail extends IncidentSummary {
  requested_by: number
  requested_by_email?: string
  approved_at?: string
  execution_id?: string
  evidence: IncidentEvidence[]
  recommendations: IncidentRecommendation[]
  analysis?: Record<string, unknown>
  execution_result?: IncidentExecutionResult
  acknowledged_by?: number
  acknowledged_at?: string
  escalated_from?: string
  escalated_at?: string
  sla_deadline?: string
  assignee_id?: number
  assignee_email?: string
  assigned_at?: string
}

export interface IncidentRecord {
  id: string
  title: string
  summary?: string
  status: IncidentStatus
  severity: IncidentSeverity
  source: string
  correlation_id: string
  requested_by: number
  requested_by_email?: string
  requested_via: 'jwt' | 'api_key'
  approved_by?: number
  approved_at?: string
  execution_id?: string
  action_type?: IncidentActionType
  action_ref?: string
  evidence: IncidentEvidence[]
  recommendations: IncidentRecommendation[]
  links?: IncidentLink[]
  analysis?: Record<string, unknown>
  execution_result?: IncidentExecutionResult
  tags: string[]
  acknowledged_by?: number
  acknowledged_at?: string
  escalated_from?: string
  escalated_at?: string
  sla_deadline?: string
  assignee_id?: number
  assignee_email?: string
  assigned_at?: string
  duplicate_of?: string
  duplicate_count?: number
  suppressed_by?: SuppressionMatch
  created_at: string
  updated_at: string
}

export type IncidentTimelineEventType =
  | 'created'
  | 'acknowledged'
  | 'escalated'
  | 'assigned'
  | 'merged'
  | 'analyzed'
  | 'approved'
  | 'executing'
  | 'resolved'
  | 'failed'
  | 'evidence_added'
  | 'comment'
  | 'severity_upgraded'
  | 'link_added'
  | 'runbook_executed'
  | 'status_changed'

export interface IncidentTimelineEvent {
  id: string
  incident_id: string
  type: IncidentTimelineEventType
  timestamp: string
  actor?: {
    user_id: number
    email?: string
    role?: string
  }
  summary: string
  details?: Record<string, unknown>
  metadata?: Record<string, unknown>
}

export interface IncidentTimeline {
  incident_id: string
  correlation_id: string
  events: IncidentTimelineEvent[]
  total_events: number
}

// Incident Comment types
export interface IncidentComment {
  id: string
  incident_id: string
  author_id: number
  author_email?: string
  author_role?: string
  content: string
  visibility: 'public' | 'internal'
  created_at: string
  updated_at: string
}

export interface IncidentCommentInput {
  content: string
  visibility?: 'public' | 'internal'
}

// Governance Policy types
export interface GovernancePolicyRule {
  id: string
  name: string
  description?: string
  enabled: boolean
  conditions: {
    severity?: IncidentSeverity[]
    action_types?: IncidentActionType[]
    sources?: string[]
  }
  effect: 'allow' | 'deny'
  principals: {
    roles?: ('admin' | 'operator' | 'viewer')[]
    user_ids?: number[]
  }
  priority: number
  created_at: string
  updated_at: string
}

export interface GovernancePolicy {
  id: string
  name: string
  description?: string
  version: number
  enabled: boolean
  default_effect: 'allow' | 'deny'
  rules: GovernancePolicyRule[]
  created_by: number
  created_at: string
  updated_at: string
}

export interface GovernanceEvaluation {
  allowed: boolean
  matched_rules: string[]
  evaluation_time: string
  policy_id: string
  policy_version: number
}

// Webhook types
export type WebhookEventType =
  | 'incident.created'
  | 'incident.acknowledged'
  | 'incident.escalated'
  | 'incident.assigned'
  | 'incident.analyzed'
  | 'incident.approved'
  | 'incident.executing'
  | 'incident.resolved'
  | 'incident.failed'

export interface WebhookEndpoint {
  id: string
  name: string
  url: string
  secret?: string
  events: WebhookEventType[]
  enabled: boolean
  headers?: Record<string, string>
  created_by: number
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: string
  webhook_id: string
  event_type: WebhookEventType
  payload: Record<string, unknown>
  response_status?: number
  response_body?: string
  delivered_at?: string
  attempts: number
  last_attempt_at?: string
  success: boolean
  created_at: string
}

// Suppression Rule types
export interface SuppressionRule {
  id: string
  name: string
  description?: string
  enabled: boolean
  conditions: {
    severity?: IncidentSeverity[]
    source?: string[]
    action_type?: IncidentActionType[]
    title_pattern?: string
    correlation_id_pattern?: string
  }
  duration_minutes: number
  created_by: number
  created_at: string
  updated_at: string
  expires_at?: string
}

export interface SuppressionMatch {
  rule_id: string
  rule_name: string
  matched_at: string
  expires_at: string
}

// Deduplication config
export interface DeduplicationConfig {
  window_minutes: number
  max_duplicates: number
  group_by: ('correlation_id' | 'source' | 'title')[]
}

// Notification Rule types for incidents
export interface NotificationRule {
  id: string
  name: string
  description?: string
  enabled: boolean
  conditions: {
    severity?: IncidentSeverity[]
    source?: string[]
    action_type?: IncidentActionType[]
    status?: IncidentStatus[]
    tags?: string[]
  }
  channels: ('email' | 'webhook' | 'slack')[]
  recipients: string[]
  template?: string
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Report types
export interface IncidentTrend {
  date: string
  total: number
  by_severity: Record<IncidentSeverity, number>
  by_status: Record<IncidentStatus, number>
  resolved_count: number
  avg_resolution_time_minutes: number
}

export interface IncidentReport {
  period: {
    start: string
    end: string
  }
  summary: {
    total_incidents: number
    open_incidents: number
    resolved_incidents: number
    avg_resolution_time_minutes: number
    sla_breach_count: number
    sla_compliance_rate: number
  }
  by_severity: Record<IncidentSeverity, { count: number; percentage: number }>
  by_status: Record<IncidentStatus, { count: number; percentage: number }>
  by_source: Array<{ source: string; count: number; percentage: number }>
  by_action_type: Array<{ action_type: string; count: number; success_rate: number }>
  trends: IncidentTrend[]
  top_tags: Array<{ tag: string; count: number }>
  top_sources: Array<{ source: string; count: number }>
  mttr_by_severity: Record<IncidentSeverity, number>
}

// Incident Template types
export interface IncidentTemplate {
  id: string
  name: string
  description?: string
  category?: string
  title_template: string
  summary_template?: string
  default_severity: IncidentSeverity
  default_source: string
  default_action_type?: IncidentActionType
  default_action_ref?: string
  default_tags: string[]
  evidence_templates: Array<{
    type: IncidentEvidence['type']
    source_template: string
    content_template?: string
  }>
  created_by: number
  created_at: string
  updated_at: string
}

// Automation Rule types
export type AutomationTrigger =
  | 'incident.created'
  | 'incident.acknowledged'
  | 'incident.escalated'
  | 'incident.analyzed'
  | 'incident.approved'
  | 'incident.resolved'
  | 'incident.failed'
  | 'sla_breach'
  | 'duplicate_detected'

export type AutomationAction =
  | { type: 'assign'; assignee_id: number }
  | { type: 'escalate'; target_severity: IncidentSeverity }
  | { type: 'add_tags'; tags: string[] }
  | { type: 'notify'; channels: string[]; recipients: string[] }
  | { type: 'execute_runbook'; playbook_id: number }
  | { type: 'set_sla'; minutes: number }
  | { type: 'add_comment'; content: string }

export interface AutomationRule {
  id: string
  name: string
  description?: string
  enabled: boolean
  trigger: AutomationTrigger
  conditions: {
    severity?: IncidentSeverity[]
    source?: string[]
    action_type?: IncidentActionType[]
    tags?: string[]
    time_range?: { start_hour: number; end_hour: number }
  }
  actions: AutomationAction[]
  priority: number
  created_by: number
  created_at: string
  updated_at: string
}

// Post-mortem types
export interface IncidentPostMortem {
  id: string
  incident_id: string
  title: string
  summary: string
  timeline: Array<{
    timestamp: string
    event: string
    details?: string
  }>
  root_cause: string
  contributing_factors: string[]
  impact: {
    users_affected?: number
    duration_minutes: number
    services_affected: string[]
  }
  resolution: string
  lessons_learned: string[]
  action_items: Array<{
    id: string
    description: string
    owner?: string
    status: 'open' | 'in_progress' | 'completed'
    due_date?: string
  }>
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Metrics Dashboard
export interface IncidentDashboardMetrics {
  current: {
    open_incidents: number
    critical_incidents: number
    unassigned_incidents: number
    sla_breach_count: number
    avg_age_minutes: number
  }
  last_24h: {
    created: number
    resolved: number
    mttr_minutes: number
    by_severity: Record<IncidentSeverity, number>
    by_source: Array<{ source: string; count: number }>
  }
  last_7d: {
    created: number
    resolved: number
    mttr_minutes: number
    sla_compliance_rate: number
    trend: Array<{ date: string; created: number; resolved: number }>
  }
  last_30d: {
    created: number
    resolved: number
    mttr_minutes: number
    top_sources: Array<{ source: string; count: number; percentage: number }>
    top_assignees: Array<{ user_id: number; email: string; count: number }>
  }
}

// Incident Correlation
export interface IncidentCorrelation {
  incident_id: string
  related_incidents: Array<{
    id: string
    title: string
    correlation_score: number
    shared_attributes: string[]
    time_proximity_minutes: number
  }>
  pattern_match?: {
    pattern_type: 'recurring' | 'cascading' | 'related_service'
    description: string
    confidence: number
  }
}

// Incident Watch/Subscription
export interface IncidentWatch {
  id: string
  incident_id: string
  user_id: number
  user_email: string
  notify_on: IncidentTimelineEventType[]
  created_at: string
}

// External Ticket Integration
export interface ExternalTicket {
  id: string
  incident_id: string
  system: 'jira' | 'servicenow' | 'zendesk' | 'linear'
  ticket_id: string
  ticket_url: string
  status: string
  synced_at: string
  created_by: number
  created_at: string
}

// Response Playbook types
export interface ResponsePlaybookStep {
  id: string
  order: number
  title: string
  description?: string
  action: 'manual' | 'automated' | 'approval'
  automated_action?: {
    type: 'run_playbook' | 'restart_deployment' | 'scale_deployment' | 'execute_webhook' | 'notify'
    ref: string
    params?: Record<string, unknown>
  }
  estimated_duration_minutes?: number
  required_role?: 'admin' | 'operator' | 'viewer'
  checklist?: string[]
}

export interface ResponsePlaybook {
  id: string
  name: string
  description?: string
  category?: string
  trigger_conditions: {
    severity?: IncidentSeverity[]
    source?: string[]
    action_type?: IncidentActionType[]
    title_pattern?: string
    tags?: string[]
  }
  steps: ResponsePlaybookStep[]
  auto_trigger: boolean
  estimated_total_duration_minutes?: number
  version: number
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

export interface ResponsePlaybookExecution {
  id: string
  incident_id: string
  playbook_id: string
  playbook_version: number
  status: 'running' | 'completed' | 'paused' | 'failed' | 'cancelled'
  current_step: number
  completed_steps: string[]
  started_at: string
  completed_at?: string
  step_results: Record<string, {
    status: 'pending' | 'running' | 'completed' | 'skipped' | 'failed'
    started_at?: string
    completed_at?: string
    result?: Record<string, unknown>
    error?: string
    actor?: { user_id: number; email: string }
  }>
}

// Custom Field types
export interface CustomFieldDefinition {
  id: string
  name: string
  key: string
  type: 'text' | 'number' | 'boolean' | 'select' | 'multiselect' | 'date' | 'datetime' | 'user' | 'url'
  required: boolean
  default_value?: string | number | boolean | string[]
  options?: string[] // for select/multiselect
  validation?: {
    min?: number
    max?: number
    pattern?: string
    min_length?: number
    max_length?: number
  }
  description?: string
  category?: string
  order: number
  created_by: number
  created_at: string
  updated_at: string
}

export interface IncidentCustomFieldValue {
  incident_id: string
  field_id: string
  value: string | number | boolean | string[] | null
  updated_by: number
  updated_at: string
}

// Incident Export types
export interface IncidentExportOptions {
  format: 'json' | 'csv'
  include_evidence: boolean
  include_timeline: boolean
  include_comments: boolean
  include_recommendations: boolean
  include_links: boolean
  include_postmortem: boolean
  date_range?: {
    start: string
    end: string
  }
  filters?: {
    status?: IncidentStatus[]
    severity?: IncidentSeverity[]
    source?: string[]
  }
}

export interface IncidentExportResult {
  id: string
  format: 'json' | 'csv'
  status: 'pending' | 'processing' | 'completed' | 'failed'
  total_incidents: number
  download_url?: string
  error?: string
  created_at: string
  completed_at?: string
  expires_at?: string
}

// AI Analysis types
export interface AIRootCauseAnalysis {
  incident_id: string
  analysis_type: 'root_cause' | 'impact' | 'similarity' | 'prediction'
  generated_at: string
  model: string
  summary: string
  root_causes: Array<{
    category: string
    description: string
    confidence: number
    evidence: string[]
    suggested_actions: string[]
  }>
  impact_analysis: {
    affected_services: string[]
    affected_users_estimate?: number
    business_impact: 'low' | 'medium' | 'high' | 'critical'
    blast_radius: string
  }
  similar_incidents: Array<{
    incident_id: string
    title: string
    similarity_score: number
    shared_patterns: string[]
  }>
  predictions: Array<{
    type: 'escalation' | 'cascading' | 'recovery_time'
    prediction: string
    confidence: number
    factors: string[]
  }>
  recommendations: Array<{
    priority: 'immediate' | 'short_term' | 'long_term'
    action: string
    rationale: string
    automated: boolean
  }>
}

// Incident Aggregation types
export interface IncidentAggregation {
  id: string
  name: string
  correlation_id: string
  incident_ids: string[]
  primary_incident_id: string
  status: 'active' | 'resolved'
  severity: IncidentSeverity
  created_at: string
  resolved_at?: string
  total_incidents: number
  aggregate_tags: string[]
  aggregate_sources: string[]
}

// Incident War Room types
export interface WarRoomParticipant {
  user_id: number
  email: string
  role: 'commander' | 'responder' | 'observer'
  joined_at: string
}

export interface IncidentWarRoom {
  id: string
  incident_id: string
  status: 'active' | 'paused' | 'closed'
  commander_id: number
  commander_email?: string
  participants: WarRoomParticipant[]
  chat_messages: Array<{
    id: string
    user_id: number
    user_email: string
    message: string
    timestamp: string
    system?: boolean
  }>
  shared_resources: Array<{
    type: 'link' | 'document' | 'dashboard' | 'log'
    title: string
    url: string
    added_by: number
    added_at: string
  }>
  created_at: string
  closed_at?: string
}

// Incident Schedule/Review types
export interface IncidentReview {
  id: string
  incident_id: string
  scheduled_at: string
  status: 'scheduled' | 'completed' | 'cancelled'
  review_type: 'post_resolution' | 'weekly' | 'monthly' | 'custom'
  attendees: Array<{
    user_id: number
    email: string
    required: boolean
  }>
  agenda: string[]
  notes?: string
  action_items: Array<{
    id: string
    description: string
    owner_id?: number
    due_date?: string
    status: 'open' | 'in_progress' | 'completed'
  }>
  completed_at?: string
  completed_by?: number
  created_by: number
  created_at: string
}

// Incident Response Analytics
export interface IncidentResponseAnalytics {
  period: {
    start: string
    end: string
  }
  response_metrics: {
    avg_time_to_acknowledge_minutes: number
    avg_time_to_assign_minutes: number
    avg_time_to_analyze_minutes: number
    avg_time_to_approve_minutes: number
    avg_time_to_resolve_minutes: number
    avg_time_to_first_response_minutes: number
  }
  by_severity: Record<IncidentSeverity, {
    count: number
    avg_resolution_time_minutes: number
    sla_compliance_rate: number
    avg_time_to_acknowledge_minutes: number
  }>
  by_source: Array<{
    source: string
    count: number
    avg_resolution_time_minutes: number
    avg_time_to_first_response_minutes: number
  }>
  by_assignee: Array<{
    user_id: number
    email: string
    incidents_handled: number
    avg_resolution_time_minutes: number
    sla_compliance_rate: number
  }>
  hourly_distribution: Array<{
    hour: number
    count: number
  }>
  daily_distribution: Array<{
    day: string
    created: number
    resolved: number
  }>
  escalation_rate: number
  reopen_rate: number
  auto_resolution_rate: number
}

// Incident Feedback types
export interface IncidentFeedback {
  id: string
  incident_id: string
  submitted_by: number
  submitted_at: string
  ratings: {
    overall_satisfaction: 1 | 2 | 3 | 4 | 5
    response_speed: 1 | 2 | 3 | 4 | 5
    communication: 1 | 2 | 3 | 4 | 5
    resolution_quality: 1 | 2 | 3 | 4 | 5
  }
  strengths?: string[]
  improvements?: string[]
  additional_comments?: string
  would_recommend: boolean
}

// Incident Cost Tracking
export interface IncidentCost {
  incident_id: string
  estimated_cost_usd: number
  cost_breakdown: {
    labor_hours: number
    labor_cost_usd: number
    infrastructure_cost_usd: number
    revenue_impact_usd: number
    third_party_cost_usd: number
  }
  calculated_at: string
  calculated_by: number
  notes?: string
}

// Incident Compliance
export interface IncidentComplianceRecord {
  id: string
  incident_id: string
  framework: 'soc2' | 'iso27001' | 'gdpr' | 'hipaa' | 'pci' | 'custom'
  requirements: Array<{
    requirement_id: string
    description: string
    status: 'pending' | 'compliant' | 'non_compliant' | 'not_applicable'
    evidence?: string
    notes?: string
  }>
  reviewed_at?: string
  reviewed_by?: number
  status: 'pending' | 'compliant' | 'non_compliant'
  created_at: string
  updated_at: string
}

// Incident Change Link
export interface IncidentChangeLink {
  id: string
  incident_id: string
  change_id: string
  change_type: 'deployment' | 'configuration' | 'infrastructure' | 'schedule'
  change_description?: string
  change_url?: string
  change_timestamp: string
  relationship: 'caused' | 'contributed' | 'resolved' | 'related'
  created_by: number
  created_at: string
}

// On-Call Schedule types
export interface OnCallSchedule {
  id: string
  name: string
  description?: string
  team?: string
  rotation_type: 'weekly' | 'biweekly' | 'monthly' | 'custom'
  rotation_config: {
    start_date: string
    members: Array<{
      user_id: number
      email: string
      order: number
    }>
    handoff_time: string // HH:mm format
    handoff_day?: number // 0-6 for weekly rotations
  }
  overrides: Array<{
    user_id: number
    email: string
    start: string
    end: string
    reason?: string
  }>
  timezone: string
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

export interface OnCallShift {
  id: string
  schedule_id: string
  user_id: number
  user_email: string
  start: string
  end: string
  is_override: boolean
  status: 'upcoming' | 'active' | 'completed'
}

// Incident Run History
export interface IncidentRunHistory {
  id: string
  incident_id: string
  action_type: string
  action_ref: string
  triggered_by: number
  triggered_at: string
  status: 'pending' | 'running' | 'success' | 'failed'
  result?: Record<string, unknown>
  duration_ms?: number
  error?: string
  retry_count: number
}

// Incident Checklist
export interface IncidentChecklist {
  id: string
  incident_id: string
  name: string
  items: Array<{
    id: string
    text: string
    checked: boolean
    checked_at?: string
    checked_by?: number
  }>
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Integration Config
export interface IncidentIntegrationConfig {
  id: string
  name: string
  type: 'pagerduty' | 'opsgenie' | 'datadog' | 'newrelic' | 'prometheus' | 'grafana' | 'slack' | 'teams' | 'custom'
  enabled: boolean
  config: Record<string, unknown>
  mapping_rules: Array<{
    source_field: string
    target_field: string
    transform?: string
  }>
  created_by: number
  created_at: string
  updated_at: string
}

// Responder Team
export interface ResponderTeam {
  id: string
  name: string
  description?: string
  members: Array<{
    user_id: number
    email: string
    role: 'lead' | 'responder' | 'observer'
    skills?: string[]
  }>
  services: string[] // Services this team is responsible for
  escalation_policy_id?: string
  on_call_schedule_id?: string
  created_by: number
  created_at: string
  updated_at: string
}

// SLA Calendar
export interface SLACalendar {
  id: string
  name: string
  description?: string
  timezone: string
  working_hours: {
    start: string // HH:mm format
    end: string
    days: number[] // 0-6, 0 = Sunday
  }
  holidays: Array<{
    date: string // YYYY-MM-DD
    name: string
  }>
  is_default: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Notification Template
export interface NotificationTemplate {
  id: string
  name: string
  description?: string
  channel: 'email' | 'slack' | 'teams' | 'webhook' | 'sms'
  event_type: WebhookEventType | 'all'
  subject_template?: string // For email
  body_template: string
  variables: string[] // Available variables for template
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Action Log
export interface IncidentActionLog {
  id: string
  incident_id: string
  action: string
  actor: {
    user_id: number
    email: string
    role: string
  }
  timestamp: string
  details: Record<string, unknown>
  ip_address?: string
  user_agent?: string
}

// Incident Attachment
export interface IncidentAttachment {
  id: string
  incident_id: string
  filename: string
  content_type: string
  size_bytes: number
  storage_key: string
  uploaded_by: number
  uploaded_at: string
  description?: string
}

// Incident Related Item
export interface IncidentRelatedItem {
  id: string
  incident_id: string
  item_type: 'log' | 'metric' | 'trace' | 'alert' | 'runbook' | 'documentation' | 'code' | 'config'
  title: string
  description?: string
  url?: string
  content?: string
  metadata?: Record<string, unknown>
  added_by: number
  added_at: string
}

// Incident Response Time Target
export interface ResponseTimeTarget {
  id: string
  name: string
  severity: IncidentSeverity
  target_type: 'acknowledge' | 'assign' | 'resolve' | 'first_response'
  target_minutes: number
  business_hours_only: boolean
  sla_calendar_id?: string
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Severity Escalation
export interface SeverityEscalationRule {
  id: string
  name: string
  description?: string
  from_severity: IncidentSeverity
  to_severity: IncidentSeverity
  trigger_conditions: {
    time_without_ack_minutes?: number
    time_without_assign_minutes?: number
    time_without_resolve_minutes?: number
    repeat_count?: number
  }
  actions: Array<{
    type: 'notify' | 'escalate' | 'auto_assign'
    target: string // user_id, team_id, or role
  }>
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Report Schedule
export interface IncidentReportSchedule {
  id: string
  name: string
  description?: string
  schedule: {
    frequency: 'daily' | 'weekly' | 'monthly'
    time: string // HH:mm
    day_of_week?: number // For weekly
    day_of_month?: number // For monthly
    timezone: string
  }
  recipients: Array<{
    type: 'user' | 'team' | 'email'
    id?: number
    email?: string
  }>
  report_config: {
    include_metrics: boolean
    include_open_incidents: boolean
    include_resolved_incidents: boolean
    include_sla_status: boolean
    date_range_days: number
  }
  format: 'html' | 'pdf' | 'json'
  enabled: boolean
  last_run?: string
  next_run?: string
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Metric Threshold
export interface IncidentMetricThreshold {
  id: string
  name: string
  metric: 'count' | 'mttr' | 'sla_breach_rate' | 'escalation_rate'
  condition: 'greater_than' | 'less_than' | 'equals'
  threshold_value: number
  time_window_minutes: number
  group_by: 'severity' | 'source' | 'team' | 'none'
  actions: Array<{
    type: 'alert' | 'webhook' | 'auto_scale'
    config: Record<string, unknown>
  }>
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Retention Policy
export interface IncidentRetentionPolicy {
  id: string
  name: string
  description?: string
  rules: Array<{
    filter: {
      severity?: IncidentSeverity[]
      status?: IncidentStatus[]
      tags?: string[]
    }
    retention_days: number
    archive_after_days?: number
    delete_after_days?: number
  }>
  is_default: boolean
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Runbook
export interface IncidentRunbook {
  id: string
  name: string
  description?: string
  category: 'incident_response' | 'remediation' | 'communication' | 'escalation' | 'recovery'
  triggers: Array<{
    type: 'severity' | 'source' | 'tag' | 'service'
    value: string
  }>
  steps: Array<{
    id: string
    order: number
    title: string
    description?: string
    action_type: 'manual' | 'automated' | 'approval' | 'notification'
    action_config?: {
      script?: string
      api_endpoint?: string
      notification_template_id?: string
      approver_roles?: string[]
      timeout_minutes?: number
    }
    required: boolean
    estimated_minutes?: number
  }>
  auto_start: boolean
  version: number
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Auto-Remediation Rule
export interface AutoRemediationRule {
  id: string
  name: string
  description?: string
  conditions: Array<{
    field: 'severity' | 'source' | 'title_pattern' | 'tag' | 'service' | 'metric_threshold'
    operator: 'equals' | 'contains' | 'matches' | 'greater_than' | 'less_than'
    value: string | number
  }>
  logical_operator: 'and' | 'or'
  action: {
    type: 'run_playbook' | 'execute_script' | 'api_call' | 'scale_service' | 'restart_service'
    config: {
      playbook_id?: string
      script?: string
      api_endpoint?: string
      api_method?: string
      api_payload?: Record<string, unknown>
      service_name?: string
      target_replicas?: number
    }
  }
  requires_approval: boolean
  approver_roles: string[]
  cooldown_minutes: number
  max_executions_per_hour: number
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Maintenance Window
export interface MaintenanceWindow {
  id: string
  name: string
  description?: string
  services: string[]
  start_time: string
  end_time: string
  timezone: string
  recurring?: {
    frequency: 'daily' | 'weekly' | 'monthly'
    interval: number
    end_date?: string
  }
  suppress_alerts: boolean
  suppress_notifications: boolean
  auto_detect_incidents: boolean
  status: 'scheduled' | 'active' | 'completed' | 'cancelled'
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Bulk Operation
export interface IncidentBulkOperation {
  id: string
  operation_type: 'assign' | 'status_change' | 'severity_change' | 'tag' | 'close' | 'escalate'
  incident_ids: string[]
  payload: Record<string, unknown>
  status: 'pending' | 'processing' | 'completed' | 'failed' | 'partial'
  results: Array<{
    incident_id: string
    success: boolean
    error?: string
  }>
  created_by: number
  created_at: string
  completed_at?: string
}

// Incident SLA Breach
export interface IncidentSLABreach {
  id: string
  incident_id: string
  sla_type: 'response' | 'acknowledgment' | 'resolution'
  target_minutes: number
  actual_minutes: number
  breached_at: string
  severity: IncidentSeverity
  acknowledged_at?: string
  resolved_at?: string
  notification_sent: boolean
  escalation_triggered: boolean
}

// Incident Analytics Snapshot
export interface IncidentAnalyticsSnapshot {
  id: string
  snapshot_date: string
  period: 'daily' | 'weekly' | 'monthly'
  metrics: {
    total_incidents: number
    by_severity: Record<IncidentSeverity, number>
    by_status: Record<IncidentStatus, number>
    by_source: Record<string, number>
    mttr_minutes: number
    mtta_minutes: number // Mean time to acknowledge
    first_response_minutes: number
    sla_breach_count: number
    sla_breach_rate: number
    escalation_count: number
    auto_resolved_count: number
    avg_participants: number
    avg_comments: number
    avg_duration_minutes: number
  }
  trends?: {
    incident_count_change: number
    mttr_change: number
    sla_breach_rate_change: number
  }
  created_at: string
}

// Incident Webhook Subscription
export interface IncidentWebhookSubscription {
  id: string
  name: string
  description?: string
  url: string
  secret?: string
  events: Array<WebhookEventType>
  filters?: {
    severities?: IncidentSeverity[]
    services?: string[]
    tags?: string[]
  }
  headers?: Record<string, string>
  retry_policy: {
    max_retries: number
    backoff_multiplier: number
    initial_delay_ms: number
  }
  last_triggered?: string
  failure_count: number
  enabled: boolean
  created_by: number
  created_at: string
  updated_at: string
}

// Incident Snooze
export interface IncidentSnooze {
  id: string
  incident_id: string
  snoozed_by: number
  snoozed_at: string
  wake_at: string
  reason: string
  auto_wake: boolean
  woke_at?: string
  woke_by?: number
  status: 'active' | 'woke' | 'cancelled'
}

// Incident Merge
export interface IncidentMerge {
  id: string
  primary_incident_id: string
  merged_incident_ids: string[]
  merged_by: number
  merged_at: string
  merge_reason: string
  preserve_sub_incidents: boolean
  notify_subscribers: boolean
}

// Incident Split
export interface IncidentSplit {
  id: string
  source_incident_id: string
  new_incidents: Array<{
    id: string
    title: string
    description?: string
    severity: IncidentSeverity
    evidence_ids?: string[]
  }>
  split_by: number
  split_at: string
  split_reason: string
}

// Incident Recurrence
export interface IncidentRecurrence {
  id: string
  incident_id: string
  previous_incident_id: string
  recurrence_count: number
  first_occurred_at: string
  last_occurred_at: string
  pattern_detected: boolean
  root_cause_resolved: boolean
  linked_incidents: string[]
}

// Realtime event types
export type RealtimeScope = 'global' | 'tenant' | 'user' | 'node' | 'task' | 'audit' | 'incident' | 'system'

export interface RealtimeActor {
  user_id: number
  email: string
  role: string
}

export interface RealtimeResource {
  kind: 'node' | 'task' | 'notification' | 'audit' | 'agent' | 'incident' | 'system' | 'user'
  id: string | number
  name?: string
}

export interface RealtimeEvent<T = unknown> {
  id: string
  type: string
  scope: RealtimeScope
  channels: string[]
  payload: T
  timestamp: string
  version: number
  tenant_id?: number
  user_id?: number
  resource?: RealtimeResource
  actor?: RealtimeActor
  correlation_id?: string
}

export interface RealtimeWebSocketConnectedPayload {
  client_id: string
  user_id: number
  email: string
  role: User['role']
  channels: string[]
}

export interface RealtimeWebSocketSubscriptionPayload {
  channel: string
  changed: number
}

export interface RealtimeWebSocketBroadcastPayload extends Record<string, unknown> {
  fromUserId: number
  clientId: string
}

export interface RealtimeWebSocketConnectedMessage extends RealtimeEvent<RealtimeWebSocketConnectedPayload> {
  type: 'connected'
}

export interface RealtimeWebSocketPingMessage {
  type: 'ping'
}

export interface RealtimeWebSocketPongMessage {
  type: 'pong'
}

export interface RealtimeWebSocketErrorMessage {
  type: 'error'
  payload: string
}

export interface RealtimeWebSocketSubscribeMessage {
  type: 'subscribe'
  payload: string
}

export interface RealtimeWebSocketUnsubscribeMessage {
  type: 'unsubscribe'
  payload: string
}

export interface RealtimeWebSocketBroadcastRequestMessage {
  type: 'broadcast'
  payload: Record<string, unknown>
}

export interface RealtimeWebSocketSubscribedMessage {
  type: 'subscribed'
  payload: RealtimeWebSocketSubscriptionPayload
}

export interface RealtimeWebSocketUnsubscribedMessage {
  type: 'unsubscribed'
  payload: RealtimeWebSocketSubscriptionPayload
}

export interface RealtimeWebSocketBroadcastMessage {
  type: 'message'
  payload: RealtimeWebSocketBroadcastPayload
  timestamp: string
}

export type RealtimeWebSocketInboundMessage =
  | RealtimeWebSocketPingMessage
  | RealtimeWebSocketPongMessage
  | RealtimeWebSocketSubscribeMessage
  | RealtimeWebSocketUnsubscribeMessage
  | RealtimeWebSocketBroadcastRequestMessage

export type RealtimeWebSocketOutboundMessage =
  | RealtimeWebSocketConnectedMessage
  | RealtimeWebSocketPingMessage
  | RealtimeWebSocketPongMessage
  | RealtimeWebSocketErrorMessage
  | RealtimeWebSocketSubscribedMessage
  | RealtimeWebSocketUnsubscribedMessage
  | RealtimeWebSocketBroadcastMessage

// API 响应类型
export interface ApiResponse<T = unknown> {
  success: boolean
  data?: T
  error?: string
  message?: string
}

// 分页响应
export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  per_page: number
  total_pages: number
}

// Hono 上下文变量
declare module 'hono' {
  interface ContextVariableMap {
    user: AuthPrincipal
  }
}