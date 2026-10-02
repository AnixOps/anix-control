// The two locales carry the same keys, so src/i18n.js loads only the active
// one (no fallback locale up front), and the core and admin message groups
// do not overlap, so merging them keeps every namespace whole.
import { describe, expect, it } from 'vitest'
import zhCore from '@/locales/zh-CN.js'
import zhAdmin from '@/locales/zh-CN.admin.js'
import enCore from '@/locales/en.js'
import enAdmin from '@/locales/en.admin.js'

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
    ['admin', zhAdmin, enAdmin]
  ])('%s: zh-CN and en have the same keys', (_group, zh, en) => {
    expect(flatKeys(withoutLegacy(en)).sort()).toEqual(flatKeys(withoutLegacy(zh)).sort())
  })

  it('core and admin groups have disjoint namespaces', () => {
    for (const [core, admin] of [[zhCore, zhAdmin], [enCore, enAdmin]]) {
      expect(Object.keys(core).filter((key) => key in admin)).toEqual([])
    }
  })

  it('the sign-in and user pages need only the core group', () => {
    expect(Object.keys(zhCore).sort()).toEqual(['app', 'auth', 'common', 'layout', 'legacy', 'pageTitles', 'portal', 'shell', 'ui'])
  })
})
