import { beforeAll, describe, expect, it } from 'vitest'
import i18n, { setLocale } from '@/i18n'
import { createFormatter } from '@/ui/composables/useFormat'
import { KERNEL_ALERT_KINDS, alertRoute, alertText, alertTone } from '@/views/admin/dashboard/kernelAlerts'

describe('kernel alerts on the dashboard', () => {
  it('maps severity to a tone: critical is danger, everything else warning', () => {
    expect(alertTone({ severity: 'critical' })).toBe('danger')
    expect(alertTone({ severity: 'warning' })).toBe('warning')
    expect(alertTone({})).toBe('warning')
  })

  it('links node subjects only', () => {
    expect(alertRoute({ subject_kind: 'node', subject: 'proxy-12' })).toBe('/admin/nodes/12')
    expect(alertRoute({ subject_kind: 'node', subject: 'forward-5' })).toBe('/admin/forward/inventory/forward-5')
    expect(alertRoute({ subject_kind: 'node', subject: 'proxy-x' })).toBeNull()
    expect(alertRoute({ subject_kind: 'ca', subject: 'service_ca:k1' })).toBeNull()
    expect(alertRoute({ subject_kind: 'module', subject: 'pkg#abc' })).toBeNull()
    expect(alertRoute(null)).toBeNull()
  })

  describe.each(['en', 'zh-CN'])('texts in %s', (locale) => {
    beforeAll(() => setLocale(locale))
    const t = (key, params) => i18n.global.t(key, params, { locale })
    const format = createFormatter(locale)
    const detail = { node: 'proxy-3', node_name: 'hk-1', package_id: 'identity', not_after: '2026-10-08T10:00:00Z', since: '2026-10-01T10:00:00Z', table: 'v2_node', phase: 'dual_write', ca: 'module', expired: false, next_staged: true }

    it.each(KERNEL_ALERT_KINDS)('writes a title and a hint for %s without leaving a placeholder or a key', (kind) => {
      for (const extra of [{}, { expired: true, next_staged: false, ca: 'forward_link' }]) {
        const { title, hint } = alertText({ kind, detail: { ...detail, ...extra }, status: 'active' }, { t, format })
        expect(title).not.toMatch(/[{}]|adminDashboard/)
        expect(hint).not.toMatch(/[{}]|adminDashboard/)
        expect(title.length).toBeGreaterThan(5)
        expect(hint.length).toBeGreaterThan(5)
      }
    })
  })

  it('names the node and the end date, and says when a resolved alert cleared', async () => {
    await setLocale('en')
    const t = (key, params) => i18n.global.t(key, params, { locale: 'en' })
    const format = createFormatter('en')
    const alert = { kind: 'link_certificate_expiring', detail: { node: 'forward-4', node_name: 'edge-1', not_after: '2026-10-08T10:00:00Z', expired: true }, status: 'active' }
    expect(alertText(alert, { t, format }).title).toBe('The forward link certificate of edge-1 has expired')
    expect(alertText({ ...alert, detail: { node: 'forward-4' } }, { t, format }).title).toContain('forward-4')
    const resolved = alertText({ ...alert, status: 'resolved', resolved_at: '2026-10-09T10:00:00Z' }, { t, format })
    expect(resolved.hint).toMatch(/^Resolved /)
  })

  it('falls back to the server message for an unknown kind', () => {
    const t = key => key
    expect(alertText({ kind: 'later_kind', message: 'Look at this', detail: null }, { t, format: createFormatter('en') }).title).toBe('Look at this')
    expect(alertText({ kind: 'later_kind', detail: null }, { t, format: createFormatter('en') }).title).toBe('later_kind')
  })
})
