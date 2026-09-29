/**
 * Tenant Service
 *
 * Handles multi-tenancy operations including:
 * - Tenant CRUD
 * - Membership management
 * - Permission checking
 * - Resource quota enforcement
 */

import type { Env } from '../types'

// Tenant types
export interface Tenant {
  id: number
  name: string
  slug: string
  plan: 'free' | 'pro' | 'enterprise'
  status: 'active' | 'suspended' | 'cancelled'
  settings?: string
  quotas?: string
  billing_email?: string
  stripe_customer_id?: string
  trial_ends_at?: string
  created_at: string
  updated_at: string
}

export interface TenantMember {
  id: number
  tenant_id: number
  user_id: number
  role_id?: number
  invited_by?: number
  joined_at: string
}

export interface Role {
  id: number
  tenant_id?: number
  name: string
  display_name?: string
  description?: string
  permissions: string
  is_system: boolean
  created_at: string
  updated_at: string
}

export interface Permission {
  id: number
  name: string
  display_name?: string
  category?: string
  description?: string
}

export interface TenantInvitation {
  id: number
  tenant_id: number
  email: string
  role_id?: number
  invited_by: number
  token: string
  expires_at: string
  accepted_at?: string
  created_at: string
}

export interface TenantQuotas {
  max_nodes: number
  max_users: number
  max_playbooks: number
  max_schedules: number
  storage_gb: number
  api_calls_per_month: number
}

export interface TenantUsage {
  nodes_count: number
  users_count: number
  playbooks_count: number
  tasks_count: number
  storage_bytes: number
  api_calls_count: number
}

// Default quotas per plan
export const DEFAULT_QUOTAS: Record<string, TenantQuotas> = {
  free: {
    max_nodes: 3,
    max_users: 1,
    max_playbooks: 3,
    max_schedules: 5,
    storage_gb: 1,
    api_calls_per_month: 10000,
  },
  pro: {
    max_nodes: 25,
    max_users: 5,
    max_playbooks: 20,
    max_schedules: 50,
    storage_gb: 10,
    api_calls_per_month: 100000,
  },
  enterprise: {
    max_nodes: -1, // Unlimited
    max_users: -1,
    max_playbooks: -1,
    max_schedules: -1,
    storage_gb: 100,
    api_calls_per_month: -1,
  },
}

/**
 * Create a new tenant
 */
export async function createTenant(
  env: Env,
  data: {
    name: string
    slug: string
    plan?: 'free' | 'pro' | 'enterprise'
    billing_email?: string
  }
): Promise<Tenant> {
  const quotas = JSON.stringify(DEFAULT_QUOTAS[data.plan || 'free'])

  const result = await env.DB
    .prepare(`
      INSERT INTO tenants (name, slug, plan, quotas, billing_email)
      VALUES (?, ?, ?, ?, ?)
      RETURNING *
    `)
    .bind(data.name, data.slug, data.plan || 'free', quotas, data.billing_email || null)
    .first<Tenant>()

  if (!result) {
    throw new Error('Failed to create tenant')
  }

  return result
}

/**
 * Get tenant by ID
 */
export async function getTenant(env: Env, tenantId: number): Promise<Tenant | null> {
  return await env.DB
    .prepare('SELECT * FROM tenants WHERE id = ?')
    .bind(tenantId)
    .first<Tenant>()
}

/**
 * Get tenant by slug
 */
export async function getTenantBySlug(env: Env, slug: string): Promise<Tenant | null> {
  return await env.DB
    .prepare('SELECT * FROM tenants WHERE slug = ?')
    .bind(slug)
    .first<Tenant>()
}

/**
 * Update tenant
 */
export async function updateTenant(
  env: Env,
  tenantId: number,
  data: Partial<{
    name: string
    plan: 'free' | 'pro' | 'enterprise'
    status: 'active' | 'suspended' | 'cancelled'
    settings: Record<string, unknown>
    quotas: TenantQuotas
    billing_email: string
  }>
): Promise<Tenant | null> {
  const updates: string[] = []
  const values: (string | number | null)[] = []

  if (data.name !== undefined) {
    updates.push('name = ?')
    values.push(data.name)
  }
  if (data.plan !== undefined) {
    updates.push('plan = ?')
    values.push(data.plan)
  }
  if (data.status !== undefined) {
    updates.push('status = ?')
    values.push(data.status)
  }
  if (data.settings !== undefined) {
    updates.push('settings = ?')
    values.push(JSON.stringify(data.settings))
  }
  if (data.quotas !== undefined) {
    updates.push('quotas = ?')
    values.push(JSON.stringify(data.quotas))
  }
  if (data.billing_email !== undefined) {
    updates.push('billing_email = ?')
    values.push(data.billing_email)
  }

  if (updates.length === 0) {
    return getTenant(env, tenantId)
  }

  updates.push("updated_at = datetime('now')")
  values.push(tenantId)

  await env.DB
    .prepare(`UPDATE tenants SET ${updates.join(', ')} WHERE id = ?`)
    .bind(...values)
    .run()

  return getTenant(env, tenantId)
}

/**
 * Delete tenant (soft delete by setting status)
 */
export async function deleteTenant(env: Env, tenantId: number): Promise<boolean> {
  const result = await env.DB
    .prepare("UPDATE tenants SET status = 'cancelled', updated_at = datetime('now') WHERE id = ?")
    .bind(tenantId)
    .run()

  return result.success
}

/**
 * Add user to tenant
 */
export async function addTenantMember(
  env: Env,
  tenantId: number,
  userId: number,
  roleId?: number,
  invitedBy?: number
): Promise<TenantMember> {
  const result = await env.DB
    .prepare(`
      INSERT INTO tenant_members (tenant_id, user_id, role_id, invited_by)
      VALUES (?, ?, ?, ?)
      RETURNING *
    `)
    .bind(tenantId, userId, roleId || null, invitedBy || null)
    .first<TenantMember>()

  if (!result) {
    throw new Error('Failed to add tenant member')
  }

  return result
}

/**
 * Remove user from tenant
 */
export async function removeTenantMember(
  env: Env,
  tenantId: number,
  userId: number
): Promise<boolean> {
  const result = await env.DB
    .prepare('DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?')
    .bind(tenantId, userId)
    .run()

  return result.success
}

/**
 * Get tenant members
 */
export async function getTenantMembers(
  env: Env,
  tenantId: number
): Promise<Array<TenantMember & { email: string; role_name?: string }>> {
  const result = await env.DB
    .prepare(`
      SELECT tm.*, u.email, r.name as role_name
      FROM tenant_members tm
      JOIN users u ON tm.user_id = u.id
      LEFT JOIN roles r ON tm.role_id = r.id
      WHERE tm.tenant_id = ?
      ORDER BY tm.joined_at ASC
    `)
    .bind(tenantId)
    .all()

  return result.results as unknown as Array<TenantMember & { email: string; role_name?: string }>
}

/**
 * Get user's tenant membership
 */
export async function getUserTenantMembership(
  env: Env,
  userId: number,
  tenantId: number
): Promise<TenantMember | null> {
  return await env.DB
    .prepare('SELECT * FROM tenant_members WHERE user_id = ? AND tenant_id = ?')
    .bind(userId, tenantId)
    .first<TenantMember>()
}

/**
 * Get user's tenants
 */
export async function getUserTenants(
  env: Env,
  userId: number
): Promise<Array<Tenant & { role_name?: string }>> {
  const result = await env.DB
    .prepare(`
      SELECT t.*, r.name as role_name
      FROM tenants t
      JOIN tenant_members tm ON t.id = tm.tenant_id
      LEFT JOIN roles r ON tm.role_id = r.id
      WHERE tm.user_id = ? AND t.status = 'active'
      ORDER BY t.name
    `)
    .bind(userId)
    .all()

  return result.results as unknown as Array<Tenant & { role_name?: string }>
}

/**
 * Get role by name
 */
export async function getRoleByName(
  env: Env,
  name: string,
  tenantId?: number
): Promise<Role | null> {
  if (tenantId) {
    return await env.DB
      .prepare('SELECT * FROM roles WHERE name = ? AND (tenant_id = ? OR tenant_id IS NULL)')
      .bind(name, tenantId)
      .first<Role>()
  }

  return await env.DB
    .prepare('SELECT * FROM roles WHERE name = ? AND tenant_id IS NULL')
    .bind(name)
    .first<Role>()
}

/**
 * Get role by ID
 */
export async function getRoleById(env: Env, roleId: number): Promise<Role | null> {
  return await env.DB
    .prepare('SELECT * FROM roles WHERE id = ?')
    .bind(roleId)
    .first<Role>()
}

/**
 * Create custom role
 */
export async function createRole(
  env: Env,
  tenantId: number,
  data: {
    name: string
    display_name?: string
    description?: string
    permissions: string[]
  }
): Promise<Role> {
  const result = await env.DB
    .prepare(`
      INSERT INTO roles (tenant_id, name, display_name, description, permissions)
      VALUES (?, ?, ?, ?, ?)
      RETURNING *
    `)
    .bind(tenantId, data.name, data.display_name || null, data.description || null, JSON.stringify(data.permissions))
    .first<Role>()

  if (!result) {
    throw new Error('Failed to create role')
  }

  return result
}

/**
 * Update role
 */
export async function updateRole(
  env: Env,
  roleId: number,
  tenantId: number,
  data: Partial<{
    name: string
    display_name: string
    description: string
    permissions: string[]
  }>
): Promise<Role | null> {
  const updates: string[] = []
  const values: (string | number | null)[] = []

  if (data.name !== undefined) {
    updates.push('name = ?')
    values.push(data.name)
  }
  if (data.display_name !== undefined) {
    updates.push('display_name = ?')
    values.push(data.display_name)
  }
  if (data.description !== undefined) {
    updates.push('description = ?')
    values.push(data.description)
  }
  if (data.permissions !== undefined) {
    updates.push('permissions = ?')
    values.push(JSON.stringify(data.permissions))
  }

  if (updates.length === 0) {
    return getRoleById(env, roleId)
  }

  updates.push("updated_at = datetime('now')")
  values.push(roleId, tenantId)

  await env.DB
    .prepare(`UPDATE roles SET ${updates.join(', ')} WHERE id = ? AND tenant_id = ?`)
    .bind(...values)
    .run()

  return getRoleById(env, roleId)
}

/**
 * Delete custom role
 */
export async function deleteRole(env: Env, roleId: number, tenantId: number): Promise<boolean> {
  // Cannot delete system roles
  const role = await getRoleById(env, roleId)
  if (!role || role.is_system) {
    return false
  }

  const result = await env.DB
    .prepare('DELETE FROM roles WHERE id = ? AND tenant_id = ?')
    .bind(roleId, tenantId)
    .run()

  return result.success
}

/**
 * Get all permissions
 */
export async function getAllPermissions(env: Env): Promise<Permission[]> {
  const result = await env.DB
    .prepare('SELECT * FROM permissions ORDER BY category, name')
    .all<Permission>()

  return result.results
}

/**
 * Check if user has permission
 */
export async function hasPermission(
  env: Env,
  userId: number,
  tenantId: number,
  permission: string
): Promise<boolean> {
  // Get user's role in tenant
  const membership = await getUserTenantMembership(env, userId, tenantId)
  if (!membership) return false

  // Get role
  const role = membership.role_id
    ? await getRoleById(env, membership.role_id)
    : await getRoleByName(env, 'viewer', tenantId)

  if (!role) return false

  // Parse permissions
  let permissions: string[] = []
  try {
    permissions = JSON.parse(role.permissions)
  } catch {
    return false
  }

  // Check for wildcard permission
  if (permissions.includes('*')) return true

  // Check specific permission
  return permissions.includes(permission)
}

/**
 * Check multiple permissions
 */
export async function hasAnyPermission(
  env: Env,
  userId: number,
  tenantId: number,
  permissions: string[]
): Promise<boolean> {
  for (const permission of permissions) {
    if (await hasPermission(env, userId, tenantId, permission)) {
      return true
    }
  }
  return false
}

/**
 * Get tenant quotas
 */
export async function getTenantQuotas(env: Env, tenantId: number): Promise<TenantQuotas | null> {
  const tenant = await getTenant(env, tenantId)
  if (!tenant) return null

  try {
    return JSON.parse(tenant.quotas || '{}')
  } catch {
    return DEFAULT_QUOTAS[tenant.plan] || DEFAULT_QUOTAS.free
  }
}

/**
 * Get tenant current usage
 */
export async function getTenantUsage(env: Env, tenantId: number): Promise<TenantUsage> {
  // Get counts from various tables
  const [nodes, users, playbooks, tasks] = await Promise.all([
    env.DB.prepare('SELECT COUNT(*) as count FROM nodes WHERE tenant_id = ?').bind(tenantId).first<{ count: number }>(),
    env.DB.prepare('SELECT COUNT(*) as count FROM tenant_members WHERE tenant_id = ?').bind(tenantId).first<{ count: number }>(),
    env.DB.prepare('SELECT COUNT(*) as count FROM playbooks WHERE tenant_id = ?').bind(tenantId).first<{ count: number }>(),
    env.DB.prepare('SELECT COUNT(*) as count FROM tasks WHERE tenant_id = ?').bind(tenantId).first<{ count: number }>(),
  ])

  return {
    nodes_count: nodes?.count || 0,
    users_count: users?.count || 0,
    playbooks_count: playbooks?.count || 0,
    tasks_count: tasks?.count || 0,
    storage_bytes: 0, // Would need R2 integration
    api_calls_count: 0, // Would need KV counter
  }
}

/**
 * Check if tenant is within quota limits
 */
export async function checkQuota(
  env: Env,
  tenantId: number,
  resource: keyof TenantQuotas,
  requested: number = 1
): Promise<{ allowed: boolean; current: number; limit: number }> {
  const quotas = await getTenantQuotas(env, tenantId)
  if (!quotas) {
    return { allowed: false, current: 0, limit: 0 }
  }

  const limit = quotas[resource] as number
  if (limit === -1) {
    return { allowed: true, current: 0, limit: -1 } // Unlimited
  }

  const usage = await getTenantUsage(env, tenantId)

  const currentMap: Record<string, number> = {
    max_nodes: usage.nodes_count,
    max_users: usage.users_count,
    max_playbooks: usage.playbooks_count,
    max_schedules: 0, // Would need schedules count
    storage_gb: Math.ceil(usage.storage_bytes / (1024 * 1024 * 1024)),
    api_calls_per_month: usage.api_calls_count,
  }

  const current = currentMap[resource] || 0
  const allowed = limit === -1 || current + requested <= limit

  return { allowed, current, limit }
}

/**
 * Create tenant invitation
 */
export async function createInvitation(
  env: Env,
  tenantId: number,
  email: string,
  roleId: number | undefined,
  invitedBy: number,
  expiresHours: number = 72
): Promise<TenantInvitation> {
  const token = crypto.randomUUID()
  const expiresAt = new Date(Date.now() + expiresHours * 60 * 60 * 1000).toISOString()

  const result = await env.DB
    .prepare(`
      INSERT INTO tenant_invitations (tenant_id, email, role_id, invited_by, token, expires_at)
      VALUES (?, ?, ?, ?, ?, ?)
      RETURNING *
    `)
    .bind(tenantId, email, roleId || null, invitedBy, token, expiresAt)
    .first<TenantInvitation>()

  if (!result) {
    throw new Error('Failed to create invitation')
  }

  return result
}

/**
 * Get invitation by token
 */
export async function getInvitationByToken(env: Env, token: string): Promise<TenantInvitation | null> {
  return await env.DB
    .prepare('SELECT * FROM tenant_invitations WHERE token = ?')
    .bind(token)
    .first<TenantInvitation>()
}

/**
 * Accept invitation
 */
export async function acceptInvitation(
  env: Env,
  token: string,
  userId: number
): Promise<{ success: boolean; tenantId?: number; error?: string }> {
  const invitation = await getInvitationByToken(env, token)

  if (!invitation) {
    return { success: false, error: 'Invitation not found' }
  }

  if (invitation.accepted_at) {
    return { success: false, error: 'Invitation already accepted' }
  }

  if (new Date(invitation.expires_at) < new Date()) {
    return { success: false, error: 'Invitation has expired' }
  }

  // Add user to tenant
  await addTenantMember(env, invitation.tenant_id, userId, invitation.role_id, invitation.invited_by)

  // Mark invitation as accepted
  await env.DB
    .prepare("UPDATE tenant_invitations SET accepted_at = datetime('now') WHERE token = ?")
    .bind(token)
    .run()

  return { success: true, tenantId: invitation.tenant_id }
}

/**
 * Delete invitation
 */
export async function deleteInvitation(env: Env, invitationId: number): Promise<boolean> {
  const result = await env.DB
    .prepare('DELETE FROM tenant_invitations WHERE id = ?')
    .bind(invitationId)
    .run()

  return result.success
}

/**
 * Get tenant invitations
 */
export async function getTenantInvitations(
  env: Env,
  tenantId: number
): Promise<TenantInvitation[]> {
  const result = await env.DB
    .prepare(`
      SELECT * FROM tenant_invitations
      WHERE tenant_id = ? AND accepted_at IS NULL AND expires_at > datetime('now')
      ORDER BY created_at DESC
    `)
    .bind(tenantId)
    .all<TenantInvitation>()

  return result.results
}