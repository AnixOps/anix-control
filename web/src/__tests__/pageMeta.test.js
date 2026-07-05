import { describe, it, expect } from 'vitest'
import { resolveDocumentTitle, resolveRouteMetaDescription, resolveRoutePageTitle } from '@/utils/pageMeta'

const messages = {
  'pageTitles.auth.login': 'Sign In',
  'pageTitles.user.dashboard': 'Dashboard',
  'pageTitles.admin.forwardNodeX': 'NodeX Runtime',
  'app.meta.defaultDescription': 'Default app description',
  'app.meta.loginDescription': 'Login page description',
  'app.meta.userDescription': 'User workspace description',
  'app.meta.adminDescription': 'Admin workspace description',
  'app.meta.forwardDescription': 'Forward workspace description'
}

function t(key) {
  return messages[key] || key
}

describe('pageMeta helpers', () => {
  it('resolves known page titles by route', () => {
    expect(resolveRoutePageTitle(t, '/login', 'Fallback')).toBe('Sign In')
    expect(resolveRoutePageTitle(t, '/user/dashboard', 'Fallback')).toBe('Dashboard')
    expect(resolveRoutePageTitle(t, '/admin/forward/nodex', 'Fallback')).toBe('NodeX Runtime')
  })

  it('falls back when a route is not mapped', () => {
    expect(resolveRoutePageTitle(t, '/unknown', 'Fallback')).toBe('Fallback')
  })

  it('derives route-aware meta descriptions by route prefix', () => {
    expect(resolveRouteMetaDescription(t, '/login', 'Fallback')).toBe('Login page description')
    expect(resolveRouteMetaDescription(t, '/user/orders', 'Fallback')).toBe('User workspace description')
    expect(resolveRouteMetaDescription(t, '/admin/dashboard', 'Fallback')).toBe('Admin workspace description')
    expect(resolveRouteMetaDescription(t, '/admin/forward/local', 'Fallback')).toBe('Forward workspace description')
    expect(resolveRouteMetaDescription(t, '/misc', 'Fallback')).toBe('Fallback')
  })

  it('builds a document title that includes the app name when needed', () => {
    expect(resolveDocumentTitle(t, '/login', 'AnixOps Studio')).toBe('Sign In | AnixOps Studio')
    expect(resolveDocumentTitle(t, '/unknown', 'AnixOps Studio')).toBe('AnixOps Studio')
  })
})
