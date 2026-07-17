import { existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync, appendFileSync } from 'node:fs'
import path from 'node:path'
import { gzipSync } from 'node:zlib'
import { fileURLToPath } from 'node:url'

const scriptDir = path.dirname(fileURLToPath(import.meta.url))
const webRoot = path.resolve(scriptDir, '..')
const publicDir = path.resolve(process.env.BUNDLE_PUBLIC_DIR || path.join(webRoot, 'public'))
const assetsDir = path.join(publicDir, 'assets')
const reportDir = path.resolve(process.env.BUNDLE_REPORT_DIR || path.join(webRoot, 'bundle-reports'))

const trackedChunkPatterns = [
  ['Forward', /^Forward-/],
  ['Tunnel', /^Tunnel-/],
  ['Users', /^Users-/],
  ['Nodes', /^Nodes-/],
  ['NodeX', /^NodeX-/],
  ['LocalRuntime', /^LocalRuntime-/],
  ['AnsibleMachines', /^AnsibleMachines-/],
  ['TrafficHourly', /^TrafficHourly-/],
  ['Observability', /^Observability-/],
  ['System', /^System-/],
  ['forwardRuntime', /^forwardRuntime-/],
  ['echarts', /^echarts-/],
  ['g6', /^g6-/],
  ['vendor', /^vendor-/],
  ['vue-vendor', /^vue-vendor-/]
]

function fail(message) {
  console.error(message)
  process.exit(1)
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return '0 B'
  }
  if (bytes < 1024) {
    return `${bytes} B`
  }
  const units = ['KiB', 'MiB', 'GiB']
  let value = bytes / 1024
  for (const unit of units) {
    if (value < 1024 || unit === units[units.length - 1]) {
      return `${value.toFixed(value >= 10 ? 1 : 2)} ${unit}`
    }
    value /= 1024
  }
  return `${bytes} B`
}

function walkFiles(dir) {
  const entries = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      entries.push(...walkFiles(fullPath))
      continue
    }
    if (entry.isFile()) {
      entries.push(fullPath)
    }
  }
  return entries
}

function assetType(filename) {
  const ext = path.extname(filename).toLowerCase()
  if (ext === '.js') {
    return 'js'
  }
  if (ext === '.css') {
    return 'css'
  }
  return 'other'
}

function trackedLabel(filename) {
  for (const [label, pattern] of trackedChunkPatterns) {
    if (pattern.test(filename)) {
      return label
    }
  }
  return ''
}

function addTotals(target, rawBytes, gzipBytes) {
  target.rawBytes += rawBytes
  target.gzipBytes += gzipBytes
  target.count += 1
}

if (!existsSync(assetsDir)) {
  fail(`Vite assets directory not found: ${assetsDir}. Run npm run build first.`)
}

const assets = walkFiles(assetsDir)
  .map((assetPath) => {
    const relativePath = path.relative(publicDir, assetPath).split(path.sep).join('/')
    const filename = path.basename(assetPath)
    const content = readFileSync(assetPath)
    const rawBytes = statSync(assetPath).size
    const gzipBytes = gzipSync(content).length
    return {
      path: relativePath,
      filename,
      type: assetType(filename),
      label: trackedLabel(filename),
      rawBytes,
      gzipBytes
    }
  })
  .sort((a, b) => a.path.localeCompare(b.path))

if (assets.length === 0) {
  fail(`No bundle assets found in ${assetsDir}. Run npm run build first.`)
}

const totals = {
  generatedAt: new Date().toISOString(),
  publicDir,
  assetCount: assets.length,
  rawBytes: 0,
  gzipBytes: 0,
  byType: {
    js: { count: 0, rawBytes: 0, gzipBytes: 0 },
    css: { count: 0, rawBytes: 0, gzipBytes: 0 },
    other: { count: 0, rawBytes: 0, gzipBytes: 0 }
  },
  trackedChunks: {}
}

for (const asset of assets) {
  totals.rawBytes += asset.rawBytes
  totals.gzipBytes += asset.gzipBytes
  addTotals(totals.byType[asset.type], asset.rawBytes, asset.gzipBytes)

  if (asset.label) {
    if (!totals.trackedChunks[asset.label]) {
      totals.trackedChunks[asset.label] = { count: 0, rawBytes: 0, gzipBytes: 0, assets: [] }
    }
    addTotals(totals.trackedChunks[asset.label], asset.rawBytes, asset.gzipBytes)
    totals.trackedChunks[asset.label].assets.push(asset.path)
  }
}

const largestAssets = [...assets]
  .sort((a, b) => b.gzipBytes - a.gzipBytes || b.rawBytes - a.rawBytes || a.path.localeCompare(b.path))
  .slice(0, 20)

const trackedRows = Object.entries(totals.trackedChunks)
  .sort(([, a], [, b]) => b.gzipBytes - a.gzipBytes)

function tableRow(cells) {
  return `| ${cells.join(' | ')} |`
}

const markdown = [
  '# Frontend Bundle Size Report',
  '',
  `Generated: ${totals.generatedAt}`,
  '',
  tableRow(['Scope', 'Files', 'Raw', 'Gzip']),
  tableRow(['---', '---:', '---:', '---:']),
  tableRow(['All assets', String(totals.assetCount), formatBytes(totals.rawBytes), formatBytes(totals.gzipBytes)]),
  tableRow(['JavaScript', String(totals.byType.js.count), formatBytes(totals.byType.js.rawBytes), formatBytes(totals.byType.js.gzipBytes)]),
  tableRow(['CSS', String(totals.byType.css.count), formatBytes(totals.byType.css.rawBytes), formatBytes(totals.byType.css.gzipBytes)]),
  tableRow(['Other', String(totals.byType.other.count), formatBytes(totals.byType.other.rawBytes), formatBytes(totals.byType.other.gzipBytes)]),
  '',
  '## Tracked Heavy/Admin Chunks',
  '',
  tableRow(['Chunk', 'Files', 'Raw', 'Gzip']),
  tableRow(['---', '---:', '---:', '---:']),
  ...trackedRows.map(([label, row]) => tableRow([label, String(row.count), formatBytes(row.rawBytes), formatBytes(row.gzipBytes)])),
  '',
  '## Largest Assets By Gzip Size',
  '',
  tableRow(['Asset', 'Type', 'Raw', 'Gzip']),
  tableRow(['---', '---', '---:', '---:']),
  ...largestAssets.map((asset) => tableRow([asset.path, asset.type, formatBytes(asset.rawBytes), formatBytes(asset.gzipBytes)])),
  ''
].join('\n')

mkdirSync(reportDir, { recursive: true })
writeFileSync(path.join(reportDir, 'bundle-size.json'), JSON.stringify({ totals, largestAssets, assets }, null, 2) + '\n')
writeFileSync(path.join(reportDir, 'bundle-size.md'), markdown)

console.log(`Bundle assets: ${totals.assetCount}`)
console.log(`Raw size: ${formatBytes(totals.rawBytes)}`)
console.log(`Gzip size: ${formatBytes(totals.gzipBytes)}`)
console.log(`Report written to ${reportDir}`)

if (process.env.GITHUB_STEP_SUMMARY) {
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${markdown}\n`)
}
