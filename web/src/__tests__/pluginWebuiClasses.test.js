// The CSS classes signed plugin WebUI bundles (packages/<name>/webui,
// anixops.webui/v1) render are a stable contract: plugins ship separately
// from Control, so a class Control renames or drops breaks every installed
// plugin that uses it. See "WebUI CSS classes" in
// docs/architecture/plugin-kernel-contract.md.
import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const WEB = join(__dirname, '..', '..')
const PACKAGES = join(WEB, '..', 'packages')

const PLUGIN_WEBUI_CLASSES = ['btn', 'btn-secondary', 'table-container', 'data-table', 'empty-state']

const globalCss = () => readFileSync(join(WEB, 'src', 'style.css'), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '')

// Selectors of every rule (at-rule bodies included), split on commas.
function selectors(css) {
  const out = []
  for (const match of css.matchAll(/([^{}]+)\{[^{}]*\}/g)) {
    for (const selector of match[1].split(',')) out.push(selector.trim())
  }
  return out
}

function webuiBundles() {
  if (!existsSync(PACKAGES)) return []
  return readdirSync(PACKAGES, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && existsSync(join(PACKAGES, entry.name, 'webui')))
    .flatMap((entry) => readdirSync(join(PACKAGES, entry.name, 'webui'))
      .filter((file) => /\.m?js$/.test(file))
      .map((file) => join(PACKAGES, entry.name, 'webui', file)))
}

describe('plugin WebUI CSS class contract', () => {
  it('the global stylesheet is loaded by the app', () => {
    expect(readFileSync(join(WEB, 'src', 'main.js'), 'utf8')).toMatch(/import ['"]\.\/style\.css['"]/)
  })

  it.each(PLUGIN_WEBUI_CLASSES)('.%s keeps its own rule in src/style.css', (name) => {
    // A rule for the bare class, not only states (:hover) or longer
    // classes (.btn-primary).
    const rules = selectors(globalCss()).filter((selector) => selector === `.${name}`)
    expect(rules, `.${name} is part of the plugin WebUI contract and must stay in src/style.css`).not.toEqual([])
  })

  it('the bundled packages use those classes, so the check guards real plugins', () => {
    const used = new Set()
    for (const file of webuiBundles()) {
      for (const match of readFileSync(file, 'utf8').matchAll(/class:\s*'([^']+)'/g)) {
        for (const name of match[1].split(/\s+/)) used.add(name)
      }
    }
    for (const name of PLUGIN_WEBUI_CLASSES) expect(used).toContain(name)
  })
})
