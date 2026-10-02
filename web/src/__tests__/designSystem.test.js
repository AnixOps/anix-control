import { readFileSync } from 'node:fs'
import path from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { check, FILES, noticeText } from '../../scripts/sync-design.mjs'
import { buildBrandIcons, tokenValue } from '../../scripts/vite-brand-icons.mjs'
import BrandLockup, { BRAND_GLYPH_SVG } from '@/components/common/BrandLockup.vue'
import { buildEChartsTheme, buildGraphColors } from '@/composables/useChartTheme'

const designDir = path.resolve(__dirname, '../design')

describe('vendored AnixOps Design', () => {
  it('matches the pinned manifest', () => {
    expect(check(designDir)).toEqual([])
    const manifest = JSON.parse(readFileSync(path.join(designDir, 'manifest.json'), 'utf8'))
    expect(readFileSync(path.join(designDir, 'VERSION'), 'utf8')).toBe(`${manifest.tag}\n`)
    expect(manifest.files.map(entry => entry.path)).toEqual(Object.keys(FILES).sort())
    expect(readFileSync(path.join(designDir, 'NOTICE.md'), 'utf8')).toBe(noticeText(manifest.tag))
  })

  it('keeps tokens.css as the generated file of the pinned version', () => {
    const manifest = JSON.parse(readFileSync(path.join(designDir, 'manifest.json'), 'utf8'))
    const tokens = readFileSync(path.join(designDir, 'tokens.css'), 'utf8')
    expect(tokens.split('\n', 1)[0]).toContain(`AnixOps Design ${manifest.tag.slice(1)}.`)
    // The theme mechanism useTheme.js relies on.
    expect(tokens).toContain(':root:not([data-theme="light"])')
    expect(tokens).toContain(':root[data-theme="dark"] {')
  })
})

describe('BrandLockup', () => {
  it('inlines the vendored glyph as a decorative currentColor svg', () => {
    const vendored = readFileSync(path.join(designDir, 'brand/mark-glyph.svg'), 'utf8')
    const paths = vendored.match(/<path[^>]*\/>|<circle[^>]*\/>/g)
    expect(paths.length).toBeGreaterThan(0)
    for (const element of paths) {
      expect(BRAND_GLYPH_SVG).toContain(element)
    }
    expect(BRAND_GLYPH_SVG).not.toContain('<title>')
    expect(BRAND_GLYPH_SVG).not.toContain('role="img"')
    expect(BRAND_GLYPH_SVG).toContain('stroke="currentColor"')
  })

  it('renders AnixOps in 600 and the product word as text', () => {
    const wrapper = mount(BrandLockup)
    expect(wrapper.find('.brand-lockup-mark').attributes('aria-hidden')).toBe('true')
    expect(wrapper.find('.brand-lockup-family').text()).toBe('AnixOps')
    expect(wrapper.find('.brand-lockup-product').text()).toBe('Control')
    expect(wrapper.text()).toBe('AnixOps Control')
    expect(mount(BrandLockup, { props: { markOnly: true } }).find('.brand-lockup-name').exists()).toBe(false)
  })
})

describe('brand icons and manifest', () => {
  it('emits hashed icons, favicon.ico and a manifest with token colours', () => {
    const tokens = readFileSync(path.join(designDir, 'tokens.css'), 'utf8')
    const light = tokenValue(tokens, ':root {', '--bg')
    const dark = tokenValue(tokens, ':root[data-theme="dark"] {', '--bg')
    const icons = buildBrandIcons({ designDir, name: 'AnixOps Control', shortName: 'AnixOps' })

    expect(icons.themeColors).toEqual({ light, dark })
    expect(icons.files.has('favicon.ico')).toBe(true)
    const names = [...icons.files.keys()]
    expect(names.filter(name => name !== 'favicon.ico').every(name => /^assets\/[a-z0-9-]+-[0-9a-f]{8}\.[a-z]+$/.test(name))).toBe(true)

    const manifestName = names.find(name => name.endsWith('.webmanifest'))
    const manifest = JSON.parse(icons.files.get(manifestName).toString('utf8'))
    expect(manifest.name).toBe('AnixOps Control')
    expect(manifest.theme_color).toBe(light)
    for (const icon of manifest.icons) {
      expect(icons.files.has(`assets/${icon.src}`)).toBe(true)
    }
    expect(manifest.icons.some(icon => icon.purpose === 'maskable')).toBe(true)

    const tags = icons.tags('/')
    expect(tags.map(tag => tag.attrs.rel || tag.attrs.name)).toEqual([
      'icon', 'icon', 'apple-touch-icon', 'manifest', 'theme-color', 'theme-color'
    ])
    expect(tags.find(tag => tag.attrs.rel === 'manifest').attrs.href).toBe(`/${manifestName}`)
  })
})

describe('chart theme', () => {
  const tokens = {
    'chart-1': 'c1', 'chart-2': 'c2', 'chart-3': 'c3', 'chart-4': 'c4',
    'chart-5': 'c5', 'chart-6': 'c6', 'chart-7': 'c7', 'chart-8': 'c8',
    'label-1': 'l1', 'label-2': 'l2', 'label-3': 'l3',
    separator: 's', 'separator-strong': 'ss', 'bg-elevated': 'bg', 'font-sans': 'sans'
  }

  it('maps the chart palette and neutral roles into an ECharts theme', () => {
    const theme = buildEChartsTheme(tokens)
    expect(theme.color).toEqual(['c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8'])
    expect(theme.backgroundColor).toBe('transparent')
    expect(theme.textStyle).toEqual({ color: 'l2', fontFamily: 'sans' })
    expect(theme.tooltip.backgroundColor).toBe('bg')
    expect(theme.categoryAxis.splitLine.lineStyle.color).toBe('s')
    expect(theme.valueAxis.axisLabel.color).toBe('l2')
  })

  it('keeps ECharts defaults when the tokens are unavailable', () => {
    expect(buildEChartsTheme({}).color).toBeUndefined()
  })

  it('derives topology colours from the same tokens', () => {
    expect(buildGraphColors(tokens)).toEqual({
      relay: 'c1',
      exit: 'c7',
      node: 'c5',
      offline: 'c8',
      label: 'l1',
      edge: 'ss',
      edgeLabel: 'l2',
      surface: 'bg',
      palette: ['c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8']
    })
  })
})

describe('useTheme', () => {
  let listeners
  let systemDark

  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    listeners = []
    systemDark = false
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      get matches() { return systemDark },
      addEventListener: (_, listener) => listeners.push(listener)
    })))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('follows the system until the user picks a theme, then keeps the choice', async () => {
    systemDark = true
    const { initTheme, useTheme } = await import('@/composables/useTheme')
    initTheme()
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')

    systemDark = false
    listeners.forEach(listener => listener())
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')

    const { setTheme, currentTheme } = useTheme()
    setTheme('dark')
    systemDark = false
    listeners.forEach(listener => listener())
    expect(currentTheme.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(localStorage.getItem('v2board-theme')).toBe('dark')
  })

  it('writes data-theme="light" explicitly so a dark system cannot override the choice', async () => {
    systemDark = true
    localStorage.setItem('v2board-theme', 'light')
    const { initTheme } = await import('@/composables/useTheme')
    initTheme()
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })
})
