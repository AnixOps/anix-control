import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { cleanup, render, screen, waitFor, within } from '@testing-library/vue'
import userEvent from '@testing-library/user-event'
import UiDataTable from '../UiDataTable.vue'
import UiPagination from '../UiPagination.vue'
import UiFilterChips from '../UiFilterChips.vue'
import UiSearchField from '../UiSearchField.vue'
import { nextSort, sortRows } from '../internal/tableModel'

afterEach(() => cleanup())

const COLUMNS = [
  { key: 'email', label: 'Email', sortable: true, primary: true },
  { key: 'traffic', label: 'Traffic', sortable: true, numeric: true, align: 'end', firstDirection: 'desc' },
  { key: 'plan', label: 'Plan', secondary: true },
  { key: 'note', label: 'Note', hidden: true }
]

const ROWS = [
  { id: 1, email: 'b@example.test', traffic: 30, plan: 'Basic', note: 'x' },
  { id: 2, email: 'a@example.test', traffic: 10, plan: 'Pro', note: 'y' },
  { id: 3, email: 'c@example.test', traffic: null, plan: '', note: 'z' }
]

function bodyRows() {
  return within(screen.getByRole('table')).getAllByRole('row').slice(1)
}

function firstCells() {
  return bodyRows().map(row => within(row).getAllByRole('cell')[0].textContent.trim())
}

describe('table model', () => {
  it('sorts numbers and text with empty values last in both directions', () => {
    expect(sortRows(ROWS, COLUMNS, { key: 'traffic', direction: 'asc' }).map(row => row.id)).toEqual([2, 1, 3])
    expect(sortRows(ROWS, COLUMNS, { key: 'traffic', direction: 'desc' }).map(row => row.id)).toEqual([1, 2, 3])
    expect(sortRows(ROWS, COLUMNS, { key: 'email', direction: 'asc' }).map(row => row.id)).toEqual([2, 1, 3])
  })

  it('cycles a header through its first direction, the other, and none', () => {
    const column = COLUMNS[1]
    let sort = nextSort({}, column)
    expect(sort).toEqual({ key: 'traffic', direction: 'desc' })
    sort = nextSort(sort, column)
    expect(sort).toEqual({ key: 'traffic', direction: 'asc' })
    expect(nextSort(sort, column)).toEqual({ key: '', direction: '' })
  })
})

describe('UiDataTable', () => {
  it('is a captioned table with column headers, aria-sort and hidden columns left out', async () => {
    const user = userEvent.setup()
    render(UiDataTable, { props: { columns: COLUMNS, rows: ROWS, label: 'Users' } })
    const table = screen.getByRole('table', { name: 'Users' })
    const headers = within(table).getAllByRole('columnheader')
    expect(headers.map(th => th.textContent.trim())).toEqual(['Email', 'Traffic', 'Plan'])
    expect(headers[0].getAttribute('aria-sort')).toBe('none')
    expect(headers[2].getAttribute('aria-sort')).toBeNull()
    expect(firstCells()).toEqual(['b@example.test', 'a@example.test', 'c@example.test'])
    // Empty cells show a dash.
    expect(within(bodyRows()[2]).getAllByRole('cell')[2].textContent.trim()).toBe('—')

    await user.click(within(headers[0]).getByRole('button', { name: 'Email' }))
    expect(headers[0].getAttribute('aria-sort')).toBe('ascending')
    expect(firstCells()).toEqual(['a@example.test', 'b@example.test', 'c@example.test'])
    await user.click(within(headers[0]).getByRole('button', { name: 'Email' }))
    expect(headers[0].getAttribute('aria-sort')).toBe('descending')
    expect(firstCells()).toEqual(['c@example.test', 'b@example.test', 'a@example.test'])
  })

  it('emits the sort without reordering rows in manual (server) mode', async () => {
    const user = userEvent.setup()
    const onSort = vi.fn()
    render(UiDataTable, { props: { columns: COLUMNS, rows: ROWS, label: 'Users', manualSort: true, 'onUpdate:sort': onSort } })
    await user.click(screen.getByRole('button', { name: 'Traffic' }))
    expect(onSort).toHaveBeenCalledWith({ key: 'traffic', direction: 'desc' })
    expect(firstCells()).toEqual(['b@example.test', 'a@example.test', 'c@example.test'])
  })

  it('renders cell slots', () => {
    render({
      components: { UiDataTable },
      setup: () => ({ columns: COLUMNS, rows: ROWS }),
      template: '<UiDataTable :columns="columns" :rows="rows" label="Users"><template #cell-plan="{ row, value }"><strong>{{ value || "none" }}#{{ row.id }}</strong></template></UiDataTable>'
    })
    expect(screen.getByText('Basic#1').tagName).toBe('STRONG')
    expect(screen.getByText('none#3')).toBeTruthy()
  })

  it('pages rows on the client and moves back when the last page empties', async () => {
    const user = userEvent.setup()
    const rows = ref(Array.from({ length: 25 }, (_, index) => ({ id: index + 1, email: `u${String(index + 1).padStart(2, '0')}@example.test` })))
    render({
      components: { UiDataTable },
      setup: () => ({ columns: COLUMNS.slice(0, 1), rows }),
      template: '<UiDataTable :columns="columns" :rows="rows" label="Users" :page-size="10" />'
    })
    expect(bodyRows()).toHaveLength(10)
    expect(screen.getByText('1–10 of 25')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Page 3' }))
    expect(firstCells()).toEqual(['u21@example.test', 'u22@example.test', 'u23@example.test', 'u24@example.test', 'u25@example.test'])
    expect(screen.getByRole('button', { name: 'Page 3' }).getAttribute('aria-current')).toBe('page')
    rows.value = rows.value.slice(0, 15)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Page 2' }).getAttribute('aria-current')).toBe('page'))
  })

  it('pages on the server with manualPagination and total', async () => {
    const user = userEvent.setup()
    const onPage = vi.fn()
    render(UiDataTable, {
      props: { columns: COLUMNS, rows: ROWS, label: 'Users', pageSize: 3, total: 40, page: 1, manualPagination: true, 'onUpdate:page': onPage }
    })
    expect(bodyRows()).toHaveLength(3)
    expect(screen.getByText('1–3 of 40')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Next page' }))
    expect(onPage).toHaveBeenCalledWith(2)
  })

  it('selects rows, shows the bulk bar with the selected rows, and clears with Esc', async () => {
    const user = userEvent.setup()
    const selected = ref([])
    render({
      components: { UiDataTable },
      setup: () => ({ columns: COLUMNS, rows: ROWS, selected }),
      template: `<UiDataTable v-model:selected="selected" :columns="columns" :rows="rows" label="Users" selectable>
        <template #bulk-actions="{ rows: chosen }"><button type="button">Ban {{ chosen.length }}</button></template>
      </UiDataTable>`
    })
    expect(document.querySelector('[data-bulk-bar]')).toBeNull()
    await user.click(screen.getByRole('checkbox', { name: 'Select a@example.test' }))
    expect(selected.value).toEqual([2])
    const bar = await screen.findByRole('region', { name: 'Actions for the selected rows' })
    expect(within(bar).getByText('1 selected')).toBeTruthy()
    expect(within(bar).getByRole('button', { name: 'Ban 1' })).toBeTruthy()

    const all = screen.getByRole('checkbox', { name: 'Select all rows on this page' })
    expect(all.getAttribute('aria-checked')).toBe('mixed')
    await user.click(all)
    expect(selected.value).toEqual([2, 1, 3])
    expect(all.getAttribute('aria-checked')).toBe('true')

    within(bar).getByRole('button', { name: 'Ban 3' }).focus()
    await user.keyboard('{Escape}')
    expect(selected.value).toEqual([])
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Actions for the selected rows' })).toBeNull())
  })

  it('offers "select all N" when every row is loaded but only one page is selected', async () => {
    const user = userEvent.setup()
    const selected = ref([])
    const rows = Array.from({ length: 12 }, (_, index) => ({ id: index + 1, email: `u${index + 1}@example.test` }))
    render({
      components: { UiDataTable },
      setup: () => ({ columns: COLUMNS.slice(0, 1), rows, selected }),
      template: '<UiDataTable v-model:selected="selected" :columns="columns" :rows="rows" label="Users" selectable :page-size="5" />'
    })
    await user.click(screen.getByRole('checkbox', { name: 'Select all rows on this page' }))
    expect(selected.value).toHaveLength(5)
    await user.click(await screen.findByRole('button', { name: 'Select all 12' }))
    expect(selected.value).toHaveLength(12)
  })

  it('activates rows with click and Enter, moves with the arrows, ignores clicks on controls', async () => {
    const user = userEvent.setup()
    const onActivate = vi.fn()
    render(UiDataTable, {
      props: {
        columns: COLUMNS,
        rows: ROWS,
        label: 'Users',
        activatable: true,
        selectable: true,
        rowActions: row => [{ key: 'ban', label: `Ban ${row.email}`, onSelect: vi.fn() }],
        onRowActivate: onActivate
      }
    })
    const rows = bodyRows()
    expect(rows.map(row => row.getAttribute('tabindex'))).toEqual(['0', '-1', '-1'])
    await user.click(within(rows[1]).getAllByRole('cell')[1])
    expect(onActivate).toHaveBeenLastCalledWith(ROWS[1])
    await user.click(within(rows[0]).getByRole('checkbox'))
    expect(onActivate).toHaveBeenCalledTimes(1)

    rows[0].focus()
    await user.keyboard('{ArrowDown}')
    expect(document.activeElement).toBe(rows[1])
    await user.keyboard('{End}')
    expect(document.activeElement).toBe(rows[2])
    await user.keyboard('{Enter}')
    expect(onActivate).toHaveBeenLastCalledWith(ROWS[2])
    await user.keyboard(' ')
    expect(within(rows[2]).getByRole('checkbox').getAttribute('aria-checked')).toBe('true')
  })

  it('opens the row menu and runs an action', async () => {
    const user = userEvent.setup()
    const onBan = vi.fn()
    render(UiDataTable, {
      props: {
        columns: COLUMNS,
        rows: ROWS.slice(0, 1),
        label: 'Users',
        rowActions: () => [{ key: 'edit', label: 'Edit', onSelect: vi.fn() }, { key: 'ban', label: 'Ban', danger: true, separatorBefore: true, onSelect: onBan }]
      }
    })
    await user.click(screen.getByRole('button', { name: 'Actions for b@example.test' }))
    const menu = await screen.findByRole('menu')
    await user.click(within(menu).getByRole('menuitem', { name: 'Ban' }))
    expect(onBan).toHaveBeenCalledTimes(1)
  })

  it('shows the error state with retry, the empty state, and "no matches" with clear filters', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    const onClear = vi.fn()
    const { rerender } = render(UiDataTable, {
      props: { columns: COLUMNS, rows: [], label: 'Users', error: new Error('boom'), errorTitle: 'Users didn’t load', onRetry, 'onClear-filters': onClear }
    })
    expect(screen.getByRole('alert').textContent).toContain('boom')
    expect(screen.getByRole('heading', { name: 'Users didn’t load' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(onRetry).toHaveBeenCalledTimes(1)

    await rerender({ error: null, emptyTitle: 'No users yet' })
    expect(screen.getByRole('heading', { name: 'No users yet' })).toBeTruthy()
    expect(screen.queryByRole('table')).toBeNull()

    await rerender({ filtered: true })
    expect(screen.getByRole('heading', { name: 'No results' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(onClear).toHaveBeenCalledTimes(1)
  })

  it('shows skeleton rows only after 300 ms of loading', async () => {
    vi.useFakeTimers()
    render(UiDataTable, { props: { columns: COLUMNS, rows: [], label: 'users', loading: true } })
    expect(screen.queryByRole('status')).toBeNull()
    await vi.advanceTimersByTimeAsync(320)
    expect(screen.getByRole('status').textContent).toContain('Loading users')
  })

  it('hides columns and changes the density from the settings menu, remembered per table', async () => {
    const user = userEvent.setup()
    const { unmount } = render(UiDataTable, { props: { columns: COLUMNS, rows: ROWS, label: 'Users', storageKey: 'test.users' } })
    await user.click(screen.getByRole('button', { name: 'Table settings' }))
    const menu = await screen.findByRole('menu')
    const plan = within(menu).getByRole('menuitemcheckbox', { name: 'Plan' })
    expect(plan.getAttribute('aria-checked')).toBe('true')
    // The primary column cannot be hidden.
    expect(within(menu).queryByRole('menuitemcheckbox', { name: 'Email' })).toBeNull()
    await user.click(plan)
    await user.click(within(menu).getByRole('menuitemcheckbox', { name: 'Note' }))
    await user.click(within(menu).getByRole('menuitemradio', { name: 'Compact' }))
    expect(within(screen.getByRole('table')).getAllByRole('columnheader').map(th => th.textContent.trim())).toEqual(['Email', 'Traffic', 'Note'])
    expect(document.querySelector('.ui-data-table').dataset.density).toBe('compact')
    expect(JSON.parse(localStorage.getItem('anix.table.test.users'))).toEqual({ hidden: ['plan'], density: 'compact' })
    unmount()

    render(UiDataTable, { props: { columns: COLUMNS, rows: ROWS, label: 'Users', storageKey: 'test.users' } })
    expect(within(screen.getByRole('table')).getAllByRole('columnheader').map(th => th.textContent.trim())).toEqual(['Email', 'Traffic', 'Note'])
  })

  it('works when storage throws', () => {
    const getItem = localStorage.getItem
    localStorage.getItem = () => { throw new Error('denied') }
    try {
      render(UiDataTable, { props: { columns: COLUMNS, rows: ROWS, label: 'Users', storageKey: 'test.blocked' } })
      expect(screen.getByRole('table')).toBeTruthy()
    } finally {
      localStorage.getItem = getItem
    }
  })

  it('renders one card per row on phones', async () => {
    const user = userEvent.setup()
    const onActivate = vi.fn()
    vi.stubGlobal('matchMedia', query => ({ matches: query.includes('639.98'), media: query, addEventListener() {}, removeEventListener() {} }))
    render(UiDataTable, { props: { columns: COLUMNS, rows: ROWS, label: 'Users', activatable: true, selectable: true, onRowActivate: onActivate } })
    expect(screen.queryByRole('table')).toBeNull()
    const list = screen.getByRole('list', { name: 'Users' })
    const cards = within(list).getAllByRole('listitem')
    expect(cards).toHaveLength(3)
    // Title, subtitle, then the other visible fields with their labels.
    expect(within(cards[0]).getByText('Basic')).toBeTruthy()
    expect(within(cards[0]).getByText('Traffic')).toBeTruthy()
    await user.click(within(cards[1]).getByRole('button', { name: 'a@example.test' }))
    expect(onActivate).toHaveBeenCalledWith(ROWS[1])
    expect(within(cards[1]).getByRole('checkbox', { name: 'Select a@example.test' })).toBeTruthy()
  })
})

describe('UiPagination', () => {
  it('shows gaps for long ranges', () => {
    render(UiPagination, { props: { page: 6, total: 400, pageSize: 20 } })
    const nav = screen.getByRole('navigation', { name: 'Pages' })
    expect(within(nav).getAllByRole('listitem').map(item => item.textContent.trim())).toEqual(['1', '…', '5', '6', '7', '…', '20'])
    expect(screen.getByText('101–120 of 400')).toBeTruthy()
  })

  it('disables previous on the first page', () => {
    render(UiPagination, { props: { page: 1, total: 50, pageSize: 20 } })
    expect(screen.getByRole('button', { name: 'Previous page' }).disabled).toBe(true)
    expect(screen.getByRole('button', { name: 'Next page' }).disabled).toBe(false)
  })
})

describe('UiFilterChips', () => {
  it('is a named group of toggle buttons; pressing the active chip clears it', async () => {
    const user = userEvent.setup()
    const value = ref('')
    render({
      components: { UiFilterChips },
      setup: () => ({ value, options: [{ value: 'banned', label: 'Banned' }, { value: 'expired', label: 'Expired', count: 4 }] }),
      template: '<UiFilterChips v-model="value" label="Status" :options="options" />'
    })
    const group = screen.getByRole('group', { name: 'Status' })
    const banned = within(group).getByRole('button', { name: 'Banned' })
    expect(banned.getAttribute('aria-pressed')).toBe('false')
    await user.click(banned)
    expect(value.value).toBe('banned')
    expect(banned.getAttribute('aria-pressed')).toBe('true')
    await user.click(banned)
    expect(value.value).toBe('')
    expect(within(group).getByRole('button', { name: 'Expired 4' })).toBeTruthy()
  })

  it('supports several chips at once', async () => {
    const user = userEvent.setup()
    const value = ref([])
    render({
      components: { UiFilterChips },
      setup: () => ({ value, options: [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }] }),
      template: '<UiFilterChips v-model="value" multiple label="Tags" :options="options" />'
    })
    await user.click(screen.getByRole('button', { name: 'A' }))
    await user.click(screen.getByRole('button', { name: 'B' }))
    expect(value.value).toEqual(['a', 'b'])
  })
})

describe('UiSearchField', () => {
  it('is a search landmark; "/" focuses it, Esc clears and Enter submits', async () => {
    const user = userEvent.setup()
    const value = ref('')
    const onSubmit = vi.fn()
    render({
      components: { UiSearchField },
      setup: () => ({ value, onSubmit }),
      template: '<div><button type="button">elsewhere</button><UiSearchField v-model="value" label="Search users" @submit="onSubmit" /></div>'
    })
    expect(screen.getByRole('search')).toBeTruthy()
    const input = screen.getByRole('searchbox', { name: 'Search users' })
    screen.getByRole('button', { name: 'elsewhere' }).focus()
    await user.keyboard('/')
    expect(document.activeElement).toBe(input)
    await user.keyboard('lin{Enter}')
    expect(value.value).toBe('lin')
    expect(onSubmit).toHaveBeenLastCalledWith('lin')
    await user.keyboard('{Escape}')
    expect(value.value).toBe('')
  })
})
