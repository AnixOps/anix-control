/**
 * Tenant API Handlers
 */

import type { Context } from 'hono'
import type { Env } from '../types'
import { z } from 'zod'
import {
  createTenant,
  getTenant,
  updateTenant,
  deleteTenant,
  getTenantMembers,
  addTenantMember,
  removeTenantMember,
  createRole,
  updateRole,
  deleteRole,
  getTenantQuotas,
  getTenantUsage,
  checkQuota,
  createInvitation,
  acceptInvitation,
  getTenantInvitations,
  deleteInvitation,
  getAllPermissions,
  getRoleById,
  hasPermission,
  DEFAULT_QUOTAS,
  type TenantQuotas,
} from '../services/tenant'
import { logAudit } from '../utils/audit'

// Validation schemas
const createTenantSchema = z.object({
  name: z.string().min(1).max(100),
  slug: z.string().min(1).max(50).regex(/^[a-z0-9-]+$/),
  plan: z.enum(['free', 'pro', 'enterprise']).optional(),
  billing_email: z.string().email().optional(),
})

const updateTenantSchema = z.object({
  name: z.string().min(1).max(100).optional(),
  plan: z.enum(['free', 'pro', 'enterprise']).optional(),
  status: z.enum(['active', 'suspended', 'cancelled']).optional(),
  settings: z.record(z.string(), z.unknown()).optional(),
  billing_email: z.string().email().optional(),
})

const createRoleSchema = z.object({
  name: z.string().min(1).max(50).regex(/^[a-z0-9_]+$/),
  display_name: z.string().min(1).max(100).optional(),
  description: z.string().max(500).optional(),
  permissions: z.array(z.string()),
})

const updateRoleSchema = z.object({
  name: z.string().min(1).max(50).regex(/^[a-z0-9_]+$/).optional(),
  display_name: z.string().min(1).max(100).optional(),
  description: z.string().max(500).optional(),
  permissions: z.array(z.string()).optional(),
})

const inviteMemberSchema = z.object({
  email: z.string().email(),
  role_id: z.number().int().positive().optional(),
})

const acceptInvitationSchema = z.object({
  token: z.string().uuid(),
})

/**
 * List tenants for current user
 */
export async function listTenantsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  const tenants = await c.env.DB
    .prepare(`
      SELECT t.*, r.name as role_name
      FROM tenants t
      JOIN tenant_members tm ON t.id = tm.tenant_id
      LEFT JOIN roles r ON tm.role_id = r.id
      WHERE tm.user_id = ? AND t.status = 'active'
      ORDER BY t.name
    `)
    .bind(user.sub)
    .all()

  return c.json({
    success: true,
    data: tenants.results,
  })
}

/**
 * Create a new tenant
 */
export async function createTenantHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const data = createTenantSchema.parse(body)

    // Check if slug is already taken
    const existing = await c.env.DB
      .prepare('SELECT id FROM tenants WHERE slug = ?')
      .bind(data.slug)
      .first()

    if (existing) {
      return c.json({ success: false, error: 'Tenant slug already taken' }, 409)
    }

    // Create tenant
    const tenant = await createTenant(c.env, {
      name: data.name,
      slug: data.slug,
      plan: data.plan,
      billing_email: data.billing_email,
    })

    // Add creator as admin
    const adminRole = await c.env.DB
      .prepare("SELECT id FROM roles WHERE name = 'admin' AND tenant_id IS NULL")
      .first<{ id: number }>()

    await addTenantMember(c.env, tenant.id, user.sub, adminRole?.id)

    await logAudit(c, user.sub, 'create_tenant', 'tenant', {
      tenant_id: tenant.id,
      name: tenant.name,
      slug: tenant.slug,
    })

    return c.json({
      success: true,
      data: tenant,
    }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Get tenant details
 */
export async function getTenantHandler(c: Context<{ Bindings: Env }>) {
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  const tenant = await getTenant(c.env, tenantId)

  if (!tenant) {
    return c.json({ success: false, error: 'Tenant not found' }, 404)
  }

  return c.json({
    success: true,
    data: tenant,
  })
}

/**
 * Update tenant
 */
export async function updateTenantHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'settings:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  try {
    const body = await c.req.json()
    const data = updateTenantSchema.parse(body)

    const tenant = await updateTenant(c.env, tenantId, data)

    if (!tenant) {
      return c.json({ success: false, error: 'Tenant not found' }, 404)
    }

    await logAudit(c, user.sub, 'update_tenant', 'tenant', {
      tenant_id: tenantId,
      changes: data,
    })

    return c.json({
      success: true,
      data: tenant,
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Delete tenant
 */
export async function deleteTenantHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'settings:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const success = await deleteTenant(c.env, tenantId)

  await logAudit(c, user.sub, 'delete_tenant', 'tenant', {
    tenant_id: tenantId,
  })

  return c.json({
    success,
    message: 'Tenant deleted successfully',
  })
}

/**
 * Get tenant members
 */
export async function getTenantMembersHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check if user is member
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'users:read')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const members = await getTenantMembers(c.env, tenantId)

  return c.json({
    success: true,
    data: members,
  })
}

/**
 * Add tenant member
 */
export async function addTenantMemberHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'users:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  try {
    const body = await c.req.json()
    const { user_id, role_id } = body

    // Check quota
    const quotaCheck = await checkQuota(c.env, tenantId, 'max_users')
    if (!quotaCheck.allowed) {
      return c.json({
        success: false,
        error: 'User quota exceeded',
        current: quotaCheck.current,
        limit: quotaCheck.limit,
      }, 403)
    }

    const member = await addTenantMember(c.env, tenantId, user_id, role_id, user.sub)

    await logAudit(c, user.sub, 'add_tenant_member', 'tenant', {
      tenant_id: tenantId,
      user_id,
    })

    return c.json({
      success: true,
      data: member,
    }, 201)
  } catch (err) {
    throw err
  }
}

/**
 * Remove tenant member
 */
export async function removeTenantMemberHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)
  const memberId = parseInt(c.req.param('memberId') as string, 10)

  if (isNaN(tenantId) || isNaN(memberId)) {
    return c.json({ success: false, error: 'Invalid ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'users:delete')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const success = await removeTenantMember(c.env, tenantId, memberId)

  await logAudit(c, user.sub, 'remove_tenant_member', 'tenant', {
    tenant_id: tenantId,
    user_id: memberId,
  })

  return c.json({
    success,
    message: 'Member removed successfully',
  })
}

/**
 * Get tenant quotas and usage
 */
export async function getTenantQuotasHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check if user is member
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'settings:read')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const [quotas, usage] = await Promise.all([
    getTenantQuotas(c.env, tenantId),
    getTenantUsage(c.env, tenantId),
  ])

  return c.json({
    success: true,
    data: {
      quotas,
      usage,
    },
  })
}

/**
 * List tenant roles
 */
export async function listTenantRolesHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'users:read')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const roles = await c.env.DB
    .prepare(`
      SELECT * FROM roles
      WHERE tenant_id = ? OR tenant_id IS NULL
      ORDER BY is_system DESC, name
    `)
    .bind(tenantId)
    .all()

  return c.json({
    success: true,
    data: roles.results,
  })
}

/**
 * Create custom role
 */
export async function createTenantRoleHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'settings:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  try {
    const body = await c.req.json()
    const data = createRoleSchema.parse(body)

    // Check if role name exists
    const existing = await c.env.DB
      .prepare('SELECT id FROM roles WHERE name = ? AND (tenant_id = ? OR tenant_id IS NULL)')
      .bind(data.name, tenantId)
      .first()

    if (existing) {
      return c.json({ success: false, error: 'Role name already exists' }, 409)
    }

    const role = await createRole(c.env, tenantId, data)

    await logAudit(c, user.sub, 'create_role', 'role', {
      tenant_id: tenantId,
      role_id: role.id,
      role_name: role.name,
    })

    return c.json({
      success: true,
      data: role,
    }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Update custom role
 */
export async function updateTenantRoleHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)
  const roleId = parseInt(c.req.param('roleId') as string, 10)

  if (isNaN(tenantId) || isNaN(roleId)) {
    return c.json({ success: false, error: 'Invalid ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'settings:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  // Check if role belongs to tenant
  const role = await getRoleById(c.env, roleId)
  if (!role || role.tenant_id !== tenantId) {
    return c.json({ success: false, error: 'Role not found' }, 404)
  }

  if (role.is_system) {
    return c.json({ success: false, error: 'Cannot modify system role' }, 400)
  }

  try {
    const body = await c.req.json()
    const data = updateRoleSchema.parse(body)

    const updatedRole = await updateRole(c.env, roleId, tenantId, data)

    await logAudit(c, user.sub, 'update_role', 'role', {
      tenant_id: tenantId,
      role_id: roleId,
    })

    return c.json({
      success: true,
      data: updatedRole,
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Delete custom role
 */
export async function deleteTenantRoleHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)
  const roleId = parseInt(c.req.param('roleId') as string, 10)

  if (isNaN(tenantId) || isNaN(roleId)) {
    return c.json({ success: false, error: 'Invalid ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'settings:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const success = await deleteRole(c.env, roleId, tenantId)

  if (!success) {
    return c.json({ success: false, error: 'Cannot delete role' }, 400)
  }

  await logAudit(c, user.sub, 'delete_role', 'role', {
    tenant_id: tenantId,
    role_id: roleId,
  })

  return c.json({
    success: true,
    message: 'Role deleted successfully',
  })
}

/**
 * List all available permissions
 */
export async function listPermissionsHandler(c: Context<{ Bindings: Env }>) {
  const permissions = await getAllPermissions(c.env)

  return c.json({
    success: true,
    data: permissions,
  })
}

/**
 * Invite member to tenant
 */
export async function inviteMemberHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'users:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  try {
    const body = await c.req.json()
    const data = inviteMemberSchema.parse(body)

    // Check quota
    const quotaCheck = await checkQuota(c.env, tenantId, 'max_users')
    if (!quotaCheck.allowed) {
      return c.json({
        success: false,
        error: 'User quota exceeded',
      }, 403)
    }

    const invitation = await createInvitation(
      c.env,
      tenantId,
      data.email,
      data.role_id,
      user.sub
    )

    await logAudit(c, user.sub, 'create_invitation', 'tenant', {
      tenant_id: tenantId,
      email: data.email,
    })

    // In a real app, send email here

    return c.json({
      success: true,
      data: {
        id: invitation.id,
        email: invitation.email,
        token: invitation.token, // Only show in development
        expires_at: invitation.expires_at,
      },
    }, 201)
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * Accept invitation
 */
export async function acceptInvitationHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')

  try {
    const body = await c.req.json()
    const data = acceptInvitationSchema.parse(body)

    const result = await acceptInvitation(c.env, data.token, user.sub)

    if (!result.success) {
      return c.json({ success: false, error: result.error }, 400)
    }

    await logAudit(c, user.sub, 'accept_invitation', 'tenant', {
      tenant_id: result.tenantId,
    })

    return c.json({
      success: true,
      message: 'Invitation accepted',
      data: { tenant_id: result.tenantId },
    })
  } catch (err) {
    if (err instanceof z.ZodError) {
      return c.json({ success: false, error: 'Validation error', details: err.issues }, 400)
    }
    throw err
  }
}

/**
 * List tenant invitations
 */
export async function listTenantInvitationsHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)

  if (isNaN(tenantId)) {
    return c.json({ success: false, error: 'Invalid tenant ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'users:read')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const invitations = await getTenantInvitations(c.env, tenantId)

  return c.json({
    success: true,
    data: invitations,
  })
}

/**
 * Cancel invitation
 */
export async function cancelInvitationHandler(c: Context<{ Bindings: Env }>) {
  const user = c.get('user')
  const tenantId = parseInt(c.req.param('id') as string, 10)
  const invitationId = parseInt(c.req.param('invitationId') as string, 10)

  if (isNaN(tenantId) || isNaN(invitationId)) {
    return c.json({ success: false, error: 'Invalid ID' }, 400)
  }

  // Check permission
  const hasAccess = await hasPermission(c.env, user.sub, tenantId, 'users:write')
  if (!hasAccess) {
    return c.json({ success: false, error: 'Permission denied' }, 403)
  }

  const success = await deleteInvitation(c.env, invitationId)

  await logAudit(c, user.sub, 'cancel_invitation', 'tenant', {
    tenant_id: tenantId,
    invitation_id: invitationId,
  })

  return c.json({
    success,
    message: 'Invitation cancelled',
  })
}