// Pure helpers for scripts/check-bundle-budget.mjs (unit-tested in
// src/__tests__/bundleBudget.test.js). A chunk graph is
// { chunks: { [fileName]: { isEntry, module, imports, dynamicImports, css } } }
// as written by scripts/vite-chunk-graph.mjs.

export const KB = 1024

// Every file that loads with the given chunks: the chunks, their static
// imports (recursively) and the CSS of each. Dynamic imports are not followed.
export function staticClosure(graph, startFiles) {
  const files = new Set()
  const stack = [...startFiles]
  while (stack.length) {
    const file = stack.pop()
    if (files.has(file)) continue
    const chunk = graph.chunks[file]
    if (!chunk) throw new Error(`chunk not in graph: ${file}`)
    files.add(file)
    for (const css of chunk.css || []) files.add(css)
    for (const dep of chunk.imports || []) stack.push(dep)
  }
  return files
}

export function entryFiles(graph) {
  return Object.entries(graph.chunks).filter(([, chunk]) => chunk.isEntry).map(([file]) => file)
}

// The chunk a source module ends up in: the chunk it is the facade of (a
// route component), else the chunk that bundles it (a manual chunk). A
// `chunk:<name>` reference names a vendor chunk (`chunk:axios`).
export function chunkForModule(graph, modulePath) {
  const entries = Object.entries(graph.chunks)
  const match = modulePath.startsWith('chunk:')
    ? entries.find(([, chunk]) => chunk.name === modulePath.slice('chunk:'.length))
    : entries.find(([, chunk]) => chunk.module === modulePath) ||
      entries.find(([, chunk]) => (chunk.modules || []).includes(modulePath))
  if (!match) throw new Error(`no chunk for module ${modulePath}`)
  return match[0]
}

// Files a first visit to a route downloads before it renders: the entry with
// its static imports, the modules awaited before mount (boot: the default
// locale) and the route's own chunks (layout and page) with theirs.
export function routeFiles(graph, { boot = [], modules = [] }) {
  const start = [...entryFiles(graph), ...[...boot, ...modules].map((m) => chunkForModule(graph, m))]
  return staticClosure(graph, start)
}

export function sumSizes(files, sizes) {
  let total = 0
  for (const file of files) {
    const size = sizes[file]
    if (size === undefined) throw new Error(`no size for ${file}`)
    total += size
  }
  return total
}

// Lazily loaded chunks with what they add on top of their own file: the
// chunk's gzip size plus its CSS. Shared vendor chunks listed in `exclude`
// (echarts, g6) are reported separately by the caller.
export function lazyChunks(graph, sizes, { exclude = [] } = {}) {
  const excluded = exclude.map((pattern) => new RegExp(pattern))
  return Object.entries(graph.chunks)
    .filter(([file, chunk]) => !chunk.isEntry && !excluded.some((re) => re.test(file)))
    .map(([file, chunk]) => ({
      file,
      module: chunk.module,
      gzip: sumSizes([file, ...(chunk.css || [])], sizes)
    }))
    .sort((a, b) => b.gzip - a.gzip || a.file.localeCompare(b.file))
}

// Compare measured routes and chunks against the budget file. Returns the
// rows for the report and the list of failures. A route's `initial` is what
// it needs to render; `total` adds what it fetches right after (`after`:
// axios for the first API call, the admin messages the router loads).
export function evaluateBudget(graph, sizes, budget) {
  const failures = []
  const kb = (bytes) => `${(bytes / KB).toFixed(1)} KB`
  const routes = Object.entries(budget.routes).map(([name, route]) => {
    const files = routeFiles(graph, { boot: budget.boot, modules: route.modules })
    const allFiles = routeFiles(graph, { boot: budget.boot, modules: [...route.modules, ...(route.after || [])] })
    const initial = sumSizes(files, sizes)
    const total = sumSizes(allFiles, sizes)
    const limit = route.maxGzipKB * KB
    const totalLimit = (route.maxTotalGzipKB ?? Infinity) * KB
    if (initial > limit) failures.push(`route ${name}: initial ${kb(initial)} gzip > ${route.maxGzipKB} KB`)
    if (total > totalLimit) failures.push(`route ${name}: total ${kb(total)} gzip > ${route.maxTotalGzipKB} KB`)
    return { name, initial, total, limit, totalLimit, files: [...allFiles].sort(), deferred: [...allFiles].filter((f) => !files.has(f)).sort() }
  })
  const chunks = lazyChunks(graph, sizes, { exclude: budget.chunks.exclude })
  const chunkLimit = budget.chunks.maxGzipKB * KB
  for (const chunk of chunks) {
    if (chunk.gzip > chunkLimit) {
      failures.push(`chunk ${chunk.file}: ${kb(chunk.gzip)} gzip > ${budget.chunks.maxGzipKB} KB`)
    }
  }
  return { routes, chunks, chunkLimit, failures }
}
