import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// The stacking order of the layers (docs/reference/frontend-design.md, "Layer
// order"). The z-index tokens come from the vendored AnixOps Design; the one
// the design does not have, --z-popover, is defined in styles/base.css. A real
// browser decides what is on top (e2e/layers.spec.js); this pins the scale and
// the two things that make a portalled menu count in it.
const root = path.resolve(__dirname, '..')
const read = file => readFileSync(path.join(root, file), 'utf8')

function scale() {
  const values = {}
  for (const [, name, value] of read('design/tokens.css').matchAll(/--z-([a-z]+):\s*(\d+);/g)) values[name] = Number(value)
  const popover = read('styles/base.css').match(/--z-popover:\s*calc\(var\(--z-([a-z]+)\) \+ (\d+)\);/)
  expect(popover, '--z-popover must be defined in styles/base.css against a design token').not.toBeNull()
  values.popover = values[popover[1]] + Number(popover[2])
  return values
}

// The first rule of a selector in a stylesheet.
function declarations(file, selector) {
  const match = read(file).match(new RegExp(`^${selector.replace('.', '\\.')} \\{([^}]*)\\}`, 'm'))
  expect(match, `${selector} in ${file}`).not.toBeNull()
  return Object.fromEntries(match[1].split(';').map(line => line.split(/:(.+)/).map(part => part?.trim())).filter(([name]) => name))
}

describe('layer order', () => {
  it('stacks sticky < dropdown < drawer < modal < popover < toast < tooltip', () => {
    const z = scale()
    const order = ['base', 'sticky', 'dropdown', 'drawer', 'modal', 'popover', 'toast', 'tooltip'].map(name => z[name])
    expect(order.every(Number.isFinite)).toBe(true)
    expect(order).toEqual([...new Set(order)].sort((a, b) => a - b))
  })

  // Reka copies the content's z-index to its fixed popper wrapper, and a
  // static element can compute to `auto`: the layer needs position and a
  // token above the modal layer, or it opens under the scrim of the dialog,
  // sheet or drawer it was opened from.
  it.each([
    ['ui/internal/menu.css', '.ui-menu'],
    ['components/shell/menu.css', '.shell-menu'],
    ['ui/internal/listbox.css', '.ui-listbox']
  ])('%s: %s sits on --z-popover, positioned', (file, selector) => {
    const rule = declarations(file, selector)
    expect(rule['z-index']).toBe('var(--z-popover)')
    expect(rule.position).toBe('relative')
  })
})
