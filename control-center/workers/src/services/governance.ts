import type {
  AuthPrincipal,
  Env,
  GovernanceEvaluation,
  GovernancePolicy,
  GovernancePolicyRule,
  IncidentActionType,
  IncidentRecord,
  IncidentSeverity,
} from '../types'

const POLICY_PREFIX = 'governance:policy:'
const POLICY_INDEX_KEY = 'governance:policy:index'
const DEFAULT_POLICY_ID = 'default-approval-policy'

function nowIso(): string {
  return new Date().toISOString()
}

function policyKey(id: string): string {
  return `${POLICY_PREFIX}${id}`
}

export function getDefaultPolicy(): GovernancePolicy {
  return {
    id: DEFAULT_POLICY_ID,
    name: 'Default Incident Approval Policy',
    description: 'Default governance policy for incident approvals. Operators can approve restart_deployment actions on non-critical incidents.',
    version: 1,
    enabled: true,
    default_effect: 'deny',
    rules: [
      {
        id: 'admin-full-access',
        name: 'Admin Full Access',
        description: 'Administrators can approve any incident',
        enabled: true,
        conditions: {},
        effect: 'allow',
        principals: { roles: ['admin'] },
        priority: 100,
        created_at: nowIso(),
        updated_at: nowIso(),
      },
      {
        id: 'operator-restart-only',
        name: 'Operator Restart Approval',
        description: 'Operators can approve restart_deployment for non-critical incidents',
        enabled: true,
        conditions: {
          severity: ['low', 'medium', 'high'],
          action_types: ['restart_deployment'],
        },
        effect: 'allow',
        principals: { roles: ['operator'] },
        priority: 50,
        created_at: nowIso(),
        updated_at: nowIso(),
      },
      {
        id: 'deny-viewer',
        name: 'Deny Viewer',
        description: 'Viewers cannot approve any incidents',
        enabled: true,
        conditions: {},
        effect: 'deny',
        principals: { roles: ['viewer'] },
        priority: 10,
        created_at: nowIso(),
        updated_at: nowIso(),
      },
    ],
    created_by: 0,
    created_at: nowIso(),
    updated_at: nowIso(),
  }
}

async function getPolicyIndex(env: Env): Promise<string[]> {
  return (await env.KV.get(POLICY_INDEX_KEY, 'json') as string[] | null) || [DEFAULT_POLICY_ID]
}

async function setPolicyIndex(env: Env, ids: string[]): Promise<void> {
  await env.KV.put(POLICY_INDEX_KEY, JSON.stringify(ids), { expirationTtl: 86400 * 30 })
}

export async function getPolicy(env: Env, id: string): Promise<GovernancePolicy | null> {
  if (id === DEFAULT_POLICY_ID) {
    return getDefaultPolicy()
  }

  return await env.KV.get(policyKey(id), 'json') as GovernancePolicy | null
}

export async function listPolicies(env: Env): Promise<GovernancePolicy[]> {
  const ids = await getPolicyIndex(env)
  const policies = await Promise.all(ids.map(id => getPolicy(env, id)))
  return policies.filter((p): p is GovernancePolicy => p !== null)
}

export async function getActivePolicy(env: Env): Promise<GovernancePolicy> {
  const policies = await listPolicies(env)
  const enabled = policies.filter(p => p.enabled)
  if (enabled.length === 0) {
    return getDefaultPolicy()
  }
  // Return the most recently updated enabled policy
  return enabled.sort((a, b) =>
    new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
  )[0]
}

export interface CreatePolicyInput {
  name: string
  description?: string
  default_effect?: 'allow' | 'deny'
  rules: Array<{
    name: string
    description?: string
    enabled?: boolean
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
    priority?: number
  }>
}

export async function createPolicy(env: Env, principal: AuthPrincipal, input: CreatePolicyInput): Promise<GovernancePolicy> {
  const id = crypto.randomUUID()
  const now = nowIso()

  const rules: GovernancePolicyRule[] = input.rules.map((rule, index) => ({
    id: `${id}:rule:${index}`,
    name: rule.name,
    description: rule.description,
    enabled: rule.enabled ?? true,
    conditions: rule.conditions,
    effect: rule.effect,
    principals: rule.principals,
    priority: rule.priority ?? 50,
    created_at: now,
    updated_at: now,
  }))

  const policy: GovernancePolicy = {
    id,
    name: input.name,
    description: input.description,
    version: 1,
    enabled: true,
    default_effect: input.default_effect ?? 'deny',
    rules,
    created_by: principal.sub,
    created_at: now,
    updated_at: now,
  }

  await env.KV.put(policyKey(id), JSON.stringify(policy), { expirationTtl: 86400 * 30 })

  const ids = await getPolicyIndex(env)
  if (!ids.includes(id)) {
    ids.push(id)
    await setPolicyIndex(env, ids)
  }

  return policy
}

export interface UpdatePolicyInput {
  name?: string
  description?: string
  enabled?: boolean
  default_effect?: 'allow' | 'deny'
  rules?: Array<{
    id?: string
    name: string
    description?: string
    enabled?: boolean
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
    priority?: number
  }>
}

export async function updatePolicy(env: Env, id: string, input: UpdatePolicyInput): Promise<GovernancePolicy | null> {
  if (id === DEFAULT_POLICY_ID) {
    // Cannot modify default policy
    return null
  }

  const existing = await getPolicy(env, id)
  if (!existing) {
    return null
  }

  const now = nowIso()

  let rules = existing.rules
  if (input.rules) {
    rules = input.rules.map((rule, index) => ({
      id: rule.id || `${id}:rule:${index}`,
      name: rule.name,
      description: rule.description,
      enabled: rule.enabled ?? true,
      conditions: rule.conditions,
      effect: rule.effect,
      principals: rule.principals,
      priority: rule.priority ?? 50,
      created_at: existing.rules.find(r => r.id === rule.id)?.created_at || now,
      updated_at: now,
    }))
  }

  const updated: GovernancePolicy = {
    ...existing,
    name: input.name ?? existing.name,
    description: input.description ?? existing.description,
    enabled: input.enabled ?? existing.enabled,
    default_effect: input.default_effect ?? existing.default_effect,
    version: existing.version + 1,
    rules,
    updated_at: now,
  }

  await env.KV.put(policyKey(id), JSON.stringify(updated), { expirationTtl: 86400 * 30 })
  return updated
}

export async function deletePolicy(env: Env, id: string): Promise<boolean> {
  if (id === DEFAULT_POLICY_ID) {
    return false
  }

  const existing = await getPolicy(env, id)
  if (!existing) {
    return false
  }

  await env.KV.delete(policyKey(id))

  const ids = await getPolicyIndex(env)
  const newIds = ids.filter(i => i !== id)
  await setPolicyIndex(env, newIds)

  return true
}

function matchesRule(
  rule: GovernancePolicyRule,
  principal: AuthPrincipal,
  incident: IncidentRecord
): boolean {
  if (!rule.enabled) {
    return false
  }

  // Check principal match
  const principalMatch =
    (rule.principals.roles?.includes(principal.role as 'admin' | 'operator' | 'viewer')) ||
    (rule.principals.user_ids?.includes(principal.sub))

  if (!principalMatch) {
    return false
  }

  // Check severity condition
  if (rule.conditions.severity?.length && !rule.conditions.severity.includes(incident.severity)) {
    return false
  }

  // Check action_type condition
  if (rule.conditions.action_types?.length && incident.action_type) {
    if (!rule.conditions.action_types.includes(incident.action_type)) {
      return false
    }
  }

  // Check source condition
  if (rule.conditions.sources?.length && !rule.conditions.sources.includes(incident.source)) {
    return false
  }

  return true
}

export function evaluatePolicy(
  policy: GovernancePolicy,
  principal: AuthPrincipal,
  incident: IncidentRecord
): GovernanceEvaluation {
  const matchedRules: string[] = []

  // Sort rules by priority (highest first)
  const sortedRules = [...policy.rules].sort((a, b) => b.priority - a.priority)

  let finalEffect = policy.default_effect

  for (const rule of sortedRules) {
    if (matchesRule(rule, principal, incident)) {
      matchedRules.push(rule.id)

      // First matching rule wins (by priority)
      if (finalEffect === policy.default_effect) {
        finalEffect = rule.effect
      }

      // If a deny rule matches, immediately deny (deny takes precedence)
      if (rule.effect === 'deny') {
        return {
          allowed: false,
          matched_rules: matchedRules,
          evaluation_time: nowIso(),
          policy_id: policy.id,
          policy_version: policy.version,
        }
      }
    }
  }

  return {
    allowed: finalEffect === 'allow',
    matched_rules: matchedRules,
    evaluation_time: nowIso(),
    policy_id: policy.id,
    policy_version: policy.version,
  }
}

export async function canApproveWithPolicy(
  env: Env,
  principal: AuthPrincipal,
  incident: IncidentRecord
): Promise<GovernanceEvaluation> {
  const policy = await getActivePolicy(env)
  return evaluatePolicy(policy, principal, incident)
}

export function canExecuteWithPolicy(
  env: Env,
  principal: AuthPrincipal,
  incident: IncidentRecord
): Promise<GovernanceEvaluation> {
  // Execution uses the same policy as approval for now
  return canApproveWithPolicy(env, principal, incident)
}