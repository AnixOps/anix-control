import type { ZodIssue } from 'zod'

// Cloudflare Workers 环境类型定义

export interface Env {
  // 环境变量
  ENVIRONMENT: 'development' | 'production'
  JWT_SECRET: string
  JWT_EXPIRE: string
  API_KEY_SALT: string
  APP_VERSION?: string
  BUILD_SHA?: string

  // D1 数据库
  DB: D1Database

  // KV 命名空间
  KV: KVNamespace

  // R2 存储桶
  R2: R2Bucket

  // Analytics Engine
  ANALYTICS?: AnalyticsEngineDataset

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
  timestamp: string
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

// Realtime event types
export type RealtimeScope = 'global' | 'tenant' | 'user' | 'node' | 'task' | 'audit' | 'system'

export interface RealtimeActor {
  user_id: number
  email: string
  role: string
}

export interface RealtimeResource {
  kind: 'node' | 'task' | 'notification' | 'audit' | 'agent' | 'system' | 'user'
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
    // Optional tenant context read by tenant-aware realtime/rate-limit code (no tenant middleware is mounted)
    tenant?: {
      id: number
      slug: string
      plan: string
    }
  }
}