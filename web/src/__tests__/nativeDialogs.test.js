// Guard for UI U4: pages give feedback through the component library
// (useToast, useConfirm, UiDialog, UiSheet), never through the browser's
// alert/confirm/prompt or a hand-rolled overlay. ESLint reports the same
// (no-alert, vue/no-restricted-class) but CI runs the tests, not the linter.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

const SRC = join(__dirname, '..')
const SCANNED = ['views', 'components', 'layouts', 'composables', 'navigation']

function files(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return name === '__tests__' ? [] : files(path)
    return /\.(vue|js)$/.test(name) ? [path] : []
  })
}

const sources = SCANNED.flatMap(dir => files(join(SRC, dir))).map(path => ({
  path: relative(SRC, path),
  text: readFileSync(path, 'utf8')
}))

function offenders(pattern) {
  return sources.flatMap(({ path, text }) => text.split('\n')
    .map((line, index) => ({ line, index }))
    .filter(({ line }) => pattern.test(line) && !/^\s*(\/\/|\*|<!--)/.test(line))
    .map(({ index }) => `${path}:${index + 1}`))
}

describe('native dialogs and hand-rolled overlays', () => {
  it('scans the app sources', () => {
    expect(sources.length).toBeGreaterThan(50)
  })

  it('never calls alert, confirm or prompt of the browser', () => {
    expect(offenders(/(?:\bwindow\.|(?<![.\w]))(?:alert|prompt)\s*\(|\bwindow\.confirm\s*\(/)).toEqual([])
  })

  it('uses UiDialog or UiSheet instead of .modal-overlay', () => {
    expect(offenders(/\bmodal-overlay\b/)).toEqual([])
  })
})
