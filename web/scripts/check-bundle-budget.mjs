// Performance budget gate (plan §13, docs/reference/frontend-design.md
// "Bundle"). Run after `npm run build`: adds up the gzip size of what the
// first visit to each route in bundle-budget.json downloads, and of every
// lazily loaded chunk, and fails when one is over its budget.
import { appendFileSync, existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { gzipSync } from 'node:zlib'
import { fileURLToPath } from 'node:url'
import { KB, evaluateBudget } from './bundle-budget-lib.mjs'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const publicDir = path.resolve(process.env.BUNDLE_PUBLIC_DIR || path.join(webRoot, 'public'))
const graphFile = path.resolve(process.env.BUNDLE_GRAPH || path.join(webRoot, 'bundle-reports', 'chunk-graph.json'))
const budgetFile = path.join(webRoot, 'bundle-budget.json')

if (!existsSync(graphFile)) {
  console.error(`Chunk graph not found: ${graphFile}. Run npm run build first.`)
  process.exit(1)
}

const graph = JSON.parse(readFileSync(graphFile, 'utf8'))
const budget = JSON.parse(readFileSync(budgetFile, 'utf8'))

const sizes = {}
const files = new Set()
for (const [file, chunk] of Object.entries(graph.chunks)) {
  files.add(file)
  for (const css of chunk.css || []) files.add(css)
}
for (const file of files) {
  sizes[file] = gzipSync(readFileSync(path.join(publicDir, file))).length
}

const result = evaluateBudget(graph, sizes, budget)
const kb = (bytes) => `${(bytes / KB).toFixed(1)} KB`

const lines = [
  '# Frontend Performance Budget',
  '',
  '| Route (first visit) | Initial | Budget | With deferred | Budget |',
  '| --- | ---: | ---: | ---: | ---: |',
  ...result.routes.map((r) => `| ${r.name} | ${kb(r.initial)} | ${kb(r.limit)} | ${kb(r.total)} | ${Number.isFinite(r.totalLimit) ? kb(r.totalLimit) : '-'} |`),
  '',
  `Largest lazy chunks (budget ${kb(result.chunkLimit)} each; excluded: ${budget.chunks.exclude.join(', ')})`,
  '',
  '| Chunk | Module | Gzip |',
  '| --- | --- | ---: |',
  ...result.chunks.slice(0, 10).map((c) => `| ${c.file} | ${c.module || ''} | ${kb(c.gzip)} |`),
  ''
]
const markdown = lines.join('\n')
console.log(markdown)
if (process.argv.includes('--verbose')) {
  for (const r of result.routes) {
    const rows = r.files.map((f) => [sizes[f], f]).sort((a, b) => b[0] - a[0])
    console.log(`${r.name}:`)
    for (const [size, f] of rows) console.log(`  ${kb(size).padStart(9)}  ${f}${r.deferred.includes(f) ? '  (deferred)' : ''}`)
  }
}
if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${markdown}\n`)

if (result.failures.length) {
  console.error('Over budget:')
  for (const failure of result.failures) console.error(`  ${failure}`)
  process.exit(1)
}
console.log('Bundle budget ok.')
