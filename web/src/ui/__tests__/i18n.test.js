import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import en from '@/locales/modules/en/ui'
import zh from '@/locales/modules/zh-CN/ui'

function keys(object, prefix = '') {
  return Object.entries(object).flatMap(([key, value]) => (
    value && typeof value === 'object' ? keys(value, `${prefix}${key}.`) : [`${prefix}${key}`]
  ))
}

const uiDir = path.resolve(__dirname, '..')

function sources(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) return entry.name === '__tests__' || entry.name === 'stories' ? [] : sources(full)
    return /\.(vue|js)$/.test(entry.name) && !entry.name.includes('histoire') ? [full] : []
  })
}

describe('component library strings', () => {
  it('has the same keys in zh-CN and en, none empty', () => {
    expect(keys(zh.ui).sort()).toEqual(keys(en.ui).sort())
    for (const locale of [zh.ui, en.ui]) {
      for (const key of keys(locale)) {
        const value = key.split('.').reduce((node, part) => node[part], locale)
        expect(String(value).trim(), key).not.toBe('')
      }
    }
  })

  it('uses only keys that exist', () => {
    const known = new Set(keys(en.ui).map(key => `ui.${key}`))
    const used = new Set()
    for (const file of sources(uiDir)) {
      const text = readFileSync(file, 'utf8')
      for (const match of text.matchAll(/['"`](ui\.[a-zA-Z.]+)['"`]/g)) used.add(match[1])
    }
    expect(used.size).toBeGreaterThan(10)
    for (const key of used) {
      if (key.endsWith('.')) continue
      expect(known.has(key), key).toBe(true)
    }
  })

  it('uses the × glyph or the Lucide x icon for close buttons, never a letter x', () => {
    for (const file of sources(uiDir)) {
      const text = readFileSync(file, 'utf8')
      expect(/>\s*x\s*</i.test(text), file).toBe(false)
    }
  })
})
