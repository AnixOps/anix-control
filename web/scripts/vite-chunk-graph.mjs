// Writes the chunk graph of a production build (which chunk statically or
// dynamically imports which, the CSS each chunk pulls in, and the source
// module a dynamic chunk stands for) to bundle-reports/chunk-graph.json.
// scripts/check-bundle-budget.mjs reads it to add up what a route loads.
// The file stays out of the served build output (web/public).
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'

export default function chunkGraph({ root, outFile }) {
  let graph = null
  return {
    name: 'anix-chunk-graph',
    apply: 'build',
    generateBundle(_options, bundle) {
      const chunks = {}
      for (const item of Object.values(bundle)) {
        if (item.type !== 'chunk') continue
        chunks[item.fileName] = {
          name: item.name,
          isEntry: Boolean(item.isEntry),
          isDynamicEntry: Boolean(item.isDynamicEntry),
          module: item.facadeModuleId && !item.facadeModuleId.startsWith('\0')
            ? path.relative(root, item.facadeModuleId).split(path.sep).join('/')
            : null,
          // Source modules bundled into the chunk (dependencies left out).
          modules: (item.moduleIds || Object.keys(item.modules || {}))
            .filter((id) => !id.startsWith('\0') && !id.includes('/node_modules/'))
            .map((id) => path.relative(root, id.split('?')[0]).split(path.sep).join('/'))
            .filter((id, index, all) => all.indexOf(id) === index),
          // npm packages bundled into the chunk, for reading the report.
          packages: [...new Set((item.moduleIds || Object.keys(item.modules || {}))
            .map((id) => id.match(/node_modules\/((?:@[^/]+\/)?[^/]+)/)?.[1])
            .filter(Boolean))].sort(),
          imports: [...item.imports],
          dynamicImports: [...item.dynamicImports],
          css: [...(item.viteMetadata?.importedCss || [])]
        }
      }
      graph = { chunks }
    },
    writeBundle() {
      if (!graph) return
      mkdirSync(path.dirname(outFile), { recursive: true })
      writeFileSync(outFile, JSON.stringify(graph, null, 2) + '\n')
    }
  }
}
