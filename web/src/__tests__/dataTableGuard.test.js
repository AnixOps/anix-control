// Guard for UI U6: pages on the list page template keep their tables in
// UiDataTable (sorting, selection, states, phone cards, a11y) instead of
// going back to a bare <table>. ESLint reports the same, but CI runs the tests.
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { DATA_TABLE_PAGES } from '../../scripts/data-table-pages.mjs'

const WEB = join(__dirname, '..', '..')

describe('list pages use UiDataTable', () => {
  it.each(DATA_TABLE_PAGES)('%s has no bare <table> and no legacy .data-table', (file) => {
    const template = readFileSync(join(WEB, file), 'utf8').split('<script')[0]
    expect(template).not.toMatch(/<table[\s>]/)
    expect(template).not.toMatch(/class="[^"]*\bdata-table\b/)
  })

  it.each(DATA_TABLE_PAGES)('%s renders a UiDataTable or the card grid of the template', (file) => {
    const text = readFileSync(join(WEB, file), 'utf8')
    expect(text).toMatch(/UiDataTable|UiPageHeader/)
  })
})
