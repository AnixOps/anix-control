// Pure helpers of UiDataTable: reading cells, sorting and keys. Kept out of
// the component so they are tested on their own.

export function rowKeyOf(row, rowKey, index) {
  if (typeof rowKey === 'function') return rowKey(row, index)
  const value = row?.[rowKey]
  return value === undefined || value === null ? index : value
}

export function cellValue(row, column) {
  if (typeof column.value === 'function') return column.value(row)
  return row?.[column.key]
}

export function cellText(row, column) {
  const value = cellValue(row, column)
  if (typeof column.format === 'function') return column.format(value, row)
  if (value === undefined || value === null || value === '') return '—'
  return String(value)
}

function sortKey(row, column) {
  if (typeof column.sortValue === 'function') return column.sortValue(row)
  return cellValue(row, column)
}

const collator = typeof Intl !== 'undefined' ? new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' }) : null

export function compareValues(a, b) {
  const aEmpty = a === undefined || a === null || a === ''
  const bEmpty = b === undefined || b === null || b === ''
  if (aEmpty || bEmpty) return aEmpty === bEmpty ? 0 : (aEmpty ? 1 : -1)
  if (typeof a === 'number' && typeof b === 'number') return a - b
  if (a instanceof Date && b instanceof Date) return a.getTime() - b.getTime()
  if (typeof a === 'boolean' && typeof b === 'boolean') return Number(a) - Number(b)
  const left = String(a)
  const right = String(b)
  return collator ? collator.compare(left, right) : left.localeCompare(right)
}

// sortRows returns a sorted copy; empty values go last in both directions;
// ties keep their order (Array.prototype.sort is stable).
export function sortRows(rows, columns, sort) {
  if (!sort?.key || !sort.direction) return rows
  const column = columns.find(item => item.key === sort.key)
  if (!column) return rows
  const factor = sort.direction === 'desc' ? -1 : 1
  return [...rows].sort((left, right) => {
    const a = sortKey(left, column)
    const b = sortKey(right, column)
    const aEmpty = a === undefined || a === null || a === ''
    const bEmpty = b === undefined || b === null || b === ''
    if (aEmpty || bEmpty) return compareValues(a, b)
    return factor * compareValues(a, b)
  })
}

// nextSort cycles a header: none → ascending → descending → none (a column's
// firstDirection may start at descending, e.g. dates and amounts).
export function nextSort(current, column) {
  const first = column.firstDirection === 'desc' ? 'desc' : 'asc'
  const second = first === 'asc' ? 'desc' : 'asc'
  if (current?.key !== column.key || !current.direction) return { key: column.key, direction: first }
  if (current.direction === first) return { key: column.key, direction: second }
  return { key: '', direction: '' }
}

export function ariaSort(sort, column) {
  if (!column.sortable) return undefined
  if (sort?.key !== column.key || !sort.direction) return 'none'
  return sort.direction === 'desc' ? 'descending' : 'ascending'
}
