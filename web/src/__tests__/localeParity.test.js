// The two locales carry the same keys, so src/i18n.js loads only the active
// one (no fallback locale up front), and the core, admin and adminPages
// message groups do not overlap, so merging them keeps every namespace whole.
import { describe, expect, it } from 'vitest'
import zhCore from '@/locales/zh-CN.js'
import zhAdmin from '@/locales/zh-CN.admin.js'
import enCore from '@/locales/en.js'
import enAdmin from '@/locales/en.admin.js'
import zhAdminPages from '@/locales/zh-CN.adminPages.js'
import enAdminPages from '@/locales/en.adminPages.js'

function flatKeys(value, prefix = '', out = []) {
  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (child && typeof child === 'object' && !Array.isArray(child)) {
      flatKeys(child, path, out)
    } else {
      out.push(path)
    }
  }
  return out
}

// legacy maps source literals of the active locale (utils/legacyI18n.js);
// its keys are the other language's text by design.
const withoutLegacy = (messages) => Object.fromEntries(Object.entries(messages).filter(([key]) => key !== 'legacy'))

describe('locale messages', () => {
  it.each([
    ['core', zhCore, enCore],
    ['admin', zhAdmin, enAdmin],
    ['adminPages', zhAdminPages, enAdminPages]
  ])('%s: zh-CN and en have the same keys', (_group, zh, en) => {
    expect(flatKeys(withoutLegacy(en)).sort()).toEqual(flatKeys(withoutLegacy(zh)).sort())
  })

  it('core, admin and adminPages groups have disjoint namespaces', () => {
    for (const [core, admin, pages] of [[zhCore, zhAdmin, zhAdminPages], [enCore, enAdmin, enAdminPages]]) {
      expect(Object.keys(core).filter((key) => key in admin || key in pages)).toEqual([])
      expect(Object.keys(admin).filter((key) => key in pages)).toEqual([])
    }
  })

  it('the admin shell group holds only what the shell and dashboard read', () => {
    expect(Object.keys(zhAdmin).sort()).toEqual(['adminDashboard', 'adminMonitor', 'adminNotify', 'adminSecurity', 'adminSettings', 'settingsForm'])
  })

  it('the sign-in and user pages need only the core group', () => {
    expect(Object.keys(zhCore).sort()).toEqual(['app', 'auth', 'common', 'layout', 'legacy', 'pageTitles', 'portal', 'shell', 'ui'])
  })
})
