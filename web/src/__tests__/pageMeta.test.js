import { describe, it, expect } from 'vitest'
import { resolveDocumentTitle, resolveRouteMetaDescription, resolveRoutePageTitle } from '@/utils/pageMeta'
import { CONTROL_NAME } from '@/constants/brand'

const messages = {
  'pageTitles.auth.login': 'Sign In',
  'pageTitles.user.dashboard': 'Dashboard',
  'pageTitles.admin.monitor': 'Monitor',
  'pageTitles.admin.trafficHourly': 'Hourly Traffic',
  'pageTitles.admin.forwardNodeX': 'NodeX Runtime',
  'pageTitles.admin.control': 'Control Kernel',
  'pageTitles.admin.accessGroups': 'Access Groups',
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
    expect(resolveRoutePageTitle(t, '/admin/monitor', 'Fallback')).toBe('Monitor')
    expect(resolveRoutePageTitle(t, '/admin/traffic-hourly', 'Fallback')).toBe('Hourly Traffic')
    expect(resolveRoutePageTitle(t, '/admin/forward/nodex', 'Fallback')).toBe('NodeX Runtime')
    expect(resolveRoutePageTitle(t, '/admin/control', 'Fallback')).toBe('Control Kernel')
    expect(resolveRoutePageTitle(t, '/admin/access-groups', 'Fallback')).toBe('Access Groups')
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
    expect(resolveDocumentTitle(t, '/login', CONTROL_NAME)).toBe(`Sign In | ${CONTROL_NAME}`)
    expect(resolveDocumentTitle(t, '/unknown', CONTROL_NAME)).toBe(CONTROL_NAME)
  })
})
