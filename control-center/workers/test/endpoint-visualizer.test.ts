import { writeFile } from 'node:fs/promises'
import { performance } from 'node:perf_hooks'
import { expect, test } from 'vitest'
import app from '../src/index'
import type {
  EndpointAuthRequirement,
  EndpointExecutionMode,
  EndpointManifestEntry,
  EndpointReadinessTier,
  Env,
} from '../src/types'
import { bootstrapPrincipals, createTestEnv } from './helpers/fixtures'

const SENSITIVE_KEYS = new Set(['access_token', 'refresh_token', 'password_hash', 'api_key', 'secret'])
const DEFAULT_FIXTURES = ['principals']
const READINESS_LABELS: Record<EndpointReadinessTier, string> = {
  verified: 'Verified',
  seeded: 'Seeded',
  diagnostic: 'Diagnostic',
  manual: 'Manual',
  inventory: 'Inventory',
}

interface RouteCase {
  group: string
  subgroup?: string
  name: string
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
  auth: EndpointAuthRequirement
  expected: number | number[]
  readiness?: EndpointReadinessTier
  executionMode?: EndpointExecutionMode
  fixtureKeys?: string[]
  manualNotes?: string
  body?: (ctx: ScriptContext) => unknown
  headers?: (ctx: ScriptContext) => Record<string, string>
  skip?: string
}

interface ResultRow {
  id: string
  group: string
  subgroup?: string
  name: string
  method: string
  path: string
  auth: EndpointAuthRequirement
  readiness: EndpointReadinessTier
  executionMode: EndpointExecutionMode
  fixtureKeys: string[]
  manualNotes?: string
  expected: string
  status: number | 'skipped' | 'error'
  pass: boolean
  ms: number
  summary: string
}

interface ScriptContext {
  env: Env
  userToken: string
  operatorToken: string
  adminToken: string
}

const INVENTORY_ONLY = 'Inventory-only route; not smoke-tested yet'

function manualCase(
  group: string,
  name: string,
  method: RouteCase['method'],
  path: string,
  auth: RouteCase['auth'],
  expected: number | number[] = 200,
  skip = INVENTORY_ONLY,
  options: Pick<RouteCase, 'subgroup' | 'readiness' | 'executionMode' | 'fixtureKeys' | 'manualNotes'> = {},
): RouteCase {
  return {
    group,
    name,
    method,
    path,
    auth,
    expected,
    skip,
    readiness: options.readiness,
    executionMode: options.executionMode,
    fixtureKeys: options.fixtureKeys,
    manualNotes: options.manualNotes,
    subgroup: options.subgroup,
  }
}

function createCases(): RouteCase[] {
  return [
    { group: 'platform', subgroup: 'health', name: 'health', method: 'GET', path: '/health', auth: 'public', expected: 200, readiness: 'verified', executionMode: 'automated' },
    { group: 'platform', subgroup: 'health', name: 'health detailed', method: 'GET', path: '/health/detailed', auth: 'public', expected: 200, readiness: 'verified', executionMode: 'automated' },
    { group: 'platform', subgroup: 'health', name: 'readiness', method: 'GET', path: '/readiness', auth: 'public', expected: 200, readiness: 'verified', executionMode: 'automated' },
    { group: 'platform', subgroup: 'health', name: 'liveness', method: 'GET', path: '/liveness', auth: 'public', expected: 200, readiness: 'verified', executionMode: 'automated' },
    { group: 'platform', subgroup: 'health', name: 'metrics', method: 'GET', path: '/metrics', auth: 'public', expected: 200, readiness: 'verified', executionMode: 'automated' },
    { group: 'platform', subgroup: 'developer-mode', name: 'developer mode status', method: 'GET', path: '/api/v1/internal/dev/status', auth: 'admin', expected: 200, readiness: 'diagnostic', executionMode: 'diagnostic', fixtureKeys: DEFAULT_FIXTURES, manualNotes: 'Confirms internal developer mode is enabled and gated to admin JWT sessions.' },
    { group: 'platform', subgroup: 'developer-mode', name: 'developer diagnostics', method: 'GET', path: '/api/v1/internal/dev/diagnostics', auth: 'admin', expected: 200, readiness: 'diagnostic', executionMode: 'diagnostic', fixtureKeys: DEFAULT_FIXTURES },
    { group: 'platform', subgroup: 'developer-mode', name: 'developer fixtures', method: 'GET', path: '/api/v1/internal/dev/fixtures', auth: 'admin', expected: 200, readiness: 'diagnostic', executionMode: 'diagnostic', fixtureKeys: DEFAULT_FIXTURES },

    manualCase('auth', 'register', 'POST', '/api/v1/auth/register', 'public', 201, 'Rate-limited by bootstrap users; inventory only', { subgroup: 'public', readiness: 'inventory', executionMode: 'inventory', manualNotes: 'Use dedicated auth flow tests when validating registration UX or rate-limit behavior.' }),
    {
      group: 'auth',
      subgroup: 'public',
      name: 'login',
      method: 'POST',
      path: '/api/v1/auth/login',
      auth: 'public',
      expected: 200,
      readiness: 'verified',
      executionMode: 'automated',
      fixtureKeys: DEFAULT_FIXTURES,
      body: () => ({ email: 'visualizer-admin@example.com', password: 'VisualizerPass123!' }),
    },
    {
      group: 'auth',
      subgroup: 'public',
      name: 'refresh',
      method: 'POST',
      path: '/api/v1/auth/refresh',
      auth: 'public',
      expected: [200, 401],
      readiness: 'diagnostic',
      executionMode: 'automated',
      manualNotes: 'Current smoke run uses an invalid refresh token to verify boundary behavior.',
      body: () => ({ refresh_token: 'invalid-token' }),
    },
    manualCase('auth', 'logout', 'POST', '/api/v1/auth/logout', 'user', 200, INVENTORY_ONLY, { subgroup: 'session', readiness: 'manual', executionMode: 'manual', fixtureKeys: DEFAULT_FIXTURES, manualNotes: 'Validate token invalidation and session teardown in targeted auth regression tests.' }),

    {
      group: 'identity',
      subgroup: 'self-service',
      name: 'current user',
      method: 'GET',
      path: '/api/v1/users/me',
      auth: 'user',
      expected: 200,
      readiness: 'verified',
      executionMode: 'automated',
      fixtureKeys: DEFAULT_FIXTURES,
    },
    manualCase('identity', 'update current user', 'PUT', '/api/v1/users/me', 'user', 200, INVENTORY_ONLY, { subgroup: 'self-service', readiness: 'manual', executionMode: 'manual', fixtureKeys: DEFAULT_FIXTURES, manualNotes: 'Manual QA should verify partial profile updates and field validation.' }),
    {
      group: 'identity',
      subgroup: 'tokens',
      name: 'api tokens',
      method: 'GET',
      path: '/api/v1/users/me/tokens',
      auth: 'user',
      expected: 200,
      readiness: 'verified',
      executionMode: 'automated',
      fixtureKeys: DEFAULT_FIXTURES,
    },
    manualCase('identity', 'create api token', 'POST', '/api/v1/users/me/tokens', 'user', 201, INVENTORY_ONLY, { subgroup: 'tokens', readiness: 'manual', executionMode: 'manual', fixtureKeys: DEFAULT_FIXTURES }),
    manualCase('identity', 'delete api token', 'DELETE', '/api/v1/users/me/tokens/:id', 'user', 200, INVENTORY_ONLY, { subgroup: 'tokens', readiness: 'inventory', executionMode: 'inventory' }),
    {
      group: 'identity',
      subgroup: 'sessions',
      name: 'sessions',
      method: 'GET',
      path: '/api/v1/users/me/sessions',
      auth: 'user',
      expected: 200,
      readiness: 'verified',
      executionMode: 'automated',
      fixtureKeys: DEFAULT_FIXTURES,
    },
    manualCase('identity', 'delete other sessions', 'DELETE', '/api/v1/users/me/sessions/others', 'user', 200, INVENTORY_ONLY, { subgroup: 'sessions', readiness: 'manual', executionMode: 'manual', fixtureKeys: DEFAULT_FIXTURES }),
    manualCase('identity', 'change password', 'PUT', '/api/v1/auth/password', 'user', 200, INVENTORY_ONLY, { subgroup: 'sessions', readiness: 'manual', executionMode: 'manual', fixtureKeys: DEFAULT_FIXTURES }),

    {
      group: 'identity-admin',
      subgroup: 'administration',
      name: 'list users',
      method: 'GET',
      path: '/api/v1/users',
      auth: 'admin',
      expected: 200,
      readiness: 'verified',
      executionMode: 'automated',
      fixtureKeys: DEFAULT_FIXTURES,
    },
    manualCase('identity-admin', 'get user', 'GET', '/api/v1/users/:id', 'admin', 200, INVENTORY_ONLY, { subgroup: 'administration', readiness: 'inventory', executionMode: 'inventory' }),
    manualCase('identity-admin', 'create user', 'POST', '/api/v1/users', 'admin', 201, INVENTORY_ONLY, { subgroup: 'administration', readiness: 'manual', executionMode: 'manual' }),
    manualCase('identity-admin', 'update user', 'PUT', '/api/v1/users/:id', 'admin', 200, INVENTORY_ONLY, { subgroup: 'administration', readiness: 'inventory', executionMode: 'inventory' }),
    manualCase('identity-admin', 'delete user', 'DELETE', '/api/v1/users/:id', 'admin', 200, INVENTORY_ONLY, { subgroup: 'administration', readiness: 'inventory', executionMode: 'inventory' }),
    manualCase('identity-admin', 'lockout status', 'GET', '/api/v1/users/:id/lockout', 'admin', 200, INVENTORY_ONLY, { subgroup: 'security', readiness: 'inventory', executionMode: 'inventory' }),
    manualCase('identity-admin', 'unlock user', 'POST', '/api/v1/users/:id/unlock', 'admin', 200, INVENTORY_ONLY, { subgroup: 'security', readiness: 'inventory', executionMode: 'inventory' }),

    {
      group: 'nodes',
      name: 'list nodes',
      method: 'GET',
      path: '/api/v1/nodes',
      auth: 'user',
      expected: 200,
    },
    {
      group: 'nodes',
      name: 'create node',
      method: 'POST',
      path: '/api/v1/nodes',
      auth: 'admin',
      expected: 201,
      body: () => ({
        name: 'visualizer-node-1',
        host: '192.168.1.100',
        port: 22,
      }),
    },
    manualCase('nodes', 'get node', 'GET', '/api/v1/nodes/:id', 'user'),
    manualCase('nodes', 'node stats', 'GET', '/api/v1/nodes/:id/stats', 'user'),
    manualCase('nodes', 'node logs', 'GET', '/api/v1/nodes/:id/logs', 'user'),
    manualCase('nodes', 'bulk nodes', 'POST', '/api/v1/nodes/bulk', 'operator'),
    manualCase('nodes', 'bulk status', 'POST', '/api/v1/nodes/bulk-status', 'operator'),
    manualCase('nodes', 'start node', 'POST', '/api/v1/nodes/:id/start', 'operator'),
    manualCase('nodes', 'stop node', 'POST', '/api/v1/nodes/:id/stop', 'operator'),
    manualCase('nodes', 'restart node', 'POST', '/api/v1/nodes/:id/restart', 'operator'),
    manualCase('nodes', 'test node', 'POST', '/api/v1/nodes/:id/test', 'user'),
    manualCase('nodes', 'sync node', 'POST', '/api/v1/nodes/:id/sync', 'operator'),
    manualCase('nodes', 'update node', 'PUT', '/api/v1/nodes/:id', 'operator'),
    manualCase('nodes', 'delete node', 'DELETE', '/api/v1/nodes/:id', 'admin'),
    manualCase('nodes', 'install script', 'GET', '/api/v1/nodes/:nodeId/install-script', 'operator'),

    {
      group: 'playbooks',
      name: 'list playbooks',
      method: 'GET',
      path: '/api/v1/playbooks',
      auth: 'user',
      expected: 200,
    },
    { group: 'playbooks', name: 'built-in playbooks', method: 'GET', path: '/api/v1/playbooks/built-in', auth: 'user', expected: 200 },
    { group: 'playbooks', name: 'categories', method: 'GET', path: '/api/v1/playbooks/categories', auth: 'user', expected: 200 },
    manualCase('playbooks', 'get playbook', 'GET', '/api/v1/playbooks/:name', 'user'),
    manualCase('playbooks', 'sync builtin', 'POST', '/api/v1/playbooks/sync-builtin', 'admin'),
    manualCase('playbooks', 'upload playbook', 'POST', '/api/v1/playbooks', 'operator', 201),
    manualCase('playbooks', 'delete playbook', 'DELETE', '/api/v1/playbooks/:name', 'admin'),

    {
      group: 'tasks',
      name: 'list tasks',
      method: 'GET',
      path: '/api/v1/tasks',
      auth: 'user',
      expected: 200,
    },
    manualCase('tasks', 'create task', 'POST', '/api/v1/tasks', 'operator', 201),
    manualCase('tasks', 'get task', 'GET', '/api/v1/tasks/:id', 'user'),
    manualCase('tasks', 'task logs', 'GET', '/api/v1/tasks/:id/logs', 'user'),
    manualCase('tasks', 'cancel task', 'POST', '/api/v1/tasks/:id/cancel', 'operator'),
    manualCase('tasks', 'retry task', 'POST', '/api/v1/tasks/:id/retry', 'operator'),

    {
      group: 'schedules',
      name: 'list schedules',
      method: 'GET',
      path: '/api/v1/schedules',
      auth: 'user',
      expected: 200,
    },
    manualCase('schedules', 'create schedule', 'POST', '/api/v1/schedules', 'operator', 201),
    manualCase('schedules', 'get schedule', 'GET', '/api/v1/schedules/:id', 'user'),
    manualCase('schedules', 'update schedule', 'PUT', '/api/v1/schedules/:id', 'operator'),
    manualCase('schedules', 'delete schedule', 'DELETE', '/api/v1/schedules/:id', 'admin'),
    manualCase('schedules', 'toggle schedule', 'POST', '/api/v1/schedules/:id/toggle', 'operator'),
    manualCase('schedules', 'run now', 'POST', '/api/v1/schedules/:id/run', 'operator'),

    {
      group: 'node-groups',
      name: 'list node groups',
      method: 'GET',
      path: '/api/v1/node-groups',
      auth: 'user',
      expected: 200,
    },
    manualCase('node-groups', 'create node group', 'POST', '/api/v1/node-groups', 'operator', 201),
    manualCase('node-groups', 'get node group', 'GET', '/api/v1/node-groups/:id', 'user'),
    manualCase('node-groups', 'update node group', 'PUT', '/api/v1/node-groups/:id', 'operator'),
    manualCase('node-groups', 'delete node group', 'DELETE', '/api/v1/node-groups/:id', 'admin'),
    manualCase('node-groups', 'add nodes to group', 'POST', '/api/v1/node-groups/:id/nodes', 'operator'),
    manualCase('node-groups', 'remove nodes from group', 'DELETE', '/api/v1/node-groups/:id/nodes', 'operator'),

    {
      group: 'plugins',
      name: 'list plugins',
      method: 'GET',
      path: '/api/v1/plugins',
      auth: 'user',
      expected: 200,
    },
    manualCase('plugins', 'get plugin', 'GET', '/api/v1/plugins/:name', 'user'),
    manualCase('plugins', 'execute plugin', 'POST', '/api/v1/plugins/:name/execute', 'operator'),

    {
      group: 'dashboard',
      name: 'dashboard',
      method: 'GET',
      path: '/api/v1/dashboard',
      auth: 'user',
      expected: 200,
    },
    {
      group: 'dashboard',
      name: 'dashboard stats',
      method: 'GET',
      path: '/api/v1/dashboard/stats',
      auth: 'user',
      expected: 200,
    },

    {
      group: 'audit',
      name: 'audit logs',
      method: 'GET',
      path: '/api/v1/audit-logs',
      auth: 'admin',
      expected: 200,
    },

    {
      group: 'notifications',
      name: 'list notifications',
      method: 'GET',
      path: '/api/v1/notifications',
      auth: 'user',
      expected: 200,
    },
    {
      group: 'notifications',
      name: 'unread count',
      method: 'GET',
      path: '/api/v1/notifications/unread-count',
      auth: 'user',
      expected: 200,
    },
    manualCase('notifications', 'create notification', 'POST', '/api/v1/notifications', 'operator', 201),
    manualCase('notifications', 'mark notification read', 'PUT', '/api/v1/notifications/:id/read', 'user'),
    manualCase('notifications', 'mark all read', 'PUT', '/api/v1/notifications/read-all', 'user'),
    manualCase('notifications', 'delete notification', 'DELETE', '/api/v1/notifications/:id', 'user'),

    {
      group: 'incidents',
      name: 'list incidents',
      method: 'GET',
      path: '/api/v1/incidents',
      auth: 'user',
      expected: 200,
    },
    {
      group: 'incidents',
      name: 'create incident',
      method: 'POST',
      path: '/api/v1/incidents',
      auth: 'user',
      expected: 201,
      body: () => ({
        title: 'Visualizer Incident',
        source: 'visualizer',
      }),
    },
    {
      group: 'incidents',
      name: 'incident stats',
      method: 'GET',
      path: '/api/v1/incidents/statistics',
      auth: 'user',
      expected: 200,
    },
    {
      group: 'incidents',
      name: 'incident search',
      method: 'GET',
      path: '/api/v1/incidents/search?q=visualizer',
      auth: 'user',
      expected: 200,
    },
    manualCase('incidents', 'incident templates', 'GET', '/api/v1/incidents/templates', 'admin'),
    {
      group: 'incidents',
      name: 'incident dashboard metrics',
      method: 'GET',
      path: '/api/v1/incidents/dashboard/metrics',
      auth: 'admin',
      expected: 200,
    },
    manualCase('incidents', 'incident teams', 'GET', '/api/v1/incidents/teams', 'admin'),
    manualCase('incidents', 'incident response playbooks', 'GET', '/api/v1/incidents/response-playbooks', 'admin'),
    {
      group: 'incidents',
      name: 'incident analytics response',
      method: 'GET',
      path: '/api/v1/incidents/analytics/response?start_date=2026-01-01&end_date=2026-01-31',
      auth: 'admin',
      expected: 200,
    },
    manualCase('incidents', 'incident report', 'GET', '/api/v1/incidents/report', 'operator'),
    manualCase('incidents', 'incident bulk analyze', 'POST', '/api/v1/incidents/bulk/analyze', 'operator'),
    manualCase('incidents', 'incident bulk approve', 'POST', '/api/v1/incidents/bulk/approve', 'operator'),
    manualCase('incidents', 'incident bulk execute', 'POST', '/api/v1/incidents/bulk/execute', 'operator'),
    manualCase('incidents', 'incident bulk delete', 'POST', '/api/v1/incidents/bulk/delete', 'admin'),
    manualCase('incidents', 'incident merge', 'POST', '/api/v1/incidents/merge', 'operator'),
    manualCase('incidents', 'get incident', 'GET', '/api/v1/incidents/:id', 'user'),
    manualCase('incidents', 'incident timeline', 'GET', '/api/v1/incidents/:id/timeline', 'user'),
    manualCase('incidents', 'incident SLA', 'GET', '/api/v1/incidents/:id/sla', 'user'),
    manualCase('incidents', 'incident comments', 'GET', '/api/v1/incidents/:id/comments', 'user'),
    manualCase('incidents', 'add comment', 'POST', '/api/v1/incidents/:id/comments', 'user'),
    manualCase('incidents', 'acknowledge incident', 'POST', '/api/v1/incidents/:id/acknowledge', 'operator'),
    manualCase('incidents', 'escalate incident', 'POST', '/api/v1/incidents/:id/escalate', 'operator'),
    manualCase('incidents', 'assign incident', 'POST', '/api/v1/incidents/:id/assign', 'operator'),
    manualCase('incidents', 'unassign incident', 'DELETE', '/api/v1/incidents/:id/assign', 'operator'),
    manualCase('incidents', 'analyze incident', 'POST', '/api/v1/incidents/:id/analyze', 'operator'),
    manualCase('incidents', 'approve incident', 'POST', '/api/v1/incidents/:id/approve', 'operator'),
    manualCase('incidents', 'execute incident', 'POST', '/api/v1/incidents/:id/execute', 'operator'),
    manualCase('incidents', 'incident links', 'POST', '/api/v1/incidents/:id/links', 'operator'),
    manualCase('incidents', 'incident activity', 'GET', '/api/v1/incidents/:id/activity', 'user'),
    manualCase('incidents', 'incident runbooks', 'GET', '/api/v1/incidents/:id/runbooks', 'operator'),
    manualCase('incidents', 'incident templates create', 'POST', '/api/v1/incidents/templates', 'admin', 201),
    manualCase('incidents', 'suppression rules', 'GET', '/api/v1/incidents/suppression-rules', 'admin'),
    manualCase('incidents', 'notification rules', 'GET', '/api/v1/incidents/notification-rules', 'admin'),
    manualCase('incidents', 'automation rules', 'GET', '/api/v1/incidents/automation-rules', 'admin'),
    manualCase('incidents', 'postmortem', 'GET', '/api/v1/incidents/:id/postmortem', 'operator'),
    manualCase('incidents', 'correlation', 'GET', '/api/v1/incidents/:id/correlation', 'operator'),
    manualCase('incidents', 'watch incident', 'POST', '/api/v1/incidents/:id/watch', 'user'),
    manualCase('incidents', 'watchers', 'GET', '/api/v1/incidents/:id/watchers', 'user'),
    manualCase('incidents', 'tickets', 'GET', '/api/v1/incidents/:id/tickets', 'operator'),
    manualCase('incidents', 'custom fields', 'GET', '/api/v1/incidents/custom-fields', 'admin'),
    manualCase('incidents', 'custom field assign', 'PUT', '/api/v1/incidents/:id/custom-fields/:fieldId', 'operator'),
    manualCase('incidents', 'ai analysis', 'POST', '/api/v1/incidents/:id/ai-analysis', 'operator'),
    manualCase('incidents', 'war room', 'POST', '/api/v1/incidents/:id/war-room', 'operator'),
    manualCase('incidents', 'export incidents', 'POST', '/api/v1/incidents/export', 'operator'),
    manualCase('incidents', 'reviews', 'GET', '/api/v1/incidents/:id/reviews', 'user'),
    manualCase('incidents', 'feedback', 'GET', '/api/v1/incidents/:id/feedback', 'operator'),
    manualCase('incidents', 'cost', 'GET', '/api/v1/incidents/:id/cost', 'operator'),
    manualCase('incidents', 'compliance', 'GET', '/api/v1/incidents/:id/compliance', 'operator'),
    manualCase('incidents', 'oncall schedules', 'GET', '/api/v1/incidents/oncall/schedules', 'operator'),
    manualCase('incidents', 'checklists', 'GET', '/api/v1/incidents/:id/checklists', 'user'),
    manualCase('incidents', 'changes', 'GET', '/api/v1/incidents/:id/changes', 'user'),
    manualCase('incidents', 'runs', 'GET', '/api/v1/incidents/:id/runs', 'operator'),
    manualCase('incidents', 'sla calendars', 'GET', '/api/v1/incidents/sla-calendars', 'operator'),
    manualCase('incidents', 'notification templates', 'GET', '/api/v1/incidents/notification-templates', 'operator'),
    manualCase('incidents', 'escalation rules', 'GET', '/api/v1/incidents/escalation-rules', 'operator'),
    manualCase('incidents', 'attachments', 'GET', '/api/v1/incidents/:id/attachments', 'user'),
    manualCase('incidents', 'related items', 'GET', '/api/v1/incidents/:id/related-items', 'user'),
    manualCase('incidents', 'response targets', 'GET', '/api/v1/incidents/response-targets', 'operator'),
    manualCase('incidents', 'integrations', 'GET', '/api/v1/incidents/integrations', 'operator'),
    manualCase('incidents', 'timeline events', 'GET', '/api/v1/incidents/:id/timeline-events', 'user'),
    manualCase('incidents', 'runbooks admin', 'GET', '/api/v1/incidents/runbooks', 'operator'),
    manualCase('incidents', 'auto remediation rules', 'GET', '/api/v1/incidents/auto-remediation-rules', 'operator'),
    manualCase('incidents', 'maintenance windows', 'GET', '/api/v1/incidents/maintenance-windows', 'operator'),
    manualCase('incidents', 'bulk operations', 'GET', '/api/v1/incidents/bulk-operations', 'operator'),
    manualCase('incidents', 'sla breaches', 'GET', '/api/v1/incidents/sla-breaches', 'operator'),
    manualCase('incidents', 'analytics snapshots', 'GET', '/api/v1/incidents/analytics', 'operator'),
    manualCase('incidents', 'webhooks', 'GET', '/api/v1/incidents/webhooks', 'operator'),
    manualCase('incidents', 'snoozes', 'GET', '/api/v1/incidents/snoozes', 'operator'),
    manualCase('incidents', 'merges', 'GET', '/api/v1/incidents/merges', 'operator'),
    manualCase('incidents', 'splits', 'GET', '/api/v1/incidents/splits', 'operator'),
    manualCase('incidents', 'recurrences', 'GET', '/api/v1/incidents/recurrences', 'operator'),

    {
      group: 'governance',
      name: 'policies',
      method: 'GET',
      path: '/api/v1/governance/policies',
      auth: 'admin',
      expected: 200,
    },
    {
      group: 'governance',
      name: 'active policy',
      method: 'GET',
      path: '/api/v1/governance/policies/active',
      auth: 'user',
      expected: 200,
    },
    manualCase('governance', 'policy detail', 'GET', '/api/v1/governance/policies/:id', 'admin'),
    manualCase('governance', 'create policy', 'POST', '/api/v1/governance/policies', 'admin', 201),
    manualCase('governance', 'update policy', 'PUT', '/api/v1/governance/policies/:id', 'admin'),
    manualCase('governance', 'delete policy', 'DELETE', '/api/v1/governance/policies/:id', 'admin'),

    {
      group: 'webhooks',
      name: 'list webhooks',
      method: 'GET',
      path: '/api/v1/webhooks',
      auth: 'admin',
      expected: 200,
    },
    {
      group: 'webhooks',
      name: 'failed deliveries',
      method: 'GET',
      path: '/api/v1/webhooks/failed-deliveries',
      auth: 'admin',
      expected: 200,
    },
    manualCase('webhooks', 'create webhook', 'POST', '/api/v1/webhooks', 'admin', 201),
    manualCase('webhooks', 'webhook detail', 'GET', '/api/v1/webhooks/:id', 'admin'),
    manualCase('webhooks', 'update webhook', 'PUT', '/api/v1/webhooks/:id', 'admin'),
    manualCase('webhooks', 'delete webhook', 'DELETE', '/api/v1/webhooks/:id', 'admin'),
    manualCase('webhooks', 'delivery list', 'GET', '/api/v1/webhooks/:id/deliveries', 'admin'),
    manualCase('webhooks', 'retry delivery', 'POST', '/api/v1/webhooks/deliveries/:deliveryId/retry', 'admin'),

    {
      group: 'backups',
      name: 'list backups',
      method: 'GET',
      path: '/api/v1/backups',
      auth: 'admin',
      expected: 200,
    },
    {
      group: 'backups',
      name: 'backup status',
      method: 'GET',
      path: '/api/v1/backups/status',
      auth: 'admin',
      expected: 200,
    },
    manualCase('backups', 'create backup', 'POST', '/api/v1/backups', 'admin', 201),
    manualCase('backups', 'backup detail', 'GET', '/api/v1/backups/:id', 'admin'),
    manualCase('backups', 'download backup', 'GET', '/api/v1/backups/:id/download', 'admin'),
    manualCase('backups', 'restore backup', 'POST', '/api/v1/backups/:id/restore', 'admin'),
    manualCase('backups', 'delete backup', 'DELETE', '/api/v1/backups/:id', 'admin'),
    manualCase('backups', 'cleanup backups', 'POST', '/api/v1/backups/cleanup', 'admin'),

    {
      group: 'agents',
      name: 'register agent',
      method: 'POST',
      path: '/api/v1/agents/register',
      auth: 'public',
      expected: 201,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'agents',
      name: 'heartbeat',
      method: 'POST',
      path: '/api/v1/agents/heartbeat',
      auth: 'public',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'agents',
      name: 'metrics',
      method: 'POST',
      path: '/api/v1/agents/metrics',
      auth: 'public',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'agents',
      name: 'command result',
      method: 'POST',
      path: '/api/v1/agents/command-result',
      auth: 'public',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    manualCase('agents', 'agent metrics', 'GET', '/api/v1/agents/:agentId/metrics', 'user'),
    manualCase('agents', 'send command', 'POST', '/api/v1/agents/:agentId/command', 'operator'),
    manualCase('agents', 'install script', 'GET', '/api/v1/nodes/:nodeId/install-script', 'operator'),

    {
      group: 'mfa',
      name: 'mfa status',
      method: 'GET',
      path: '/api/v1/mfa/status',
      auth: 'user',
      expected: 200,
    },
    manualCase('mfa', 'setup mfa', 'POST', '/api/v1/mfa/setup', 'user'),
    manualCase('mfa', 'enable mfa', 'POST', '/api/v1/mfa/enable', 'user'),
    manualCase('mfa', 'disable mfa', 'POST', '/api/v1/mfa/disable', 'user'),
    manualCase('mfa', 'verify mfa', 'POST', '/api/v1/mfa/verify', 'user'),
    manualCase('mfa', 'recovery codes', 'POST', '/api/v1/mfa/recovery-codes', 'user'),
    manualCase('mfa', 'admin disable mfa', 'POST', '/api/v1/admin/users/:id/mfa/disable', 'admin'),

    manualCase('batch', 'batch operations', 'POST', '/api/v1/batch', 'operator'),

    {
      group: 'kubernetes',
      name: 'cluster overview',
      method: 'GET',
      path: '/api/v1/kubernetes/overview',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'kubernetes',
      name: 'namespaces',
      method: 'GET',
      path: '/api/v1/kubernetes/namespaces',
      auth: 'operator',
      expected: 200,
    },
    manualCase('kubernetes', 'namespace details', 'GET', '/api/v1/kubernetes/namespaces/:namespace', 'operator'),
    {
      group: 'kubernetes',
      name: 'cluster nodes',
      method: 'GET',
      path: '/api/v1/kubernetes/nodes',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'kubernetes',
      name: 'pods',
      method: 'GET',
      path: '/api/v1/kubernetes/pods',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'kubernetes',
      name: 'deployments',
      method: 'GET',
      path: '/api/v1/kubernetes/deployments',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'kubernetes',
      name: 'services',
      method: 'GET',
      path: '/api/v1/kubernetes/services',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'kubernetes',
      name: 'events',
      method: 'GET',
      path: '/api/v1/kubernetes/events',
      auth: 'operator',
      expected: 200,
    },
    manualCase('kubernetes', 'pod logs', 'GET', '/api/v1/kubernetes/namespaces/:namespace/pods/:pod/logs', 'operator'),
    manualCase('kubernetes', 'scale deployment', 'POST', '/api/v1/kubernetes/namespaces/:namespace/deployments/:name/scale', 'admin'),
    manualCase('kubernetes', 'restart deployment', 'POST', '/api/v1/kubernetes/namespaces/:namespace/deployments/:name/restart', 'operator'),

    {
      group: 'mesh',
      name: 'mesh overview',
      method: 'GET',
      path: '/api/v1/mesh/overview',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'mesh',
      name: 'mesh services',
      method: 'GET',
      path: '/api/v1/mesh/services',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'mesh',
      name: 'virtual services',
      method: 'GET',
      path: '/api/v1/mesh/virtualservices',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'mesh',
      name: 'destination rules',
      method: 'GET',
      path: '/api/v1/mesh/destinationrules',
      auth: 'operator',
      expected: 200,
    },
    {
      group: 'mesh',
      name: 'gateways',
      method: 'GET',
      path: '/api/v1/mesh/gateways',
      auth: 'operator',
      expected: 200,
    },
    manualCase('mesh', 'traffic split', 'POST', '/api/v1/mesh/traffic/split', 'admin'),
    manualCase('mesh', 'circuit breaker', 'POST', '/api/v1/mesh/circuit-breaker', 'admin'),
    manualCase('mesh', 'fault inject', 'POST', '/api/v1/mesh/fault/inject', 'admin'),

    {
      group: 'logs',
      name: 'search logs',
      method: 'GET',
      path: '/api/v1/logs',
      auth: 'admin',
      expected: 200,
    },
    {
      group: 'logs',
      name: 'log stats',
      method: 'GET',
      path: '/api/v1/logs/stats',
      auth: 'admin',
      expected: 200,
    },
    manualCase('logs', 'export logs', 'GET', '/api/v1/logs/export', 'admin'),
    manualCase('logs', 'index log', 'POST', '/api/v1/logs', 'user'),
    manualCase('logs', 'bulk index logs', 'POST', '/api/v1/logs/bulk', 'operator'),
    manualCase('logs', 'get log', 'GET', '/api/v1/logs/:id', 'operator'),
    manualCase('logs', 'create log index', 'POST', '/api/v1/logs/index', 'admin'),
    manualCase('logs', 'delete old logs', 'DELETE', '/api/v1/logs/old', 'admin'),
    manualCase('logs', 'trace logs', 'GET', '/api/v1/logs/trace/:traceId', 'operator'),
    manualCase('logs', 'node logs v2', 'GET', '/api/v1/logs/node/:nodeId', 'operator'),
    manualCase('logs', 'service logs', 'GET', '/api/v1/logs/service/:service', 'operator'),

    {
      group: 'scaling',
      name: 'scaling policies',
      method: 'GET',
      path: '/api/v1/scaling/policies',
      auth: 'operator',
      expected: 200,
    },
    manualCase('scaling', 'create scaling policy', 'POST', '/api/v1/scaling/policies', 'admin', 201),
    manualCase('scaling', 'get scaling policy', 'GET', '/api/v1/scaling/policies/:id', 'operator'),
    manualCase('scaling', 'update scaling policy', 'PUT', '/api/v1/scaling/policies/:id', 'admin'),
    manualCase('scaling', 'delete scaling policy', 'DELETE', '/api/v1/scaling/policies/:id', 'admin'),
    manualCase('scaling', 'toggle scaling policy', 'POST', '/api/v1/scaling/policies/:id/toggle', 'admin'),
    manualCase('scaling', 'evaluate scaling policy', 'GET', '/api/v1/scaling/policies/:id/evaluate', 'operator'),
    manualCase('scaling', 'execute scaling action', 'POST', '/api/v1/scaling/policies/:id/execute', 'admin'),
    manualCase('scaling', 'scaling history', 'GET', '/api/v1/scaling/policies/:id/history', 'operator'),
    manualCase('scaling', 'recommend replicas', 'GET', '/api/v1/scaling/policies/:id/recommend', 'operator'),
    manualCase('scaling', 'scaling metrics', 'GET', '/api/v1/scaling/policies/:id/metrics', 'operator'),
    manualCase('scaling', 'health by type', 'GET', '/api/v1/scaling/health/:type/:id', 'operator'),
    manualCase('scaling', 'run scaling check', 'POST', '/api/v1/scaling/check', 'admin'),

    {
      group: 'load-balancing',
      name: 'load balancers',
      method: 'GET',
      path: '/api/v1/lb',
      auth: 'operator',
      expected: 200,
    },
    manualCase('load-balancing', 'create load balancer', 'POST', '/api/v1/lb', 'admin', 201),
    manualCase('load-balancing', 'get load balancer', 'GET', '/api/v1/lb/:id', 'operator'),
    manualCase('load-balancing', 'update load balancer', 'PUT', '/api/v1/lb/:id', 'admin'),
    manualCase('load-balancing', 'delete load balancer', 'DELETE', '/api/v1/lb/:id', 'admin'),
    manualCase('load-balancing', 'toggle load balancer', 'POST', '/api/v1/lb/:id/toggle', 'admin'),
    manualCase('load-balancing', 'select target', 'GET', '/api/v1/lb/:id/select', 'user'),
    manualCase('load-balancing', 'lb stats', 'GET', '/api/v1/lb/:id/stats', 'operator'),
    manualCase('load-balancing', 'health check', 'POST', '/api/v1/lb/:id/health-check', 'operator'),
    manualCase('load-balancing', 'target health', 'GET', '/api/v1/lb/:id/targets/:targetId/health', 'operator'),
    manualCase('load-balancing', 'add target', 'POST', '/api/v1/lb/:id/targets', 'admin'),
    manualCase('load-balancing', 'remove target', 'DELETE', '/api/v1/lb/:id/targets/:targetId', 'admin'),
    manualCase('load-balancing', 'weight update', 'PUT', '/api/v1/lb/:id/targets/:targetId/weight', 'admin'),
    manualCase('load-balancing', 'complete target', 'POST', '/api/v1/lb/:id/targets/:targetId/complete', 'user'),

    {
      group: 'observability',
      name: 'sse status',
      method: 'GET',
      path: '/api/v1/sse/status',
      auth: 'user',
      expected: 200,
    },
    manualCase('observability', 'sse stream', 'GET', '/api/v1/sse', 'user'),
    manualCase('observability', 'sse subscribe', 'POST', '/api/v1/sse/subscribe', 'user'),
    manualCase('observability', 'sse unsubscribe', 'POST', '/api/v1/sse/unsubscribe', 'user'),
    {
      group: 'observability',
      name: 'websocket handshake',
      method: 'GET',
      path: '/api/v1/ws',
      auth: 'user',
      expected: [426, 101],
      skip: 'WebSocket upgrade is not suitable for a plain HTTP smoke request',
    },

    {
      group: 'ai',
      name: 'chat',
      method: 'POST',
      path: '/api/v1/ai/chat',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'ai',
      name: 'analyze log',
      method: 'POST',
      path: '/api/v1/ai/analyze-log',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'ai',
      name: 'ops advice',
      method: 'POST',
      path: '/api/v1/ai/ops-advice',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'ai',
      name: 'embedding',
      method: 'POST',
      path: '/api/v1/ai/embedding',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'ai',
      name: 'query',
      method: 'POST',
      path: '/api/v1/ai/query',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },

    {
      group: 'vectors',
      name: 'search vectors',
      method: 'POST',
      path: '/api/v1/vectors/search',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'vectors',
      name: 'insert vectors',
      method: 'POST',
      path: '/api/v1/vectors',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'vectors',
      name: 'delete vector',
      method: 'DELETE',
      path: '/api/v1/vectors/:id',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },

    {
      group: 'ipfs',
      name: 'upload ipfs',
      method: 'POST',
      path: '/api/v1/ipfs/upload',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'ipfs',
      name: 'get ipfs',
      method: 'GET',
      path: '/api/v1/ipfs/:cid',
      auth: 'public',
      expected: 200,
      skip: INVENTORY_ONLY,
    },

    {
      group: 'web3',
      name: 'challenge',
      method: 'POST',
      path: '/api/v1/web3/challenge',
      auth: 'public',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'web3',
      name: 'verify',
      method: 'POST',
      path: '/api/v1/web3/verify',
      auth: 'public',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
    {
      group: 'web3',
      name: 'audit',
      method: 'POST',
      path: '/api/v1/web3/audit',
      auth: 'user',
      expected: 200,
      skip: INVENTORY_ONLY,
    },
  ]
}

function sanitizeValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(sanitizeValue)
  }

  if (!value || typeof value !== 'object') {
    return value
  }

  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>).map(([key, child]) => [
      key,
      SENSITIVE_KEYS.has(key) ? '[redacted]' : sanitizeValue(child),
    ]),
  )
}

function stringifySummary(res: Response): Promise<string> {
  const contentType = res.headers.get('content-type') || ''
  if (contentType.includes('application/json')) {
    return res.json().then((body) => {
      if (body && typeof body === 'object') {
        const record = sanitizeValue(body) as Record<string, unknown>
        if ('error' in record) return String(record.error)
        if ('message' in record) return String(record.message)
        if ('status' in record) return `status=${String(record.status)}`
        return JSON.stringify(record).slice(0, 120)
      }
      return JSON.stringify(body).slice(0, 120)
    }).catch(() => 'json parse failed')
  }
  return res.text().then((text) => text.slice(0, 120)).catch(() => 'text parse failed')
}

async function runCase(route: RouteCase, ctx: ScriptContext): Promise<ResultRow> {
  const manifest = normalizeManifestEntry(route)

  if (route.skip) {
    return {
      id: manifest.id,
      group: manifest.family,
      subgroup: manifest.subgroup,
      name: manifest.name,
      method: manifest.method,
      path: manifest.path,
      auth: manifest.auth,
      readiness: manifest.readiness,
      executionMode: manifest.execution_mode,
      fixtureKeys: manifest.fixture_keys || [],
      manualNotes: manifest.manual_notes,
      expected: Array.isArray(route.expected) ? route.expected.join(', ') : String(route.expected),
      status: 'skipped',
      pass: true,
      ms: 0,
      summary: route.skip,
    }
  }

  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (route.auth === 'user') headers.Authorization = `Bearer ${ctx.userToken}`
  if (route.auth === 'operator') headers.Authorization = `Bearer ${ctx.operatorToken}`
  if (route.auth === 'admin') headers.Authorization = `Bearer ${ctx.adminToken}`
  Object.assign(headers, route.headers?.(ctx) || {})

  const body = route.body?.(ctx)
  const started = performance.now()
  try {
    const res = await app.request(route.path, {
      method: route.method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    }, ctx.env)
    const elapsed = Math.round(performance.now() - started)
    const summary = await stringifySummary(res)
    const pass = Array.isArray(route.expected)
      ? route.expected.includes(res.status)
      : res.status === route.expected

    return {
      id: manifest.id,
      group: manifest.family,
      subgroup: manifest.subgroup,
      name: manifest.name,
      method: manifest.method,
      path: manifest.path,
      auth: manifest.auth,
      readiness: manifest.readiness,
      executionMode: manifest.execution_mode,
      fixtureKeys: manifest.fixture_keys || [],
      manualNotes: manifest.manual_notes,
      expected: Array.isArray(route.expected) ? route.expected.join(', ') : String(route.expected),
      status: res.status,
      pass,
      ms: elapsed,
      summary,
    }
  } catch (err) {
    return {
      id: manifest.id,
      group: manifest.family,
      subgroup: manifest.subgroup,
      name: manifest.name,
      method: manifest.method,
      path: manifest.path,
      auth: manifest.auth,
      readiness: manifest.readiness,
      executionMode: manifest.execution_mode,
      fixtureKeys: manifest.fixture_keys || [],
      manualNotes: manifest.manual_notes,
      expected: Array.isArray(route.expected) ? route.expected.join(', ') : String(route.expected),
      status: 'error',
      pass: false,
      ms: Math.round(performance.now() - started),
      summary: err instanceof Error ? err.message : 'Unknown error',
    }
  }
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function normalizeManifestEntry(route: RouteCase): EndpointManifestEntry {
  const readiness = route.readiness || (route.skip ? 'inventory' : 'verified')
  const executionMode = route.executionMode || (route.skip ? 'inventory' : 'automated')
  const fixtureKeys = route.fixtureKeys || (route.auth === 'public' ? [] : DEFAULT_FIXTURES)

  return {
    id: `${route.method} ${route.path}`,
    family: route.group,
    subgroup: route.subgroup,
    name: route.name,
    method: route.method,
    path: route.path,
    auth: route.auth,
    readiness,
    execution_mode: executionMode,
    expected_status: route.expected,
    fixture_keys: fixtureKeys,
    manual_notes: route.manualNotes || route.skip,
  }
}

function renderHtmlReport(rows: ResultRow[]): string {
  const grouped = rows.reduce<Record<string, ResultRow[]>>((acc, row) => {
    (acc[row.group] ||= []).push(row)
    return acc
  }, {})

  const readinessCounts = rows.reduce<Record<EndpointReadinessTier, number>>((acc, row) => {
    acc[row.readiness] += 1
    return acc
  }, { verified: 0, seeded: 0, diagnostic: 0, manual: 0, inventory: 0 })

  const authCounts = rows.reduce<Record<EndpointAuthRequirement, number>>((acc, row) => {
    acc[row.auth] += 1
    return acc
  }, { public: 0, user: 0, operator: 0, admin: 0 })

  const summary = {
    total: rows.length,
    passed: rows.filter(row => row.pass).length,
    failed: rows.filter(row => !row.pass).length,
    skipped: rows.filter(row => row.status === 'skipped').length,
  }

  const tocItems = Object.entries(grouped).map(([group, groupRows]) => ({
    group,
    count: groupRows.length,
    anchor: `family-${group.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`,
  }))

  const sections = Object.entries(grouped).map(([group, groupRows]) => {
    const subgrouped = groupRows.reduce<Record<string, ResultRow[]>>((acc, row) => {
      const key = row.subgroup || 'general'
      ;(acc[key] ||= []).push(row)
      return acc
    }, {})

    const anchor = `family-${group.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`
    const familySummary = `${groupRows.length} routes · ${groupRows.filter(row => row.pass).length} passing · ${groupRows.filter(row => row.status === 'skipped').length} skipped`

    return `
    <section class="card family-section" id="${escapeHtml(anchor)}" data-family="${escapeHtml(group.toLowerCase())}">
      <h2>${escapeHtml(group)}</h2>
      <p class="family-summary">${escapeHtml(familySummary)}</p>
      ${Object.entries(subgrouped).map(([subgroup, subgroupRows]) => `
        <details class="subgroup" ${subgroup === 'general' ? 'open' : ''} data-subgroup="${escapeHtml(subgroup.toLowerCase())}">
          <summary>${escapeHtml(subgroup)} <span>${subgroupRows.length} routes</span></summary>
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Method</th>
                <th>Path</th>
                <th>Auth</th>
                <th>Readiness</th>
                <th>Mode</th>
                <th>Expected</th>
                <th>Status</th>
                <th>Pass</th>
                <th>Fixtures</th>
                <th>ms</th>
                <th>Summary</th>
                <th>Manual QA</th>
              </tr>
            </thead>
            <tbody>
              ${subgroupRows.map(row => `
                <tr class="route-row" data-search="${escapeHtml(`${row.group} ${row.subgroup || ''} ${row.name} ${row.path} ${row.auth} ${row.readiness} ${row.executionMode} ${row.fixtureKeys.join(' ')} ${row.manualNotes || ''}`.toLowerCase())}">
                  <td>${escapeHtml(row.name)}</td>
                  <td><code>${escapeHtml(row.method)}</code></td>
                  <td><code>${escapeHtml(row.path)}</code></td>
                  <td>${escapeHtml(row.auth)}</td>
                  <td><span class="badge readiness-${row.readiness}">${escapeHtml(READINESS_LABELS[row.readiness])}</span></td>
                  <td>${escapeHtml(row.executionMode)}</td>
                  <td>${escapeHtml(row.expected)}</td>
                  <td><span class="status ${row.status === 'error' ? 'fail' : row.status === 'skipped' ? 'skipped' : 'ok'}">${escapeHtml(String(row.status))}</span></td>
                  <td><span class="pass ${row.pass ? 'ok' : 'fail'}">${row.pass ? 'yes' : 'no'}</span></td>
                  <td>${escapeHtml(row.fixtureKeys.join(', ') || 'none')}</td>
                  <td>${row.ms}</td>
                  <td class="summary">${escapeHtml(row.summary)}</td>
                  <td class="manual-notes">${escapeHtml(row.manualNotes || '')}</td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        </details>
      `).join('')}
    </section>
  `}).join('')

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Developer Endpoint Dashboard</title>
  <style>
    :root { color-scheme: light dark; }
    body { font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; margin: 0; padding: 24px; background: #0f172a; color: #e2e8f0; }
    h1, h2, p { margin: 0 0 12px; }
    .layout { display: grid; grid-template-columns: 280px minmax(0, 1fr); gap: 20px; align-items: start; }
    .sidebar { position: sticky; top: 24px; background: #111827; border: 1px solid #334155; border-radius: 12px; padding: 16px; }
    .sidebar h2 { font-size: 16px; margin-bottom: 10px; }
    .sidebar ul { list-style: none; padding: 0; margin: 0; display: grid; gap: 8px; }
    .sidebar a { color: #cbd5e1; text-decoration: none; display: flex; justify-content: space-between; gap: 8px; }
    .sidebar a:hover { color: #fff; }
    .search-panel { margin-bottom: 16px; }
    .search-input { width: 100%; box-sizing: border-box; border-radius: 10px; border: 1px solid #334155; background: #0f172a; color: #e2e8f0; padding: 10px 12px; }
    .search-hint { color: #94a3b8; font-size: 12px; margin-top: 8px; }
    .content { min-width: 0; }
    .meta, .meta-small { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; margin-bottom: 24px; }
    .meta div, .meta-small div, .card { background: #111827; border: 1px solid #334155; border-radius: 12px; padding: 16px; }
    .meta strong, .meta-small strong { display: block; font-size: 28px; margin-bottom: 4px; }
    .meta-small strong { font-size: 18px; }
    .card { margin-bottom: 20px; overflow-x: auto; }
    .family-summary { color: #94a3b8; }
    .subgroup { margin-top: 12px; border: 1px solid #334155; border-radius: 10px; padding: 8px 12px; }
    .subgroup summary { cursor: pointer; font-weight: 600; display: flex; justify-content: space-between; }
    table { width: 100%; border-collapse: collapse; font-size: 14px; margin-top: 12px; }
    th, td { border-bottom: 1px solid #334155; padding: 10px 8px; text-align: left; vertical-align: top; }
    th { color: #94a3b8; font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
    code { background: #0f172a; padding: 2px 6px; border-radius: 6px; }
    .status, .pass, .badge { display: inline-block; padding: 2px 8px; border-radius: 999px; font-size: 12px; font-weight: 700; text-transform: uppercase; }
    .ok, .readiness-verified { background: #064e3b; color: #bbf7d0; }
    .fail { background: #7f1d1d; color: #fecaca; }
    .skipped, .readiness-inventory { background: #78350f; color: #fde68a; }
    .readiness-seeded { background: #1d4ed8; color: #bfdbfe; }
    .readiness-diagnostic { background: #5b21b6; color: #ddd6fe; }
    .readiness-manual { background: #9a3412; color: #fed7aa; }
    .summary, .manual-notes { max-width: 320px; word-break: break-word; }
    .hidden { display: none !important; }
    @media (max-width: 1100px) {
      .layout { grid-template-columns: 1fr; }
      .sidebar { position: static; }
    }
  </style>
</head>
<body>
  <div class="layout">
    <aside class="sidebar">
      <div class="search-panel">
        <h2>Search</h2>
        <input id="route-search" class="search-input" type="search" placeholder="Search family, route, path, auth, readiness..." />
        <div class="search-hint">Filter by route name, path, auth, readiness, subgroup, or fixture.</div>
      </div>
      <h2>Index</h2>
      <ul>
        ${tocItems.map(item => `<li><a href="#${escapeHtml(item.anchor)}"><span>${escapeHtml(item.group)}</span><span>${item.count}</span></a></li>`).join('')}
      </ul>
    </aside>
    <main class="content">
      <h1>Developer Endpoint Dashboard</h1>
      <p>Generated at ${escapeHtml(new Date().toISOString())}</p>
      <div class="meta">
        <div><strong>${summary.total}</strong><span>Total routes</span></div>
        <div><strong>${summary.passed}</strong><span>Passing</span></div>
        <div><strong>${summary.failed}</strong><span>Failing</span></div>
        <div><strong>${summary.skipped}</strong><span>Skipped</span></div>
      </div>
      <div class="meta-small">
        <div><strong>${readinessCounts.verified}</strong><span>Verified</span></div>
        <div><strong>${readinessCounts.seeded}</strong><span>Seeded</span></div>
        <div><strong>${readinessCounts.diagnostic}</strong><span>Diagnostic</span></div>
        <div><strong>${readinessCounts.manual}</strong><span>Manual</span></div>
        <div><strong>${readinessCounts.inventory}</strong><span>Inventory</span></div>
        <div><strong>${authCounts.public}</strong><span>Public</span></div>
        <div><strong>${authCounts.user}</strong><span>User</span></div>
        <div><strong>${authCounts.operator}</strong><span>Operator</span></div>
        <div><strong>${authCounts.admin}</strong><span>Admin</span></div>
      </div>
      ${sections}
    </main>
  </div>
  <script>
    const input = document.getElementById('route-search');
    const routeRows = Array.from(document.querySelectorAll('.route-row'));
    const familySections = Array.from(document.querySelectorAll('.family-section'));
    const subgroupDetails = Array.from(document.querySelectorAll('.subgroup'));

    function applySearch() {
      const term = input.value.trim().toLowerCase();

      routeRows.forEach((row) => {
        const haystack = row.getAttribute('data-search') || '';
        row.classList.toggle('hidden', term.length > 0 && !haystack.includes(term));
      });

      subgroupDetails.forEach((detail) => {
        const visibleRows = detail.querySelectorAll('.route-row:not(.hidden)').length;
        detail.classList.toggle('hidden', visibleRows === 0);
        if (term.length > 0 && visibleRows > 0) detail.open = true;
      });

      familySections.forEach((section) => {
        const visibleRows = section.querySelectorAll('.route-row:not(.hidden)').length;
        section.classList.toggle('hidden', visibleRows === 0);
      });
    }

    input.addEventListener('input', applySearch);
  </script>
</body>
</html>`
}

async function printResults(rows: ResultRow[]) {
  const grouped = rows.reduce<Record<string, ResultRow[]>>((acc, row) => {
    (acc[row.group] ||= []).push(row)
    return acc
  }, {})

  for (const [group, groupRows] of Object.entries(grouped)) {
    console.log(`\n## ${group}`)
    console.table(groupRows.map(row => ({
      name: row.name,
      method: row.method,
      path: row.path,
      auth: row.auth,
      readiness: row.readiness,
      mode: row.executionMode,
      expected: row.expected,
      status: row.status,
      pass: row.pass ? 'yes' : 'no',
      fixtures: row.fixtureKeys.join(', ') || 'none',
      ms: row.ms,
      summary: row.summary,
    })))
  }

  const passed = rows.filter(row => row.pass).length
  const failed = rows.filter(row => !row.pass).length
  const skipped = rows.filter(row => row.status === 'skipped').length

  console.log('\n## Summary')
  console.table([{ total: rows.length, passed, failed, skipped }])

  if (failed > 0) {
    console.log('\nFailed routes:')
    for (const row of rows.filter(r => !r.pass)) {
      console.log(`- ${row.method} ${row.path} (${row.auth}/${row.readiness}) => ${row.status} | expected ${row.expected} | ${row.summary}`)
    }
  }

  await writeFile('endpoint-visualizer-report.html', renderHtmlReport(rows), 'utf8')
  await writeFile('endpoint-visualizer-report.json', JSON.stringify({
    generated_at: new Date().toISOString(),
    summary: {
      total: rows.length,
      passed,
      failed,
      skipped,
    },
    readiness: rows.reduce<Record<EndpointReadinessTier, number>>((acc, row) => {
      acc[row.readiness] += 1
      return acc
    }, { verified: 0, seeded: 0, diagnostic: 0, manual: 0, inventory: 0 }),
    routes: rows,
  }, null, 2), 'utf8')
  console.log('\nHTML report written to endpoint-visualizer-report.html')
  console.log('JSON report written to endpoint-visualizer-report.json')
}


async function main() {
  const env = createTestEnv({ DEVELOPER_MODE: 'true' })
  const principals = await bootstrapPrincipals(env)

  const ctx: ScriptContext = {
    env,
    userToken: principals.viewer.token,
    operatorToken: principals.operator.token,
    adminToken: principals.admin.token,
  }

  const results = []
  for (const route of createCases()) {
    results.push(await runCase(route, ctx))
  }

  await printResults(results)
  return results
}

void test('endpoint visualizer', async () => {
  const results = await main()
  expect(results.filter(row => !row.pass)).toHaveLength(0)
})
