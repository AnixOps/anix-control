import { describe, it, expect } from 'vitest'

// Utility functions for date/time formatting
const formatDate = (date) => {
  if (!date) return '-'
  return new Date(date).toLocaleDateString()
}

const formatTime = (date) => {
  if (!date) return '-'
  return new Date(date).toLocaleTimeString()
}

const formatDateTime = (date) => {
  if (!date) return '-'
  return new Date(date).toLocaleString()
}

const getRelativeTime = (date) => {
  if (!date) return '-'
  const now = new Date()
  const then = new Date(date)
  const diffMs = now - then
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHour = Math.floor(diffMin / 60)
  const diffDay = Math.floor(diffHour / 24)

  if (diffSec < 60) return 'Just now'
  if (diffMin < 60) return `${diffMin}m ago`
  if (diffHour < 24) return `${diffHour}h ago`
  if (diffDay < 7) return `${diffDay}d ago`
  return formatDate(date)
}

// Utility functions for size formatting
const formatBytes = (bytes, decimals = 2) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`
}

describe('Date Formatting Utilities', () => {
  it('formats date correctly', () => {
    const date = '2026-03-20T10:00:00Z'
    const result = formatDate(date)

    expect(result).toBeTruthy()
    expect(result).not.toBe('-')
  })

  it('handles null date', () => {
    expect(formatDate(null)).toBe('-')
    expect(formatDate(undefined)).toBe('-')
    expect(formatDate('')).toBe('-')
  })

  it('formats time correctly', () => {
    const date = '2026-03-20T10:30:00Z'
    const result = formatTime(date)

    expect(result).toBeTruthy()
  })

  it('formats datetime correctly', () => {
    const date = '2026-03-20T10:30:00Z'
    const result = formatDateTime(date)

    expect(result).toBeTruthy()
  })
})

describe('Relative Time Utilities', () => {
  it('shows "Just now" for recent times', () => {
    const date = new Date(Date.now() - 30000).toISOString() // 30 seconds ago
    expect(getRelativeTime(date)).toBe('Just now')
  })

  it('shows minutes for times within an hour', () => {
    const date = new Date(Date.now() - 5 * 60000).toISOString() // 5 minutes ago
    expect(getRelativeTime(date)).toBe('5m ago')
  })

  it('shows hours for times within a day', () => {
    const date = new Date(Date.now() - 3 * 3600000).toISOString() // 3 hours ago
    expect(getRelativeTime(date)).toBe('3h ago')
  })

  it('shows days for times within a week', () => {
    const date = new Date(Date.now() - 2 * 86400000).toISOString() // 2 days ago
    expect(getRelativeTime(date)).toBe('2d ago')
  })

  it('handles null date', () => {
    expect(getRelativeTime(null)).toBe('-')
  })
})

describe('Byte Formatting Utilities', () => {
  it('formats bytes correctly', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(1024)).toBe('1 KB')
    expect(formatBytes(1048576)).toBe('1 MB')
    expect(formatBytes(1073741824)).toBe('1 GB')
  })

  it('formats with custom decimals', () => {
    expect(formatBytes(1536, 0)).toBe('2 KB')
    expect(formatBytes(1536, 1)).toBe('1.5 KB')
    expect(formatBytes(1536, 2)).toBe('1.5 KB')
  })

  it('handles large values', () => {
    expect(formatBytes(1099511627776)).toBe('1 TB')
    expect(formatBytes(1125899906842624)).toBe('1 PB')
  })

  it('handles small values', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(100)).toBe('100 B')
  })
})

describe('String Utilities', () => {
  const truncate = (str, length) => {
    if (!str) return ''
    if (str.length <= length) return str
    return str.substring(0, length) + '...'
  }

  const capitalize = (str) => {
    if (!str) return ''
    return str.charAt(0).toUpperCase() + str.slice(1).toLowerCase()
  }

  const slugify = (str) => {
    if (!str) return ''
    return str
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/(^-|-$)/g, '')
  }

  it('truncates long strings', () => {
    expect(truncate('Hello World', 5)).toBe('Hello...')
    expect(truncate('Short', 10)).toBe('Short')
  })

  it('handles null/empty in truncate', () => {
    expect(truncate(null, 10)).toBe('')
    expect(truncate('', 10)).toBe('')
  })

  it('capitalizes strings', () => {
    expect(capitalize('hello')).toBe('Hello')
    expect(capitalize('HELLO')).toBe('Hello')
    expect(capitalize('hELLO')).toBe('Hello')
  })

  it('slugifies strings', () => {
    expect(slugify('Hello World')).toBe('hello-world')
    expect(slugify('Test Name!@#')).toBe('test-name')
    expect(slugify('--test--')).toBe('test')
  })
})

describe('Array Utilities', () => {
  const unique = (arr) => [...new Set(arr)]

  const groupBy = (arr, key) => {
    return arr.reduce((groups, item) => {
      const value = item[key]
      groups[value] = groups[value] || []
      groups[value].push(item)
      return groups
    }, {})
  }

  it('returns unique values', () => {
    expect(unique([1, 2, 2, 3, 3, 3])).toEqual([1, 2, 3])
    expect(unique(['a', 'b', 'a', 'c'])).toEqual(['a', 'b', 'c'])
  })

  it('groups items by key', () => {
    const items = [
      { id: 1, category: 'a' },
      { id: 2, category: 'b' },
      { id: 3, category: 'a' },
    ]

    const groups = groupBy(items, 'category')

    expect(groups.a.length).toBe(2)
    expect(groups.b.length).toBe(1)
  })
})