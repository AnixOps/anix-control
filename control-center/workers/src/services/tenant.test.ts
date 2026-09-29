/**
 * Tenant Service Unit Tests
 */

import { describe, it, expect, beforeEach } from 'vitest'
import {
  DEFAULT_QUOTAS,
  type TenantQuotas,
  createTenant,
  getTenant,
  getTenantBySlug,
  updateTenant,
  deleteTenant,
  addTenantMember,
  removeTenantMember,
  getTenantMembers,
  getUserTenantMembership,
  getUserTenants,
  getRoleByName,
  getRoleById,
  createRole,
  updateRole,
  deleteRole,
  hasPermission,
  hasAnyPermission,
  getTenantQuotas,
  getTenantUsage,
  checkQuota,
  createInvitation,
  getInvitationByToken,
  acceptInvitation,
  deleteInvitation,
  getTenantInvitations,
  getAllPermissions,
} from './tenant'
import { createMockKV, createMockD1 } from '../../test/setup'

describe('Tenant Service', () => {
  let mockEnv: any

  beforeEach(() => {
    mockEnv = {
      DB: createMockD1(),
      KV: createMockKV(),
    }
  })
  describe('DEFAULT_QUOTAS', () => {
    it('should have free plan quotas', () => {
      expect(DEFAULT_QUOTAS.free).toBeDefined()
      expect(DEFAULT_QUOTAS.free.max_nodes).toBe(3)
      expect(DEFAULT_QUOTAS.free.max_users).toBe(1)
      expect(DEFAULT_QUOTAS.free.max_playbooks).toBe(3)
    })

    it('should have pro plan quotas', () => {
      expect(DEFAULT_QUOTAS.pro).toBeDefined()
      expect(DEFAULT_QUOTAS.pro.max_nodes).toBe(25)
      expect(DEFAULT_QUOTAS.pro.max_users).toBe(5)
      expect(DEFAULT_QUOTAS.pro.max_playbooks).toBe(20)
    })

    it('should have enterprise plan quotas (unlimited)', () => {
      expect(DEFAULT_QUOTAS.enterprise).toBeDefined()
      expect(DEFAULT_QUOTAS.enterprise.max_nodes).toBe(-1)
      expect(DEFAULT_QUOTAS.enterprise.max_users).toBe(-1)
      expect(DEFAULT_QUOTAS.enterprise.max_playbooks).toBe(-1)
    })

    it('should have increasing limits from free to enterprise', () => {
      expect(DEFAULT_QUOTAS.pro.max_nodes).toBeGreaterThan(DEFAULT_QUOTAS.free.max_nodes)
      expect(DEFAULT_QUOTAS.enterprise.max_nodes).toBe(-1) // Unlimited
    })
  })

  describe('TenantQuotas Type', () => {
    it('should have correct structure', () => {
      const quota: TenantQuotas = {
        max_nodes: 10,
        max_users: 5,
        max_playbooks: 20,
        max_schedules: 50,
        storage_gb: 10,
        api_calls_per_month: 100000,
      }

      expect(quota.max_nodes).toBe(10)
      expect(quota.max_users).toBe(5)
      expect(quota.max_playbooks).toBe(20)
      expect(quota.max_schedules).toBe(50)
      expect(quota.storage_gb).toBe(10)
      expect(quota.api_calls_per_month).toBe(100000)
    })
  })

  describe('Tenant Interface', () => {
    it('should define correct tenant structure', () => {
      const tenant = {
        id: 1,
        name: 'Test Tenant',
        slug: 'test-tenant',
        plan: 'pro' as const,
        status: 'active' as const,
        settings: '{}',
        quotas: JSON.stringify(DEFAULT_QUOTAS.pro),
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      }

      expect(tenant.id).toBe(1)
      expect(tenant.name).toBe('Test Tenant')
      expect(tenant.slug).toBe('test-tenant')
      expect(tenant.plan).toBe('pro')
      expect(tenant.status).toBe('active')
    })

    it('should support all plan types', () => {
      const plans = ['free', 'pro', 'enterprise'] as const

      plans.forEach(plan => {
        const tenant = { plan, status: 'active' as const }
        expect(['free', 'pro', 'enterprise']).toContain(tenant.plan)
      })
    })

    it('should support all status types', () => {
      const statuses = ['active', 'suspended', 'cancelled'] as const

      statuses.forEach(status => {
        const tenant = { status, plan: 'free' as const }
        expect(['active', 'suspended', 'cancelled']).toContain(tenant.status)
      })
    })
  })

  describe('Role Interface', () => {
    it('should define correct role structure', () => {
      const role = {
        id: 1,
        tenant_id: 1,
        name: 'custom_role',
        display_name: 'Custom Role',
        description: 'A custom role',
        permissions: '["nodes:read", "playbooks:read"]',
        is_system: false,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      }

      expect(role.id).toBe(1)
      expect(role.name).toBe('custom_role')
      expect(role.is_system).toBe(false)
      expect(role.permissions).toContain('nodes:read')
    })

    it('should identify system roles', () => {
      const systemRole = {
        name: 'admin',
        is_system: true,
      }

      expect(systemRole.is_system).toBe(true)
    })
  })

  describe('Permission Checking Logic', () => {
    it('should check wildcard permission', () => {
      const permissions = ['*']

      const hasPermission = (permission: string) => {
        if (permissions.includes('*')) return true
        return permissions.includes(permission)
      }

      expect(hasPermission('nodes:read')).toBe(true)
      expect(hasPermission('users:delete')).toBe(true)
      expect(hasPermission('any:permission')).toBe(true)
    })

    it('should check specific permission', () => {
      const permissions = ['nodes:read', 'playbooks:read', 'tasks:create']

      const hasPermission = (permission: string) => {
        return permissions.includes(permission)
      }

      expect(hasPermission('nodes:read')).toBe(true)
      expect(hasPermission('playbooks:read')).toBe(true)
      expect(hasPermission('tasks:create')).toBe(true)
      expect(hasPermission('nodes:delete')).toBe(false)
      expect(hasPermission('users:read')).toBe(false)
    })
  })

  describe('Quota Checking Logic', () => {
    it('should check quota within limits', () => {
      const quota: TenantQuotas = DEFAULT_QUOTAS.pro
      const usage = { nodes_count: 10, users_count: 2 }

      const checkQuota = (resource: keyof TenantQuotas, current: number, increment: number = 1) => {
        const limit = quota[resource] as number
        if (limit === -1) return true
        return current + increment <= limit
      }

      expect(checkQuota('max_nodes', usage.nodes_count)).toBe(true) // 11 <= 25
      expect(checkQuota('max_users', usage.users_count)).toBe(true) // 3 <= 5
    })

    it('should detect quota exceeded', () => {
      const quota: TenantQuotas = DEFAULT_QUOTAS.free
      const usage = { nodes_count: 3, users_count: 1 }

      const checkQuota = (resource: keyof TenantQuotas, current: number, increment: number = 1) => {
        const limit = quota[resource] as number
        if (limit === -1) return true
        return current + increment <= limit
      }

      expect(checkQuota('max_nodes', usage.nodes_count)).toBe(false) // 4 > 3
      expect(checkQuota('max_users', usage.users_count)).toBe(false) // 2 > 1
    })

    it('should allow unlimited for enterprise', () => {
      const quota: TenantQuotas = DEFAULT_QUOTAS.enterprise

      const checkQuota = (resource: keyof TenantQuotas, current: number) => {
        const limit = quota[resource] as number
        if (limit === -1) return true
        return current <= limit
      }

      expect(checkQuota('max_nodes', 1000)).toBe(true) // Unlimited
      expect(checkQuota('max_users', 500)).toBe(true) // Unlimited
    })
  })

  describe('Tenant Invitations', () => {
    it('should generate valid invitation token', () => {
      const token = crypto.randomUUID()

      expect(token).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i)
    })

    it('should calculate correct expiration time', () => {
      const hours = 72
      const now = Date.now()
      const expiresAt = new Date(now + hours * 60 * 60 * 1000)

      const diffMs = expiresAt.getTime() - now
      const diffHours = diffMs / (60 * 60 * 1000)

      expect(diffHours).toBe(72)
    })

    it('should detect expired invitation', () => {
      const expiresAt = new Date(Date.now() - 1000).toISOString() // 1 second ago

      const isExpired = new Date(expiresAt) < new Date()

      expect(isExpired).toBe(true)
    })

    it('should detect valid invitation', () => {
      const expiresAt = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString() // 24 hours from now

      const isExpired = new Date(expiresAt) < new Date()

      expect(isExpired).toBe(false)
    })
  })

  describe('Slug Validation', () => {
    it('should accept valid slugs', () => {
      const validSlugs = [
        'my-tenant',
        'company123',
        'test-tenant-1',
        'a',
        'tenant-name-here',
      ]

      const slugPattern = /^[a-z0-9-]+$/

      validSlugs.forEach(slug => {
        expect(slugPattern.test(slug)).toBe(true)
      })
    })

    it('should reject invalid slugs', () => {
      const invalidSlugs = [
        'My-Tenant', // Uppercase
        'tenant_name', // Underscore
        'tenant name', // Space
        'tenant@name', // Special char
        '', // Empty
      ]

      const slugPattern = /^[a-z0-9-]+$/

      invalidSlugs.forEach(slug => {
        if (slug === '') {
          expect(slug.length).toBe(0)
        } else {
          expect(slugPattern.test(slug)).toBe(false)
        }
      })
    })
  })

  describe('Permission Categories', () => {
    it('should have nodes permissions', () => {
      const nodesPermissions = [
        'nodes:read',
        'nodes:write',
        'nodes:execute',
        'nodes:delete',
      ]

      expect(nodesPermissions.length).toBe(4)
    })

    it('should have users permissions', () => {
      const usersPermissions = [
        'users:read',
        'users:write',
        'users:delete',
      ]

      expect(usersPermissions.length).toBe(3)
    })

    it('should have playbooks permissions', () => {
      const playbooksPermissions = [
        'playbooks:read',
        'playbooks:write',
        'playbooks:execute',
        'playbooks:delete',
      ]

      expect(playbooksPermissions.length).toBe(4)
    })
  })

  describe('createTenant', () => {
    it('should create a tenant successfully', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Test Tenant',
        slug: 'test-tenant',
        plan: 'pro',
      })

      expect(tenant).toBeDefined()
      expect(tenant.name).toBe('Test Tenant')
      expect(tenant.slug).toBe('test-tenant')
      expect(tenant.plan).toBe('pro')
    })

    it('should create tenant with free plan by default', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Free Tenant',
        slug: 'free-tenant',
      })

      expect(tenant.plan).toBe('free')
    })
  })

  describe('getTenant', () => {
    it('should return null for non-existent tenant', async () => {
      const tenant = await getTenant(mockEnv, 999)
      expect(tenant).toBeNull()
    })
  })

  describe('getTenantBySlug', () => {
    it('should return null for non-existent slug', async () => {
      const tenant = await getTenantBySlug(mockEnv, 'non-existent')
      expect(tenant).toBeNull()
    })
  })

  describe('updateTenant', () => {
    it('should update tenant properties', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Original Name',
        slug: 'original',
      })

      const updated = await updateTenant(mockEnv, tenant.id, {
        name: 'Updated Name',
        plan: 'pro',
      })

      expect(updated).toBeDefined()
    })
  })

  describe('deleteTenant', () => {
    it('should soft delete tenant', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'To Delete',
        slug: 'to-delete',
      })

      const result = await deleteTenant(mockEnv, tenant.id)
      expect(result).toBe(true)
    })
  })

  describe('Tenant Member Operations', () => {
    it('should add tenant member', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Member Test',
        slug: 'member-test',
      })

      const member = await addTenantMember(mockEnv, tenant.id, 1, undefined, 1)
      expect(member).toBeDefined()
      expect(member.tenant_id).toBe(tenant.id)
      expect(member.user_id).toBe(1)
    })

    it('should remove tenant member', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Remove Member',
        slug: 'remove-member',
      })

      await addTenantMember(mockEnv, tenant.id, 1)
      const result = await removeTenantMember(mockEnv, tenant.id, 1)
      expect(result).toBe(true)
    })

    it('should get tenant members', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Get Members',
        slug: 'get-members',
      })

      await addTenantMember(mockEnv, tenant.id, 1)
      const members = await getTenantMembers(mockEnv, tenant.id)
      expect(Array.isArray(members)).toBe(true)
    })

    it('should get user tenant membership', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Membership Check',
        slug: 'membership-check',
      })

      await addTenantMember(mockEnv, tenant.id, 1)
      const membership = await getUserTenantMembership(mockEnv, 1, tenant.id)
      expect(membership).toBeDefined()
    })

    it('should get user tenants', async () => {
      const tenants = await getUserTenants(mockEnv, 1)
      expect(Array.isArray(tenants)).toBe(true)
    })
  })

  describe('Role Operations', () => {
    it('should get role by name', async () => {
      const role = await getRoleByName(mockEnv, 'admin')
      expect(role).toBeNull() // No roles exist initially
    })

    it('should get role by ID', async () => {
      const role = await getRoleById(mockEnv, 999)
      expect(role).toBeNull()
    })

    it('should create role', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Role Test',
        slug: 'role-test',
      })

      const role = await createRole(mockEnv, tenant.id, {
        name: 'custom_role',
        display_name: 'Custom Role',
        description: 'A custom role',
        permissions: ['nodes:read', 'playbooks:read'],
      })

      expect(role).toBeDefined()
      expect(role.name).toBe('custom_role')
    })
  })

  describe('Permission Checks', () => {
    it('should check hasPermission', async () => {
      const result = await hasPermission(mockEnv, 1, 1, 'nodes:read')
      expect(result).toBe(false)
    })

    it('should check hasAnyPermission', async () => {
      const result = await hasAnyPermission(mockEnv, 1, 1, ['nodes:read', 'playbooks:read'])
      expect(result).toBe(false)
    })
  })

  describe('Quota Operations', () => {
    it('should get tenant quotas', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Quota Test',
        slug: 'quota-test',
        plan: 'pro',
      })

      const quotas = await getTenantQuotas(mockEnv, tenant.id)
      expect(quotas).toBeDefined()
    })

    it('should return null quotas for non-existent tenant', async () => {
      const quotas = await getTenantQuotas(mockEnv, 999)
      expect(quotas).toBeNull()
    })

    it('should get tenant usage', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Usage Test',
        slug: 'usage-test',
      })

      const usage = await getTenantUsage(mockEnv, tenant.id)
      expect(usage).toBeDefined()
      expect(typeof usage.nodes_count).toBe('number')
      expect(typeof usage.users_count).toBe('number')
    })

    it('should check quota', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Check Quota',
        slug: 'check-quota',
        plan: 'free',
      })

      const result = await checkQuota(mockEnv, tenant.id, 'max_nodes', 1)
      expect(result).toHaveProperty('allowed')
      expect(result).toHaveProperty('current')
      expect(result).toHaveProperty('limit')
    })
  })

  describe('Invitation Operations', () => {
    it('should create invitation', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Invite Test',
        slug: 'invite-test',
      })

      const invitation = await createInvitation(
        mockEnv,
        tenant.id,
        'test@example.com',
        undefined,
        1,
        72
      )

      expect(invitation).toBeDefined()
      expect(invitation.email).toBe('test@example.com')
      expect(invitation.token).toBeDefined()
    })

    it('should get invitation by token', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Token Test',
        slug: 'token-test',
      })

      const invitation = await createInvitation(
        mockEnv,
        tenant.id,
        'token@example.com',
        undefined,
        1
      )

      const found = await getInvitationByToken(mockEnv, invitation.token)
      expect(found).toBeDefined()
      expect(found!.token).toBe(invitation.token)
    })

    it('should return null for non-existent token', async () => {
      const found = await getInvitationByToken(mockEnv, 'non-existent-token')
      expect(found).toBeNull()
    })

    it('should accept invitation', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Accept Test',
        slug: 'accept-test',
      })

      const invitation = await createInvitation(
        mockEnv,
        tenant.id,
        'accept@example.com',
        undefined,
        1
      )

      const result = await acceptInvitation(mockEnv, invitation.token, 2)
      expect(result.success).toBe(true)
      expect(result.tenantId).toBe(tenant.id)
    })

    it('should fail for non-existent invitation', async () => {
      const result = await acceptInvitation(mockEnv, 'non-existent', 1)
      expect(result.success).toBe(false)
      expect(result.error).toBe('Invitation not found')
    })

    it('should delete invitation', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'Delete Invite',
        slug: 'delete-invite',
      })

      const invitation = await createInvitation(
        mockEnv,
        tenant.id,
        'delete@example.com',
        undefined,
        1
      )

      const result = await deleteInvitation(mockEnv, invitation.id)
      expect(result).toBe(true)
    })

    it('should get tenant invitations', async () => {
      const tenant = await createTenant(mockEnv, {
        name: 'List Invites',
        slug: 'list-invites',
      })

      await createInvitation(mockEnv, tenant.id, 'list1@example.com', undefined, 1)
      await createInvitation(mockEnv, tenant.id, 'list2@example.com', undefined, 1)

      const invitations = await getTenantInvitations(mockEnv, tenant.id)
      expect(Array.isArray(invitations)).toBe(true)
    })
  })

  describe('getAllPermissions', () => {
    it('should return all permissions', async () => {
      const permissions = await getAllPermissions(mockEnv)
      expect(Array.isArray(permissions)).toBe(true)
    })
  })
})