// Vite plugin: ships the vendored AnixOps favicon and app icons
// (src/design/brand/) and a web app manifest, and adds their tags to
// index.html.
//
// web/public/ is the build output (publicDir is off) and the Go frontend
// server only serves /assets/* (immutable cache) and /favicon.ico, so the
// icons and the manifest are emitted under assets/ with content-hashed names
// and favicon.ico at the root. Theme colours are read from the vendored
// tokens.css, so nothing here hard-codes a colour.

import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import path from 'node:path'

const ICONS = [
  { key: 'svg', file: 'brand/favicon.svg', type: 'image/svg+xml' },
  { key: 'apple', file: 'brand/favicon/apple-touch-icon.png', type: 'image/png' },
  { key: 'icon192', file: 'brand/favicon/icon-192.png', type: 'image/png', sizes: '192x192' },
  { key: 'icon512', file: 'brand/favicon/icon-512.png', type: 'image/png', sizes: '512x512' },
  { key: 'maskable', file: 'brand/favicon/icon-maskable-512.png', type: 'image/png', sizes: '512x512', purpose: 'maskable' }
]

function shortHash(buffer) {
  return createHash('sha256').update(buffer).digest('hex').slice(0, 8)
}

function hashedName(file, buffer) {
  const ext = path.extname(file)
  return `assets/${path.basename(file, ext)}-${shortHash(buffer)}${ext}`
}

// tokenValue reads one custom property from a block of the generated
// tokens.css: ':root {' (light) or ':root[data-theme="dark"] {' (dark).
export function tokenValue(css, blockStart, name) {
  const start = css.indexOf(`${blockStart}\n`)
  if (start < 0) throw new Error(`tokens.css: block ${blockStart} not found`)
  const block = css.slice(start, css.indexOf('\n}', start))
  const match = block.match(new RegExp(`\\n\\s*${name}:\\s*([^;]+);`))
  if (!match) throw new Error(`tokens.css: ${name} not found in ${blockStart}`)
  return match[1].trim()
}

export function buildBrandIcons({ designDir, name, shortName, description = '' }) {
  const read = file => readFileSync(path.join(designDir, file))
  const tokens = read('tokens.css').toString('utf8')
  const light = tokenValue(tokens, ':root {', '--bg')
  const dark = tokenValue(tokens, ':root[data-theme="dark"] {', '--bg')

  const files = new Map()
  const icons = {}
  for (const icon of ICONS) {
    const source = read(icon.file)
    const fileName = hashedName(icon.file, source)
    files.set(fileName, source)
    icons[icon.key] = { ...icon, fileName }
  }
  files.set('favicon.ico', read('brand/favicon/favicon.ico'))

  // Icon URLs in a manifest resolve against the manifest's own URL, and both
  // live in assets/, so plain file names work under any base path.
  const manifest = Buffer.from(`${JSON.stringify({
    name,
    short_name: shortName,
    description,
    start_url: '../',
    scope: '../',
    display: 'standalone',
    background_color: light,
    theme_color: light,
    icons: ['icon192', 'icon512', 'maskable'].map(key => ({
      src: path.basename(icons[key].fileName),
      sizes: icons[key].sizes,
      type: icons[key].type,
      ...(icons[key].purpose ? { purpose: icons[key].purpose } : {})
    }))
  }, null, 2)}\n`)
  const manifestFile = hashedName('manifest.webmanifest', manifest)
  files.set(manifestFile, manifest)

  return {
    files,
    themeColors: { light, dark },
    tags(base = '/') {
      const href = fileName => `${base}${fileName}`
      const link = attrs => ({ tag: 'link', attrs, injectTo: 'head' })
      const meta = attrs => ({ tag: 'meta', attrs, injectTo: 'head' })
      return [
        link({ rel: 'icon', href: href('favicon.ico'), sizes: '48x48' }),
        link({ rel: 'icon', href: href(icons.svg.fileName), type: 'image/svg+xml' }),
        link({ rel: 'apple-touch-icon', href: href(icons.apple.fileName) }),
        link({ rel: 'manifest', href: href(manifestFile) }),
        meta({ name: 'theme-color', content: light, media: '(prefers-color-scheme: light)' }),
        meta({ name: 'theme-color', content: dark, media: '(prefers-color-scheme: dark)' })
      ]
    }
  }
}

const CONTENT_TYPES = {
  '.ico': 'image/x-icon',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.webmanifest': 'application/manifest+json'
}

export default function brandIcons(options) {
  let base = '/'
  let result
  const load = () => (result ??= buildBrandIcons(options))
  return {
    name: 'anixops-brand-icons',
    configResolved(config) {
      base = config.base || '/'
    },
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const pathname = (req.url || '').split('?')[0]
        const relative = pathname.startsWith(base) ? pathname.slice(base.length) : ''
        const content = relative && load().files.get(relative)
        if (!content) return next()
        res.setHeader('Content-Type', CONTENT_TYPES[path.extname(relative)] || 'application/octet-stream')
        res.end(content)
      })
    },
    transformIndexHtml() {
      return load().tags(base)
    },
    generateBundle() {
      for (const [fileName, source] of load().files) {
        this.emitFile({ type: 'asset', fileName, source })
      }
    }
  }
}
