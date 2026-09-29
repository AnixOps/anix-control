import type { Context } from 'hono'
import { z } from 'zod'
import type { ApiErrorResponse, Env } from '../types'
import { logAudit } from '../utils/audit'
import {
  createPolicy,
  deletePolicy,
  getActivePolicy,
  getPolicy,
  listPolicies,
  type CreatePolicyInput,
  updatePolicy,
  type UpdatePolicyInput,
} from '../services/governance'

const createRuleSchema = z.object({
  name: z.string().min(1),
  description: z.string().optional(),
  enabled: z.boolean().optional(),
  conditions: z.object({
    severity: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
    action_types: z.array(z.enum(['scale_policy', 'restart_deployment'])).optional(),
    sources: z.array(z.string()).optional(),
  }),
  effect: z.enum(['allow', 'deny']),
  principals: z.object({
    roles: z.array(z.enum(['admin', 'operator', 'viewer'])).optional(),
    user_ids: z.array(z.number().int()).optional(),
  }),
  priority: z.number().int().optional(),
})

const createPolicySchema = z.object({
  name: z.string().min(1),
  description: z.string().optional(),
  default_effect: z.enum(['allow', 'deny']).optional(),
  rules: z.array(createRuleSchema).min(1),
})

const updateRuleSchema = z.object({
  id: z.string().optional(),
  name: z.string().min(1),
  description: z.string().optional(),
  enabled: z.boolean().optional(),
  conditions: z.object({
    severity: z.array(z.enum(['low', 'medium', 'high', 'critical'])).optional(),
    action_types: z.array(z.enum(['scale_policy', 'restart_deployment'])).optional(),
    sources: z.array(z.string()).optional(),
  }),
  effect: z.enum(['allow', 'deny']),
  principals: z.object({
    roles: z.array(z.enum(['admin', 'operator', 'viewer'])).optional(),
    user_ids: z.array(z.number().int()).optional(),
  }),
  priority: z.number().int().optional(),
})

const updatePolicySchema = z.object({
  name: z.string().min(1).optional(),
  description: z.string().optional(),
  enabled: z.boolean().optional(),
  default_effect: z.enum(['allow', 'deny']).optional(),
  rules: z.array(updateRuleSchema).min(1).optional(),
})

async function requirePolicy(c: Context<{ Bindings: Env }>, id: string) {
  const policy = await getPolicy(c.env, id)

  if (!policy) {
    return c.json({ success: false, error: 'Policy not found' } as ApiErrorResponse, 404)
  }

  return policy
}

export async function listPoliciesHandler(c: Context<{ Bindings: Env }>) {
  const policies = await listPolicies(c.env)

  return c.json({
    success: true,
    data: policies.map(p => ({
      id: p.id,
      name: p.name,
      description: p.description,
      version: p.version,
      enabled: p.enabled,
      default_effect: p.default_effect,
      rules_count: p.rules.length,
      created_by: p.created_by,
      created_at: p.created_at,
      updated_at: p.updated_at,
    })),
  })
}

export async function getPolicyHandler(c: Context<{ Bindings: Env }>) {
  const policyId = c.req.param('id') as string
  const policy = await requirePolicy(c, policyId)

  if (policy instanceof Response) {
    return policy
  }

  return c.json({ success: true, data: policy })
}

export async function getActivePolicyHandler(c: Context<{ Bindings: Env }>) {
  const policy = await getActivePolicy(c.env)
  return c.json({ success: true, data: policy })
}

export async function createPolicyHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')

  try {
    const body = createPolicySchema.parse(await c.req.json())
    const policy = await createPolicy(c.env, principal, body as CreatePolicyInput)

    await logAudit(c, principal.sub, 'create_governance_policy', 'governance_policy', {
      policy_id: policy.id,
      policy_name: policy.name,
      rules_count: policy.rules.length,
    })

    return c.json({ success: true, data: policy }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

export async function updatePolicyHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const policyId = c.req.param('id') as string

  if (policyId === 'default-approval-policy') {
    return c.json({ success: false, error: 'Cannot modify default policy' } as ApiErrorResponse, 403)
  }

  const existing = await requirePolicy(c, policyId)
  if (existing instanceof Response) {
    return existing
  }

  try {
    const body = updatePolicySchema.parse(await c.req.json())
    const updated = await updatePolicy(c.env, policyId, body as UpdatePolicyInput)

    if (!updated) {
      return c.json({ success: false, error: 'Failed to update policy' } as ApiErrorResponse, 400)
    }

    await logAudit(c, principal.sub, 'update_governance_policy', 'governance_policy', {
      policy_id: updated.id,
      policy_name: updated.name,
      version: updated.version,
    })

    return c.json({ success: true, data: updated })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

export async function deletePolicyHandler(c: Context<{ Bindings: Env }>) {
  const principal = c.get('user')
  const policyId = c.req.param('id') as string

  if (policyId === 'default-approval-policy') {
    return c.json({ success: false, error: 'Cannot delete default policy' } as ApiErrorResponse, 403)
  }

  const existing = await requirePolicy(c, policyId)
  if (existing instanceof Response) {
    return existing
  }

  const deleted = await deletePolicy(c.env, policyId)

  if (!deleted) {
    return c.json({ success: false, error: 'Failed to delete policy' } as ApiErrorResponse, 400)
  }

  await logAudit(c, principal.sub, 'delete_governance_policy', 'governance_policy', {
    policy_id: policyId,
  })

  return c.json({ success: true })
}